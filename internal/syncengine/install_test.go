package syncengine

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/qinqingxu/synchub-for-agents/internal/installplan"
	"github.com/qinqingxu/synchub-for-agents/internal/resource"
	"github.com/qinqingxu/synchub-for-agents/internal/resourcecollect"
)

func TestSyncOnceCreatesPendingInstallWithoutWritingManifestToPluginRoot(t *testing.T) {
	bare := newBareRemote(t)
	publisher := filepath.Join(t.TempDir(), "publisher")
	cloneWorkspace(t, bare, publisher)
	remoteManifest := filepath.Join(
		publisher,
		"agents",
		"_portable",
		"config",
		"providers",
		"copilot",
		"plugins",
		"plugins",
		"manifest.json",
	)
	writeFile(t, remoteManifest, `[{
		"id":"wiqd@wiqd",
		"adapter":"copilot-plugin",
		"source":"wiqd@wiqd",
		"version":"0.6.0",
		"enabled":true
	}]`)
	git(t, publisher, "add", ".")
	git(t, publisher, "commit", "-m", "add plugin declaration")
	git(t, publisher, "push", "origin", "main")

	repo := filepath.Join(t.TempDir(), "repo")
	client := cloneWorkspace(t, bare, repo)
	home := t.TempDir()
	pluginRoot := filepath.Join(home, ".copilot", "installed-plugins")
	runner := &installRunner{}
	inventory := installplan.NewBuiltinInventory(runner)
	manager := installplan.NewManager(home, inventory, runner)
	engine := &Engine{
		Git:       client,
		RepoDir:   repo,
		Home:      home,
		UserHome:  home,
		GOOS:      "windows",
		StatePath: filepath.Join(home, "state.json"),
		Resources: map[string]resource.Spec{
			"copilot/plugins": {
				Key:       "copilot/plugins",
				Provider:  "copilot",
				ID:        "plugins",
				Category:  resource.CategoryPlugins,
				Strategy:  resource.StrategyInstallManifest,
				Layout:    resource.LayoutPortable,
				Root:      pluginRoot,
				Targets:   []string{pluginRoot},
				Installer: "copilot-plugin",
			},
		},
		Inventory:      inventory,
		InstallManager: manager,
		Now:            func() time.Time { return time.Unix(1000, 0) },
	}

	result, err := engine.SyncOnce()
	if err != nil {
		t.Fatal(err)
	}
	if result.PendingInstalls != 1 || !result.NeedsAttention {
		t.Fatalf("result = %#v", result)
	}
	if _, err := os.Stat(filepath.Join(pluginRoot, "manifest.json")); !os.IsNotExist(err) {
		t.Fatalf("repository manifest was restored into plugin root: %v", err)
	}
	pending, err := manager.Store.Pending()
	if err != nil {
		t.Fatal(err)
	}
	if pending == nil || len(pending.Operations) != 1 {
		t.Fatalf("pending plan = %#v", pending)
	}
	if len(runner.installCalls) != 0 {
		t.Fatalf("unapproved install ran: %#v", runner.installCalls)
	}
	if err := manager.Store.Approve(pending.Operations[0]); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(pluginRoot, 0o755); err != nil {
		t.Fatal(err)
	}

	result, err = engine.SyncOnce()
	if err != nil {
		t.Fatal(err)
	}
	if result.Reinstalled != 1 || result.PendingInstalls != 0 {
		t.Fatalf("approved result = %#v", result)
	}
	if len(runner.installCalls) != 1 {
		t.Fatalf("approved install calls = %#v", runner.installCalls)
	}
	if _, err := os.Stat(remoteManifestPath(repo)); err != nil {
		t.Fatalf("desired plugin declaration was removed: %v", err)
	}
}

type installRunner struct {
	installCalls []string
}

func (r *installRunner) Run(
	_ context.Context,
	executable string,
	args []string,
	_ string,
) (installplan.RunResult, error) {
	if executable == "copilot" &&
		len(args) == 2 &&
		args[0] == "plugin" &&
		args[1] == "list" {
		return installplan.RunResult{Stdout: []byte("Installed plugins:\n")}, nil
	}
	r.installCalls = append(r.installCalls, executable+" "+joinInstallArgs(args))
	return installplan.RunResult{}, nil
}

func joinInstallArgs(args []string) string {
	result := ""
	for index, arg := range args {
		if index > 0 {
			result += " "
		}
		result += arg
	}
	return result
}

func remoteManifestPath(repo string) string {
	return filepath.Join(
		repo,
		"agents",
		"_portable",
		"config",
		"providers",
		"copilot",
		"plugins",
		"plugins",
		"manifest.json",
	)
}

func TestMergeInstallManifestDoesNotRestoreIntoPluginRoot(t *testing.T) {
	repo := t.TempDir()
	pluginRoot := t.TempDir()
	spec := resource.Spec{
		Key: "copilot/plugins", Provider: "copilot", ID: "plugins",
		Category: resource.CategoryPlugins, Strategy: resource.StrategyInstallManifest,
		Layout: resource.LayoutPortable, Installer: "copilot-plugin",
		Root: pluginRoot, Targets: []string{pluginRoot},
	}
	repoRel, err := spec.RepoPath("manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	data := []byte(`[{"id":"wiqd@wiqd","adapter":"copilot-plugin","source":"wiqd@wiqd","enabled":true}]`)
	writeFile(t, filepath.Join(repo, filepath.FromSlash(repoRel)), string(data))
	stage := filepath.Join(t.TempDir(), "manifest.json")
	writeFile(t, stage, string(data))
	engine := &Engine{RepoDir: repo}
	applier := &ResourceApplier{RepoDir: repo}

	conflicted, err := engine.mergeResource(
		repoRel,
		spec,
		data,
		true,
		map[string]resourcecollect.Artifact{
			repoRel: {RepoRel: repoRel, Relative: "manifest.json", StagePath: stage},
		},
		nil,
		nil,
		applier,
		time.Unix(1000, 0),
	)
	if err != nil {
		t.Fatal(err)
	}
	if conflicted {
		t.Fatal("identical install declarations conflicted")
	}
	if _, err := os.Stat(filepath.Join(pluginRoot, "manifest.json")); !os.IsNotExist(err) {
		t.Fatalf("install manifest was restored into plugin root: %v", err)
	}
}
