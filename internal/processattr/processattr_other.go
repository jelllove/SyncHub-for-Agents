//go:build !windows

package processattr

import "os/exec"

// HideWindow is unnecessary on platforms without Windows console allocation.
func HideWindow(*exec.Cmd) {}
