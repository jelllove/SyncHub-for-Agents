// Package syncengine orchestrates a full sync pass: collect, reconcile, apply.
package syncengine

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/qinqingxu/acsync/internal/secret"
	"github.com/qinqingxu/acsync/internal/state"
)

// AgentSpec is a provider resolved to a concrete local directory for the
// current machine.
type AgentSpec struct {
	Name     string
	Root     string
	Include  []string // globs relative to Root -> agents/<name>/config/
	Sessions []string // globs relative to Root -> agents/<name>/sessions/
	Scanner  *secret.Scanner
}

// Collected is the result of walking all agent directories.
type Collected struct {
	Snapshot state.Snapshot    // repo-relative path -> metadata
	Sources  map[string]string // repo-relative path -> local absolute path
	Blocked  []string          // repo-relative paths blocked by the scanner
}

// Collect walks each spec's directory and builds the desired repo snapshot.
func Collect(specs []AgentSpec) (Collected, error) {
	out := Collected{
		Snapshot: state.Snapshot{},
		Sources:  map[string]string{},
	}
	for _, spec := range specs {
		if err := collectOne(spec, &out); err != nil {
			return Collected{}, err
		}
	}
	return out, nil
}

// ScanRemoteBlocked scans pulled repo files with the owning provider's scanner.
// Malformed JSONL fails closed and is removed from the remote repository.
func ScanRemoteBlocked(repoDir string, specs map[string]AgentSpec, remote state.Snapshot) ([]string, error) {
	var blocked []string
	for repoRel := range remote {
		parts := strings.Split(repoRel, "/")
		if len(parts) < 4 || parts[0] != "agents" {
			blocked = append(blocked, repoRel)
			continue
		}
		spec, ok := specs[parts[1]]
		if !ok {
			continue
		}
		rel := strings.Join(parts[3:], "/")
		sub, allowed := classify(rel, spec.Include, spec.Sessions)
		if !allowed || sub != parts[2] {
			blocked = append(blocked, repoRel)
			continue
		}
		if spec.Scanner == nil {
			continue
		}
		data, err := os.ReadFile(filepath.Join(repoDir, filepath.FromSlash(repoRel)))
		if err != nil {
			return nil, err
		}
		isBlocked, scanErr := spec.Scanner.Scan(rel, data)
		if isBlocked || scanErr != nil {
			blocked = append(blocked, repoRel)
		}
	}
	return blocked, nil
}

// FilterSnapshotForSpecs removes paths owned by disabled or unknown providers
// so they remain untouched in the remote repository.
func FilterSnapshotForSpecs(snapshot state.Snapshot, specs map[string]AgentSpec) state.Snapshot {
	filtered := state.Snapshot{}
	for repoRel, metadata := range snapshot {
		parts := strings.Split(repoRel, "/")
		if len(parts) < 4 || parts[0] != "agents" {
			continue
		}
		if _, enabled := specs[parts[1]]; enabled {
			filtered[repoRel] = metadata
		}
	}
	return filtered
}

func mergeBlocked(groups ...[]string) []string {
	set := map[string]struct{}{}
	for _, group := range groups {
		for _, repoRel := range group {
			set[repoRel] = struct{}{}
		}
	}
	out := make([]string, 0, len(set))
	for repoRel := range set {
		out = append(out, repoRel)
	}
	return out
}

func collectOne(spec AgentSpec, out *Collected) error {
	info, err := os.Stat(spec.Root)
	if err != nil || !info.IsDir() {
		return nil // missing agent dir: nothing to collect
	}

	return filepath.WalkDir(spec.Root, func(abs string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		relOS, err := filepath.Rel(spec.Root, abs)
		if err != nil {
			return err
		}
		rel := filepath.ToSlash(relOS)

		sub, ok := classify(rel, spec.Include, spec.Sessions)
		if !ok {
			return nil
		}
		repoRel := path.Join("agents", spec.Name, sub, rel)

		fi, err := d.Info()
		if err != nil {
			return err
		}
		data, err := os.ReadFile(abs)
		if err != nil {
			return err
		}
		if spec.Scanner != nil {
			blocked, err := spec.Scanner.Scan(rel, data)
			if err != nil {
				return fmt.Errorf("scan %s: %w", repoRel, err)
			}
			if blocked {
				out.Blocked = append(out.Blocked, repoRel)
				return nil
			}
		}

		sum := sha256.Sum256(data)
		out.Snapshot[repoRel] = state.FileMeta{
			Hash:    hex.EncodeToString(sum[:]),
			ModTime: fi.ModTime().Unix(),
			Size:    int64(len(data)),
		}
		out.Sources[repoRel] = abs
		return nil
	})
}

// classify returns the repo subdirectory ("config" or "sessions") for a file,
// or ok=false if it matches no glob. Session globs take precedence.
func classify(rel string, include, sessions []string) (string, bool) {
	for _, g := range sessions {
		if matchGlob(g, rel) {
			return "sessions", true
		}
	}
	for _, g := range include {
		if matchGlob(g, rel) {
			return "config", true
		}
	}
	return "", false
}

func matchGlob(glob, rel string) bool {
	glob = strings.ReplaceAll(glob, "\\", "/")
	ok, _ := doublestar.Match(glob, rel)
	return ok
}
