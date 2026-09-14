package desktop

import (
	"context"
	"errors"

	"github.com/qinqingxu/synchub-for-agents/internal/scheduler"
	"github.com/qinqingxu/synchub-for-agents/internal/updater"
)

const UpdateEvent = "updater:status"

func WithUpdates(service *WailsService, manager *updater.Manager, quit func()) *WailsService {
	service.updates = manager
	service.quitForUpdate = quit
	return service
}

func (s *WailsService) UpdateStatus() (updater.Status, error) {
	if s.updates == nil {
		return updater.Status{}, errors.New("software updater is not initialized")
	}
	return s.updates.Status(), nil
}

func (s *WailsService) CheckForUpdates(ctx context.Context) (updater.Status, error) {
	if s.updates == nil {
		return updater.Status{}, errors.New("software updater is not initialized")
	}
	return s.updates.Check(ctx)
}

func (s *WailsService) SetAutomaticUpdates(enabled bool) (updater.Status, error) {
	if s.updates == nil {
		return updater.Status{}, errors.New("software updater is not initialized")
	}
	return s.updates.SetAutomatic(enabled)
}

func (s *WailsService) RestartToUpdate() error {
	if s.updates == nil || s.quitForUpdate == nil {
		return errors.New("software updater is not initialized")
	}
	var resume func()
	if daemon := s.core.Daemon(); daemon != nil {
		wasPaused := daemon.Scheduler.State() == scheduler.StatePaused
		if !daemon.Scheduler.PauseIfIdle() {
			return errors.New("wait for the current synchronization to finish before restarting")
		}
		if !wasPaused {
			resume = daemon.Scheduler.Resume
		}
	}
	if err := s.updates.RequestRestart(); err != nil {
		if resume != nil {
			resume()
		}
		return err
	}
	s.quitForUpdate()
	return nil
}
