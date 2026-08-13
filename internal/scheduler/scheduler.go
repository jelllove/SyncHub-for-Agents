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
	StateError
	StatePaused
)

func (s State) String() string {
	switch s {
	case StateIdle:
		return "idle"
	case StateUpdating:
		return "updating"
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

// Scheduler runs Job every Interval, with manual Trigger and Pause/Resume.
// OnState (if set) is called on every state transition. All methods are safe
// for concurrent use.
type Scheduler struct {
	Job Job

	mu          sync.Mutex
	interval    time.Duration
	paused      bool
	state       State
	nextSubID   uint64
	subscribers map[uint64]func(State)

	trigger        chan struct{}
	intervalChange chan struct{}
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
	s.mu.Unlock()
	select {
	case s.intervalChange <- struct{}{}:
	default:
	}
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
	select {
	case s.trigger <- struct{}{}:
	default:
	}
}

// Pause stops periodic and triggered runs until Resume.
func (s *Scheduler) Pause() {
	s.mu.Lock()
	s.paused = true
	s.mu.Unlock()
	s.setState(StatePaused)
}

// Resume re-enables runs.
func (s *Scheduler) Resume() {
	s.mu.Lock()
	s.paused = false
	s.mu.Unlock()
	s.setState(StateIdle)
}

// State returns the current state.
func (s *Scheduler) State() State {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state
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
	if s.isPaused() {
		return
	}
	s.setState(StateUpdating)
	err := s.Job()
	if s.isPaused() {
		return // a Pause arrived during the run; keep the paused state
	}
	if err != nil {
		s.setState(StateError)
		return
	}
	s.setState(StateIdle)
}

// Run blocks executing the schedule until ctx is cancelled.
func (s *Scheduler) Run(ctx context.Context) {
	ticker := time.NewTicker(s.IntervalDuration())
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.intervalChange:
			ticker.Reset(s.IntervalDuration())
		case <-ticker.C:
			s.runCycle()
		case <-s.trigger:
			s.runCycle()
		}
	}
}
