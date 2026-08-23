package syncengine

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/qinqingxu/synchub-for-agents/internal/conflict"
	"github.com/qinqingxu/synchub-for-agents/internal/gitclient"
	"github.com/qinqingxu/synchub-for-agents/internal/installplan"
	"github.com/qinqingxu/synchub-for-agents/internal/portableconfig"
	"github.com/qinqingxu/synchub-for-agents/internal/portablemerge"
	"github.com/qinqingxu/synchub-for-agents/internal/resource"
	"github.com/qinqingxu/synchub-for-agents/internal/resourcecollect"
	"github.com/qinqingxu/synchub-for-agents/internal/state"
)

type Result struct {
	Actions         []Action
	Blocked         []string
	Issues          []resource.Issue
	Pushed          bool
	Restored        int
	Reinstalled     int
	Skipped         int
	Conflicts       int
	PendingInstalls int
	NeedsAttention  bool
}

type Engine struct {
	Git            *gitclient.Client
	RepoDir        string
	Home           string
	UserHome       string
	GOOS           string
	StatePath      string
	Resources      map[string]resource.Spec
	Codecs         *portableconfig.Registry
	Base           *state.BaseStore
	Conflicts      *conflict.Store
	Inventory      resourcecollect.InventoryProvider
	InstallManager *installplan.Manager
	ApprovedLinks  resourcecollect.ApprovedLinkStore
	TextMerger     portablemerge.TextMerger
	PushRetries    int
	Now            func() time.Time
	OnProgress     func(Progress)
	FirstSyncMode  string
	FirstSyncRun   bool
}

func (e *Engine) SyncOnce() (result Result, retErr error) {
	attempts := e.PushRetries
	if attempts <= 0 {
		attempts = 3
	}
	return e.syncOnce(attempts)
}

