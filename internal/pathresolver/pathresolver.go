// Package pathresolver expands OS-specific path templates from provider
// definitions into concrete absolute paths.
package pathresolver

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Resolve expands raw using the current runtime OS and user home directory.
func Resolve(raw string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return ResolveFor(raw, runtime.GOOS, home)
}

// ResolveFor expands raw for the given goos and home directory. It handles a
// leading "~", "%USERPROFILE%", and "$HOME", then normalizes separators for
// the target OS.
func ResolveFor(raw, goos, home string) (string, error) {
	if home == "" {
		return "", fmt.Errorf("pathresolver: empty home directory")
	}

	p := raw
	switch {
	case strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`):
		p = home + string(os.PathSeparator) + p[2:]
	case p == "~":
		p = home
	}

	p = strings.ReplaceAll(p, "%USERPROFILE%", home)
	p = strings.ReplaceAll(p, "$HOME", home)

	// Normalize slashes to the target OS separator.
	if goos == "windows" {
		p = strings.ReplaceAll(p, "/", `\`)
	} else {
		p = strings.ReplaceAll(p, `\`, "/")
	}

	return filepath.Clean(p), nil
}