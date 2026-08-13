package syncengine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/qinqingxu/acsync/internal/secret"
)

func TestApplyPushPullDelete(t *testing.T) {
	repoDir := t.TempDir()
	localRoot := t.TempDir()

	specs := map[string]AgentSpec{"c": {Name: "c", Root: localRoot}}

	// Prepare a local source file for push.
	localSrc := filepath.Join(localRoot, "a.json")
	writeFile(t, localSrc, `{"v":1}`)

	// Prepare a repo file for pull.
	writeFile(t, filepath.Join(repoDir, "agents", "c", "config", "b.json"), `{"v":2}`)

	// Prepare a repo file to delete.
	writeFile(t, filepath.Join(repoDir, "agents", "c", "config", "d.json"), `{"v":3}`)

	// Prepare a local file to delete.
	writeFile(t, filepath.Join(localRoot, "e.json"), `{"v":4}`)

	// Prepare a blocked repo file to remove without retaining it in trash.
	writeFile(t, filepath.Join(repoDir, "agents", "c", "config", "blocked.json"), `{"token":"old"}`)

	ap := &Applier{
		RepoDir: repoDir,
		Specs:   specs,
		Sources: map[string]string{"agents/c/config/a.json": localSrc},
		Now:     time.Unix(1000, 0),
	}
	actions := []Action{
		{PushToRemote, "agents/c/config/a.json"},
		{PullToLocal, "agents/c/config/b.json"},
		{DeleteRemote, "agents/c/config/d.json"},
		{DeleteLocal, "agents/c/config/e.json"},
		{RemoveRemote, "agents/c/config/blocked.json"},
	}
	if err := ap.Apply(actions); err != nil {
		t.Fatalf("Apply error: %v", err)
	}

	// Push landed in repo.
	if _, err := os.Stat(filepath.Join(repoDir, "agents", "c", "config", "a.json")); err != nil {
		t.Errorf("pushed file missing in repo: %v", err)
	}
	// Pull landed locally.
	if _, err := os.Stat(filepath.Join(localRoot, "b.json")); err != nil {
		t.Errorf("pulled file missing locally: %v", err)
	}
	// Delete-remote removed from repo and moved to trash.
	if _, err := os.Stat(filepath.Join(repoDir, "agents", "c", "config", "d.json")); !os.IsNotExist(err) {
		t.Errorf("deleted repo file should be gone, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(repoDir, ".trash", "files", "agents", "c", "config", "d.json")); err != nil {
		t.Errorf("deleted file should be in trash: %v", err)
	}
	// Trash index records deletion time.
	idxData, err := os.ReadFile(filepath.Join(repoDir, ".trash", "index.json"))
	if err != nil {
		t.Fatalf("trash index missing: %v", err)
	}
	var idx map[string]int64
	if err := json.Unmarshal(idxData, &idx); err != nil {
		t.Fatal(err)
	}
	if idx["agents/c/config/d.json"] != 1000 {
		t.Errorf("trash index time = %d, want 1000", idx["agents/c/config/d.json"])
	}
	// Delete-local removed the local file.
	if _, err := os.Stat(filepath.Join(localRoot, "e.json")); !os.IsNotExist(err) {
		t.Errorf("deleted local file should be gone, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(repoDir, "agents", "c", "config", "blocked.json")); !os.IsNotExist(err) {
		t.Errorf("blocked repo file should be removed, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(repoDir, ".trash", "files", "agents", "c", "config", "blocked.json")); !os.IsNotExist(err) {
		t.Errorf("blocked repo file must not be retained in trash, err=%v", err)
	}
}

func TestApplyPushRescansExactBytesBeforeReplacingRepoFile(t *testing.T) {
	repoDir := t.TempDir()
	localRoot := t.TempDir()
	repoRel := "agents/c/config/settings.json"
	localSrc := filepath.Join(localRoot, "settings.json")
	repoPath := filepath.Join(repoDir, filepath.FromSlash(repoRel))
	writeFile(t, localSrc, `{"accessToken":"new-secret"}`)
	writeFile(t, repoPath, `{"theme":"safe"}`)

	ap := &Applier{
		RepoDir: repoDir,
		Specs: map[string]AgentSpec{"c": {
			Name:    "c",
			Root:    localRoot,
			Scanner: secret.NewScanner(nil, []string{"token"}),
		}},
		Sources: map[string]string{repoRel: localSrc},
	}
	if err := ap.Apply([]Action{{PushToRemote, repoRel}}); err == nil {
		t.Fatal("expected exact push bytes to be blocked")
	}
	got, err := os.ReadFile(repoPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `{"theme":"safe"}` {
		t.Fatalf("repo file changed despite blocked push: %s", got)
	}
	if matches, err := filepath.Glob(filepath.Join(repoDir, "agents", "**", "*.acsync-stage*")); err != nil || len(matches) != 0 {
		t.Fatalf("staging files must not be created under agents: matches=%v err=%v", matches, err)
	}
}

func TestSnapshotRepoSkipsTrashAndGit(t *testing.T) {
	repoDir := t.TempDir()
	writeFile(t, filepath.Join(repoDir, "agents", "c", "config", "a.json"), `{"v":1}`)
	writeFile(t, filepath.Join(repoDir, ".trash", "files", "agents", "c", "config", "old.json"), `{"v":9}`)
	writeFile(t, filepath.Join(repoDir, ".git", "HEAD"), "ref: refs/heads/main")
	writeFile(t, filepath.Join(repoDir, "manifest.json"), `{}`)

	snap, err := SnapshotRepo(repoDir)
	if err != nil {
		t.Fatalf("SnapshotRepo error: %v", err)
	}
	if _, ok := snap["agents/c/config/a.json"]; !ok {
		t.Error("expected agents file in snapshot")
	}
	if len(snap) != 1 {
		t.Errorf("snapshot should only include agents/** files, got %v", snap)
	}
}