func (e *Engine) syncOnce(attempts int) (result Result, retErr error) {
	if err := e.validate(); err != nil {
		return Result{}, err
	}
	if err := e.Git.SetLocalConfig("core.autocrlf", "false"); err != nil {
		return Result{}, fmt.Errorf("configure repository line endings: %w", err)
	}
	now := e.Now
	if now == nil {
		now = time.Now
	}
	codecs := e.Codecs
	if codecs == nil {
		codecs = portableconfig.NewRegistry()
	}
	baseStore := e.Base
	if baseStore == nil {
		baseStore = state.NewBaseStore(e.Home)
	}
	conflicts := e.Conflicts
	if conflicts == nil {
		conflicts = conflict.NewStore(
			filepath.Join(e.Home, "conflicts"),
			e.RepoDir,
			ConflictScanner(e.Resources),
		)
	}
	textMerger := e.TextMerger
	if textMerger == nil {
		textMerger = portablemerge.GitTextMerger{
			TempParent: filepath.Join(e.RepoDir, ".git", "synchub-stage"),
		}
	}
	if err := conflicts.RecoverTransactions(); err != nil {
		return Result{}, fmt.Errorf("recover conflict transaction: %w", err)
	}

	e.publish(Progress{
		Stage:      StagePulling,
		Label:      "Pulling latest repository changes",
		Percentage: 10,
	})
	if err := e.Git.PullRebase(); err != nil {
		return Result{}, fmt.Errorf("pull: %w", err)
	}

	stageParent := filepath.Join(e.RepoDir, ".git", "synchub-stage")
	if err := os.MkdirAll(stageParent, 0o700); err != nil {
		return Result{}, fmt.Errorf("create resource stage parent: %w", err)
	}
	e.publish(Progress{
		Stage:      StageScanning,
		Label:      "Scanning portable resources",
		Percentage: 30,
	})
	collector := resourcecollect.New(resourcecollect.Options{
		StageParent:   stageParent,
		GOOS:          e.GOOS,
		UserHome:      e.UserHome,
		Projector:     codecs,
		Inventory:     e.Inventory,
		ApprovedLinks: e.ApprovedLinks,
	})
	collected, err := collector.Collect(sortedResourceSpecs(e.Resources))
	if err != nil {
		return Result{}, fmt.Errorf("collect resources: %w", err)
	}
	defer func() {
		retErr = errors.Join(retErr, collected.Close())
	}()
	result.Issues = append(result.Issues, collected.Blocked...)
	result.Issues = append(result.Issues, collected.Skipped...)
	result.Skipped = len(collected.Skipped)

	baseSnapshot, err := state.Load(e.StatePath)
	if err != nil {
		return Result{}, fmt.Errorf("load state: %w", err)
	}
	remoteSnapshot, err := SnapshotRepo(e.RepoDir)
	if err != nil {
		return Result{}, fmt.Errorf("snapshot repo: %w", err)
	}
	remoteOwned, _, ownershipBlocked := SplitRemoteSnapshot(remoteSnapshot, e.Resources)
	result.Issues = append(result.Issues, ownershipBlocked...)
	validRemote, validationBlocked := ValidateRemoteResources(
		e.RepoDir,
		remoteOwned,
		e.Resources,
		codecs,
		e.GOOS,
		e.UserHome,
	)
	result.Issues = append(result.Issues, validationBlocked...)
	if err := conflicts.MirrorFromRepo(
		conflictIDsForIssues(append(ownershipBlocked, validationBlocked...)),
	); err != nil {
		return Result{}, fmt.Errorf("mirror conflicts: %w", err)
	}
	if pending, err := conflicts.PendingBatch(); err != nil {
		return Result{}, fmt.Errorf("load pending conflict batch: %w", err)
	} else if pending != nil && pending.Status == "queued" {
		if err := conflicts.ApplyPendingBatch(); err != nil {
			return Result{}, fmt.Errorf("apply pending conflict batch: %w", err)
		}
	}

	e.publish(Progress{
		Stage:        StageComparing,
		Label:        "Comparing local and remote resources",
		Percentage:   45,
		BlockedFiles: len(collected.Blocked) + len(ownershipBlocked) + len(validationBlocked),
		Skipped:      result.Skipped,
	})
	local := cloneSnapshot(collected.Snapshot)
	baseOwned, _, _ := SplitRemoteSnapshot(baseSnapshot, e.Resources)
	for repoRel, meta := range validRemote {
		if isInternalPortablePath(repoRel) {
			local[repoRel] = meta
		}
	}
	skipPrefixes := skippedRepoPrefixes(collected.Skipped, e.Resources)
	local = withoutPrefixes(local, skipPrefixes)
	baseOwned = withoutPrefixes(baseOwned, skipPrefixes)
	validRemote = withoutPrefixes(validRemote, skipPrefixes)

	blockedPaths := blockedRepoPaths(
		append(append(append(
			[]resource.Issue{},
			collected.Blocked...),
			ownershipBlocked...),
			validationBlocked...),
		e.Resources,
	)
	for _, repoRel := range blockedPaths {
		if meta, ok := remoteSnapshot[repoRel]; ok {
			validRemote[repoRel] = meta
		}
	}
	result.Blocked = append(result.Blocked, blockedPaths...)
	actions := ReconcileWithBlocked(baseOwned, local, validRemote, blockedPaths)
	if e.FirstSyncRun {
		actions = e.applyFirstSyncStrategy(actions, local, validRemote)
	}
	result.Actions = actions

	applier := &ResourceApplier{
		RepoDir:  e.RepoDir,
		Home:     e.Home,
		GOOS:     e.GOOS,
		UserHome: e.UserHome,
		Codecs:   codecs,
		Base:     baseStore,
		Now:      now,
	}
	existingConflictRecords, err := conflicts.List()
	if err != nil {
		return Result{}, fmt.Errorf("list conflicts: %w", err)
	}
	existingConflictPaths := make(map[string]struct{}, len(existingConflictRecords))
	for _, record := range existingConflictRecords {
		existingConflictPaths[record.RepoRel] = struct{}{}
	}
	var pendingInstallPlan *installplan.Plan
	if e.InstallManager != nil {
		pendingInstallPlan, err = e.InstallManager.Store.Pending()
		if err != nil {
			return Result{}, fmt.Errorf("load pending install plan: %w", err)
		}
	}
	e.publish(Progress{
		Stage:        StageApplying,
		Label:        "Applying synchronized resources",
		Percentage:   60,
		TotalActions: len(actions),
		BlockedFiles: len(result.Blocked),
		Skipped:      result.Skipped,
	})
	preserve := map[string]struct{}{}
	result.Conflicts = len(existingConflictRecords)
	for _, record := range existingConflictRecords {
		preserve[record.RepoRel] = struct{}{}
	}
	for _, prefix := range skipPrefixes {
		for repoRel := range baseOwned {
			if pathHasPrefix(repoRel, prefix) {
				preserve[repoRel] = struct{}{}
			}
		}
		for repoRel := range remoteOwned {
			if pathHasPrefix(repoRel, prefix) {
				preserve[repoRel] = struct{}{}
			}
		}
	}
	for index, action := range actions {
		if _, unresolved := existingConflictPaths[action.RepoRel]; unresolved &&
			action.Type != MergeBoth &&
			action.Type != RemoveRemote {
			preserve[action.RepoRel] = struct{}{}
			continue
		}
		spec, specErr := specForRepoPath(e.Resources, action.RepoRel)
		if specErr != nil && action.Type != RemoveRemote {
			return Result{}, specErr
		}
		if action.Type == DeleteRemote &&
			spec.Strategy == resource.StrategyInstallManifest &&
			pendingInstallForAdapter(pendingInstallPlan, spec.Installer) {
			preserve[action.RepoRel] = struct{}{}
			continue
		}
		switch action.Type {
		case MergeBoth:
			base, baseExists, baseErr := baseStore.Get(action.RepoRel)
			if baseErr != nil {
				return Result{}, fmt.Errorf("load merge base %q: %w", action.RepoRel, baseErr)
			}
			conflicted, mergeErr := e.mergeResource(
				action.RepoRel,
				spec,
				base,
				baseExists,
				collected.Artifacts,
				conflicts,
				textMerger,
				applier,
				now(),
			)
			if mergeErr != nil {
				result.Issues = append(
					result.Issues,
					applyIssue(spec.Key, action.RepoRel, "merge-failed", mergeErr),
				)
				preserve[action.RepoRel] = struct{}{}
				continue
			}
			if conflicted {
				result.Conflicts++
				preserve[action.RepoRel] = struct{}{}
			} else {
				result.Restored++
			}
		case PushToRemote:
			artifact, ok := collected.Artifacts[action.RepoRel]
			if !ok {
				return Result{}, fmt.Errorf("no staged artifact for %q", action.RepoRel)
			}
			if err := applier.PushArtifact(artifact); err != nil {
				return Result{}, fmt.Errorf("push resource %q: %w", action.RepoRel, err)
			}
		case PullToLocal:
			if spec.Strategy == resource.StrategyInstallManifest {
				break
			}
			if err := applier.Restore(spec, action.RepoRel); err != nil {
				result.Issues = append(result.Issues, applyIssue(spec.Key, action.RepoRel, "restore-failed", err))
				preserve[action.RepoRel] = struct{}{}
				continue
			}
			result.Restored++
		case DeleteRemote:
			if err := applier.SoftDeleteRemote(action.RepoRel); err != nil {
				return Result{}, fmt.Errorf("delete remote resource %q: %w", action.RepoRel, err)
			}
			if spec.Strategy != resource.StrategyInstallManifest {
				if err := applier.Delete(spec, action.RepoRel); err != nil {
					result.Issues = append(
						result.Issues,
						applyIssue(spec.Key, action.RepoRel, "delete-local-alias-failed", err),
					)
					preserve[action.RepoRel] = struct{}{}
					continue
				}
			}
		case DeleteLocal:
			if spec.Strategy == resource.StrategyInstallManifest {
				break
			}
			if err := applier.Delete(spec, action.RepoRel); err != nil {
				result.Issues = append(result.Issues, applyIssue(spec.Key, action.RepoRel, "delete-local-failed", err))
				preserve[action.RepoRel] = struct{}{}
				continue
			}
		case RemoveRemote:
			filename, err := safeDeletableJoin(e.RepoDir, action.RepoRel)
			if err != nil {
				return Result{}, err
			}
			if err := os.Remove(filename); err != nil && !errors.Is(err, os.ErrNotExist) {
				return Result{}, fmt.Errorf("remove blocked remote resource %q: %w", action.RepoRel, err)
			}
		default:
			return Result{}, fmt.Errorf("unknown resource action %v", action.Type)
		}
		e.publish(Progress{
			Stage:            StageApplying,
			Label:            "Applying synchronized resources",
			Percentage:       actionPercentage(index+1, len(actions)),
			CompletedActions: index + 1,
			TotalActions:     len(actions),
			BlockedFiles:     len(result.Blocked),
			Restored:         result.Restored,
			Reinstalled:      result.Reinstalled,
			Skipped:          result.Skipped,
			Conflicts:        result.Conflicts,
			PendingInstalls:  result.PendingInstalls,
			NeedsAttention:   len(result.Issues) > 0 || result.Conflicts > 0,
		})
	}
	conflictRecords, err := conflicts.List()
	if err != nil {
		return Result{}, fmt.Errorf("list updated conflicts: %w", err)
	}
	result.Conflicts = len(conflictRecords)
	for _, record := range conflictRecords {
		preserve[record.RepoRel] = struct{}{}
	}

	if e.InstallManager != nil {
		currentManifests, err := currentInstallManifests(collected.Artifacts, e.Resources)
		if err != nil {
			return Result{}, err
		}
		installResult, err := e.InstallManager.Reconcile(
			context.Background(),
			e.RepoDir,
			e.Resources,
			currentManifests,
		)
		if err != nil {
			return Result{}, fmt.Errorf("reconcile install plans: %w", err)
		}
		result.Reinstalled += installResult.Executed
		result.PendingInstalls += installResult.Pending
		for operationID, message := range installResult.Errors {
			result.Issues = append(result.Issues, resource.Issue{
				ResourceKey: operationID,
				Code:        "install-failed",
				Message:     message,
			})
		}
	}

	e.publish(Progress{
		Stage:            StageUploading,
		Label:            "Uploading synchronized changes",
		Percentage:       85,
		CompletedActions: len(actions),
		TotalActions:     len(actions),
		BlockedFiles:     len(result.Blocked),
		Restored:         result.Restored,
		Reinstalled:      result.Reinstalled,
		Skipped:          result.Skipped,
		Conflicts:        result.Conflicts,
		PendingInstalls:  result.PendingInstalls,
		NeedsAttention:   len(result.Issues) > 0 || result.Conflicts > 0,
	})
	if err := e.Git.AddAll(); err != nil {
		return Result{}, fmt.Errorf("git add: %w", err)
	}
	changed, err := e.Git.HasChanges()
	if err != nil {
		return Result{}, fmt.Errorf("git status: %w", err)
	}
	if changed {
		if err := e.Git.Commit("sync: reconcile agent files"); err != nil {
			return Result{}, fmt.Errorf("commit: %w", err)
		}
		if err := e.Git.Push(); err != nil {
			if attempts <= 1 {
				return Result{}, fmt.Errorf("push: %w", err)
			}
			if fetchErr := e.Git.FetchOrigin(); fetchErr != nil {
				return Result{}, errors.Join(
					fmt.Errorf("push: %w", err),
					fmt.Errorf("fetch after rejected push: %w", fetchErr),
				)
			}
			if resetErr := e.Git.ResetKeepUpstream(); resetErr != nil {
				return Result{}, errors.Join(
					fmt.Errorf("push: %w", err),
					fmt.Errorf("restore repository after rejected push: %w", resetErr),
				)
			}
			return e.syncOnce(attempts - 1)
		}
		result.Pushed = true
	}

	currentRepo, err := SnapshotRepo(e.RepoDir)
	if err != nil {
		return Result{}, fmt.Errorf("snapshot updated repo: %w", err)
	}
	currentOwned, _, currentBlocked := SplitRemoteSnapshot(currentRepo, e.Resources)
	if len(currentBlocked) != 0 {
		return Result{}, fmt.Errorf("updated repository contains malformed portable paths")
	}
	for repoRel := range currentOwned {
		if isInternalPortablePath(repoRel) {
			delete(currentOwned, repoRel)
		}
	}
	nextBase := preserveSnapshot(baseOwned, currentOwned, preserve)
	if err := state.Save(e.StatePath, nextBase); err != nil {
		return Result{}, fmt.Errorf("save state: %w", err)
	}
	if err := baseStore.CaptureRepoPreserving(e.RepoDir, currentOwned, preserve); err != nil {
		rollbackErr := state.Save(e.StatePath, baseSnapshot)
		return Result{}, errors.Join(fmt.Errorf("capture base: %w", err), rollbackErr)
	}

	result.NeedsAttention = len(result.Issues) > 0 || result.Conflicts > 0 || result.PendingInstalls > 0
	return result, nil
}

