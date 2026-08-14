package conflict

import (
	"errors"
	"os"
	"path/filepath"
	"sort"
	"testing"
	"time"
)

func TestStoreCreatesScannedLocalAndRepositoryBundles(t *testing.T) {
	localRoot := t.TempDir()
	repoDir := t.TempDir()
	var scanned []string
	store := NewStore(localRoot, repoDir, func(_ Record, variant string, _ []byte) error {
		scanned = append(scanned, variant)
		return nil
	})
	record := Record{
		ID:          "conflict-1",
		ResourceKey: "claude/settings",
		RepoRel:     "agents/_portable/config/providers/claude/config/settings/settings.json",
		CreatedAt:   time.Date(2026, 8, 14, 1, 2, 3, 0, time.UTC),
	}

	if err := store.Create(record, []byte("base"), []byte("local"), []byte("remote")); err != nil {
		t.Fatal(err)
	}
	sort.Strings(scanned)
	if got := scanned; len(got) != 3 || got[0] != "base" || got[1] != "local" || got[2] != "remote" {
		t.Fatalf("scanned = %#v", got)
	}
	for _, root := range []string{
		filepath.Join(localRoot, record.ID),
		filepath.Join(repoDir, "agents", "_portable", "config", "conflicts", record.ID),
	} {
		for filename, want := range map[string]string{
			"base":   "base",
			"local":  "local",
			"remote": "remote",
		} {
			data, err := os.ReadFile(filepath.Join(root, filename))
			if err != nil || string(data) != want {
				t.Fatalf("read %s/%s = %q, %v", root, filename, data, err)
			}
		}
		if _, err := os.Stat(filepath.Join(root, "record.json")); err != nil {
			t.Fatal(err)
		}
	}
	records, err := store.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 1 || records[0] != record {
		t.Fatalf("records = %#v", records)
	}
}

func TestStoreScansAllVariantsBeforeWritingAnything(t *testing.T) {
	localRoot := t.TempDir()
	repoDir := t.TempDir()
	store := NewStore(localRoot, repoDir, func(_ Record, variant string, _ []byte) error {
		if variant == "local" {
			return errors.New("secret detected")
		}
		return nil
	})
	record := Record{
		ID:          "blocked",
		ResourceKey: "demo/settings",
		RepoRel:     "agents/_portable/config/providers/demo/config/settings/settings.json",
	}
	if err := store.Create(record, []byte("base"), []byte("secret"), []byte("remote")); err == nil {
		t.Fatal("Create() error = nil")
	}
	if _, err := os.Stat(filepath.Join(localRoot, record.ID)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("local bundle exists: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repoDir, "agents", "_portable", "config", "conflicts", record.ID)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("repository bundle exists: %v", err)
	}
}

func TestStoreRejectsUnsafeIDsAndRepoPaths(t *testing.T) {
	store := NewStore(t.TempDir(), t.TempDir(), nil)
	for _, record := range []Record{
		{ID: "../escape", ResourceKey: "demo/settings", RepoRel: "agents/demo/config/settings.json"},
		{ID: "safe", ResourceKey: "demo/settings", RepoRel: "../outside"},
		{ID: "safe", ResourceKey: "demo/settings", RepoRel: `agents\demo\config\settings.json`},
	} {
		if err := store.Create(record, nil, nil, nil); err == nil {
			t.Fatalf("Create(%#v) error = nil", record)
		}
	}
}

func TestStoreResolveScansWritesCanonicalAndRemovesBundles(t *testing.T) {
	localRoot := t.TempDir()
	repoDir := t.TempDir()
	var variants []string
	store := NewStore(localRoot, repoDir, func(_ Record, variant string, _ []byte) error {
		variants = append(variants, variant)
		return nil
	})
	record := Record{
		ID:          "resolve-me",
		ResourceKey: "demo/settings",
		RepoRel:     "agents/_portable/config/providers/demo/config/settings/settings.json",
	}
	if err := store.Create(record, []byte("base"), []byte("local"), []byte("remote")); err != nil {
		t.Fatal(err)
	}
	if err := store.Resolve(record.ID, []byte("merged")); err != nil {
		t.Fatal(err)
	}
	if variants[len(variants)-1] != "merged" {
		t.Fatalf("variants = %#v", variants)
	}
	data, err := os.ReadFile(filepath.Join(repoDir, filepath.FromSlash(record.RepoRel)))
	if err != nil || string(data) != "merged" {
		t.Fatalf("canonical = %q, %v", data, err)
	}
	for _, path := range []string{
		filepath.Join(localRoot, record.ID),
		filepath.Join(repoDir, "agents", "_portable", "config", "conflicts", record.ID),
	} {
		if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("bundle remains at %s: %v", path, err)
		}
	}
	records, err := store.List()
	if err != nil || len(records) != 0 {
		t.Fatalf("List() = %#v, %v", records, err)
	}
	if err := store.Resolve("missing", []byte("data")); err == nil {
		t.Fatal("resolving a missing conflict succeeded")
	}
}
