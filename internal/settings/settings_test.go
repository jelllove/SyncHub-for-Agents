package settings

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qinqingxu/synchub-for-agents/internal/cli"
	"github.com/qinqingxu/synchub-for-agents/internal/config"
)

func setupHome(t *testing.T) string {
	t.Helper()
	home := filepath.Join(t.TempDir(), ".synchub")
	if err := os.MkdirAll(cli.ProvidersDir(home), 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(cli.ProvidersDir(home), "demo.yaml"),
		[]byte("name: demo\nconfig:\n  paths:\n    linux: ~/.demo\n  include:\n    - settings.json\n  exclude:\n    - \"**/secret*\"\n"), 0o644)
	if err := config.Save(cli.ConfigPath(home), config.Config{
		RepoURL:             "https://example.com/data.git",
		SyncIntervalMinutes: 10,
		TrashGraceDays:      30,
		Agents:              map[string]bool{"demo": true},
	}); err != nil {
		t.Fatal(err)
	}
	return home
}

func TestHandlerGETRendersAgents(t *testing.T) {
	home := setupHome(t)
	srv := httptest.NewServer(Handler(home))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	s := string(body)
	if !strings.Contains(s, "demo") {
		t.Error("page should list the demo agent")
	}
	if !strings.Contains(s, "https://example.com/data.git") {
		t.Error("page should show the repo URL")
	}
	if !strings.Contains(s, "**/secret*") {
		t.Error("page should show the exclude rule")
	}
}

func TestHandlerSaveUpdatesConfig(t *testing.T) {
	home := setupHome(t)
	srv := httptest.NewServer(Handler(home))
	defer srv.Close()

	form := url.Values{}
	form.Set("repo_url", "https://example.com/new.git")
	form.Set("sync_interval_minutes", "15")
	form.Set("trash_grace_days", "45")
	// no agent_demo field => demo becomes disabled

	resp, err := http.PostForm(srv.URL+"/save", form)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status after save = %d", resp.StatusCode)
	}

	cfg, err := config.Load(cli.ConfigPath(home))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.RepoURL != "https://example.com/new.git" {
		t.Errorf("repo url = %q", cfg.RepoURL)
	}
	if cfg.SyncIntervalMinutes != 15 {
		t.Errorf("interval = %d", cfg.SyncIntervalMinutes)
	}
	if cfg.TrashGraceDays != 45 {
		t.Errorf("grace = %d", cfg.TrashGraceDays)
	}
	if cfg.Agents["demo"] {
		t.Error("demo should be disabled after saving with its checkbox unchecked")
	}
}
