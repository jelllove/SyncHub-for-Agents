package desktop

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/qinqingxu/synchub-for-agents/internal/cli"
	"github.com/qinqingxu/synchub-for-agents/internal/config"
	"github.com/qinqingxu/synchub-for-agents/internal/daemon"
	"github.com/qinqingxu/synchub-for-agents/internal/provider"
	"github.com/qinqingxu/synchub-for-agents/internal/scheduler"
	"github.com/qinqingxu/synchub-for-agents/internal/syncengine"
)

func configuredHome(t *testing.T) string {
	t.Helper()
	userHome := t.TempDir()
	t.Setenv("HOME", userHome)
	t.Setenv("USERPROFILE", userHome)
	home := filepath.Join(userHome, ".synchub")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := config.Save(cli.ConfigPath(home), config.Config{
		RepoURL:             "git@github.com:owner/repo.git",
		SyncIntervalMinutes: 10,
		TrashGraceDays:      30,
		Agents:              map[string]bool{"claude": true},
	}); err != nil {
		t.Fatal(err)
	}
	return home
}

func writeDesktopPreview(t *testing.T, home string, preview ResourcePreview) {
	t.Helper()
	if err := newSummaryStore(home).savePreview(preview); err != nil {
		t.Fatal(err)
	}
}

func TestSnapshotUsesPersistedSummariesWithoutCollectingResources(t *testing.T) {
	home := configuredHome(t)
	generatedAt := time.Date(2026, 8, 18, 9, 0, 0, 0, time.UTC)
	finishedAt := generatedAt.Add(-time.Minute)
	writeDesktopPreview(t, home, ResourcePreview{
		GeneratedAt: generatedAt,
		Files:       12,
	})
	service, err := New(home, runtime.GOOS)
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	service.recordCycle(daemon.CycleResult{
		Blocked:    2,
		FinishedAt: finishedAt,
	})
	service.recordProgress(syncengine.Progress{
		CompletedActions: 3,
		TotalActions:     8,
	})
	collectionCalled := false
	service.previewCollector = func(
		context.Context,
		config.Config,
		[]provider.Provider,
	) (ResourcePreview, error) {
		collectionCalled = true
		return ResourcePreview{}, errors.New("resource collection invoked by Snapshot")
	}

	done := make(chan struct{})
	var snapshot Snapshot
	var snapshotErr error
	go func() {
		snapshot, snapshotErr = service.Snapshot()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Snapshot() waited for resource collection")
	}
	if snapshotErr != nil {
		t.Fatal(snapshotErr)
	}
	if collectionCalled {
		t.Fatal("Snapshot() invoked resource collection")
	}
	if snapshot.Preview.Files != 12 || !snapshot.Preview.GeneratedAt.Equal(generatedAt) {
		t.Fatalf("preview = %#v, want persisted summary", snapshot.Preview)
	}
	if !snapshot.LastSync.Equal(finishedAt) {
		t.Fatalf("last sync = %v, want %v", snapshot.LastSync, finishedAt)
	}
	if snapshot.PendingActions != 5 {
		t.Fatalf("pending actions = %d, want 5", snapshot.PendingActions)
	}
	if snapshot.BlockedFiles != 2 {
		t.Fatalf("blocked files = %d, want 2", snapshot.BlockedFiles)
	}
}

func TestSnapshotIncludesCurrentProgress(t *testing.T) {
	service, err := New(configuredHome(t), runtime.GOOS)
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()

	service.recordProgress(syncengine.Progress{
		Stage:            syncengine.StageApplying,
		Label:            "Applying changes",
		Percentage:       65,
		CompletedActions: 2,
		TotalActions:     4,
		BlockedFiles:     1,
	})

	got, err := service.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if got.Progress.Stage != "applying" ||
		got.Progress.Percentage != 65 ||
		got.Progress.TotalActions != 4 {
		t.Fatalf("progress = %#v", got.Progress)
	}

	service.recordProgress(syncengine.Progress{
		CompletedActions: 5,
		TotalActions:     4,
	})
	got, err = service.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if got.PendingActions != 0 {
		t.Fatalf("pending actions = %d, want clamped zero", got.PendingActions)
	}
}

func TestSnapshotMapsConfiguredStatus(t *testing.T) {
	service, err := New(configuredHome(t), runtime.GOOS)
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()

	got, err := service.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if !got.Configured {
		t.Fatal("configured home reported as unconfigured")
	}
	if got.RepositoryURL != "git@github.com:owner/repo.git" || got.IntervalMinutes != 10 {
		t.Fatalf("snapshot = %#v", got)
	}
	if len(got.Agents) == 0 {
		t.Fatal("snapshot should contain builtin agents")
	}
}

func TestSnapshotReportsDoneAfterSuccessfulCycle(t *testing.T) {
	service, err := New(configuredHome(t), runtime.GOOS)
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()

	d := service.Daemon()
	d.Scheduler.Job = func() error { return nil }
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go d.Scheduler.Run(ctx)
	d.Scheduler.Trigger()

	deadline := time.Now().Add(time.Second)
	for d.Scheduler.State() != scheduler.StateDone && time.Now().Before(deadline) {
		time.Sleep(2 * time.Millisecond)
	}
	got, err := service.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if got.State != "done" {
		t.Fatalf("snapshot state = %q, want done", got.State)
	}
}

func TestSnapshotPromotesDoneToErrorWhenLastCycleNeedsAttention(t *testing.T) {
	service, err := New(configuredHome(t), runtime.GOOS)
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()

	d := service.Daemon()
	d.Scheduler.Job = func() error { return nil }
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go d.Scheduler.Run(ctx)
	d.Scheduler.Trigger()

	deadline := time.Now().Add(time.Second)
	for d.Scheduler.State() != scheduler.StateDone && time.Now().Before(deadline) {
		time.Sleep(2 * time.Millisecond)
	}
	service.mu.Lock()
	service.last.NeedsAttention = true
	service.mu.Unlock()

	got, err := service.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if got.State != "error" {
		t.Fatalf("snapshot state = %q, want error", got.State)
	}
}

