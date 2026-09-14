//go:build windows

package updater

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const helperIntegrationRootEnv = "SYNCHUB_HELPER_INTEGRATION_ROOT"

// The copied test executable routes the same helper entry point as main, but
// contains no GUI. An accidental restart is recorded rather than launching UI.
func TestMain(m *testing.M) {
	if handled, err := RunHelper(os.Args[1:]); handled {
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		os.Exit(0)
	}
	if root := os.Getenv(helperIntegrationRootEnv); root != "" && len(os.Args) == 1 {
		_ = os.WriteFile(filepath.Join(root, "unexpected-restart"), []byte("restarted"), 0600)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

type helperIntegrationConfig struct {
	Pending    pendingUpdate
	ResultPath string
	Mode       string
	Restart    bool
}

func TestHelperIntegrationParent(t *testing.T) {
	root := os.Getenv(helperIntegrationRootEnv)
	if root == "" {
		t.Skip("isolated integration subprocess only")
	}
	var config helperIntegrationConfig
	data, err := os.ReadFile(filepath.Join(root, "parent-config.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &config); err != nil {
		t.Fatal(err)
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	launchErr := LaunchInstaller(config.Pending, executable, config.Restart, config.ResultPath)
	if err := writeHelperStatus(filepath.Join(root, "parent-ready.json"), launchErr); err != nil {
		t.Fatal(err)
	}
	if launchErr != nil {
		t.Fatal(launchErr)
	}
	deadline := time.Now().Add(20 * time.Second)
	for {
		if _, err := os.Stat(filepath.Join(root, "release-parent")); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("test parent was not released")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if config.Mode == "tamper" {
		file, err := os.OpenFile(config.Pending.Path, os.O_WRONLY|os.O_APPEND, 0600)
		if err != nil {
			t.Fatal(err)
		}
		_, writeErr := file.Write([]byte("modified after helper readiness"))
		if err := errors.Join(writeErr, file.Close()); err != nil {
			t.Fatal(err)
		}
	}
}

func TestHelperRealInstallerHandoff(t *testing.T) {
	if os.Getenv("SYNCHUB_RUN_HELPER_INTEGRATION") != "1" {
		t.Skip("set SYNCHUB_RUN_HELPER_INTEGRATION=1 to use the local harmless NSIS fixture")
	}
	compiler, err := filepath.Abs(filepath.Join("..", "..", "bin", "tools", "nsis", "makensis.exe"))
	if err != nil {
		t.Fatal(err)
	}
	if err := helperRegularFile(compiler); err != nil {
		t.Fatalf("local NSIS compiler is required: %v", err)
	}
	for _, mode := range []string{"success-quit", "tamper", "installer-failure"} {
		t.Run(mode, func(t *testing.T) {
			o := helperTestOptions(t)
			root := filepath.Dir(o.resultPath)
			scratch := filepath.Join(root, "scratch")
			if err := os.Mkdir(scratch, 0700); err != nil {
				t.Fatal(err)
			}
			env := append(os.Environ(), "TEMP="+scratch, "TMP="+scratch, helperIntegrationRootEnv+"="+root)
			code := 0
			if mode == "installer-failure" {
				code = 17
			}
			script := fmt.Sprintf(`Unicode true
Name "SyncHub isolated helper validation"
OutFile "%s"
RequestExecutionLevel user
SilentInstall silent
AutoCloseWindow true
InstallDir "$EXEDIR\fallback-target"
Section
    SetOutPath "$INSTDIR"
    FileOpen $0 "$INSTDIR\fixture-installed.txt" w
    FileWrite $0 "isolated fixture only"
    FileClose $0
    SetErrorLevel %d
SectionEnd
`, strings.ReplaceAll(o.installer, "$", "$$"), code)
			scriptPath := filepath.Join(root, "fixture.nsi")
			if err := os.WriteFile(scriptPath, []byte(script), 0600); err != nil {
				t.Fatal(err)
			}
			build := exec.Command(compiler, "/V2", scriptPath)
			build.Env = env
			if output, err := build.CombinedOutput(); err != nil {
				t.Fatalf("compile isolated NSIS fixture: %v\n%s", err, output)
			}
			payload, err := os.ReadFile(o.installer)
			if err != nil {
				t.Fatal(err)
			}
			hash := sha256.Sum256(payload)
			config := helperIntegrationConfig{
				Pending: pendingUpdate{
					Path: o.installer, Checksum: hex.EncodeToString(hash[:]), Version: "v0.3.0",
				},
				ResultPath: o.resultPath, Mode: mode, Restart: mode != "success-quit",
			}
			if err := saveJSON(filepath.Join(root, "parent-config.json"), config); err != nil {
				t.Fatal(err)
			}
			current, err := os.Executable()
			if err != nil {
				t.Fatal(err)
			}
			copy, err := copyHelperExecutable(current, filepath.Dir(o.executable))
			if err != nil {
				t.Fatal(err)
			}
			if err := os.Rename(copy, o.executable); err != nil {
				t.Fatal(err)
			}
			parent := exec.Command(o.executable, "-test.run=^TestHelperIntegrationParent$")
			parent.Env = env
			var output bytes.Buffer
			parent.Stdout, parent.Stderr = &output, &output
			if err := parent.Start(); err != nil {
				t.Fatal(err)
			}
			parentDone := make(chan struct{})
			var parentErr error
			go func() {
				parentErr = parent.Wait()
				close(parentDone)
			}()
			t.Cleanup(func() {
				_ = os.WriteFile(filepath.Join(root, "release-parent"), []byte("exit"), 0600)
				select {
				case <-parentDone:
				case <-time.After(30 * time.Second):
					t.Error("isolated parent did not exit naturally")
				}
				helperIntegrationAwaitHelperExit(t, filepath.Dir(o.installer))
			})
			ready := helperIntegrationAwaitStatus(t, filepath.Join(root, "parent-ready.json"))
			if ready.Error != "" {
				t.Fatalf("real launch handshake failed: %s", ready.Error)
			}
			installedMarker := filepath.Join(filepath.Dir(o.executable), "fixture-installed.txt")
			time.Sleep(400 * time.Millisecond)
			select {
			case <-parentDone:
				t.Fatalf("parent exited before release: %v\n%s", parentErr, output.String())
			default:
			}
			for _, path := range []string{installedMarker, o.resultPath} {
				if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
					t.Fatalf("helper ran before parent exit: %s (%v)", path, err)
				}
			}
			if err := os.WriteFile(filepath.Join(root, "release-parent"), []byte("exit"), 0600); err != nil {
				t.Fatal(err)
			}
			select {
			case <-parentDone:
				if parentErr != nil {
					t.Fatalf("parent failed: %v\n%s", parentErr, output.String())
				}
			case <-time.After(30 * time.Second):
				t.Fatal("parent did not exit naturally")
			}
			result := helperIntegrationAwaitStatus(t, o.resultPath)
			switch mode {
			case "success-quit":
				if result.Error != "" {
					t.Fatalf("real fixture installation failed: %s", result.Error)
				}
			case "tamper":
				if !strings.Contains(result.Error, "SHA-256") {
					t.Fatalf("tampered installer was not rejected: %#v", result)
				}
			case "installer-failure":
				if !strings.Contains(result.Error, "exit status 17") {
					t.Fatalf("NSIS nonzero exit was not persisted: %#v", result)
				}
			}
			data, markerErr := os.ReadFile(installedMarker)
			if mode == "tamper" {
				if !errors.Is(markerErr, os.ErrNotExist) {
					t.Fatalf("tampered installer executed: %v", markerErr)
				}
			} else if markerErr != nil || string(data) != "isolated fixture only" {
				t.Fatalf("NSIS did not honor final unquoted /D with spaces: %q, %v", data, markerErr)
			}
			helperIntegrationAwaitHelperExit(t, filepath.Dir(o.installer))
			if _, err := os.Stat(filepath.Join(root, "unexpected-restart")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("helper unexpectedly restarted target: %v", err)
			}
		})
	}
}

func helperIntegrationAwaitStatus(t *testing.T, path string) helperStatus {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for {
		status, err := readHelperStatus(path)
		if err == nil {
			return status
		}
		if !helperStatusPending(err) || time.Now().After(deadline) {
			t.Fatalf("waiting for isolated helper status %s: %v", path, err)
		}
		time.Sleep(20 * time.Millisecond)
	}
}

func helperIntegrationAwaitHelperExit(t *testing.T, cache string) {
	t.Helper()
	entries, err := os.ReadDir(cache)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), "updater-helper-") && strings.HasSuffix(entry.Name(), ".exe") {
			if err := waitHelperExecutableUnlocked(filepath.Join(cache, entry.Name()), 10*time.Second); err != nil {
				t.Fatalf("isolated copied helper did not finish: %v", err)
			}
		}
	}
}
