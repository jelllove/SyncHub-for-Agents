package cli

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/qinqingxu/acsync/internal/config"
	"github.com/qinqingxu/acsync/internal/gitclient"
)

func gitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

func TestRunCleanupPurgesExpiredAndCommits(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	home := filepath.Join(t.TempDir(), ".acsync")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	remote := bareRemote(t)
	repo := RepoDir(home)

	client := &gitclient.Client{Dir: repo}
	if err := client.Clone(remote, repo); err != nil {
		t.Fatal(err)
	}
	gitCmd(t, repo, "config", "user.email", "t@e.com")
	gitCmd(t, repo, "config", "user.name", "t")

	// Seed a trashed file as if a prior sync recorded it, then commit+push.
	trashFile := filepath.Join(repo, ".trash", "files", "agents", "a", "config", "old.json")
	if err := os.MkdirAll(filepath.Dir(trashFile), 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(trashFile, []byte("old"), 0o644)
	idx := map[string]int64{"agents/a/config/old.json": time.Now().AddDate(0, 0, -40).Unix()}
	data, _ := json.MarshalIndent(idx, "", "  ")
	os.WriteFile(filepath.Join(repo, ".trash", "index.json"), data, 0o644)
	gitCmd(t, repo, "add", ".")
	gitCmd(t, repo, "commit", "-m", "seed trash")
	gitCmd(t, repo, "push", "origin", "main")

	if err := config.Save(ConfigPath(home), config.Config{
		RepoURL:        remote,
		TrashGraceDays: 30,
		Agents:         map[string]bool{},
	}); err != nil {
		t.Fatal(err)
	}

	purged, err := RunCleanup(home, time.Now())
	if err != nil {
		t.Fatalf("RunCleanup error: %v", err)
	}
	if len(purged) != 1 || purged[0] != "agents/a/config/old.json" {
		t.Fatalf("purged = %v", purged)
	}
	if _, err := os.Stat(trashFile); !os.IsNotExist(err) {
		t.Error("expired trash file should be gone")
	}
	if s := strings.TrimSpace(gitOut(t, repo, "status", "--porcelain")); s != "" {
		t.Errorf("worktree should be clean after cleanup commit, got %q", s)
	}
}
