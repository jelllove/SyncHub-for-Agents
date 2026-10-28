//go:build !windows || server

package desktop

import "github.com/wailsapp/wails/v3/pkg/application"

func popupSupported() bool { return false }

func showTipWindow(window *application.WebviewWindow) error {
	window.Show()
	return nil
}
