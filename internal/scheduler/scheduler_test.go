package scheduler

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type recorder struct {
	mu     sync.Mutex
	states []State
}

func (r *recorder) add(s State) {
	r.mu.Lock()
	r.states = append(r.states, s)
	r.mu.Unlock()
}

func (r *recorder) snapshot() []State {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]State, len(r.states))
	copy(out, r.states)
	return out
}

func waitFor(t *testing.T, cond func() bool, timeout time.Duration, msg string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for %s", msg)
}

func TestRunCycleReportsUpdatingThenIdle(t *testing.T) {
	var runs int64
	rec := &recorder{}
	s := New(time.Hour, func() error {
		atomic.AddInt64(&runs, 1)
		return nil
	})
	s.OnState = rec.add

	s.runCycle()

	if got := atomic.LoadInt64(&runs); got != 1 {
		t.Fatalf("runs = %d, want 1", got)
	}
	states := rec.snapshot()
	if len(states) != 2 || states[0] != StateUpdating || states[1] != StateIdle {
		t.Fatalf("states = %v, want [updating idle]", states)
	}
}

func TestRunCycleReportsErrorOnJobFailure(t *testing.T) {
	rec := &recorder{}
	s := New(time.Hour, func() error { return errors.New("boom") })
	s.OnState = rec.add

	s.runCycle()

	states := rec.snapshot()
	if len(states) != 2 || states[0] != StateUpdating || states[1] != StateError {
		t.Fatalf("states = %v, want [updating error]", states)
	}
}

func TestPausedRunCycleSkips(t *testing.T) {
	var runs int64
	s := New(time.Hour, func() error {
		atomic.AddInt64(&runs, 1)
		return nil
	})
	s.Pause()
	s.runCycle()

	if got := atomic.LoadInt64(&runs); got != 0 {
		t.Fatalf("runs = %d, want 0 while paused", got)
	}
	if s.State() != StatePaused {
		t.Fatalf("state = %v, want paused", s.State())
	}
}

func TestTriggerRunsViaLoop(t *testing.T) {
	var runs int64
	s := New(time.Hour, func() error {
		atomic.AddInt64(&runs, 1)
		return nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.Run(ctx)

	s.Trigger()
	waitFor(t, func() bool { return atomic.LoadInt64(&runs) == 1 }, time.Second, "first run")
}

func TestPauseBlocksTriggerThenResume(t *testing.T) {
	var runs int64
	s := New(time.Hour, func() error {
		atomic.AddInt64(&runs, 1)
		return nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.Run(ctx)

	s.Pause()
	s.Trigger()
	time.Sleep(50 * time.Millisecond)
	if got := atomic.LoadInt64(&runs); got != 0 {
		t.Fatalf("runs = %d, want 0 while paused", got)
	}

	s.Resume()
	s.Trigger()
	waitFor(t, func() bool { return atomic.LoadInt64(&runs) == 1 }, time.Second, "run after resume")
}
