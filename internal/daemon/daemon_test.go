package daemon

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/qinqingxu/synchub-for-agents/internal/cli"
	"github.com/qinqingxu/synchub-for-agents/internal/config"
	"github.com/qinqingxu/synchub-for-agents/internal/resource"
	"github.com/qinqingxu/synchub-for-agents/internal/syncengine"
)

func writeConfig(t *testing.T, home string, cfg config.Config) {
	t.Helper()
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}

	if err := config.Save(cli.ConfigPath(home), cfg); err != nil {
		t.Fatal(err)
	}
}

func TestDaemonPublishesCompleteOnlyAfterCleanup(t *testing.T) {
	home := filepath.Join(t.TempDir(), ".synchub")
	writeConfig(t, home, config.Config{SyncIntervalMinutes: 60, Agents: map[string]bool{}})
	d, err := New(home, runtime.GOOS)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	d.sync = func(_ string, _ string, publish func(syncengine.Progress)) (syncengine.Result, error) {
		publish(syncengine.Progress{Stage: syncengine.StageUploading, Percentage: 85})
		return syncengine.Result{
			Actions: []syncengine.Action{{Type: syncengine.PushToRemote, RepoRel: "agents/demo/config/settings.json"}},
			Pushed:  true,
		}, nil
	}

	t.Run("cleanup failure", func(t *testing.T) {
		d.cleanup = func(string, time.Time) ([]string, error) {
			return nil, errors.New("cleanup failed")
		}
		var updates []syncengine.Progress
		d.OnProgress = func(update syncengine.Progress) {
			updates = append(updates, update)
		}
		if err := d.syncJob(); err == nil {
			t.Fatal("expected cleanup error")
		}
		for _, update := range updates {
			if update.Stage == syncengine.StageComplete {
				t.Fatalf("complete published before failed cleanup: %#v", updates)
			}
		}
	})

	t.Run("cleanup success", func(t *testing.T) {
		d.cleanup = func(string, time.Time) ([]string, error) {
			return nil, nil
		}
		var updates []syncengine.Progress
		d.OnProgress = func(update syncengine.Progress) {
			updates = append(updates, update)
		}
		if err := d.syncJob(); err != nil {
			t.Fatal(err)
		}
		last := updates[len(updates)-1]
		if last.Stage != syncengine.StageComplete ||
			last.Percentage != 100 ||
			last.TotalActions != 1 ||
			!last.Pushed {
			t.Fatalf("completion = %#v", last)
		}
	})
}

func TestNewUsesConfiguredInterval(t *testing.T) {
	home := filepath.Join(t.TempDir(), ".synchub")
	writeConfig(t, home, config.Config{SyncIntervalMinutes: 3, Agents: map[string]bool{}})

	d, err := New(home, runtime.GOOS)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if d.Scheduler.IntervalDuration() != 3*time.Minute {
		t.Errorf("interval = %v, want 3m", d.Scheduler.IntervalDuration())
	}
}

func TestNewDefaultsIntervalTo10m(t *testing.T) {
	home := filepath.Join(t.TempDir(), ".synchub")
	writeConfig(t, home, config.Config{SyncIntervalMinutes: 0, Agents: map[string]bool{}})

	d, err := New(home, runtime.GOOS)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	if d.Scheduler.IntervalDuration() != 10*time.Minute {
		t.Errorf("interval = %v, want 10m", d.Scheduler.IntervalDuration())
	}
}

func TestRunStopsOnContextCancelAndLogs(t *testing.T) {
	home := filepath.Join(t.TempDir(), ".synchub")
	// Long interval so only the startup trigger fires; the sync job may error
	// (no repo) — the lifecycle must still complete cleanly.
	writeConfig(t, home, config.Config{SyncIntervalMinutes: 60, Agents: map[string]bool{}})

	d, err := New(home, runtime.GOOS)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- d.Run(ctx) }()

	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not stop after context cancel")
	}

	data, err := os.ReadFile(filepath.Join(cli.LogsDir(home), "daemon.log"))
	if err != nil {
		t.Fatalf("daemon log missing: %v", err)
	}
	if !strings.Contains(string(data), "daemon started") {
		t.Errorf("log missing startup line: %s", data)
	}
}

func TestDaemonPublishesCycleErrors(t *testing.T) {
	home := filepath.Join(t.TempDir(), ".synchub")
	writeConfig(t, home, config.Config{
		SyncIntervalMinutes: 60,
		Agents:              map[string]bool{},
	})
	d, err := New(home, runtime.GOOS)
	if err != nil {
		t.Fatal(err)
	}

	defer d.Close()

	events := make(chan CycleResult, 1)
	d.OnCycle = func(result CycleResult) {
		events <- result
	}

	if err := d.syncJob(); err == nil {
		t.Fatal("expected missing repository error")
	}
	select {
	case result := <-events:
		if result.Error == "" {
			t.Fatal("cycle result should contain the sync error")
		}
		if result.FinishedAt.IsZero() {
			t.Fatal("cycle result should contain a completion time")
		}
	case <-time.After(time.Second):
		t.Fatal("cycle result was not published")
	}
}

func TestDaemonCompletesCleanupWhenSyncNeedsAttention(t *testing.T) {
	home := filepath.Join(t.TempDir(), ".synchub")
	writeConfig(t, home, config.Config{SyncIntervalMinutes: 60, Agents: map[string]bool{}})
	d, err := New(home, runtime.GOOS)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	d.sync = func(_ string, _ string, _ func(syncengine.Progress)) (syncengine.Result, error) {
		return syncengine.Result{
			Restored: 1,
			Blocked:  []string{"agents/demo/config/secret.json"},
			Issues: []resource.Issue{{
				ResourceKey: "demo/config",
				Path:        "secret.json",
				Code:        "secret-detected",
				Message:     "blocked",
			}},
			NeedsAttention: true,
		}, nil
	}
	cleanupRan := false
	d.cleanup = func(string, time.Time) ([]string, error) {
		cleanupRan = true
		return nil, nil
	}
	var cycle CycleResult
	d.OnCycle = func(result CycleResult) {
		cycle = result
	}
	var progress []syncengine.Progress
	d.OnProgress = func(update syncengine.Progress) {
		progress = append(progress, update)
	}

	if err := d.syncJob(); err != nil {
		t.Fatal(err)
	}
	if !cleanupRan {
		t.Fatal("cleanup did not run")
	}
	if !cycle.NeedsAttention || cycle.Restored != 1 || cycle.Blocked != 1 {
		t.Fatalf("cycle = %#v", cycle)
	}
	last := progress[len(progress)-1]
	if last.Stage != syncengine.StageComplete ||
		last.Label != "Synchronization needs attention" ||
		!last.NeedsAttention {
		t.Fatalf("completion = %#v", last)
	}
}
