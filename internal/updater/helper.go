package updater

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	helperFlag             = "--apply-update"
	helperHandshakeTimeout = 15 * time.Second
	helperParentTimeout    = 2 * time.Minute
	helperUnlockTimeout    = 30 * time.Second
	helperInstallTimeout   = 10 * time.Minute
	helperPollInterval     = 50 * time.Millisecond
	helperStaleAge         = 24 * time.Hour
)

type helperOptions struct {
	parentPID  uint32
	installer  string
	checksum   string
	executable string
	restart    bool
	resultPath string
	handoffDir string
}

type helperStatus struct {
	Error string `json:"error"`
}

// RunHelper must run before creating the application or starting background work.
// A handled command, including a malformed helper command, must not start the UI.
func RunHelper(args []string) (handled bool, err error) {
	if len(args) == 0 || args[0] != helperFlag {
		return false, nil
	}
	options, err := parseHelperArgs(args)
	if err != nil {
		return true, err
	}
	return true, runPlatformHelper(options)
}

func (o helperOptions) args() []string {
	return []string{
		helperFlag,
		"--parent-pid", strconv.FormatUint(uint64(o.parentPID), 10),
		"--installer", o.installer,
		"--sha256", o.checksum,
		"--executable", o.executable,
		"--restart", strconv.FormatBool(o.restart),
		"--result", o.resultPath,
		"--handoff", o.handoffDir,
	}
}

func parseHelperArgs(args []string) (helperOptions, error) {
	var o helperOptions
	keys := []string{helperFlag, "--parent-pid", "--installer", "--sha256", "--executable", "--restart", "--result", "--handoff"}
	if len(args) != 15 || args[0] != keys[0] {
		return o, errors.New("invalid updater helper arguments")
	}
	for i, key := range keys[1:] {
		if args[1+i*2] != key {
			return o, fmt.Errorf("invalid updater helper arguments: expected %s", key)
		}
	}
	pid, err := strconv.ParseUint(args[2], 10, 32)
	if err != nil || pid == 0 {
		return o, errors.New("invalid updater parent process ID")
	}
	if args[10] != "true" && args[10] != "false" {
		return o, errors.New("invalid updater restart flag")
	}
	o = helperOptions{
		parentPID: uint32(pid), installer: args[4], checksum: args[6],
		executable: args[8], restart: args[10] == "true",
		resultPath: args[12], handoffDir: args[14],
	}
	return o, o.validate()
}

func validateHelperPath(name, path string) error {
	if !filepath.IsAbs(path) || strings.ContainsAny(path, "\"\r\n\x00") {
		return fmt.Errorf("invalid %s path: expected an absolute path without quotes or line breaks", name)
	}
	if filepath.Clean(path) != path {
		return fmt.Errorf("invalid %s path: expected a clean absolute path", name)
	}
	return nil
}

func (o helperOptions) validate() error {
	for _, entry := range []struct{ name, path string }{
		{"installer", o.installer}, {"executable", o.executable},
		{"result", o.resultPath}, {"handoff", o.handoffDir},
	} {
		if err := validateHelperPath(entry.name, entry.path); err != nil {
			return err
		}
	}
	if !strings.EqualFold(filepath.Base(o.executable), "SyncHub.exe") {
		return errors.New("automatic installation requires an executable named SyncHub.exe")
	}
	if !strings.EqualFold(filepath.Ext(o.installer), ".exe") {
		return errors.New("cached Windows installer must be an .exe file")
	}
	if filepath.Dir(o.executable) == filepath.VolumeName(o.executable)+string(filepath.Separator) {
		return errors.New("refusing to install SyncHub in a filesystem root")
	}
	raw, err := hex.DecodeString(o.checksum)
	if err != nil || len(raw) != sha256.Size {
		return errors.New("invalid expected installer SHA-256 checksum")
	}
	if !strings.EqualFold(filepath.Dir(o.handoffDir), filepath.Dir(o.installer)) ||
		!strings.HasPrefix(filepath.Base(o.handoffDir), "handoff-") {
		return errors.New("updater handoff must be a private directory beside the cached installer")
	}
	if strings.EqualFold(o.resultPath, o.installer) || strings.EqualFold(o.resultPath, o.executable) ||
		strings.EqualFold(o.installer, o.executable) ||
		strings.EqualFold(filepath.Dir(o.installer), filepath.Dir(o.executable)) ||
		strings.EqualFold(filepath.Dir(o.resultPath), o.handoffDir) {
		return errors.New("updater paths must not overlap")
	}
	return nil
}

