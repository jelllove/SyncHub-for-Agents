package repository

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qinqingxu/acsync/internal/gitclient"
)

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	command := exec.Command("git", args...)
	command.Dir = dir
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, output)
	}
	return string(output)
}

func TestSetupInitializesEmptyRemoteAndIsIdempotent(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	root := t.TempDir()
	remote := filepath.Join(root, "remote.git")
	runGit(t, root, "init", "--bare", "-b", "main", remote)
	work := filepath.Join(root, "work")
	setup := &Setup{
		Client: &gitclient.Client{},
		Name:   "AgentConfigSync Test",
		Email:  "test@example.com",
	}

	if err := setup.Initialize(remote, work); err != nil {
		t.Fatal(err)
	}
	attributes, err := os.ReadFile(filepath.Join(work, ".gitattributes"))
	if err != nil {
		t.Fatal(err)
	}
	if string(attributes) != "* -text\n" {
		t.Fatalf(".gitattributes = %q", attributes)
	}
	if branch := strings.TrimSpace(runGit(t, work, "branch", "--show-current")); branch != "main" {
		t.Fatalf("branch = %q, want main", branch)
	}
	if refs := strings.TrimSpace(runGit(t, root, "--git-dir", remote, "show-ref", "refs/heads/main")); refs == "" {
		t.Fatal("remote main branch was not created")
	}
	firstHead := strings.TrimSpace(runGit(t, work, "rev-parse", "HEAD"))

	if err := setup.Initialize(remote, work); err != nil {
		t.Fatal(err)
	}
	secondHead := strings.TrimSpace(runGit(t, work, "rev-parse", "HEAD"))
	if secondHead != firstHead {
		t.Fatalf("second setup created another commit: %s != %s", secondHead, firstHead)
	}
}

func TestSetupRejectsExistingWorktreeForDifferentRemote(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	root := t.TempDir()
	firstRemote := filepath.Join(root, "first.git")
	secondRemote := filepath.Join(root, "second.git")
	runGit(t, root, "init", "--bare", "-b", "main", firstRemote)
	runGit(t, root, "init", "--bare", "-b", "main", secondRemote)
	work := filepath.Join(root, "work")
	setup := &Setup{Client: &gitclient.Client{}, Name: "Test", Email: "test@example.com"}
	if err := setup.Initialize(firstRemote, work); err != nil {
		t.Fatal(err)
	}
	if err := setup.Initialize(secondRemote, work); err == nil {
		t.Fatal("existing worktree accepted a different remote")
	}
}
