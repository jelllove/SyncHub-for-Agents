//go:build windows

package processattr

import (
	"os/exec"
	"testing"

	"golang.org/x/sys/windows"
)

func TestHideWindowSetsCreateNoWindow(t *testing.T) {
	command := exec.Command("git", "--version")

	HideWindow(command)

	if command.SysProcAttr == nil {
		t.Fatal("SysProcAttr is nil")
	}
	if command.SysProcAttr.CreationFlags&windows.CREATE_NO_WINDOW == 0 {
		t.Fatalf("CreationFlags = %#x, want CREATE_NO_WINDOW", command.SysProcAttr.CreationFlags)
	}
}
