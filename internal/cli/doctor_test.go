package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/qinqingxu/synchub-for-agents/internal/config"
)

func TestDoctorReportsMissingProfileWithoutCreatingConfig(t *testing.T) {
	home := t.TempDir()
	checks := Doctor(home, runtime.GOOS)
	found := false
	for _, check := range checks {
		if check.Name == "configuration" {
			found = true
			if check.OK || check.Message == "" {
				t.Fatalf("missing config check = %+v", check)
			}
		}
	}
	if !found {
		t.Fatal("configuration check missing")
	}
	if _, err := os.Stat(ConfigPath(home)); !os.IsNotExist(err) {
		t.Fatalf("doctor created config: %v", err)
	}
}

func TestDoctorChecksLocalProfileWithoutCredentialsOrNetwork(t *testing.T) {
	userHome := t.TempDir()
	t.Setenv("HOME", userHome)
	t.Setenv("USERPROFILE", userHome)
	home := filepath.Join(userHome, ".synchub")
	cfg := config.Default(nil)
	cfg.RepoURL = "git@github.com:example/synthetic.git"
	if err := config.Save(ConfigPath(home), cfg); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(RepoDir(home), ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(ConfigPath(home))
	if err != nil {
		t.Fatal(err)
	}
	for _, check := range Doctor(home, runtime.GOOS) {
		if !check.OK {
			t.Fatalf("local check failed: %+v", check)
		}
	}
	after, err := os.ReadFile(ConfigPath(home))
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("doctor changed config: %v", err)
	}
}

func TestCredentialHelperUsesSelectedProfile(t *testing.T) {
	userHome := t.TempDir()
	t.Setenv("HOME", userHome)
	t.Setenv("USERPROFILE", userHome)
	defaultProfile := filepath.Join(userHome, ".synchub")
	if err := os.MkdirAll(defaultProfile, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(AuthMetadataPath(defaultProfile), []byte("not valid metadata"), 0o600); err != nil {
		t.Fatal(err)
	}
	selected := t.TempDir()
	t.Setenv("SYNCHUB_CREDENTIAL_HOME", selected)
	var out bytes.Buffer
	if err := RunCredential("get", strings.NewReader("protocol=https\nhost=github.com\n\n"), &out); err != nil {
		t.Fatal(err)
	}
	if out.String() != "quit=1\n\n" {
		t.Fatalf("helper response = %q", out.String())
	}
	t.Setenv("SYNCHUB_CREDENTIAL_HOME", "relative-profile")
	if err := RunCredential("get", strings.NewReader(""), &out); err == nil {
		t.Fatal("relative credential profile accepted")
	}
}
