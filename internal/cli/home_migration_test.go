package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureHomeReturnsCurrentWhenPresent(t *testing.T) {
	userHome := t.TempDir()
	current := filepath.Join(userHome, currentHomeDirName)
	if err := os.MkdirAll(current, 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := ensureHome(userHome)
	if err != nil {
		t.Fatal(err)
	}
	if got != current {
		t.Fatalf("ensureHome() = %q, want %q", got, current)
	}
}

func TestEnsureHomeMigratesLegacyDirectory(t *testing.T) {
	userHome := t.TempDir()
	legacy := filepath.Join(userHome, legacyHomeDirName)
	current := filepath.Join(userHome, currentHomeDirName)
	if err := os.MkdirAll(filepath.Join(legacy, "logs"), 0o755); err != nil {
		t.Fatal(err)
	}
	configPath := filepath.Join(legacy, "config.yaml")
	if err := os.WriteFile(configPath, []byte("version: 3\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	logPath := filepath.Join(legacy, "logs", "daemon.log")
	if err := os.WriteFile(logPath, []byte("ok"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := ensureHome(userHome)
	if err != nil {
		t.Fatal(err)
	}
	if got != current {
		t.Fatalf("ensureHome() = %q, want %q", got, current)
	}
	if _, err := os.Stat(filepath.Join(current, "config.yaml")); err != nil {
		t.Fatalf("migrated config missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(current, "logs", "daemon.log")); err != nil {
		t.Fatalf("migrated logs missing: %v", err)
	}
}

func TestEnsureHomeReturnsCurrentPathWhenLegacyMissing(t *testing.T) {
	userHome := t.TempDir()
	want := filepath.Join(userHome, currentHomeDirName)

	got, err := ensureHome(userHome)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("ensureHome() = %q, want %q", got, want)
	}
}

func TestEnsureHomeRejectsLegacyFile(t *testing.T) {
	userHome := t.TempDir()
	legacy := filepath.Join(userHome, legacyHomeDirName)
	if err := os.WriteFile(legacy, []byte("file"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := ensureHome(userHome); err == nil {
		t.Fatal("expected error when legacy path is not a directory")
	}
}

