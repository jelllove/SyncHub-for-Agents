package updater

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func helperTestDirectory(t *testing.T) string {
	t.Helper()
	directory, err := os.MkdirTemp(".", "helper-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(directory) })
	directory, err = filepath.Abs(directory)
	if err != nil {
		t.Fatal(err)
	}
	return directory
}

func helperTestOptions(t *testing.T) helperOptions {
	t.Helper()
	directory := helperTestDirectory(t)
	cache := filepath.Join(directory, "private cache")
	install := filepath.Join(directory, "SyncHub 应用")
	for _, path := range []string{cache, install} {
		if err := os.Mkdir(path, 0700); err != nil {
			t.Fatal(err)
		}
	}
	data := []byte("test installer payload; never executable")
	hash := sha256.Sum256(data)
	o := helperOptions{
		parentPID: 123, installer: filepath.Join(cache, "installer-example.exe"),
		checksum: hex.EncodeToString(hash[:]), executable: filepath.Join(install, "SyncHub.exe"),
		restart: true, resultPath: filepath.Join(directory, "update-result.json"),
		handoffDir: filepath.Join(cache, "handoff-example"),
	}
	if err := os.WriteFile(o.installer, data, 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(o.handoffDir, 0700); err != nil {
		t.Fatal(err)
	}
	return o
}

func TestHelperArgumentRoundTrip(t *testing.T) {
	options := helperTestOptions(t)
	for _, restart := range []bool{false, true} {
		options.restart = restart
		parsed, err := parseHelperArgs(options.args())
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(parsed, options) {
			t.Fatalf("parsed options = %#v, want %#v", parsed, options)
		}
	}
	options.executable = filepath.Join(filepath.Dir(options.executable), "sYNChUB.EXE")
	if _, err := parseHelperArgs(options.args()); err != nil {
		t.Fatalf("case-insensitive executable name rejected: %v", err)
	}
}

func TestHelperRejectsMalformedArguments(t *testing.T) {
	options := helperTestOptions(t)
	tests := map[string]func([]string) []string{
		"missing":          func(a []string) []string { return a[:len(a)-1] },
		"extra":            func(a []string) []string { return append(a, "unexpected") },
		"unknown flag":     func(a []string) []string { a[3] = "--anything"; return a },
		"reordered flag":   func(a []string) []string { a[1], a[3] = a[3], a[1]; return a },
		"zero pid":         func(a []string) []string { a[2] = "0"; return a },
		"negative pid":     func(a []string) []string { a[2] = "-1"; return a },
		"overflow pid":     func(a []string) []string { a[2] = "4294967296"; return a },
		"invalid checksum": func(a []string) []string { a[6] = strings.Repeat("x", 64); return a },
		"short checksum":   func(a []string) []string { a[6] = "abcd"; return a },
		"boolean alias":    func(a []string) []string { a[10] = "1"; return a },
		"relative":         func(a []string) []string { a[4] = "installer.exe"; return a },
		"quote":            func(a []string) []string { a[4] += "\""; return a },
		"line feed":        func(a []string) []string { a[8] += "\n"; return a },
		"carriage return":  func(a []string) []string { a[12] += "\r"; return a },
		"nul":              func(a []string) []string { a[14] += "\x00"; return a },
		"wrong target": func(a []string) []string {
			a[8] = filepath.Join(filepath.Dir(a[8]), "not-synchub.exe")
			return a
		},
		"wrong installer": func(a []string) []string {
			a[4] = filepath.Join(filepath.Dir(a[4]), "installer.cmd")
			return a
		},
		"outside handoff": func(a []string) []string {
			a[14] = filepath.Join(filepath.Dir(a[8]), "handoff-example")
			return a
		},
		"overlapping result": func(a []string) []string { a[12] = a[4]; return a },
		"result removed with handshake": func(a []string) []string {
			a[12] = filepath.Join(a[14], "update-result.json")
			return a
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			if _, err := parseHelperArgs(mutate(options.args())); err == nil {
				t.Fatal("accepted malformed helper arguments")
			}
		})
	}
}

func TestHelperRoutingDoesNotConsumeNormalLaunch(t *testing.T) {
	for _, args := range [][]string{nil, {"--minimized"}, {"file.txt"}, {"--other", helperFlag}} {
		handled, err := RunHelper(args)
		if handled || err != nil {
			t.Fatalf("RunHelper(%v) = %v, %v", args, handled, err)
		}
	}
	handled, err := RunHelper([]string{helperFlag})
	if !handled || err == nil {
		t.Fatalf("malformed helper must be handled without starting UI: %v, %v", handled, err)
	}
}

type fakeHelperParent struct {
	events   *[]string
	onWait   func() error
	closeErr error
}

func (p fakeHelperParent) wait(timeout time.Duration) error {
	*p.events = append(*p.events, "wait")
	if timeout != 2*time.Minute {
		return errors.New("unexpected parent wait timeout")
	}
	if p.onWait != nil {
		return p.onWait()
	}
	return nil
}

func (p fakeHelperParent) close() error {
	*p.events = append(*p.events, "close")
	return p.closeErr
}

func helperTestOperations(t *testing.T, o helperOptions, events *[]string) helperOperations {
	t.Helper()
	return helperOperations{
		openParent: func(pid uint32) (helperParent, error) {
			if pid != o.parentPID {
				t.Fatalf("parent PID = %d, want %d", pid, o.parentPID)
			}
			*events = append(*events, "open")
			return fakeHelperParent{events: events}, nil
		},
		verify: func(path, checksum string) error {
			*events = append(*events, "verify")
			return verifyInstaller(path, checksum)
		},
		writable: func(directory string) error {
			*events = append(*events, "writable")
			return helperDirectoryWritable(directory)
		},
		ready: func(err error) error {
			if err == nil {
				*events = append(*events, "ready")
			} else {
				*events = append(*events, "ready-error")
			}
			return writeHelperStatus(filepath.Join(o.handoffDir, "ready.json"), err)
		},
		awaitProceed: func() error {
			*events = append(*events, "proceed")
			return nil
		},
		waitUnlocked: func(executable string, timeout time.Duration) error {
			if executable != o.executable || timeout != 30*time.Second {
				t.Fatalf("unexpected unlock parameters: %s, %s", executable, timeout)
			}
			*events = append(*events, "unlock")
			return nil
		},
		install: func(installer, directory string, timeout time.Duration) error {
			if installer != o.installer || directory != filepath.Dir(o.executable) || timeout != 10*time.Minute {
				t.Fatalf("unexpected install parameters: %s, %s, %s", installer, directory, timeout)
			}
			*events = append(*events, "install")
			return nil
		},
		relaunch: func(executable string) error {
			if executable != o.executable {
				t.Fatalf("relaunch = %s, want %s", executable, o.executable)
			}
			status, err := readHelperStatus(o.resultPath)
			if err != nil || status.Error != "" {
				t.Fatalf("successful result must be published before relaunch: %#v, %v", status, err)
			}
			*events = append(*events, "relaunch")
			return nil
		},
		result: func(err error) error {
			*events = append(*events, "result")
			return writeHelperStatus(o.resultPath, err)
		},
		removeInstall: func(path string) error {
			*events = append(*events, "remove")
			return os.Remove(path)
		},
	}
}

func TestHelperInstallOrderingAndQuitDoesNotRelaunch(t *testing.T) {
	for _, restart := range []bool{false, true} {
		t.Run(map[bool]string{false: "quit", true: "restart"}[restart], func(t *testing.T) {
			o := helperTestOptions(t)
			o.restart = restart
			var events []string
			if err := applyHelperUpdate(o, helperTestOperations(t, o, &events)); err != nil {
				t.Fatal(err)
			}
			want := []string{"open", "verify", "writable", "ready", "proceed", "wait", "close", "unlock", "verify", "install", "remove", "result"}
			if restart {
				want = append(want, "relaunch")
			}
			if !reflect.DeepEqual(events, want) {
				t.Fatalf("events = %v, want %v", events, want)
			}
			if _, err := os.Stat(o.installer); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("completed installer was not cleaned up: %v", err)
			}
		})
	}
}

