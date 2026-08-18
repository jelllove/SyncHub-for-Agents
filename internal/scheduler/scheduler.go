// Package scheduler runs a job periodically with manual trigger and pause.
package scheduler

import (
	"context"
	"sync"
	"time"
)

// State is the scheduler's current status, surfaced to the UI.
type State int

const (
	StateIdle State = iota
	StateUpdating
	StateDone
	StateError
	StatePaused
)

func (s State) String() string {
	switch s {
	case StateIdle:
		return "idle"
	case StateUpdating:
		return "updating"
	case StateDone:
		return "done"
	case StateError:
		return "error"
	case StatePaused:
		return "paused"
	default:
		return "unknown"
	}
}

// Job performs one unit of work (a full sync pass). Returning an error moves the
// scheduler into StateError for that cycle.
type Job func() error

type schedulerTimer interface {
	C() <-chan time.Time
	Reset(time.Duration)
	Stop()
}

type realSchedulerTimer struct {
	timer *time.Timer
}

func (t *realSchedulerTimer) C() <-chan time.Time {
	return t.timer.C
}

func (t *realSchedulerTimer) Reset(interval time.Duration) {
	t.timer.Reset(interval)
}

func (t *realSchedulerTimer) Stop() {
	t.timer.Stop()
}

// Scheduler runs Job every Interval, with manual Trigger and Pause/Resume.
// OnState (if set) is called on every state transition. All methods are safe
// for concurrent use.
type Scheduler struct {
	Job Job

	mu          sync.Mutex
	interval    time.Duration
	paused      bool
	state       State
	nextRun     time.Time
	running     bool
	cancelled   bool
	nextSubID   uint64
	subscribers map[uint64]func(State)

	trigger        chan struct{}
	intervalChange chan struct{}
	now            func() time.Time
	newTimer       func(time.Duration) schedulerTimer
}

// New returns a Scheduler that runs job every interval.
func New(interval time.Duration, job Job) *Scheduler {
	if interval <= 0 {
		interval = 10 * time.Minute
	}
	return &Scheduler{
		Job:            job,
		interval:       interval,
		state:          StateIdle,
		subscribers:    map[uint64]func(State){},
		trigger:        make(chan struct{}, 1),
		intervalChange: make(chan struct{}, 1),
		now:            time.Now,
		newTimer: func(interval time.Duration) schedulerTimer {
			return &realSchedulerTimer{timer: time.NewTimer(interval)}
		},
	}
}

// IntervalDuration returns the current interval.
func (s *Scheduler) IntervalDuration() time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.interval
}

// SetInterval updates the running schedule. Non-positive values restore the
// default interval.
func (s *Scheduler) SetInterval(interval time.Duration) {
	if interval <= 0 {
		interval = 10 * time.Minute
	}
	s.mu.Lock()
	s.interval = interval
	if s.running && !s.paused && !s.cancelled {
		s.nextRun = s.now().Add(interval)
	}
	s.mu.Unlock()
	s.notifyIntervalChange()
}

// Subscribe registers a state observer and returns an unsubscribe function.
func (s *Scheduler) Subscribe(observer func(State)) func() {
	s.mu.Lock()
	id := s.nextSubID
	s.nextSubID++
	s.subscribers[id] = observer
	s.mu.Unlock()
	return func() {
		s.mu.Lock()
		delete(s.subscribers, id)
		s.mu.Unlock()
	}
}

// Trigger requests an immediate run. Non-blocking; coalesces a pending request.
func (s *Scheduler) Trigger() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running && s.cancelled {
		return
	}
	select {
	case s.trigger <- struct{}{}:
	default:
	}
}

// Pause stops periodic and triggered runs until Resume.
func (s *Scheduler) Pause() {
	s.mu.Lock()
	s.paused = true
	s.nextRun = time.Time{}
	s.mu.Unlock()
	s.setState(StatePaused)
	s.notifyIntervalChange()
}

// Resume re-enables runs.
func (s *Scheduler) Resume() {
	s.mu.Lock()
	s.paused = false
	if s.running && !s.cancelled {
		s.nextRun = s.now().Add(s.interval)
	}
	s.mu.Unlock()
	s.setState(StateIdle)
	s.notifyIntervalChange()
}

// State returns the current state.
func (s *Scheduler) State() State {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state
}

// NextRun returns the authoritative next scheduled run, or zero while stopped
// or paused.
func (s *Scheduler) NextRun() time.Time {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.nextRun
}

