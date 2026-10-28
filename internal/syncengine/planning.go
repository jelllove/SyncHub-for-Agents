package syncengine

import (
	"github.com/qinqingxu/synchub-for-agents/internal/resource"
	"github.com/qinqingxu/synchub-for-agents/internal/state"
)

type resourceActionPlan struct {
	base, local, remote        state.Snapshot
	skipPrefixes, blockedPaths []string
	actions                    []Action
}

// PrepareResourceActions computes the guarded plan used by status reporting
// without modifying the input snapshots. Sync applies first-run choices separately.
func PrepareResourceActions(
	base, local, remoteSnapshot, validRemote state.Snapshot,
	specs map[string]resource.Spec,
	skipped, blocked []resource.Issue,
) ([]Action, []string) {
	plan := prepareResourcePlan(base, local, remoteSnapshot, validRemote, specs, skipped, blocked)
	return plan.actions, plan.blockedPaths
}

func prepareResourcePlan(
	base, local, remoteSnapshot, validRemote state.Snapshot,
	specs map[string]resource.Spec,
	skipped, blocked []resource.Issue,
) resourceActionPlan {
	baseOwned, _, _ := SplitRemoteSnapshot(base, specs)
	local = cloneSnapshot(local)
	for repoRel, meta := range validRemote {
		if isInternalPortablePath(repoRel) {
			local[repoRel] = meta
		}
	}
	skipPrefixes := skippedRepoPrefixes(skipped, specs)
	local = withoutPrefixes(local, skipPrefixes)
	baseOwned = withoutPrefixes(baseOwned, skipPrefixes)
	validRemote = withoutPrefixes(validRemote, skipPrefixes)
	blockedPaths := blockedRepoPaths(blocked, specs)
	for _, repoRel := range blockedPaths {
		if meta, ok := remoteSnapshot[repoRel]; ok {
			validRemote[repoRel] = meta
		}
	}
	return resourceActionPlan{
		base: baseOwned, local: local, remote: validRemote,
		skipPrefixes: skipPrefixes, blockedPaths: blockedPaths,
		actions: ReconcileWithBlocked(baseOwned, local, validRemote, blockedPaths),
	}
}

func (e *Engine) applyFirstSyncStrategy(
	actions []Action,
	local state.Snapshot,
	remote state.Snapshot,
) []Action {
	switch e.FirstSyncMode {
	case "use-cloud":
		mapped := make([]Action, 0, len(actions))
		for _, action := range actions {
			switch action.Type {
			case PushToRemote:
				mapped = append(mapped, Action{Type: DeleteLocal, RepoRel: action.RepoRel})
			case MergeBoth:
				if _, exists := remote[action.RepoRel]; exists {
					mapped = append(mapped, Action{Type: PullToLocal, RepoRel: action.RepoRel})
				} else {
					mapped = append(mapped, Action{Type: DeleteLocal, RepoRel: action.RepoRel})
				}
			default:
				mapped = append(mapped, action)
			}
		}
		return mapped
	case "use-local":
		mapped := make([]Action, 0, len(actions))
		for _, action := range actions {
			switch action.Type {
			case PullToLocal:
				mapped = append(mapped, Action{Type: DeleteRemote, RepoRel: action.RepoRel})
			case MergeBoth:
				if _, exists := local[action.RepoRel]; exists {
					mapped = append(mapped, Action{Type: PushToRemote, RepoRel: action.RepoRel})
				} else {
					mapped = append(mapped, Action{Type: DeleteRemote, RepoRel: action.RepoRel})
				}
			default:
				mapped = append(mapped, action)
			}
		}
		return mapped
	default:
		return actions
	}
}

func cloneSnapshot(source state.Snapshot) state.Snapshot {
	out := make(state.Snapshot, len(source))
	for repoRel, meta := range source {
		out[repoRel] = meta
	}
	return out
}
