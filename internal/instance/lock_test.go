package instance

import (
	"bufio"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestOwnershipIsExclusiveAndReusable(t *testing.T) {
	home := t.TempDir()
	first, err := Acquire(home)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	if other, err := Acquire(home); !errors.Is(err, ErrBusy) {
		if other != nil {
			other.Close()
		}
		t.Fatalf("second owner error = %v, want busy", err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	next, err := Acquire(home)
	if err != nil {
		t.Fatal(err)
	}
	if err := next.Close(); err != nil {
		t.Fatal(err)
	}
	if err := next.Close(); err != nil {
		t.Fatalf("repeated close: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".instance.lock")); err != nil {
		t.Fatalf("lock file must not be unlinked: %v", err)
	}
}

func TestChildOwnership(t *testing.T) {
	home := os.Getenv("SYNCHUB_LOCK_TEST_HOME")
	if home == "" {
		return
	}
	lock, err := Acquire(home)
	if errors.Is(err, ErrBusy) {
		os.Exit(23)
	}
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	if _, err := os.Stdout.WriteString("owned\n"); err != nil {
		t.Fatal(err)
	}
	var input [1]byte
	_, _ = os.Stdin.Read(input[:])
}

func TestOwnershipSurvivesCrossProcessContentionAndReleasesOnDeath(t *testing.T) {
	home := t.TempDir()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	start := func() *exec.Cmd {
		cmd := exec.Command(executable, "-test.run=^TestChildOwnership$")
		cmd.Env = append(os.Environ(), "SYNCHUB_LOCK_TEST_HOME="+home)
		return cmd
	}
	owner := start()
	stdin, err := owner.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer stdin.Close()
	stdout, err := owner.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if owner.ProcessState == nil {
			owner.Process.Kill()
			owner.Wait()
		}
	}()
	line, err := bufio.NewReader(stdout).ReadString('\n')
	if err != nil || line != "owned\n" {
		t.Fatalf("child not ready: %q, %v", line, err)
	}
	contender := start()
	err = contender.Run()
	var exit *exec.ExitError
	if !errors.As(err, &exit) || exit.ExitCode() != 23 {
		t.Fatalf("contender = %v, want exit 23", err)
	}
	if err := owner.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = owner.Wait()
	recovered, err := Acquire(home)
	if err != nil {
		t.Fatalf("crashed owner retained lock: %v", err)
	}
	defer recovered.Close()
}

func TestRejectsUnsafeLockFiles(t *testing.T) {
	home := t.TempDir()
	if err := os.Mkdir(filepath.Join(home, ".instance.lock"), 0o700); err != nil {
		t.Fatal(err)
	}
	if lock, err := Acquire(home); err == nil {
		lock.Close()
		t.Fatal("directory accepted as lock file")
	}
	home = t.TempDir()
	target := filepath.Join(t.TempDir(), "outside")
	if err := os.WriteFile(target, []byte("unchanged"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(home, ".instance.lock")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if lock, err := Acquire(home); err == nil {
		lock.Close()
		t.Fatal("symlink accepted as lock file")
	}
	data, err := os.ReadFile(target)
	if err != nil || string(data) != "unchanged" {
		t.Fatalf("outside file changed: %q, %v", data, err)
	}
}
