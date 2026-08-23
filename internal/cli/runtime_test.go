package cli

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/qinqingxu/synchub-for-agents/internal/config"
	"github.com/qinqingxu/synchub-for-agents/internal/provider"
	"github.com/qinqingxu/synchub-for-agents/internal/resource"
)

func TestBuildResourceSpecsSkipsDisabledCategoriesAndNormalizesV1(t *testing.T) {
	providers := []provider.Provider{
		{
			Name: "claude",
			Config: provider.ConfigSpec{
				Paths:    map[string]string{"linux": "~/.claude"},
				Include:  []string{"settings.json"},
				Sessions: []string{"sessions/**/*.jsonl"},
				Exclude:  []string{"**/*token*"},
			},
			Secrets: provider.SecretSpec{KeyPatterns: []string{"token"}},
		},
	}
	cfg := config.Config{
		Agents: map[string]bool{"claude": true},
		Categories: map[string]map[string]bool{
			"claude": {string(resource.CategorySessions): false},
		},
	}

	specs, err := BuildResourceSpecs(cfg, providers, "linux", "/home/alice")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := specs["claude/legacy-sessions"]; ok {
		t.Fatal("disabled category should be excluded")
	}
	spec, ok := specs["claude/legacy-config"]
	if !ok {
		t.Fatal("legacy config spec missing")
	}
	if spec.Layout != resource.LayoutLegacy ||
		spec.Root != "/home/alice/.claude" ||
		len(spec.Targets) != 1 ||
		spec.Targets[0] != "/home/alice/.claude" {
		t.Fatalf("legacy spec = %#v", spec)
	}
	if len(spec.KeyPatterns) != 1 || spec.KeyPatterns[0] != "token" {
		t.Fatalf("key patterns = %#v", spec.KeyPatterns)
	}
}

func TestBuildResourceSpecsCoalescesSharedSkills(t *testing.T) {
	providers := []provider.Provider{
		{
			Name: "claude",
			Resources: []resource.Declaration{{
				ID:       "skills",
				Category: resource.CategorySkills,
				Paths:    map[string]string{"windows": `%USERPROFILE%\.claude\skills`},
				Strategy: resource.StrategySourceTree,
				SharedAs: "common-skills",
			}},
		},
		{
			Name: "common",
			Resources: []resource.Declaration{{
				ID:       "skills",
				Category: resource.CategorySkills,
				Paths:    map[string]string{"windows": `%USERPROFILE%\.agents\skills`},
				Strategy: resource.StrategySourceTree,
				SharedAs: "common-skills",
			}},
		},
	}
	cfg := config.Config{Agents: map[string]bool{"claude": true, "common": true}}

	specs, err := BuildResourceSpecs(cfg, providers, "windows", `C:\Users\alice`)
	if err != nil {
		t.Fatal(err)
	}
	spec, ok := specs["common/common-skills"]
	if !ok {
		t.Fatal("shared spec missing")
	}
	if spec.Provider != "common" || spec.SharedAs != "common-skills" || spec.Layout != resource.LayoutPortable {
		t.Fatalf("shared spec = %#v", spec)
	}
	if len(spec.Targets) != 2 {
		t.Fatalf("targets = %#v", spec.Targets)
	}
}

func TestBuildResourceSpecsBuildsCustomStructuredResources(t *testing.T) {
	cfg := config.Config{
		Agents: map[string]bool{"claude": true},
		CustomResources: []config.CustomResource{{
			ID:       "notes",
			Category: resource.CategoryConfig,
			Paths:    map[string]string{"windows": `%USERPROFILE%\notes`},
			Targets:  map[string]string{"windows": `%USERPROFILE%\Documents\notes`},
			Include:  []string{"settings.json"},
			Strategy: resource.StrategyStructuredMerge,
		}},
	}

	specs, err := BuildResourceSpecs(cfg, nil, "windows", `C:\Users\alice`)
	if err != nil {
		t.Fatal(err)
	}
	spec, ok := specs["custom/notes"]
	if !ok {
		t.Fatal("custom spec missing")
	}
	if spec.Provider != "custom" ||
		spec.Layout != resource.LayoutPortable ||
		spec.Root != `C:\Users\alice\notes` ||
		len(spec.Targets) != 1 ||
		spec.Targets[0] != `C:\Users\alice\Documents\notes` ||
		spec.Transformer != "generic-safe" ||
		spec.Installer != "" {
		t.Fatalf("custom spec = %#v", spec)
	}
}

func TestBuildResourceSpecsKeepsCustomResourceWithoutCurrentPlatformTarget(t *testing.T) {
	cfg := config.Config{
		Agents: map[string]bool{"claude": true},
		CustomResources: []config.CustomResource{{
			ID:       "notes",
			Category: resource.CategoryInstructions,
			Paths:    map[string]string{"windows": `%USERPROFILE%\notes`},
			Targets:  map[string]string{"linux": "~/notes"},
			Include:  []string{"**/*.md"},
			Strategy: resource.StrategyTextTree,
		}},
	}

	specs, err := BuildResourceSpecs(cfg, nil, "windows", `C:\Users\alice`)
	if err != nil {
		t.Fatal(err)
	}
	spec, ok := specs["custom/notes"]
	if !ok {
		t.Fatal("custom spec missing")
	}
	if len(spec.Targets) != 0 {
		t.Fatalf("targets = %#v, want empty without current-platform mapping", spec.Targets)
	}
}

func TestBuildResourceSpecsRejectsUnsafeIdentifiers(t *testing.T) {
	providers := []provider.Provider{
		{
			Name: "foo/bar",
			Resources: []resource.Declaration{{
				ID:       "settings",
				Category: resource.CategoryConfig,
				Paths:    map[string]string{"linux": "~/.demo"},
				Include:  []string{"settings.json"},
				Strategy: resource.StrategyFileTree,
			}},
		},
	}

	_, err := BuildResourceSpecs(config.Config{Agents: map[string]bool{"foo/bar": true}}, providers, "linux", "/home/alice")
	if err == nil || !testingErrorContains(err, "invalid provider name") {
		t.Fatalf("BuildResourceSpecs() error = %v", err)
	}
}

func testingErrorContains(err error, want string) bool {
	return err != nil && strings.Contains(err.Error(), want)
}

func TestResolveRepoDirUsesConfiguredPath(t *testing.T) {
	home := filepath.Join(`C:\Users\alice`, ".synchub")
	cfg := config.Config{RepoDir: `${HOME}\my-sync`}
	resolved, err := ResolveRepoDir(home, cfg, "windows", `C:\Users\alice`)
	if err != nil {
		t.Fatal(err)
	}
	if resolved != `C:\Users\alice\my-sync` {
		t.Fatalf("resolved = %q", resolved)
	}
}

func TestResolveRepoDirFallsBackToDefault(t *testing.T) {
	home := filepath.Join(`C:\Users\alice`, ".synchub")
	resolved, err := ResolveRepoDir(home, config.Config{}, "windows", `C:\Users\alice`)
	if err != nil {
		t.Fatal(err)
	}
	if resolved != filepath.Join(home, "repo") {
		t.Fatalf("resolved = %q", resolved)
	}
}
