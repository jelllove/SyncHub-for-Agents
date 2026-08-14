package syncengine

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"sort"
	"strings"

	"github.com/qinqingxu/acsync/internal/conflict"
	"github.com/qinqingxu/acsync/internal/installplan"
	"github.com/qinqingxu/acsync/internal/portableconfig"
	"github.com/qinqingxu/acsync/internal/resource"
	"github.com/qinqingxu/acsync/internal/secret"
	"github.com/qinqingxu/acsync/internal/state"
)

const portablePrefix = "agents/_portable/"

func SplitRemoteSnapshot(
	remote state.Snapshot,
	specs map[string]resource.Spec,
) (owned, untouched state.Snapshot, blocked []resource.Issue) {
	owned = state.Snapshot{}
	untouched = state.Snapshot{}
	for repoRel, meta := range remote {
		if isInternalPortablePath(repoRel) {
			owned[repoRel] = meta
			continue
		}
		ref, err := resource.ParseRepoPath(repoRel)
		if err != nil {
			if strings.HasPrefix(repoRel, portablePrefix) {
				blocked = append(blocked, resource.Issue{
					Path:    repoRel,
					Code:    "remote-path-invalid",
					Message: err.Error(),
				})
				continue
			}
			untouched[repoRel] = meta
			continue
		}
		spec, ok := specForRef(specs, ref)
		if !ok || !refMatchesSpec(ref, spec) {
			untouched[repoRel] = meta
			continue
		}
		owned[repoRel] = meta
	}
	return owned, untouched, blocked
}

func ValidateRemoteResources(
	repoDir string,
	owned state.Snapshot,
	specs map[string]resource.Spec,
	codecs *portableconfig.Registry,
	goos, userHome string,
) (state.Snapshot, []resource.Issue) {
	valid := state.Snapshot{}
	var blocked []resource.Issue
	for repoRel, meta := range owned {
		if isInternalPortablePath(repoRel) {
			if err := validateInternalRemote(repoDir, repoRel, specs); err != nil {
				blocked = append(
					blocked,
					remoteIssue("", repoRel, "remote-internal-invalid", err),
				)
				continue
			}
			valid[repoRel] = meta
			continue
		}

		ref, err := resource.ParseRepoPath(repoRel)
		if err != nil {
			blocked = append(blocked, remoteIssue("", repoRel, "remote-path-invalid", err))
			continue
		}
		spec, ok := specForRef(specs, ref)
		if !ok || !refMatchesSpec(ref, spec) {
			blocked = append(blocked, resource.Issue{
				Path:    repoRel,
				Code:    "remote-resource-unknown",
				Message: "remote file does not belong to an enabled resource",
			})
			continue
		}
		filename := filepath.Join(repoDir, filepath.FromSlash(repoRel))
		info, err := os.Lstat(filename)
		if err != nil {
			blocked = append(blocked, remoteIssue(spec.Key, repoRel, "remote-read-failed", err))
			continue
		}
		if info.Mode()&os.ModeSymlink != 0 {
			blocked = append(blocked, resource.Issue{
				ResourceKey: spec.Key,
				Path:        repoRel,
				Code:        "remote-link-blocked",
				Message:     "repository symbolic links are not portable resources",
			})
			continue
		}
		data, err := os.ReadFile(filename)
		if err != nil {
			blocked = append(blocked, remoteIssue(spec.Key, repoRel, "remote-read-failed", err))
			continue
		}
		scanner := secret.NewScanner(spec.Exclude, spec.KeyPatterns)
		detected, err := scanner.Scan(ref.Relative, data)
		if err != nil {
			blocked = append(blocked, remoteIssue(spec.Key, repoRel, "remote-scan-failed", err))
			continue
		}
		if detected {
			blocked = append(blocked, resource.Issue{
				ResourceKey: spec.Key,
				Path:        repoRel,
				Code:        "remote-secret-detected",
				Message:     "remote portable artifact contains secret-looking data",
				Bytes:       int64(len(data)),
			})
			continue
		}
		if spec.Strategy == resource.StrategyInstallManifest {
			if ref.Relative != "manifest.json" {
				blocked = append(blocked, resource.Issue{
					ResourceKey: spec.Key,
					Path:        repoRel,
					Code:        "remote-install-manifest-invalid",
					Message:     "install resources may contain only manifest.json",
				})
				continue
			}
			if _, err := installplan.ValidateDeclarations(spec.Installer, data); err != nil {
				blocked = append(blocked, remoteIssue(
					spec.Key,
					repoRel,
					"remote-install-manifest-invalid",
					err,
				))
				continue
			}
		}
		if spec.Transformer != "" {
			if codecs == nil {
				blocked = append(blocked, resource.Issue{
					ResourceKey: spec.Key,
					Path:        repoRel,
					Code:        "remote-projector-unavailable",
					Message:     "remote resource requires a configuration projector",
				})
				continue
			}
			projected, err := codecs.Project(spec.Transformer, ref.Relative, goos, userHome, data)
			if err != nil {
				blocked = append(blocked, remoteIssue(spec.Key, repoRel, "remote-projection-invalid", err))
				continue
			}
			if !documentsEqual(ref.Relative, data, projected) {
				blocked = append(blocked, resource.Issue{
					ResourceKey: spec.Key,
					Path:        repoRel,
					Code:        "remote-nonportable-fields",
					Message:     "remote structured artifact contains fields excluded by its portable policy",
				})
				continue
			}
		}
		valid[repoRel] = meta
	}
	return valid, blocked
}

