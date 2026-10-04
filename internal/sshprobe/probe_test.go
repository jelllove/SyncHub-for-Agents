package sshprobe

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type fakeRunner struct {
	calls []string
}

func (runner *fakeRunner) Run(_ context.Context, name string, args ...string) (string, string, error) {
	runner.calls = append(runner.calls, name+" "+strings.Join(args, " "))
	switch name {
	case "ssh":
		if len(args) > 0 && args[0] == "-G" {
			return "hostname github.com\nuser git\nidentityfile ~/.ssh/jelllove\n", "", nil
		}
		return "", "OpenSSH_9.0", nil
	case "ssh-add":
		return "ssh-ed25519 AAAA alice@example.com\n", "", nil
	case "git":
		return "abc123\tHEAD\n", "", nil
	default:
		return "", "", nil
	}
}

func TestCheckVerifiesUnattendedRepositoryAccess(t *testing.T) {
	runner := &fakeRunner{}
	repositoryURL := "git@github-jelllove:jelllove/agents-backup.git"
	status, err := Check(context.Background(), runner, repositoryURL)
	if err != nil {
		t.Fatal(err)
	}
	if !status.SSHAvailable || !status.AgentHasKeys || !status.RepositoryAccess {
		t.Fatalf("status = %#v", status)
	}
	if len(status.IdentityFiles) != 1 || status.IdentityFiles[0] != "~/.ssh/jelllove" {
		t.Fatalf("identity files = %#v", status.IdentityFiles)
	}
	calls := strings.Join(runner.calls, "\n")
	for _, wanted := range []string{
		"ssh -G -l git github-jelllove",
		repositoryURL,
		"BatchMode=yes",
		"StrictHostKeyChecking=yes",
		"ls-remote",
	} {
		if !strings.Contains(calls, wanted) {
			t.Fatalf("commands missing %q:\n%s", wanted, calls)
		}
	}
}

type nonGitHubAliasRunner struct {
	gitCalled bool
}

func (runner *nonGitHubAliasRunner) Run(
	_ context.Context,
	name string,
	args ...string,
) (string, string, error) {
	if name == "ssh" && len(args) > 0 && args[0] == "-G" {
		return "hostname evil.example\nuser git\n", "", nil
	}
	if name == "git" {
		runner.gitCalled = true
	}
	return "", "", nil
}

func TestCheckRejectsAliasResolvingOutsideGitHub(t *testing.T) {
	runner := &nonGitHubAliasRunner{}
	status, err := Check(
		context.Background(),
		runner,
		"git@github-jelllove:jelllove/agents-backup.git",
	)
	if err != nil {
		t.Fatal(err)
	}
	if status.RepositoryAccess ||
		!strings.Contains(status.Message, "must resolve to github.com") ||
		runner.gitCalled {
		t.Fatalf("status = %#v, git called = %v", status, runner.gitCalled)
	}
}

type nonGitUserRunner struct {
	gitCalled bool
}

func (runner *nonGitUserRunner) Run(
	_ context.Context,
	name string,
	args ...string,
) (string, string, error) {
	if name == "ssh" && len(args) > 0 && args[0] == "-G" {
		return "hostname github.com\nuser deploy\n", "", nil
	}
	if name == "git" {
		runner.gitCalled = true
	}
	return "", "", nil
}

func TestCheckRejectsEffectiveUserOtherThanGit(t *testing.T) {
	runner := &nonGitUserRunner{}
	status, err := Check(
		context.Background(),
		runner,
		"git@github-jelllove:jelllove/agents-backup.git",
	)
	if err != nil {
		t.Fatal(err)
	}
	if status.RepositoryAccess ||
		!strings.Contains(status.Message, "must use the git user") ||
		runner.gitCalled {
		t.Fatalf("status = %#v, git called = %v", status, runner.gitCalled)
	}
}

type keygenRunner struct{}

func (keygenRunner) Run(_ context.Context, name string, args ...string) (string, string, error) {
	for index, arg := range args {
		if arg == "-f" && index+1 < len(args) {
			return "", "", os.WriteFile(args[index+1]+".pub", []byte("ssh-ed25519 AAAA generated\n"), 0o644)
		}
	}
	return "", "", nil
}

func TestGenerateKeyReturnsOnlyPublicKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "id_synchub")
	publicKey, err := GenerateKey(context.Background(), keygenRunner{}, path, "alice@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if publicKey != "ssh-ed25519 AAAA generated" {
		t.Fatalf("public key = %q", publicKey)
	}
}
