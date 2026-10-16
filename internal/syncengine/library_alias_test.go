package syncengine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/qinqingxu/synchub-for-agents/internal/resource"
)

func TestLibraryCopyOnlyRejectsRedirectedAlias(t *testing.T) {
	root, external := t.TempDir(), t.TempDir()
	primary := filepath.Join(root, "primary")
	alias := filepath.Join(root, "alias")
	if err := os.MkdirAll(primary, 0o700); err != nil {
		t.Fatal(err)
	}
	libraryDirectoryLink(t, external, alias)
	spec := resource.Spec{Key: "common/common-skills", Provider: "common", ID: "skills", SharedAs: "common-skills",
		Layout: resource.LayoutPortable, Category: resource.CategorySkills, Strategy: resource.StrategySourceTree,
		Targets: []string{primary, alias}, CopyOnly: true}
	a := ResourceApplier{}
	if err := a.RestoreBytes(spec, "unrelated/SKILL.md", []byte("Must not escape")); err == nil {
		t.Fatal("copy-only mode accepted redirected alias")
	}
	if _, err := a.targetsForDelete(spec); err == nil {
		t.Fatal("copy-only deletion accepted redirected alias")
	}
}

func TestLibraryCopyOnlyDoesNotCreateWholeDirectoryAlias(t *testing.T) {
	root := t.TempDir()
	primary, alias := filepath.Join(root, "primary"), filepath.Join(root, "alias")
	spec := resource.Spec{Key: "common/common-skills", Provider: "common", ID: "skills", SharedAs: "common-skills",
		Layout: resource.LayoutPortable, Category: resource.CategorySkills, Strategy: resource.StrategySourceTree,
		Targets: []string{primary, alias}, CopyOnly: true}
	a := ResourceApplier{Symlink: func(string, string) error { t.Fatal("created new alias"); return nil }}
	if err := a.RestoreBytes(spec, "private/SKILL.md", []byte("Private\n")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(alias, "private", "SKILL.md")); err != nil {
		t.Fatal(err)
	}
}
