//go:build windows

package updater

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
)

// LaunchInstaller returns only after the copied helper has opened a handle to
// this process and accepted the update. The caller must then exit normally.
func LaunchInstaller(pending pendingUpdate, executable string, restart bool, resultPath string) (err error) {
	if runtime.GOARCH != "amd64" {
		return errors.New("automatic installation is supported only on Windows amd64")
	}
	options := helperOptions{
		parentPID: uint32(os.Getpid()), installer: pending.Path, checksum: pending.Checksum,
		executable: executable, restart: restart, resultPath: resultPath,
		handoffDir: filepath.Join(filepath.Dir(pending.Path), "handoff-validation"),
	}
	if err := options.validate(); err != nil {
		return err
	}
	if _, err := helperInstallerCommandLine(options.installer, filepath.Dir(executable)); err != nil {
		return err
	}
	if err := helperRegularFile(pending.Path); err != nil {
		return fmt.Errorf("invalid cached installer: %w", err)
	}
	if err := verifyInstaller(pending.Path, pending.Checksum); err != nil {
		return fmt.Errorf("refusing to launch updater: %w", err)
	}
	current, err := os.Executable()
	if err != nil {
		return fmt.Errorf("locate running executable: %w", err)
	}
	currentInfo, err := os.Stat(current)
	if err != nil {
		return err
	}
	targetInfo, err := os.Stat(executable)
	if err != nil || !os.SameFile(currentInfo, targetInfo) {
		return errors.New("automatic installation must target the running SyncHub.exe")
	}
	if err := helperDirectoryWritable(filepath.Dir(executable)); err != nil {
		return fmt.Errorf("installation directory is not writable; update manually: %w", err)
	}
	if err := helperDirectoryWritable(filepath.Dir(resultPath)); err != nil {
		return fmt.Errorf("cannot record update results: %w", err)
	}
	directory := filepath.Dir(pending.Path)
	if err := makeHelperCachePrivate(directory); err != nil {
		return fmt.Errorf("secure private update cache: %w", err)
	}
	options.handoffDir, err = os.MkdirTemp(directory, "handoff-")
	if err != nil {
		return fmt.Errorf("create updater handshake: %w", err)
	}
	defer func() {
		if err != nil {
			os.RemoveAll(options.handoffDir)
		}
	}()
	helperPath, err := copyHelperExecutable(current, directory)
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			os.Remove(helperPath)
		}
	}()
	cmd := exec.Command(helperPath, options.args()...)
	cmd.Dir = directory
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: windows.CREATE_NEW_PROCESS_GROUP | windows.CREATE_NO_WINDOW,
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start updater helper: %w", err)
	}
	exited := make(chan error, 1)
	go func() {
		exited <- cmd.Wait()
		os.Remove(helperPath)
	}()
	return waitHelperReady(options.handoffDir, exited, helperHandshakeTimeout)
}

func makeHelperCachePrivate(directory string) error {
	info, err := os.Lstat(directory)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("update cache must be a directory, not a link")
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return err
	}
	descriptor, err := windows.SecurityDescriptorFromString(
		"D:P(A;OICI;FA;;;SY)(A;OICI;FA;;;" + user.User.Sid.String() + ")",
	)
	if err != nil {
		return err
	}
	acl, _, err := descriptor.DACL()
	if err != nil {
		return err
	}
	// Windows ignores Unix 0700 modes. Protect this dedicated cache and let
	// the helper executable and handshake files inherit the user's ACL.
	return windows.SetNamedSecurityInfo(directory, windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil, nil, acl, nil)
}

func runPlatformHelper(o helperOptions) error {
	if runtime.GOARCH != "amd64" {
		return errors.New("automatic installation is supported only on Windows amd64")
	}
	if o.parentPID == uint32(os.Getpid()) {
		return errors.New("updater helper cannot wait for itself")
	}
	if _, err := helperInstallerCommandLine(o.installer, filepath.Dir(o.executable)); err != nil {
		return err
	}
	current, err := os.Executable()
	if err != nil {
		return err
	}
	if !strings.EqualFold(filepath.Dir(current), filepath.Dir(o.installer)) ||
		!strings.HasPrefix(filepath.Base(current), "updater-helper-") {
		return errors.New("updater helper must be a copied executable in the update cache")
	}
	info, err := os.Lstat(o.handoffDir)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("updater handshake directory is missing or invalid")
	}
	defer os.RemoveAll(o.handoffDir)
	return applyHelperUpdate(o, helperOperations{
		openParent: openHelperParent,
		verify: func(path, checksum string) error {
			if err := helperRegularFile(path); err != nil {
				return err
			}
			return verifyInstaller(path, checksum)
		},
		writable: helperDirectoryWritable,
		ready: func(err error) error {
			return writeHelperStatus(filepath.Join(o.handoffDir, "ready.json"), err)
		},
		awaitProceed:  func() error { return waitHelperProceed(o.handoffDir, helperHandshakeTimeout) },
		waitUnlocked:  waitHelperExecutableUnlocked,
		install:       runHelperInstaller,
		relaunch:      relaunchHelperTarget,
		result:        func(err error) error { return writeHelperStatus(o.resultPath, err) },
		removeInstall: os.Remove,
	})
}

