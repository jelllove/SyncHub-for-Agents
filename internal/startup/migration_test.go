package startup

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRemoveLegacyEntries(t *testing.T) {
	tests := []struct {
		goos string
		path func(home string) string
	}{
		{
			goos: "windows",
			path: func(home string) string {
				appData := filepath.Join(home, "AppData", "Roaming")
				t.Setenv("APPDATA", appData)
				return filepath.Join(appData, "Microsoft", "Windows", "Start Menu", "Programs", "Startup", "acsync.cmd")
			},
		},
		{
			goos: "darwin",
			path: func(home string) string {
				return filepath.Join(home, "Library", "LaunchAgents", "com.acsync.agent.plist")
			},
		},
		{
			goos: "linux",
			path: func(home string) string {
				return filepath.Join(home, ".config", "systemd", "user", "acsync.service")
			},
		},
	}
	for _, test := range tests {
		t.Run(test.goos, func(t *testing.T) {
			home := t.TempDir()
			path := test.path(home)
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte("legacy"), 0o644); err != nil {
				t.Fatal(err)
			}
			var calls []string
			removed, err := RemoveLegacy(test.goos, home, func(name string, args ...string) error {
				calls = append(calls, name+" "+strings.Join(args, " "))
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			if len(removed) != 1 || removed[0] != path {
				t.Fatalf("removed = %#v", removed)
			}
			if _, err := os.Stat(path); !os.IsNotExist(err) {
				t.Fatalf("legacy entry still exists: %v", err)
			}
			wantCalls := 0
			if test.goos == "darwin" {
				wantCalls = 1
			}
			if test.goos == "linux" {
				wantCalls = 2
				if calls[0] != "systemctl --user disable --now acsync.service" ||
					calls[1] != "systemctl --user daemon-reload" {
					t.Fatalf("systemd calls = %#v", calls)
				}
			}
			if len(calls) != wantCalls {
				t.Fatalf("unregister calls = %#v", calls)
			}
			if _, err := RemoveLegacy(test.goos, home, nil); err != nil {
				t.Fatalf("idempotent removal: %v", err)
			}
		})
	}
}
