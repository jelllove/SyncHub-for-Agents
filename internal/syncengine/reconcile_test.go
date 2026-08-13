package syncengine

import (
	"testing"

	"github.com/qinqingxu/acsync/internal/state"
)

func meta(hash string, mtime int64) state.FileMeta {
	return state.FileMeta{Hash: hash, ModTime: mtime, Size: 1}
}

// find returns the action for a path, or nil.
func find(actions []Action, repoRel string) *Action {
	for i := range actions {
		if actions[i].RepoRel == repoRel {
			return &actions[i]
		}
	}
	return nil
}

func TestReconcileLocalNewPushes(t *testing.T) {
	base := state.Snapshot{}
	local := state.Snapshot{"agents/c/config/a.json": meta("h1", 10)}
	remote := state.Snapshot{}
	a := find(Reconcile(base, local, remote), "agents/c/config/a.json")
	if a == nil || a.Type != PushToRemote {
		t.Fatalf("expected PushToRemote, got %+v", a)
	}
}

func TestReconcileRemoteNewPulls(t *testing.T) {
	base := state.Snapshot{}
	local := state.Snapshot{}
	remote := state.Snapshot{"agents/c/config/a.json": meta("h1", 10)}
	a := find(Reconcile(base, local, remote), "agents/c/config/a.json")
	if a == nil || a.Type != PullToLocal {
		t.Fatalf("expected PullToLocal, got %+v", a)
	}
}

func TestReconcileLocalDeletePropagates(t *testing.T) {
	base := state.Snapshot{"agents/c/config/a.json": meta("h1", 10)}
	local := state.Snapshot{}                                          // locally deleted
	remote := state.Snapshot{"agents/c/config/a.json": meta("h1", 10)} // unchanged remote
	a := find(Reconcile(base, local, remote), "agents/c/config/a.json")
	if a == nil || a.Type != DeleteRemote {
		t.Fatalf("expected DeleteRemote, got %+v", a)
	}
}

func TestReconcileRemoteDeletePropagates(t *testing.T) {
	base := state.Snapshot{"agents/c/config/a.json": meta("h1", 10)}
	local := state.Snapshot{"agents/c/config/a.json": meta("h1", 10)} // unchanged local
	remote := state.Snapshot{}                                        // deleted remote
	a := find(Reconcile(base, local, remote), "agents/c/config/a.json")
	if a == nil || a.Type != DeleteLocal {
		t.Fatalf("expected DeleteLocal, got %+v", a)
	}
}

func TestReconcileConflictLastWriterWins(t *testing.T) {
	base := state.Snapshot{"p": meta("h0", 5)}
	// Local newer.
	local := state.Snapshot{"p": meta("hL", 20)}
	remote := state.Snapshot{"p": meta("hR", 10)}
	a := find(Reconcile(base, local, remote), "p")
	if a == nil || a.Type != PushToRemote {
		t.Fatalf("expected PushToRemote (local newer), got %+v", a)
	}
	// Remote newer.
	local2 := state.Snapshot{"p": meta("hL", 10)}
	remote2 := state.Snapshot{"p": meta("hR", 20)}
	a2 := find(Reconcile(base, local2, remote2), "p")
	if a2 == nil || a2.Type != PullToLocal {
		t.Fatalf("expected PullToLocal (remote newer), got %+v", a2)
	}
}

func TestReconcileUnchangedNoAction(t *testing.T) {
	base := state.Snapshot{"p": meta("h1", 10)}
	local := state.Snapshot{"p": meta("h1", 10)}
	remote := state.Snapshot{"p": meta("h1", 10)}
	if got := Reconcile(base, local, remote); len(got) != 0 {
		t.Fatalf("expected no actions, got %v", got)
	}
}

func TestReconcileDeleteModifyResurrects(t *testing.T) {
	// Local deleted, remote modified -> keep remote copy (pull), avoid data loss.
	base := state.Snapshot{"p": meta("h0", 5)}
	local := state.Snapshot{}
	remote := state.Snapshot{"p": meta("hR", 20)}
	a := find(Reconcile(base, local, remote), "p")
	if a == nil || a.Type != PullToLocal {
		t.Fatalf("expected PullToLocal (resurrect), got %+v", a)
	}
}

func TestReconcileBlockedLocalNeverPullsRemote(t *testing.T) {
	remote := state.Snapshot{"agents/c/config/secret.json": meta("remote", 10)}
	actions := ReconcileWithBlocked(
		state.Snapshot{},
		state.Snapshot{},
		remote,
		[]string{"agents/c/config/secret.json"},
	)
	a := find(actions, "agents/c/config/secret.json")
	if a == nil || a.Type != RemoveRemote {
		t.Fatalf("blocked path should be removed remotely, got %+v", a)
	}
}
