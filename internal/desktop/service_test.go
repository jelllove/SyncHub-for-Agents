package desktop

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/qinqingxu/acsync/internal/cli"
	"github.com/qinqingxu/acsync/internal/config"
	"github.com/qinqingxu/acsync/internal/scheduler"
	"github.com/qinqingxu/acsync/internal/syncengine"
)

func configuredHome(t *testing.T) string {
	t.Helper()
	userHome := t.TempDir()
	t.Setenv("HOME", userHome)
	t.Setenv("USERPROFILE", userHome)
	home := filepath.Join(userHome, ".acsync")
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
	service, err := New(filepath.Join(t.TempDir(), ".acsync"), runtime.GOOS)
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
		IntervalMinutes: 3,
		TrashGraceDays:  45,
		Agents:          map[string]bool{"claude": false},
	})
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(cli.ConfigPath(home))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.RepoURL != "git@github.com:owner/new.git" || cfg.SyncIntervalMinutes != 3 {
		t.Fatalf("config = %#v", cfg)
	}
	if got := service.Daemon().Scheduler.IntervalDuration(); got != 3*time.Minute {
		t.Fatalf("interval = %v, want 3m", got)
	}
}

func TestSubscribeStateAttachesWhenFirstRunConfigurationStartsDaemon(t *testing.T) {
	service, err := New(filepath.Join(t.TempDir(), ".acsync"), runtime.GOOS)
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
