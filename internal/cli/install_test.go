package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunInstallUninstallWindows(t *testing.T) {
	userHome := t.TempDir()
	t.Setenv("APPDATA", filepath.Join(userHome, "AppData", "Roaming"))

	path, err := RunInstall("windows", userHome, `C:\Program Files\acsync\acsync.exe`)
	if err != nil {
		t.Fatalf("RunInstall error: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("entry not written: %v", err)
	}
	if !strings.Contains(string(data), `"C:\Program Files\acsync\acsync.exe" daemon`) {
		t.Errorf("entry body = %q", string(data))
	}

	if err := RunUninstall("windows", userHome); err != nil {
		t.Fatalf("RunUninstall error: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("entry should be removed after uninstall")
	}
}
