package syncengine

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/qinqingxu/synchub-for-agents/internal/portableconfig"
	"github.com/qinqingxu/synchub-for-agents/internal/resource"
	"github.com/qinqingxu/synchub-for-agents/internal/resourcecollect"
	"github.com/qinqingxu/synchub-for-agents/internal/state"
)

func TestResourceApplierPushesImmutableStagedArtifact(t *testing.T) {
	repo := t.TempDir()
	stage := filepath.Join(t.TempDir(), "staged")
	if err := os.WriteFile(stage, []byte("staged"), 0o600); err != nil {
		t.Fatal(err)
	}
	live := filepath.Join(t.TempDir(), "live")
	if err := os.WriteFile(live, []byte("changed after collection"), 0o600); err != nil {
		t.Fatal(err)
	}
	repoRel := "agents/demo/config/settings.json"
	applier := &ResourceApplier{RepoDir: repo}

	if err := applier.PushArtifact(resourcecollect.Artifact{
		RepoRel:   repoRel,
		StagePath: stage,
		Hash:      hashBytes([]byte("staged")),
	}); err != nil {
		t.Fatal(err)
	}

	assertFileContent(t, filepath.Join(repo, filepath.FromSlash(repoRel)), "staged")
}

func TestResourceApplierRestoresStructuredResourceAndPreservesLocalToken(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()
	target := t.TempDir()
	spec := resource.Spec{
		Key:         "demo/settings",
		Provider:    "demo",
		ID:          "settings",
		Category:    resource.CategoryConfig,
		Strategy:    resource.StrategyStructuredMerge,
		Layout:      resource.LayoutPortable,
		Transformer: "demo-settings",
		Targets:     []string{target},
	}
	repoRel, err := spec.RepoPath("settings.json")
	if err != nil {
		t.Fatal(err)
	}
	writeRepoFile(t, repo, repoRel, []byte(`{"theme":"light"}`))
	writeRepoFile(t, target, "settings.json", []byte(`{"theme":"dark","token":"local-only"}`))
	base := state.NewBaseStore(home)
	if err := base.Put(repoRel, []byte(`{"theme":"dark"}`)); err != nil {
		t.Fatal(err)
	}
	registry := portableconfig.NewRegistry()
	registry.Register("demo-settings", portableconfig.Policy{
		Portable:  []string{"theme"},
		Sensitive: []string{"token"},
	})
	applier := &ResourceApplier{
		RepoDir: repo,
		Home:    home,
		GOOS:    "windows",
		Codecs:  registry,
		Base:    base,
	}

	if err := applier.Restore(spec, repoRel); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(target, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got["theme"] != "light" || got["token"] != "local-only" {
		t.Fatalf("restored document = %#v", got)
	}
}

func TestResourceApplierMovesSkillDeletionToRecovery(t *testing.T) {
	home := t.TempDir()
	target := t.TempDir()
	writeRepoFile(t, target, "my-skill/SKILL.md", []byte("instructions"))
	spec := resource.Spec{
		Key:      "demo/skills",
		Provider: "demo",
		ID:       "skills",
		Category: resource.CategorySkills,
		Layout:   resource.LayoutPortable,
		Targets:  []string{target},
	}
	repoRel, err := spec.RepoPath("my-skill/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	applier := &ResourceApplier{
		Home: home,
		Now:  func() time.Time { return time.Date(2026, 8, 14, 12, 0, 0, 0, time.UTC) },
	}

	if err := applier.Delete(spec, repoRel); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(target, "my-skill", "SKILL.md")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("active skill should be removed, stat error = %v", err)
	}
	recovered := filepath.Join(home, "local-trash", "20260814T120000Z", "demo", "skills", "my-skill", "SKILL.md")
	assertFileContent(t, recovered, "instructions")
}

