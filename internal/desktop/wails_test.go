package desktop

import (
	"errors"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/qinqingxu/synchub-for-agents/internal/onboarding"
	"github.com/qinqingxu/synchub-for-agents/internal/startup"
	"github.com/wailsapp/wails/v3/pkg/application"
)

func TestWailsServiceDelegatesToDesktopCore(t *testing.T) {
	core, err := New(filepath.Join(t.TempDir(), ".synchub"), runtime.GOOS)
	if err != nil {
		t.Fatal(err)
	}
	service := NewWailsService(nil, core, nil, nil)

	snapshot, err := service.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Configured {
		t.Fatal("new home should require onboarding")
	}
	if err := service.TriggerSync(); !errors.Is(err, ErrNotConfigured) {
		t.Fatalf("TriggerSync error = %v", err)
	}
}

func TestWaitForDesktopRunTimesOutInsteadOfBlockingQuit(t *testing.T) {
	done := make(chan error)
	started := time.Now()
	err := waitForDesktopRun(done, 10*time.Millisecond)

	if err == nil || !strings.Contains(err.Error(), "timed out") {
		t.Fatalf("error = %v, want shutdown timeout", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("shutdown waited too long: %v", elapsed)
	}
}

func TestWailsServiceReturnsOnboardingToRepositoryStep(t *testing.T) {
	onboardingService := onboarding.New(onboarding.Dependencies{})
	if err := onboardingService.SetRepository("git@github.com:acme/wrong.git"); err != nil {
		t.Fatal(err)
	}
	service := NewWailsService(nil, nil, onboardingService, nil)

	state := service.ReturnToRepository()

	if state.Step != onboarding.Repository ||
		state.RepositoryURL != "git@github.com:acme/wrong.git" {
		t.Fatalf("state = %#v", state)
	}
}

type desktopStartupBackend struct {
	enabled bool
}

func (backend *desktopStartupBackend) EnableWithOptions(application.AutostartOptions) error {
	backend.enabled = true
	return nil
}

func (backend *desktopStartupBackend) Disable() error {
	backend.enabled = false
	return nil
}

func (backend *desktopStartupBackend) IsEnabled() (bool, error) {
	return backend.enabled, nil
}

func TestWailsStartupDefaultAndExplicitOptOut(t *testing.T) {
	backend := &desktopStartupBackend{}
	manager := &startup.Manager{
		Backend: backend, Identifier: "io.github.qinqingxu.synchub",
		Arguments: []string{"--hidden"}, GOOS: "windows",
		PreferencesPath: filepath.Join(t.TempDir(), "startup-settings.json"),
	}
	service := NewWailsService(nil, nil, nil, manager)
	if enabled, err := service.StartAtLogin(); err != nil || !enabled {
		t.Fatalf("initial Windows startup = %v, error = %v", enabled, err)
	}
	if err := service.SetStartAtLogin(false); err != nil {
		t.Fatal(err)
	}
	if enabled, err := service.StartAtLogin(); err != nil || enabled {
		t.Fatalf("Windows startup opt-out = %v, error = %v", enabled, err)
	}
}

func TestWailsStartupReportsMissingManager(t *testing.T) {
	service := NewWailsService(nil, nil, nil, nil)
	if _, err := service.StartAtLogin(); err == nil {
		t.Fatal("missing startup manager must be reported")
	}
	if err := service.SetStartAtLogin(true); err == nil {
		t.Fatal("missing startup manager must not appear to enable startup")
	}
}
