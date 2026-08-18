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

func mainWindowOptions(hidden bool, screen *application.Screen) application.WebviewWindowOptions {
	workWidth, workHeight := 0, 0
	if screen != nil {
		workWidth = screen.WorkArea.Width
		workHeight = screen.WorkArea.Height
	}
	size := calculateMainWindowSize(workWidth, workHeight)

	return application.WebviewWindowOptions{
		Name:            "main",
		Title:           "AgentConfigSync",
		URL:             "/",
		Width:           size.Width,
		Height:          size.Height,
		MinWidth:        size.MinWidth,
		MinHeight:       size.MinHeight,
		InitialPosition: application.WindowCentered,
		Screen:          screen,
		Hidden:          hidden,
	}
}