type windowsHelperParent struct {
	handle windows.Handle
}

func openHelperParent(pid uint32) (helperParent, error) {
	handle, err := windows.OpenProcess(windows.SYNCHRONIZE, false, pid)
	if err != nil {
		return nil, err
	}
	status, err := windows.WaitForSingleObject(handle, 0)
	if err != nil || status != uint32(windows.WAIT_TIMEOUT) {
		windows.CloseHandle(handle)
		return nil, errors.New("parent process has already exited or cannot be waited on")
	}
	return &windowsHelperParent{handle: handle}, nil
}

func (p *windowsHelperParent) wait(timeout time.Duration) error {
	status, err := windows.WaitForSingleObject(p.handle, uint32(timeout/time.Millisecond))
	if err != nil {
		return err
	}
	switch status {
	case windows.WAIT_OBJECT_0:
		return nil
	case uint32(windows.WAIT_TIMEOUT):
		return errors.New("parent process did not exit within the update timeout; it was not terminated")
	default:
		return fmt.Errorf("unexpected parent process wait status: %d", status)
	}
}

func (p *windowsHelperParent) close() error {
	return windows.CloseHandle(p.handle)
}

func waitHelperExecutableUnlocked(executable string, timeout time.Duration) error {
	path, err := windows.UTF16PtrFromString(executable)
	if err != nil {
		return err
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	ticker := time.NewTicker(helperPollInterval)
	defer ticker.Stop()
	for {
		// OPEN_EXISTING does not truncate or change the executable. Opening for
		// write catches image mappings and handles left briefly after exit.
		handle, err := windows.CreateFile(path, windows.GENERIC_WRITE,
			windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE,
			nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
		if err == nil {
			return windows.CloseHandle(handle)
		}
		if errors.Is(err, windows.ERROR_FILE_NOT_FOUND) {
			return nil
		}
		if !errors.Is(err, windows.ERROR_SHARING_VIOLATION) &&
			!errors.Is(err, windows.ERROR_LOCK_VIOLATION) &&
			!errors.Is(err, windows.ERROR_ACCESS_DENIED) {
			return err
		}
		select {
		case <-timer.C:
			return fmt.Errorf("SyncHub.exe remained locked or write-protected; no installer was started: %w", err)
		case <-ticker.C:
		}
	}
}

func helperInstallerCommandLine(installer, directory string) (string, error) {
	for _, entry := range []struct{ name, path string }{{"installer", installer}, {"installation directory", directory}} {
		if err := validateHelperPath(entry.name, entry.path); err != nil {
			return "", err
		}
		// NSIS /D is a literal directory, not a shell or CRT-quoted argument.
		// Restrict it to ordinary local drive paths supported by the installer.
		volume := filepath.VolumeName(entry.path)
		if len(volume) != 2 || volume[1] != ':' || strings.Contains(entry.path[len(volume):], ":") ||
			strings.HasSuffix(entry.path, " ") || strings.HasSuffix(entry.path, ".") {
			return "", fmt.Errorf("unsupported %s path for the Windows installer", entry.name)
		}
	}
	return windows.EscapeArg(installer) + " /S /D=" + directory, nil
}

func runHelperInstaller(installer, directory string, timeout time.Duration) error {
	commandLine, err := helperInstallerCommandLine(installer, directory)
	if err != nil {
		return err
	}
	cmd := exec.Command(installer)
	cmd.Dir = filepath.Dir(installer)
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CmdLine: commandLine, CreationFlags: windows.CREATE_NO_WINDOW,
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case err := <-exited:
		if err != nil {
			return fmt.Errorf("Windows installer failed: %w", err)
		}
		return nil
	case <-timer.C:
		return errors.New("Windows installer timed out; it was not terminated and SyncHub was not restarted")
	}
}

func relaunchHelperTarget(executable string) error {
	cmd := exec.Command(executable)
	cmd.Dir = filepath.Dir(executable)
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

func replaceHelperFile(source, destination string) error {
	from, err := windows.UTF16PtrFromString(source)
	if err != nil {
		return err
	}
	to, err := windows.UTF16PtrFromString(destination)
	if err != nil {
		return err
	}
	return windows.MoveFileEx(from, to, windows.MOVEFILE_REPLACE_EXISTING|windows.MOVEFILE_WRITE_THROUGH)
}

func helperStatusPending(err error) bool {
	return errors.Is(err, os.ErrNotExist) ||
		errors.Is(err, windows.ERROR_SHARING_VIOLATION) ||
		errors.Is(err, windows.ERROR_LOCK_VIOLATION)
}
