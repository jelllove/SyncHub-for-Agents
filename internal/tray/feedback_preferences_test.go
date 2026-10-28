package tray

import (
	"testing"
	"time"

	"github.com/qinqingxu/synchub-for-agents/internal/scheduler"
)

func TestActiveTipsHideAfterFiveSecondsAndKeepUpdatingIcon(t *testing.T) {
	f, updates, timers := feedbackFixture()
	defer f.Close()
	f.Ready()
	f.State(scheduler.StateUpdating)
	if err := f.Progress("pulling"); err != nil {
		t.Fatal(err)
	}
	if len(*timers) == 0 {
		t.Fatal("active pulling tip has no hide timer")
	}
	timer := (*timers)[len(*timers)-1]
	if timer.delay != 5*time.Second {
		t.Fatalf("active visibility duration = %s, want 5s", timer.delay)
	}
	timer.fire()
	last := (*updates)[len(*updates)-1]
	if last.Show || last.IconState != scheduler.StateUpdating || last.Tip.Kind != "pulling" {
		t.Fatalf("timeout affected tray operation status: %+v", last)
	}
	count := len(*updates)
	if err := f.Progress("pulling"); err != nil {
		t.Fatal(err)
	}
	if len(*updates) != count {
		t.Fatal("duplicate phase resurrected a timed-out popup")
	}
	if err := f.Progress("uploading"); err != nil {
		t.Fatal(err)
	}
	if !(*updates)[len(*updates)-1].Show {
		t.Fatal("new phase did not display after prior popup timed out")
	}
}

func TestPopupOptOutHidesAllKindsWithoutDisablingTrayStatus(t *testing.T) {
	f, updates, timers := feedbackFixture()
	defer f.Close()
	f.Ready()
	f.State(scheduler.StateUpdating)
	f.SetEnabled(false)
	if (*updates)[len(*updates)-1].Show {
		t.Fatal("disabling popup did not hide it immediately")
	}
	timerCount := len(*timers)
	_ = f.Progress("uploading")
	f.Finish(true, false)
	last := (*updates)[len(*updates)-1]
	if last.Show || last.Tip.Kind != "error" || last.IconState != scheduler.StateError ||
		len(*timers) != timerCount {
		t.Fatalf("opt-out disabled status or created a popup timer: %+v", last)
	}
	f.SetEnabled(true)
	if (*updates)[len(*updates)-1].Show {
		t.Fatal("reenabling replayed an old error popup")
	}
	f.State(scheduler.StateUpdating)
	if !(*updates)[len(*updates)-1].Show {
		t.Fatal("reenabling did not allow a new run's popup")
	}
}

func TestAllTipKindsHaveBoundedTimersAndRejectOldCallbacks(t *testing.T) {
	for _, outcome := range []struct {
		failed, attention bool
		delay             time.Duration
	}{
		{false, false, 5 * time.Second}, {true, false, 10 * time.Second}, {false, true, 10 * time.Second},
	} {
		f, updates, timers := feedbackFixture()
		f.Ready()
		f.State(scheduler.StateUpdating)
		f.Finish(outcome.failed, outcome.attention)
		timer := (*timers)[len(*timers)-1]
		if timer.delay != outcome.delay {
			t.Fatalf("visibility = %s, want %s", timer.delay, outcome.delay)
		}
		f.State(scheduler.StateUpdating)
		count := len(*updates)
		timer.fire()
		if len(*updates) != count {
			t.Fatal("old timer hid new phase")
		}
		f.Close()
	}
}
