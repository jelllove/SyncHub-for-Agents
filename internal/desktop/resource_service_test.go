package desktop

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/qinqingxu/acsync/internal/cli"
	"github.com/qinqingxu/acsync/internal/config"
	"github.com/qinqingxu/acsync/internal/conflict"
	"github.com/qinqingxu/acsync/internal/installplan"
	"github.com/qinqingxu/acsync/internal/provider"
	"github.com/qinqingxu/acsync/internal/resource"
)

func configuredResourceService(t *testing.T) *Service {
	t.Helper()
	home := configuredHome(t)
	plans := installplan.NewStore(filepath.Join(home, "install"))
	if err := plans.SavePending(installplan.Plan{
		ID: "plan-1",
		Operations: []installplan.Operation{{
			ID: "copilot:wiqd@wiqd", Adapter: "copilot-plugin",
			Source: "wiqd@wiqd", Kind: "install", Executable: "copilot",
			Args: []string{"plugin", "install", "wiqd@wiqd"},
		}},
	}); err != nil {
		t.Fatal(err)
	}
	conflicts := conflict.NewStore(
		filepath.Join(home, "conflicts"),
		filepath.Join(home, "repo"),
		nil,
	)
	if err := conflicts.Create(conflict.Record{
		ID: "conflict-1", ResourceKey: "claude/settings",
		RepoRel:   "agents/_portable/config/providers/claude/config/settings/settings.json",
		CreatedAt: time.Unix(1000, 0),
	}, []byte(`{"theme":"dark"}`), []byte(`{"theme":"light"}`), []byte(`{"theme":"system"}`)); err != nil {
		t.Fatal(err)
	}
	service, err := New(home, runtime.GOOS)
	if err != nil {
		t.Fatal(err)
	}
	return service
}

func findAgent(agents []Agent, name string) Agent {
	for _, agent := range agents {
		if agent.Name == name {
			return agent
		}
	}
	return Agent{}
}

func TestSnapshotIncludesResourceCategoriesAndPendingWork(t *testing.T) {
	service := configuredResourceService(t)
	defer service.Close()

	got, err := service.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	claude := findAgent(got.Agents, "claude")
	if len(claude.Resources) == 0 {
		t.Fatal("Claude resources missing")
	}
	if got.PendingInstallPlan == nil || len(got.Conflicts) != 1 {
		t.Fatalf("pending state = %#v", got)
	}
	if got.Conflicts[0].Revision == "" {
		t.Fatal("conflict revision is missing")
	}
	if got.Platform != runtime.GOOS {
		t.Fatalf("platform = %q", got.Platform)
	}
}

func TestSnapshotIncludesPendingConflictResolutionStatus(t *testing.T) {
	service := configuredResourceService(t)
	defer service.Close()

	store := conflictStore(service.home, nil)
	visible, _, err := store.VisibleConflicts()
	if err != nil {
		t.Fatal(err)
	}
	batch, err := store.QueueBatch([]conflict.ResolutionSelection{{
		ID: visible[0].Record.ID, Revision: visible[0].Revision,
		Choice: conflict.ChoiceLocal,
	}})
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := service.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.ConflictResolution == nil ||
		snapshot.ConflictResolution.ID != batch.ID ||
		snapshot.ConflictResolution.Status != "queued" ||
		snapshot.ConflictResolution.Selected != 1 {
		t.Fatalf("conflict resolution = %#v", snapshot.ConflictResolution)
	}
}

func TestSnapshotIncludesFailedConflictResolutionStatus(t *testing.T) {
	service := configuredResourceService(t)
	defer service.Close()

	metadata := filepath.Join(service.home, "conflict-resolution")
	if err := os.MkdirAll(metadata, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(metadata, "failed.json"), []byte(`{
			"id": "failed-batch",
			"status": "failed",
			"error": "stale revision",
			"selections": [{"id": "conflict-1", "revision": "old", "choice": "local"}]
		}`), 0o600); err != nil {
		t.Fatal(err)
	}
	snapshot, err := service.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.ConflictResolution == nil ||
		snapshot.ConflictResolution.ID != "failed-batch" ||
		snapshot.ConflictResolution.Status != "failed" ||
		snapshot.ConflictResolution.Selected != 1 ||
		snapshot.ConflictResolution.Error != "stale revision" {
		t.Fatalf("conflict resolution = %#v", snapshot.ConflictResolution)
	}
}