func (s *Scheduler) isPaused() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.paused
}

func (s *Scheduler) setState(st State) {
	s.mu.Lock()
	s.state = st
	observers := make([]func(State), 0, len(s.subscribers))
	for _, observer := range s.subscribers {
		observers = append(observers, observer)
	}
	s.mu.Unlock()
	for _, observer := range observers {
		observer(st)
	}
}

// runCycle runs the job once and updates state, unless paused.
func (s *Scheduler) runCycle() {
	s.runCycleBeforeTerminal(nil)
}

func (s *Scheduler) runCycleBeforeTerminal(beforeTerminal func() bool) {
	if s.isPaused() {
		return
	}
	s.setState(StateUpdating)
	err := s.Job()
	if beforeTerminal != nil && !beforeTerminal() {
		return
	}
	if s.isPaused() {
		return // a Pause arrived during the run; keep the paused state
	}
	if err != nil {
		s.setState(StateError)
		return
	}
	s.setState(StateDone)
}

// Run blocks executing the schedule until ctx is cancelled.
func (s *Scheduler) Run(ctx context.Context) {
	interval, armed := s.beginRun()
	if !armed {
		return
	}
	timer := s.newTimer(interval)
	if s.NextRun().IsZero() {
		timer.Stop()
	}
	stopCancellationWatch := make(chan struct{})
	cancellationWatchDone := make(chan struct{})
	go func() {
		defer close(cancellationWatchDone)
		select {
		case <-ctx.Done():
			s.cancelRun()
		case <-stopCancellationWatch:
		}
	}()
	defer func() {
		close(stopCancellationWatch)
		<-cancellationWatchDone
		if ctx.Err() != nil {
			s.cancelRun()
		}
		s.endRun()
		timer.Stop()
	}()
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.intervalChange:
			if ctx.Err() != nil {
				return
			}
			s.resetTimer(timer)
		case firedAt := <-timer.C():
			if ctx.Err() != nil {
				return
			}
			if !s.timerEventCurrent(firedAt) {
				s.resetTimer(timer)
				continue
			}
			s.runAndRearm(ctx, timer)
		case <-s.trigger:
			if ctx.Err() != nil {
				return
			}
			s.runAndRearm(ctx, timer)
		}
	}
}

func (s *Scheduler) runAndRearm(ctx context.Context, timer schedulerTimer) {
	timer.Stop()
	s.runCycleBeforeTerminal(func() bool {
		if ctx.Err() != nil {
			s.cancelRun()
			return false
		}
		return s.rearmAfterCycle(timer)
	})
}

func (s *Scheduler) notifyIntervalChange() {
	select {
	case s.intervalChange <- struct{}{}:
	default:
	}
}

func (s *Scheduler) beginRun() (time.Duration, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running {
		return 0, false
	}
	s.running = true
	s.cancelled = false
	if !s.paused {
		s.nextRun = s.now().Add(s.interval)
	} else {
		s.nextRun = time.Time{}
	}
	return s.interval, true
}

func (s *Scheduler) cancelRun() {
	s.mu.Lock()
	if s.running {
		s.cancelled = true
		s.nextRun = time.Time{}
		select {
		case <-s.trigger:
		default:
		}
	}
	s.mu.Unlock()
}

func (s *Scheduler) endRun() {
	s.mu.Lock()
	s.running = false
	s.cancelled = false
	s.nextRun = time.Time{}
	s.mu.Unlock()
}

func (s *Scheduler) resetTimer(timer schedulerTimer) {
	timer.Stop()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.paused || !s.running || s.cancelled || s.nextRun.IsZero() {
		return
	}
	delay := s.nextRun.Sub(s.now())
	if delay < 0 {
		delay = 0
	}
	timer.Reset(delay)
}

func (s *Scheduler) timerEventCurrent(firedAt time.Time) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.paused || !s.running || s.cancelled || s.nextRun.IsZero() {
		return false
	}
	return !firedAt.Before(s.nextRun)
}

func (s *Scheduler) rearmAfterCycle(timer schedulerTimer) bool {
	s.mu.Lock()
	if s.paused || !s.running || s.cancelled {
		s.nextRun = time.Time{}
		s.mu.Unlock()
		return false
	}
	interval := s.interval
	s.nextRun = s.now().Add(interval)
	s.mu.Unlock()
	timer.Reset(interval)
	return true
}
