package syncengine

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/qinqingxu/synchub-for-agents/internal/resource"
	"github.com/qinqingxu/synchub-for-agents/internal/resourcecollect"
	"github.com/qinqingxu/synchub-for-agents/internal/sharedlibrary"
)

func libraryDirectoryLink(t *testing.T, target, link string) {
	t.Helper()
	err := os.Symlink(target, link)
	if err == nil {
		return
	}
	if runtime.GOOS == "windows" {
		cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command",
			"$ErrorActionPreference = 'Stop'; New-Item -ItemType Junction -Path $env:SYNCHUB_TEST_LINK -Target $env:SYNCHUB_TEST_TARGET | Out-Null")
		cmd.Env = append(os.Environ(), "SYNCHUB_TEST_LINK="+link, "SYNCHUB_TEST_TARGET="+target)
		if out, junctionErr := cmd.CombinedOutput(); junctionErr == nil {
			return
		} else {
			t.Skipf("directory links unavailable: %v; %v %s", err, junctionErr, out)
		}
	}
	t.Skipf("directory links unavailable: %v", err)
}

func TestCanonicalLibraryRejectsNestedLinksDuringCollectionAndRestore(t *testing.T) {
	user := t.TempDir()
	root := filepath.Join(sharedlibrary.Root(user), "skills")
	sibling := filepath.Join(root, "second")
	writeFile(t, filepath.Join(sibling, "SKILL.md"), "Original sibling\n")
	libraryDirectoryLink(t, sibling, filepath.Join(root, "first"))
	specs := map[string]resource.Spec{
		"common/library-skills": {Key: "common/library-skills", Provider: "common", ID: "library-skills",
			Category: resource.CategorySkills, Strategy: resource.StrategySourceTree, Layout: resource.LayoutPortable,
			Root: root, Targets: []string{root}, Include: []string{"**"}},
	}
	if err := sharedlibrary.ProtectSpecs(specs, user); err != nil {
		t.Fatal(err)
	}
	spec := specs["common/library-skills"]
	result, err := resourcecollect.New(resourcecollect.Options{}).Collect([]resource.Spec{spec})
	if err == nil {
		result.Close()
		t.Error("collector followed a nested canonical link")
	}
	a := ResourceApplier{}
	if err := a.RestoreBytes(spec, "first/SKILL.md", []byte("Overwrite sibling")); err == nil {
		t.Error("restore followed a nested canonical link")
	}
	repoRel, err := spec.RepoPath("first/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	if err := a.Delete(spec, repoRel); err == nil {
		t.Error("delete followed a nested canonical link")
	}
	data, err := os.ReadFile(filepath.Join(sibling, "SKILL.md"))
	if err != nil || string(data) != "Original sibling\n" {
		t.Errorf("sibling source modified: %q %v", data, err)
	}
}
