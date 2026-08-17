package tray

import (
	"bytes"
	"encoding/binary"
	"image/png"
	"testing"

	"github.com/qinqingxu/acsync/internal/scheduler"
)

func TestAssetPNGSupportsEveryStateAndSize(t *testing.T) {
	states := []scheduler.State{
		scheduler.StateIdle,
		scheduler.StateUpdating,
		scheduler.StateDone,
		scheduler.StateError,
		scheduler.StatePaused,
	}
	for _, state := range states {
		for _, size := range iconSizes {
			data := assetPNG(state, size)
			img, err := png.Decode(bytes.NewReader(data))
			if err != nil {
				t.Fatalf("decode %s %dpx: %v", state, size, err)
			}
			if bounds := img.Bounds(); bounds.Dx() != size || bounds.Dy() != size {
				t.Fatalf("%s size = %dx%d, want %dx%d", state, bounds.Dx(), bounds.Dy(), size, size)
			}
		}
	}
}

func TestReadyAssetUsesTransparency(t *testing.T) {
	img, err := png.Decode(bytes.NewReader(assetPNG(scheduler.StateIdle, 32)))
	if err != nil {
		t.Fatal(err)
	}
	_, _, _, alpha := img.At(0, 0).RGBA()
	if alpha != 0 {
		t.Fatalf("corner alpha = %d, want transparent", alpha)
	}
}

func TestWindowsIconContainsEveryResolution(t *testing.T) {
	ico := Icon(scheduler.StateDone, "windows")
	if len(ico) < 6 {
		t.Fatal("ICO is too short")
	}
	if got := binary.LittleEndian.Uint16(ico[2:4]); got != 1 {
		t.Fatalf("ICO type = %d, want 1", got)
	}
	if got := int(binary.LittleEndian.Uint16(ico[4:6])); got != len(iconSizes) {
		t.Fatalf("ICO image count = %d, want %d", got, len(iconSizes))
	}
	for index, want := range iconSizes {
		offset := 6 + index*16
		if got := int(ico[offset]); got != want {
			t.Fatalf("entry %d width = %d, want %d", index, got, want)
		}
		if got := int(ico[offset+1]); got != want {
			t.Fatalf("entry %d height = %d, want %d", index, got, want)
		}
	}
}

func TestIconDiffersByState(t *testing.T) {
	icons := map[string]struct{}{}
	for _, state := range []scheduler.State{
		scheduler.StateIdle,
		scheduler.StateUpdating,
		scheduler.StateDone,
		scheduler.StateError,
		scheduler.StatePaused,
	} {
		icons[string(Icon(state, "linux"))] = struct{}{}
	}
	if len(icons) != 5 {
		t.Fatalf("distinct icons = %d, want 5", len(icons))
	}
}

func TestNonWindowsIconIsLargestPNG(t *testing.T) {
	got := Icon(scheduler.StateIdle, "linux")
	want := assetPNG(scheduler.StateIdle, 32)
	if !bytes.Equal(got, want) {
		t.Fatal("non-Windows icon should use the 32px embedded PNG")
	}
}
