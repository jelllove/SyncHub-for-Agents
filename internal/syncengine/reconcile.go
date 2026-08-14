package syncengine

import (
	"sort"

	"github.com/qinqingxu/acsync/internal/state"
)

// ActionType enumerates the operations a sync pass can perform on one path.
type ActionType int

const (
	// PushToRemote copies the local file into the repo working tree.
	PushToRemote ActionType = iota
	// PullToLocal copies the repo file into the local agent directory.
	PullToLocal
	// DeleteRemote soft-deletes the repo file (moves it to .trash).
	DeleteRemote
	// DeleteLocal removes the local agent file.
	DeleteLocal
	// RemoveRemote deletes a blocked file from the repo without retaining it in trash.
	RemoveRemote
	// MergeBoth requests a strategy-specific three-way merge.
	MergeBoth
)

func (t ActionType) String() string {
	switch t {
	case PushToRemote:
		return "push"
	case PullToLocal:
		return "pull"
	case DeleteRemote:
		return "delete-remote"
	case DeleteLocal:
		return "delete-local"
	case RemoveRemote:
		return "remove-remote"
	case MergeBoth:
		return "merge"
	default:
		return "unknown"
	}
}

// Action is a single reconciliation operation on a repo-relative path.
type Action struct {
	Type    ActionType
	RepoRel string
}

// Reconcile compares base, local, and remote snapshots and returns the actions
// needed to converge. base is the snapshot recorded after the last successful
// sync; local is the current agent-dir snapshot; remote is the pulled repo
// snapshot.
func Reconcile(base, local, remote state.Snapshot) []Action {
	return ReconcileWithBlocked(base, local, remote, nil)
}

// ReconcileWithBlocked protects locally blocked files from remote writes and
// removes any current remote copy without retaining it in trash.
func ReconcileWithBlocked(base, local, remote state.Snapshot, blocked []string) []Action {
	keys := unionKeys(base, local, remote)
	var actions []Action
	protected := make(map[string]struct{}, len(blocked))
	for _, p := range blocked {
		protected[p] = struct{}{}
	}

	for _, p := range keys {
		b, inB := base[p]
		l, inL := local[p]
		r, inR := remote[p]
		if _, blocked := protected[p]; blocked {
			if inR {
				actions = append(actions, Action{RemoveRemote, p})
			}
			continue
		}

		localChanged := sideChanged(inB, b, inL, l)
		remoteChanged := sideChanged(inB, b, inR, r)

		switch {
		case !localChanged && !remoteChanged:
			// converged; nothing to do
		case localChanged && !remoteChanged:
			if inL {
				actions = append(actions, Action{PushToRemote, p})
			} else {
				actions = append(actions, Action{DeleteRemote, p})
			}
		case remoteChanged && !localChanged:
			if inR {
				actions = append(actions, Action{PullToLocal, p})
			} else {
				actions = append(actions, Action{DeleteLocal, p})
			}
		default:
			if a, ok := resolveConflict(p, inL, l, inR, r); ok {
				actions = append(actions, a)
			}
		}
	}
	return actions
}

// sideChanged reports whether a side differs from base.
func sideChanged(inBase bool, base state.FileMeta, inSide bool, side state.FileMeta) bool {
	if inBase != inSide {
		return true
	}
	if !inBase {
		return false
	}
	return base.Hash != side.Hash
}

// resolveConflict handles the both-sides-changed case.
func resolveConflict(p string, inL bool, l state.FileMeta, inR bool, r state.FileMeta) (Action, bool) {
	switch {
	case inL && inR:
		if l.Hash == r.Hash {
			return Action{}, false // same content, already converged
		}
		return Action{MergeBoth, p}, true
	case inL && !inR:
		return Action{MergeBoth, p}, true
	case !inL && inR:
		return Action{MergeBoth, p}, true
	default:
		// both deleted
		return Action{}, false
	}
}

func unionKeys(snaps ...state.Snapshot) []string {
	set := map[string]struct{}{}
	for _, s := range snaps {
		for k := range s {
			set[k] = struct{}{}
		}
	}
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