func TestSnapshotPropagatesMalformedConflictResolutionMetadata(t *testing.T) {
	service := configuredResourceService(t)
	defer service.Close()

	metadata := filepath.Join(service.home, "conflict-resolution")
	if err := os.MkdirAll(metadata, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(metadata, "pending.json"), []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := service.Snapshot(); err == nil {
		t.Fatal("malformed conflict resolution metadata was ignored")
	}
}

func TestSaveSettingsPersistsCategoriesAndCustomResources(t *testing.T) {
	home := configuredHome(t)
	service, err := New(home, runtime.GOOS)
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	input := SettingsInput{
		RepositoryURL:   "git@github.com:owner/repo.git",
		IntervalMinutes: 15,
		TrashGraceDays:  45,
		Agents:          map[string]bool{"claude": true},
		Categories: map[string]map[string]bool{
			"claude": {"skills": false},
		},
		CustomResources: []CustomResourceInput{{
			ID: "notes", Category: "instructions",
			Paths:   map[string]string{runtime.GOOS: filepath.Join(t.TempDir(), "notes")},
			Targets: map[string]string{runtime.GOOS: filepath.Join(t.TempDir(), "restored")},
			Include: []string{"**/*.md"}, Strategy: "text-tree",
		}},
	}
	if err := service.SaveSettings(input); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(cli.ConfigPath(home))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.CategoryEnabled("claude", resource.CategorySkills) {
		t.Fatal("skills category was not disabled")
	}
	if len(cfg.CustomResources) != 1 || cfg.CustomResources[0].ID != "notes" {
		t.Fatalf("custom resources = %#v", cfg.CustomResources)
	}
	snapshot, err := service.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshot.CustomResources) != 1 ||
		snapshot.CustomResources[0].Paths[runtime.GOOS] != input.CustomResources[0].Paths[runtime.GOOS] {
		t.Fatalf("snapshot custom resources = %#v", snapshot.CustomResources)
	}
}

func TestSaveSettingsClearsPreviewWithoutCollecting(t *testing.T) {
	home := configuredHome(t)
	oldPreview := ResourcePreview{
		GeneratedAt: time.Date(2026, 8, 18, 9, 0, 0, 0, time.UTC),
		Files:       4,
	}
	writeDesktopPreview(t, home, oldPreview)
	service, err := New(home, runtime.GOOS)
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	var collectionCalls atomic.Int32
	service.previewCollector = func(
		context.Context,
		config.Config,
		[]provider.Provider,
	) (ResourcePreview, error) {
		collectionCalls.Add(1)
		return ResourcePreview{}, nil
	}

	if err := service.SaveSettings(SettingsInput{
		RepositoryURL:   "git@github.com:owner/repo.git",
		IntervalMinutes: 15,
		TrashGraceDays:  45,
		Agents:          map[string]bool{"claude": true},
	}); err != nil {
		t.Fatal(err)
	}
	preview, err := newSummaryStore(home).loadPreview()
	if err != nil {
		t.Fatal(err)
	}
	if !preview.GeneratedAt.IsZero() || preview.Files != 0 || len(preview.Resources) != 0 {
		t.Fatalf("preview after settings save = %#v, want empty", preview)
	}
	if collectionCalls.Load() != 0 {
		t.Fatalf("settings save collected preview %d times", collectionCalls.Load())
	}
}

