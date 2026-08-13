package desktop

import (
	"context"

	"github.com/qinqingxu/acsync/internal/daemon"
	"github.com/qinqingxu/acsync/internal/scheduler"
)

// Run waits until configuration is ready, then runs the daemon until context
// cancellation. State changes publish fresh desktop snapshots.
func (s *Service) Run(ctx context.Context, publish func(Snapshot)) error {
	var d *daemon.Daemon
	select {
	case configured := <-s.start:
		d = configured
	case <-ctx.Done():
		return nil
	}

	unsubscribe := d.Scheduler.Subscribe(func(scheduler.State) {
		snapshot, err := s.Snapshot()
		if err == nil {
			publish(snapshot)
		}
	})
	defer unsubscribe()
	return d.Run(ctx)
}
