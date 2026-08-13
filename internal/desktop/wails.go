package desktop

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/qinqingxu/acsync/internal/onboarding"
	"github.com/qinqingxu/acsync/internal/startup"
	"github.com/wailsapp/wails/v3/pkg/application"
)

const (
	SnapshotEvent   = "desktop:snapshot"
	OnboardingEvent = "onboarding:state"
)

// WailsService exposes the UI-safe desktop API to generated Wails bindings.
type WailsService struct {
	app        *application.App
	core       *Service
	onboarding *onboarding.Service
	startup    *startup.Manager
	done       chan error
}

func NewWailsService(
	app *application.App,
	core *Service,
	onboardingService *onboarding.Service,
	startupManager *startup.Manager,
) *WailsService {
	return &WailsService{
		app:        app,
		core:       core,
		onboarding: onboardingService,
		startup:    startupManager,
	}
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

func (s *WailsService) SetStartAtLogin(enabled bool) error {
	if enabled {
		return s.startup.Enable()
	}
	return s.startup.Disable()
}

func (s *WailsService) StartAtLogin() (bool, error) {
	return s.startup.IsEnabled()
}

func (s *WailsService) NeedsOnboarding() bool {
	daemon := s.core.Daemon()
	if daemon == nil {
		return true
	}
	info, err := os.Stat(filepath.Join(daemon.Home, "repo", ".git"))
	return err != nil || !info.IsDir()
}

func (s *WailsService) OnboardingState() onboarding.State {
	return s.onboarding.State()
}

func (s *WailsService) SetRepository(raw string) error {
	if err := s.onboarding.SetRepository(raw); err != nil {
		return err
	}
	s.emitOnboarding()
	return nil
}

func (s *WailsService) StartGitHubLogin(ctx context.Context) (onboarding.State, error) {
	state, err := s.onboarding.StartGitHubLogin(ctx)
	if err == nil {
		s.emitOnboarding()
	}
	return state, err
}

func (s *WailsService) WaitGitHubLogin(ctx context.Context) (onboarding.State, error) {
	state, err := s.onboarding.WaitGitHubLogin(ctx)
	s.emitOnboarding()
	return state, err
}

func (s *WailsService) VerifySSH(ctx context.Context) (onboarding.State, error) {
	state, err := s.onboarding.VerifySSH(ctx)
	s.emitOnboarding()
	return state, err
}

func (s *WailsService) CompleteOnboarding(ctx context.Context, enabled map[string]bool) error {
	if err := s.onboarding.Complete(ctx, enabled); err != nil {
		return err
	}
	s.emitOnboarding()
	return nil
}

func (s *WailsService) CancelOnboarding() {
	s.onboarding.Cancel()
	s.emitOnboarding()
}

func (s *WailsService) emitOnboarding() {
	if s.app != nil {
		s.app.Event.Emit(OnboardingEvent, s.onboarding.State())
	}
}
