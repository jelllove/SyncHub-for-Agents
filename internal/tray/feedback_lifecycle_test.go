package tray

import (
	"testing"

	"github.com/qinqingxu/synchub-for-agents/internal/scheduler"
)

func TestTrayFeedbackReviewClearsWarningsButNotErrors(t *testing.T) {
	feedback, updates, timers := feedbackFixture()
	defer feedback.Close()
	feedback.Ready()
	feedback.State(scheduler.StateUpdating)
	feedback.Finish(false, true)
	timer := (*timers)[len(*timers)-1]
	count := len(*updates)
	feedback.Reviewed(true)
	if len(*updates) != count {
		t.Fatal("unreviewed attention should remain visible")
	}
	feedback.Reviewed(false)
	update := (*updates)[len(*updates)-1]
	if !timer.stopped || update.Show || update.IconState != scheduler.StateDone {
		t.Fatalf("reviewed warning did not clear: %#v", update)
	}
	feedback.State(scheduler.StateUpdating)
	feedback.Finish(true, false)
	count = len(*updates)
	feedback.Reviewed(false)
	if len(*updates) != count || (*updates)[count-1].Tip.Kind != "error" {
		t.Fatal("reviewing notices must not turn a synchronization error into success")
	}
}

func TestTrayFeedbackTerminalTimeoutStartsAfterRendererReadiness(t *testing.T) {
	feedback, updates, timers := feedbackFixture()
	defer feedback.Close()
	feedback.State(scheduler.StateUpdating)
	feedback.Finish(true, false)
	if len(*timers) != 0 {
		t.Fatal("a slow renderer must not consume the error visibility budget")
	}
	feedback.Ready()
	if len(*timers) != 1 || !(*updates)[len(*updates)-1].Show {
		t.Fatal("the terminal tip was not replayed when the renderer became ready")
	}
	feedback.Close()
	count := len(*updates)
	(*timers)[0].fire()
	if len(*updates) != count {
		t.Fatal("a timer survived feedback shutdown")
	}
}
