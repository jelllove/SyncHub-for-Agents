//go:build windows

package installplan

import (
	"context"
	"testing"

	"golang.org/x/sys/windows"
)

func TestCommandRunnerCreatesHiddenWindowsProcess(t *testing.T) {
	command := newRunnerCommand(context.Background(), "copilot", []string{"plugin", "list"})

	if command.SysProcAttr == nil {
		t.Fatal("SysProcAttr is nil")
	}
	if command.SysProcAttr.CreationFlags&windows.CREATE_NO_WINDOW == 0 {
		t.Fatalf("CreationFlags = %#x, want CREATE_NO_WINDOW", command.SysProcAttr.CreationFlags)
	}
}
