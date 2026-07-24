package state

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHashFileStable(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(f, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	h1, err := HashFile(f)
	if err != nil {
		t.Fatalf("HashFile error: %v", err)
	}
	h2, _ := HashFile(f)
	if h1 != h2 {
		t.Errorf("hash not stable: %q vs %q", h1, h2)
	}
	// Known SHA-256 of "hello".
	want := "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
	if h1 != want {
		t.Errorf("HashFile = %q, want %q", h1, want)
	}
}

func TestSaveAndLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	snap := Snapshot{
		"agents/claude/config/settings.json": {Hash: "abc", ModTime: 100, Size: 12},
	}
	if err := Save(path, snap); err != nil {
		t.Fatalf("Save error: %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	fm, ok := got["agents/claude/config/settings.json"]
	if !ok {
		t.Fatal("missing entry after round trip")
	}
	if fm.Hash != "abc" || fm.ModTime != 100 || fm.Size != 12 {
		t.Errorf("round trip mismatch: %+v", fm)
	}
}

func TestLoadMissingReturnsEmpty(t *testing.T) {
	got, err := Load(filepath.Join(t.TempDir(), "nope.json"))
	if err != nil {
		t.Fatalf("Load of missing file should not error, got %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty snapshot, got %d entries", len(got))
	}
}