package syncengine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/qinqingxu/synchub-for-agents/internal/resource"
)

func TestFreshLibraryRemoteProtectsLegacyOutputsBeforeCollection(t *testing.T) {
	user, repo := t.TempDir(), t.TempDir()
	old := "agents/_portable/config/common/skills/common-skills/review/SKILL.md"
	canonical := "agents/_portable/config/common/skills/library-skills/review/SKILL.md"
	for _, rel := range []string{old, canonical} {
		name := filepath.Join(repo, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(name), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(name, []byte("Review tests.\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	specs := map[string]resource.Spec{
		"common/common-skills": {
			Key: "common/common-skills", Provider: "common", ID: "skills", SharedAs: "common-skills",
			Category: resource.CategorySkills, Strategy: resource.StrategySourceTree, Layout: resource.LayoutPortable,
			Root:    filepath.Join(user, ".agents", "skills"),
			Targets: []string{filepath.Join(user, ".agents", "skills"), filepath.Join(user, ".claude", "skills")},
		},
		"common/library-skills": {
			Key: "common/library-skills", Provider: "common", ID: "library-skills",
			Category: resource.CategorySkills, Strategy: resource.StrategySourceTree, Layout: resource.LayoutPortable,
			Root:    filepath.Join(user, ".agents", "library", "skills"),
			Targets: []string{filepath.Join(user, ".agents", "library", "skills")},
		},
	}
	engine := Engine{RepoDir: repo, UserHome: user, Resources: specs}
	remote, err := engine.loadRemote(nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := remote.owned[old]; ok {
		t.Fatal("legacy output owned before first canonical restore")
	}
	if _, ok := remote.owned[canonical]; !ok {
		t.Fatal("canonical source not owned")
	}
	if engine.Resources["common/common-skills"].Owns("review/SKILL.md") {
		t.Fatal("local collection can still feed legacy alias loop")
	}
}
