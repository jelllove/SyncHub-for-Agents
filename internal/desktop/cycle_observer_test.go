package desktop

import (
	"path/filepath"
	"testing"

	"github.com/qinqingxu/synchub-for-agents/internal/daemon"
)

func TestCycleObserversReceiveEveryTerminalOutcomeOutsideServiceLock(t *testing.T) {
	service, err := New(t.TempDir(), "windows")
	if err != nil {
		t.Fatal(err)
	}
	var outcomes []daemon.CycleResult
	unsubscribe := service.SubscribeCycle(func(result daemon.CycleResult) {
		outcomes = append(outcomes, result)
		// Observers may query desktop state without deadlocking the cycle writer.
		service.Daemon()
	})
	service.recordCycle(daemon.CycleResult{Error: "synthetic pull error"})
	service.recordCycle(daemon.CycleResult{NeedsAttention: true, Conflicts: 1})
	if len(outcomes) != 2 || outcomes[0].Error == "" || !outcomes[1].NeedsAttention {
		t.Fatalf("terminal outcomes = %#v", outcomes)
	}
	unsubscribe()
	unsubscribe()
	service.recordCycle(daemon.CycleResult{})
	if len(outcomes) != 2 {
		t.Fatal("cycle callback survived unsubscription")
	}
}

func TestCycleObserversReceivePersistenceErrorsInsteadOfFalseSuccess(t *testing.T) {
	service, err := New(filepath.Join(t.TempDir(), "home"), "windows")
	if err != nil {
		t.Fatal(err)
	}
	resetFile(t, service.home)
	var received daemon.CycleResult
	service.SubscribeCycle(func(result daemon.CycleResult) { received = result })
	service.recordCycle(daemon.CycleResult{})
	if received.Error == "" || !received.NeedsAttention {
		t.Fatalf("persistence error appeared as a successful cycle: %#v", received)
	}
}
