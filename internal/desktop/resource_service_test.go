package desktop

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/qinqingxu/acsync/internal/cli"
	"github.com/qinqingxu/acsync/internal/config"
	"github.com/qinqingxu/acsync/internal/conflict"
	"github.com/qinqingxu/acsync/internal/installplan"
	"github.com/qinqingxu/acsync/internal/resource"
)

func configuredResourceService(t *testing.T) *Service {
	t.Helper()
	home := configuredHome(t)
	plans := installplan.NewStore(filepath.Join(home, "install"))
	if err := plans.SavePending(installplan.Plan{
		ID: "plan-1",
		Operations: []installplan.Operation{{
			ID: "copilot:wiqd@wiqd", Adapter: "copilot-plugin",
			Source: "wiqd@wiqd", Kind: "install", Executable: "copilot",
			Args: []string{"plugin", "install", "wiqd@wiqd"},
		}},
	}); err != nil {
		t.Fatal(err)
	}
	conflicts := conflict.NewStore(
		filepath.Join(home, "conflicts"),
		filepath.Join(home, "repo"),
		nil,
	)
	if err := conflicts.Create(conflict.Record{
		ID: "conflict-1", ResourceKey: "claude/settings",
		RepoRel:   "agents/_portable/config/providers/claude/config/settings/settings.json",
		CreatedAt: time.Unix(1000, 0),
	}, []byte(`{"theme":"dark"}`), []byte(`{"theme":"light"}`), []byte(`{"theme":"system"}`)); err != nil {
		t.Fatal(err)
	}
	service, err := New(home, runtime.GOOS)
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func findAgent(agents []Agent, name string) Agent {
	for _, agent := range agents {
		if agent.Name == name {
			return agent
		}
	}
	return Agent{}
}

func TestSnapshotIncludesResourceCategoriesAndPendingWork(t *testing.T) {
	service := configuredResourceService(t)
	defer service.Close()

	got, err := service.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	claude := findAgent(got.Agents, "claude")
	if len(claude.Resources) == 0 {
		t.Fatal("Claude resources missing")
	}
	if got.PendingInstallPlan == nil || len(got.Conflicts) != 1 {
		t.Fatalf("pending state = %#v", got)
	}
	if got.Platform != runtime.GOOS {
		t.Fatalf("platform = %q", got.Platform)
	}
}

func TestSaveSettingsPersistsCategoriesAndCustomResources(t *testing.T) {
	home := configuredHome(t)
	service, err := New(home, runtime.GOOS)
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	input := SettingsInput{
		RepositoryURL:   "git@github.com:owner/repo.git",
		IntervalMinutes: 15,
		TrashGraceDays:  45,
		Agents:          map[string]bool{"claude": true},
		Categories: map[string]map[string]bool{
			"claude": {"skills": false},
		},
		CustomResources: []CustomResourceInput{{
			ID: "notes", Category: "instructions",
			Paths:   map[string]string{runtime.GOOS: filepath.Join(t.TempDir(), "notes")},
			Targets: map[string]string{runtime.GOOS: filepath.Join(t.TempDir(), "restored")},
			Include: []string{"**/*.md"}, Strategy: "text-tree",
		}},
	}
	if err := service.SaveSettings(input); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(cli.ConfigPath(home))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.CategoryEnabled("claude", resource.CategorySkills) {
		t.Fatal("skills category was not disabled")
	}
	if len(cfg.CustomResources) != 1 || cfg.CustomResources[0].ID != "notes" {
		t.Fatalf("custom resources = %#v", cfg.CustomResources)
	}
}

func TestApproveInstallPlanRequiresCurrentID(t *testing.T) {
	service := configuredResourceService(t)
	defer service.Close()
	if err := service.ApproveInstallPlan("other"); err == nil {
		t.Fatal("wrong pending plan ID was accepted")
	}
	if err := service.ApproveInstallPlan("plan-1"); err != nil {
		t.Fatal(err)
	}
	store := installplan.NewStore(filepath.Join(service.home, "install"))
	pending, err := store.Pending()
	if err != nil {
		t.Fatal(err)
	}
	if pending == nil || !store.IsApproved(pending.Operations[0]) {
		t.Fatalf("approval was not saved: %#v", pending)
	}
}

func TestResolveConflictValidatesChoiceAndID(t *testing.T) {
	service := configuredResourceService(t)
	defer service.Close()
	if err := service.ResolveConflict(ConflictResolution{
		ID: "missing", Choice: "local",
	}); err == nil {
		t.Fatal("missing conflict was accepted")
	}
	if err := service.ResolveConflict(ConflictResolution{
		ID: "conflict-1", Choice: "invalid",
	}); err == nil {
		t.Fatal("invalid conflict choice was accepted")
	}
	if err := service.ResolveConflict(ConflictResolution{
		ID: "conflict-1", Choice: "merged", Content: `{"theme":"merged"}`,
	}); err != nil {
		t.Fatal(err)
	}
	canonical := filepath.Join(
		service.home,
		"repo",
		"agents",
		"_portable",
		"config",
		"providers",
		"claude",
		"config",
		"settings",
		"settings.json",
	)
	if data, err := os.ReadFile(canonical); err != nil || string(data) != `{"theme":"merged"}` {
		t.Fatalf("resolved content = %q, %v", data, err)
	}
}

func TestPreviewCustomResourceDoesNotSaveCandidate(t *testing.T) {
	home := configuredHome(t)
	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "README.md"), []byte("portable"), 0o644); err != nil {
		t.Fatal(err)
	}

	service, err := New(home, runtime.GOOS)
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	preview, err := service.PreviewCustomResource(CustomResourceInput{
		ID: "notes", Category: "instructions",
		Paths:   map[string]string{runtime.GOOS: source},
		Targets: map[string]string{runtime.GOOS: t.TempDir()},
		Include: []string{"**"}, Strategy: "text-tree",
	})
	if err != nil {
		t.Fatal(err)
	}
	if preview.Files != 1 || preview.Bytes == 0 {
		t.Fatalf("preview = %#v", preview)
	}
	cfg, err := config.Load(cli.ConfigPath(home))
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.CustomResources) != 0 {
		t.Fatalf("preview saved candidate: %#v", cfg.CustomResources)
	}
}

