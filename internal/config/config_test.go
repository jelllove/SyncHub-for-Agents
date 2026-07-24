package config

import (
	"path/filepath"
	"testing"
)

func TestDefault(t *testing.T) {
	c := Default([]string{"claude", "copilot"})
	if c.SyncIntervalMinutes != 10 {
		t.Errorf("interval = %d, want 10", c.SyncIntervalMinutes)
	}
	if c.TrashGraceDays != 30 {
		t.Errorf("grace = %d, want 30", c.TrashGraceDays)
	}
	if !c.Agents["claude"] || !c.Agents["copilot"] {
		t.Errorf("all agents should default to enabled: %+v", c.Agents)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	in := Config{
		RepoURL:             "git@github.com:me/acsync-data.git",
		SyncIntervalMinutes: 15,
		TrashGraceDays:      7,
		Agents:              map[string]bool{"claude": true, "gemini": false},
	}
	if err := Save(path, in); err != nil {
		t.Fatalf("Save: %v", err)
	}
	out, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if out.RepoURL != in.RepoURL || out.SyncIntervalMinutes != 15 || out.TrashGraceDays != 7 {
		t.Errorf("round trip mismatch: %+v", out)
	}
	if out.Agents["claude"] != true || out.Agents["gemini"] != false {
		t.Errorf("agents mismatch: %+v", out.Agents)
	}
}

func TestLoadMissingIsError(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "nope.yaml")); err == nil {
		t.Fatal("expected error loading missing config")
	}
}

func TestEnabledAgents(t *testing.T) {
	c := Config{Agents: map[string]bool{"claude": true, "gemini": false, "copilot": true}}
	got := c.EnabledAgents()
	if len(got) != 2 {
		t.Fatalf("expected 2 enabled, got %v", got)
	}
}