func TestNewAllowsMissingConfiguration(t *testing.T) {
	service, err := New(filepath.Join(t.TempDir(), ".synchub"), runtime.GOOS)
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()

	got, err := service.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if got.Configured {
		t.Fatal("missing config should require onboarding")
	}
	if got.IntervalMinutes != 10 || got.TrashGraceDays != 30 {
		t.Fatalf("defaults = %#v", got)
	}
	if err := service.Trigger(); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("Trigger error = %v, want ErrNotConfigured", err)
	}
}

func TestOnboardingAgentsLoadSettingsWithoutBuildingSnapshot(t *testing.T) {
	service, err := New(configuredHome(t), runtime.GOOS)
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()

	agents, err := service.OnboardingAgents()
	if err != nil {
		t.Fatal(err)
	}
	for _, agent := range agents {
		if agent.Name == "claude" {
			if !agent.Enabled {
				t.Fatal("configured Claude agent should be enabled")
			}
			if len(agent.Exclude) == 0 {
				t.Fatal("Claude onboarding settings should include declaration exclusions")
			}
			return
		}
	}
	t.Fatal("Claude onboarding settings are missing")
}

func TestSaveSettingsUpdatesConfigAndLiveInterval(t *testing.T) {
	home := configuredHome(t)
	service, err := New(home, runtime.GOOS)
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()

	err = service.SaveSettings(SettingsInput{
		RepositoryURL:   "git@github.com:owner/new.git",
		IntervalMinutes: 1440,
		TrashGraceDays:  365,
		Agents:          map[string]bool{"claude": false},
	})
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(cli.ConfigPath(home))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.RepoURL != "git@github.com:owner/new.git" ||
		cfg.SyncIntervalMinutes != 1440 ||
		cfg.TrashGraceDays != 365 {
		t.Fatalf("config = %#v", cfg)
	}
	if got := service.Daemon().Scheduler.IntervalDuration(); got != 1440*time.Minute {
		t.Fatalf("interval = %v, want 1440m", got)
	}
}

func TestSaveSettingsAcceptsMinimumTimingValues(t *testing.T) {
	home := configuredHome(t)
	service, err := New(home, runtime.GOOS)
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()

	err = service.SaveSettings(SettingsInput{
		RepositoryURL:   "git@github.com:owner/repo.git",
		IntervalMinutes: 1,
		TrashGraceDays:  1,
		Agents:          map[string]bool{"claude": true},
	})
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(cli.ConfigPath(home))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SyncIntervalMinutes != 1 || cfg.TrashGraceDays != 1 {
		t.Fatalf("config = %#v", cfg)
	}
	if got := service.Daemon().Scheduler.IntervalDuration(); got != time.Minute {
		t.Fatalf("interval = %v, want 1m", got)
	}
}

func TestSaveSettingsRejectsTimingOutsideSupportedRanges(t *testing.T) {
	tests := []struct {
		name     string
		interval int
		grace    int
		want     string
	}{
		{name: "zero interval", interval: 0, grace: 30, want: "sync interval must be between 1 and 1440 minutes"},
		{name: "interval above one day", interval: 1441, grace: 30, want: "sync interval must be between 1 and 1440 minutes"},
		{name: "zero retention", interval: 10, grace: 0, want: "archive retention must be between 1 and 365 days"},
		{name: "retention above one year", interval: 10, grace: 366, want: "archive retention must be between 1 and 365 days"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			home := configuredHome(t)
			service, err := New(home, runtime.GOOS)
			if err != nil {
				t.Fatal(err)
			}
			defer service.Close()

			err = service.SaveSettings(SettingsInput{
				RepositoryURL:   "git@github.com:owner/changed.git",
				IntervalMinutes: test.interval,
				TrashGraceDays:  test.grace,
				Agents:          map[string]bool{"claude": false},
			})
			if err == nil || err.Error() != test.want {
				t.Fatalf("error = %v, want %q", err, test.want)
			}

			cfg, err := config.Load(cli.ConfigPath(home))
			if err != nil {
				t.Fatal(err)
			}
			if cfg.RepoURL != "git@github.com:owner/repo.git" ||
				cfg.SyncIntervalMinutes != 10 ||
				cfg.TrashGraceDays != 30 {
				t.Fatalf("config changed after rejected settings: %#v", cfg)
			}
			if got := service.Daemon().Scheduler.IntervalDuration(); got != 10*time.Minute {
				t.Fatalf("interval = %v, want unchanged 10m", got)
			}
		})
	}
}

func TestSubscribeStateAttachesWhenFirstRunConfigurationStartsDaemon(t *testing.T) {
	service, err := New(filepath.Join(t.TempDir(), ".synchub"), runtime.GOOS)
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()

	states := make(chan scheduler.State, 1)
	unsubscribe := service.SubscribeState(func(state scheduler.State) {
		states <- state
	})
	defer unsubscribe()

	if err := service.SaveSettings(SettingsInput{
		RepositoryURL:   "git@github.com:owner/repo.git",
		IntervalMinutes: 10,
		TrashGraceDays:  30,
		Agents:          map[string]bool{},
	}); err != nil {
		t.Fatal(err)
	}
	service.Daemon().Scheduler.Pause()

	select {
	case got := <-states:
		if got != scheduler.StatePaused {
			t.Fatalf("state = %v, want paused", got)
		}
	case <-time.After(time.Second):
		t.Fatal("state observer was not attached to newly configured daemon")
	}
}
