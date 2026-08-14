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

	config := got[0]
	if config.ID != "legacy-config" ||
		config.Category != resource.CategoryConfig ||
		config.Strategy != resource.StrategyFileTree ||
		config.Layout != resource.LayoutLegacy {
		t.Fatalf("config declaration = %#v", config)
	}
	if config.Paths["windows"] != "%USERPROFILE%\\.demo" ||
		len(config.Include) != 1 || config.Include[0] != "settings.json" ||
		len(config.Exclude) != 1 || config.Exclude[0] != "**/*token*" {
		t.Fatalf("config declaration inheritance = %#v", config)
	}

	sessions := got[1]
	if sessions.ID != "legacy-sessions" ||
		sessions.Category != resource.CategorySessions ||
		sessions.Strategy != resource.StrategyFileTree ||
		sessions.Layout != resource.LayoutLegacy {
		t.Fatalf("sessions declaration = %#v", sessions)
	}
	if sessions.Paths["windows"] != "%USERPROFILE%\\.demo" ||
		len(sessions.Include) != 1 || sessions.Include[0] != "sessions/**/*.jsonl" ||
		len(sessions.Exclude) != 1 || sessions.Exclude[0] != "**/*token*" {
		t.Fatalf("sessions declaration inheritance = %#v", sessions)
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

func TestParseRejectsMalformedTypedResources(t *testing.T) {
	tests := []struct {
		name string
		data string
		want string
	}{
		{
			name: "structured-merge-missing-transformer",
			data: `
schema_version: 2
name: demo
resources:
  - id: settings
    category: config
    paths: {linux: "~/.demo"}
    include: ["settings.json"]
    strategy: structured-merge
`,
			want: "missing transformer",
		},
		{
			name: "install-manifest-missing-installer",
			data: `
schema_version: 2
name: demo
resources:
  - id: plugins
    category: plugins
    paths: {linux: "~/.demo/plugins"}
    strategy: install-manifest
`,
			want: "missing installer",
		},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse([]byte(tt.data))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want substring %q", err, tt.want)
			}
		})
	}
}