func helperRegularFile(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("not a regular file: %s", path)
	}
	return nil
}

func helperDirectoryWritable(directory string) error {
	file, err := os.CreateTemp(directory, ".synchub-write-test-*")
	if err != nil {
		return fmt.Errorf("update directory is not writable (%s): %w", directory, err)
	}
	closeErr := file.Close()
	removeErr := os.Remove(file.Name())
	return errors.Join(closeErr, removeErr)
}

// CleanupStaleHelpers can run at startup against the dedicated updates cache.
// It never removes installers or recent helper copies that may be starting.
func CleanupStaleHelpers(directory string) error {
	if err := validateHelperPath("update cache", directory); err != nil {
		return err
	}
	info, err := os.Lstat(directory)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("update cache must be a directory, not a link")
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		return err
	}
	cutoff := time.Now().Add(-helperStaleAge)
	var failures []error
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, "updater-helper-") || !strings.HasSuffix(name, ".exe") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				failures = append(failures, err)
			}
			continue
		}
		if !info.Mode().IsRegular() || !info.ModTime().Before(cutoff) {
			continue
		}
		if err := os.Remove(filepath.Join(directory, name)); err != nil && !errors.Is(err, os.ErrNotExist) {
			// Windows refuses deletion of a still-running helper. Leave it for
			// another startup rather than stopping any process.
			failures = append(failures, err)
		}
	}
	return errors.Join(failures...)
}

func copyHelperExecutable(executable, directory string) (path string, err error) {
	source, err := os.Open(executable)
	if err != nil {
		return "", fmt.Errorf("open running executable: %w", err)
	}
	defer source.Close()
	target, err := os.CreateTemp(directory, "updater-helper-*.exe")
	if err != nil {
		return "", fmt.Errorf("create updater helper: %w", err)
	}
	path = target.Name()
	defer func() {
		target.Close()
		if err != nil {
			os.Remove(path)
		}
	}()
	if _, err = io.Copy(target, source); err != nil {
		return path, fmt.Errorf("copy updater helper: %w", err)
	}
	if err = target.Sync(); err != nil {
		return path, err
	}
	err = target.Close()
	return path, err
}

func writeHelperStatus(path string, result error) error {
	status := helperStatus{}
	if result != nil {
		status.Error = result.Error()
	}
	data, err := json.Marshal(status)
	if err != nil {
		return err
	}
	return writeHelperFile(path, data)
}

