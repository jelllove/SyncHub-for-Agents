package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qinqingxu/synchub-for-agents/internal/resource"
)

func TestDefault(t *testing.T) {
	c := Default([]string{"claude", "copilot"})
	if c.SyncIntervalMinutes != 10 {
		t.Errorf("interval = %d, want 10", c.SyncIntervalMinutes)
	}
	if c.TrashGraceDays != 30 {
		t.Errorf("grace = %d, want 30", c.TrashGraceDays)
	}
	if !c.Agents["claude"] || !c.Agents["copilot"] {
		t.Errorf("all agents should default to enabled: %+v", c.Agents)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	in := Config{
		RepoURL:             "git@github.com:me/synchub-data.git",
		SyncIntervalMinutes: 15,
		TrashGraceDays:      7,
		Agents:              map[string]bool{"claude": true, "gemini": false},
		Categories: map[string]map[string]bool{
			"claude": {string(resource.CategoryPlugins): false},
		},
		CustomResources: []CustomResource{
			{
				ID:       "notes",
				Category: resource.CategoryInstructions,
				Paths:    map[string]string{"windows": "%USERPROFILE%\\notes"},
				Targets:  map[string]string{"windows": "%USERPROFILE%\\notes"},
				Include:  []string{"**/*.md"},
				Strategy: resource.StrategyTextTree,
			},
		},
	}
	if err := Save(path, in); err != nil {
		t.Fatalf("Save: %v", err)
	}
	out, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if out.RepoURL != in.RepoURL || out.SyncIntervalMinutes != 15 || out.TrashGraceDays != 7 {
		t.Errorf("round trip mismatch: %+v", out)
	}
	if out.Agents["claude"] != true || out.Agents["gemini"] != false {
		t.Errorf("agents mismatch: %+v", out.Agents)
	}
	if out.Categories["claude"][string(resource.CategoryPlugins)] {
		t.Errorf("categories mismatch: %+v", out.Categories)
	}
	if len(out.CustomResources) != 1 || out.CustomResources[0].ID != "notes" {
		t.Errorf("custom resources mismatch: %+v", out.CustomResources)
	}
}

func TestLoadMissingIsError(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "nope.yaml")); err == nil {
		t.Fatal("expected error loading missing config")
	}
}

func TestEnabledAgents(t *testing.T) {
	c := Config{Agents: map[string]bool{"claude": true, "gemini": false, "copilot": true}}
	got := c.EnabledAgents()
	if len(got) != 2 {
		t.Fatalf("expected 2 enabled, got %v", got)
	}
}

func TestLoadMigratesVSCodeProviderIntoOlderConfig(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "config.yaml")
	data := []byte("agents:\n  claude: false\n")
	if err := os.WriteFile(filename, data, 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := Load(filename)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Agents["vscode-copilot"] {
		t.Fatal("new VS Code provider should default to enabled during config migration")
	}
	if got.Agents["claude"] {
		t.Fatal("migration must preserve explicit agent settings")
	}
}

