package tray

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/qinqingxu/synchub-for-agents/internal/scheduler"
)

type fakeTipTimer struct {
	stopped bool
	fire    func()
	delay   time.Duration
}

func (timer *fakeTipTimer) Stop() bool {
	timer.stopped = true
	return true
}

func feedbackFixture() (*Feedback, *[]FeedbackUpdate, *[]*fakeTipTimer) {
	updates := &[]FeedbackUpdate{}
	timers := &[]*fakeTipTimer{}
	feedback := NewFeedback(func(update FeedbackUpdate) error {
		*updates = append(*updates, update)
		return nil
	}, func(error) {})
	feedback.after = func(delay time.Duration, fire func()) tipTimer {
		timer := &fakeTipTimer{fire: fire, delay: delay}
		*timers = append(*timers, timer)
		return timer
	}
	return feedback, updates, timers
}

func TestTrayFeedbackMapsAndDeduplicatesSyncStages(t *testing.T) {
	feedback, updates, _ := feedbackFixture()
	defer feedback.Close()
	feedback.Ready()
	feedback.State(scheduler.StateUpdating)
	for _, phase := range []struct{ stage, kind, title string }{
		{"pulling", "pulling", "Pulling"}, {"scanning", "scanning", "Scanning"},
		{"comparing", "comparing", "Comparing"}, {"applying", "applying", "Applying"},
		{"uploading", "pushing", "Pushing"},
	} {
		if err := feedback.Progress(phase.stage); err != nil {
			t.Fatal(err)
		}
		update := (*updates)[len(*updates)-1]
		if update.Tip.Kind != phase.kind || update.Tip.Title != phase.title ||
			!update.Show || update.IconState != scheduler.StateUpdating || update.Tooltip != "SyncHub for Agents: "+phase.title {
			t.Fatalf("stage %s = %#v", phase.stage, update)
		}
		count := len(*updates)
		for i := 0; i < 50; i++ {
			if err := feedback.Progress(phase.stage); err != nil {
				t.Fatal(err)
			}
		}
		if len(*updates) != count {
			t.Fatal("per-file progress must not repeat tray notifications")
		}
	}
	if err := feedback.Progress("unknown"); err == nil {
		t.Fatal("unknown stages must be surfaced rather than presented as success")
	}
}

func TestTrayFeedbackReplaysOnlyLatestStageWhenRendererIsReady(t *testing.T) {
	feedback, updates, _ := feedbackFixture()
	defer feedback.Close()
	feedback.State(scheduler.StateUpdating)
	if err := feedback.Progress("pulling"); err != nil {
		t.Fatal(err)
	}
	if err := feedback.Progress("uploading"); err != nil {
		t.Fatal(err)
	}
	for _, update := range *updates {
		if update.Show {
			t.Fatal("must not display the tip before its renderer subscribes")
		}
	}
	feedback.Ready()
	if update := (*updates)[len(*updates)-1]; !update.Show || update.Tip.Kind != "pushing" {
		t.Fatalf("ready replay = %#v", update)
	}
}

func TestTrayFeedbackErrorOverridesDismissedProgressAndPersistsInTooltip(t *testing.T) {
	feedback, updates, timers := feedbackFixture()
	defer feedback.Close()
	feedback.Ready()
	feedback.State(scheduler.StateUpdating)
	feedback.Dismiss()
	if err := feedback.Progress("uploading"); err != nil {
		t.Fatal(err)
	}
	if (*updates)[len(*updates)-1].Show {
		t.Fatal("dismissed cycle must not pop up at every new phase")
	}
	feedback.Finish(true, false)
	update := (*updates)[len(*updates)-1]
	if !update.Show || update.Tip.Kind != "error" || update.IconState != scheduler.StateError {
		t.Fatalf("error must remain visible after progress dismissal: %#v", update)
	}
	timer := (*timers)[len(*timers)-1]
	if timer.delay < 10*time.Second {
		t.Fatal("error tips need more time than ordinary completion tips")
	}
	timer.fire()
	update = (*updates)[len(*updates)-1]
	if update.Show || update.Tip.Kind != "error" || update.IconState != scheduler.StateError {
		t.Fatalf("error tooltip/icon must remain after the tip hides: %#v", update)
	}
}

func TestTrayFeedbackCompletionTimerCannotHideANewCycle(t *testing.T) {
	feedback, updates, timers := feedbackFixture()
	defer feedback.Close()
	feedback.Ready()
	feedback.State(scheduler.StateUpdating)
	feedback.Finish(false, false)
	timer := (*timers)[len(*timers)-1]
	feedback.State(scheduler.StateDone)
	feedback.State(scheduler.StateUpdating)
	if err := feedback.Progress("pulling"); err != nil {
		t.Fatal(err)
	}
	count := len(*updates)
	timer.fire()
	if !timer.stopped || len(*updates) != count || !(*updates)[count-1].Show {
		t.Fatal("a stale completion timer affected a newer synchronization")
	}
}

func TestTrayFeedbackReportsWarningsAndPausedCycleFailures(t *testing.T) {
	feedback, updates, _ := feedbackFixture()
	defer feedback.Close()
	feedback.Ready()
	feedback.State(scheduler.StateUpdating)
	feedback.Finish(false, true)
	if update := (*updates)[len(*updates)-1]; update.Tip.Kind != "warning" || update.IconState != scheduler.StateError {
		t.Fatalf("attention must not appear as clean success: %#v", update)
	}
	feedback.State(scheduler.StateUpdating)
	feedback.State(scheduler.StatePaused)
	feedback.Finish(true, false)
	if update := (*updates)[len(*updates)-1]; update.Tip.Kind != "error" || !update.Show {
		t.Fatalf("paused failures must still show error feedback: %#v", update)
	}
}

func TestTrayFeedbackSurfacesDeliveryFailuresAndStopsAfterClose(t *testing.T) {
	fail := errors.New("tip positioning failed")
	var reported error
	feedback := NewFeedback(func(FeedbackUpdate) error { return fail }, func(err error) { reported = err })
	feedback.Ready()
	feedback.State(scheduler.StateUpdating)
	if !errors.Is(reported, fail) {
		t.Fatal("notification failures were silently swallowed")
	}
	feedback.Close()
	reported = nil
	feedback.State(scheduler.StateUpdating)
	feedback.Finish(true, false)
	feedback.Ready()
	if reported != nil {
		t.Fatal("feedback attempted delivery after shutdown")
	}
}

func TestTrayFeedbackConcurrentEventsStayOrdered(t *testing.T) {
	var mu sync.Mutex
	var revisions []uint64
	feedback := NewFeedback(func(update FeedbackUpdate) error {
		mu.Lock()
		revisions = append(revisions, update.Tip.Revision)
		mu.Unlock()
		return nil
	}, func(error) {})
	defer feedback.Close()
	feedback.Ready()
	feedback.State(scheduler.StateUpdating)
	var group sync.WaitGroup
	for _, stage := range []string{"pulling", "scanning", "comparing", "applying", "uploading"} {
		group.Go(func() { _ = feedback.Progress(stage) })
	}
	group.Wait()
	for i := 1; i < len(revisions); i++ {
		if revisions[i] <= revisions[i-1] {
			t.Fatalf("stale tray events were delivered out of order: %v", revisions)
		}
	}
}
