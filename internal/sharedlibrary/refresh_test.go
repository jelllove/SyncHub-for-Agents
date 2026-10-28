package sharedlibrary

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRefreshHonorsDisabledCategories(t *testing.T) {
	m := fixture(t)
	source := filepath.Join(Root(m.UserHome), "skills", "review", "SKILL.md")
	target := filepath.Join(m.UserHome, ".claude", "skills", "review", "SKILL.md")
	put(t, source, "Original\n")
	execute(t, m, Request{Action: "apply", Kind: "skills", Name: "review", Harness: "claude"})
	put(t, source, "Updated\n")
	if err := m.Refresh(false, true); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(target)
	if err != nil || string(data) != "Original\n" {
		t.Fatal("disabled skills category was applied")
	}
}
