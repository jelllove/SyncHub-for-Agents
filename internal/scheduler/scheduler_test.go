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

type fakeSchedulerTimer struct {
	ch      chan time.Time
	resets  chan time.Duration
	stopped chan struct{}
	once    sync.Once
}

type fakeSchedulerClock struct {
	mu      sync.Mutex
	current time.Time
}

func newFakeSchedulerTimer() *fakeSchedulerTimer {
	return &fakeSchedulerTimer{
		ch:      make(chan time.Time, 1),
		resets:  make(chan time.Duration, 8),
		stopped: make(chan struct{}),
	}
}

func newFakeSchedulerClock(current time.Time) *fakeSchedulerClock {
	return &fakeSchedulerClock{current: current}
}

func (c *fakeSchedulerClock) now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.current
}

func (c *fakeSchedulerClock) advance(interval time.Duration) {
	c.mu.Lock()
	c.current = c.current.Add(interval)
	c.mu.Unlock()
}

func (t *fakeSchedulerTimer) C() <-chan time.Time {
	return t.ch
}

func (t *fakeSchedulerTimer) Reset(interval time.Duration) {
	t.resets <- interval
}

func (t *fakeSchedulerTimer) Stop() {
	t.once.Do(func() {
		close(t.stopped)
	})
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

func TestRunCycleReportsUpdatingThenDone(t *testing.T) {
	var runs int64
	rec := &recorder{}
	s := New(time.Hour, func() error {
		atomic.AddInt64(&runs, 1)
		return nil
	})
	s.Subscribe(rec.add)

	s.runCycle()

	if got := atomic.LoadInt64(&runs); got != 1 {
		t.Fatalf("runs = %d, want 1", got)
	}
	states := rec.snapshot()
	if len(states) != 2 || states[0] != StateUpdating || states[1] != StateDone {
		t.Fatalf("states = %v, want [updating done]", states)
	}
	if StateDone.String() != "done" {
		t.Fatalf("StateDone.String() = %q, want done", StateDone.String())
	}
}

func TestRunCycleReportsErrorOnJobFailure(t *testing.T) {
	rec := &recorder{}
	s := New(time.Hour, func() error { return errors.New("boom") })
	s.Subscribe(rec.add)

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

func TestSetIntervalReplacesTickerWithoutRestart(t *testing.T) {
	var runs atomic.Int32
	s := New(time.Hour, func() error {
		runs.Add(1)
		return nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.Run(ctx)

	s.SetInterval(10 * time.Millisecond)
	waitFor(t, func() bool { return runs.Load() > 0 }, time.Second, "run at updated interval")

	if got := s.IntervalDuration(); got != 10*time.Millisecond {
		t.Fatalf("interval = %v, want 10ms", got)
	}
}

func TestSubscribeReceivesStateAndUnsubscribeStopsIt(t *testing.T) {
	s := New(time.Hour, func() error { return nil })
	states := make(chan State, 4)
	unsubscribe := s.Subscribe(func(state State) {
		states <- state
	})

	s.runCycle()
	if got := <-states; got != StateUpdating {
		t.Fatalf("first state = %v, want updating", got)
	}
	if got := <-states; got != StateDone {
		t.Fatalf("second state = %v, want done", got)
	}

	unsubscribe()
	s.Pause()
	select {
	case got := <-states:
		t.Fatalf("received state after unsubscribe: %v", got)
	case <-time.After(20 * time.Millisecond):
	}
}

func TestNextRunIsArmedWhenRunStarts(t *testing.T) {
	clock := newFakeSchedulerClock(time.Date(2026, 8, 18, 8, 0, 0, 0, time.UTC))
	timer := newFakeSchedulerTimer()
	created := make(chan time.Duration, 1)
	s := New(10*time.Minute, func() error { return nil })
	s.now = clock.now
	s.newTimer = func(interval time.Duration) schedulerTimer {
		created <- interval
		return timer
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.Run(ctx)

	if got := <-created; got != 10*time.Minute {
		t.Fatalf("initial timer = %v, want 10m", got)
	}
	waitFor(t, func() bool {
		return s.NextRun().Equal(clock.now().Add(10 * time.Minute))
	}, time.Second, "initial next run")
}

func TestSetIntervalRearmsNextRunFromChangeTime(t *testing.T) {
	clock := newFakeSchedulerClock(time.Date(2026, 8, 18, 8, 0, 0, 0, time.UTC))
	timer := newFakeSchedulerTimer()
	s := New(10*time.Minute, func() error { return nil })
	s.now = clock.now
	s.newTimer = func(time.Duration) schedulerTimer { return timer }
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.Run(ctx)
	waitFor(t, func() bool { return !s.NextRun().IsZero() }, time.Second, "scheduler startup")

	clock.advance(3 * time.Minute)
	s.SetInterval(25 * time.Minute)

	select {
	case got := <-timer.resets:
		if got != 25*time.Minute {
			t.Fatalf("reset interval = %v, want 25m", got)
		}
	case <-time.After(time.Second):
		t.Fatal("interval change did not reset timer")
	}
	if got := s.NextRun(); !got.Equal(clock.now().Add(25 * time.Minute)) {
		t.Fatalf("NextRun() = %v", got)
	}
}

func TestManualCycleRearmsAfterJobFinishes(t *testing.T) {
	clock := newFakeSchedulerClock(time.Date(2026, 8, 18, 8, 0, 0, 0, time.UTC))
	timer := newFakeSchedulerTimer()
	started := make(chan struct{})
	release := make(chan struct{})
	terminalNextRun := make(chan time.Time, 1)
	s := New(10*time.Minute, func() error {
		close(started)
		<-release
		return nil
	})
	s.now = clock.now
	s.newTimer = func(time.Duration) schedulerTimer { return timer }
	s.Subscribe(func(state State) {
		if state == StateDone {
			terminalNextRun <- s.NextRun()
		}
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.Run(ctx)
	waitFor(t, func() bool { return !s.NextRun().IsZero() }, time.Second, "scheduler startup")

	s.Trigger()
	<-started
	clock.advance(4 * time.Minute)
	close(release)

	select {
	case got := <-timer.resets:
		if got != 10*time.Minute {
			t.Fatalf("reset interval = %v, want 10m", got)
		}
	case <-time.After(time.Second):
		t.Fatal("manual cycle did not reset timer")
	}
	if got := s.NextRun(); !got.Equal(clock.now().Add(10 * time.Minute)) {
		t.Fatalf("NextRun() = %v", got)
	}
	if got := <-terminalNextRun; !got.Equal(clock.now().Add(10 * time.Minute)) {
		t.Fatalf("terminal observer NextRun() = %v", got)
	}
}

func TestPeriodicCycleRearmsAfterJobFinishes(t *testing.T) {
	clock := newFakeSchedulerClock(time.Date(2026, 8, 18, 8, 0, 0, 0, time.UTC))
	timer := newFakeSchedulerTimer()
	var runs atomic.Int32
	s := New(10*time.Minute, func() error {
		runs.Add(1)
		return nil
	})
	s.now = clock.now
	s.newTimer = func(time.Duration) schedulerTimer { return timer }
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.Run(ctx)
	waitFor(t, func() bool { return !s.NextRun().IsZero() }, time.Second, "scheduler startup")

	clock.advance(10 * time.Minute)
	timer.ch <- clock.now()
	waitFor(t, func() bool { return runs.Load() == 1 }, time.Second, "periodic run")

	select {
	case got := <-timer.resets:
		if got != 10*time.Minute {
			t.Fatalf("reset interval = %v, want 10m", got)
		}
	case <-time.After(time.Second):
		t.Fatal("periodic cycle did not reset timer")
	}
	if got := s.NextRun(); !got.Equal(clock.now().Add(10 * time.Minute)) {
		t.Fatalf("NextRun() = %v", got)
	}
}

func TestStaleTimerEventDoesNotRunBeforeRearmedDeadline(t *testing.T) {
	clock := newFakeSchedulerClock(time.Date(2026, 8, 18, 8, 0, 0, 0, time.UTC))
	timer := newFakeSchedulerTimer()
	var runs atomic.Int32
	s := New(10*time.Minute, func() error {
		runs.Add(1)
		return nil
	})
	s.now = clock.now
	s.newTimer = func(time.Duration) schedulerTimer { return timer }
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.Run(ctx)
	waitFor(t, func() bool { return !s.NextRun().IsZero() }, time.Second, "scheduler startup")

	clock.advance(10 * time.Minute)
	s.SetInterval(20 * time.Minute)
	select {
	case <-timer.resets:
	case <-time.After(time.Second):
		t.Fatal("interval change did not reset timer")
	}

	timer.ch <- clock.now()
	select {
	case got := <-timer.resets:
		if got != 20*time.Minute {
			t.Fatalf("stale event reset interval = %v, want 20m", got)
		}
	case <-time.After(time.Second):
		t.Fatal("stale timer event did not preserve rearmed deadline")
	}
	if got := runs.Load(); got != 0 {
		t.Fatalf("runs after stale timer event = %d, want 0", got)
	}
}

func TestPauseClearsAndResumeRearmsNextRun(t *testing.T) {
	clock := newFakeSchedulerClock(time.Date(2026, 8, 18, 8, 0, 0, 0, time.UTC))
	timer := newFakeSchedulerTimer()
	s := New(10*time.Minute, func() error { return nil })
	s.now = clock.now
	s.newTimer = func(time.Duration) schedulerTimer { return timer }
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.Run(ctx)
	waitFor(t, func() bool { return !s.NextRun().IsZero() }, time.Second, "scheduler startup")

	s.Pause()
	if got := s.NextRun(); !got.IsZero() {
		t.Fatalf("paused NextRun() = %v, want zero", got)
	}

	clock.advance(time.Minute)
	s.Resume()
	select {
	case got := <-timer.resets:
		if got != 10*time.Minute {
			t.Fatalf("resume reset interval = %v, want 10m", got)
		}
	case <-time.After(time.Second):
		t.Fatal("resume did not reset timer")
	}
	if got := s.NextRun(); !got.Equal(clock.now().Add(10 * time.Minute)) {
		t.Fatalf("resumed NextRun() = %v", got)
	}
}

func TestCancellationClearsNextRun(t *testing.T) {
	clock := newFakeSchedulerClock(time.Date(2026, 8, 18, 8, 0, 0, 0, time.UTC))
	timer := newFakeSchedulerTimer()
	s := New(10*time.Minute, func() error { return nil })
	s.now = clock.now
	s.newTimer = func(time.Duration) schedulerTimer { return timer }
	ctx, cancel := context.WithCancel(context.Background())
	go s.Run(ctx)
	waitFor(t, func() bool { return !s.NextRun().IsZero() }, time.Second, "scheduler startup")

	cancel()
	select {
	case <-timer.stopped:
	case <-time.After(time.Second):
		t.Fatal("scheduler timer did not stop")
	}
	if got := s.NextRun(); !got.IsZero() {
		t.Fatalf("cancelled NextRun() = %v, want zero", got)
	}
}

func TestCancellationDuringJobClearsNextRunAndDropsQueuedTrigger(t *testing.T) {
	clock := newFakeSchedulerClock(time.Date(2026, 8, 18, 8, 0, 0, 0, time.UTC))
	timer := newFakeSchedulerTimer()
	started := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseJob := func() {
		releaseOnce.Do(func() { close(release) })
	}
	defer releaseJob()
	var runs atomic.Int32
	s := New(10*time.Minute, func() error {
		if runs.Add(1) == 1 {
			close(started)
			<-release
		}
		return nil
	})
	s.now = clock.now
	s.newTimer = func(time.Duration) schedulerTimer { return timer }
	ctx, cancel := context.WithCancel(context.Background())
	runDone := make(chan struct{})
	go func() {
		s.Run(ctx)
		close(runDone)
	}()
	waitFor(t, func() bool { return !s.NextRun().IsZero() }, time.Second, "scheduler startup")

	s.Trigger()
	<-started
	s.Trigger()
	cancel()
	waitFor(t, func() bool { return s.NextRun().IsZero() }, time.Second, "cancelled next run")
	releaseJob()

	select {
	case <-runDone:
	case <-time.After(time.Second):
		t.Fatal("scheduler did not stop after blocked job returned")
	}
	if got := runs.Load(); got != 1 {
		t.Fatalf("runs after cancellation = %d, want 1", got)
	}

	nextCtx, nextCancel := context.WithCancel(context.Background())
	nextRunDone := make(chan struct{})
	go func() {
		s.Run(nextCtx)
		close(nextRunDone)
	}()
	waitFor(t, func() bool { return !s.NextRun().IsZero() }, time.Second, "restarted scheduler")
	time.Sleep(20 * time.Millisecond)
	if got := runs.Load(); got != 1 {
		t.Fatalf("stale trigger ran after restart: runs = %d", got)
	}
	nextCancel()
	select {
	case <-nextRunDone:
	case <-time.After(time.Second):
		t.Fatal("restarted scheduler did not stop")
	}
}
