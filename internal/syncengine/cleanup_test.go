package syncengine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/qinqingxu/acsync/internal/resource"
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

func TestPurgeBlockedTrashRemovesSecretAndIndexEntry(t *testing.T) {
	repo := t.TempDir()
	repoRel := "agents/demo/sessions/secret.json"
	writeFile(t, filepath.Join(repo, ".trash", "files", filepath.FromSlash(repoRel)), `{"accessToken":"secret"}`)
	data, _ := json.Marshal(map[string]int64{repoRel: time.Now().Unix()})
	writeFile(t, filepath.Join(repo, ".trash", "index.json"), string(data))
	specs := map[string]resource.Spec{"demo/legacy-sessions": {
		Key:         "demo/legacy-sessions",
		Provider:    "demo",
		ID:          "legacy-sessions",
		Category:    resource.CategorySessions,
		Strategy:    resource.StrategyFileTree,
		Layout:      resource.LayoutLegacy,
		Include:     []string{"*.json"},
		KeyPatterns: []string{"token"},
	}}

	purged, err := PurgeBlockedTrash(repo, specs)
	if err != nil {
		t.Fatal(err)
	}

	if len(purged) != 1 || purged[0] != repoRel {
		t.Fatalf("purged = %#v", purged)
	}
	if _, err := os.Stat(filepath.Join(repo, ".trash", "files", filepath.FromSlash(repoRel))); !os.IsNotExist(err) {
		t.Fatal("blocked trash file still exists")
	}
	raw, err := os.ReadFile(filepath.Join(repo, ".trash", "index.json"))
	if err != nil {
		t.Fatal(err)
	}
	var index map[string]int64
	if err := json.Unmarshal(raw, &index); err != nil {
		t.Fatal(err)
	}
	if _, exists := index[repoRel]; exists {
		t.Fatal("blocked trash entry still exists in index")
	}
}

func TestPurgeBlockedTrashValidatesPortableResourcePaths(t *testing.T) {
	repo := t.TempDir()
	spec := resource.Spec{
		Key:         "demo/settings",
		Provider:    "demo",
		ID:          "settings",
		Category:    resource.CategoryConfig,
		Strategy:    resource.StrategyFileTree,
		Layout:      resource.LayoutPortable,
		Include:     []string{"*.json"},
		KeyPatterns: []string{"token"},
	}
	repoRel, err := spec.RepoPath("settings.json")
	if err != nil {
		t.Fatal(err)
	}
	writeFile(
		t,
		filepath.Join(repo, ".trash", "files", filepath.FromSlash(repoRel)),
		`{"accessToken":"secret"}`,
	)
	data, _ := json.Marshal(map[string]int64{repoRel: time.Now().Unix()})
	writeFile(t, filepath.Join(repo, ".trash", "index.json"), string(data))

	purged, err := PurgeBlockedTrash(repo, map[string]resource.Spec{spec.Key: spec})
	if err != nil {
		t.Fatal(err)
	}
	if len(purged) != 1 || purged[0] != repoRel {
		t.Fatalf("purged = %#v", purged)
	}
}

func TestPurgeBlockedTrashPreservesValidConflictBundle(t *testing.T) {
	repo := t.TempDir()
	bundleRoot := filepath.Join(
		repo,
		".trash",
		"files",
		"agents",
		"_portable",
		"config",
		"conflicts",
		"c1",
	)
	record := `{
  "id": "c1",
  "resourceKey": "disabled/settings",
  "repoRel": "agents/_portable/config/providers/disabled/config/settings/settings.json",
  "createdAt": "2026-08-14T12:00:00Z"
}`
	writeFile(t, filepath.Join(bundleRoot, "record.json"), record)
	for _, variant := range []string{"base", "local", "remote"} {
		writeFile(t, filepath.Join(bundleRoot, variant), `{"theme":"dark"}`)
	}
	index := map[string]int64{}
	for _, filename := range []string{"record.json", "base", "local", "remote"} {
		index["agents/_portable/config/conflicts/c1/"+filename] = time.Now().Unix()
	}
	data, _ := json.Marshal(index)
	writeFile(t, filepath.Join(repo, ".trash", "index.json"), string(data))

	purged, err := PurgeBlockedTrash(repo, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(purged) != 0 {
		t.Fatalf("valid conflict bundle was purged: %#v", purged)
	}
}

func TestCleanupTrashRejectsTraversalIndexPath(t *testing.T) {
	repo := t.TempDir()
	victim := filepath.Join(repo, "victim.txt")
	writeFile(t, victim, "keep")
	repoRel := "agents/demo/config/../../../../victim.txt"
	data, _ := json.Marshal(map[string]int64{repoRel: 0})
	writeFile(t, filepath.Join(repo, ".trash", "index.json"), string(data))

	if _, err := CleanupTrash(repo, time.Now(), 0); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(victim); err != nil || string(got) != "keep" {
		t.Fatalf("victim was changed: %q, %v", got, err)
	}
}

func TestPurgeBlockedTrashRemovesUnindexedFile(t *testing.T) {
	repo := t.TempDir()
	orphan := filepath.Join(repo, ".trash", "files", "agents", "demo", "sessions", "orphan.json")
	writeFile(t, orphan, `{"accessToken":"secret"}`)
	writeFile(t, filepath.Join(repo, ".trash", "index.json"), "{}")

	purged, err := PurgeBlockedTrash(repo, map[string]resource.Spec{})
	if err != nil {
		t.Fatal(err)
	}
	if len(purged) != 1 || purged[0] != "agents/demo/sessions/orphan.json" {
		t.Fatalf("purged = %#v", purged)
	}
	if _, err := os.Stat(orphan); !os.IsNotExist(err) {
		t.Fatal("unindexed trash file still exists")
	}
}
