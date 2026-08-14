package state

import (
	"os"
	"path/filepath"
	"testing"
)

func TestBaseStoreCapturesAndLoadsByRepoPath(t *testing.T) {
	root := t.TempDir()
	store := NewBaseStore(root)
	repoRel := "agents/_portable/config/providers/demo/config/settings/settings.json"
	if err := store.Put(repoRel, []byte("base")); err != nil {
		t.Fatal(err)
	}
	got, ok, err := store.Get(repoRel)
	if err != nil || !ok || string(got) != "base" {
		t.Fatalf("Get = %q, %v, %v", got, ok, err)
	}

	reopened := NewBaseStore(root)
	got, ok, err = reopened.Get(repoRel)
	if err != nil || !ok || string(got) != "base" {
		t.Fatalf("reopened Get = %q, %v, %v", got, ok, err)
	}
}

func TestBaseStoreDeduplicatesObjectsAndDeletesOnlyIndexEntry(t *testing.T) {
	root := t.TempDir()
	store := NewBaseStore(root)
	first := "agents/_portable/config/providers/demo/config/settings/one.json"
	second := "agents/_portable/config/providers/demo/config/settings/two.json"
	if err := store.Put(first, []byte("same")); err != nil {
		t.Fatal(err)
	}
	if err := store.Put(second, []byte("same")); err != nil {
		t.Fatal(err)
	}
	objects, err := os.ReadDir(filepath.Join(root, "base", "objects"))
	if err != nil {
		t.Fatal(err)
	}
	if len(objects) != 1 {
		t.Fatalf("objects = %d, want 1", len(objects))
	}
	if err := store.Delete(first); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := store.Get(first); err != nil || ok {
		t.Fatalf("deleted Get ok=%v err=%v", ok, err)
	}
	got, ok, err := store.Get(second)
	if err != nil || !ok || string(got) != "same" {
		t.Fatalf("remaining Get = %q, %v, %v", got, ok, err)
	}
}

func TestBaseStoreCaptureRepoReadsOnlySnapshotPaths(t *testing.T) {
	repoDir := t.TempDir()
	tracked := "agents/demo/config/settings.json"
	untracked := "agents/demo/config/local.json"
	writeStateFile(t, filepath.Join(repoDir, filepath.FromSlash(tracked)), "tracked")
	writeStateFile(t, filepath.Join(repoDir, filepath.FromSlash(untracked)), "untracked")
	store := NewBaseStore(t.TempDir())

	if err := store.CaptureRepo(repoDir, Snapshot{tracked: {Hash: "ignored"}}); err != nil {
		t.Fatal(err)
	}
	if got, ok, err := store.Get(tracked); err != nil || !ok || string(got) != "tracked" {
		t.Fatalf("tracked Get = %q, %v, %v", got, ok, err)
	}
	if _, ok, err := store.Get(untracked); err != nil || ok {
		t.Fatalf("untracked Get ok=%v err=%v", ok, err)
	}
}

func TestBaseStoreCaptureRepoPreservingDoesNotAdvancePreservedBytes(t *testing.T) {
	repoDir := t.TempDir()
	repoRel := "agents/demo/config/settings.json"
	writeStateFile(t, filepath.Join(repoDir, filepath.FromSlash(repoRel)), "new")
	store := NewBaseStore(t.TempDir())
	if err := store.Put(repoRel, []byte("old")); err != nil {
		t.Fatal(err)
	}

	if err := store.CaptureRepoPreserving(
		repoDir,
		Snapshot{repoRel: {Hash: "new"}},
		map[string]struct{}{repoRel: {}},
	); err != nil {
		t.Fatal(err)
	}

	got, ok, err := store.Get(repoRel)
	if err != nil || !ok || string(got) != "old" {
		t.Fatalf("preserved Get = %q, %v, %v", got, ok, err)
	}
}

func TestBaseStoreRejectsUnsafeRepoPaths(t *testing.T) {
	store := NewBaseStore(t.TempDir())
	for _, repoRel := range []string{
		"",
		"../outside",
		"/absolute",
		`agents\demo\config\settings.json`,
		"agents/demo/config/../secret",
	} {
		if err := store.Put(repoRel, []byte("data")); err == nil {
			t.Fatalf("Put(%q) error = nil", repoRel)
		}
		if _, _, err := store.Get(repoRel); err == nil {
			t.Fatalf("Get(%q) error = nil", repoRel)
		}
	}
}

func writeStateFile(t *testing.T, filename, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filename, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
