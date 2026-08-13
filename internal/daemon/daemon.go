// Package daemon runs the acsync sync scheduler as a long-lived service.
package daemon

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/qinqingxu/acsync/internal/cli"
	"github.com/qinqingxu/acsync/internal/config"
	"github.com/qinqingxu/acsync/internal/scheduler"
)

// CycleResult describes one completed sync and cleanup cycle.
type CycleResult struct {
	Actions    int
	Blocked    int
	Pushed     bool
	Purged     int
	Error      string
	FinishedAt time.Time
}

// Daemon runs the sync scheduler for a given acsync home.
type Daemon struct {
	Home      string
	GOOS      string
	Scheduler *scheduler.Scheduler
	Logger    *log.Logger
	OnCycle   func(CycleResult)

	closeLog func() error
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

	d := &Daemon{Home: home, GOOS: goos, Logger: logger, closeLog: closeLog}
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

	res, err := cli.RunSync(d.Home, d.GOOS)
	if err != nil {
		result.Error = err.Error()
		d.Logger.Printf("sync error: %v", err)
		return err
	}
	result.Actions = len(res.Actions)
	result.Blocked = len(res.Blocked)
	result.Pushed = res.Pushed
	d.Logger.Printf("sync ok: %d actions, %d blocked, pushed=%v",
		result.Actions, result.Blocked, result.Pushed)

	purged, err := cli.RunCleanup(d.Home, time.Now())
	if err != nil {
		result.Error = err.Error()
		d.Logger.Printf("cleanup error: %v", err)
		return err
	}
	result.Purged = len(purged)
	if result.Purged > 0 {
		d.Logger.Printf("purged %d expired trash entries", result.Purged)
	}
	return nil
}

func (d *Daemon) logState(s scheduler.State) {
	d.Logger.Printf("state: %s", s)
}

// Run triggers an initial sync then runs the scheduler until ctx is cancelled.
func (d *Daemon) Run(ctx context.Context) error {
	d.Logger.Printf("daemon started (home=%s, interval=%s)", d.Home, d.Scheduler.IntervalDuration())
	d.Scheduler.Trigger() // sync promptly on startup
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
