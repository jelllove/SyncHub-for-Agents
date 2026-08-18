package main

import (
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func TestCalculateMainWindowSize(t *testing.T) {
	tests := []struct {
		name                  string
		workWidth, workHeight int
		want                  mainWindowSize
	}{
		{
			name:       "large display uses preferred size",
			workWidth:  2560,
			workHeight: 1400,
			want:       mainWindowSize{Width: 1200, Height: 850, MinWidth: 640, MinHeight: 480},
		},
		{
			name:       "full HD work area uses preferred size",
			workWidth:  1920,
			workHeight: 1040,
			want:       mainWindowSize{Width: 1200, Height: 850, MinWidth: 640, MinHeight: 480},
		},
		{
			name:       "laptop work area reduces height",
			workWidth:  1366,
			workHeight: 728,
			want:       mainWindowSize{Width: 1200, Height: 632, MinWidth: 640, MinHeight: 480},
		},
		{
			name:       "compact work area lowers initial and minimum dimensions",
			workWidth:  600,
			workHeight: 450,
			want:       mainWindowSize{Width: 504, Height: 354, MinWidth: 504, MinHeight: 354},
		},
		{
			name:       "work area smaller than margins still stays visible",
			workWidth:  80,
			workHeight: 70,
			want:       mainWindowSize{Width: 80, Height: 70, MinWidth: 80, MinHeight: 70},
		},
		{
			name:       "invalid work area uses fallback",
			workWidth:  0,
			workHeight: 1080,
			want:       mainWindowSize{Width: 1080, Height: 720, MinWidth: 640, MinHeight: 480},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := calculateMainWindowSize(test.workWidth, test.workHeight)
			if got != test.want {
				t.Fatalf("calculateMainWindowSize(%d, %d) = %+v, want %+v",
					test.workWidth, test.workHeight, got, test.want)
			}
		})
	}
}

func TestMainWindowOptionsTargetsAndCentersPrimaryScreen(t *testing.T) {
	screen := &application.Screen{
		WorkArea: application.Rect{Width: 1366, Height: 728},
	}

	got := mainWindowOptions(true, screen)

	if got.Width != 1200 || got.Height != 632 {
		t.Fatalf("window size = %dx%d, want 1200x632", got.Width, got.Height)
	}
	if got.MinWidth != 640 || got.MinHeight != 480 {
		t.Fatalf("minimum size = %dx%d, want 640x480", got.MinWidth, got.MinHeight)
	}
	if got.Screen != screen {
		t.Fatal("window did not retain the selected primary screen")
	}
	if got.InitialPosition != application.WindowCentered {
		t.Fatalf("initial position = %v, want WindowCentered", got.InitialPosition)
	}
	if !got.Hidden {
		t.Fatal("hidden startup flag was not preserved")
	}
}

func TestMainWindowOptionsFallsBackWithoutAValidScreen(t *testing.T) {
	got := mainWindowOptions(false, nil)

	if got.Width != 1080 || got.Height != 720 {
		t.Fatalf("fallback window size = %dx%d, want 1080x720", got.Width, got.Height)
	}
	if got.MinWidth != 640 || got.MinHeight != 480 {
		t.Fatalf("fallback minimum = %dx%d, want 640x480", got.MinWidth, got.MinHeight)
	}
	if got.Screen != nil {
		t.Fatal("fallback options unexpectedly target a screen")
	}
	if got.Hidden {
		t.Fatal("visible startup unexpectedly became hidden")
	}
}
