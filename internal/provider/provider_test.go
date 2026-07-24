package provider

import "testing"

func TestBuiltinsLoaded(t *testing.T) {
	ps, err := Builtins()
	if err != nil {
		t.Fatalf("Builtins() error: %v", err)
	}
	byName := map[string]Provider{}
	for _, p := range ps {
		byName[p.Name] = p
	}
	for _, want := range []string{"claude", "copilot", "gemini", "cursor"} {
		if _, ok := byName[want]; !ok {
			t.Errorf("missing builtin provider %q", want)
		}
	}
	claude := byName["claude"]
	if claude.Config.Paths["linux"] != "~/.claude" {
		t.Errorf("claude linux path = %q", claude.Config.Paths["linux"])
	}
	if len(claude.Config.Sessions) == 0 {
		t.Error("claude should declare session globs")
	}
	if len(claude.Secrets.KeyPatterns) == 0 {
		t.Error("claude should declare secret key patterns")
	}
}

func TestLoadFromBytes(t *testing.T) {
	data := []byte(`
name: demo
config:
  paths:
    linux: "~/.demo"
  include: ["settings.json"]
  sessions: ["s/**/*.json"]
  exclude: ["**/*token*"]
secrets:
  key_patterns: ["token"]
`)
	p, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if p.Name != "demo" {
		t.Errorf("name = %q, want demo", p.Name)
	}
	if p.Config.Include[0] != "settings.json" {
		t.Errorf("include = %v", p.Config.Include)
	}
}

func TestParseRejectsMissingName(t *testing.T) {
	if _, err := Parse([]byte(`config: {}`)); err == nil {
		t.Fatal("expected error for provider without name")
	}
}
