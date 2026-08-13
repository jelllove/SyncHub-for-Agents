package desktop

import (
	"context"
	"fmt"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
)

const SnapshotEvent = "desktop:snapshot"

// WailsService exposes the UI-safe desktop API to generated Wails bindings.
type WailsService struct {
	app  *application.App
	core *Service
	done chan error
}

func NewWailsService(app *application.App, core *Service) *WailsService {
	return &WailsService{app: app, core: core}
}

func (s *WailsService) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	s.done = make(chan error, 1)
	go func() {
		s.done <- s.core.Run(ctx, func(snapshot Snapshot) {
			if s.app != nil {
				s.app.Event.Emit(SnapshotEvent, snapshot)
			}
		})
	}()
	return nil
}

func (s *WailsService) ServiceShutdown() error {
	if s.done != nil {
		if err := waitForDesktopRun(s.done, 5*time.Second); err != nil {
			return err
		}
	}
	return s.core.Close()
}

func waitForDesktopRun(done <-chan error, timeout time.Duration) error {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case err := <-done:
		return err
	case <-timer.C:
		return fmt.Errorf("desktop shutdown timed out after %s", timeout)
	}
}

func (s *WailsService) Snapshot() (Snapshot, error) {
	return s.core.Snapshot()
}

func (s *WailsService) TriggerSync() error {
	return s.core.Trigger()
}

func (s *WailsService) Pause() error {
	return s.core.Pause()
}

func (s *WailsService) Resume() error {
	return s.core.Resume()
}

func (s *WailsService) SaveSettings(input SettingsInput) error {
	return s.core.SaveSettings(input)
}
