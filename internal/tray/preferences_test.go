package tray

import (
	"os"
	"path/filepath"
	"testing"
)

func TestTipPreferenceDefaultsToEnabledAndPersistsOptOut(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tray-settings.json")
	prefs, err := NewTipPreferences(path)
	if err != nil {
		t.Fatal(err)
	}
	if !prefs.Enabled() {
		t.Fatal("first-run activity tips should remain enabled")
	}
	changes := []bool{}
	unsubscribe := prefs.Subscribe(func(enabled bool) { changes = append(changes, enabled) })
	if err := prefs.SetEnabled(false); err != nil {
		t.Fatal(err)
	}
	reloaded, err := NewTipPreferences(path)
	if err != nil || reloaded.Enabled() {
		t.Fatalf("opt-out did not survive restart: %v", err)
	}
	if err := prefs.SetEnabled(true); err != nil {
		t.Fatal(err)
	}
	unsubscribe()
	if err := prefs.SetEnabled(false); err != nil {
		t.Fatal(err)
	}
	if len(changes) != 2 || changes[0] || !changes[1] {
		t.Fatalf("preference changes = %v", changes)
	}
}

func TestTipPreferenceSaveFailureDoesNotChangeEnabledOrNotify(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "tray-settings.json")
	prefs, err := NewTipPreferences(path)
	if err != nil {
		t.Fatal(err)
	}
	called := false
	prefs.Subscribe(func(bool) { called = true })
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := prefs.SetEnabled(false); err == nil {
		t.Fatal("saving to a directory succeeded")
	}
	if !prefs.Enabled() || called {
		t.Fatal("failed save was presented as a successful opt-out")
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 1 {
		t.Fatalf("failed save left temporary files: %v, %v", entries, err)
	}
}

func TestTipPreferenceRejectsMalformedFiles(t *testing.T) {
	for _, data := range []string{"", "{", "{}", `{"enabled":"yes"}`, `{"enabled":false} {}`} {
		path := filepath.Join(t.TempDir(), "tray-settings.json")
		if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := NewTipPreferences(path); err == nil {
			t.Fatalf("invalid preference accepted: %q", data)
		}
	}
	if _, err := NewTipPreferences(""); err == nil {
		t.Fatal("missing preference path accepted")
	}
}
