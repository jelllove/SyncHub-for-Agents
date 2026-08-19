package desktop

import (
	"errors"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
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
