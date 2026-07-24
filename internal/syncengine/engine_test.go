package syncengine

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/qinqingxu/acsync/internal/gitclient"
)

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func newBareRemote(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	bare := filepath.Join(root, "remote.git")
	git(t, root, "init", "--bare", "-b", "main", bare)
	seed := filepath.Join(root, "seed")
	git(t, root, "clone", bare, seed)
	git(t, seed, "config", "user.email", "s@e.com")
	git(t, seed, "config", "user.name", "seed")
	os.WriteFile(filepath.Join(seed, "manifest.json"), []byte("{}\n"), 0o644)
	git(t, seed, "add", ".")
	git(t, seed, "commit", "-m", "seed")
	git(t, seed, "push", "origin", "main")
	return bare
}

func cloneWorkspace(t *testing.T, bare, dir string) *gitclient.Client {
	t.Helper()
	c := &gitclient.Client{Dir: dir}
	if err := c.Clone(bare, dir); err != nil {
		t.Fatalf("clone: %v", err)
	}
	git(t, dir, "config", "user.email", "m@e.com")
	git(t, dir, "config", "user.name", "machine")
	return c
}

func engineFor(client *gitclient.Client, repoDir, statePath, agentRoot string) *Engine {
	return &Engine{
		Git:       client,
		RepoDir:   repoDir,
		StatePath: statePath,
		Specs: map[string]AgentSpec{
			"demo": {Name: "demo", Root: agentRoot, Include: []string{"settings.json"}},
		},
		PushRetries: 3,
		Now:         func() time.Time { return time.Unix(1000, 0) },
	}
}

func TestSyncOncePropagatesCreateAndDelete(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	bare := newBareRemote(t)

	// Machine A.
	repoA := filepath.Join(t.TempDir(), "repoA")
	clientA := cloneWorkspace(t, bare, repoA)
	rootA := t.TempDir()
	stateA := filepath.Join(t.TempDir(), "stateA.json")
	engA := engineFor(clientA, repoA, stateA, rootA)

	// Machine B.
	repoB := filepath.Join(t.TempDir(), "repoB")
	clientB := cloneWorkspace(t, bare, repoB)
	rootB := t.TempDir()
	stateB := filepath.Join(t.TempDir(), "stateB.json")
	engB := engineFor(clientB, repoB, stateB, rootB)

	// A creates a config file and syncs (push).
	writeFile(t, filepath.Join(rootA, "settings.json"), `{"theme":"dark"}`)
	if _, err := engA.SyncOnce(); err != nil {
		t.Fatalf("A first sync: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repoA, "agents", "demo", "config", "settings.json")); err != nil {
		t.Fatalf("A repo should contain settings.json: %v", err)
	}

	// B syncs (pull) and should receive the file locally.
	if _, err := engB.SyncOnce(); err != nil {
		t.Fatalf("B first sync: %v", err)
	}
	if _, err := os.Stat(filepath.Join(rootB, "settings.json")); err != nil {
		t.Fatalf("B should have pulled settings.json locally: %v", err)
	}

	// A deletes the file and syncs (delete propagation + soft delete).
	if err := os.Remove(filepath.Join(rootA, "settings.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := engA.SyncOnce(); err != nil {
		t.Fatalf("A delete sync: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repoA, ".trash", "files", "agents", "demo", "config", "settings.json")); err != nil {
		t.Fatalf("deleted file should be in A's trash: %v", err)
	}

	// B syncs and should delete its local copy.
	if _, err := engB.SyncOnce(); err != nil {
		t.Fatalf("B delete sync: %v", err)
	}
	if _, err := os.Stat(filepath.Join(rootB, "settings.json")); !os.IsNotExist(err) {
		t.Fatalf("B local settings.json should be deleted, err=%v", err)
	}
}
