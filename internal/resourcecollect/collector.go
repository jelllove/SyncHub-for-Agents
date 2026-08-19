package resourcecollect

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/qinqingxu/acsync/internal/resource"
	"github.com/qinqingxu/acsync/internal/secret"
	"github.com/qinqingxu/acsync/internal/state"
)

type Projector interface {
	Project(transformer, rel, goos, home string, data []byte) ([]byte, error)
}

type InventoryProvider interface {
	InventoryContext(context.Context, resource.Spec) (map[string][]byte, error)
}

type Options struct {
	StageParent   string
	GOOS          string
	UserHome      string
	Projector     Projector
	Inventory     InventoryProvider
	Filter        resource.FilterPolicy
	ApprovedLinks ApprovedLinkStore
}

type Collector struct {
	options Options
}

func New(options Options) *Collector {
	if options.Filter.MaxFileSize == 0 {
		options.Filter = resource.DefaultFilterPolicy()
	}
	return &Collector{options: options}
}

type Artifact struct {
	RepoRel     string
	ResourceKey string
	Relative    string
	StagePath   string
	Targets     []string
	Hash        string
	Size        int64
	ModTime     int64
}

type Result struct {
	Snapshot  state.Snapshot
	Artifacts map[string]Artifact
	Blocked   []resource.Issue
	Skipped   []resource.Issue
	StageRoot string
}

func (r *Result) Close() error {
	if r.StageRoot == "" {
		return nil
	}
	stageRoot := r.StageRoot
	if err := os.RemoveAll(stageRoot); err != nil {
		return fmt.Errorf("remove resource stage %q: %w", stageRoot, err)
	}
	r.StageRoot = ""
	return nil
}

func (c *Collector) Collect(specs []resource.Spec) (Result, error) {
	return c.CollectContext(context.Background(), specs)
}

func (c *Collector) CollectContext(
	ctx context.Context,
	specs []resource.Spec,
) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	stageRoot, err := os.MkdirTemp(c.options.StageParent, "acsync-resources-*")
	if err != nil {
		return Result{}, fmt.Errorf("create resource stage: %w", err)
	}
	result := Result{
		Snapshot:  state.Snapshot{},
		Artifacts: map[string]Artifact{},
		StageRoot: stageRoot,
	}
	coalesced, issues, err := c.coalesce(ctx, specs)
	if err != nil {
		return Result{}, closeAfterError(&result, err)
	}
	result.Skipped = append(result.Skipped, issues...)
	for _, item := range coalesced {
		if err := ctx.Err(); err != nil {
			return Result{}, closeAfterError(&result, err)
		}
		if err := c.collectSpec(ctx, item, &result); err != nil {
			return Result{}, closeAfterError(&result, err)
		}
	}
	return result, nil
}

func closeAfterError(result *Result, cause error) error {
	return errors.Join(cause, result.Close())
}

func (c *Collector) coalesce(
	ctx context.Context,
	specs []resource.Spec,
) ([]resource.Spec, []resource.Issue, error) {
	byKey := make(map[string]resource.Spec, len(specs))
	order := make([]string, 0, len(specs))
	var issues []resource.Issue
	for _, spec := range specs {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		key := spec.Key
		if spec.SharedAs != "" {
			key = "common/" + spec.SharedAs
			spec.Key = key
			spec.Provider = "common"
		}
		root, err := ResolveRoot(spec.Root)
		if err != nil {
			issues = append(issues, resource.Issue{
				ResourceKey: spec.Key,
				Path:        spec.Root,
				Code:        "root-unavailable",
				Message:     err.Error(),
			})
			continue
		}
		spec.Root = root.CanonicalRoot
		if len(spec.Targets) == 0 {
			spec.Targets = []string{root.OriginalRoot}
		}
		existing, ok := byKey[key]
		if !ok {
			byKey[key] = spec
			order = append(order, key)
			continue
		}
		existing.Targets = appendUniquePaths(existing.Targets, spec.Targets...)
		byKey[key] = existing
	}
	out := make([]resource.Spec, 0, len(order))
	for _, key := range order {
		out = append(out, byKey[key])
	}
	return out, issues, nil
}

