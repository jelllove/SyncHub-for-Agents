package autostart

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewWindowsManager(t *testing.T) {
	t.Setenv("APPDATA", `C:\Users\alice\AppData\Roaming`)
	m, err := New("windows", `C:\Users\alice`)
	if err != nil {
		t.Fatal(err)
	}
	wantDir := filepath.Join(`C:\Users\alice\AppData\Roaming`, "Microsoft", "Windows", "Start Menu", "Programs", "Startup")
	if m.Dir != wantDir {
		t.Errorf("dir = %q, want %q", m.Dir, wantDir)
	}
	if m.File != "acsync.cmd" {
		t.Errorf("file = %q", m.File)
	}
}

func TestNewUnsupportedOS(t *testing.T) {
	if _, err := New("plan9", "/home/x"); err == nil {
		t.Fatal("expected error for unsupported OS")
	}
}

func TestContentGenerators(t *testing.T) {
	if got := windowsCmd(`C:\acsync.exe`); !strings.Contains(got, `start "" "C:\acsync.exe" tray`) {
		t.Errorf("windows cmd = %q", got)
	}
	plist := launchAgentPlist("/usr/local/bin/acsync")
	if !strings.Contains(plist, "<string>/usr/local/bin/acsync</string>") || !strings.Contains(plist, "com.acsync.agent") {
		t.Errorf("plist = %q", plist)
	}
	unit := systemdUnit("/usr/local/bin/acsync")
	if !strings.Contains(unit, "ExecStart=/usr/local/bin/acsync tray") {
		t.Errorf("unit = %q", unit)
	}
}

func TestEnableDisableLinux(t *testing.T) {
	home := t.TempDir()
	m, err := New("linux", home)
	if err != nil {
		t.Fatal(err)
	}
	var calls [][]string
	m.Run = func(name string, args ...string) error {
		calls = append(calls, append([]string{name}, args...))
		return nil
	}

	if err := m.Enable("/opt/acsync"); err != nil {
		t.Fatal(err)
	}
	if en, _ := m.IsEnabled(); !en {
		t.Error("should be enabled")
	}
	data, _ := os.ReadFile(m.Path())
	if !strings.Contains(string(data), "ExecStart=/opt/acsync tray") {
		t.Errorf("unit body = %q", string(data))
	}
	if len(calls) != 1 || calls[0][0] != "systemctl" {
		t.Errorf("register calls = %v", calls)
	}

	if err := m.Disable(); err != nil {
		t.Fatal(err)
	}
	if en, _ := m.IsEnabled(); en {
		t.Error("should be disabled after Disable")
	}
}

func TestEnableWindowsWritesCmd(t *testing.T) {
	home := t.TempDir()
	t.Setenv("APPDATA", filepath.Join(home, "AppData", "Roaming"))
	m, err := New("windows", home)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Enable(`C:\acsync.exe`); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(m.Path())
	if !strings.Contains(string(data), `"C:\acsync.exe" tray`) {
		t.Errorf("cmd body = %q", string(data))
	}
}
