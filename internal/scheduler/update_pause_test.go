package scheduler

import (
	"testing"
	"time"
)

func TestPauseIfIdleNeverInterruptsAnActivePausedCycle(t *testing.T) {
	started := make(chan struct{})
	finish := make(chan struct{})
	done := make(chan struct{})
	s := New(time.Minute, func() error {
		close(started)
		<-finish
		return nil
	})
	go func() {
		s.runCycle()
		close(done)
	}()
	<-started
	s.Pause()
	if s.PauseIfIdle() {
		t.Error("visible Paused state hid an active job")
	}
	close(finish)
	<-done
	if !s.PauseIfIdle() {
		t.Fatal("idle scheduler could not be paused")
	}
	s.Job = func() error { t.Error("cycle started after update pause"); return nil }
	s.runCycle()
	if s.State() != StatePaused {
		t.Fatal("update pause was not preserved")
	}
}