func writeHelperFile(path string, data []byte) (err error) {
	file, err := os.CreateTemp(filepath.Dir(path), ".update-status-*")
	if err != nil {
		return err
	}
	defer func() {
		file.Close()
		os.Remove(file.Name())
	}()
	if _, err = file.Write(data); err != nil {
		return err
	}
	if err = file.Sync(); err != nil {
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	return replaceHelperFile(file.Name(), path)
}

func readHelperStatus(path string) (helperStatus, error) {
	var status helperStatus
	file, err := os.Open(path)
	if err != nil {
		return status, err
	}
	defer file.Close()
	data, err := readLimited(file, 64<<10)
	if err != nil {
		return status, err
	}
	var response struct {
		Error *string `json:"error"`
	}
	if err := json.Unmarshal(data, &response); err != nil {
		return status, fmt.Errorf("invalid updater readiness response: %w", err)
	}
	if response.Error == nil {
		return status, errors.New("invalid updater readiness response: missing error field")
	}
	status.Error = *response.Error
	return status, nil
}

func waitHelperReady(directory string, exited <-chan error, timeout time.Duration) error {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	ticker := time.NewTicker(helperPollInterval)
	defer ticker.Stop()
	for {
		select {
		case err := <-exited:
			if err != nil {
				return fmt.Errorf("updater helper exited before handoff: %w", err)
			}
			return errors.New("updater helper exited before handoff")
		default:
		}
		status, err := readHelperStatus(filepath.Join(directory, "ready.json"))
		if err == nil {
			if status.Error != "" {
				return fmt.Errorf("updater helper refused handoff: %s", status.Error)
			}
			// A helper may install only after this acknowledgement. A failed
			// launch handshake must never become an update on a later Quit.
			if err := writeHelperFile(filepath.Join(directory, "proceed"), []byte("proceed")); err != nil {
				return fmt.Errorf("acknowledge updater handoff: %w", err)
			}
			return nil
		}
		if !helperStatusPending(err) {
			return fmt.Errorf("read updater readiness: %w", err)
		}
		select {
		case err := <-exited:
			return fmt.Errorf("updater helper exited before readiness (exit error: %v)", err)
		case <-timer.C:
			return errors.New("timed out waiting for updater helper readiness; no update was authorized")
		case <-ticker.C:
		}
	}
}

func waitHelperProceed(directory string, timeout time.Duration) error {
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	ticker := time.NewTicker(helperPollInterval)
	defer ticker.Stop()
	for {
		data, err := os.ReadFile(filepath.Join(directory, "proceed"))
		if err == nil {
			if string(data) != "proceed" {
				return errors.New("invalid updater handoff acknowledgement")
			}
			return nil
		}
		if !helperStatusPending(err) {
			return fmt.Errorf("read updater acknowledgement: %w", err)
		}
		if _, err := os.Stat(directory); err != nil {
			return errors.New("updater handoff was cancelled")
		}
		select {
		case <-timer.C:
			return errors.New("timed out waiting for updater handoff acknowledgement")
		case <-ticker.C:
		}
	}
}

type helperParent interface {
	wait(time.Duration) error
	close() error
}

type helperOperations struct {
	openParent    func(uint32) (helperParent, error)
	verify        func(string, string) error
	writable      func(string) error
	ready         func(error) error
	awaitProceed  func() error
	waitUnlocked  func(string, time.Duration) error
	install       func(string, string, time.Duration) error
	relaunch      func(string) error
	result        func(error) error
	removeInstall func(string) error
}

func applyHelperUpdate(o helperOptions, ops helperOperations) (err error) {
	ready := false
	defer func() {
		if err != nil {
			if writeErr := ops.result(err); writeErr != nil {
				err = errors.Join(err, fmt.Errorf("save update failure: %w", writeErr))
			}
			if !ready {
				if readyErr := ops.ready(err); readyErr != nil {
					err = errors.Join(err, fmt.Errorf("report updater readiness failure: %w", readyErr))
				}
			}
		}
	}()
	parent, err := ops.openParent(o.parentPID)
	if err != nil {
		return fmt.Errorf("open updater parent process: %w", err)
	}
	parentClosed := false
	defer func() {
		if !parentClosed {
			parent.close()
		}
	}()
	if err := ops.verify(o.installer, o.checksum); err != nil {
		return fmt.Errorf("verify cached installer before handoff: %w", err)
	}
	if err := ops.writable(filepath.Dir(o.executable)); err != nil {
		return fmt.Errorf("installation directory is not writable: %w", err)
	}
	if err := ops.ready(nil); err != nil {
		return fmt.Errorf("signal updater readiness: %w", err)
	}
	ready = true
	if err := ops.awaitProceed(); err != nil {
		return err
	}
	if err := parent.wait(helperParentTimeout); err != nil {
		return fmt.Errorf("wait for SyncHub to exit: %w", err)
	}
	parentClosed = true
	if err := parent.close(); err != nil {
		return fmt.Errorf("close exited SyncHub process handle: %w", err)
	}
	if err := ops.waitUnlocked(o.executable, helperUnlockTimeout); err != nil {
		return fmt.Errorf("wait for SyncHub executable to become writable: %w", err)
	}
	if err := ops.verify(o.installer, o.checksum); err != nil {
		return fmt.Errorf("verify cached installer after exit: %w", err)
	}
	if err := ops.install(o.installer, filepath.Dir(o.executable), helperInstallTimeout); err != nil {
		return fmt.Errorf("install update: %w", err)
	}
	_ = ops.removeInstall(o.installer)
	// Publish completion before relaunch, so the next app startup can consume it.
	if err := ops.result(nil); err != nil {
		return fmt.Errorf("save update result: %w", err)
	}
	if o.restart {
		if err := ops.relaunch(o.executable); err != nil {
			return fmt.Errorf("restart SyncHub after update: %w", err)
		}
	}
	return nil
}
