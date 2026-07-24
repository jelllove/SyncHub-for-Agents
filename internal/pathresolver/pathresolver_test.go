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