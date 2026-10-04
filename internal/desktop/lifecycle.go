package desktop

import (
	"context"

	"github.com/qinqingxu/synchub-for-agents/internal/daemon"
	"github.com/qinqingxu/synchub-for-agents/internal/scheduler"
)

// Run waits until configuration is ready, then runs the daemon until context
// cancellation. State changes publish fresh desktop snapshots.
func (s *Service) Run(ctx context.Context, publish func(Snapshot)) error {
	for {
		var d *daemon.Daemon
		select {
		case configured := <-s.start:
			d = configured
		case <-ctx.Done():
			return nil
		}
		s.operations.Lock()
		if s.Daemon() != d {
			s.operations.Unlock()
			continue
		}
		runCtx, cancel := context.WithCancel(ctx)
		done := make(chan struct{})
		s.runMu.Lock()
		s.runCancel, s.runDone = cancel, done
		s.runMu.Unlock()
		s.operations.Unlock()
		unsubscribe := d.Scheduler.Subscribe(func(scheduler.State) {
			snapshot, err := s.Snapshot()
			if err == nil {
				publish(snapshot)
			}
		})
		err := d.Run(runCtx)
		unsubscribe()
		cancel()
		s.runMu.Lock()
		s.runCancel, s.runDone = nil, nil
		close(done)
		s.runMu.Unlock()
		if err != nil {
			return err
		}
		if ctx.Err() != nil {
			return nil
		}
	}
}