func TestHelperFailuresNeverInstallOrRelaunchUnexpectedly(t *testing.T) {
	for _, failure := range []string{"open", "hash-before", "writable", "ready", "proceed", "wait", "close", "unlock", "hash-after", "installer-exit", "installer-timeout", "result"} {
		t.Run(failure, func(t *testing.T) {
			o := helperTestOptions(t)
			var events []string
			ops := helperTestOperations(t, o, &events)
			failed := errors.New(failure)
			switch failure {
			case "open":
				ops.openParent = func(uint32) (helperParent, error) { return nil, failed }
			case "hash-before":
				if err := os.WriteFile(o.installer, []byte("corrupted"), 0600); err != nil {
					t.Fatal(err)
				}
			case "writable":
				ops.writable = func(string) error { return failed }
			case "ready":
				ops.ready = func(error) error { return failed }
			case "proceed":
				ops.awaitProceed = func() error { return failed }
			case "close":
				ops.openParent = func(uint32) (helperParent, error) {
					return fakeHelperParent{events: &events, closeErr: failed}, nil
				}
			case "unlock":
				ops.waitUnlocked = func(string, time.Duration) error { return failed }
			case "wait", "hash-after":
				ops.openParent = func(uint32) (helperParent, error) {
					return fakeHelperParent{events: &events, onWait: func() error {
						if failure == "wait" {
							return failed
						}
						return os.WriteFile(o.installer, []byte("changed while waiting"), 0600)
					}}, nil
				}
			case "installer-exit", "installer-timeout":
				ops.install = func(string, string, time.Duration) error {
					events = append(events, "install")
					return failed
				}
			case "result":
				ops.result = func(error) error { return failed }
			}
			err := applyHelperUpdate(o, ops)
			if err == nil {
				t.Fatal("expected helper error")
			}
			for _, event := range events {
				if event == "relaunch" {
					t.Fatal("relaunched after failed update")
				}
				if event == "install" && failure != "installer-exit" && failure != "installer-timeout" && failure != "result" {
					t.Fatalf("started installer after %s failure", failure)
				}
			}
			if failure != "result" {
				status, readErr := readHelperStatus(o.resultPath)
				if readErr != nil || status.Error == "" {
					t.Fatalf("failure was not persisted: %#v, %v", status, readErr)
				}
			}
			if failure != "result" {
				if _, statErr := os.Stat(o.installer); statErr != nil {
					t.Fatalf("failed installer should remain available for diagnosis: %v", statErr)
				}
			}
		})
	}
}

