package tray

import (
	"bytes"
	"image/color"
	"image/png"
	"testing"

	"github.com/qinqingxu/acsync/internal/scheduler"
)

func TestRenderPNGIsSolidColor(t *testing.T) {
	data := renderPNG(color.RGBA{10, 20, 30, 255}, 8)
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if b := img.Bounds(); b.Dx() != 8 || b.Dy() != 8 {
		t.Fatalf("size = %dx%d, want 8x8", b.Dx(), b.Dy())
	}
	r, g, b, a := img.At(4, 4).RGBA()
	if uint8(r>>8) != 10 || uint8(g>>8) != 20 || uint8(b>>8) != 30 || uint8(a>>8) != 255 {
		t.Errorf("center pixel = %d,%d,%d,%d", uint8(r>>8), uint8(g>>8), uint8(b>>8), uint8(a>>8))
	}
}

func TestPngToICOHeader(t *testing.T) {
	ico := pngToICO(renderPNG(colorFor(scheduler.StateIdle), 32), 32)
	// ICONDIR: reserved=0, type=1 (icon), count=1
	if ico[0] != 0 || ico[1] != 0 || ico[2] != 1 || ico[3] != 0 || ico[4] != 1 || ico[5] != 0 {
		t.Fatalf("bad ICO header: % x", ico[:6])
	}
}

func TestIconDiffersByState(t *testing.T) {
	idle := Icon(scheduler.StateIdle, "linux")
	fail := Icon(scheduler.StateError, "linux")
	if bytes.Equal(idle, fail) {
		t.Error("idle and error icons should differ")
	}
}

func TestIconWindowsIsICO(t *testing.T) {
	ico := Icon(scheduler.StateIdle, "windows")
	if len(ico) < 6 || ico[2] != 1 {
		t.Error("windows icon should be ICO format")
	}
	png := Icon(scheduler.StateIdle, "linux")
	if len(png) < 8 || png[0] != 0x89 || png[1] != 'P' {
		t.Error("non-windows icon should be PNG format")
	}
}
