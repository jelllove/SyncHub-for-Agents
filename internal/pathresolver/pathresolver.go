// Package pathresolver expands OS-specific path templates from provider
// definitions into concrete absolute paths.
package pathresolver

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"
)

const HomeToken = "${HOME}"

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

func TokenizeHome(value, goos, home string) (string, bool) {
	normalizedValue, err := normalizePortablePath(value, goos)
	if err != nil {
		return value, false
	}
	normalizedHome, err := normalizePortablePath(home, goos)
	if err != nil {
		return value, false
	}
	valueForCompare := normalizedValue
	homeForCompare := normalizedHome
	if goos == "windows" {
		valueForCompare = strings.ToLower(valueForCompare)
		homeForCompare = strings.ToLower(homeForCompare)
	}
	if valueForCompare == homeForCompare {
		return HomeToken, true
	}
	prefix := homeForCompare + "/"
	if !strings.HasPrefix(valueForCompare, prefix) {
		return value, false
	}
	suffix := normalizedValue[len(normalizedHome):]
	return HomeToken + suffix, true
}

func ExpandHomeToken(value, goos, home string) (string, error) {
	if value != HomeToken &&
		!strings.HasPrefix(value, HomeToken+"/") &&
		!strings.HasPrefix(value, HomeToken+`\`) {
		return value, nil
	}
	normalizedHome, err := normalizePortablePath(home, goos)
	if err != nil {
		return "", fmt.Errorf("pathresolver: invalid home: %w", err)
	}
	suffix := strings.TrimPrefix(value, HomeToken)
	suffix = strings.ReplaceAll(suffix, `\`, "/")
	expanded, err := normalizePortablePath(normalizedHome+suffix, goos)
	if err != nil {
		return "", fmt.Errorf("pathresolver: invalid home token path %q: %w", value, err)
	}
	if goos == "windows" {
		return strings.ReplaceAll(expanded, "/", `\`), nil
	}
	return expanded, nil
}

func normalizePortablePath(value, goos string) (string, error) {
	if strings.TrimSpace(value) == "" {
		return "", fmt.Errorf("path is empty")
	}
	normalized := strings.ReplaceAll(value, `\`, "/")
	if strings.Contains(normalized, "//") {
		return "", fmt.Errorf("path %q contains an empty segment", value)
	}
	if !isTargetAbsolute(normalized, goos) {
		return "", fmt.Errorf("path %q is not absolute", value)
	}
	cleaned := path.Clean(normalized)
	if cleaned != normalized {
		return "", fmt.Errorf("path %q contains traversal or malformed segments", value)
	}
	return cleaned, nil
}

func isTargetAbsolute(value, goos string) bool {
	if goos != "windows" {
		return strings.HasPrefix(value, "/")
	}
	if len(value) < 3 || value[1] != ':' || value[2] != '/' {
		return false
	}
	drive := value[0]
	return (drive >= 'A' && drive <= 'Z') || (drive >= 'a' && drive <= 'z')
}