func validateInternalRemote(
	repoDir, repoRel string,
	specs map[string]resource.Spec,
) error {
	filename := filepath.Join(repoDir, filepath.FromSlash(repoRel))
	info, err := os.Lstat(filename)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("repository symbolic links are not valid internal metadata")
	}
	data, err := os.ReadFile(filename)
	if err != nil {
		return err
	}
	parts := strings.Split(repoRel, "/")
	if len(parts) == 5 && parts[3] == "install" {
		if parts[4] != "skill-dependencies.json" {
			return fmt.Errorf("invalid install metadata path")
		}
		if _, err := installplan.ValidateDeclarations("skill-dependencies", data); err != nil {
			return fmt.Errorf("validate Skill dependency declarations: %w", err)
		}
		return scanInternalData(repoRel, data)
	}
	if len(parts) != 6 || parts[3] != "conflicts" {
		return fmt.Errorf("invalid internal portable path")
	}
	recordPath := filepath.Join(
		repoDir,
		"agents",
		"_portable",
		"config",
		"conflicts",
		parts[4],
		"record.json",
	)
	recordData, err := os.ReadFile(recordPath)
	if err != nil {
		return fmt.Errorf("read conflict record: %w", err)
	}
	var record conflict.Record
	if err := json.Unmarshal(recordData, &record); err != nil {
		return fmt.Errorf("parse conflict record: %w", err)
	}
	if record.ID != parts[4] || record.ResourceKey == "" {
		return fmt.Errorf("conflict record identity does not match its directory")
	}
	ref, err := resource.ParseRepoPath(record.RepoRel)
	if err != nil {
		return fmt.Errorf("validate conflict canonical path: %w", err)
	}
	expectedKey := ref.Provider + "/" + ref.ResourceID
	if ref.Common {
		expectedKey = "common/" + ref.ResourceID
	}
	if record.ResourceKey != expectedKey {
		return fmt.Errorf("conflict record resource key does not match its canonical path")
	}
	for _, required := range []string{"base", "local", "remote"} {
		if _, err := os.Stat(filepath.Join(filepath.Dir(recordPath), required)); err != nil {
			return fmt.Errorf("validate conflict variant %s: %w", required, err)
		}
	}
	if parts[5] == "record.json" {
		return scanInternalData(repoRel, data)
	}
	spec, ok := specForRef(specs, ref)
	if !ok || !refMatchesSpec(ref, spec) {
		return scanInternalData(repoRel, data)
	}
	return ConflictScanner(specs)(record, parts[5], data)
}

func scanInternalData(repoRel string, data []byte) error {
	scanner := secret.NewScanner(nil, []string{
		"apiKey", "token", "secret", "password", "oauth", "refresh_token",
	})
	blocked, err := scanner.Scan(repoRel, data)
	if err != nil {
		return err
	}
	if blocked {
		return fmt.Errorf("secret-looking data detected")
	}
	return nil
}

func conflictIDsForIssues(issues []resource.Issue) map[string]struct{} {
	ids := map[string]struct{}{}
	for _, issue := range issues {
		parts := strings.Split(issue.Path, "/")
		if len(parts) >= 5 &&
			parts[0] == "agents" &&
			parts[1] == "_portable" &&
			parts[2] == "config" &&
			parts[3] == "conflicts" &&
			parts[4] != "" {
			ids[parts[4]] = struct{}{}
		}
	}
	return ids
}

