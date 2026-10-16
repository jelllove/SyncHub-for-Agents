package sharedlibrary

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/qinqingxu/synchub-for-agents/internal/resource"
)

func directoryAlias(t *testing.T, target, alias string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		canonical, err := filepath.EvalSymlinks(target)
		if err != nil {
			t.Fatal(err)
		}
		if output, err := exec.Command("cmd", "/c", "mklink", "/J", alias, canonical).CombinedOutput(); err != nil {
			t.Fatalf("create isolated junction: %v: %s", err, output)
		}
		return
	}
	if err := os.Symlink(target, alias); err != nil {
		t.Fatal(err)
	}
}

func TestLibraryAllowsAliasesAboveUserHome(t *testing.T) {
	parent := t.TempDir()
	real := filepath.Join(parent, "real")
	if err := os.MkdirAll(filepath.Join(real, "user"), 0o700); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(parent, "alias")
	directoryAlias(t, real, alias)
	m := Manager{UserHome: filepath.Join(alias, "user")}
	missing := Manager{UserHome: filepath.Join(alias, "missing-user")}
	if _, err := missing.Snapshot(); err != nil {
		t.Fatalf("absent home below an ancestor alias blocked inventory: %v", err)
	}
	lock, err := Lock(m.UserHome)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := lock.Close(); err != nil {
			t.Error(err)
		}
	})
	if _, err := m.Snapshot(); err != nil {
		t.Fatalf("system-level ancestor alias blocked inventory: %v", err)
	}
	if _, err := SkillNames(m.UserHome); err != nil {
		t.Fatal(err)
	}
	put(t, filepath.Join(m.UserHome, ".claude", "skills", "review", "SKILL.md"), "Review carefully.\n")
	execute(t, m, Request{Action: "import", Kind: "skills", Name: "review", Harness: "claude"})
	execute(t, m, Request{Action: "apply", Kind: "skills", Name: "review", Harness: "common"})
	if _, err := m.Snapshot(); err != nil {
		t.Fatal(err)
	}
	specs := map[string]resource.Spec{
		"common/library-skills": {Provider: "common", ID: "library-skills", Root: filepath.Join(Root(m.UserHome), "skills")},
	}
	if err := ProtectSpecs(specs, m.UserHome); err != nil {
		t.Fatal(err)
	}
	spec := specs["common/library-skills"]
	if err := spec.ValidateRoot(spec.Root); err != nil {
		t.Fatal(err)
	}
	canonical, err := filepath.EvalSymlinks(filepath.Join(Root(filepath.Join(real, "user")), "skills"))
	if err != nil {
		t.Fatal(err)
	}
	if err := spec.ValidateRoot(canonical); err != nil {
		t.Fatalf("collector's canonical path blocked: %v", err)
	}
	execute(t, m, Request{Action: "restore", Kind: "skills", Name: "review", Harness: "common"})
}

func TestLibraryStillRejectsAliasesAtOrBelowUserHome(t *testing.T) {
	for _, rel := range []string{".", ".agents", ".agents/library", ".claude", ".copilot", ".synchub"} {
		t.Run(rel, func(t *testing.T) {
			parent := t.TempDir()
			home := filepath.Join(parent, "user")
			alias := filepath.Join(home, filepath.FromSlash(rel))
			if err := os.MkdirAll(filepath.Dir(alias), 0o700); err != nil {
				t.Fatal(err)
			}
			target := t.TempDir()
			directoryAlias(t, target, alias)
			harness := "claude"
			if rel == ".copilot" {
				harness = "copilot"
			}
			m := Manager{UserHome: home}
			if _, err := m.Preview(Request{Action: "import", Kind: "skills", Name: "review", Harness: harness}); err == nil || !strings.Contains(err.Error(), "symbolic link") {
				t.Fatalf("expected an alias diagnostic within the managed boundary, got %v", err)
			}
		})
	}
}

func TestLibraryValidationRejectsOutsideUserHome(t *testing.T) {
	home := t.TempDir()
	specs := map[string]resource.Spec{
		"common/library-skills": {Provider: "common", ID: "library-skills", Root: filepath.Join(Root(home), "skills")},
	}
	if err := ProtectSpecs(specs, home); err != nil {
		t.Fatal(err)
	}
	if err := specs["common/library-skills"].ValidateRoot(t.TempDir()); err == nil {
		t.Fatal("accepted a library validation path outside the user home")
	}
}
