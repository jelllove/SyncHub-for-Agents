package startup

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"github.com/wailsapp/wails/v3/pkg/application"
)

type Backend interface {
	EnableWithOptions(application.AutostartOptions) error
	Disable() error
	IsEnabled() (bool, error)
}

type Manager struct {
	Backend    Backend
	Identifier string
	Arguments  []string
	GOOS       string
	HomeDir    func() (string, error)
	Executable func() (string, error)
	LookupEnv  func(string) (string, bool)
}

func (manager *Manager) Enable() error {
	switch manager.goos() {
	case "darwin":
		path, err := manager.registrationPath()
		if err != nil {
			return err
		}
		executable, err := manager.executable()
		if err != nil {
			return err
		}
		return writeFile(path, launchAgent(manager.Identifier, executable, manager.Arguments), 0o644)
	case "linux":
		path, err := manager.registrationPath()
		if err != nil {
			return err
		}
		executable, err := manager.linuxExecutable()
		if err != nil {
			return err
		}
		return writeFile(path, desktopEntry(executable, manager.Arguments), 0o644)
	}
	if manager.Backend == nil {
		return errors.New("autostart backend is not configured")
	}
	return manager.Backend.EnableWithOptions(application.AutostartOptions{
		Identifier: manager.Identifier,
		Arguments:  append([]string(nil), manager.Arguments...),
	})
}

func (manager *Manager) Disable() error {
	if manager.goos() == "darwin" || manager.goos() == "linux" {
		path, err := manager.registrationPath()
		if err != nil {
			return err
		}
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("disable autostart: %w", err)
		}
		return nil
	}
	if manager.Backend == nil {
		return errors.New("autostart backend is not configured")
	}
	return manager.Backend.Disable()
}

func (manager *Manager) IsEnabled() (bool, error) {
	if manager.goos() == "darwin" || manager.goos() == "linux" {
		path, err := manager.registrationPath()
		if err != nil {
			return false, err
		}
		_, err = os.Stat(path)
		if err == nil {
			return true, nil
		}
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	if manager.Backend == nil {
		return false, errors.New("autostart backend is not configured")
	}
	return manager.Backend.IsEnabled()
}

func (manager *Manager) goos() string {
	if manager.GOOS != "" {
		return manager.GOOS
	}
	return runtime.GOOS
}

func (manager *Manager) homeDir() (string, error) {
	if manager.HomeDir != nil {
		return manager.HomeDir()
	}
	return os.UserHomeDir()
}

func (manager *Manager) executable() (string, error) {
	if manager.Executable != nil {
		return manager.Executable()
	}
	return os.Executable()
}

func (manager *Manager) lookupEnv(key string) (string, bool) {
	if manager.LookupEnv != nil {
		return manager.LookupEnv(key)
	}
	return os.LookupEnv(key)
}

func (manager *Manager) linuxExecutable() (string, error) {
	if appImage, ok := manager.lookupEnv("APPIMAGE"); ok {
		if !filepath.IsAbs(appImage) {
			return "", errors.New("APPIMAGE must be an absolute path")
		}
		if _, err := os.Stat(appImage); err != nil {
			return "", fmt.Errorf("resolve AppImage autostart executable: %w", err)
		}
		return appImage, nil
	}
	return manager.executable()
}

func (manager *Manager) registrationPath() (string, error) {
	if manager.Identifier == "" || strings.ContainsAny(manager.Identifier, `/\`) {
		return "", errors.New("autostart identifier must not be empty or contain path separators")
	}
	home, err := manager.homeDir()
	if err != nil {
		return "", err
	}
	switch manager.goos() {
	case "darwin":
		return filepath.Join(home, "Library", "LaunchAgents", manager.Identifier+".plist"), nil
	case "linux":
		configHome, ok := manager.lookupEnv("XDG_CONFIG_HOME")
		if !ok || configHome == "" {
			configHome = filepath.Join(home, ".config")
		}
		return filepath.Join(configHome, "autostart", manager.Identifier+".desktop"), nil
	default:
		return "", fmt.Errorf("custom autostart is unsupported on %s", manager.goos())
	}
}

func writeFile(path string, contents []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create autostart directory: %w", err)
	}
	temp, err := os.CreateTemp(filepath.Dir(path), ".autostart-*")
	if err != nil {
		return fmt.Errorf("create autostart entry: %w", err)
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)
	if err := temp.Chmod(mode); err != nil {
		temp.Close()
		return fmt.Errorf("set autostart permissions: %w", err)
	}
	if _, err := temp.Write(contents); err != nil {
		temp.Close()
		return fmt.Errorf("write autostart entry: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close autostart entry: %w", err)
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("replace autostart entry: %w", err)
	}
	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("write autostart entry: %w", err)
	}
	return nil
}

func launchAgent(identifier, executable string, arguments []string) []byte {
	var body bytes.Buffer
	body.WriteString(xml.Header)
	body.WriteString("<!DOCTYPE plist PUBLIC \"-//Apple//DTD PLIST 1.0//EN\" \"http://www.apple.com/DTDs/PropertyList-1.0.dtd\">\n")
	body.WriteString("<plist version=\"1.0\"><dict><key>Label</key><string>")
	xml.EscapeText(&body, []byte(identifier))
	body.WriteString("</string><key>ProgramArguments</key><array><string>")
	xml.EscapeText(&body, []byte(executable))
	body.WriteString("</string>")
	for _, argument := range arguments {
		body.WriteString("<string>")
		xml.EscapeText(&body, []byte(argument))
		body.WriteString("</string>")
	}
	body.WriteString("</array><key>RunAtLoad</key><true/></dict></plist>\n")
	return body.Bytes()
}

func desktopEntry(executable string, arguments []string) []byte {
	tokens := []string{desktopToken(executable)}
	for _, argument := range arguments {
		tokens = append(tokens, desktopToken(argument))
	}
	var body bytes.Buffer
	body.WriteString("[Desktop Entry]\nType=Application\nName=AgentConfigSync\nExec=")
	body.WriteString(strings.Join(tokens, " "))
	body.WriteString("\nX-GNOME-Autostart-enabled=true\n")
	return body.Bytes()
}

func desktopToken(value string) string {
	value = strings.ReplaceAll(value, "%", "%%")
	return strconv.Quote(value)
}
