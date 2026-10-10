package tray

import (
	"fmt"
	"sync"
	"time"

	"github.com/qinqingxu/synchub-for-agents/internal/scheduler"
)

type Tip struct {
	Revision uint64 `json:"revision"`
	Kind     string `json:"kind"`
	Title    string `json:"title"`
	Message  string `json:"message"`
}

type FeedbackUpdate struct {
	Tip       Tip
	Tooltip   string
	IconState scheduler.State
	Show      bool
}

type tipTimer interface {
	Stop() bool
}

type Feedback struct {
	mu            sync.Mutex
	delivery      sync.Mutex
	tip           Tip
	active        bool
	paused        bool
	finished      bool
	ready         bool
	dismissed     bool
	visible       bool
	closed        bool
	timer         tipTimer
	after         func(time.Duration, func()) tipTimer
	render        func(FeedbackUpdate) error
	report        func(error)
	delivered     bool
	lastDelivered uint64
}

func NewFeedback(render func(FeedbackUpdate) error, report func(error)) *Feedback {
	return &Feedback{
		tip:    Tip{Kind: "ready", Title: "Ready", Message: "SyncHub for Agents is running in the tray."},
		render: render, report: report,
		after: func(delay time.Duration, callback func()) tipTimer { return time.AfterFunc(delay, callback) },
	}
}

func (f *Feedback) State(state scheduler.State) {
	f.mu.Lock()
	if f.closed {
		f.mu.Unlock()
		return
	}
	switch state {
	case scheduler.StateUpdating:
		if !f.active {
			f.active, f.finished, f.dismissed, f.paused = true, false, false, false
			f.setLocked(Tip{Kind: "syncing", Title: "Syncing", Message: "Preparing to synchronize selected resources."}, true)
		}
	case scheduler.StatePaused:
		f.paused = true
		if !f.active && f.tip.Kind != "error" && f.tip.Kind != "warning" {
			f.setLocked(Tip{Kind: "paused", Title: "Paused", Message: "Automatic synchronization is paused."}, true)
		}
	case scheduler.StateIdle:
		f.paused = false
		if !f.active && f.tip.Kind != "error" && f.tip.Kind != "warning" {
			f.setLocked(Tip{Kind: "ready", Title: "Ready", Message: "SyncHub for Agents is running in the tray."}, false)
		}
	case scheduler.StateDone, scheduler.StateError:
		if !f.finished {
			f.finishLocked(state == scheduler.StateError, false)
		}
	default:
		f.mu.Unlock()
		f.report(fmt.Errorf("unsupported tray state %q", state))
		return
	}
	update := f.updateLocked()
	f.mu.Unlock()
	f.deliver(update)
}

func (f *Feedback) Progress(stage string) error {
	tip, err := progressTip(stage)
	if err != nil {
		return err
	}
	if stage == "complete" {
		return nil
	}
	f.mu.Lock()
	if f.closed || !f.active {
		f.mu.Unlock()
		return nil
	}
	if f.paused {
		tip.Message += " Automatic syncing is paused; this run is finishing."
	}
	f.setLocked(tip, !f.dismissed)
	update := f.updateLocked()
	f.mu.Unlock()
	f.deliver(update)
	return nil
}

func progressTip(stage string) (Tip, error) {
	switch stage {
	case "pulling":
		return Tip{Kind: "pulling", Title: "Pulling", Message: "Retrieving the latest changes from your sync repository."}, nil
	case "scanning":
		return Tip{Kind: "scanning", Title: "Scanning", Message: "Finding portable resources and applying safety exclusions."}, nil
	case "comparing":
		return Tip{Kind: "comparing", Title: "Comparing", Message: "Comparing local and remote resources."}, nil
	case "applying":
		return Tip{Kind: "applying", Title: "Applying", Message: "Applying synchronized resources on this computer."}, nil
	case "uploading":
		return Tip{Kind: "pushing", Title: "Pushing", Message: "Uploading synchronized changes to your sync repository."}, nil
	case "complete":
		return Tip{}, nil
	default:
		return Tip{}, fmt.Errorf("unsupported tray progress stage %q", stage)
	}
}

