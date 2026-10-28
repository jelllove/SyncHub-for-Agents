package syncengine

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/qinqingxu/synchub-for-agents/internal/portableconfig"
	"github.com/qinqingxu/synchub-for-agents/internal/resource"
	"github.com/qinqingxu/synchub-for-agents/internal/resourcecollect"
	"github.com/qinqingxu/synchub-for-agents/internal/state"
)

type ResourceApplier struct {
	RepoDir  string
	Home     string
	GOOS     string
	UserHome string
	Codecs   *portableconfig.Registry
	Base     *state.BaseStore
	Now      func() time.Time
	Symlink  func(oldname, newname string) error
}

type aliasRecord struct {
	Canonical string `json:"canonical"`
	Mode      string `json:"mode"`
}

func (a *ResourceApplier) PushArtifact(artifact resourcecollect.Artifact) error {
	data, err := os.ReadFile(artifact.StagePath)
	if err != nil {
		return fmt.Errorf("read staged artifact %q: %w", artifact.RepoRel, err)
	}
	if artifact.Hash != "" && hashBytes(data) != artifact.Hash {
		return fmt.Errorf("staged artifact %q changed after collection", artifact.RepoRel)
	}
	target, err := safeWritableJoin(a.RepoDir, artifact.RepoRel)
	if err != nil {
		return err
	}
	return writeAtomic(target, data, 0o600)
}

func (a *ResourceApplier) Restore(spec resource.Spec, repoRel string) error {
	ref, err := resource.ParseRepoPath(repoRel)
	if err != nil {
		return err
	}
	if !refMatchesSpec(ref, spec) {
		return fmt.Errorf("repository path %q does not belong to resource %q", repoRel, spec.Key)
	}
	source, err := safeJoin(a.RepoDir, repoRel)
	if err != nil {
		return err
	}
	remote, err := os.ReadFile(source)
	if err != nil {
		return fmt.Errorf("read remote resource %q: %w", repoRel, err)
	}
	return a.restoreBytes(spec, repoRel, ref.Relative, remote)
}

func (a *ResourceApplier) RestoreBytes(spec resource.Spec, relative string, data []byte) error {
	if _, err := safeJoin(".", relative); err != nil {
		return err
	}
	repoRel, err := spec.RepoPath(relative)
	if err != nil {
		return err
	}
	return a.restoreBytes(spec, repoRel, relative, data)
}

func (a *ResourceApplier) restoreBytes(spec resource.Spec, repoRel, relative string, remote []byte) error {
	targets, err := a.materializeTargets(spec)
	if err != nil {
		return err
	}
	var base []byte
	if spec.Strategy == resource.StrategyStructuredMerge && a.Base != nil {
		var ok bool
		base, ok, err = a.Base.Get(repoRel)
		if err != nil {
			return err
		}
		if !ok {
			base = nil
		}
	}
	for _, root := range targets {
		target, err := safeWritableJoin(root, relative)
		if err != nil {
			return err
		}
		data := remote
		if spec.Strategy == resource.StrategyStructuredMerge {
			if a.Codecs == nil {
				return fmt.Errorf("resource %q requires a configuration codec", spec.Key)
			}
			local, readErr := os.ReadFile(target)
			if readErr != nil && !errors.Is(readErr, os.ErrNotExist) {
				return fmt.Errorf("read local resource %q: %w", target, readErr)
			}
			if errors.Is(readErr, os.ErrNotExist) {
				local = nil
			}
			data, err = a.Codecs.Restore(
				spec.Transformer,
				relative,
				a.GOOS,
				a.UserHome,
				local,
				base,
				remote,
			)
			if err != nil {
				return fmt.Errorf("restore resource %q: %w", repoRel, err)
			}
		}
		if err := writeAtomic(target, data, 0o600); err != nil {
			return err
		}
	}
	return nil
}

func (a *ResourceApplier) targetsForDelete(spec resource.Spec) ([]string, error) {
	if len(spec.Targets) == 0 {
		return nil, fmt.Errorf("resource %q has no restore targets", spec.Key)
	}
	if spec.SharedAs == "" && spec.Provider != "common" {
		return append([]string(nil), spec.Targets...), nil
	}
	targets := []string{filepath.Clean(spec.Targets[0])}
	for _, rawAlias := range spec.Targets[1:] {
		alias := filepath.Clean(rawAlias)
		info, err := os.Lstat(alias)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			continue
		}
		if !info.IsDir() {
			return nil, fmt.Errorf("shared alias %q exists and is not a directory", alias)
		}
		targets = append(targets, alias)
	}
	return targets, nil
}

func (a *ResourceApplier) Delete(spec resource.Spec, repoRel string) error {
	ref, err := resource.ParseRepoPath(repoRel)
	if err != nil {
		return err
	}

	if !refMatchesSpec(ref, spec) {
		return fmt.Errorf("repository path %q does not belong to resource %q", repoRel, spec.Key)
	}
	targets, err := a.targetsForDelete(spec)
	if err != nil {
		return err
	}
	for index, root := range targets {
		target, err := safeDeletableJoin(root, ref.Relative)
		if err != nil {
			return err
		}
		if spec.Category == resource.CategorySkills || spec.Category == resource.CategoryPlugins {
			if err := a.recoverBeforeDelete(spec, ref.Relative, target, index); err != nil {
				return err
			}
			continue
		}
		if err := os.Remove(target); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		removeEmptyParents(filepath.Dir(target), root)
	}
	return nil
}

