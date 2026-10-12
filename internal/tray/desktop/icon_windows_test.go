//go:build windows && !server

package desktop

import (
	"testing"

	"github.com/qinqingxu/synchub-for-agents/internal/scheduler"
	"github.com/wailsapp/wails/v3/pkg/w32"
)

func TestWindowsDesktopStatusAssetsCreateNativeTrayIcons(t *testing.T) {
	for _, state := range []scheduler.State{
		scheduler.StateIdle, scheduler.StateUpdating, scheduler.StateDone,
		scheduler.StateError, scheduler.StatePaused,
	} {
		icon, err := w32.CreateSmallHIconFromImage(desktopIcon(state))
		if err != nil || icon == 0 {
			t.Fatalf("desktop icon %s cannot be used by Wails: %v", state, err)
		}
		w32.DestroyIcon(icon)
	}
}
