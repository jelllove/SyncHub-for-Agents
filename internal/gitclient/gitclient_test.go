package gitclient

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func mustGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}

func TestOAuthNetworkCommandUsesScopedNoninteractiveCredentialHelper(t *testing.T) {
	t.Setenv("GIT_TRACE", "1")
	t.Setenv("GIT_CURL_VERBOSE", "1")
	executable := filepath.Join(t.TempDir(), "AgentConfigSync.exe")
	client := &Client{
		AuthMode:   AuthOAuth,
		Executable: executable,
	}

	command, err := client.command("clone", "https://github.com/acme/sync.git", "work")
	if err != nil {
		t.Fatal(err)
	}
	arguments := strings.Join(command.Args, "\n")
	for _, wanted := range []string{
		"credential.helper=",
		"credential.https://github.com.helper=",
		"--git-credential",
		"credential.interactive=false",
	} {
		if !strings.Contains(arguments, wanted) {
			t.Fatalf("command arguments missing %q:\n%s", wanted, arguments)
		}
	}
	environment := strings.Join(command.Env, "\n")
	if !strings.Contains(environment, "GIT_TERMINAL_PROMPT=0") {
		t.Fatalf("environment missing GIT_TERMINAL_PROMPT=0:\n%s", environment)
	}
	if strings.Contains(environment, "GIT_TRACE=") || strings.Contains(environment, "GIT_CURL_VERBOSE=") {
		t.Fatalf("trace environment leaked to Git:\n%s", environment)
	}
}

// setupBareRemote creates a bare repo with one initial commit and returns its path.
func setupBareRemote(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	bare := filepath.Join(root, "remote.git")
	mustGit(t, root, "init", "--bare", "-b", "main", bare)

	// Seed the bare repo via a temporary working clone.
	seed := filepath.Join(root, "seed")
	mustGit(t, root, "clone", bare, seed)
	mustGit(t, seed, "config", "user.email", "t@example.com")
	mustGit(t, seed, "config", "user.name", "tester")
	if err := os.WriteFile(filepath.Join(seed, "README.md"), []byte("seed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGit(t, seed, "add", ".")
	mustGit(t, seed, "commit", "-m", "seed")
	mustGit(t, seed, "push", "origin", "main")
	return bare
}

func TestCloneAndHasChanges(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	bare := setupBareRemote(t)
	work := filepath.Join(t.TempDir(), "work")

	c := &Client{Dir: work}
	if err := c.Clone(bare, work); err != nil {
		t.Fatalf("Clone error: %v", err)
	}
	mustGit(t, work, "config", "user.email", "t@example.com")
	mustGit(t, work, "config", "user.name", "tester")

	changed, err := c.HasChanges()
	if err != nil {
		t.Fatalf("HasChanges error: %v", err)
	}
	if changed {
		t.Error("fresh clone should have no changes")
	}

	if err := os.WriteFile(filepath.Join(work, "new.txt"), []byte("hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	changed, err = c.HasChanges()
	if err != nil {
		t.Fatalf("HasChanges error: %v", err)
	}
	if !changed {
		t.Error("expected changes after writing a new file")
	}
}

func TestCommitPushPullRoundTrip(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	bare := setupBareRemote(t)

	// Machine A clones, commits, pushes.
	workA := filepath.Join(t.TempDir(), "A")
	a := &Client{Dir: workA}
	if err := a.Clone(bare, workA); err != nil {
		t.Fatal(err)
	}
	mustGit(t, workA, "config", "user.email", "a@example.com")
	mustGit(t, workA, "config", "user.name", "A")
	if err := os.WriteFile(filepath.Join(workA, "fromA.txt"), []byte("A\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := a.AddAll(); err != nil {
		t.Fatalf("AddAll: %v", err)
	}
	if err := a.Commit("add fromA"); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	ahead, err := a.AheadOfUpstream()
	if err != nil {
		t.Fatal(err)
	}
	if !ahead {
		t.Fatal("local commit should be ahead before push")
	}
	if err := a.Push(); err != nil {
		t.Fatalf("Push: %v", err)
	}
	ahead, err = a.AheadOfUpstream()
	if err != nil {
		t.Fatal(err)
	}
	if ahead {
		t.Fatal("branch should not be ahead after push")
	}

	// Machine B clones and should see A's file after pull.
	workB := filepath.Join(t.TempDir(), "B")
	b := &Client{Dir: workB}
	if err := b.Clone(bare, workB); err != nil {
		t.Fatal(err)
	}
	mustGit(t, workB, "config", "user.email", "b@example.com")
	mustGit(t, workB, "config", "user.name", "B")
	if err := b.PullRebase(); err != nil {
		t.Fatalf("PullRebase: %v", err)
	}
	if _, err := os.Stat(filepath.Join(workB, "fromA.txt")); err != nil {
		t.Errorf("B should have fromA.txt after pull: %v", err)
	}
}
