package resource

import "testing"

func TestPortableRepoPathUsesUnknownPseudoProvider(t *testing.T) {
	spec := Spec{
		Key:      "claude/settings",
		Provider: "claude",
		ID:       "settings",
		Category: CategoryConfig,
		Layout:   LayoutPortable,
	}

	got, err := spec.RepoPath("settings.json")
	if err != nil {
		t.Fatal(err)
	}
	want := "agents/_portable/config/providers/claude/config/settings/settings.json"
	if got != want {
		t.Fatalf("RepoPath = %q, want %q", got, want)
	}
}

func TestCommonRepoPathUsesSharedIdentity(t *testing.T) {
	spec := Spec{
		Key:      "common/common-skills",
		Provider: "common",
		ID:       "skills",
		Category: CategorySkills,
		Layout:   LayoutPortable,
		SharedAs: "common-skills",
	}

	got, err := spec.RepoPath("brainstorming/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	want := "agents/_portable/config/common/skills/common-skills/brainstorming/SKILL.md"
	if got != want {
		t.Fatalf("RepoPath = %q, want %q", got, want)
	}
}

func TestPortableRepoPathRoundTrips(t *testing.T) {
	spec := Spec{
		Key:      "common/common-skills",
		Provider: "common",
		ID:       "skills",
		Category: CategorySkills,
		Layout:   LayoutPortable,
		SharedAs: "common-skills",
	}

	repoPath, err := spec.RepoPath("brainstorming/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	got, err := ParseRepoPath(repoPath)
	if err != nil {
		t.Fatal(err)
	}
	if got.Provider != "common" ||
		got.Category != CategorySkills ||
		got.ResourceID != "common-skills" ||
		got.Relative != "brainstorming/SKILL.md" ||
		!got.Portable ||
		!got.Common {
		t.Fatalf("round trip = %#v", got)
	}
}

func TestRepoPathRejectsUnsafeRelativePaths(t *testing.T) {
	spec := Spec{
		Key:      "claude/settings",
		Provider: "claude",
		ID:       "settings",
		Category: CategoryConfig,
		Layout:   LayoutPortable,
	}

	for _, relative := range []string{
		"",
		"/etc/passwd",
		"C:/Windows/system.ini",
		`nested\file.txt`,
		"../secrets.txt",
		"./settings.json",
		"dir/../settings.json",
		"dir//settings.json",
	} {
		if _, err := spec.RepoPath(relative); err == nil {
			t.Fatalf("RepoPath(%q) error = nil, want rejection", relative)
		}
	}
}

func TestRepoPathRejectsUnsafeIdentifiers(t *testing.T) {
	tests := []Spec{
		{
			Key:      "bad/settings",
			Provider: "foo/bar",
			ID:       "settings",
			Category: CategoryConfig,
			Layout:   LayoutPortable,
		},
		{
			Key:      "claude/bad",
			Provider: "claude",
			ID:       "config/settings",
			Category: CategoryConfig,
			Layout:   LayoutPortable,
		},
		{
			Key:      "common/bad",
			Provider: "common",
			ID:       "skills",
			Category: CategorySkills,
			Layout:   LayoutPortable,
			SharedAs: "../common-skills",
		},
	}

	for _, spec := range tests {
		if _, err := spec.RepoPath("settings.json"); err == nil {
			t.Fatalf("RepoPath(%#v) error = nil, want rejection", spec)
		}
	}
}

func TestParseRepoPathRecognizesPortableProviderPath(t *testing.T) {
	got, err := ParseRepoPath("agents/_portable/config/providers/claude/config/settings/settings.json")
	if err != nil {
		t.Fatal(err)
	}
	if got.Provider != "claude" ||
		got.Category != CategoryConfig ||
		got.ResourceID != "settings" ||
		got.Relative != "settings.json" ||
		!got.Portable ||
		got.Common {
		t.Fatalf("ParseRepoPath() = %#v", got)
	}
}

func TestParseRepoPathRecognizesCommonPath(t *testing.T) {
	got, err := ParseRepoPath("agents/_portable/config/common/skills/common-skills/brainstorming/SKILL.md")
	if err != nil {
		t.Fatal(err)
	}
	if got.Provider != "common" ||
		got.Category != CategorySkills ||
		got.ResourceID != "common-skills" ||
		got.Relative != "brainstorming/SKILL.md" ||
		!got.Portable ||
		!got.Common {
		t.Fatalf("ParseRepoPath() = %#v", got)
	}
}

func TestParseRepoPathRecognizesLegacyPath(t *testing.T) {
	got, err := ParseRepoPath("agents/claude/sessions/history.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	if got.Provider != "claude" ||
		got.Category != CategorySessions ||
		got.ResourceID != "legacy-sessions" ||
		got.Relative != "history.jsonl" ||
		got.Portable ||
		got.Common {
		t.Fatalf("ParseRepoPath() = %#v", got)
	}
}

func TestParseRepoPathRejectsMalformedPaths(t *testing.T) {
	for _, repoPath := range []string{
		"",
		"agents",
		"agents/claude/config",
		"agents/claude/config/../settings.json",
		`agents\claude\config\settings.json`,
		"agents/_portable/config/providers/claude/config/settings",
		"agents/_portable/config/common/skills",
	} {
		if _, err := ParseRepoPath(repoPath); err == nil {
			t.Fatalf("ParseRepoPath(%q) error = nil, want rejection", repoPath)
		}
	}
}
