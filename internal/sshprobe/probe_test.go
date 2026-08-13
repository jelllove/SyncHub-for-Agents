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
			return "identityfile ~/.ssh/id_ed25519\nidentityfile ~/.ssh/id_rsa\n", "", nil
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
	status, err := Check(context.Background(), runner, "git@github.com:acme/sync.git")
	if err != nil {
		t.Fatal(err)
	}
	if !status.SSHAvailable || !status.AgentHasKeys || !status.RepositoryAccess {
		t.Fatalf("status = %#v", status)
	}
	if len(status.IdentityFiles) != 2 {
		t.Fatalf("identity files = %#v", status.IdentityFiles)
	}
	calls := strings.Join(runner.calls, "\n")
	for _, wanted := range []string{"BatchMode=yes", "StrictHostKeyChecking=yes", "ls-remote"} {
		if !strings.Contains(calls, wanted) {
			t.Fatalf("commands missing %q:\n%s", wanted, calls)
		}
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
	path := filepath.Join(t.TempDir(), "id_acsync")
	publicKey, err := GenerateKey(context.Background(), keygenRunner{}, path, "alice@example.com")
	if err != nil {
		t.Fatal(err)
	}
	if publicKey != "ssh-ed25519 AAAA generated" {
		t.Fatalf("public key = %q", publicKey)
	}
}
