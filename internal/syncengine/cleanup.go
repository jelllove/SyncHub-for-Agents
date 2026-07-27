package syncengine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"time"
)

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
			filePath := filepath.Join(repoDir, ".trash", "files", filepath.FromSlash(repoRel))
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
