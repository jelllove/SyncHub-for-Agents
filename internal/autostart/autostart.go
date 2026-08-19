// Package autostart manages an OS-appropriate "run at login" entry.
package autostart

import (
	"encoding/xml"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const label = "io.github.qinqingxu.synchub.agent"

// Runner executes an external command (launchctl/systemctl). Injectable for tests.
type Runner func(name string, args ...string) error

// Manager writes and removes an OS-appropriate autostart entry.
type Manager struct {
	GOOS string
	Dir  string // directory holding the entry
	File string // entry filename
	Run  Runner // used on darwin/linux to (de)register; nil on windows
}

// New builds a Manager for goos using home as the base for user directories.
func New(goos, home string) (*Manager, error) {
	m := &Manager{GOOS: goos, Run: execRunner}
	switch goos {
	case "windows":
		appData := os.Getenv("APPDATA")
		if appData == "" {
			appData = filepath.Join(home, "AppData", "Roaming")
		}
		m.Dir = filepath.Join(appData, "Microsoft", "Windows", "Start Menu", "Programs", "Startup")
		m.File = "synchub.cmd"
		m.Run = nil
	case "darwin":
		m.Dir = filepath.Join(home, "Library", "LaunchAgents")
		m.File = label + ".plist"
	case "linux":
		m.Dir = filepath.Join(home, ".config", "systemd", "user")
		m.File = "synchub.service"
	default:
		return nil, fmt.Errorf("autostart: unsupported OS %q", goos)
	}
	return m, nil
}

// Path returns the full path of the autostart entry file.
func (m *Manager) Path() string { return filepath.Join(m.Dir, m.File) }

func (m *Manager) content(execPath string) string {
	switch m.GOOS {
	case "windows":
		return windowsCmd(execPath)
	case "darwin":
		return launchAgentPlist(execPath)
	case "linux":
		return systemdUnit(execPath)
	default:
		return ""
	}
}

// Enable writes the autostart entry pointing at execPath and, on darwin/linux,
// registers it with the user service manager.
func (m *Manager) Enable(execPath string) error {
	if err := os.MkdirAll(m.Dir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(m.Path(), []byte(m.content(execPath)), 0o644); err != nil {
		return err
	}
	if m.Run == nil {
		return nil
	}
	switch m.GOOS {
	case "darwin":
		return m.Run("launchctl", "load", "-w", m.Path())
	case "linux":
		return m.Run("systemctl", "--user", "enable", "synchub.service")
	}
	return nil
}

// Disable removes the autostart entry (and unregisters it on darwin/linux).
func (m *Manager) Disable() error {
	if m.Run != nil {
		switch m.GOOS {
		case "darwin":
			_ = m.Run("launchctl", "unload", "-w", m.Path())
		case "linux":
			_ = m.Run("systemctl", "--user", "disable", "synchub.service")
		}
	}
	err := os.Remove(m.Path())
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// IsEnabled reports whether the autostart entry file exists.
func (m *Manager) IsEnabled() (bool, error) {
	_, err := os.Stat(m.Path())
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func execRunner(name string, args ...string) error {
	return exec.Command(name, args...).Run()
}

func windowsCmd(execPath string) string {
	return "@echo off\r\nstart \"\" \"" + execPath + "\" tray\r\n"
}

func launchAgentPlist(execPath string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>` + label + `</string>
	<key>ProgramArguments</key>
	<array>
		<string>` + xmlEscape(execPath) + `</string>
		<string>tray</string>
	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	<true/>
</dict>
</plist>
`
}

func systemdUnit(execPath string) string {
	return `[Unit]
Description=SyncHub daemon
After=network-online.target

[Service]
ExecStart=` + execPath + ` tray
Restart=on-failure

[Install]
WantedBy=default.target
`
}

func xmlEscape(s string) string {
	var b strings.Builder
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}
