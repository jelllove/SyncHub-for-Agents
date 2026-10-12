package startup

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"
)

type preferenceBackend struct {
	fakeBackend
	enables    int
	disables   int
	readErr    error
	writeErr   error
	disableErr error
}

func (backend *preferenceBackend) EnableWithOptions(options application.AutostartOptions) error {
	backend.enables++
	if backend.writeErr != nil {
		return backend.writeErr
	}
	return backend.fakeBackend.EnableWithOptions(options)
}

func (backend *preferenceBackend) Disable() error {
	backend.disables++
	if backend.disableErr != nil {
		return backend.disableErr
	}
	if backend.writeErr != nil {
		return backend.writeErr
	}
	return backend.fakeBackend.Disable()
}

func (backend *preferenceBackend) IsEnabled() (bool, error) {
	return backend.enabled, backend.readErr
}

func startupFixture(t *testing.T) (*Manager, *preferenceBackend) {
	t.Helper()
	backend := &preferenceBackend{}
	return &Manager{
		Backend:         backend,
		Identifier:      "io.github.qinqingxu.synchub",
		Arguments:       []string{"--hidden"},
		GOOS:            "windows",
		PreferencesPath: filepath.Join(t.TempDir(), "startup-settings.json"),
	}, backend
}

func TestWindowsStartupIsEnabledByDefaultOnce(t *testing.T) {
	manager, backend := startupFixture(t)
	if err := manager.InitializeDefault(); err != nil {
		t.Fatal(err)
	}
	if !backend.enabled || backend.enables != 1 || backend.options.Identifier != manager.Identifier ||
		len(backend.options.Arguments) != 1 || backend.options.Arguments[0] != "--hidden" {
		t.Fatalf("startup registration = %#v", backend)
	}
	if err := manager.SetEnabled(false); err != nil {
		t.Fatal(err)
	}
	if err := manager.InitializeDefault(); err != nil {
		t.Fatal(err)
	}
	if backend.enabled || backend.enables != 1 {
		t.Fatal("explicit startup opt-out was overwritten")
	}
	if err := manager.SetEnabled(true); err != nil {
		t.Fatal(err)
	}
	backend.enabled = false // The user removes the entry outside SyncHub.
	if err := manager.InitializeDefault(); err != nil {
		t.Fatal(err)
	}
	if backend.enabled || backend.enables != 2 {
		t.Fatal("initialization must not recreate an externally removed entry")
	}
}

func TestWindowsStartupPreservesExistingRegistration(t *testing.T) {
	manager, backend := startupFixture(t)
	backend.enabled = true
	if err := manager.InitializeDefault(); err != nil {
		t.Fatal(err)
	}
	if backend.enables != 0 || !backend.enabled {
		t.Fatal("existing registration must not be duplicated or rewritten")
	}
}

func TestStartupDefaultIsWindowsOnly(t *testing.T) {
	for _, goos := range []string{"darwin", "linux"} {
		manager, backend := startupFixture(t)
		manager.GOOS = goos
		if err := manager.InitializeDefault(); err != nil {
			t.Fatal(err)
		}
		if backend.enables != 0 {
			t.Fatalf("changed %s startup defaults", goos)
		}
		if _, err := os.Stat(manager.PreferencesPath); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("created Windows preferences on %s: %v", goos, err)
		}
	}
}

func TestStartupDefaultDoesNotRegisterDevelopmentBuilds(t *testing.T) {
	manager, backend := startupFixture(t)
	manager.SkipDefault = true
	if err := manager.InitializeDefault(); err != nil {
		t.Fatal(err)
	}
	if backend.enabled || backend.enables != 0 {
		t.Fatal("development launch automatically registered Windows startup")
	}
	if _, err := os.Stat(manager.PreferencesPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("development launch wrote startup preferences: %v", err)
	}
	if err := manager.SetEnabled(true); err != nil {
		t.Fatal(err)
	}
	if !backend.enabled {
		t.Fatal("explicit startup choices should still work in developer builds")
	}
}
func TestWindowsStartupPreferenceFailuresAreVisibleAndRolledBack(t *testing.T) {
	for _, failure := range []string{"read registry", "write registry", "invalid preferences", "save preferences"} {
		t.Run(failure, func(t *testing.T) {
			manager, backend := startupFixture(t)
			switch failure {
			case "read registry":
				backend.readErr = errors.New("registry unavailable")
			case "write registry":
				backend.writeErr = errors.New("registry denied")
			case "invalid preferences":
				if err := os.WriteFile(manager.PreferencesPath, []byte(`{"enabled":"bad"}`), 0o600); err != nil {
					t.Fatal(err)
				}
			case "save preferences":
				blocker := filepath.Join(filepath.Dir(manager.PreferencesPath), "not-a-directory")
				if err := os.WriteFile(blocker, nil, 0o600); err != nil {
					t.Fatal(err)
				}
				manager.PreferencesPath = filepath.Join(blocker, "startup-settings.json")
			}
			if err := manager.InitializeDefault(); err == nil {
				t.Fatal("expected an explicit startup initialization error")
			}
			if backend.enabled {
				t.Fatal("failed initialization left a new startup entry behind")
			}
		})
	}
}

func TestWindowsStartupSaveFailurePreservesThePreviousState(t *testing.T) {
	manager, backend := startupFixture(t)
	backend.enabled = true
	blocker := filepath.Join(filepath.Dir(manager.PreferencesPath), "not-a-directory")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	manager.PreferencesPath = filepath.Join(blocker, "startup-settings.json")
	if err := manager.SetEnabled(false); err == nil {
		t.Fatal("saving a startup opt-out must not report success without persistence")
	}
	if !backend.enabled {
		t.Fatal("failed preference write did not restore the existing registration")
	}
}

func TestWindowsStartupReportsRollbackFailure(t *testing.T) {
	manager, backend := startupFixture(t)
	backend.disableErr = errors.New("registry rollback denied")
	blocker := filepath.Join(filepath.Dir(manager.PreferencesPath), "not-a-directory")
	if err := os.WriteFile(blocker, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	manager.PreferencesPath = filepath.Join(blocker, "startup-settings.json")
	err := manager.SetEnabled(true)
	if err == nil || !errors.Is(err, backend.disableErr) {
		t.Fatalf("rollback failure was not surfaced: %v", err)
	}
}

func TestAutostartWriterReplacesFilesWithoutDeletingFailedDestinations(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "settings.json")
	if err := os.WriteFile(path, []byte("previous"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeFile(path, []byte("replacement"), 0o600); err != nil {
		t.Fatal(err)
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != "replacement" {
		t.Fatalf("replacement = %s, error = %v", data, err)
	}
	directory := filepath.Join(root, "blocked")
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	sentinel := filepath.Join(directory, "keep")
	if err := os.WriteFile(sentinel, []byte("previous"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := writeFile(directory, []byte("replacement"), 0o600); err == nil {
		t.Fatal("directory destination should fail explicitly")
	}
	if data, err := os.ReadFile(sentinel); err != nil || string(data) != "previous" {
		t.Fatalf("failed destination changed: %s, error = %v", data, err)
	}
	temps, err := filepath.Glob(filepath.Join(root, ".autostart-*"))
	if err != nil || len(temps) != 0 {
		t.Fatalf("temporary writes were not cleaned up: %v, %v", temps, err)
	}
}