func (a *ResourceApplier) SoftDeleteRemote(repoRel string) error {
	repoPath, err := safeDeletableJoin(a.RepoDir, repoRel)
	if err != nil {
		return err
	}
	trashRoot := filepath.Join(a.RepoDir, ".trash", "files")
	trashPath, err := safeJoin(trashRoot, repoRel)
	if err != nil {
		return err
	}
	if _, err := os.Stat(repoPath); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(trashPath), 0o755); err != nil {
		return err
	}
	if err := os.Rename(repoPath, trashPath); err != nil {
		data, readErr := os.ReadFile(repoPath)
		if readErr != nil {
			return errors.Join(err, readErr)
		}
		if writeErr := writeAtomic(trashPath, data, 0o600); writeErr != nil {
			return errors.Join(err, writeErr)
		}
		if removeErr := os.Remove(repoPath); removeErr != nil {
			return errors.Join(err, removeErr)
		}
	}
	return a.recordTrash(repoRel)
}

func (a *ResourceApplier) recordTrash(repoRel string) error {
	indexPath := filepath.Join(a.RepoDir, ".trash", "index.json")
	index := map[string]int64{}
	data, err := os.ReadFile(indexPath)
	if err == nil {
		if err := json.Unmarshal(data, &index); err != nil {
			return fmt.Errorf("load trash index: %w", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	now := time.Now
	if a.Now != nil {
		now = a.Now
	}
	index[repoRel] = now().Unix()
	data, err = json.MarshalIndent(index, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(indexPath, append(data, '\n'), 0o600)
}

func (a *ResourceApplier) materializeTargets(spec resource.Spec) ([]string, error) {
	if len(spec.Targets) == 0 {
		return nil, fmt.Errorf("resource %q has no restore targets", spec.Key)
	}
	primary := filepath.Clean(spec.Targets[0])
	if spec.SharedAs == "" && spec.Provider != "common" {
		return append([]string(nil), spec.Targets...), nil
	}
	if err := os.MkdirAll(primary, 0o755); err != nil {
		return nil, err
	}
	writeTargets := []string{primary}
	for _, rawAlias := range spec.Targets[1:] {
		alias := filepath.Clean(rawAlias)
		mode, err := a.ensureAlias(primary, alias)
		if err != nil {
			return nil, err
		}
		if mode == "managed-copy" {
			writeTargets = append(writeTargets, alias)
		}
		if err := a.saveAlias(alias, aliasRecord{Canonical: primary, Mode: mode}); err != nil {
			return nil, err
		}
	}
	return writeTargets, nil
}

func (a *ResourceApplier) ensureAlias(primary, alias string) (string, error) {
	info, err := os.Lstat(alias)
	if err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			target, evalErr := filepath.EvalSymlinks(alias)
			if evalErr != nil {
				return "", fmt.Errorf("resolve shared alias %q: %w", alias, evalErr)
			}
			target, evalErr = filepath.Abs(target)
			if evalErr != nil {
				return "", evalErr
			}
			want, evalErr := filepath.Abs(primary)
			if evalErr != nil {
				return "", evalErr
			}
			if !samePath(target, want) {
				return "", fmt.Errorf("shared alias %q points outside canonical target %q", alias, primary)
			}
			return "symlink", nil
		}

		if info.IsDir() {
			return "managed-copy", nil
		}
		return "", fmt.Errorf("shared alias %q exists and is not a directory", alias)
	}
	if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(alias), 0o755); err != nil {
		return "", err
	}
	link := a.Symlink
	if link == nil {
		link = os.Symlink
	}
	if err := link(primary, alias); err == nil {
		return "symlink", nil
	}
	if err := os.MkdirAll(alias, 0o755); err != nil {
		return "", fmt.Errorf("create managed alias copy %q: %w", alias, err)
	}
	return "managed-copy", nil
}

func samePath(left, right string) bool {
	left = filepath.Clean(left)
	right = filepath.Clean(right)
	if runtime.GOOS == "windows" {
		return strings.EqualFold(left, right)
	}
	return left == right
}

func (a *ResourceApplier) saveAlias(alias string, record aliasRecord) error {
	if strings.TrimSpace(a.Home) == "" {
		return fmt.Errorf("resource applier home is required for shared aliases")
	}
	filename := filepath.Join(a.Home, "aliases.json")
	records := map[string]aliasRecord{}
	data, err := os.ReadFile(filename)
	if err == nil {
		if err := json.Unmarshal(data, &records); err != nil {
			return fmt.Errorf("load aliases: %w", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	records[filepath.Clean(alias)] = record
	data, err = json.MarshalIndent(records, "", "  ")
	if err != nil {
		return err
	}
	return writeAtomic(filename, append(data, '\n'), 0o600)
}

func (a *ResourceApplier) recoverBeforeDelete(
	spec resource.Spec,
	relative, target string,
	targetIndex int,
) error {
	if _, err := os.Stat(target); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return err
	}
	now := time.Now
	if a.Now != nil {
		now = a.Now
	}
	resourcePath := strings.ReplaceAll(spec.Key, "/", string(filepath.Separator))
	recoveryRoot := filepath.Join(
		a.Home,
		"local-trash",
		now().UTC().Format("20060102T150405Z"),
		resourcePath,
	)
	if targetIndex > 0 {
		recoveryRoot = filepath.Join(recoveryRoot, fmt.Sprintf("target-%d", targetIndex+1))
	}
	recovery, err := safeJoin(recoveryRoot, relative)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(recovery), 0o700); err != nil {
		return err
	}
	if err := os.Rename(target, recovery); err != nil {
		return fmt.Errorf("move %q to local recovery: %w", target, err)
	}
	return nil
}

func safeJoin(root, relative string) (string, error) {
	if strings.TrimSpace(root) == "" {
		return "", fmt.Errorf("target root is empty")
	}
	if strings.TrimSpace(relative) == "" ||
		strings.Contains(relative, `\`) ||
		filepath.IsAbs(relative) {
		return "", fmt.Errorf("invalid relative target path %q", relative)
	}
	cleaned := filepath.Clean(filepath.FromSlash(relative))
	if cleaned == "." || cleaned == ".." ||
		strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("target path %q escapes its root", relative)
	}
	root = filepath.Clean(root)
	target := filepath.Join(root, cleaned)
	back, err := filepath.Rel(root, target)
	if err != nil || back == ".." || strings.HasPrefix(back, ".."+string(filepath.Separator)) || filepath.IsAbs(back) {
		return "", fmt.Errorf("target path %q escapes its root", relative)
	}
	return target, nil
}

func safeWritableJoin(root, relative string) (string, error) {
	target, err := safeJoin(root, relative)
	if err != nil {
		return "", err
	}
	rootAbsolute, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	rootCanonical := rootAbsolute
	rootExists := false
	if _, err := os.Lstat(rootAbsolute); err == nil {
		rootExists = true
		rootCanonical, err = filepath.EvalSymlinks(rootAbsolute)
		if err != nil {
			return "", err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	if !rootExists {
		return target, nil
	}

	if info, err := os.Lstat(target); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return "", fmt.Errorf("target %q is a symbolic link", target)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}

	ancestor := filepath.Dir(target)
	for {
		if _, err := os.Lstat(ancestor); err == nil {
			break
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		parent := filepath.Dir(ancestor)
		if parent == ancestor {
			return "", fmt.Errorf("cannot resolve target ancestor for %q", target)
		}
		ancestor = parent
	}
	ancestorCanonical, err := filepath.EvalSymlinks(ancestor)
	if err != nil {
		return "", err
	}
	if !pathWithinRoot(rootCanonical, ancestorCanonical) {
		return "", fmt.Errorf("target %q resolves outside root %q", target, root)
	}
	return target, nil
}

func safeDeletableJoin(root, relative string) (string, error) {
	target, err := safeJoin(root, relative)
	if err != nil {
		return "", err
	}
	rootAbsolute, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	if _, err := os.Lstat(rootAbsolute); errors.Is(err, os.ErrNotExist) {
		return target, nil
	} else if err != nil {
		return "", err
	}
	rootCanonical, err := filepath.EvalSymlinks(rootAbsolute)
	if err != nil {
		return "", err
	}
	ancestor := filepath.Dir(target)
	for {
		if _, err := os.Lstat(ancestor); err == nil {
			break
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		parent := filepath.Dir(ancestor)
		if parent == ancestor {
			return "", fmt.Errorf("cannot resolve target ancestor for %q", target)
		}
		ancestor = parent
	}
	ancestorCanonical, err := filepath.EvalSymlinks(ancestor)
	if err != nil {
		return "", err
	}
	if !pathWithinRoot(rootCanonical, ancestorCanonical) {
		return "", fmt.Errorf("target %q resolves outside root %q", target, root)
	}
	return target, nil
}

func pathWithinRoot(root, candidate string) bool {
	relative, err := filepath.Rel(root, candidate)
	if err != nil {
		return false
	}
	if runtime.GOOS == "windows" {
		relative = strings.ToLower(relative)
	}
	return relative != ".." &&
		!strings.HasPrefix(relative, ".."+string(filepath.Separator)) &&
		!filepath.IsAbs(relative)
}

func writeAtomic(filename string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(filename), "."+filepath.Base(filename)+".tmp-*")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	if err := temp.Chmod(mode); err != nil {
		temp.Close()
		return err
	}
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(tempName, filename)
}

func removeEmptyParents(current, root string) {
	root = filepath.Clean(root)
	for current != root {
		if err := os.Remove(current); err != nil {
			return
		}
		current = filepath.Dir(current)
	}
}

func hashBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
