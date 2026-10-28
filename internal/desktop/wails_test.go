package desktop

import (
	"errors"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/qinqingxu/synchub-for-agents/internal/onboarding"
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
