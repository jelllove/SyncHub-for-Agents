//go:build !windows || server

package desktop

func popupSupported() bool { return false }
