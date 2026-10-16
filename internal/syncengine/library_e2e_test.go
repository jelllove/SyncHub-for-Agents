package syncengine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/qinqingxu/synchub-for-agents/internal/resource"
	"github.com/qinqingxu/synchub-for-agents/internal/sharedlibrary"
)

func TestLibrarySourceRestoresToFreshComputerWithoutApprovingOutputs(t *testing.T) {
	bare := newBareRemote(t)
	userA, userB := t.TempDir(), t.TempDir()
	newEngine := func(user string) *Engine {
		repo := filepath.Join(t.TempDir(), "repo")
		root := filepath.Join(sharedlibrary.Root(user), "skills")
		specs := map[string]resource.Spec{
			"common/library-skills": {
				Key: "common/library-skills", Provider: "common", ID: "library-skills",
				Category: resource.CategorySkills, Strategy: resource.StrategySourceTree,
				Layout: resource.LayoutPortable, Root: root, Targets: []string{root}, Include: []string{"**"},
			},
		}
		if err := sharedlibrary.ProtectSpecs(specs, user); err != nil {
			t.Fatal(err)
		}
		return &Engine{Git: cloneWorkspace(t, bare, repo), RepoDir: repo,
			Home: t.TempDir(), UserHome: user, GOOS: "windows",
			StatePath: filepath.Join(t.TempDir(), "state.json"), Resources: specs}
	}
	writeFile(t, filepath.Join(sharedlibrary.Root(userA), "skills", "review", "SKILL.md"), "Review all tests.\n")
	a := newEngine(userA)
	if _, err := a.SyncOnce(); err != nil {
		t.Fatal(err)
	}
	b := newEngine(userB)
	if _, err := b.SyncOnce(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(sharedlibrary.Root(userB), "skills", "review", "SKILL.md"))
	if err != nil || string(data) != "Review all tests.\n" {
		t.Fatalf("fresh canonical restore = %q, %v", data, err)
	}
	for _, output := range []string{".claude", ".copilot", filepath.Join(".agents", "skills")} {
		if _, err := os.Stat(filepath.Join(userB, output)); !os.IsNotExist(err) {
			t.Fatalf("fresh machine wrote unapproved output %s", output)
		}
	}
}
