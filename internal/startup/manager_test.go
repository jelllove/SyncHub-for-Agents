package startup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"
)

type fakeBackend struct {
	options application.AutostartOptions
	enabled bool
}

func TestManagerUsesStableAppImagePath(t *testing.T) {
	home := t.TempDir()
	appImage := filepath.Join(home, "Applications", "SyncHub.AppImage")
	if err := os.MkdirAll(filepath.Dir(appImage), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(appImage, nil, 0o755); err != nil {
		t.Fatal(err)
	}
	manager := &Manager{
		Identifier: "io.github.qinqingxu.synchub",
		Arguments:  []string{"--hidden"},
		GOOS:       "linux",
		HomeDir:    func() (string, error) { return home, nil },
		LookupEnv: func(key string) (string, bool) {
			if key == "APPIMAGE" {
				return appImage, true
			}
			return "", false
		},
	}
	if err := manager.Enable(); err != nil {
		t.Fatal(err)
	}
	path, err := manager.registrationPath()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), strconvQuote(appImage)) ||
		strings.Contains(string(data), "/tmp/.mount_") {
		t.Fatalf("desktop entry = %s", data)
	}
}

func TestManagerCreatesHiddenMacLaunchAgent(t *testing.T) {
	home := t.TempDir()
	manager := &Manager{
		Identifier: "io.github.qinqingxu.synchub",
		Arguments:  []string{"--hidden"},
		GOOS:       "darwin",
		HomeDir:    func() (string, error) { return home, nil },
		Executable: func() (string, error) { return "/Applications/SyncHub.app/Contents/MacOS/SyncHub", nil },
	}
	if err := manager.Enable(); err != nil {
		t.Fatal(err)
	}
	path, err := manager.registrationPath()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "<string>--hidden</string>") {
		t.Fatalf("launch agent = %s", data)
	}
}

func strconvQuote(value string) string {
	return `"` + strings.ReplaceAll(value, `\`, `\\`) + `"`
}

func (backend *fakeBackend) EnableWithOptions(options application.AutostartOptions) error {
	backend.options = options
	backend.enabled = true
	return nil
}

func (backend *fakeBackend) Disable() error {
	backend.enabled = false
	return nil
}

func (backend *fakeBackend) IsEnabled() (bool, error) {
	return backend.enabled, nil
}

func TestManagerControlsWindowsNativeAutostart(t *testing.T) {
	backend := &fakeBackend{}
	manager := &Manager{
		Backend:    backend,
		Identifier: "io.github.qinqingxu.synchub",
		Arguments:  []string{"--hidden"},
		GOOS:       "windows",
	}
	if err := manager.Enable(); err != nil {
		t.Fatal(err)
	}
	if backend.options.Identifier != manager.Identifier ||
		len(backend.options.Arguments) != 1 ||
		backend.options.Arguments[0] != "--hidden" {
		t.Fatalf("options = %#v", backend.options)
	}
	if enabled, err := manager.IsEnabled(); err != nil || !enabled {
		t.Fatalf("enabled = %v, err = %v", enabled, err)
	}
	if err := manager.Disable(); err != nil {
		t.Fatal(err)
	}
	if enabled, err := manager.IsEnabled(); err != nil || enabled {
		t.Fatalf("enabled after disable = %v, err = %v", enabled, err)
	}
}
