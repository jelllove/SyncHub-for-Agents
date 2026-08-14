package syncengine

import (
	"encoding/json"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/qinqingxu/acsync/internal/resource"
	"github.com/qinqingxu/acsync/internal/secret"
)

// PurgeBlockedTrash permanently removes trash files that are no longer allowed
// by their provider or are blocked by its secret scanner.
func PurgeBlockedTrash(repoDir string, specs map[string]resource.Spec) ([]string, error) {
	idxPath := filepath.Join(repoDir, ".trash", "index.json")
	data, err := os.ReadFile(idxPath)
	if os.IsNotExist(err) {
		data = nil
	} else if err != nil {
		return nil, err
	}
	idx := map[string]int64{}
	if len(data) > 0 {
		if err := json.Unmarshal(data, &idx); err != nil {
			return nil, err
		}
	}

	var purged []string
	filesRoot := filepath.Join(repoDir, ".trash", "files")
	found := map[string]struct{}{}
	err = filepath.WalkDir(filesRoot, func(filename string, entry os.DirEntry, walkErr error) error {
		if os.IsNotExist(walkErr) {
			return nil
		}
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(filesRoot, filename)
		if err != nil {
			return err
		}
		found[filepath.ToSlash(rel)] = struct{}{}
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	for repoRel := range found {
		filePath, valid := safeTrashPath(repoDir, repoRel)
		if !valid {
			continue
		}
		_, indexed := idx[repoRel]
		blocked := !indexed
		if isInternalPortablePath(repoRel) {
			if !blocked {
				blocked = validateInternalRemote(filesRoot, repoRel, specs) != nil
			}
			if !blocked {
				continue
			}
			if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
				return nil, err
			}
			delete(idx, repoRel)
			purged = append(purged, repoRel)
			continue
		}
		ref, parseErr := resource.ParseRepoPath(repoRel)
		if parseErr != nil {
			if strings.HasPrefix(repoRel, portablePrefix) {
				blocked = true
			} else if !blocked {
				continue
			}
		}
		spec, ok := specForRef(specs, ref)
		if !ok {
			if !blocked {
				continue
			}
		} else {
			info, statErr := os.Stat(filePath)
			if statErr != nil {
				return nil, statErr
			}
			allowed := resourcePathAllowed(spec, ref.Relative)
			if allowed {
				allowed, _ = resource.DefaultFilterPolicy().Check(ref.Relative, info.Size(), spec.Strategy)
			}
			blocked = blocked || !refMatchesSpec(ref, spec) || !allowed
			if !blocked {
				content, readErr := os.ReadFile(filePath)
				if readErr != nil {
					return nil, readErr
				}
				scanner := secret.NewScanner(spec.Exclude, spec.KeyPatterns)
				isBlocked, scanErr := scanner.Scan(ref.Relative, content)
				blocked = isBlocked || scanErr != nil
			}

		}
		if !blocked {
			continue
		}
		if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
			return nil, err
		}
		delete(idx, repoRel)
		purged = append(purged, repoRel)
	}
	for repoRel := range idx {
		if _, exists := found[repoRel]; exists {
			continue
		}
		if _, valid := safeTrashPath(repoDir, repoRel); !valid {
			delete(idx, repoRel)
			purged = append(purged, repoRel)
		}
	}
	if len(purged) == 0 {
		return nil, nil
	}
	out, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(idxPath, out, 0o644); err != nil {
		return nil, err
	}
	sort.Strings(purged)
	return purged, nil
}

func resourcePathAllowed(spec resource.Spec, relative string) bool {
	if spec.Strategy == resource.StrategyInstallManifest {
		return true
	}
	for _, pattern := range spec.Include {
		matched, err := doublestar.Match(strings.ReplaceAll(pattern, `\`, "/"), relative)
		if err == nil && matched {
			return true
		}
	}
	return false
}

// CleanupTrash permanently removes soft-deleted files whose deletion time is
// older than graceDays. It updates .trash/index.json and deletes the backing
// files under .trash/files/. It returns the repo-relative paths that were
// purged (sorted). A missing trash index is treated as empty (no-op).
func CleanupTrash(repoDir string, now time.Time, graceDays int) ([]string, error) {
	idxPath := filepath.Join(repoDir, ".trash", "index.json")
	data, err := os.ReadFile(idxPath)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	idx := map[string]int64{}
	if len(data) > 0 {
		if err := json.Unmarshal(data, &idx); err != nil {
			return nil, err
		}
	}

	cutoff := now.Add(-time.Duration(graceDays) * 24 * time.Hour).Unix()
	var purged []string
	for repoRel, deletedAt := range idx {
		if deletedAt <= cutoff {
			filePath, valid := safeTrashPath(repoDir, repoRel)
			if !valid {
				purged = append(purged, repoRel)
				continue
			}
			if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
				return nil, err
			}
			purged = append(purged, repoRel)
		}
	}
	if len(purged) == 0 {
		return nil, nil
	}

	for _, repoRel := range purged {
		delete(idx, repoRel)
	}
	out, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(idxPath, out, 0o644); err != nil {
		return nil, err
	}

	sort.Strings(purged)
	return purged, nil
}

func safeTrashPath(repoDir, repoRel string) (string, bool) {
	if repoRel == "" ||
		strings.Contains(repoRel, `\`) ||
		path.Clean(repoRel) != repoRel ||
		filepath.IsAbs(filepath.FromSlash(repoRel)) ||
		filepath.VolumeName(filepath.FromSlash(repoRel)) != "" {
		return "", false
	}
	parts := strings.Split(repoRel, "/")
	if len(parts) < 4 || parts[0] != "agents" {
		return "", false
	}
	return filepath.Join(repoDir, ".trash", "files", filepath.FromSlash(repoRel)), true
}
