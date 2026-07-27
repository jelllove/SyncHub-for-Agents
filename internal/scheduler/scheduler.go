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
	Interval time.Duration
	Job      Job
	OnState  func(State)

	mu     sync.Mutex
	paused bool
	state  State

	trigger chan struct{}
}

// New returns a Scheduler that runs job every interval.
func New(interval time.Duration, job Job) *Scheduler {
	return &Scheduler{
		Interval: interval,
		Job:      job,
		state:    StateIdle,
		trigger:  make(chan struct{}, 1),
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
	cb := s.OnState
	s.mu.Unlock()
	if cb != nil {
		cb(st)
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
	ticker := time.NewTicker(s.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.runCycle()
		case <-s.trigger:
			s.runCycle()
		}
	}
}
