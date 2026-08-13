// Package startup manages desktop login startup and legacy entry migration.
package startup

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type Runner func(name string, args ...string) error

func RemoveLegacy(goos, home string, run Runner) ([]string, error) {
	path, err := legacyPath(goos, home)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	switch goos {
	case "darwin":
		if run == nil {
			return nil, errors.New("launchd unregister runner is required")
		}
		if err := run("launchctl", "unload", "-w", path); err != nil {
			return nil, fmt.Errorf("unregister legacy launch agent: %w", err)
		}
	case "linux":
		if run == nil {
			return nil, errors.New("systemd unregister runner is required")
		}
		if err := run("systemctl", "--user", "disable", "--now", "acsync.service"); err != nil {
			return nil, fmt.Errorf("unregister legacy systemd service: %w", err)
		}
	}
	if err := os.Remove(path); err != nil {
		return nil, err
	}
	if goos == "linux" {
		if err := run("systemctl", "--user", "daemon-reload"); err != nil {
			return nil, fmt.Errorf("reload user systemd services: %w", err)
		}
	}
	return []string{path}, nil
}

func legacyPath(goos, home string) (string, error) {
	switch goos {
	case "windows":
		appData := os.Getenv("APPDATA")
		if appData == "" {
			appData = filepath.Join(home, "AppData", "Roaming")
		}
		return filepath.Join(appData, "Microsoft", "Windows", "Start Menu", "Programs", "Startup", "acsync.cmd"), nil
	case "darwin":
		return filepath.Join(home, "Library", "LaunchAgents", "com.acsync.agent.plist"), nil
	case "linux":
		return filepath.Join(home, ".config", "systemd", "user", "acsync.service"), nil
	default:
		return "", fmt.Errorf("startup migration: unsupported OS %q", goos)
	}
}
