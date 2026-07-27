// Package tray renders status icons and runs the system-tray application.
package tray

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"

	"github.com/qinqingxu/acsync/internal/scheduler"
)

func colorFor(s scheduler.State) color.RGBA {
	switch s {
	case scheduler.StateUpdating:
		return color.RGBA{0x1e, 0x90, 0xff, 0xff} // blue
	case scheduler.StateError:
		return color.RGBA{0xd3, 0x2f, 0x2f, 0xff} // red
	case scheduler.StatePaused:
		return color.RGBA{0x9e, 0x9e, 0x9e, 0xff} // gray
	default:
		return color.RGBA{0x2e, 0x7d, 0x32, 0xff} // green (idle)
	}
}

func renderPNG(c color.RGBA, size int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			img.SetRGBA(x, y, c)
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

// pngToICO wraps a square PNG (side=size) in a single-image ICO container.
// Windows Vista+ accepts PNG-compressed icon images.
func pngToICO(pngData []byte, size int) []byte {
	var buf bytes.Buffer
	// ICONDIR
	binary.Write(&buf, binary.LittleEndian, uint16(0)) // reserved
	binary.Write(&buf, binary.LittleEndian, uint16(1)) // type: 1 = icon
	binary.Write(&buf, binary.LittleEndian, uint16(1)) // image count
	// ICONDIRENTRY
	dim := byte(size)
	if size >= 256 {
		dim = 0 // 0 means 256
	}
	buf.WriteByte(dim)                                           // width
	buf.WriteByte(dim)                                           // height
	buf.WriteByte(0)                                             // palette size
	buf.WriteByte(0)                                             // reserved
	binary.Write(&buf, binary.LittleEndian, uint16(1))          // color planes
	binary.Write(&buf, binary.LittleEndian, uint16(32))         // bits per pixel
	binary.Write(&buf, binary.LittleEndian, uint32(len(pngData))) // image size
	binary.Write(&buf, binary.LittleEndian, uint32(22))         // offset (6 + 16)
	buf.Write(pngData)
	return buf.Bytes()
}

// Icon returns the tray icon bytes for a state, in the format the OS expects
// (ICO on Windows, PNG elsewhere).
func Icon(s scheduler.State, goos string) []byte {
	const size = 32
	pngData := renderPNG(colorFor(s), size)
	if goos == "windows" {
		return pngToICO(pngData, size)
	}
	return pngData
}