func (e *Engine) validate() error {
	if e.Git == nil {
		return fmt.Errorf("sync engine requires a git client")
	}
	if strings.TrimSpace(e.RepoDir) == "" {
		return fmt.Errorf("sync engine requires a repository directory")
	}
	if strings.TrimSpace(e.StatePath) == "" {
		return fmt.Errorf("sync engine requires a state path")
	}
	if strings.TrimSpace(e.Home) == "" {
		e.Home = filepath.Dir(e.StatePath)
	}
	if e.InstallManager != nil && e.InstallManager.Store == nil {
		return fmt.Errorf("sync engine install manager requires a store")
	}
	for _, spec := range e.Resources {
		if spec.Strategy != resource.StrategyInstallManifest {
			continue
		}
		if e.Inventory == nil {
			return fmt.Errorf("sync engine requires inventory for install resources")
		}
		if e.InstallManager == nil {
			return fmt.Errorf("sync engine requires an install manager for install resources")
		}
	}
	return nil
}

func (e *Engine) mergeResource(
	repoRel string,
	spec resource.Spec,
	base []byte,
	baseExists bool,
	artifacts map[string]resourcecollect.Artifact,
	conflicts *conflict.Store,
	textMerger portablemerge.TextMerger,
	applier *ResourceApplier,
	now time.Time,
) (bool, error) {
	artifact, localExists := artifacts[repoRel]
	var local []byte
	if localExists {
		var err error
		local, err = os.ReadFile(artifact.StagePath)
		if err != nil {
			return false, fmt.Errorf("read staged merge input %q: %w", repoRel, err)
		}
	}
	remotePath, err := safeJoin(e.RepoDir, repoRel)
	if err != nil {
		return false, err
	}
	remote, remoteErr := os.ReadFile(remotePath)
	remoteExists := remoteErr == nil
	if remoteErr != nil && !errors.Is(remoteErr, os.ErrNotExist) {
		return false, fmt.Errorf("read remote merge input %q: %w", repoRel, remoteErr)
	}

	var merged portablemerge.Result
	if !baseExists &&
		localExists &&
		remoteExists &&
		spec.Strategy == resource.StrategyStructuredMerge {
		ref, parseErr := resource.ParseRepoPath(repoRel)
		if parseErr != nil {
			return false, parseErr
		}
		format, _, parseErr := portableconfig.Parse(ref.Relative, remote)
		if parseErr != nil {
			return false, fmt.Errorf("parse initial structured remote %q: %w", repoRel, parseErr)
		}
		emptyBase, marshalErr := portableconfig.Marshal(format, map[string]any{})
		if marshalErr != nil {
			return false, fmt.Errorf("create initial structured base %q: %w", repoRel, marshalErr)
		}
		merged, err = portablemerge.StructuredDocument(ref.Relative, emptyBase, local, remote)
		if err != nil {
			return false, fmt.Errorf("merge resource %q: %w", repoRel, err)
		}
	} else if !baseExists || !localExists || !remoteExists {
		merged.Conflict = true
	} else {
		switch spec.Strategy {
		case resource.StrategyStructuredMerge:
			ref, parseErr := resource.ParseRepoPath(repoRel)
			if parseErr != nil {
				return false, parseErr
			}
			merged, err = portablemerge.StructuredDocument(ref.Relative, base, local, remote)
		case resource.StrategyTextTree:
			merged, err = textMerger.Merge(base, local, remote)
		default:
			merged = portablemerge.Binary(base, local, remote)
		}
		if err != nil {
			return false, fmt.Errorf("merge resource %q: %w", repoRel, err)
		}
	}
	if merged.Conflict {
		exists, err := conflictExists(conflicts, repoRel)
		if err != nil {
			return false, err
		}
		if exists {
			return true, nil
		}
		record := conflict.Record{
			ID:          conflictID(repoRel, base, local, remote),
			ResourceKey: spec.Key,
			RepoRel:     repoRel,
			CreatedAt:   now.UTC(),
		}
		if err := conflicts.Create(record, base, local, remote); err != nil {
			return false, fmt.Errorf("create conflict for %q: %w", repoRel, err)
		}
		return true, nil
	}
	if err := writeAtomic(remotePath, merged.Data, 0o600); err != nil {
		return false, err
	}
	if spec.Strategy == resource.StrategyInstallManifest {
		return false, nil
	}
	ref, err := resource.ParseRepoPath(repoRel)
	if err != nil {
		return false, err
	}
	if err := applier.RestoreBytes(spec, ref.Relative, merged.Data); err != nil {
		return false, fmt.Errorf("restore merged resource %q: %w", repoRel, err)
	}
	return false, nil
}