func TestHelperRelaunchFailureIsRecorded(t *testing.T) {
	o := helperTestOptions(t)
	var events []string
	ops := helperTestOperations(t, o, &events)
	ops.relaunch = func(string) error { return errors.New("test restart failure") }
	if err := applyHelperUpdate(o, ops); err == nil {
		t.Fatal("expected restart error")
	}
	status, err := readHelperStatus(o.resultPath)
	if err != nil || !strings.Contains(status.Error, "test restart failure") {
		t.Fatalf("restart failure result = %#v, %v", status, err)
	}
}

func TestHelperAtomicResultReplacement(t *testing.T) {
	path := filepath.Join(helperTestDirectory(t), "update-result.json")
	for _, result := range []error{errors.New("installer exit status 3"), nil} {
		if err := writeHelperStatus(path, result); err != nil {
			t.Fatal(err)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var value map[string]string
		if err := json.Unmarshal(data, &value); err != nil {
			t.Fatal(err)
		}
		want := ""
		if result != nil {
			want = result.Error()
		}
		if len(value) != 1 || value["error"] != want {
			t.Fatalf("result = %s", data)
		}
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil || len(entries) != 1 {
		t.Fatalf("status left scratch files behind: %v, %v", entries, err)
	}
}

func TestHelperReadyHandshakeRequiresAcknowledgement(t *testing.T) {
	o := helperTestOptions(t)
	if err := writeHelperStatus(filepath.Join(o.handoffDir, "ready.json"), nil); err != nil {
		t.Fatal(err)
	}
	if err := waitHelperReady(o.handoffDir, make(chan error), time.Second); err != nil {
		t.Fatal(err)
	}
	if err := waitHelperProceed(o.handoffDir, time.Second); err != nil {
		t.Fatal(err)
	}
}

func TestHelperFailedHandshakeNeverAuthorizesInstallation(t *testing.T) {
	for _, failure := range []string{"refused", "malformed", "missing status", "exited", "timeout"} {
		t.Run(failure, func(t *testing.T) {
			o := helperTestOptions(t)
			exited := make(chan error, 1)
			switch failure {
			case "refused":
				if err := writeHelperStatus(filepath.Join(o.handoffDir, "ready.json"), errors.New("no parent handle")); err != nil {
					t.Fatal(err)
				}
			case "malformed":
				if err := os.WriteFile(filepath.Join(o.handoffDir, "ready.json"), []byte("{"), 0600); err != nil {
					t.Fatal(err)
				}
			case "missing status":
				if err := os.WriteFile(filepath.Join(o.handoffDir, "ready.json"), []byte("{}"), 0600); err != nil {
					t.Fatal(err)
				}
			case "exited":
				exited <- errors.New("exit status 2")
			}
			if err := waitHelperReady(o.handoffDir, exited, time.Millisecond); err == nil {
				t.Fatal("failed handshake accepted")
			}
			if _, err := os.Stat(filepath.Join(o.handoffDir, "proceed")); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("failed handshake wrote authorization: %v", err)
			}
		})
	}
}

func TestHelperProceedTimeoutAndCancellation(t *testing.T) {
	o := helperTestOptions(t)
	if err := waitHelperProceed(o.handoffDir, time.Millisecond); err == nil {
		t.Fatal("missing acknowledgement accepted")
	}
	if err := os.Remove(o.handoffDir); err != nil {
		t.Fatal(err)
	}
	if err := waitHelperProceed(o.handoffDir, time.Second); err == nil {
		t.Fatal("removed handshake directory accepted")
	}
}

func TestHelperCopyAndWritableProbe(t *testing.T) {
	directory := helperTestDirectory(t)
	source := filepath.Join(directory, "SyncHub.exe")
	data := []byte("not executable test contents")
	if err := os.WriteFile(source, data, 0600); err != nil {
		t.Fatal(err)
	}
	first, err := copyHelperExecutable(source, directory)
	if err != nil {
		t.Fatal(err)
	}
	second, err := copyHelperExecutable(source, directory)
	if err != nil {
		t.Fatal(err)
	}
	if first == second || filepath.Dir(first) != directory || !strings.HasPrefix(filepath.Base(first), "updater-helper-") {
		t.Fatalf("helper copies must be unique cache-local files: %s, %s", first, second)
	}
	copied, err := os.ReadFile(first)
	if err != nil || string(copied) != string(data) {
		t.Fatalf("copy = %q, %v", copied, err)
	}
	if err := helperDirectoryWritable(directory); err != nil {
		t.Fatal(err)
	}
	if err := helperDirectoryWritable(filepath.Join(directory, "missing")); err == nil {
		t.Fatal("missing install directory passed the write probe")
	}
	if err := helperRegularFile(directory); err == nil {
		t.Fatal("directory accepted as executable")
	}
}

func TestHelperCleanupOnlyRemovesAgedHelperExecutables(t *testing.T) {
	directory := helperTestDirectory(t)
	for _, name := range []string{"updater-helper-old.exe", "updater-helper-new.exe", "installer-old.exe", "updater-helper-old.txt"} {
		path := filepath.Join(directory, name)
		if err := os.WriteFile(path, []byte("never executable"), 0600); err != nil {
			t.Fatal(err)
		}
		if name != "updater-helper-new.exe" {
			past := time.Now().Add(-2 * helperStaleAge)
			if err := os.Chtimes(path, past, past); err != nil {
				t.Fatal(err)
			}
		}
	}
	for _, name := range []string{"handoff-old", "updater-helper-directory.exe"} {
		if err := os.Mkdir(filepath.Join(directory, name), 0700); err != nil {
			t.Fatal(err)
		}
	}
	if err := CleanupStaleHelpers(directory); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	want := []string{"handoff-old", "installer-old.exe", "updater-helper-directory.exe", "updater-helper-new.exe", "updater-helper-old.txt"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("cleanup retained %v, want %v", names, want)
	}
	if err := CleanupStaleHelpers(filepath.Join(directory, "missing")); err != nil {
		t.Fatalf("absent cache should be a no-op: %v", err)
	}
}