func specForRef(specs map[string]resource.Spec, ref resource.RepoRef) (resource.Spec, bool) {
	key := ref.Provider + "/" + ref.ResourceID
	if ref.Common {
		key = "common/" + ref.ResourceID
	}
	spec, ok := specs[key]
	return spec, ok
}

func refMatchesSpec(ref resource.RepoRef, spec resource.Spec) bool {
	return ref.Category == spec.Category &&
		ref.Portable == (spec.Layout == resource.LayoutPortable)
}

func isInternalPortablePath(repoRel string) bool {
	parts := strings.Split(repoRel, "/")
	if len(parts) == 5 &&
		parts[0] == "agents" &&
		parts[1] == "_portable" &&
		parts[2] == "config" &&
		parts[3] == "install" &&
		parts[4] != "" {
		return true
	}
	if len(parts) == 6 &&
		parts[0] == "agents" &&
		parts[1] == "_portable" &&
		parts[2] == "config" &&
		parts[3] == "conflicts" &&
		parts[4] != "" {
		switch parts[5] {
		case "record.json", "base", "local", "remote":
			return true
		}
	}
	return false
}

func documentsEqual(rel string, left, right []byte) bool {
	_, leftDocument, leftErr := portableconfig.Parse(rel, left)
	_, rightDocument, rightErr := portableconfig.Parse(rel, right)
	if leftErr == nil && rightErr == nil {
		return reflect.DeepEqual(leftDocument, rightDocument)
	}
	return bytes.Equal(left, right)
}

func remoteIssue(resourceKey, repoRel, code string, err error) resource.Issue {
	return resource.Issue{
		ResourceKey: resourceKey,
		Path:        repoRel,
		Code:        code,
		Message:     fmt.Sprintf("%v", err),
	}
}

func specForRepoPath(specs map[string]resource.Spec, repoRel string) (resource.Spec, error) {
	ref, err := resource.ParseRepoPath(repoRel)
	if err != nil {
		return resource.Spec{}, err
	}
	spec, ok := specForRef(specs, ref)
	if !ok || !refMatchesSpec(ref, spec) {
		return resource.Spec{}, fmt.Errorf("no enabled resource owns repository path %q", repoRel)
	}
	return spec, nil
}

func skippedRepoPrefixes(issues []resource.Issue, specs map[string]resource.Spec) []string {
	seen := map[string]struct{}{}
	for _, issue := range issues {
		spec, ok := specs[issue.ResourceKey]
		if !ok {
			continue
		}
		prefix := resourceRepoPrefix(spec)
		if issue.Path != "" && !filepath.IsAbs(issue.Path) {
			if repoRel, err := spec.RepoPath(filepath.ToSlash(issue.Path)); err == nil {
				prefix = repoRel
			}
		}
		if prefix != "" {
			seen[prefix] = struct{}{}
		}
	}
	prefixes := make([]string, 0, len(seen))
	for prefix := range seen {
		prefixes = append(prefixes, prefix)
	}
	sort.Strings(prefixes)
	return prefixes
}

func blockedRepoPaths(issues []resource.Issue, specs map[string]resource.Spec) []string {
	seen := map[string]struct{}{}
	for _, issue := range issues {
		if strings.HasPrefix(issue.Path, "agents/") {
			seen[issue.Path] = struct{}{}
			continue
		}

		spec, ok := specs[issue.ResourceKey]
		if !ok || issue.Path == "" || filepath.IsAbs(issue.Path) {
			continue
		}
		repoRel, err := spec.RepoPath(filepath.ToSlash(issue.Path))
		if err == nil {
			seen[repoRel] = struct{}{}
		}
	}
	paths := make([]string, 0, len(seen))
	for repoRel := range seen {
		paths = append(paths, repoRel)
	}
	sort.Strings(paths)
	return paths
}

func PrepareResourceActions(
	base state.Snapshot,
	local state.Snapshot,
	remoteSnapshot state.Snapshot,
	validRemote state.Snapshot,
	specs map[string]resource.Spec,
	skipped []resource.Issue,
	blocked []resource.Issue,
) ([]Action, []string) {
	baseOwned, _, _ := SplitRemoteSnapshot(base, specs)
	local = cloneSnapshot(local)
	validRemote = cloneSnapshot(validRemote)
	for repoRel, meta := range validRemote {
		if isInternalPortablePath(repoRel) {
			local[repoRel] = meta
		}
	}
	skipPrefixes := skippedRepoPrefixes(skipped, specs)
	local = withoutPrefixes(local, skipPrefixes)
	baseOwned = withoutPrefixes(baseOwned, skipPrefixes)
	validRemote = withoutPrefixes(validRemote, skipPrefixes)
	blockedPaths := blockedRepoPaths(blocked, specs)
	for _, repoRel := range blockedPaths {
		if meta, ok := remoteSnapshot[repoRel]; ok {
			validRemote[repoRel] = meta
		}
	}
	return ReconcileWithBlocked(baseOwned, local, validRemote, blockedPaths), blockedPaths
}

