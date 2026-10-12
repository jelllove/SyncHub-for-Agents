//go:build windows && !server

package desktop

import (
	"errors"
	"fmt"

	"github.com/wailsapp/wails/v3/pkg/application"
	"golang.org/x/sys/windows"
)

var showWindow = windows.NewLazySystemDLL("user32.dll").NewProc("ShowWindow")

func popupSupported() bool { return true }

func showTipWindow(window *application.WebviewWindow) error {
	handle := uintptr(window.NativeWindow())
	if handle == 0 {
		return errors.New("activity tip native window is unavailable")
	}
	if err := showWindow.Find(); err != nil {
		return fmt.Errorf("load non-activating window API: %w", err)
	}
	application.InvokeSync(func() {
		// Wails Show uses SW_SHOW, which activates even a WS_EX_NOACTIVATE window.
		// ShowWindow returns previous visibility, not a success/error indicator.
		showWindow.Call(handle, 4) // SW_SHOWNOACTIVATE
	})
	if !window.IsVisible() {
		return errors.New("activity tip did not become visible")
	}
	return nil
}
