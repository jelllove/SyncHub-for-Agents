// Package syncengine orchestrates a full sync pass: collect, reconcile, apply.
package syncengine

import (
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

		data, err := os.ReadFile(abs)
		if err != nil {
			return err
		}
		if spec.Scanner != nil && spec.Scanner.ShouldBlock(rel, data) {
			out.Blocked = append(out.Blocked, repoRel)
			return nil
		}

		hash, err := state.HashFile(abs)
		if err != nil {
			return err
		}
		fi, err := d.Info()
		if err != nil {
			return err
		}
		out.Snapshot[repoRel] = state.FileMeta{
			Hash:    hash,
			ModTime: fi.ModTime().Unix(),
			Size:    fi.Size(),
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