func pendingInstallForAdapter(plan *installplan.Plan, adapter string) bool {
	if plan == nil {
		return false
	}
	for _, operation := range plan.Operations {
		if operation.Adapter == adapter &&
			(operation.Kind == "install" || operation.Kind == "update") {
			return true
		}
	}
	return false
}

func (e *Engine) publish(progress Progress) {
	if e.OnProgress != nil {
		e.OnProgress(progress)
	}
}

func (e *Engine) applyFirstSyncStrategy(
	actions []Action,
	local state.Snapshot,
	remote state.Snapshot,
) []Action {
	switch e.FirstSyncMode {
	case "use-cloud":
		mapped := make([]Action, 0, len(actions))
		for _, action := range actions {
			switch action.Type {
			case PushToRemote:
				mapped = append(mapped, Action{Type: DeleteLocal, RepoRel: action.RepoRel})
			case MergeBoth:
				if _, exists := remote[action.RepoRel]; exists {
					mapped = append(mapped, Action{Type: PullToLocal, RepoRel: action.RepoRel})
				} else {
					mapped = append(mapped, Action{Type: DeleteLocal, RepoRel: action.RepoRel})
				}
			default:
				mapped = append(mapped, action)
			}
		}
		return mapped
	case "use-local":
		mapped := make([]Action, 0, len(actions))
		for _, action := range actions {
			switch action.Type {
			case PullToLocal:
				mapped = append(mapped, Action{Type: DeleteRemote, RepoRel: action.RepoRel})
			case MergeBoth:
				if _, exists := local[action.RepoRel]; exists {
					mapped = append(mapped, Action{Type: PushToRemote, RepoRel: action.RepoRel})
				} else {
					mapped = append(mapped, Action{Type: DeleteRemote, RepoRel: action.RepoRel})
				}
			default:
				mapped = append(mapped, action)
			}
		}
		return mapped
	default:
		return actions
	}
}

