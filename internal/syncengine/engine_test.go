package syncengine

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/qinqingxu/synchub-for-agents/internal/gitclient"
	"github.com/qinqingxu/synchub-for-agents/internal/portableconfig"
	"github.com/qinqingxu/synchub-for-agents/internal/resource"
	"github.com/qinqingxu/synchub-for-agents/internal/state"
)

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func newBareRemote(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	bare := filepath.Join(root, "remote.git")
	git(t, root, "init", "--bare", "-b", "main", bare)
	seed := filepath.Join(root, "seed")
	git(t, root, "clone", bare, seed)
	git(t, seed, "config", "user.email", "s@e.com")
	git(t, seed, "config", "user.name", "seed")
	if err := os.WriteFile(filepath.Join(seed, "manifest.json"), []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, seed, "add", ".")
	git(t, seed, "commit", "-m", "seed")
	git(t, seed, "push", "origin", "main")
	return bare
}

func cloneWorkspace(t *testing.T, bare, dir string) *gitclient.Client {
	t.Helper()
	client := &gitclient.Client{Dir: dir}
	if err := client.Clone(bare, dir); err != nil {
		t.Fatalf("clone: %v", err)
	}
	git(t, dir, "config", "user.email", "m@e.com")
	git(t, dir, "config", "user.name", "machine")
	git(t, dir, "config", "gc.auto", "0")
	git(t, dir, "config", "maintenance.auto", "false")
	return client
}

func engineFor(client *gitclient.Client, repoDir, statePath, agentRoot string) *Engine {
	return &Engine{
		Git:       client,
		RepoDir:   repoDir,
		Home:      filepath.Dir(statePath),
		UserHome:  filepath.Dir(agentRoot),
		GOOS:      "windows",
		StatePath: statePath,
		Resources: map[string]resource.Spec{
			"demo/legacy-config": {
				Key:      "demo/legacy-config",
				Provider: "demo",
				ID:       "legacy-config",
				Category: resource.CategoryConfig,
				Strategy: resource.StrategyFileTree,
				Layout:   resource.LayoutLegacy,
				Root:     agentRoot,
				Targets:  []string{agentRoot},
				Include:  []string{"settings.json"},
			},
		},
		PushRetries: 3,
		Now:         func() time.Time { return time.Unix(1000, 0) },
	}
}

