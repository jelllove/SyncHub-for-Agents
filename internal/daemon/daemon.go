// Package daemon runs the synchub sync scheduler as a long-lived service.
package daemon

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/qinqingxu/synchub-for-agents/internal/cli"
	"github.com/qinqingxu/synchub-for-agents/internal/config"
	"github.com/qinqingxu/synchub-for-agents/internal/scheduler"
	"github.com/qinqingxu/synchub-for-agents/internal/syncengine"
)

// CycleResult describes one completed sync and cleanup cycle.
type CycleResult struct {
	Actions         int
	Blocked         int
	Pushed          bool
	Purged          int
	Restored        int
	Reinstalled     int
	Skipped         int
	Conflicts       int
	PendingInstalls int
	NeedsAttention  bool
	Error           string
	FinishedAt      time.Time
}

// Daemon runs the sync scheduler for a given synchub home.
type Daemon struct {
	Home             string
	GOOS             string
	Scheduler        *scheduler.Scheduler
	Logger           *log.Logger
	AutoTriggerOnRun bool
	OnCycle          func(CycleResult)
	OnProgress       func(syncengine.Progress)

	closeLog func() error
	sync     func(string, string, func(syncengine.Progress)) (syncengine.Result, error)
	cleanup  func(string, time.Time) ([]string, error)
}

// New builds a Daemon: it loads config for the interval and wires a scheduler
// whose job runs one sync pass followed by trash cleanup.
func New(home, goos string) (*Daemon, error) {
	cfg, err := config.Load(cli.ConfigPath(home))
	if err != nil {
		return nil, err
	}
	interval := time.Duration(cfg.SyncIntervalMinutes) * time.Minute
	if interval <= 0 {
		interval = 10 * time.Minute
	}

	logger, closeLog, err := newLogger(home)
	if err != nil {
		return nil, err
	}

	d := &Daemon{
		Home:             home,
		GOOS:             goos,
		Logger:           logger,
		AutoTriggerOnRun: true,
		closeLog:         closeLog,
		sync:             cli.RunSyncWithProgress,
		cleanup:          cli.RunCleanup,
	}
	d.Scheduler = scheduler.New(interval, d.syncJob)
	d.Scheduler.Subscribe(d.logState)
	return d, nil
}

// syncJob runs one full sync followed by a trash cleanup pass.
func (d *Daemon) syncJob() error {
	result := CycleResult{}
	defer func() {
		result.FinishedAt = time.Now()
		if d.OnCycle != nil {
			d.OnCycle(result)
		}
	}()

	res, err := d.sync(d.Home, d.GOOS, d.OnProgress)
	if err != nil {
		result.Error = err.Error()
		d.Logger.Printf("sync error: %v", err)
		return err
	}
	result.Actions = len(res.Actions)
	result.Blocked = len(res.Blocked)
	result.Pushed = res.Pushed
	result.Restored = res.Restored
	result.Reinstalled = res.Reinstalled
	result.Skipped = res.Skipped
	result.Conflicts = res.Conflicts
	result.PendingInstalls = res.PendingInstalls
	result.NeedsAttention = res.NeedsAttention
	d.Logger.Printf(
		"sync ok: %d actions, %d blocked, %d skipped, %d conflicts, pushed=%v",
		result.Actions,
		result.Blocked,
		result.Skipped,
		result.Conflicts,
		result.Pushed,
	)

	purged, err := d.cleanup(d.Home, time.Now())
	if err != nil {
		result.Error = err.Error()
		d.Logger.Printf("cleanup error: %v", err)
		return err
	}
	result.Purged = len(purged)
	if result.Purged > 0 {
		d.Logger.Printf("purged %d expired trash entries", result.Purged)
	}
	if d.OnProgress != nil {
		label := "Synchronization complete"
		if result.NeedsAttention {
			label = "Synchronization needs attention"
		}
		d.OnProgress(syncengine.Progress{
			Stage:            syncengine.StageComplete,
			Label:            label,
			Percentage:       100,
			CompletedActions: result.Actions,
			TotalActions:     result.Actions,
			BlockedFiles:     result.Blocked,
			Pushed:           result.Pushed,
			Restored:         result.Restored,
			Reinstalled:      result.Reinstalled,
			Skipped:          result.Skipped,
			Conflicts:        result.Conflicts,
			PendingInstalls:  result.PendingInstalls,
			NeedsAttention:   result.NeedsAttention,
		})
	}
	return nil
}

func (d *Daemon) logState(s scheduler.State) {
	d.Logger.Printf("state: %s", s)
}

// Run triggers an initial sync then runs the scheduler until ctx is cancelled.
func (d *Daemon) Run(ctx context.Context) error {
	d.Logger.Printf("daemon started (home=%s, interval=%s)", d.Home, d.Scheduler.IntervalDuration())
	if d.AutoTriggerOnRun {
		d.Scheduler.Trigger() // sync promptly on startup
	} else {
		d.Logger.Printf("initial sync is waiting for first-sync strategy selection")
	}
	d.Scheduler.Run(ctx)
	d.Logger.Printf("daemon stopped")
	return d.Close()
}

// Close releases the daemon's log file. It is safe to call multiple times and
// is invoked automatically by Run; construct-only callers should call it to
// avoid leaking the log handle.
func (d *Daemon) Close() error {
	if d.closeLog != nil {
		err := d.closeLog()
		d.closeLog = nil
		return err
	}
	return nil
}

func newLogger(home string) (*log.Logger, func() error, error) {
	dir := cli.LogsDir(home)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, nil, err
	}
	f, err := os.OpenFile(filepath.Join(dir, "daemon.log"),
		os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, nil, err
	}
	return log.New(f, "", log.LstdFlags), f.Close, nil
}
