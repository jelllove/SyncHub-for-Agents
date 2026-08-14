package pathresolver

import (
	"path/filepath"
	"testing"
)

func TestResolveForUnixTilde(t *testing.T) {
	got, err := ResolveFor("~/.claude", "linux", "/home/alice")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := filepath.FromSlash("/home/alice/.claude")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestResolveForWindowsUserProfile(t *testing.T) {
	got, err := ResolveFor("%USERPROFILE%\\.claude", "windows", `C:\Users\alice`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := filepath.FromSlash(`C:\Users\alice\.claude`)
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestResolveForTildeOnWindows(t *testing.T) {
	got, err := ResolveFor("~/.gemini", "windows", `C:\Users\alice`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := filepath.FromSlash(`C:\Users\alice\.gemini`)
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestResolveForEmptyHome(t *testing.T) {
	if _, err := ResolveFor("~/.claude", "linux", ""); err == nil {
		t.Fatal("expected error when home is empty, got nil")
	}
}

func TestTokenizeHomeOnlyRewritesHomeOrDescendants(t *testing.T) {
	tests := []struct {
		name  string
		value string
		goos  string
		home  string
		want  string
		ok    bool
	}{
		{"windows-home", `C:\Users\alice`, "windows", `C:\Users\alice`, HomeToken, true},
		{"windows-child", `C:\Users\alice\.agents\skills`, "windows", `C:\Users\alice`, `${HOME}/.agents/skills`, true},
		{"windows-case", `c:\users\ALICE\.agents`, "windows", `C:\Users\alice`, `${HOME}/.agents`, true},
		{"windows-prefix-collision", `C:\Users\alice2\.agents`, "windows", `C:\Users\alice`, `C:\Users\alice2\.agents`, false},
		{"windows-traversal", `C:\Users\alice\..\bob`, "windows", `C:\Users\alice`, `C:\Users\alice\..\bob`, false},
		{"linux-home", "/home/alice", "linux", "/home/alice", HomeToken, true},
		{"linux-child", "/home/alice/.agents/skills", "linux", "/home/alice", `${HOME}/.agents/skills`, true},
		{"linux-outside", "/opt/skills", "linux", "/home/alice", "/opt/skills", false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := TokenizeHome(test.value, test.goos, test.home)
			if got != test.want || ok != test.ok {
				t.Fatalf("TokenizeHome() = (%q, %v), want (%q, %v)", got, ok, test.want, test.ok)
			}
		})
	}
}

func TestExpandHomeTokenUsesTargetPlatformAndRejectsTraversal(t *testing.T) {
	tests := []struct {
		value string
		goos  string
		home  string
		want  string
	}{
		{HomeToken, "windows", `C:\Users\alice`, `C:\Users\alice`},
		{`${HOME}/.agents/skills`, "windows", `C:\Users\alice`, `C:\Users\alice\.agents\skills`},
		{`${HOME}/.agents/skills`, "linux", "/home/alice", "/home/alice/.agents/skills"},
		{"/opt/skills", "linux", "/home/alice", "/opt/skills"},
	}
	for _, test := range tests {
		got, err := ExpandHomeToken(test.value, test.goos, test.home)
		if err != nil {
			t.Fatal(err)
		}
		if got != test.want {
			t.Fatalf("ExpandHomeToken(%q) = %q, want %q", test.value, got, test.want)
		}
	}

	for _, value := range []string{
		`${HOME}/../bob`,
		`${HOME}//skills`,
		`${HOME}\..\bob`,
	} {
		if _, err := ExpandHomeToken(value, "windows", `C:\Users\alice`); err == nil {
			t.Fatalf("ExpandHomeToken(%q) error = nil", value)
		}
	}
}