func resourceRepoPrefix(spec resource.Spec) string {
	const sentinel = "__acsync_resource_root__"
	repoRel, err := spec.RepoPath(sentinel)
	if err != nil {
		return ""
	}
	return strings.TrimSuffix(repoRel, sentinel)
}

func withoutPrefixes(snapshot state.Snapshot, prefixes []string) state.Snapshot {
	out := state.Snapshot{}
	for repoRel, meta := range snapshot {
		skip := false
		for _, prefix := range prefixes {
			if pathHasPrefix(repoRel, prefix) {
				skip = true
				break
			}
		}
		if !skip {
			out[repoRel] = meta
		}
	}
	return out
}

func pathHasPrefix(repoRel, prefix string) bool {
	prefix = strings.TrimSuffix(prefix, "/")
	return repoRel == prefix || strings.HasPrefix(repoRel, prefix+"/")
}

func preserveSnapshot(
	previous, current state.Snapshot,
	preserve map[string]struct{},
) state.Snapshot {
	next := cloneSnapshot(current)
	for repoRel := range preserve {
		if meta, ok := previous[repoRel]; ok {
			next[repoRel] = meta
		} else {
			delete(next, repoRel)
		}
	}
	return next
}

func conflictID(repoRel string, variants ...[]byte) string {
	digest := sha256.New()
	_, _ = digest.Write([]byte(repoRel))
	for _, data := range variants {
		_, _ = digest.Write([]byte{0})
		_, _ = digest.Write(data)
	}
	return hex.EncodeToString(digest.Sum(nil))[:24]
}

func conflictExists(store *conflict.Store, repoRel string) (bool, error) {
	records, err := store.List()
	if err != nil {
		return false, err
	}
	for _, record := range records {
		if record.RepoRel == repoRel {
			return true, nil
		}
	}
	return false, nil
}

func ConflictScanner(specs map[string]resource.Spec) conflict.Scanner {
	return func(record conflict.Record, variant string, data []byte) error {
		if len(bytes.TrimSpace(data)) == 0 {
			return nil
		}
		ref, err := resource.ParseRepoPath(record.RepoRel)
		if err != nil {
			return err
		}
		spec, ok := specForRef(specs, ref)
		if !ok || !refMatchesSpec(ref, spec) {
			return scanInternalData(path.Join("conflict", variant, ref.Relative), data)
		}
		scanner := secret.NewScanner(spec.Exclude, spec.KeyPatterns)
		blocked, err := scanner.Scan(path.Join("conflict", variant, ref.Relative), data)
		if err != nil {
			return err
		}
		if blocked {
			return fmt.Errorf("secret-looking data detected")
		}
		return nil
	}
}

func SnapshotRepo(repoDir string) (state.Snapshot, error) {
	snapshot := state.Snapshot{}
	agentsRoot := filepath.Join(repoDir, "agents")
	rootInfo, err := os.Lstat(agentsRoot)
	if errors.Is(err, os.ErrNotExist) {
		return snapshot, nil
	} else if err != nil {
		return nil, err
	}
	if rootInfo.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("repository agents root must not be a symbolic link")
	}
	err = filepath.WalkDir(agentsRoot, func(filename string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(repoDir, filename)
		if err != nil {
			return err
		}
		repoRel := path.Clean(filepath.ToSlash(relative))
		if !strings.HasPrefix(repoRel, "agents/") {
			return nil
		}
		info, err := os.Lstat(filename)
		if err != nil {
			return err
		}
		var hash string
		var size int64
		if info.Mode()&os.ModeSymlink != 0 {
			target, err := os.Readlink(filename)
			if err != nil {
				return err
			}
			hash = hashBytes([]byte(target))
			size = int64(len(target))
		} else {
			hash, err = state.HashFile(filename)
			if err != nil {
				return err
			}
			size = info.Size()
		}
		snapshot[repoRel] = state.FileMeta{
			Hash:    hash,
			ModTime: info.ModTime().Unix(),
			Size:    size,
		}
		return nil
	})
	return snapshot, err
}
