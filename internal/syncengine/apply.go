package syncengine

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/qinqingxu/acsync/internal/state"
)

// Applier executes reconcile actions against the repo and local agent dirs.
type Applier struct {
	RepoDir string
	Specs   map[string]AgentSpec // by agent name, for local path resolution
	Sources map[string]string    // repo-relative path -> local absolute source
	Now     time.Time
}

// Apply runs all actions in order.
func (a *Applier) Apply(actions []Action) error {
	for _, act := range actions {
		if err := a.applyOne(act); err != nil {
			return fmt.Errorf("apply %s %s: %w", act.Type, act.RepoRel, err)
		}
	}
	return nil
}

func (a *Applier) applyOne(act Action) error {
	switch act.Type {
	case PushToRemote:
		src := a.Sources[act.RepoRel]
		if src == "" {
			return fmt.Errorf("no local source for push")
		}
		return a.pushToRemote(src, act.RepoRel)
	case PullToLocal:
		dst, err := a.localPathFor(act.RepoRel)
		if err != nil {
			return err
		}
		return copyFile(filepath.Join(a.RepoDir, filepath.FromSlash(act.RepoRel)), dst)
	case DeleteRemote:
		return a.softDeleteRemote(act.RepoRel)
	case DeleteLocal:
		dst, err := a.localPathFor(act.RepoRel)
		if err != nil {
			return err
		}
		if err := os.Remove(dst); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	case RemoveRemote:
		err := os.Remove(filepath.Join(a.RepoDir, filepath.FromSlash(act.RepoRel)))
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	default:
		return fmt.Errorf("unknown action type %v", act.Type)
	}
}

func (a *Applier) pushToRemote(src, repoRel string) (retErr error) {
	dst := filepath.Join(a.RepoDir, filepath.FromSlash(repoRel))
	stageDir := filepath.Join(a.RepoDir, ".git", "acsync-stage")
	if err := os.MkdirAll(stageDir, 0o700); err != nil {
		return err
	}
	stageFile, err := os.CreateTemp(stageDir, "source-*")
	if err != nil {
		return err
	}
	staged := stageFile.Name()
	if err := stageFile.Close(); err != nil {
		return err
	}
	defer func() {
		if err := os.Remove(staged); err != nil && !os.IsNotExist(err) && retErr == nil {
			retErr = fmt.Errorf("remove staged source: %w", err)
		}
	}()

	if err := copyFile(src, staged); err != nil {
		return err
	}
	data, err := os.ReadFile(staged)
	if err != nil {
		return err
	}
	parts := strings.Split(repoRel, "/")
	if len(parts) < 4 || parts[0] != "agents" {
		return fmt.Errorf("cannot map repo path %q", repoRel)
	}
	spec, ok := a.Specs[parts[1]]
	if !ok {
		return fmt.Errorf("no spec for agent %q", parts[1])
	}
	rel := strings.Join(parts[3:], "/")
	if spec.Scanner != nil {
		blocked, err := spec.Scanner.Scan(rel, data)
		if err != nil {
			return fmt.Errorf("scan staged source: %w", err)
		}
		if blocked {
			return fmt.Errorf("staged source was blocked by secret scanner")
		}
	}
	return copyFile(staged, dst)
}

// localPathFor maps a repo-relative path back to the local agent file path.
// Expects "agents/<name>/<sub>/<rest...>".
func (a *Applier) localPathFor(repoRel string) (string, error) {
	parts := strings.Split(repoRel, "/")
	if len(parts) < 4 || parts[0] != "agents" {
		return "", fmt.Errorf("cannot map repo path %q", repoRel)
	}
	name := parts[1]
	rest := parts[3:]
	spec, ok := a.Specs[name]
	if !ok {
		return "", fmt.Errorf("no spec for agent %q", name)
	}
	return filepath.Join(spec.Root, filepath.FromSlash(strings.Join(rest, "/"))), nil
}

func (a *Applier) softDeleteRemote(repoRel string) error {
	repoPath := filepath.Join(a.RepoDir, filepath.FromSlash(repoRel))
	trashPath := filepath.Join(a.RepoDir, ".trash", "files", filepath.FromSlash(repoRel))

	if _, err := os.Stat(repoPath); os.IsNotExist(err) {
		return nil // already gone
	}
	if err := os.MkdirAll(filepath.Dir(trashPath), 0o755); err != nil {
		return err
	}
	if err := os.Rename(repoPath, trashPath); err != nil {
		// Cross-device fallback: copy then remove.
		if cerr := copyFile(repoPath, trashPath); cerr != nil {
			return cerr
		}
		if rerr := os.Remove(repoPath); rerr != nil {
			return rerr
		}
	}
	return a.recordTrash(repoRel)
}

func (a *Applier) recordTrash(repoRel string) error {
	idxPath := filepath.Join(a.RepoDir, ".trash", "index.json")
	idx := map[string]int64{}
	if data, err := os.ReadFile(idxPath); err == nil {
		_ = json.Unmarshal(data, &idx)
	}
	idx[repoRel] = a.Now.Unix()
	data, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(idxPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(idxPath, data, 0o644)
}

func copyFile(src, dst string) (retErr error) {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	tmp := dst + ".tmp"
	defer func() {
		if err := os.Remove(tmp); err != nil && !os.IsNotExist(err) && retErr == nil {
			retErr = fmt.Errorf("remove temporary file: %w", err)
		}
	}()
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, dst)
}

// SnapshotRepo hashes every file under the repo's agents/ tree, keyed by
// repo-relative forward-slash path. It skips .git, .trash, and top-level
// metadata files.
func SnapshotRepo(repoDir string) (state.Snapshot, error) {
	snap := state.Snapshot{}
	agentsRoot := filepath.Join(repoDir, "agents")
	if _, err := os.Stat(agentsRoot); os.IsNotExist(err) {
		return snap, nil
	}

	err := filepath.WalkDir(agentsRoot, func(abs string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		relOS, err := filepath.Rel(repoDir, abs)
		if err != nil {
			return err
		}
		rel := filepath.ToSlash(relOS)
		if !strings.HasPrefix(rel, "agents/") {
			return nil
		}
		hash, err := state.HashFile(abs)
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		snap[path.Clean(rel)] = state.FileMeta{
			Hash:    hash,
			ModTime: info.ModTime().Unix(),
			Size:    info.Size(),
		}
		return nil
	})
	return snap, err
}
