package cli

import (
	"testing"

	"github.com/qinqingxu/acsync/internal/config"
	"github.com/qinqingxu/acsync/internal/provider"
)

func TestBuildSpecsEnabledOnly(t *testing.T) {
	providers := []provider.Provider{
		{
			Name: "claude",
			Config: provider.ConfigSpec{
				Paths:    map[string]string{"linux": "~/.claude"},
				Include:  []string{"settings.json"},
				Sessions: []string{"projects/**/*.jsonl"},
				Exclude:  []string{"**/*token*"},
			},
			Secrets: provider.SecretSpec{KeyPatterns: []string{"token"}},
		},
		{
			Name: "gemini",
			Config: provider.ConfigSpec{
				Paths: map[string]string{"linux": "~/.gemini"},
			},
		},
	}
	cfg := config.Config{Agents: map[string]bool{"claude": true, "gemini": false}}

	specs, err := BuildSpecs(cfg, providers, "linux", "/home/alice")
	if err != nil {
		t.Fatalf("BuildSpecs error: %v", err)
	}
	if _, ok := specs["gemini"]; ok {
		t.Error("disabled agent should be excluded")
	}
	s, ok := specs["claude"]
	if !ok {
		t.Fatal("claude spec missing")
	}
	if s.Root != "/home/alice/.claude" {
		t.Errorf("root = %q", s.Root)
	}
	if s.Scanner == nil {
		t.Error("scanner should be set")
	}
}