func writeFile(t *testing.T, filename, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filename, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestSyncOncePropagatesCreateAndDelete(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	bare := newBareRemote(t)
	repoA := filepath.Join(t.TempDir(), "repoA")
	rootA := t.TempDir()
	engA := engineFor(cloneWorkspace(t, bare, repoA), repoA, filepath.Join(t.TempDir(), "stateA.json"), rootA)
	repoB := filepath.Join(t.TempDir(), "repoB")
	rootB := t.TempDir()
	engB := engineFor(cloneWorkspace(t, bare, repoB), repoB, filepath.Join(t.TempDir(), "stateB.json"), rootB)

	writeFile(t, filepath.Join(rootA, "settings.json"), `{"theme":"dark"}`)
	if _, err := engA.SyncOnce(); err != nil {
		t.Fatalf("A first sync: %v", err)
	}
	if _, err := engB.SyncOnce(); err != nil {
		t.Fatalf("B first sync: %v", err)
	}
	if _, err := os.Stat(filepath.Join(rootB, "settings.json")); err != nil {
		t.Fatalf("B should have pulled settings.json locally: %v", err)
	}

	if err := os.Remove(filepath.Join(rootA, "settings.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := engA.SyncOnce(); err != nil {
		t.Fatalf("A delete sync: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repoA, ".trash", "files", "agents", "demo", "config", "settings.json")); err != nil {
		t.Fatalf("deleted file should be in A's trash: %v", err)
	}
	if _, err := engB.SyncOnce(); err != nil {
		t.Fatalf("B delete sync: %v", err)
	}
	if _, err := os.Stat(filepath.Join(rootB, "settings.json")); !os.IsNotExist(err) {
		t.Fatalf("B local settings.json should be deleted, err=%v", err)
	}
}

func TestEnginePublishesProgress(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	bare := newBareRemote(t)
	repo := filepath.Join(t.TempDir(), "repo")
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "settings.json"), `{"theme":"dark"}`)
	engine := engineFor(
		cloneWorkspace(t, bare, repo),
		repo,
		filepath.Join(t.TempDir(), "state.json"),
		root,
	)
	var progress []Progress
	engine.OnProgress = func(update Progress) {
		progress = append(progress, update)
	}

	result, err := engine.SyncOnce()
	if err != nil {
		t.Fatal(err)
	}
	wantStages := []ProgressStage{
		StagePulling,
		StageScanning,
		StageComparing,
		StageApplying,
		StageApplying,
		StageUploading,
	}
	if len(progress) != len(wantStages) {
		t.Fatalf("progress stages = %#v", progress)
	}
	for index, want := range wantStages {
		if progress[index].Stage != want {
			t.Fatalf("progress[%d].Stage = %q, want %q", index, progress[index].Stage, want)
		}
		if index > 0 && progress[index].Percentage < progress[index-1].Percentage {
			t.Fatalf("progress percentage decreased: %#v", progress)
		}
	}
	last := progress[len(progress)-1]
	if last.Percentage != 85 ||
		last.CompletedActions != len(result.Actions) ||
		last.TotalActions != len(result.Actions) ||
		last.BlockedFiles != len(result.Blocked) {
		t.Fatalf("complete progress = %#v, result = %#v", last, result)
	}
}

func TestSyncOncePreservesConcurrentBinaryEditsAsConflict(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	bare := newBareRemote(t)
	repoA := filepath.Join(t.TempDir(), "repoA")
	repoB := filepath.Join(t.TempDir(), "repoB")
	rootA := t.TempDir()
	rootB := t.TempDir()
	engineA := engineFor(cloneWorkspace(t, bare, repoA), repoA, filepath.Join(t.TempDir(), "state.json"), rootA)
	engineB := engineFor(cloneWorkspace(t, bare, repoB), repoB, filepath.Join(t.TempDir(), "state.json"), rootB)

	writeFile(t, filepath.Join(rootA, "settings.json"), `{"value":"base"}`)
	if _, err := engineA.SyncOnce(); err != nil {
		t.Fatal(err)
	}
	if _, err := engineB.SyncOnce(); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(rootA, "settings.json"), `{"value":"machine-a"}`)
	if _, err := engineA.SyncOnce(); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(rootB, "settings.json"), `{"value":"machine-b"}`)

	result, err := engineB.SyncOnce()
	if err != nil {
		t.Fatal(err)
	}
	if result.Conflicts != 1 || !result.NeedsAttention {
		t.Fatalf("result = %#v", result)
	}
	assertFileContent(t, filepath.Join(rootB, "settings.json"), `{"value":"machine-b"}`)
	assertFileContent(t, filepath.Join(repoB, "agents", "demo", "config", "settings.json"), `{"value":"machine-a"}`)
	entries, err := os.ReadDir(filepath.Join(engineB.Home, "conflicts"))
	if err != nil || len(entries) != 1 {
		t.Fatalf("local conflicts = %#v, %v", entries, err)
	}
}

func TestSyncOnceDoesNotAdvanceUnavailableResourceBase(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	bare := newBareRemote(t)
	repoA := filepath.Join(t.TempDir(), "repoA")
	rootA := t.TempDir()
	engineA := engineFor(cloneWorkspace(t, bare, repoA), repoA, filepath.Join(t.TempDir(), "state.json"), rootA)
	writeFile(t, filepath.Join(rootA, "settings.json"), "remote")
	if _, err := engineA.SyncOnce(); err != nil {
		t.Fatal(err)
	}

	repoB := filepath.Join(t.TempDir(), "repoB")
	rootB := filepath.Join(t.TempDir(), "not-created")
	stateB := filepath.Join(t.TempDir(), "state.json")
	engineB := engineFor(cloneWorkspace(t, bare, repoB), repoB, stateB, rootB)
	result, err := engineB.SyncOnce()
	if err != nil {
		t.Fatal(err)
	}
	if !result.NeedsAttention || result.Skipped == 0 {
		t.Fatalf("result = %#v", result)
	}
	snapshot, err := state.Load(stateB)
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot) != 0 {
		t.Fatalf("unavailable resource advanced state: %#v", snapshot)
	}

	if err := os.MkdirAll(rootB, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := engineB.SyncOnce(); err != nil {
		t.Fatal(err)
	}
	assertFileContent(t, filepath.Join(rootB, "settings.json"), "remote")
}