func sortedResourceSpecs(specs map[string]resource.Spec) []resource.Spec {
	keys := make([]string, 0, len(specs))
	for key := range specs {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	out := make([]resource.Spec, 0, len(keys))
	for _, key := range keys {
		out = append(out, specs[key])
	}
	return out
}

func currentInstallManifests(
	artifacts map[string]resourcecollect.Artifact,
	specs map[string]resource.Spec,
) (map[string][]byte, error) {
	manifests := map[string][]byte{}
	for repoRel, artifact := range artifacts {
		spec, err := specForRepoPath(specs, repoRel)
		if err != nil ||
			spec.Strategy != resource.StrategyInstallManifest ||
			artifact.Relative != "manifest.json" {
			continue
		}
		data, err := os.ReadFile(artifact.StagePath)
		if err != nil {
			return nil, fmt.Errorf("read staged install manifest %q: %w", repoRel, err)
		}
		manifests[spec.Key] = data
	}
	return manifests, nil
}

func cloneSnapshot(source state.Snapshot) state.Snapshot {
	out := make(state.Snapshot, len(source))
	for repoRel, meta := range source {
		out[repoRel] = meta
	}
	return out
}

func applyIssue(resourceKey, repoRel, code string, err error) resource.Issue {
	return resource.Issue{
		ResourceKey: resourceKey,
		Path:        repoRel,
		Code:        code,
		Message:     err.Error(),
	}
}

func actionPercentage(completed, total int) int {
	if total == 0 {
		return 75
	}
	return 60 + completed*15/total
}