func TestLoadV1DefaultsSafeCategoriesWithoutReenablingAgent(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(filename, []byte(`
version: 1
agents:
  claude: false
`), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(filename)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Agents["claude"] {
		t.Fatal("disabled provider was re-enabled")
	}
	if cfg.CategoryEnabled("claude", resource.CategorySkills) {
		t.Fatal("disabled provider should keep all categories disabled")
	}
}

func TestLoadV2EnablesNewCommonProvider(t *testing.T) {
	filename := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(filename, []byte(`
version: 2
agents:
  claude: true
`), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load(filename)
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.Agents["common"] {
		t.Fatal("new common provider should default enabled during v3 migration")
	}
}

func TestCategoryEnabledDefaultsWhenProviderEnabled(t *testing.T) {
	cfg := Config{
		Agents: map[string]bool{"claude": true},
	}
	if !cfg.CategoryEnabled("claude", resource.CategorySkills) {
		t.Fatal("missing category should default enabled when provider is enabled")
	}
}

func TestExplicitCategoryDisableWins(t *testing.T) {
	cfg := Config{
		Agents: map[string]bool{"claude": true},
		Categories: map[string]map[string]bool{
			"claude": {string(resource.CategoryPlugins): false},
		},
	}
	if cfg.CategoryEnabled("claude", resource.CategoryPlugins) {
		t.Fatal("explicit category disable was ignored")
	}
}

func TestSaveRejectsInvalidCustomResources(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	tests := []struct {
		name     string
		resource CustomResource
	}{
		{
			name: "missing-id",
			resource: CustomResource{
				Category: resource.CategoryInstructions,
				Paths:    map[string]string{"windows": "%USERPROFILE%\\notes"},
				Targets:  map[string]string{"windows": "%USERPROFILE%\\notes"},
				Strategy: resource.StrategyTextTree,
			},
		},
		{
			name: "bad-category",
			resource: CustomResource{
				ID:       "notes",
				Category: resource.Category("bad"),
				Paths:    map[string]string{"windows": "%USERPROFILE%\\notes"},
				Targets:  map[string]string{"windows": "%USERPROFILE%\\notes"},
				Strategy: resource.StrategyTextTree,
			},
		},
		{
			name: "bad-strategy",
			resource: CustomResource{
				ID:       "notes",
				Category: resource.CategoryInstructions,
				Paths:    map[string]string{"windows": "%USERPROFILE%\\notes"},
				Targets:  map[string]string{"windows": "%USERPROFILE%\\notes"},
				Strategy: resource.Strategy("bad"),
			},
		},
		{
			name: "install-manifest",
			resource: CustomResource{
				ID:       "notes",
				Category: resource.CategoryPlugins,
				Paths:    map[string]string{"windows": "%USERPROFILE%\\notes"},
				Targets:  map[string]string{"windows": "%USERPROFILE%\\notes"},
				Strategy: resource.StrategyInstallManifest,
			},
		},
		{
			name: "missing-paths",
			resource: CustomResource{
				ID:       "notes",
				Category: resource.CategoryInstructions,
				Targets:  map[string]string{"windows": "%USERPROFILE%\\notes"},
				Strategy: resource.StrategyTextTree,
			},
		},
		{
			name: "missing-targets",
			resource: CustomResource{
				ID:       "notes",
				Category: resource.CategoryInstructions,
				Paths:    map[string]string{"windows": "%USERPROFILE%\\notes"},
				Strategy: resource.StrategyTextTree,
			},
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			cfg := Config{
				Agents:          map[string]bool{"claude": true},
				CustomResources: []CustomResource{tt.resource},
			}
			if err := Save(path, cfg); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
}

func TestSaveRejectsCustomInstallManifestWithIntentionalError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	cfg := Config{
		Agents: map[string]bool{"claude": true},
		CustomResources: []CustomResource{{
			ID:       "notes",
			Category: resource.CategoryPlugins,
			Paths:    map[string]string{"windows": "%USERPROFILE%\\notes"},
			Targets:  map[string]string{"windows": "%USERPROFILE%\\notes"},
			Strategy: resource.StrategyInstallManifest,
		}},
	}

	err := Save(path, cfg)
	want := `custom resource "notes": strategy "install-manifest" is not supported`
	if err == nil || err.Error() != want {
		t.Fatalf("Save() error = %v, want %q", err, want)
	}
}

func TestSaveRejectsDuplicateCustomResourceIDs(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	cfg := Config{
		Agents: map[string]bool{"claude": true},
		CustomResources: []CustomResource{
			{
				ID:       "notes",
				Category: resource.CategoryInstructions,
				Paths:    map[string]string{"windows": "%USERPROFILE%\\notes"},
				Targets:  map[string]string{"windows": "%USERPROFILE%\\notes"},
				Strategy: resource.StrategyTextTree,
			},
			{
				ID:       "notes",
				Category: resource.CategorySkills,
				Paths:    map[string]string{"windows": "%USERPROFILE%\\skills"},
				Targets:  map[string]string{"windows": "%USERPROFILE%\\skills"},
				Strategy: resource.StrategySourceTree,
			},
		},
	}

	err := Save(path, cfg)
	if err == nil || !strings.Contains(err.Error(), `duplicate custom resource id "notes"`) {
		t.Fatalf("Save() error = %v", err)
	}
}
