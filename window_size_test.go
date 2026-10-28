package main

import (
	"fmt"
	"reflect"
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

func TestInitialMainWindowOptionsStayHiddenUntilDisplayReady(t *testing.T) {
	got := initialMainWindowOptions()

	if got.Width != 1080 || got.Height != 720 {
		t.Fatalf("initial window size = %dx%d, want fallback 1080x720", got.Width, got.Height)
	}
	if got.MinWidth != 640 || got.MinHeight != 480 {
		t.Fatalf("initial minimum = %dx%d, want 640x480", got.MinWidth, got.MinHeight)
	}
	if got.Screen != nil {
		t.Fatal("initial options unexpectedly target a screen before Wails starts")
	}
	if !got.Hidden {
		t.Fatal("window must remain hidden until display-aware sizing is applied")
	}
}

func TestApplyMainWindowLayoutTargetsScreenBeforeShowing(t *testing.T) {
	screen := &application.Screen{
		WorkArea: application.Rect{Width: 1366, Height: 728},
	}
	target := &recordingWindow{}

	applyMainWindowLayout(target, screen, false)

	want := []string{
		"screen",
		"min:640x480",
		"size:1200x632",
		"center",
		"show",
		"restore",
		"focus",
	}
	if !reflect.DeepEqual(target.calls, want) {
		t.Fatalf("layout calls = %v, want %v", target.calls, want)
	}
}

func TestApplyMainWindowLayoutKeepsAutostartWindowHidden(t *testing.T) {
	screen := &application.Screen{
		WorkArea: application.Rect{Width: 1920, Height: 1040},
	}
	target := &recordingWindow{}

	applyMainWindowLayout(target, screen, true)

	want := []string{"screen", "min:640x480", "size:1200x850", "center"}
	if !reflect.DeepEqual(target.calls, want) {
		t.Fatalf("hidden layout calls = %v, want %v", target.calls, want)
	}
}

type recordingWindow struct {
	calls []string
}

func (w *recordingWindow) SetScreen(*application.Screen) application.Window {
	w.calls = append(w.calls, "screen")
	return nil
}

func (w *recordingWindow) SetMinSize(width, height int) application.Window {
	w.calls = append(w.calls, fmt.Sprintf("min:%dx%d", width, height))
	return nil
}

func (w *recordingWindow) SetSize(width, height int) application.Window {
	w.calls = append(w.calls, fmt.Sprintf("size:%dx%d", width, height))
	return nil
}

func (w *recordingWindow) Center() {
	w.calls = append(w.calls, "center")
}

func (w *recordingWindow) Show() application.Window {
	w.calls = append(w.calls, "show")
	return nil
}

func (w *recordingWindow) Restore() {
	w.calls = append(w.calls, "restore")
}

func (w *recordingWindow) Focus() {
	w.calls = append(w.calls, "focus")
}
