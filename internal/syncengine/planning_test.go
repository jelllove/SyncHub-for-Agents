package syncengine

import (
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"testing"

	"github.com/qinqingxu/synchub-for-agents/internal/resource"
	"github.com/qinqingxu/synchub-for-agents/internal/state"
)

type planningCase struct {
	base, local, remote, valid state.Snapshot
	specs                      map[string]resource.Spec
	skipped, blocked           []resource.Issue
	want                       []Action
	wantBlocked                []string
}

func planningSpec() resource.Spec {
	return resource.Spec{
		Key: "demo/global", Provider: "demo", ID: "global",
		Category: resource.CategoryInstructions, Strategy: resource.StrategyTextTree,
		Layout: resource.LayoutPortable,
	}
}

func planningPath(t *testing.T, relative string) string {
	t.Helper()
	value, err := planningSpec().RepoPath(relative)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func planningSnapshot(t *testing.T, hashes map[string]string) state.Snapshot {
	t.Helper()
	snapshot := state.Snapshot{}
	for relative, hash := range hashes {
		snapshot[planningPath(t, relative)] = state.FileMeta{Hash: hash, ModTime: 42, Size: int64(len(hash))}
	}
	return snapshot
}

func assertResourcePlanning(t *testing.T, tc planningCase) {
	t.Helper()
	original := []state.Snapshot{maps.Clone(tc.base), maps.Clone(tc.local), maps.Clone(tc.remote), maps.Clone(tc.valid)}
	skipped, blocked := slices.Clone(tc.skipped), slices.Clone(tc.blocked)
	actions, blockedPaths := PrepareResourceActions(tc.base, tc.local, tc.remote, tc.valid, tc.specs, tc.skipped, tc.blocked)
	if !slices.Equal(actions, tc.want) || !slices.Equal(blockedPaths, tc.wantBlocked) {
		t.Fatalf("planning = (%v, %v), want (%v, %v)", actions, blockedPaths, tc.want, tc.wantBlocked)
	}
	for index, current := range []state.Snapshot{tc.base, tc.local, tc.remote, tc.valid} {
		if !reflect.DeepEqual(original[index], current) {
			t.Fatalf("planning mutated input snapshot %d", index)
		}
	}
	if !reflect.DeepEqual(skipped, tc.skipped) || !reflect.DeepEqual(blocked, tc.blocked) {
		t.Fatal("planning mutated input issues")
	}
}

func TestPrepareResourceActionsPreservesOrderedDecisionsAndInputs(t *testing.T) {
	spec := planningSpec()
	valid := planningSnapshot(t, map[string]string{
		"b-remote.md": "remote", "c-local-deleted.md": "base",
		"e-conflict.md": "remote", "g-same.md": "same",
	})
	raw := maps.Clone(valid)
	blockedPath := planningPath(t, "f-blocked.md")
	raw[blockedPath] = state.FileMeta{Hash: "unsafe"}
	assertResourcePlanning(t, planningCase{
		specs: map[string]resource.Spec{spec.Key: spec},
		base: planningSnapshot(t, map[string]string{
			"c-local-deleted.md": "base", "d-remote-deleted.md": "base",
			"e-conflict.md": "base", "f-blocked.md": "base",
			"g-same.md": "same", "h-both-deleted.md": "base",
		}),
		local: planningSnapshot(t, map[string]string{
			"a-local.md": "local", "d-remote-deleted.md": "base",
			"e-conflict.md": "local", "f-blocked.md": "unsafe", "g-same.md": "same",
		}),
		remote: raw, valid: valid,
		blocked: []resource.Issue{
			{ResourceKey: spec.Key, Path: "f-blocked.md"},
			{Path: blockedPath},
		},
		want: []Action{
			{PushToRemote, planningPath(t, "a-local.md")},
			{PullToLocal, planningPath(t, "b-remote.md")},
			{DeleteRemote, planningPath(t, "c-local-deleted.md")},
			{DeleteLocal, planningPath(t, "d-remote-deleted.md")},
			{MergeBoth, planningPath(t, "e-conflict.md")},
			{RemoveRemote, blockedPath},
		},
		wantBlocked: []string{blockedPath},
	})
}

func TestPrepareResourceActionsRetainsBlockedRemovalUnderSkippedRoot(t *testing.T) {
	spec := planningSpec()
	raw := planningSnapshot(t, map[string]string{"keep.md": "remote", "blocked.md": "unsafe"})
	assertResourcePlanning(t, planningCase{
		specs:  map[string]resource.Spec{spec.Key: spec},
		base:   planningSnapshot(t, map[string]string{"keep.md": "base"}),
		local:  planningSnapshot(t, map[string]string{"keep.md": "local"}),
		remote: raw, valid: planningSnapshot(t, map[string]string{"keep.md": "remote"}),
		skipped:     []resource.Issue{{ResourceKey: spec.Key}},
		blocked:     []resource.Issue{{ResourceKey: spec.Key, Path: "blocked.md"}},
		want:        []Action{{RemoveRemote, planningPath(t, "blocked.md")}},
		wantBlocked: []string{planningPath(t, "blocked.md")},
	})
}

func TestPrepareResourceActionsUsesPathSegmentSkipBoundaries(t *testing.T) {
	spec := planningSpec()
	assertResourcePlanning(t, planningCase{
		specs:  map[string]resource.Spec{spec.Key: spec},
		base:   planningSnapshot(t, map[string]string{"nested/keep.md": "base"}),
		local:  planningSnapshot(t, map[string]string{"nested/keep.md": "local", "nested-other/new.md": "new"}),
		remote: planningSnapshot(t, map[string]string{"nested/keep.md": "remote"}),
		valid:  planningSnapshot(t, map[string]string{"nested/keep.md": "remote"}),
		skipped: []resource.Issue{
			{ResourceKey: spec.Key, Path: "nested"},
			{ResourceKey: "unknown", Path: "nested-other"},
		},
		want: []Action{{PushToRemote, planningPath(t, "nested-other/new.md")}},
	})
}

func TestPrepareResourceActionsDoesNotRestoreInternalOrUnownedMetadata(t *testing.T) {
	const internalPath = "agents/_portable/config/conflicts/example/record.json"
	const unknownPath = "agents/unknown/config/settings.json"
	assertResourcePlanning(t, planningCase{
		base:   state.Snapshot{unknownPath: {Hash: "old"}},
		remote: state.Snapshot{internalPath: {Hash: "record"}, unknownPath: {Hash: "new"}},
		valid:  state.Snapshot{internalPath: {Hash: "record"}},
	})
}

func TestResourcePlanKeepsFilteredStateAndExecutionViewsSeparate(t *testing.T) {
	spec := planningSpec()
	keep := planningPath(t, "keep.md")
	const internalPath = "agents/_portable/config/conflicts/example/record.json"
	base := planningSnapshot(t, map[string]string{"keep.md": "base", "skipped/a.md": "base"})
	base["agents/unknown/config/settings.json"] = state.FileMeta{Hash: "unknown"}
	local := planningSnapshot(t, map[string]string{"keep.md": "local", "skipped/a.md": "local"})
	remote := planningSnapshot(t, map[string]string{"keep.md": "remote", "skipped/a.md": "remote"})
	remote[internalPath] = state.FileMeta{Hash: "record"}
	baseBefore, localBefore, remoteBefore := maps.Clone(base), maps.Clone(local), maps.Clone(remote)
	plan := prepareResourcePlan(
		base, local, remote, remote, map[string]resource.Spec{spec.Key: spec},
		[]resource.Issue{{ResourceKey: spec.Key, Path: "skipped"}}, nil,
	)
	if !maps.Equal(plan.base, state.Snapshot{keep: base[keep]}) ||
		!maps.Equal(plan.local, state.Snapshot{keep: local[keep], internalPath: remote[internalPath]}) ||
		!maps.Equal(plan.remote, state.Snapshot{keep: remote[keep], internalPath: remote[internalPath]}) {
		t.Fatalf("unexpected normalized views: %#v", plan)
	}
	if !slices.Equal(plan.skipPrefixes, []string{planningPath(t, "skipped")}) ||
		!slices.Equal(plan.actions, []Action{{MergeBoth, keep}}) {
		t.Fatalf("unexpected plan: %#v", plan)
	}
	plan.base[keep] = state.FileMeta{Hash: "different"}
	plan.local[keep] = state.FileMeta{Hash: "different"}
	delete(plan.remote, internalPath)
	if !maps.Equal(base, baseBefore) || !maps.Equal(local, localBefore) || !maps.Equal(remote, remoteBefore) {
		t.Fatal("normalized plan aliases its input snapshots")
	}
}

func TestFirstSyncStrategyPreservesProtectionAndInputs(t *testing.T) {
	actions := []Action{
		{PushToRemote, "local"}, {PullToLocal, "remote"}, {MergeBoth, "both"},
		{MergeBoth, "local-only"}, {MergeBoth, "remote-only"},
		{DeleteLocal, "deleted-local"}, {DeleteRemote, "deleted-remote"}, {RemoveRemote, "blocked"},
	}
	local := state.Snapshot{"local": {Hash: "l"}, "both": {Hash: "l"}, "local-only": {Hash: "l"}}
	remote := state.Snapshot{"remote": {Hash: "r"}, "both": {Hash: "r"}, "remote-only": {Hash: "r"}}
	for _, tc := range []struct {
		mode string
		want []ActionType
	}{
		{"use-cloud", []ActionType{DeleteLocal, PullToLocal, PullToLocal, DeleteLocal, PullToLocal, DeleteLocal, DeleteRemote, RemoveRemote}},
		{"use-local", []ActionType{PushToRemote, DeleteRemote, PushToRemote, PushToRemote, DeleteRemote, DeleteLocal, DeleteRemote, RemoveRemote}},
		{"", []ActionType{PushToRemote, PullToLocal, MergeBoth, MergeBoth, MergeBoth, DeleteLocal, DeleteRemote, RemoveRemote}},
		{"unknown", []ActionType{PushToRemote, PullToLocal, MergeBoth, MergeBoth, MergeBoth, DeleteLocal, DeleteRemote, RemoveRemote}},
	} {
		t.Run(tc.mode, func(t *testing.T) {
			before := slices.Clone(actions)
			localBefore, remoteBefore := maps.Clone(local), maps.Clone(remote)
			got := (&Engine{FirstSyncMode: tc.mode}).applyFirstSyncStrategy(actions, local, remote)
			if len(got) != len(actions) {
				t.Fatalf("action count changed: %v", got)
			}
			for index, action := range got {
				if action.Type != tc.want[index] || action.RepoRel != actions[index].RepoRel {
					t.Fatalf("action %d = %v", index, action)
				}
			}
			if !slices.Equal(actions, before) || !maps.Equal(local, localBefore) || !maps.Equal(remote, remoteBefore) {
				t.Fatal("first-sync mapping mutated its inputs")
			}
		})
	}
}

func TestSyncOnceAppliesCloudChoiceOnlyDuringFirstSync(t *testing.T) {
	for _, firstRun := range []bool{false, true} {
		name := "subsequent sync"
		if firstRun {
			name = "first sync"
		}
		t.Run(name, func(t *testing.T) {
			bare := newBareRemote(t)
			repo := filepath.Join(t.TempDir(), "repo")
			local := t.TempDir()
			writeFile(t, filepath.Join(local, "settings.json"), `{"theme":"local"}`)
			engine := engineFor(cloneWorkspace(t, bare, repo), repo, filepath.Join(t.TempDir(), "state.json"), local)
			engine.FirstSyncMode, engine.FirstSyncRun = "use-cloud", firstRun
			result, err := engine.SyncOnce()
			if err != nil {
				t.Fatal(err)
			}
			want := PushToRemote
			if firstRun {
				want = DeleteLocal
			}
			if !slices.Equal(result.Actions, []Action{{want, "agents/demo/config/settings.json"}}) {
				t.Fatalf("actions = %v", result.Actions)
			}
			if firstRun {
				if _, err := os.Stat(filepath.Join(local, "settings.json")); !os.IsNotExist(err) {
					t.Fatalf("cloud choice did not remove the local-only resource: %v", err)
				}
			} else {
				assertFileContent(t, filepath.Join(local, "settings.json"), `{"theme":"local"}`)
				assertFileContent(t, filepath.Join(repo, "agents", "demo", "config", "settings.json"), `{"theme":"local"}`)
			}
		})
	}
}
