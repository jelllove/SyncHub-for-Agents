package syncengine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/qinqingxu/acsync/internal/portableconfig"
	"github.com/qinqingxu/acsync/internal/resource"
	"github.com/qinqingxu/acsync/internal/state"
)

func TestSplitRemoteSnapshotPreservesUnknownAndDisabledResources(t *testing.T) {
	specs := map[string]resource.Spec{
		"claude/legacy-config": {
			Key:      "claude/legacy-config",
			Provider: "claude",
			ID:       "legacy-config",
			Category: resource.CategoryConfig,
			Layout:   resource.LayoutLegacy,
		},
		"copilot/instructions": {
			Key:      "copilot/instructions",
			Provider: "copilot",
			ID:       "instructions",
			Category: resource.CategoryInstructions,
			Layout:   resource.LayoutPortable,
		},
		"common/shared-skills": {
			Key:      "common/shared-skills",
			Provider: "common",
			ID:       "shared-skills",
			SharedAs: "shared-skills",
			Category: resource.CategorySkills,
			Layout:   resource.LayoutPortable,
		},
	}
	remote := state.Snapshot{
		"agents/claude/config/settings.json":                                            {Hash: "1"},
		"agents/gemini/config/settings.json":                                            {Hash: "2"},
		"agents/_portable/config/providers/copilot/instructions/instructions/README.md": {Hash: "3"},
		"agents/_portable/config/providers/gemini/config/settings/settings.json":        {Hash: "4"},
		"agents/_portable/config/common/skills/shared-skills/SKILL.md":                  {Hash: "5"},
		"agents/_portable/config/conflicts/c1/record.json":                              {Hash: "6"},
		"agents/_portable/config/install/skill-dependencies.json":                       {Hash: "7"},
	}

	owned, untouched, blocked := SplitRemoteSnapshot(remote, specs)

	for _, repoRel := range []string{
		"agents/claude/config/settings.json",
		"agents/_portable/config/providers/copilot/instructions/instructions/README.md",
		"agents/_portable/config/common/skills/shared-skills/SKILL.md",
		"agents/_portable/config/conflicts/c1/record.json",
		"agents/_portable/config/install/skill-dependencies.json",
	} {
		if _, ok := owned[repoRel]; !ok {
			t.Errorf("%q should be owned", repoRel)
		}
	}
	for _, repoRel := range []string{
		"agents/gemini/config/settings.json",
		"agents/_portable/config/providers/gemini/config/settings/settings.json",
	} {
		if _, ok := untouched[repoRel]; !ok {
			t.Errorf("%q should remain untouched", repoRel)
		}
	}
	if len(blocked) != 0 {
		t.Fatalf("unexpected blocked paths: %#v", blocked)
	}
}

func TestSplitRemoteSnapshotBlocksMalformedPortablePaths(t *testing.T) {
	remote := state.Snapshot{
		"agents/_portable/config/providers/claude/config":         {Hash: "1"},
		"agents/_portable/config/providers/claude/unknown/x/file": {Hash: "2"},
		"agents/_portable/sessions/file":                          {Hash: "3"},
	}

	owned, untouched, blocked := SplitRemoteSnapshot(remote, nil)

	if len(owned) != 0 || len(untouched) != 0 {
		t.Fatalf("malformed portable paths must not be reconciled: owned=%v untouched=%v", owned, untouched)
	}
	if len(blocked) != len(remote) {
		t.Fatalf("blocked = %#v, want all malformed paths", blocked)
	}
}

func TestValidateRemoteResourcesBlocksSecretsAndInvalidStructuredProjection(t *testing.T) {
	repo := t.TempDir()
	spec := resource.Spec{
		Key:         "demo/settings",
		Provider:    "demo",
		ID:          "settings",
		Category:    resource.CategoryConfig,
		Strategy:    resource.StrategyStructuredMerge,
		Layout:      resource.LayoutPortable,
		Transformer: "demo-settings",
	}
	registry := portableconfig.NewRegistry()
	registry.Register("demo-settings", portableconfig.Policy{
		Portable:  []string{"theme"},
		Sensitive: []string{"token"},
	})
	secretPath, err := spec.RepoPath("secret.txt")
	if err != nil {
		t.Fatal(err)
	}
	configPath, err := spec.RepoPath("settings.json")
	if err != nil {
		t.Fatal(err)
	}
	writeRepoFile(t, repo, secretPath, []byte("github_pat_abcdefghijklmnopqrstuvwxyz1234567890"))
	writeRepoFile(t, repo, configPath, []byte(`{"theme":"dark","token":"remote-secret"}`))
	owned := state.Snapshot{
		secretPath: {Hash: "1"},
		configPath: {Hash: "2"},
	}

	valid, blocked := ValidateRemoteResources(
		repo,
		owned,
		map[string]resource.Spec{spec.Key: spec},
		registry,
		"windows",
		t.TempDir(),
	)

	if len(valid) != 0 {
		t.Fatalf("invalid remote files should not be valid: %#v", valid)
	}
	if len(blocked) != 2 {
		t.Fatalf("blocked = %#v, want 2 issues", blocked)
	}
}

func TestValidateRemoteResourcesRejectsMalformedInternalMetadata(t *testing.T) {
	repo := t.TempDir()
	installPath := "agents/_portable/config/install/skill-dependencies.json"
	recordPath := "agents/_portable/config/conflicts/c1/record.json"
	writeRepoFile(t, repo, installPath, []byte("not-json"))
	writeRepoFile(t, repo, recordPath, []byte(`{"id":"other"}`))

	valid, blocked := ValidateRemoteResources(
		repo,
		state.Snapshot{installPath: {}, recordPath: {}},
		nil,
		portableconfig.NewRegistry(),
		"windows",
		t.TempDir(),
	)

	if len(valid) != 0 || len(blocked) != 2 {
		t.Fatalf("valid=%#v blocked=%#v", valid, blocked)
	}
}

func TestValidateRemoteResourcesBlocksRepositorySymlinks(t *testing.T) {
	repo := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.json")
	if err := os.WriteFile(outside, []byte(`{"token":"must-not-be-read"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	spec := resource.Spec{
		Key:      "demo/settings",
		Provider: "demo",
		ID:       "settings",
		Category: resource.CategoryConfig,
		Strategy: resource.StrategyFileTree,
		Layout:   resource.LayoutPortable,
	}
	repoRel, err := spec.RepoPath("settings.json")
	if err != nil {
		t.Fatal(err)
	}
	filename := filepath.Join(repo, filepath.FromSlash(repoRel))
	if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filename); err != nil {
		t.Skipf("symbolic links are unavailable: %v", err)
	}
	snapshot, err := SnapshotRepo(repo)
	if err != nil {
		t.Fatal(err)
	}

	valid, blocked := ValidateRemoteResources(
		repo,
		snapshot,
		map[string]resource.Spec{spec.Key: spec},
		portableconfig.NewRegistry(),
		"windows",
		t.TempDir(),
	)

	if len(valid) != 0 || len(blocked) != 1 || blocked[0].Code != "remote-link-blocked" {
		t.Fatalf("valid=%#v blocked=%#v", valid, blocked)
	}
}

func writeRepoFile(t *testing.T, repo, repoRel string, data []byte) {
	t.Helper()
	filename := filepath.Join(repo, filepath.FromSlash(repoRel))
	if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filename, data, 0o600); err != nil {
		t.Fatal(err)
	}
}
