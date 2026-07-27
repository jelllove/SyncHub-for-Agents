package syncengine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCleanupTrashRemovesExpired(t *testing.T) {
	repo := t.TempDir()
	writeFile(t, filepath.Join(repo, ".trash", "files", "agents", "a", "config", "old.json"), "old")
	writeFile(t, filepath.Join(repo, ".trash", "files", "agents", "a", "sessions", "new.jsonl"), "new")

	now := time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC)
	idx := map[string]int64{
		"agents/a/config/old.json":    now.AddDate(0, 0, -40).Unix(),
		"agents/a/sessions/new.jsonl": now.AddDate(0, 0, -5).Unix(),
	}
	data, _ := json.MarshalIndent(idx, "", "  ")
	writeFile(t, filepath.Join(repo, ".trash", "index.json"), string(data))

	purged, err := CleanupTrash(repo, now, 30)
	if err != nil {
		t.Fatalf("CleanupTrash error: %v", err)
	}
	if len(purged) != 1 || purged[0] != "agents/a/config/old.json" {
		t.Fatalf("purged = %v, want [agents/a/config/old.json]", purged)
	}
	if _, err := os.Stat(filepath.Join(repo, ".trash", "files", "agents", "a", "config", "old.json")); !os.IsNotExist(err) {
		t.Error("expired file should be removed")
	}
	if _, err := os.Stat(filepath.Join(repo, ".trash", "files", "agents", "a", "sessions", "new.jsonl")); err != nil {
		t.Error("recent file should remain")
	}

	raw, _ := os.ReadFile(filepath.Join(repo, ".trash", "index.json"))
	got := map[string]int64{}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if _, ok := got["agents/a/config/old.json"]; ok {
		t.Error("expired entry should be gone from index")
	}
	if _, ok := got["agents/a/sessions/new.jsonl"]; !ok {
		t.Error("recent entry should remain in index")
	}
}

func TestCleanupTrashNoIndex(t *testing.T) {
	purged, err := CleanupTrash(t.TempDir(), time.Now(), 30)
	if err != nil {
		t.Fatalf("CleanupTrash error: %v", err)
	}
	if purged != nil {
		t.Errorf("purged = %v, want nil when no trash index", purged)
	}
}