func TestResourceApplierCreatesSharedAliasAndFallsBackToManagedCopy(t *testing.T) {
	for _, tc := range []struct {
		name       string
		symlink    func(string, string) error
		wantMode   string
		wantIsLink bool
	}{
		{name: "symlink", wantMode: "symlink", wantIsLink: true},
		{
			name:     "managed copy",
			symlink:  func(string, string) error { return errors.New("privilege not held") },
			wantMode: "managed-copy",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			repo := t.TempDir()
			home := t.TempDir()
			canonical := filepath.Join(t.TempDir(), "canonical")
			alias := filepath.Join(t.TempDir(), "alias")
			spec := resource.Spec{
				Key:      "common/shared-skills",
				Provider: "common",
				ID:       "shared-skills",
				SharedAs: "shared-skills",
				Category: resource.CategorySkills,
				Strategy: resource.StrategySourceTree,
				Layout:   resource.LayoutPortable,
				Targets:  []string{canonical, alias},
			}
			repoRel, err := spec.RepoPath("review/SKILL.md")
			if err != nil {
				t.Fatal(err)
			}
			writeRepoFile(t, repo, repoRel, []byte("shared"))
			applier := &ResourceApplier{
				RepoDir: repo,
				Home:    home,
				Symlink: tc.symlink,
			}

			if err := applier.Restore(spec, repoRel); err != nil {
				t.Fatal(err)
			}

			assertFileContent(t, filepath.Join(canonical, "review", "SKILL.md"), "shared")
			assertFileContent(t, filepath.Join(alias, "review", "SKILL.md"), "shared")
			info, err := os.Lstat(alias)
			if err != nil {
				t.Fatal(err)
			}
			if (info.Mode()&os.ModeSymlink != 0) != tc.wantIsLink {
				t.Fatalf("alias mode = %v, want symlink=%v", info.Mode(), tc.wantIsLink)
			}
			data, err := os.ReadFile(filepath.Join(home, "aliases.json"))
			if err != nil {
				t.Fatal(err)
			}
			var aliases map[string]aliasRecord
			if err := json.Unmarshal(data, &aliases); err != nil {
				t.Fatal(err)
			}
			if aliases[filepath.Clean(alias)].Mode != tc.wantMode {
				t.Fatalf("alias record = %#v", aliases)
			}
		})
	}
}

func TestResourceApplierRejectsTargetTraversal(t *testing.T) {
	spec := resource.Spec{
		Key:      "demo/config",
		Provider: "demo",
		ID:       "config",
		Category: resource.CategoryConfig,
		Layout:   resource.LayoutPortable,
		Targets:  []string{t.TempDir()},
	}
	applier := &ResourceApplier{RepoDir: t.TempDir()}
	if err := applier.RestoreBytes(spec, "../escape", []byte("bad")); err == nil {
		t.Fatal("expected target traversal to be rejected")
	}
}

func TestResourceApplierSharedDeleteDoesNotCreateMissingTargets(t *testing.T) {
	parent := t.TempDir()
	primary := filepath.Join(parent, "primary")
	alias := filepath.Join(parent, "alias")
	spec := resource.Spec{
		Key:      "common/shared-skills",
		Provider: "common",
		ID:       "shared-skills",
		SharedAs: "shared-skills",
		Category: resource.CategorySkills,
		Layout:   resource.LayoutPortable,
		Targets:  []string{primary, alias},
	}
	repoRel, err := spec.RepoPath("missing/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}

	if err := (&ResourceApplier{Home: t.TempDir()}).Delete(spec, repoRel); err != nil {
		t.Fatal(err)
	}
	for _, target := range []string{primary, alias} {
		if _, err := os.Lstat(target); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("delete created missing target %q: %v", target, err)
		}
	}
}

func TestResourceApplierRejectsNestedLinkEscape(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	link := filepath.Join(root, "linked")
	if err := os.Symlink(outside, link); err != nil {
		t.Skipf("symbolic links are unavailable: %v", err)
	}
	spec := resource.Spec{
		Key:      "demo/config",
		Provider: "demo",
		ID:       "config",
		Category: resource.CategoryConfig,
		Layout:   resource.LayoutPortable,
		Targets:  []string{root},
	}

	err := (&ResourceApplier{}).RestoreBytes(spec, "linked/settings.json", []byte("bad"))
	if err == nil {
		t.Fatal("expected nested link escape to be rejected")
	}
	if _, err := os.Stat(filepath.Join(outside, "settings.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("restore wrote outside target root: %v", err)
	}
	outsideFile := filepath.Join(outside, "existing.json")
	if err := os.WriteFile(outsideFile, []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	repoRel, err := spec.RepoPath("linked/existing.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := (&ResourceApplier{}).Delete(spec, repoRel); err == nil {
		t.Fatal("expected nested link deletion to be rejected")
	}
	assertFileContent(t, outsideFile, "keep")
}

func assertFileContent(t *testing.T, filename, want string) {
	t.Helper()
	data, err := os.ReadFile(filename)
	if err != nil {
		t.Fatalf("read %s: %v", filename, err)
	}
	if string(data) != want {
		t.Fatalf("%s = %q, want %q", filename, data, want)
	}
}
