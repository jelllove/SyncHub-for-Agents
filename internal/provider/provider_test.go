package provider

import (
	"strings"
	"testing"

	"github.com/qinqingxu/acsync/internal/resource"
)

func TestBuiltinsLoaded(t *testing.T) {
	ps, err := Builtins()
	if err != nil {
		t.Fatalf("Builtins() error: %v", err)
	}
	byName := map[string]Provider{}
	for _, p := range ps {
		byName[p.Name] = p
	}
	for _, want := range []string{"claude", "copilot", "gemini", "cursor", "vscode-copilot"} {
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
	for _, session := range []string{
		"history.jsonl",
		"sessions/**/*.json",
		"projects/**/sessions-index.json",
	} {
		if !contains(claude.Config.Sessions, session) {
			t.Errorf("claude sessions = %#v, missing %q", claude.Config.Sessions, session)
		}
	}

	copilot := byName["copilot"]
	for _, session := range []string{
		"session-state/*/events.jsonl",
		"session-state/*/workspace.yaml",
		"session-state/*/checkpoints/**/*.md",
	} {
		if !contains(copilot.Config.Sessions, session) {
			t.Errorf("copilot sessions = %#v, missing %q", copilot.Config.Sessions, session)
		}
	}

	gemini := byName["gemini"]
	if !contains(gemini.Config.Sessions, "tmp/**/chats/**/*.jsonl") {
		t.Fatalf("gemini sessions = %#v, want current Gemini CLI chat path", gemini.Config.Sessions)
	}

	vscode := byName["vscode-copilot"]
	if len(vscode.Secrets.KeyPatterns) == 0 {
		t.Error("VS Code provider must declare top-level secret key patterns")
	}
	if vscode.Config.Paths["windows"] != "%USERPROFILE%\\AppData\\Roaming\\Code\\User\\workspaceStorage" {
		t.Errorf("VS Code Windows path = %q", vscode.Config.Paths["windows"])
	}
	for _, session := range []string{
		"*/workspace.json",
		"*/chatSessions/*.json",
		"*/chatSessions/*.jsonl",
	} {
		if !contains(vscode.Config.Sessions, session) {
			t.Errorf("VS Code sessions = %#v, missing %q", vscode.Config.Sessions, session)
		}
	}
	if contains(vscode.Config.Sessions, "*/chatEditingSessions/*/state.json") {
		t.Error("VS Code editing state contains raw source snapshots and must not be synced")
	}
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
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

func TestProviderDeclarationsNormalizeV1(t *testing.T) {
	p, err := Parse([]byte(`
name: demo
config:
  paths: {windows: "%USERPROFILE%\\.demo"}
  include: ["settings.json"]
  sessions: ["sessions/**/*.jsonl"]
  exclude: ["**/*token*"]
`))
	if err != nil {
		t.Fatal(err)
	}

	got, err := p.Declarations()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("declarations = %#v", got)
	}
	if got[0].Category != resource.CategoryConfig ||
		got[0].Layout != resource.LayoutLegacy ||
		got[1].Category != resource.CategorySessions {
		t.Fatalf("normalized declarations = %#v", got)
	}
}

func TestParseRejectsReservedPortableProvider(t *testing.T) {
	_, err := Parse([]byte("schema_version: 2\nname: _portable\nresources: []\n"))
	if err == nil || !strings.Contains(err.Error(), "reserved") {
		t.Fatalf("error = %v", err)
	}
}

func TestParseRejectsMixedLegacyAndTypedDefinitions(t *testing.T) {
	_, err := Parse([]byte(`
schema_version: 2
name: demo
config:
  paths: {linux: "~/.demo"}
  include: ["settings.json"]
resources:
  - id: config
    category: config
    paths: {linux: "~/.demo"}
    include: ["settings.json"]
    strategy: file-tree
`))
	if err == nil || !strings.Contains(err.Error(), "mixed") {
		t.Fatalf("error = %v", err)
	}
}
