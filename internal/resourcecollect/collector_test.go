package resourcecollect

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"

	"github.com/qinqingxu/synchub-for-agents/internal/resource"
)

func writeCollectorFile(t *testing.T, filename string, data []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filename, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCollectorStagesExactScannedBytes(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "SKILL.md")
	writeCollectorFile(t, source, []byte("safe"))
	stageParent := t.TempDir()
	spec := resource.Spec{
		Key:      "common/common-skills",
		Provider: "common",
		ID:       "skills",
		Category: resource.CategorySkills,
		Strategy: resource.StrategySourceTree,
		Layout:   resource.LayoutPortable,
		Root:     root,
		Targets:  []string{root},
		Include:  []string{"**"},
		SharedAs: "common-skills",
	}

	result, err := New(Options{
		StageParent: stageParent,
		GOOS:        "linux",
		UserHome:    "/home/alice",
	}).Collect([]resource.Spec{spec})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if result.StageRoot != "" {
			_ = result.Close()
		}
	})

	if err := os.WriteFile(source, []byte(`{"accessToken":"changed"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	repoRel := "agents/_portable/config/common/skills/common-skills/SKILL.md"
	artifact, ok := result.Artifacts[repoRel]
	if !ok {
		t.Fatalf("artifact %q missing: %#v", repoRel, result.Artifacts)
	}
	got, err := os.ReadFile(artifact.StagePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "safe" {
		t.Fatalf("staged bytes = %q", got)
	}
	if artifact.Hash != result.Snapshot[repoRel].Hash || artifact.Size != 4 {
		t.Fatalf("artifact metadata = %#v, snapshot = %#v", artifact, result.Snapshot[repoRel])
	}

	stageRoot := result.StageRoot
	if err := result.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stageRoot); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stage root still exists: %v", err)
	}
	if _, err := os.Stat(stageParent); err != nil {
		t.Fatalf("stage parent was removed: %v", err)
	}
	result.StageRoot = ""
}

func TestCloseAfterErrorPreservesCollectionAndCleanupFailures(t *testing.T) {
	result := Result{StageRoot: "\x00"}
	err := closeAfterError(&result, errors.New("stage failed"))
	if err == nil ||
		!strings.Contains(err.Error(), "stage failed") ||
		!strings.Contains(err.Error(), "remove resource stage") {
		t.Fatalf("closeAfterError() = %v", err)
	}
}

func TestCollectorCoalescesRootLinksAndPreservesTargets(t *testing.T) {
	target := t.TempDir()
	writeCollectorFile(t, filepath.Join(target, "brainstorming", "SKILL.md"), []byte("content"))
	linkParent := t.TempDir()
	linkA := filepath.Join(linkParent, "claude-skills")
	linkB := filepath.Join(linkParent, "common-skills")
	createRootLink(t, target, linkA)
	createRootLink(t, target, linkB)

	first, err := ResolveRoot(linkA)
	if err != nil {
		t.Fatal(err)
	}
	second, err := ResolveRoot(linkB)
	if err != nil {
		t.Fatal(err)
	}
	if first.CanonicalRoot != second.CanonicalRoot || first.Identity != second.Identity {
		t.Fatalf("root identities differ: %#v %#v", first, second)
	}

	base := resource.Spec{
		ID:       "skills",
		Category: resource.CategorySkills,
		Strategy: resource.StrategySourceTree,
		Layout:   resource.LayoutPortable,
		Include:  []string{"**"},
		SharedAs: "common-skills",
	}
	one := base
	one.Key = "claude/skills"
	one.Provider = "claude"
	one.Root = linkA
	one.Targets = []string{linkA}
	two := base
	two.Key = "common/skills"
	two.Provider = "common"
	two.Root = linkB
	two.Targets = []string{linkB}

	result, err := New(Options{StageParent: t.TempDir()}).Collect([]resource.Spec{one, two})
	if err != nil {
		t.Fatal(err)
	}
	defer result.Close()

	if len(result.Artifacts) != 1 {
		t.Fatalf("artifacts = %#v", result.Artifacts)
	}
	for _, artifact := range result.Artifacts {
		if artifact.ResourceKey != "common/common-skills" {
			t.Fatalf("resource key = %q", artifact.ResourceKey)
		}
		if len(artifact.Targets) != 2 ||
			!containsPath(artifact.Targets, linkA) ||
			!containsPath(artifact.Targets, linkB) {
			t.Fatalf("targets = %#v", artifact.Targets)
		}
	}
}

func TestCollectorFiltersGeneratedDeclaredAndOversizedFiles(t *testing.T) {
	root := t.TempDir()
	writeCollectorFile(t, filepath.Join(root, "SKILL.md"), []byte("safe"))
	writeCollectorFile(t, filepath.Join(root, "node_modules", "pkg", "index.js"), []byte("generated"))
	writeCollectorFile(t, filepath.Join(root, "generated", "output.txt"), []byte("declared"))
	writeCollectorFile(t, filepath.Join(root, "bin", "tool.exe"), []byte("binary"))
	oversized := filepath.Join(root, "assets", "large.bin")
	writeCollectorFile(t, oversized, nil)
	if err := os.Truncate(oversized, resource.DefaultMaxFileSize); err != nil {
		t.Fatal(err)
	}

	spec := sourceSpec(root)
	spec.Exclude = []string{"generated/**"}
	result, err := New(Options{StageParent: t.TempDir()}).Collect([]resource.Spec{spec})
	if err != nil {
		t.Fatal(err)
	}
	defer result.Close()

	if len(result.Artifacts) != 1 {
		t.Fatalf("artifacts = %#v", result.Artifacts)
	}
	wantCodes := map[string]bool{
		"generated-content": false,
		"platform-binary":   false,
		"file-too-large":    false,
	}
	for _, item := range result.Skipped {
		if _, ok := wantCodes[item.Code]; ok {
			wantCodes[item.Code] = true
		}
	}
	for code, found := range wantCodes {
		if !found {
			t.Errorf("missing skipped issue %q: %#v", code, result.Skipped)
		}
	}
	if !hasIssue(result.Skipped, "node_modules", "generated-content") {
		t.Fatalf("generated directory was not pruned: %#v", result.Skipped)
	}
}

func TestCollectorProjectsThenScansAndContinuesAfterFailures(t *testing.T) {
	root := t.TempDir()
	writeCollectorFile(t, filepath.Join(root, "safe.json"), []byte(`{"theme":"dark","local":"remove"}`))
	writeCollectorFile(t, filepath.Join(root, "bad.json"), []byte(`{"theme":"bad"}`))
	writeCollectorFile(t, filepath.Join(root, "secret.json"), []byte(`{"accessToken":"secret"}`))
	spec := resource.Spec{
		Key:         "claude/settings",
		Provider:    "claude",
		ID:          "settings",
		Category:    resource.CategoryConfig,
		Strategy:    resource.StrategyStructuredMerge,
		Layout:      resource.LayoutPortable,
		Root:        root,
		Targets:     []string{root},
		Include:     []string{"*.json"},
		Transformer: "claude-settings",
		KeyPatterns: []string{"token"},
	}
	projector := fakeProjector{project: func(_ string, rel string, _ string, _ string, data []byte) ([]byte, error) {
		if rel == "bad.json" {
			return nil, fmt.Errorf("malformed document")
		}
		return []byte(strings.ReplaceAll(string(data), `,"local":"remove"`, "")), nil
	}}

	result, err := New(Options{
		StageParent: t.TempDir(),
		Projector:   projector,
	}).Collect([]resource.Spec{spec})
	if err != nil {
		t.Fatal(err)
	}
	defer result.Close()

	safeRel := "agents/_portable/config/providers/claude/config/settings/safe.json"
	safe, ok := result.Artifacts[safeRel]
	if !ok {
		t.Fatalf("safe projected artifact missing: %#v", result.Artifacts)
	}
	data, err := os.ReadFile(safe.StagePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != `{"theme":"dark"}` {
		t.Fatalf("projected bytes = %s", data)
	}
	if !hasIssue(result.Blocked, "bad.json", "projection-failed") {
		t.Fatalf("projection issue missing: %#v", result.Blocked)
	}
	if !hasIssue(result.Blocked, "secret.json", "secret-detected") {
		t.Fatalf("secret issue missing: %#v", result.Blocked)
	}
}

func TestCollectorRechecksProjectedArtifactSize(t *testing.T) {
	root := t.TempDir()
	writeCollectorFile(t, filepath.Join(root, "settings.json"), []byte(`{}`))
	spec := resource.Spec{
		Key:         "claude/settings",
		Provider:    "claude",
		ID:          "settings",
		Category:    resource.CategoryConfig,
		Strategy:    resource.StrategyStructuredMerge,
		Layout:      resource.LayoutPortable,
		Root:        root,
		Targets:     []string{root},
		Include:     []string{"*.json"},
		Transformer: "claude-settings",
	}
	policy := resource.DefaultFilterPolicy()
	policy.MaxFileSize = 8

	result, err := New(Options{
		StageParent: t.TempDir(),
		Projector: fakeProjector{project: func(_ string, _ string, _ string, _ string, _ []byte) ([]byte, error) {
			return []byte("12345678"), nil
		}},
		Filter: policy,
	}).Collect([]resource.Spec{spec})
	if err != nil {
		t.Fatal(err)
	}
	defer result.Close()

	if len(result.Artifacts) != 0 {
		t.Fatalf("oversized projected artifact was staged: %#v", result.Artifacts)
	}
	if !hasIssue(result.Skipped, "settings.json", "file-too-large") {
		t.Fatalf("projected size issue missing: %#v", result.Skipped)
	}
}

func TestCollectorUsesInstallInventoryAndScansReturnedBytes(t *testing.T) {
	spec := resource.Spec{
		Key:         "claude/plugins",
		Provider:    "claude",
		ID:          "plugins",
		Category:    resource.CategoryPlugins,
		Strategy:    resource.StrategyInstallManifest,
		Layout:      resource.LayoutPortable,
		Root:        t.TempDir(),
		Targets:     []string{t.TempDir()},
		Include:     []string{"**"},
		Installer:   "claude-plugin",
		KeyPatterns: []string{"token"},
	}

	missing, err := New(Options{StageParent: t.TempDir()}).Collect([]resource.Spec{spec})
	if err != nil {
		t.Fatal(err)
	}
	defer missing.Close()
	if !hasIssue(missing.Skipped, "", "installer-unavailable") {
		t.Fatalf("missing inventory issue = %#v", missing.Skipped)
	}

	withInventory, err := New(Options{
		StageParent: t.TempDir(),
		Inventory: fakeInventory{files: map[string][]byte{
			"plugins.json": []byte(`{"plugins":["safe"]}`),
			"secret.json":  []byte(`{"accessToken":"secret"}`),
		}},
	}).Collect([]resource.Spec{spec})
	if err != nil {
		t.Fatal(err)
	}
	defer withInventory.Close()
	if len(withInventory.Artifacts) != 1 {
		t.Fatalf("inventory artifacts = %#v", withInventory.Artifacts)
	}
	if !hasIssue(withInventory.Blocked, "secret.json", "secret-detected") {
		t.Fatalf("inventory secret issue = %#v", withInventory.Blocked)
	}
}

func TestCollectorCancellationStopsInventoryAndCleansStage(t *testing.T) {
	spec := resource.Spec{
		Key:       "claude/plugins",
		Provider:  "claude",
		ID:        "plugins",
		Category:  resource.CategoryPlugins,
		Strategy:  resource.StrategyInstallManifest,
		Layout:    resource.LayoutPortable,
		Root:      t.TempDir(),
		Targets:   []string{t.TempDir()},
		Include:   []string{"**"},
		Installer: "claude-plugin",
	}
	started := make(chan struct{})
	stageParent := t.TempDir()
	collector := New(Options{
		StageParent: stageParent,
		Inventory: fakeInventory{inventoryContext: func(
			ctx context.Context,
			_ resource.Spec,
		) (map[string][]byte, error) {
			close(started)
			<-ctx.Done()
			return nil, ctx.Err()
		}},
	})
	ctx, cancel := context.WithCancel(context.Background())
	type collectionResult struct {
		result Result
		err    error
	}
	done := make(chan collectionResult, 1)
	go func() {
		result, err := collector.CollectContext(ctx, []resource.Spec{spec})
		done <- collectionResult{result: result, err: err}
	}()

	<-started
	cancel()
	collected := <-done
	if !errors.Is(collected.err, context.Canceled) {
		t.Fatalf("CollectContext() error = %v, want context canceled", collected.err)
	}
	if collected.result.StageRoot != "" || len(collected.result.Artifacts) != 0 {
		t.Fatalf("canceled collection returned partial result: %#v", collected.result)
	}
	entries, err := os.ReadDir(stageParent)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("canceled collection left stage entries: %#v", entries)
	}
}

func sourceSpec(root string) resource.Spec {
	return resource.Spec{
		Key:      "common/common-skills",
		Provider: "common",
		ID:       "skills",
		Category: resource.CategorySkills,
		Strategy: resource.StrategySourceTree,
		Layout:   resource.LayoutPortable,
		Root:     root,
		Targets:  []string{root},
		Include:  []string{"**"},
		SharedAs: "common-skills",
	}
}

type fakeProjector struct {
	project func(transformer, rel, goos, home string, data []byte) ([]byte, error)
}

func (f fakeProjector) Project(transformer, rel, goos, home string, data []byte) ([]byte, error) {
	return f.project(transformer, rel, goos, home, data)
}

type fakeInventory struct {
	files            map[string][]byte
	err              error
	inventoryContext func(context.Context, resource.Spec) (map[string][]byte, error)
}

func (f fakeInventory) Inventory(resource.Spec) (map[string][]byte, error) {
	return f.files, f.err
}

func (f fakeInventory) InventoryContext(
	ctx context.Context,
	spec resource.Spec,
) (map[string][]byte, error) {
	if f.inventoryContext != nil {
		return f.inventoryContext(ctx, spec)
	}
	return f.files, f.err
}

func hasIssue(issues []resource.Issue, path, code string) bool {
	for _, item := range issues {
		if item.Path == path && item.Code == code {
			return true
		}
	}
	return false
}

func createRootLink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		if runtime.GOOS == "windows" &&
			(os.IsPermission(err) || errors.Is(err, syscall.Errno(1314))) {
			t.Skipf("symbolic links require Windows privilege: %v", err)
		}
		t.Fatal(err)
	}
}

func containsPath(paths []string, want string) bool {
	for _, item := range paths {
		if item == want {
			return true
		}
	}
	return false
}
