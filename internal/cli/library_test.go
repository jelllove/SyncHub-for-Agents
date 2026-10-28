package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/qinqingxu/synchub-for-agents/internal/config"
	"github.com/qinqingxu/synchub-for-agents/internal/provider"
	"github.com/qinqingxu/synchub-for-agents/internal/sharedlibrary"
	"github.com/qinqingxu/synchub-for-agents/internal/state"
	"github.com/qinqingxu/synchub-for-agents/internal/syncengine"
)

func TestLibrarySourcesHaveExclusiveSyncOwnership(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, ".agents", "library", "skills", "review")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte("Review tests."), 0o600); err != nil {
		t.Fatal(err)
	}
	providers, err := provider.Builtins()
	if err != nil {
		t.Fatal(err)
	}
	specs, err := BuildResourceSpecs(config.Default([]string{"common", "claude", "copilot"}), providers, runtime.GOOS, home)
	if err != nil {
		t.Fatal(err)
	}
	shared := specs["common/common-skills"]
	if shared.Owns("review/SKILL.md") || !shared.Owns("private/SKILL.md") || !shared.CopyOnly {
		t.Fatalf("legacy ownership not protected: %+v", shared)
	}
	canonical, ok := specs["common/library-skills"]
	if !ok || canonical.Root != filepath.Join(sharedlibrary.Root(home), "skills") || !canonical.Owns("review/SKILL.md") {
		t.Fatalf("canonical library not enabled: %+v", canonical)
	}
	repoPath, _ := shared.RepoPath("review/SKILL.md")
	owned, untouched, blocked := syncengine.SplitRemoteSnapshot(state.Snapshot{repoPath: {}}, specs)
	if len(owned) != 0 || len(untouched) != 1 || len(blocked) != 0 {
		t.Fatal("managed output is still restored by legacy sync")
	}
	applier := syncengine.ResourceApplier{}
	if err := applier.RestoreBytes(shared, "review/SKILL.md", []byte("must not overwrite")); err == nil {
		t.Fatal("direct restoration bypassed library ownership")
	}
}