func (c *Collector) collectSpec(
	ctx context.Context,
	spec resource.Spec,
	result *Result,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if spec.Strategy == resource.StrategyInstallManifest {
		return c.collectInventory(ctx, spec, result)
	}
	scanner := secret.NewScanner(spec.Exclude, spec.KeyPatterns)
	ancestors := map[string]struct{}{canonicalLinkPath(spec.Root): {}}
	return c.walkDirectory(ctx, spec, spec.Root, "", ancestors, scanner, result)
}

func (c *Collector) collectInventory(
	ctx context.Context,
	spec resource.Spec,
	result *Result,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if c.options.Inventory == nil {
		result.Skipped = append(result.Skipped, resource.Issue{
			ResourceKey: spec.Key,
			Code:        "installer-unavailable",
			Message:     "resource inventory adapter is unavailable",
		})
		return nil
	}
	files, err := c.options.Inventory.InventoryContext(ctx, spec)
	if err != nil {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		result.Skipped = append(result.Skipped, issue(spec, "", "inventory-failed", err))
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	scanner := secret.NewScanner(spec.Exclude, spec.KeyPatterns)
	for rel, data := range files {
		if err := ctx.Err(); err != nil {
			return err
		}
		if allowed, code := c.options.Filter.Check(rel, int64(len(data)), spec.Strategy); !allowed {
			result.Skipped = append(result.Skipped, resource.Issue{
				ResourceKey: spec.Key,
				Path:        rel,
				Code:        code,
				Message:     "inventory file excluded by portable resource policy",
				Bytes:       int64(len(data)),
			})
			continue
		}
		blocked, scanErr := scanner.Scan(rel, data)
		if scanErr != nil {
			result.Blocked = append(result.Blocked, issue(spec, rel, "scan-failed", scanErr))
			continue
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if blocked {
			result.Blocked = append(result.Blocked, blockedIssue(spec, rel, int64(len(data))))
			continue
		}
		if err := c.stage(ctx, spec, rel, data, 0, result); err != nil {
			return err
		}
	}
	return nil
}

func (c *Collector) walkDirectory(
	ctx context.Context,
	spec resource.Spec,
	physicalDir string,
	logicalDir string,
	ancestors map[string]struct{},
	scanner *secret.Scanner,
	result *Result,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	entries, err := os.ReadDir(physicalDir)
	if err != nil {
		result.Skipped = append(result.Skipped, issue(spec, logicalDir, "walk-failed", err))
		return nil
	}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		physicalPath := filepath.Join(physicalDir, entry.Name())
		logicalPath := path.Join(logicalDir, entry.Name())
		if entry.Type()&os.ModeSymlink != 0 {
			if c.skipDirectoryPath(spec, logicalPath, scanner, result) {
				continue
			}
			if err := c.walkLink(ctx, spec, physicalPath, logicalPath, ancestors, scanner, result); err != nil {
				return err
			}
			continue
		}
		if entry.IsDir() {
			if c.skipDirectoryPath(spec, logicalPath, scanner, result) {
				continue
			}
			nextAncestors := cloneAncestors(ancestors)
			nextAncestors[canonicalLinkPath(physicalPath)] = struct{}{}
			if err := c.walkDirectory(ctx, spec, physicalPath, logicalPath, nextAncestors, scanner, result); err != nil {
				return err
			}
			continue
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		info, err := entry.Info()
		if err != nil {
			result.Skipped = append(result.Skipped, issue(spec, logicalPath, "stat-failed", err))
			continue
		}
		if err := c.collectFile(ctx, spec, physicalPath, logicalPath, info, scanner, result); err != nil {
			return err
		}
	}
	return nil
}

func (c *Collector) skipDirectoryPath(
	spec resource.Spec,
	logicalPath string,
	scanner *secret.Scanner,
	result *Result,
) bool {
	if scanner.IsExcluded(logicalPath) ||
		scanner.IsExcluded(path.Join(logicalPath, ".acsync-entry")) {
		return true
	}
	if allowed, code := c.options.Filter.CheckDirectory(logicalPath); !allowed {
		result.Skipped = append(result.Skipped, resource.Issue{
			ResourceKey: spec.Key,
			Path:        logicalPath,
			Code:        code,
			Message:     "directory excluded by portable resource policy",
		})
		return true
	}
	return false
}

func (c *Collector) walkLink(
	ctx context.Context,
	spec resource.Spec,
	linkPath string,
	logicalPath string,
	ancestors map[string]struct{},
	scanner *secret.Scanner,
	result *Result,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	target, err := filepath.EvalSymlinks(linkPath)
	if err != nil {
		result.Skipped = append(result.Skipped, issue(spec, logicalPath, "link-unavailable", err))
		return nil
	}
	target, err = filepath.Abs(target)
	if err != nil {
		result.Skipped = append(result.Skipped, issue(spec, logicalPath, "link-unavailable", err))
		return nil
	}
	target = filepath.Clean(target)
	if !pathWithin(spec.Root, target) &&
		(c.options.ApprovedLinks == nil || !c.options.ApprovedLinks.IsApproved(target)) {
		result.Skipped = append(result.Skipped, resource.Issue{
			ResourceKey: spec.Key,
			Path:        logicalPath,
			Code:        "unapproved-link",
			Message:     "symbolic link resolves outside the approved resource root",
		})
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	info, err := os.Stat(target)
	if err != nil {
		result.Skipped = append(result.Skipped, issue(spec, logicalPath, "link-unavailable", err))
		return nil
	}
	if !info.IsDir() {
		return c.collectFile(ctx, spec, target, logicalPath, info, scanner, result)
	}
	canonical := canonicalLinkPath(target)
	if _, exists := ancestors[canonical]; exists {
		result.Skipped = append(result.Skipped, resource.Issue{
			ResourceKey: spec.Key,
			Path:        logicalPath,
			Code:        "link-cycle",
			Message:     "symbolic link creates a directory cycle",
		})
		return nil
	}
	nextAncestors := cloneAncestors(ancestors)
	nextAncestors[canonical] = struct{}{}
	return c.walkDirectory(ctx, spec, target, logicalPath, nextAncestors, scanner, result)
}

func (c *Collector) collectFile(
	ctx context.Context,
	spec resource.Spec,
	physicalPath string,
	rel string,
	info os.FileInfo,
	scanner *secret.Scanner,
	result *Result,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !matchesAny(spec.Include, rel) || scanner.IsExcluded(rel) {
		return nil
	}
	if allowed, code := c.options.Filter.Check(rel, info.Size(), spec.Strategy); !allowed {
		result.Skipped = append(result.Skipped, resource.Issue{
			ResourceKey: spec.Key,
			Path:        rel,
			Code:        code,
			Message:     "file excluded by portable resource policy",
			Bytes:       info.Size(),
		})
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	data, err := os.ReadFile(physicalPath)
	if err != nil {
		result.Skipped = append(result.Skipped, issue(spec, rel, "read-failed", err))
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if spec.Transformer != "" {
		if c.options.Projector == nil {
			result.Skipped = append(result.Skipped, resource.Issue{
				ResourceKey: spec.Key,
				Path:        rel,
				Code:        "projector-unavailable",
				Message:     "resource requires a configuration projector",
			})
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		data, err = c.options.Projector.Project(spec.Transformer, rel, c.options.GOOS, c.options.UserHome, data)
		if err != nil {
			result.Blocked = append(result.Blocked, issue(spec, rel, "projection-failed", err))
			return nil
		}
		if err := ctx.Err(); err != nil {
			return err
		}
	}
	if allowed, code := c.options.Filter.Check(rel, int64(len(data)), spec.Strategy); !allowed {
		result.Skipped = append(result.Skipped, resource.Issue{
			ResourceKey: spec.Key,
			Path:        rel,
			Code:        code,
			Message:     "projected artifact excluded by portable resource policy",
			Bytes:       int64(len(data)),
		})
		return nil
	}
	blocked, err := scanner.Scan(rel, data)
	if err != nil {
		result.Blocked = append(result.Blocked, issue(spec, rel, "scan-failed", err))
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if blocked {
		result.Blocked = append(result.Blocked, blockedIssue(spec, rel, int64(len(data))))
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return c.stage(ctx, spec, rel, data, info.ModTime().Unix(), result)
}

func (c *Collector) stage(
	ctx context.Context,
	spec resource.Spec,
	rel string,
	data []byte,
	modTime int64,
	result *Result,
) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	repoRel, err := spec.RepoPath(rel)
	if err != nil {
		result.Skipped = append(result.Skipped, issue(spec, rel, "repo-path-invalid", err))
		return nil
	}
	stagePath := filepath.Join(result.StageRoot, filepath.FromSlash(repoRel))
	if err := os.MkdirAll(filepath.Dir(stagePath), 0o755); err != nil {
		return fmt.Errorf("create stage directory: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.WriteFile(stagePath, data, 0o600); err != nil {
		return fmt.Errorf("write stage artifact: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	sum := sha256.Sum256(data)
	if err := ctx.Err(); err != nil {
		return err
	}
	hash := hex.EncodeToString(sum[:])
	size := int64(len(data))
	result.Snapshot[repoRel] = state.FileMeta{Hash: hash, ModTime: modTime, Size: size}
	result.Artifacts[repoRel] = Artifact{
		RepoRel:     repoRel,
		ResourceKey: spec.Key,
		Relative:    rel,
		StagePath:   stagePath,
		Targets:     append([]string(nil), spec.Targets...),
		Hash:        hash,
		Size:        size,
		ModTime:     modTime,
	}
	return nil
}

func matchesAny(patterns []string, rel string) bool {
	for _, pattern := range patterns {
		matched, err := doublestar.Match(strings.ReplaceAll(pattern, `\`, "/"), rel)
		if err == nil && matched {
			return true
		}
	}
	return false
}

func appendUniquePaths(values []string, additions ...string) []string {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		seen[filepath.Clean(value)] = struct{}{}
	}
	for _, value := range additions {
		cleaned := filepath.Clean(value)
		if _, ok := seen[cleaned]; ok {
			continue
		}
		values = append(values, value)
		seen[cleaned] = struct{}{}
	}
	return values
}

func pathWithin(root, candidate string) bool {
	relative, err := filepath.Rel(root, candidate)
	if err != nil {
		return false
	}
	return relative != ".." &&
		!strings.HasPrefix(relative, ".."+string(filepath.Separator)) &&
		!filepath.IsAbs(relative)
}

func canonicalLinkPath(value string) string {
	value = filepath.Clean(value)
	if runtime.GOOS == "windows" {
		return strings.ToLower(value)
	}
	return value
}

func cloneAncestors(source map[string]struct{}) map[string]struct{} {
	clone := make(map[string]struct{}, len(source)+1)
	for key := range source {
		clone[key] = struct{}{}
	}
	return clone
}

func issue(spec resource.Spec, path, code string, err error) resource.Issue {
	return resource.Issue{
		ResourceKey: spec.Key,
		Path:        path,
		Code:        code,
		Message:     err.Error(),
	}
}

func blockedIssue(spec resource.Spec, rel string, size int64) resource.Issue {
	return resource.Issue{
		ResourceKey: spec.Key,
		Path:        rel,
		Code:        "secret-detected",
		Message:     "portable artifact contains secret-looking data",
		Bytes:       size,
	}
}
