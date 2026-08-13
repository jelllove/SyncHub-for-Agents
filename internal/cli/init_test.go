package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/qinqingxu/acsync/internal/config"
)

type fakeInitializer struct{ called bool }

func (f *fakeInitializer) Initialize(url, dir string) error {
	f.called = true
	return os.MkdirAll(dir, 0o755)
}

func TestRunInitScaffolds(t *testing.T) {
	home := filepath.Join(t.TempDir(), ".acsync")
	fc := &fakeInitializer{}

	if err := RunInit(home, "https://github.com/me/data", fc); err != nil {
		t.Fatalf("RunInit error: %v", err)
	}
	if !fc.called {
		t.Error("cloner should have been called")
	}
	cfg, err := config.Load(ConfigPath(home))
	if err != nil {
		t.Fatalf("config not written: %v", err)
	}
	if cfg.RepoURL != "https://github.com/me/data.git" {
		t.Errorf("repo url = %q", cfg.RepoURL)
	}
	for _, name := range []string{"claude", "copilot", "gemini", "cursor"} {
		if !cfg.Agents[name] {
			t.Errorf("agent %q should be enabled by default", name)
		}
	}
	if _, err := os.Stat(ProvidersDir(home)); err != nil {
		t.Errorf("providers dir missing: %v", err)
	}
	attr, err := os.ReadFile(filepath.Join(RepoDir(home), ".gitattributes"))
	if err != nil {
		t.Fatalf(".gitattributes not written: %v", err)
	}
	if string(attr) != "* -text\n" {
		t.Errorf(".gitattributes = %q, want %q", string(attr), "* -text\n")
	}
}

func TestRunInitRejectsRepositoryURLWithEmbeddedCredential(t *testing.T) {
	home := filepath.Join(t.TempDir(), ".acsync")
	initializer := &fakeInitializer{}
	err := RunInit(home, "https://secret@github.com/me/data.git", initializer)
	if err == nil {
		t.Fatal("embedded credential unexpectedly accepted")
	}
	if initializer.called {
		t.Fatal("repository initializer called for unsafe URL")
	}
}
