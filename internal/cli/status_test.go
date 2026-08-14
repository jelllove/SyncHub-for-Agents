package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/qinqingxu/acsync/internal/config"
	"github.com/qinqingxu/acsync/internal/provider"
	"gopkg.in/yaml.v3"
)

func TestRunStatusCountsPending(t *testing.T) {
	home := filepath.Join(t.TempDir(), ".acsync")
	if err := os.MkdirAll(ProvidersDir(home), 0o755); err != nil {
		t.Fatal(err)
	}

	agentRoot := t.TempDir()
	os.WriteFile(filepath.Join(agentRoot, "settings.json"), []byte(`{"a":1}`), 0o644)
	p := provider.Provider{
		Name: "demo",
		Config: provider.ConfigSpec{
			Paths:   map[string]string{runtime.GOOS: agentRoot},
			Include: []string{"settings.json"},
		},
	}
	data, _ := yaml.Marshal(p)
	os.WriteFile(filepath.Join(ProvidersDir(home), "demo.yaml"), data, 0o644)

	cfg := config.Config{
		RepoURL: "https://example.com/data.git",
		Agents:  map[string]bool{"demo": true},
	}
	if err := config.Save(ConfigPath(home), cfg); err != nil {
		t.Fatal(err)
	}

	st, err := RunStatus(home, runtime.GOOS)
	if err != nil {
		t.Fatalf("RunStatus error: %v", err)
	}
	if st.RepoURL != "https://example.com/data.git" {
		t.Errorf("repo url = %q", st.RepoURL)
	}
	if len(st.EnabledAgents) != 1 || st.EnabledAgents[0] != "demo" {
		t.Errorf("enabled agents = %v", st.EnabledAgents)
	}
	if st.PendingActions != 1 {
		t.Errorf("pending = %d, want 1", st.PendingActions)
	}
	if !st.LastSync.IsZero() {
		t.Errorf("last sync should be zero when no state file")
	}
}

func TestRunStatusUsesUserHomeForProviderPaths(t *testing.T) {
	userHome := t.TempDir()
	dataHome := filepath.Join(userHome, ".acsync")
	if err := os.MkdirAll(RepoDir(dataHome), 0o755); err != nil {
		t.Fatal(err)
	}
	claudeRoot := filepath.Join(userHome, ".claude")
	if err := os.MkdirAll(claudeRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(claudeRoot, "settings.json"), []byte(`{"theme":"dark"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := config.Save(ConfigPath(dataHome), config.Config{
		RepoURL: "https://example.com/data.git",
		Agents:  map[string]bool{"claude": true},
	}); err != nil {
		t.Fatal(err)
	}

	status, err := runStatusWithUserHome(dataHome, runtime.GOOS, userHome)
	if err != nil {
		t.Fatal(err)
	}
	if status.PendingActions != 2 {
		t.Fatalf("pending actions = %d, want legacy and portable settings", status.PendingActions)
	}
}
