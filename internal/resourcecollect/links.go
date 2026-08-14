package resourcecollect

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
)

type RootInfo struct {
	CanonicalRoot string
	OriginalRoot  string
	Identity      string
}

type ApprovedLinkStore interface {
	IsApproved(target string) bool
}

func ResolveRoot(root string) (RootInfo, error) {
	if strings.TrimSpace(root) == "" {
		return RootInfo{}, fmt.Errorf("resolve root: path is empty")
	}
	original, err := filepath.Abs(root)
	if err != nil {
		return RootInfo{}, fmt.Errorf("resolve root %q: %w", root, err)
	}
	canonical, err := filepath.EvalSymlinks(original)
	if err != nil {
		return RootInfo{}, fmt.Errorf("resolve root %q: %w", root, err)
	}
	canonical, err = filepath.Abs(canonical)
	if err != nil {
		return RootInfo{}, fmt.Errorf("resolve root %q: %w", root, err)
	}
	canonical = filepath.Clean(canonical)
	identityPath := canonical
	if runtime.GOOS == "windows" {
		identityPath = strings.ToLower(identityPath)
	}
	sum := sha256.Sum256([]byte(identityPath))
	return RootInfo{
		CanonicalRoot: canonical,
		OriginalRoot:  filepath.Clean(original),
		Identity:      hex.EncodeToString(sum[:]),
	}, nil
}
