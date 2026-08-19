package main

import "github.com/wailsapp/wails/v3/pkg/application"

const (
	preferredWindowWidth  = 1200
	preferredWindowHeight = 850
	fallbackWindowWidth   = 1080
	fallbackWindowHeight  = 720
	normalMinWindowWidth  = 640
	normalMinWindowHeight = 480
	windowSafeMargin      = 48
)

type mainWindowSize struct {
	Width     int
	Height    int
	MinWidth  int
	MinHeight int
}

func calculateMainWindowSize(workWidth, workHeight int) mainWindowSize {
	if workWidth <= 0 || workHeight <= 0 {
		return mainWindowSize{
			Width:     fallbackWindowWidth,
			Height:    fallbackWindowHeight,
			MinWidth:  normalMinWindowWidth,
			MinHeight: normalMinWindowHeight,
		}
	}

	width := availableWindowDimension(workWidth, preferredWindowWidth)
	height := availableWindowDimension(workHeight, preferredWindowHeight)
	return mainWindowSize{
		Width:     width,
		Height:    height,
		MinWidth:  min(normalMinWindowWidth, width),
		MinHeight: min(normalMinWindowHeight, height),
	}
}

func availableWindowDimension(workDimension, preferred int) int {
	available := workDimension - 2*windowSafeMargin
	if available <= 0 {
		available = workDimension
	}
	return min(preferred, available)
}

func initialMainWindowOptions() application.WebviewWindowOptions {
	size := calculateMainWindowSize(0, 0)
	return application.WebviewWindowOptions{
		Name:      "main",
		Title:     "SyncHub for Agents",
		URL:       "/",
		Width:     size.Width,
		Height:    size.Height,
		MinWidth:  size.MinWidth,
		MinHeight: size.MinHeight,
		Hidden:    true,
	}
}

type windowLayoutTarget interface {
	SetScreen(*application.Screen) application.Window
	SetMinSize(int, int) application.Window
	SetSize(int, int) application.Window
	Center()
	Show() application.Window
	Restore()
	Focus()
}

func applyMainWindowLayout(target windowLayoutTarget, screen *application.Screen, hidden bool) {
	workWidth, workHeight := 0, 0
	if screen != nil {
		workWidth = screen.WorkArea.Width
		workHeight = screen.WorkArea.Height
		target.SetScreen(screen)
	}
	size := calculateMainWindowSize(workWidth, workHeight)
	target.SetMinSize(size.MinWidth, size.MinHeight)
	target.SetSize(size.Width, size.Height)
	target.Center()

	if !hidden {
		target.Show()
		target.Restore()
		target.Focus()
	}
}
