package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/qinqingxu/synchub-for-agents/internal/config"
	"github.com/qinqingxu/synchub-for-agents/internal/conflict"
	"github.com/qinqingxu/synchub-for-agents/internal/provider"
	"github.com/qinqingxu/synchub-for-agents/internal/resource"
	"github.com/qinqingxu/synchub-for-agents/internal/resourcecollect"
	"github.com/qinqingxu/synchub-for-agents/internal/state"
	"github.com/qinqingxu/synchub-for-agents/internal/syncengine"
)

func librarySpecs(t *testing.T, home string) map[string]resource.Spec {
	t.Helper()
	providers, err := provider.Builtins()
	if err != nil {
		t.Fatal(err)
	}
	specs, err := BuildResourceSpecs(config.Default([]string{"common"}), providers, runtime.GOOS, home)
	if err != nil {
		t.Fatal(err)
	}
	return specs
}

func TestLibrarySyncBlocksCredentialsBeforeStagingAndRestore(t *testing.T) {
	home, repo := t.TempDir(), t.TempDir()
	dir := filepath.Join(home, ".agents", "library", "skills", "review")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("PASSWORD=local-value\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	specs := librarySpecs(t, home)
	spec := specs["common/library-skills"]
	collected, err := resourcecollect.New(resourcecollect.Options{}).Collect([]resource.Spec{spec})
	if err != nil {
		t.Fatal(err)
	}
	defer collected.Close()
	if len(collected.Snapshot) != 0 || len(collected.Blocked) == 0 {
		t.Fatal("canonical credential file was staged")
	}
	rel, err := spec.RepoPath("review/.env")
	if err != nil {
		t.Fatal(err)
	}
	remoteFile := filepath.Join(repo, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(remoteFile), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(remoteFile, []byte("PASSWORD=remote-value\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	valid, blocked := syncengine.ValidateRemoteResources(repo, state.Snapshot{rel: {}}, specs, nil, runtime.GOOS, home)
	if len(valid) != 0 || len(blocked) != 1 {
		t.Fatal("remote library credentials were accepted")
	}
	applier := syncengine.ResourceApplier{}
	if err := syncengine.ConflictScanner(specs)(conflict.Record{RepoRel: rel}, "merged", []byte("PASSWORD=merged-value\n")); err == nil {
		t.Fatal("conflict resolution bypassed library policy")
	}
	if err := applier.RestoreBytes(spec, "review/.env", []byte("PASSWORD=remote-value\n")); err == nil {
		t.Fatal("direct restore bypassed library policy")
	}
}

func TestAbsentLibraryRootsAreEmptyNotUnavailable(t *testing.T) {
	home := t.TempDir()
	specs := librarySpecs(t, home)
	collected, err := resourcecollect.New(resourcecollect.Options{}).Collect([]resource.Spec{
		specs["common/library-skills"], specs["common/library-instructions"],
	})
	if err != nil {
		t.Fatal(err)
	}
	defer collected.Close()
	if len(collected.Skipped) != 0 || len(collected.Blocked) != 0 || len(collected.Snapshot) != 0 {
		t.Fatalf("optional absent roots blocked restoration: %+v", collected)
	}
}
