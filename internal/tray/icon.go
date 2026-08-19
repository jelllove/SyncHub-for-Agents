// Package tray renders status icons and runs the system-tray application.
package tray

import (
	"bytes"
	"embed"
	"encoding/binary"
	"fmt"

	"github.com/qinqingxu/synchub-for-agents/internal/scheduler"
)

var iconSizes = []int{16, 20, 24, 32}

//go:embed assets/*.png
var iconAssets embed.FS

func assetName(state scheduler.State) string {
	switch state {
	case scheduler.StateUpdating:
		return "updating"
	case scheduler.StateDone:
		return "done"
	case scheduler.StateError:
		return "error"
	case scheduler.StatePaused:
		return "paused"
	default:
		return "ready"
	}
}

func assetPNG(state scheduler.State, size int) []byte {
	data, err := iconAssets.ReadFile(fmt.Sprintf("assets/%s-%d.png", assetName(state), size))
	if err != nil {
		panic(fmt.Sprintf("tray icon asset is missing: %v", err))
	}
	return data
}

// pngsToICO wraps PNG-compressed images in a multi-resolution ICO container.
func pngsToICO(images [][]byte, sizes []int) []byte {
	const (
		headerSize = 6
		entrySize  = 16
	)
	dataOffset := headerSize + entrySize*len(images)
	totalSize := dataOffset
	for _, image := range images {
		totalSize += len(image)
	}

	ico := make([]byte, totalSize)
	binary.LittleEndian.PutUint16(ico[2:4], 1)
	binary.LittleEndian.PutUint16(ico[4:6], uint16(len(images)))

	imageOffset := dataOffset
	for index, image := range images {
		entry := headerSize + index*entrySize
		ico[entry] = byte(sizes[index])
		ico[entry+1] = byte(sizes[index])
		binary.LittleEndian.PutUint16(ico[entry+4:entry+6], 1)
		binary.LittleEndian.PutUint16(ico[entry+6:entry+8], 32)
		binary.LittleEndian.PutUint32(ico[entry+8:entry+12], uint32(len(image)))
		binary.LittleEndian.PutUint32(ico[entry+12:entry+16], uint32(imageOffset))
		copy(ico[imageOffset:], image)
		imageOffset += len(image)
	}
	return ico
}

// Icon returns tray icon bytes in the format the OS expects.
func Icon(state scheduler.State, goos string) []byte {
	if goos != "windows" {
		return bytes.Clone(assetPNG(state, 32))
	}
	images := make([][]byte, 0, len(iconSizes))
	for _, size := range iconSizes {
		images = append(images, assetPNG(state, size))
	}
	return pngsToICO(images, iconSizes)
}
