package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/qinqingxu/synchub-for-agents/internal/config"
	"github.com/qinqingxu/synchub-for-agents/internal/gitclient"
	"github.com/qinqingxu/synchub-for-agents/internal/provider"
	"gopkg.in/yaml.v3"
)

func gitCmd(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func bareRemote(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	bare := filepath.Join(root, "remote.git")
	gitCmd(t, root, "init", "--bare", "-b", "main", bare)
	seed := filepath.Join(root, "seed")
	gitCmd(t, root, "clone", bare, seed)
	gitCmd(t, seed, "config", "user.email", "s@e.com")
	gitCmd(t, seed, "config", "user.name", "seed")
	os.WriteFile(filepath.Join(seed, "manifest.json"), []byte("{}\n"), 0o644)
	gitCmd(t, seed, "add", ".")
	gitCmd(t, seed, "commit", "-m", "seed")
	gitCmd(t, seed, "push", "origin", "main")
	return bare
}

func TestRunSyncPushesConfig(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	home := filepath.Join(t.TempDir(), ".synchub")
	if err := os.MkdirAll(ProvidersDir(home), 0o755); err != nil {
		t.Fatal(err)
	}
	bare := bareRemote(t)

	// Clone repo workspace and set identity.
	client := &gitclient.Client{Dir: RepoDir(home)}
	if err := client.Clone(bare, RepoDir(home)); err != nil {
		t.Fatalf("clone: %v", err)
	}
	gitCmd(t, RepoDir(home), "config", "user.email", "m@e.com")
	gitCmd(t, RepoDir(home), "config", "user.name", "machine")

	// A user provider pointing at a temp agent root.
	agentRoot := t.TempDir()
	os.WriteFile(filepath.Join(agentRoot, "settings.json"), []byte(`{"theme":"dark"}`), 0o644)
	p := provider.Provider{
		Name: "demo",
		Config: provider.ConfigSpec{
			Paths:   map[string]string{runtime.GOOS: agentRoot},
			Include: []string{"settings.json"},
		},
	}
	data, _ := yaml.Marshal(p)
	if err := os.WriteFile(filepath.Join(ProvidersDir(home), "demo.yaml"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	// Config enabling only demo.
	cfg := config.Config{
		RepoURL:             bare,
		SyncIntervalMinutes: 10,
		TrashGraceDays:      30,
		Agents:              map[string]bool{"demo": true},
	}
	if err := config.Save(ConfigPath(home), cfg); err != nil {
		t.Fatal(err)
	}

	res, err := RunSync(home, runtime.GOOS)
	if err != nil {
		t.Fatalf("RunSync error: %v", err)
	}
	if !res.Pushed {
		t.Error("expected a push")
	}
	if _, err := os.Stat(filepath.Join(RepoDir(home), "agents", "demo", "config", "settings.json")); err != nil {
		t.Errorf("repo should contain the pushed file: %v", err)
	}
}

func TestRunSyncUsesUserHomeForProviderPaths(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	userHome := t.TempDir()
	dataHome := filepath.Join(userHome, ".synchub")
	if err := os.MkdirAll(ProvidersDir(dataHome), 0o755); err != nil {
		t.Fatal(err)
	}
	bare := bareRemote(t)
	client := &gitclient.Client{Dir: RepoDir(dataHome)}
	if err := client.Clone(bare, RepoDir(dataHome)); err != nil {
		t.Fatalf("clone: %v", err)
	}
	gitCmd(t, RepoDir(dataHome), "config", "user.email", "m@e.com")
	gitCmd(t, RepoDir(dataHome), "config", "user.name", "machine")

	claudeRoot := filepath.Join(userHome, ".claude")
	if err := os.MkdirAll(claudeRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(claudeRoot, "settings.json"), []byte(`{"theme":"dark"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{
		RepoURL:             bare,
		SyncIntervalMinutes: 10,
		TrashGraceDays:      30,
		Agents:              map[string]bool{"claude": true},
	}
	if err := config.Save(ConfigPath(dataHome), cfg); err != nil {
		t.Fatal(err)
	}

	result, err := runSyncWithUserHome(dataHome, runtime.GOOS, userHome)
	if err != nil {
		t.Fatalf("runSyncWithUserHome error: %v", err)
	}
	if !result.Pushed {
		t.Fatal("expected provider file from the user home to be pushed")
	}
	want := filepath.Join(RepoDir(dataHome), "agents", "claude", "config", "settings.json")
	if _, err := os.Stat(want); err != nil {
		t.Fatalf("synced provider file missing: %v", err)
	}
}
