package installplan

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/qinqingxu/acsync/internal/resource"
)

func TestManagerPersistsUnapprovedWorkThenExecutesApproval(t *testing.T) {
	home := t.TempDir()
	repo := t.TempDir()
	spec := resource.Spec{
		Key:       "copilot/plugins",
		Provider:  "copilot",
		ID:        "plugins",
		Category:  resource.CategoryPlugins,
		Strategy:  resource.StrategyInstallManifest,
		Layout:    resource.LayoutPortable,
		Installer: "copilot-plugin",
		Root:      filepath.Join(home, ".copilot", "installed-plugins"),
		Targets:   []string{filepath.Join(home, ".copilot", "installed-plugins")},
	}
	desired := []Declaration{{
		ID: "wiqd@wiqd", Adapter: "copilot-plugin",
		Source: "wiqd@wiqd", Version: "0.6.0", Enabled: true,
	}}
	data, err := json.Marshal(desired)
	if err != nil {
		t.Fatal(err)
	}
	repoRel, err := spec.RepoPath("manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	filename := filepath.Join(repo, filepath.FromSlash(repoRel))
	if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filename, data, 0o644); err != nil {
		t.Fatal(err)
	}

	runner := &fakeRunner{}
	manager := NewManager(home, NewBuiltinInventory(runner), runner)
	result, err := manager.Reconcile(
		context.Background(),
		repo,
		map[string]resource.Spec{spec.Key: spec},
		map[string][]byte{spec.Key: []byte("[]")},
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Pending != 1 || result.Executed != 0 || len(runner.calls) != 0 {
		t.Fatalf("unapproved result = %#v, calls = %#v", result, runner.calls)
	}
	pending, err := manager.Store.Pending()
	if err != nil {
		t.Fatal(err)
	}
	if pending == nil || len(pending.Operations) != 1 {
		t.Fatalf("pending plan = %#v", pending)
	}
	if err := manager.Store.Approve(pending.Operations[0]); err != nil {
		t.Fatal(err)
	}

	result, err = manager.Reconcile(
		context.Background(),
		repo,
		map[string]resource.Spec{spec.Key: spec},
		map[string][]byte{spec.Key: []byte("[]")},
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Executed != 1 || result.Pending != 0 || len(runner.calls) != 1 {
		t.Fatalf("approved result = %#v, calls = %#v", result, runner.calls)
	}
}

func TestInventoryRegistryEmitsDeclarationsOnly(t *testing.T) {
	registry := NewBuiltinInventory(&fakeRunner{responses: map[string]RunResult{
		"copilot plugin list": {Stdout: copilotListFixture},
	}})
	files, err := registry.Inventory(resource.Spec{Installer: "copilot-plugin"})
	if err != nil {
		t.Fatal(err)
	}
	data := files["manifest.json"]
	for _, forbidden := range []string{"executable", `"args"`, "workingDir"} {
		if containsString(data, forbidden) {
			t.Fatalf("inventory contains %q: %s", forbidden, data)
		}
	}
}

func TestInventoryRegistryForwardsCancellationToAdapter(t *testing.T) {
	started := make(chan struct{})
	registry := NewInventoryRegistry()
	registry.Register("blocking", blockingInventoryAdapter{discover: func(
		ctx context.Context,
		_ resource.Spec,
	) ([]Declaration, error) {
		close(started)
		<-ctx.Done()
		return nil, ctx.Err()
	}})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := registry.InventoryContext(ctx, resource.Spec{Installer: "blocking"})
		done <- err
	}()

	<-started
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("InventoryContext() error = %v, want context canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("adapter did not receive inventory cancellation")
	}
}

func TestManagerBacksUpPluginPayloadBeforeUninstall(t *testing.T) {
	home := t.TempDir()
	repo := t.TempDir()
	pluginRoot := filepath.Join(home, ".copilot", "installed-plugins")
	payload := filepath.Join(pluginRoot, "wiqd", "plugin.json")
	if err := os.MkdirAll(filepath.Dir(payload), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(payload, []byte(`{"name":"wiqd"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	spec := resource.Spec{
		Key: "copilot/plugins", Provider: "copilot", ID: "plugins",
		Category: resource.CategoryPlugins, Strategy: resource.StrategyInstallManifest,
		Layout: resource.LayoutPortable, Installer: "copilot-plugin",
		Root: pluginRoot, Targets: []string{pluginRoot},
	}
	current, err := json.Marshal([]Declaration{{
		ID: "wiqd@wiqd", Adapter: "copilot-plugin",
		Source: "wiqd@wiqd", Version: "0.6.0", Enabled: true,
	}})
	if err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{}
	manager := NewManager(home, NewBuiltinInventory(runner), runner)
	manager.Now = func() time.Time { return time.Unix(1000, 0) }
	specs := map[string]resource.Spec{spec.Key: spec}
	inventory := map[string][]byte{spec.Key: current}
	if _, err := manager.Reconcile(context.Background(), repo, specs, inventory); err != nil {
		t.Fatal(err)
	}
	pending, err := manager.Store.Pending()
	if err != nil {
		t.Fatal(err)
	}
	if pending == nil || len(pending.Operations) != 1 {
		t.Fatalf("pending uninstall = %#v", pending)
	}
	if err := manager.Store.Approve(pending.Operations[0]); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Reconcile(context.Background(), repo, specs, inventory); err != nil {
		t.Fatal(err)
	}
	backup := filepath.Join(
		home,
		"local-trash",
		time.Unix(1000, 0).UTC().Format("20060102T150405.000000000Z"),
		"plugins",
		"copilot-plugin",
		"wiqd",
		"plugin.json",
	)
	if data, err := os.ReadFile(backup); err != nil || string(data) != `{"name":"wiqd"}` {
		t.Fatalf("plugin backup = %q, %v", data, err)
	}
}

func TestManagerPublishesSkillDependenciesWithoutCommands(t *testing.T) {
	home := t.TempDir()
	repo := t.TempDir()
	canonicalSkills := filepath.Join(
		repo,
		"agents",
		"_portable",
		"config",
		"common",
		"skills",
		"common-skills",
		"review",
	)
	if err := os.MkdirAll(canonicalSkills, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(
		filepath.Join(canonicalSkills, "package-lock.json"),
		[]byte("{}"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	localSkills := filepath.Join(home, ".agents", "skills")
	spec := resource.Spec{
		Key: "common/common-skills", Provider: "common", ID: "skills",
		Category: resource.CategorySkills, Strategy: resource.StrategySourceTree,
		Layout: resource.LayoutPortable, Root: localSkills, Targets: []string{localSkills},
		SharedAs: "common-skills",
	}
	runner := &fakeRunner{}
	manager := NewManager(home, NewBuiltinInventory(runner), runner)
	result, err := manager.Reconcile(
		context.Background(),
		repo,
		map[string]resource.Spec{spec.Key: spec},
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if result.Pending != 1 {
		t.Fatalf("dependency result = %#v", result)
	}
	data, err := os.ReadFile(filepath.Join(
		repo,
		filepath.FromSlash(skillDependencyManifest),
	))
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"executable", `"args"`, "workingDir"} {
		if containsString(data, forbidden) {
			t.Fatalf("Skill manifest contains %q: %s", forbidden, data)
		}
	}
}

func containsString(data []byte, value string) bool {
	for index := 0; index+len(value) <= len(data); index++ {
		if string(data[index:index+len(value)]) == value {
			return true
		}
	}
	return false
}

type blockingInventoryAdapter struct {
	discover func(context.Context, resource.Spec) ([]Declaration, error)
}

func (a blockingInventoryAdapter) Discover(
	ctx context.Context,
	spec resource.Spec,
) ([]Declaration, error) {
	return a.discover(ctx, spec)
}

func (blockingInventoryAdapter) Operations(
	[]Declaration,
	[]Declaration,
) ([]Operation, error) {
	return nil, nil
}
