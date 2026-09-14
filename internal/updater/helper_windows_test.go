//go:build windows

package updater

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestHelperReadinessRetriesTransientFileLocks(t *testing.T) {
	directory := helperTestDirectory(t)
	path := filepath.Join(directory, "ready.json")
	if err := writeHelperStatus(path, nil); err != nil {
		t.Fatal(err)
	}
	nativePath, err := windows.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	handle, err := windows.CreateFile(nativePath, windows.GENERIC_READ, 0,
		nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatal(err)
	}
	closed := make(chan error, 1)
	go func() {
		time.Sleep(3 * helperPollInterval)
		closed <- windows.CloseHandle(handle)
	}()
	err = waitHelperReady(directory, make(chan error), time.Second)
	if closeErr := <-closed; closeErr != nil {
		t.Fatal(closeErr)
	}
	if err != nil {
		t.Fatalf("transient readiness lock aborted update: %v", err)
	}
}

func TestHelperNSISCommandLine(t *testing.T) {
	installer := `C:\Private Cache\installer.exe`
	directory := `C:\Users\Test User\Programs\SyncHub 应用`
	line, err := helperInstallerCommandLine(installer, directory)
	if err != nil {
		t.Fatal(err)
	}
	want := `"C:\Private Cache\installer.exe" /S /D=C:\Users\Test User\Programs\SyncHub 应用`
	if line != want {
		t.Fatalf("command line = %q, want %q", line, want)
	}
	if strings.Contains(line, `/D="`) || !strings.HasSuffix(line, directory) {
		t.Fatal("NSIS /D must be the final, unquoted argument")
	}
	for _, bad := range []string{
		`relative\path`, `C:\quote"path`, "C:\\line\npath", "C:\\return\rpath",
		`\\server\share\path`, `\\?\C:\extended`, `C:\stream:ads`, `C:\trailing.`, `C:\trailing `,
	} {
		if _, err := helperInstallerCommandLine(installer, bad); err == nil {
			t.Errorf("accepted invalid install directory %q", bad)
		}
		if _, err := helperInstallerCommandLine(bad, directory); err == nil {
			t.Errorf("accepted invalid installer path %q", bad)
		}
	}
}

func TestHelperLaunchRejectsInvalidHashAndExecutable(t *testing.T) {
	o := helperTestOptions(t)
	pending := pendingUpdate{Path: o.installer, Checksum: strings.Repeat("0", 64), Version: "v1.0.0"}
	if err := LaunchInstaller(pending, o.executable, false, o.resultPath); err == nil || !strings.Contains(err.Error(), "SHA-256") {
		t.Fatalf("invalid hash should fail before launch: %v", err)
	}
	pending.Checksum = o.checksum
	if err := os.WriteFile(o.executable, []byte("not the running executable"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := LaunchInstaller(pending, o.executable, false, o.resultPath); err == nil || !strings.Contains(err.Error(), "running SyncHub.exe") {
		t.Fatalf("must only copy the running executable: %v", err)
	}
}

func TestHelperInstallerStartFailure(t *testing.T) {
	directory := helperTestDirectory(t)
	err := runHelperInstaller(filepath.Join(directory, "does-not-exist.exe"), directory, time.Second)
	if err == nil {
		t.Fatal("expected an installer start error")
	}
}

func TestHelperPrivateCachePermissions(t *testing.T) {
	directory := helperTestDirectory(t)
	if err := makeHelperCachePrivate(directory); err != nil {
		t.Fatal(err)
	}
	descriptor, err := windows.GetNamedSecurityInfo(directory, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	control, _, err := descriptor.Control()
	if err != nil {
		t.Fatal(err)
	}
	if control&windows.SE_DACL_PROTECTED == 0 {
		t.Fatal("cache still inherits potentially public directory permissions")
	}
	if err := helperDirectoryWritable(directory); err != nil {
		t.Fatalf("private cache is no longer writable by its owner: %v", err)
	}
}

func TestHelperWaitsForExecutableUnlockWithoutChangingIt(t *testing.T) {
	directory := helperTestDirectory(t)
	executable := filepath.Join(directory, "SyncHub.exe")
	content := []byte("non-executable isolated lock fixture")
	if err := os.WriteFile(executable, content, 0600); err != nil {
		t.Fatal(err)
	}
	path, err := windows.UTF16PtrFromString(executable)
	if err != nil {
		t.Fatal(err)
	}
	handle, err := windows.CreateFile(path, windows.GENERIC_READ, windows.FILE_SHARE_READ,
		nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatal(err)
	}
	if err := waitHelperExecutableUnlocked(executable, time.Millisecond); err == nil {
		windows.CloseHandle(handle)
		t.Fatal("write-locked executable must not be considered ready")
	}
	closed := make(chan error, 1)
	go func() {
		time.Sleep(2 * helperPollInterval)
		closed <- windows.CloseHandle(handle)
	}()
	unlockErr := waitHelperExecutableUnlocked(executable, time.Second)
	if err := <-closed; err != nil {
		t.Fatal(err)
	}
	if unlockErr != nil {
		t.Fatalf("did not retry a transient file lock: %v", unlockErr)
	}
	data, err := os.ReadFile(executable)
	if err != nil || string(data) != string(content) {
		t.Fatalf("unlock probe changed executable contents: %q, %v", data, err)
	}
}

func TestHelperParentHandleWaitsForOnlyItsChild(t *testing.T) {
	directory := helperTestDirectory(t)
	release := filepath.Join(directory, "release")
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(executable, "-test.run=^TestHelperWaitChild$")
	cmd.Env = append(os.Environ(), "SYNCHUB_TEST_WAIT_CHILD="+release)
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer cmd.Wait()
	parent, err := openHelperParent(uint32(cmd.Process.Pid))
	if err != nil {
		t.Fatal(err)
	}
	defer parent.close()
	if err := parent.wait(time.Millisecond); err == nil {
		t.Fatal("running child must time out without being terminated")
	}
	if err := os.WriteFile(release, []byte("exit"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := parent.wait(10 * time.Second); err != nil {
		t.Fatalf("actual process handle did not observe child exit: %v", err)
	}
}

func TestHelperWaitChild(t *testing.T) {
	release := os.Getenv("SYNCHUB_TEST_WAIT_CHILD")
	if release == "" {
		return
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(release); err == nil {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("test child was not released before timeout")
}