func TestSaveSettingsRejectsInFlightPreviewFromPreviousConfig(t *testing.T) {
	home := configuredHome(t)
	writeDesktopPreview(t, home, ResourcePreview{
		GeneratedAt: time.Date(2026, 8, 18, 9, 0, 0, 0, time.UTC),
		Files:       4,
	})
	service, err := New(home, runtime.GOOS)
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()

	started := make(chan struct{})
	release := make(chan struct{})
	service.previewCollector = func(
		ctx context.Context,
		_ config.Config,
		_ []provider.Provider,
	) (ResourcePreview, error) {
		close(started)
		select {
		case <-release:
			return ResourcePreview{Files: 9}, nil
		case <-ctx.Done():
			return ResourcePreview{}, ctx.Err()
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	previewDone := make(chan error, 1)
	go func() {
		_, err := service.ResourcePreview(ctx)
		previewDone <- err
	}()
	<-started

	if err := service.SaveSettings(SettingsInput{
		RepositoryURL:   "git@github.com:owner/changed.git",
		IntervalMinutes: 15,
		TrashGraceDays:  45,
		Agents:          map[string]bool{"claude": true},
	}); err != nil {
		t.Fatal(err)
	}
	close(release)
	if err := <-previewDone; err == nil {
		t.Fatal("in-flight preview from previous settings was accepted")
	}

	preview, err := newSummaryStore(home).loadPreview()
	if err != nil {
		t.Fatal(err)
	}
	if !preview.GeneratedAt.IsZero() || preview.Files != 0 {
		t.Fatalf("preview after stale collection = %#v, want empty", preview)
	}
}

func TestRejectedSettingsKeepPersistedPreview(t *testing.T) {
	home := configuredHome(t)
	oldPreview := ResourcePreview{
		GeneratedAt: time.Date(2026, 8, 18, 9, 0, 0, 0, time.UTC),
		Files:       4,
	}
	writeDesktopPreview(t, home, oldPreview)
	service, err := New(home, runtime.GOOS)
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()

	err = service.SaveSettings(SettingsInput{
		RepositoryURL:   "git@github.com:owner/repo.git",
		IntervalMinutes: 0,
		TrashGraceDays:  45,
		Agents:          map[string]bool{"claude": true},
	})
	if err == nil {
		t.Fatal("invalid settings were accepted")
	}
	preview, loadErr := newSummaryStore(home).loadPreview()
	if loadErr != nil {
		t.Fatal(loadErr)
	}
	if !preview.GeneratedAt.Equal(oldPreview.GeneratedAt) || preview.Files != oldPreview.Files {
		t.Fatalf("preview after rejected settings = %#v", preview)
	}
}

func TestApproveInstallPlanRequiresCurrentID(t *testing.T) {
	service := configuredResourceService(t)
	defer service.Close()
	if err := service.ApproveInstallPlan("other"); err == nil {
		t.Fatal("wrong pending plan ID was accepted")
	}
	if err := service.ApproveInstallPlan("plan-1"); err != nil {
		t.Fatal(err)
	}
	store := installplan.NewStore(filepath.Join(service.home, "install"))
	pending, err := store.Pending()
	if err != nil {
		t.Fatal(err)
	}
	if pending == nil || !store.IsApproved(pending.Operations[0]) {
		t.Fatalf("approval was not saved: %#v", pending)
	}
}

func TestRetryInstallPlanRequiresFailedPendingOperations(t *testing.T) {
	service := configuredResourceService(t)
	defer service.Close()
	if err := service.Pause(); err != nil {
		t.Fatal(err)
	}
	store := installplan.NewStore(filepath.Join(service.home, "install"))
	pending, err := store.Pending()
	if err != nil {
		t.Fatal(err)
	}
	if pending == nil {
		t.Fatal("pending install plan is missing")
	}
	if err := service.RetryInstallPlan(pending.ID); err == nil {
		t.Fatal("retry was accepted without failed operations")
	}

	pending.Errors = map[string]string{pending.Operations[0].ID: "access is denied"}
	if err := store.SavePending(*pending); err != nil {
		t.Fatal(err)
	}
	if err := service.RetryInstallPlan("other"); err == nil {
		t.Fatal("retry accepted a mismatched plan ID")
	}
	if err := service.RetryInstallPlan(pending.ID); err != nil {
		t.Fatal(err)
	}
}

func TestQueueConflictBatchValidatesAndQueues(t *testing.T) {
	service := configuredResourceService(t)
	defer service.Close()
	if err := service.Pause(); err != nil {
		t.Fatal(err)
	}
	snapshot, err := service.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	entry := snapshot.Conflicts[0]
	if err := service.QueueConflictBatch([]ConflictSelection{{
		ID: entry.ID, Revision: entry.Revision, Choice: "invalid",
	}}); err == nil {
		t.Fatal("invalid queue choice was accepted")
	}
	if err := service.QueueConflictBatch([]ConflictSelection{{
		ID: entry.ID, Revision: entry.Revision, Choice: "local",
	}}); err != nil {
		t.Fatal(err)
	}
	pending, err := conflictStore(service.home, nil).PendingBatch()
	if err != nil {
		t.Fatal(err)
	}
	if pending == nil || pending.Status != "queued" || len(pending.Selections) != 1 {
		t.Fatalf("pending conflict batch = %#v", pending)
	}
}

func TestRetryConflictBatchRequeuesFailedSelectionSet(t *testing.T) {
	service := configuredResourceService(t)
	defer service.Close()
	if err := service.Pause(); err != nil {
		t.Fatal(err)
	}
	snapshot, err := service.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	entry := snapshot.Conflicts[0]
	store := conflictStore(service.home, nil)
	failed := conflict.ResolutionBatch{
		ID:     "failed-1",
		Status: "failed",
		Selections: []conflict.ResolutionSelection{{
			ID: entry.ID, Revision: entry.Revision, Choice: conflict.ChoiceLocal,
		}},
		Error: "transient error",
	}
	metadataRoot := filepath.Join(service.home, "conflict-resolution")
	if err := os.MkdirAll(metadataRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	data, err := json.MarshalIndent(failed, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(filepath.Join(metadataRoot, "failed.json"), data, 0o600); err != nil {
		t.Fatal(err)
	}

	if err := service.RetryConflictBatch("other"); err == nil {
		t.Fatal("mismatched failed batch ID was accepted")
	}
	if err := service.RetryConflictBatch("failed-1"); err != nil {
		t.Fatal(err)
	}
	pending, err := store.PendingBatch()
	if err != nil {
		t.Fatal(err)
	}
	if pending == nil || pending.Status != "queued" || len(pending.Selections) != 1 {
		t.Fatalf("pending conflict retry batch = %#v", pending)
	}
	failedAfterRetry, err := store.FailedBatch()
	if err != nil {
		t.Fatal(err)
	}
	if failedAfterRetry != nil {
		t.Fatalf("failed batch remained after retry: %#v", failedAfterRetry)
	}
}

func TestResolveConflictValidatesChoiceAndID(t *testing.T) {
	service := configuredResourceService(t)
	defer service.Close()
	if err := service.ResolveConflict(ConflictResolution{
		ID: "missing", Choice: "local",
	}); err == nil {
		t.Fatal("missing conflict was accepted")
	}
	if err := service.ResolveConflict(ConflictResolution{
		ID: "conflict-1", Choice: "invalid",
	}); err == nil {
		t.Fatal("invalid conflict choice was accepted")
	}
	if err := service.ResolveConflict(ConflictResolution{
		ID: "conflict-1", Choice: "merged", Content: `{"theme":"merged"}`,
	}); err != nil {
		t.Fatal(err)
	}
	canonical := filepath.Join(
		service.home,
		"repo",
		"agents",
		"_portable",
		"config",
		"providers",
		"claude",
		"config",
		"settings",
		"settings.json",
	)
	if data, err := os.ReadFile(canonical); err != nil || string(data) != `{"theme":"merged"}` {
		t.Fatalf("resolved content = %q, %v", data, err)
	}
}

func TestPreviewCustomResourceDoesNotSaveCandidate(t *testing.T) {
	home := configuredHome(t)
	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "README.md"), []byte("portable"), 0o644); err != nil {
		t.Fatal(err)
	}

	service, err := New(home, runtime.GOOS)
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	preview, err := service.PreviewCustomResource(context.Background(), CustomResourceInput{
		ID: "notes", Category: "instructions",
		Paths:   map[string]string{runtime.GOOS: source},
		Targets: map[string]string{runtime.GOOS: t.TempDir()},
		Include: []string{"**"}, Strategy: "text-tree",
	})
	if err != nil {
		t.Fatal(err)
	}
	if preview.Files != 1 || preview.Bytes == 0 {
		t.Fatalf("preview = %#v", preview)
	}
	cfg, err := config.Load(cli.ConfigPath(home))
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.CustomResources) != 0 {
		t.Fatalf("preview saved candidate: %#v", cfg.CustomResources)
	}
}

func TestPreviewCustomResourceForwardsCancellation(t *testing.T) {
	home := configuredHome(t)
	service, err := New(home, runtime.GOOS)
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	started := make(chan struct{})
	service.previewCollector = func(
		ctx context.Context,
		_ config.Config,
		_ []provider.Provider,
	) (ResourcePreview, error) {
		close(started)
		<-ctx.Done()
		return ResourcePreview{}, ctx.Err()
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	source := t.TempDir()
	target := t.TempDir()
	go func() {
		_, err := service.PreviewCustomResource(ctx, CustomResourceInput{
			ID:       "notes",
			Category: "instructions",
			Paths:    map[string]string{runtime.GOOS: source},
			Targets:  map[string]string{runtime.GOOS: target},
			Include:  []string{"**"},
			Strategy: "text-tree",
		})
		done <- err
	}()

	<-started
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("PreviewCustomResource() error = %v, want context canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("custom preview did not receive cancellation")
	}
}

func TestResourcePreviewPersistsSuccessfulSummary(t *testing.T) {
	home := configuredHome(t)
	service, err := New(home, runtime.GOOS)
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	service.previewCollector = func(
		context.Context,
		config.Config,
		[]provider.Provider,
	) (ResourcePreview, error) {
		return ResourcePreview{Files: 7}, nil
	}

	before := time.Now().UTC()
	preview, err := service.ResourcePreview(context.Background())
	after := time.Now().UTC()
	if err != nil {
		t.Fatal(err)
	}
	if preview.Files != 7 ||
		preview.GeneratedAt.Before(before) ||
		preview.GeneratedAt.After(after) ||
		preview.GeneratedAt.Location() != time.UTC {
		t.Fatalf("generated preview = %#v", preview)
	}
	persisted, err := newSummaryStore(home).loadPreview()
	if err != nil {
		t.Fatal(err)
	}
	if !persisted.GeneratedAt.Equal(preview.GeneratedAt) || persisted.Files != 7 {
		t.Fatalf("persisted preview = %#v, want %#v", persisted, preview)
	}
}

func TestResourcePreviewCollectionErrorKeepsPreviousSummary(t *testing.T) {
	home := configuredHome(t)
	oldPreview := ResourcePreview{
		GeneratedAt: time.Date(2026, 8, 18, 9, 0, 0, 0, time.UTC),
		Files:       4,
	}
	writeDesktopPreview(t, home, oldPreview)
	service, err := New(home, runtime.GOOS)
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	collectionErr := errors.New("collection failed")
	service.previewCollector = func(
		context.Context,
		config.Config,
		[]provider.Provider,
	) (ResourcePreview, error) {
		return ResourcePreview{}, collectionErr
	}

	if _, err := service.ResourcePreview(context.Background()); !errors.Is(err, collectionErr) {
		t.Fatalf("ResourcePreview() error = %v, want collection failure", err)
	}
	persisted, err := newSummaryStore(home).loadPreview()
	if err != nil {
		t.Fatal(err)
	}
	if !persisted.GeneratedAt.Equal(oldPreview.GeneratedAt) || persisted.Files != oldPreview.Files {
		t.Fatalf("persisted preview after failure = %#v", persisted)
	}
}

func TestPreviewKeepsCountsForMultipleResourcesAndProviders(t *testing.T) {
	preview := ResourcePreview{Resources: []ResourceCategory{
		{Provider: "claude", ID: "settings", Category: "config", FileCount: 1},
		{Provider: "copilot", ID: "settings", Category: "config", FileCount: 2},
	}}
	indexed := previewByIdentity(preview)
	if got := indexed[resourceIdentity("claude", "settings", "config")].FileCount; got != 1 {
		t.Fatalf("Claude count = %d", got)
	}
	if got := indexed[resourceIdentity("copilot", "settings", "config")].FileCount; got != 2 {
		t.Fatalf("Copilot count = %d", got)
	}

	home := configuredHome(t)
	first := t.TempDir()
	second := t.TempDir()
	if err := os.WriteFile(filepath.Join(first, "first.md"), []byte("first"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(second, "second.md"), []byte("second"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(cli.ConfigPath(home))
	if err != nil {
		t.Fatal(err)
	}
	cfg.CustomResources = []config.CustomResource{
		{
			ID: "first", Category: resource.CategoryInstructions,
			Paths:   map[string]string{runtime.GOOS: first},
			Targets: map[string]string{runtime.GOOS: first},
			Include: []string{"**"}, Strategy: resource.StrategyTextTree,
		},
		{
			ID: "second", Category: resource.CategoryInstructions,
			Paths:   map[string]string{runtime.GOOS: second},
			Targets: map[string]string{runtime.GOOS: second},
			Include: []string{"**"}, Strategy: resource.StrategyTextTree,
		},
	}
	if err := config.Save(cli.ConfigPath(home), cfg); err != nil {
		t.Fatal(err)
	}
	service, err := New(home, runtime.GOOS)
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	got, err := service.ResourcePreview(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	indexed = previewByIdentity(got)
	for _, id := range []string{"first", "second"} {
		item := indexed[resourceIdentity("custom", id, "instructions")]
		if item.FileCount != 1 || item.Bytes == 0 {
			t.Fatalf("%s preview = %#v", id, item)
		}
	}
}
