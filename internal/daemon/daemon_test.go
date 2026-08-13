package daemon

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/qinqingxu/acsync/internal/cli"
	"github.com/qinqingxu/acsync/internal/config"
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

func TestNewUsesConfiguredInterval(t *testing.T) {
	home := filepath.Join(t.TempDir(), ".acsync")
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
	home := filepath.Join(t.TempDir(), ".acsync")
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
	home := filepath.Join(t.TempDir(), ".acsync")
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
	home := filepath.Join(t.TempDir(), ".acsync")
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
