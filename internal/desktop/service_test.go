package desktop

import (
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
	home := filepath.Join(t.TempDir(), ".acsync")
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