func (f *Feedback) Finish(failed, needsAttention bool) {
	f.mu.Lock()
	if f.closed {
		f.mu.Unlock()
		return
	}
	f.finishLocked(failed, needsAttention)
	update := f.updateLocked()
	f.mu.Unlock()
	f.deliver(update)
}

func (f *Feedback) finishLocked(failed, needsAttention bool) {
	f.active, f.finished = false, true
	tip := Tip{Kind: "done", Title: "Sync complete", Message: "Your selected resources have been synchronized."}
	if failed {
		tip = Tip{Kind: "error", Title: "Sync error", Message: "Synchronization failed. Open SyncHub for Agents to review the details."}
		f.dismissed = false
	} else if needsAttention {
		tip = Tip{Kind: "warning", Title: "Needs attention", Message: "Review conflicts, blocked files or pending installations in the app."}
		f.dismissed = false
	}
	f.setLocked(tip, !f.dismissed)
}

func (f *Feedback) Reviewed(needsAttention bool) {
	f.mu.Lock()
	if f.closed || needsAttention || f.tip.Kind != "warning" {
		f.mu.Unlock()
		return
	}
	f.setLocked(Tip{Kind: "done", Title: "Sync complete", Message: "Synchronization notices have been reviewed."}, false)
	update := f.updateLocked()
	f.mu.Unlock()
	f.deliver(update)
}

func (f *Feedback) Ready() {
	f.mu.Lock()
	if f.closed || f.ready {
		f.mu.Unlock()
		return
	}
	f.ready = true
	f.tip.Revision++
	f.scheduleHideLocked()
	update := f.updateLocked()
	f.mu.Unlock()
	f.deliver(update)
}

func (f *Feedback) Dismiss() {
	f.mu.Lock()
	if f.closed {
		f.mu.Unlock()
		return
	}
	f.dismissed, f.visible = true, false
	f.tip.Revision++
	f.stopTimerLocked()
	update := f.updateLocked()
	f.mu.Unlock()
	f.deliver(update)
}

func (f *Feedback) Close() {
	f.mu.Lock()
	f.closed = true
	f.stopTimerLocked()
	f.mu.Unlock()
}

func (f *Feedback) setLocked(tip Tip, visible bool) {
	if f.tip.Kind == tip.Kind && f.tip.Message == tip.Message && f.visible == visible {
		return
	}
	tip.Revision = f.tip.Revision + 1
	f.tip, f.visible = tip, visible
	f.stopTimerLocked()
	f.scheduleHideLocked()
}

func (f *Feedback) scheduleHideLocked() {
	if !f.ready || !f.visible || f.active {
		return
	}
	delay := 4 * time.Second
	if f.tip.Kind == "error" || f.tip.Kind == "warning" {
		delay = 12 * time.Second
	}
	revision := f.tip.Revision
	f.timer = f.after(delay, func() {
		f.mu.Lock()
		if f.closed || f.tip.Revision != revision {
			f.mu.Unlock()
			return
		}
		f.visible = false
		f.tip.Revision++
		update := f.updateLocked()
		f.mu.Unlock()
		f.deliver(update)
	})
}

func (f *Feedback) stopTimerLocked() {
	if f.timer != nil {
		f.timer.Stop()
		f.timer = nil
	}
}

func (f *Feedback) updateLocked() FeedbackUpdate {
	state := scheduler.StateIdle
	switch {
	case f.tip.Kind == "error" || f.tip.Kind == "warning":
		state = scheduler.StateError
	case f.active:
		state = scheduler.StateUpdating
	case f.paused:
		state = scheduler.StatePaused
	case f.tip.Kind == "done":
		state = scheduler.StateDone
	}
	return FeedbackUpdate{
		Tip: f.tip, Tooltip: "SyncHub for Agents: " + f.tip.Title,
		IconState: state, Show: f.ready && f.visible,
	}
}

func (f *Feedback) deliver(update FeedbackUpdate) {
	f.delivery.Lock()
	defer f.delivery.Unlock()
	f.mu.Lock()
	current := !f.closed && update.Tip.Revision == f.tip.Revision
	f.mu.Unlock()
	if !current || (f.delivered && update.Tip.Revision <= f.lastDelivered) {
		return
	}
	f.delivered, f.lastDelivered = true, update.Tip.Revision
	if err := f.render(update); err != nil {
		f.report(err)
	}
}