func TestSyncOnceMergesIndependentStructuredEdits(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	bare := newBareRemote(t)
	makeEngine := func(repo, statePath, root string) *Engine {
		engine := engineFor(cloneWorkspace(t, bare, repo), repo, statePath, root)
		spec := engine.Resources["demo/legacy-config"]
		spec.Strategy = resource.StrategyStructuredMerge
		spec.Transformer = "demo-settings"
		engine.Resources[spec.Key] = spec
		registry := portableconfig.NewRegistry()
		registry.Register("demo-settings", portableconfig.Policy{Portable: []string{"theme", "font"}})
		engine.Codecs = registry
		return engine
	}
	repoA := filepath.Join(t.TempDir(), "repoA")
	repoB := filepath.Join(t.TempDir(), "repoB")
	rootA := t.TempDir()
	rootB := t.TempDir()
	engineA := makeEngine(repoA, filepath.Join(t.TempDir(), "state.json"), rootA)
	engineB := makeEngine(repoB, filepath.Join(t.TempDir(), "state.json"), rootB)

	writeFile(t, filepath.Join(rootA, "settings.json"), `{"theme":"dark","font":12}`)
	if _, err := engineA.SyncOnce(); err != nil {
		t.Fatal(err)
	}
	if _, err := engineB.SyncOnce(); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(rootA, "settings.json"), `{"theme":"light","font":12}`)
	if _, err := engineA.SyncOnce(); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(rootB, "settings.json"), `{"theme":"dark","font":14}`)

	result, err := engineB.SyncOnce()
	if err != nil {
		t.Fatal(err)
	}
	if result.Conflicts != 0 {
		t.Fatalf("result = %#v", result)
	}
	data, err := os.ReadFile(filepath.Join(rootB, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	var document map[string]any
	if err := json.Unmarshal(data, &document); err != nil {
		t.Fatal(err)
	}
	if document["theme"] != "light" || document["font"] != float64(14) {
		t.Fatalf("merged document = %#v", document)
	}
}

func TestSyncOnceCombinesLegacySessionsAndPortableInstructions(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	bare := newBareRemote(t)
	makeEngine := func(repo, home, sessionRoot, instructionRoot string) *Engine {
		return &Engine{
			Git:       cloneWorkspace(t, bare, repo),
			RepoDir:   repo,
			Home:      home,
			UserHome:  filepath.Dir(sessionRoot),
			GOOS:      "windows",
			StatePath: filepath.Join(home, "state.json"),
			Resources: map[string]resource.Spec{
				"demo/legacy-sessions": {
					Key:      "demo/legacy-sessions",
					Provider: "demo",
					ID:       "legacy-sessions",
					Category: resource.CategorySessions,
					Strategy: resource.StrategyFileTree,
					Layout:   resource.LayoutLegacy,
					Root:     sessionRoot,
					Targets:  []string{sessionRoot},
					Include:  []string{"*.jsonl"},
				},
				"demo/global": {
					Key:      "demo/global",
					Provider: "demo",
					ID:       "global",
					Category: resource.CategoryInstructions,
					Strategy: resource.StrategyTextTree,
					Layout:   resource.LayoutPortable,
					Root:     instructionRoot,
					Targets:  []string{instructionRoot},
					Include:  []string{"*.md"},
				},
			},
			PushRetries: 3,
		}
	}
	sessionA := t.TempDir()
	sessionB := t.TempDir()
	instructionsA := t.TempDir()
	instructionsB := t.TempDir()
	repoA := filepath.Join(t.TempDir(), "repoA")
	repoB := filepath.Join(t.TempDir(), "repoB")
	engineA := makeEngine(repoA, t.TempDir(), sessionA, instructionsA)
	engineB := makeEngine(repoB, t.TempDir(), sessionB, instructionsB)
	baseText := "first\nlocal-base\nmiddle\nremote-base\nlast\n"
	writeFile(t, filepath.Join(sessionA, "session.jsonl"), `{"message":"hello"}`+"\n")
	writeFile(t, filepath.Join(instructionsA, "AGENTS.md"), baseText)

	if _, err := engineA.SyncOnce(); err != nil {
		t.Fatal(err)
	}
	if _, err := engineB.SyncOnce(); err != nil {
		t.Fatal(err)
	}
	assertFileContent(t, filepath.Join(sessionB, "session.jsonl"), `{"message":"hello"}`+"\n")
	portablePath := filepath.Join(
		repoB,
		"agents",
		"_portable",
		"config",
		"providers",
		"demo",
		"instructions",
		"global",
		"AGENTS.md",
	)
	assertFileContent(t, portablePath, baseText)

	writeFile(t, filepath.Join(instructionsA, "AGENTS.md"), "first\nmachine-a\nmiddle\nremote-base\nlast\n")
	writeFile(t, filepath.Join(instructionsB, "AGENTS.md"), "first\nlocal-base\nmiddle\nmachine-b\nlast\n")
	if _, err := engineA.SyncOnce(); err != nil {
		t.Fatal(err)
	}
	result, err := engineB.SyncOnce()
	if err != nil {
		t.Fatal(err)
	}
	if result.Conflicts != 0 {
		t.Fatalf("result = %#v", result)
	}
	wantMerged := "first\nmachine-a\nmiddle\nmachine-b\nlast\n"
	assertFileContent(t, filepath.Join(instructionsB, "AGENTS.md"), wantMerged)
	if _, err := engineA.SyncOnce(); err != nil {
		t.Fatal(err)
	}
	assertFileContent(t, filepath.Join(instructionsA, "AGENTS.md"), wantMerged)
}

func TestApplyFirstSyncStrategyUseCloud(t *testing.T) {
	engine := &Engine{
		FirstSyncMode: "use-cloud",
		FirstSyncRun:  true,
	}
	actions := []Action{
		{Type: PushToRemote, RepoRel: "a"},
		{Type: MergeBoth, RepoRel: "b"},
		{Type: PullToLocal, RepoRel: "c"},
	}
	local := state.Snapshot{
		"a": {Hash: "a"},
		"b": {Hash: "b"},
	}
	remote := state.Snapshot{
		"b": {Hash: "r"},
		"c": {Hash: "c"},
	}
	got := engine.applyFirstSyncStrategy(actions, local, remote)
	if got[0].Type != DeleteLocal {
		t.Fatalf("action[0] = %v", got[0].Type)
	}
	if got[1].Type != PullToLocal {
		t.Fatalf("action[1] = %v", got[1].Type)
	}
	if got[2].Type != PullToLocal {
		t.Fatalf("action[2] = %v", got[2].Type)
	}
}

func TestApplyFirstSyncStrategyUseLocal(t *testing.T) {
	engine := &Engine{
		FirstSyncMode: "use-local",
		FirstSyncRun:  true,
	}
	actions := []Action{
		{Type: PullToLocal, RepoRel: "a"},
		{Type: MergeBoth, RepoRel: "b"},
		{Type: MergeBoth, RepoRel: "c"},
	}
	local := state.Snapshot{
		"b": {Hash: "l"},
	}
	remote := state.Snapshot{
		"a": {Hash: "r"},
		"c": {Hash: "r"},
	}
	got := engine.applyFirstSyncStrategy(actions, local, remote)
	if got[0].Type != DeleteRemote {
		t.Fatalf("action[0] = %v", got[0].Type)
	}
	if got[1].Type != PushToRemote {
		t.Fatalf("action[1] = %v", got[1].Type)
	}
	if got[2].Type != DeleteRemote {
		t.Fatalf("action[2] = %v", got[2].Type)
	}
}