func TestPreviewKeepsCountsForMultipleResourcesAndProviders(t *testing.T) {
	preview := ResourcePreview{Resources: []ResourceCategory{
		{Provider: "claude", ID: "settings", Category: "config", FileCount: 1},
		{Provider: "copilot", ID: "settings", Category: "config", FileCount: 2},
	}}
	indexed := previewByIdentity(preview)
	if got := indexed[resourceIdentity("claude", "settings", "config")].FileCount; got != 1 {
		t.Fatalf("Claude count = %d", got)
	}
	if got := indexed[resourceIdentity("copilot", "settings", "config")].FileCount; got != 2 {
		t.Fatalf("Copilot count = %d", got)
	}

	home := configuredHome(t)
	first := t.TempDir()
	second := t.TempDir()
	if err := os.WriteFile(filepath.Join(first, "first.md"), []byte("first"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(second, "second.md"), []byte("second"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(cli.ConfigPath(home))
	if err != nil {
		t.Fatal(err)
	}
	cfg.CustomResources = []config.CustomResource{
		{
			ID: "first", Category: resource.CategoryInstructions,
			Paths:   map[string]string{runtime.GOOS: first},
			Targets: map[string]string{runtime.GOOS: first},
			Include: []string{"**"}, Strategy: resource.StrategyTextTree,
		},
		{
			ID: "second", Category: resource.CategoryInstructions,
			Paths:   map[string]string{runtime.GOOS: second},
			Targets: map[string]string{runtime.GOOS: second},
			Include: []string{"**"}, Strategy: resource.StrategyTextTree,
		},
	}
	if err := config.Save(cli.ConfigPath(home), cfg); err != nil {
		t.Fatal(err)
	}
	service, err := New(home, runtime.GOOS)
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	got, err := service.ResourcePreview()
	if err != nil {
		t.Fatal(err)
	}
	indexed = previewByIdentity(got)
	for _, id := range []string{"first", "second"} {
		item := indexed[resourceIdentity("custom", id, "instructions")]
		if item.FileCount != 1 || item.Bytes == 0 {
			t.Fatalf("%s preview = %#v", id, item)
		}
	}
}
