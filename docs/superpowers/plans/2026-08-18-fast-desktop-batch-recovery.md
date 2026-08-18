# Fast Desktop, Batch Conflict, and Plugin Recovery Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make routine desktop actions return in about one second, resolve selected conflicts with one queued synchronization, and require an explicit retry after a locked Plugin update.

**Architecture:** Split the desktop control plane from resource collection: `Snapshot()` reads only persisted summaries and small local stores, while resource preview runs on demand and coalesces concurrent callers. Conflict choices are preflighted into one local pending batch and applied transactionally by the sync engine after pull. Install command failures become structured, revoke approval, and are retried only against the exact pending plan revision.

**Tech Stack:** Go 1.26.5, Wails v3.0.0-beta.8, React 19, TypeScript 7, Vite 8, Vitest, Testing Library

**Design:** `docs/superpowers/specs/2026-08-18-batch-conflicts-and-sync-performance-design.md`

**Execution order:** Complete this plan before `docs/superpowers/plans/2026-08-18-indexed-sync-performance.md`. The second plan reuses the preview-summary and cycle-summary boundaries introduced here.

---

## File structure

### New files

- `internal/desktop/summary_store.go` — atomically persists the latest cycle and preview summaries.
- `internal/desktop/summary_store_test.go` — summary round-trip, corruption, and permissions tests.
- `internal/desktop/preview_coordinator.go` — single-flight on-demand preview collection.
- `internal/desktop/preview_coordinator_test.go` — cache-hit and concurrent-call tests.
- `internal/conflict/resolution.go` — revisions, pending batches, preflight, transaction, and recovery.
- `internal/conflict/resolution_test.go` — batch and rollback tests.
- `frontend/src/test/setup.ts` — DOM matcher setup.
- `frontend/src/resources/ConflictPanel.test.tsx` — staged-selection behavior.
- `frontend/src/resources/InstallPlanPanel.test.tsx` — approve/retry behavior.
- `frontend/src/SettingsPanel.tsx` — independently testable settings and preview surface.
- `frontend/src/SettingsPanel.test.tsx` — nonblocking settings and preview refresh tests.
- `frontend/src/desktopState.ts` — normalized non-null frontend snapshot model.

### Modified files

- `internal/desktop/models.go` — preview timestamp, conflict revision/batch models, structured install failure.
- `internal/desktop/service.go` — lightweight snapshots and persisted cycle state.
- `internal/desktop/resources.go` — preview coordination, batch queue API, exact-plan retry.
- `internal/desktop/lifecycle.go` — publish only lightweight snapshots.
- `internal/desktop/wails.go` — batch and retry bindings.
- `internal/desktop/*_test.go` — service, lifecycle, and Wails delegation coverage.
- `internal/scheduler/scheduler.go` — authoritative next-run tracking.
- `internal/scheduler/scheduler_test.go` — startup, interval, trigger, pause, and resume scheduling tests.
- `internal/conflict/store.go` — tombstone filtering and resolution helpers.
- `internal/syncengine/engine.go` — apply one queued batch after pull and conflict mirroring.
- `internal/syncengine/engine_test.go` — batch integration and one-cycle behavior.
- `internal/installplan/model.go` — structured `Failure`.
- `internal/installplan/store.go` — approval revocation.
- `internal/installplan/executor.go` — classify failures and revoke approval.
- `internal/installplan/*_test.go` — failure and manual-retry coverage.
- `frontend/package.json`, `frontend/package-lock.json`, `frontend/vite.config.ts` — frontend test runner.
- `frontend/src/App.tsx` — fast action refresh, batch wiring, extracted settings.
- `frontend/src/resources/ConflictPanel.tsx` — local selections and one Apply.
- `frontend/src/resources/InstallPlanPanel.tsx` — recovery text and Retry.
- `frontend/src/style.css` — selection toolbar, selected row, loading, and recovery styles.
- `frontend/bindings/github.com/qinqingxu/acsync/internal/desktop/models.ts` — regenerated models.
- `frontend/bindings/github.com/qinqingxu/acsync/internal/desktop/wailsservice.ts` — regenerated methods.

### Deliberately unchanged

- `internal/cli/status.go` remains a full CLI status calculation in this plan.
- `internal/resourcecollect/collector.go` remains the full collector until the indexed-sync plan.
- Sync frequency and archive-retention behavior do not change.

## Task 1: Add focused frontend test support

**Files:**
- Modify: `frontend/package.json`
- Modify: `frontend/package-lock.json`
- Modify: `frontend/vite.config.ts`
- Create: `frontend/src/test/setup.ts`
- Create: `frontend/src/resources/InstallPlanPanel.test.tsx`

- [ ] **Step 1: Install the existing-stack test dependencies**

Run:

```powershell
npm --prefix frontend install --save-dev vitest jsdom @testing-library/react @testing-library/user-event @testing-library/jest-dom
```

Expected: `package.json` and `package-lock.json` change without modifying runtime dependencies.

- [ ] **Step 2: Add the test script and DOM setup**

Add this script to `frontend/package.json`:

```json
"test": "vitest run"
```

Extend `frontend/vite.config.ts`:

```ts
/// <reference types="vitest/config" />
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";
import wails from "@wailsio/runtime/plugins/vite";

export default defineConfig({
  server: {
    host: "127.0.0.1",
    port: Number(process.env.WAILS_VITE_PORT) || 9245,
    strictPort: true,
  },
  plugins: [react(), wails("./bindings")],
  test: {
    environment: "jsdom",
    setupFiles: ["./src/test/setup.ts"],
    clearMocks: true,
  },
});
```

Create `frontend/src/test/setup.ts`:

```ts
import '@testing-library/jest-dom/vitest'
```

- [ ] **Step 3: Write and run a component smoke test**

Create `frontend/src/resources/InstallPlanPanel.test.tsx`:

```tsx
import { render, screen } from '@testing-library/react'
import { describe, expect, it, vi } from 'vitest'
import { InstallPlanPanel } from './InstallPlanPanel'

describe('InstallPlanPanel', () => {
  it('shows the trusted executable separately from its arguments', () => {
    render(
      <InstallPlanPanel
        plan={{
          id: 'plan-1',
          approved: false,
          operations: [{
            id: 'operation-1',
            adapter: 'copilot-plugin',
            source: 'wiqd@wiqd',
            kind: 'update',
            executable: 'copilot',
            args: ['plugin', 'update', 'wiqd@wiqd'],
            workingDir: '',
            error: '',
          }],
        }}
        busy={false}
        approve={vi.fn()}
      />,
    )
    expect(screen.getByText('Executable: copilot')).toBeInTheDocument()
    expect(screen.getByText('Arguments: ["plugin","update","wiqd@wiqd"]')).toBeInTheDocument()
  })
})
```

Run:

```powershell
npm --prefix frontend test -- InstallPlanPanel.test.tsx
```

Expected: PASS.

- [ ] **Step 4: Commit the test foundation**

```powershell
git add frontend\package.json frontend\package-lock.json frontend\vite.config.ts frontend\src\test\setup.ts frontend\src\resources\InstallPlanPanel.test.tsx
git commit -m "test: add desktop component test support" -m "Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
```

## Task 2: Persist compact cycle and preview summaries

**Files:**
- Create: `internal/desktop/summary_store.go`
- Create: `internal/desktop/summary_store_test.go`
- Modify: `internal/desktop/models.go`
- Modify: `internal/desktop/service.go`

- [ ] **Step 1: Write failing store tests**

Create tests that exercise the public store contract:

```go
func TestSummaryStoreRoundTripsOwnerOnlyState(t *testing.T) {
	root := t.TempDir()
	store := newSummaryStore(root)
	cycle := daemon.CycleResult{
		Actions:    3,
		Blocked:    2,
		FinishedAt: time.Date(2026, 8, 18, 10, 11, 12, 0, time.UTC),
	}
	preview := ResourcePreview{
		GeneratedAt: time.Date(2026, 8, 18, 10, 10, 0, 0, time.UTC),
		Files:       7,
	}

	if err := store.saveCycle(cycle); err != nil {
		t.Fatal(err)
	}
	if err := store.savePreview(preview); err != nil {
		t.Fatal(err)
	}
	gotCycle, err := store.loadCycle()
	if err != nil || !reflect.DeepEqual(gotCycle, cycle) {
		t.Fatalf("loadCycle() = %#v, %v", gotCycle, err)
	}
	gotPreview, err := store.loadPreview()
	if err != nil || !reflect.DeepEqual(gotPreview, preview) {
		t.Fatalf("loadPreview() = %#v, %v", gotPreview, err)
	}
	for _, name := range []string{"cycle.json", "preview.json"} {
		info, err := os.Stat(filepath.Join(root, "desktop", name))
		if err != nil || info.Mode().Perm()&0o077 != 0 {
			t.Fatalf("%s mode = %v, %v", name, info.Mode().Perm(), err)
		}
	}
}

func TestSummaryStoreRejectsMalformedJSON(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "desktop", "cycle.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := newSummaryStore(root).loadCycle(); err == nil {
		t.Fatal("loadCycle() error = nil")
	}
}
```

Run:

```powershell
go test ./internal/desktop -run 'TestSummaryStore' -count=1
```

Expected: FAIL because `newSummaryStore` and `ResourcePreview.GeneratedAt` do not exist.

- [ ] **Step 2: Implement atomic summary persistence**

Add to `internal/desktop/models.go`:

```go
type ResourcePreview struct {
	GeneratedAt   time.Time          `json:"generatedAt"`
	Resources     []ResourceCategory `json:"resources"`
	Files         int                `json:"files"`
	Bytes         int64              `json:"bytes"`
	ExcludedFiles int                `json:"excludedFiles"`
	ExcludedBytes int64              `json:"excludedBytes"`
	Issues        []ResourceIssue    `json:"issues"`
}
```

Implement `summaryStore` in `internal/desktop/summary_store.go` with this exact API:

```go
type summaryStore struct {
	root string
}

func newSummaryStore(home string) *summaryStore
func (s *summaryStore) loadCycle() (daemon.CycleResult, error)
func (s *summaryStore) saveCycle(daemon.CycleResult) error
func (s *summaryStore) loadPreview() (ResourcePreview, error)
func (s *summaryStore) savePreview(ResourcePreview) error
func (s *summaryStore) clearPreview() error
```

Each load treats `os.ErrNotExist` as a zero value. Each save must create
the home's `desktop` directory with `0700`, write a sibling temporary file with `0600`, call
`Sync`, close it, and rename it over the target. JSON parse, sync, close, and
rename errors are returned with the target name. `clearPreview` removes only
`preview.json`, treating a missing file as success.

- [ ] **Step 3: Load the last cycle once and persist every completed cycle**

In `New`, load the compact cycle before `StartConfigured`. A malformed summary
must fail construction rather than masquerade as a successful last sync.

Change `recordCycle` to persist before publishing the new in-memory value:

```go
func appendCycleError(existing string, err error) string {
	if err == nil {
		return existing
	}
	if existing == "" {
		return err.Error()
	}
	return errors.Join(errors.New(existing), err).Error()
}

func (s *Service) recordCycle(result daemon.CycleResult) {
	if err := newSummaryStore(s.home).saveCycle(result); err != nil {
		result.Error = appendCycleError(
			result.Error,
			fmt.Errorf("save desktop cycle summary: %w", err),
		)
		result.NeedsAttention = true
	}
	s.mu.Lock()
	s.last = result
	s.mu.Unlock()
}
```

- [ ] **Step 4: Run the focused tests**

```powershell
go test ./internal/desktop -run 'TestSummaryStore|TestService' -count=1
```

Expected: PASS.

- [ ] **Step 5: Regenerate bindings for the preview timestamp**

```powershell
wails3 generate bindings -clean=true -ts -i
```

Expected: `ResourcePreview.generatedAt` appears in
`frontend\bindings\github.com\qinqingxu\acsync\internal\desktop\models.ts`.

- [ ] **Step 6: Commit**

```powershell
git add internal\desktop\models.go internal\desktop\service.go internal\desktop\summary_store.go internal\desktop\summary_store_test.go frontend\bindings
git commit -m "feat: persist desktop sync summaries" -m "Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
```

## Task 3: Make Snapshot a lightweight control-plane read

**Files:**
- Modify: `internal/desktop/service.go`
- Modify: `internal/desktop/service_test.go`
- Modify: `internal/desktop/lifecycle_test.go`
- Modify: `internal/scheduler/scheduler.go`
- Modify: `internal/scheduler/scheduler_test.go`

- [ ] **Step 1: Write regression tests that cannot tolerate collection**

Create a configured service whose provider root contains a named pipe or an
unreadable file that would block/fail collection, then call `Snapshot()` with a
one-second deadline:

```go
func TestSnapshotUsesPersistedSummariesWithoutCollectingResources(t *testing.T) {
	home := configuredDesktopHome(t)
	writeDesktopPreview(t, home, ResourcePreview{
		GeneratedAt: time.Date(2026, 8, 18, 9, 0, 0, 0, time.UTC),
		Files:       12,
	})
	service, err := New(home, "windows")
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()

	done := make(chan struct{})
	var snapshot Snapshot
	var snapshotErr error
	go func() {
		snapshot, snapshotErr = service.Snapshot()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("Snapshot() waited for resource collection")
	}
	if snapshotErr != nil {
		t.Fatal(snapshotErr)
	}
	if snapshot.Preview.Files != 12 {
		t.Fatalf("preview files = %d", snapshot.Preview.Files)
	}
}
```

Use the existing `t.TempDir`, config persistence, and provider fixture helpers
from `service_test.go`; do not start a real Git network operation.

Add a lifecycle test that drives scheduler state transitions and asserts two
snapshot publications arrive inside one second.

Run:

```powershell
go test ./internal/desktop -run 'TestSnapshotUsesPersisted|TestRunPublishes' -count=1
```

Expected: FAIL or timeout because `Snapshot()` calls `cli.RunStatus()` and
`preview()`.

- [ ] **Step 2: Remove full scans from Snapshot**

First add an authoritative scheduler API:

```go
func (s *Scheduler) NextRun() time.Time
```

Track `nextRun` under the scheduler mutex. `Run` sets it to `now + interval`.
An interval change resets it from the change time. A periodic or manual run
resets it after that cycle finishes. `Pause` clears it; `Resume` rearms it.
Context cancellation clears it. Add a package-private `now func() time.Time`
and timer factory so tests use deterministic time without sleeping.

Tests must prove startup, `SetInterval`, manual `Trigger`, periodic completion,
Pause, and Resume all report the exact next run. Do not derive next run from
the last successful cycle.

Delete the `cli.RunStatus` and `s.preview` calls from `Snapshot()`. Load the
persisted preview and use the in-memory/persisted cycle:

```go
preview, err := newSummaryStore(s.home).loadPreview()
if err != nil {
	return Snapshot{}, fmt.Errorf("load desktop preview summary: %w", err)
}

s.mu.RLock()
d := s.daemon
last := s.last
progress := s.progress
s.mu.RUnlock()

pendingActions := progress.TotalActions - progress.CompletedActions
if pendingActions < 0 {
	pendingActions = 0
}
```

Populate:

```go
LastSync:       last.FinishedAt,
NextSync:       d.Scheduler.NextRun(),
PendingActions: pendingActions,
BlockedFiles:  last.Blocked,
Preview:       preview,
```

Continue loading config, provider declarations, install plan, and conflict
records. `makeAgents` must accept an empty preview and still return configured
agents/categories with zero counts.

- [ ] **Step 3: Verify lifecycle publications remain lightweight**

```powershell
go test ./internal/scheduler ./internal/desktop -run 'TestNextRun|TestSnapshot|TestRun|TestService' -count=1
```

Expected: PASS in under five seconds without invoking agent inventory commands.

- [ ] **Step 4: Commit**

```powershell
git add internal\scheduler internal\desktop\service.go internal\desktop\service_test.go internal\desktop\lifecycle_test.go
git commit -m "perf: make desktop snapshots lightweight" -m "Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
```

## Task 4: Coalesce and persist on-demand resource previews

**Files:**
- Create: `internal/desktop/preview_coordinator.go`
- Create: `internal/desktop/preview_coordinator_test.go`
- Modify: `internal/desktop/service.go`
- Modify: `internal/desktop/resources.go`
- Modify: `internal/desktop/resource_service_test.go`
- Modify: `internal/desktop/wails.go`
- Modify: `internal/desktop/resource_wails_test.go`
- Modify: `internal/resourcecollect/collector.go`
- Modify: `internal/resourcecollect/collector_test.go`
- Modify: `internal/installplan/inventory.go`
- Modify: `internal/installplan/manager_test.go`
- Create: `frontend/src/SettingsPanel.tsx`
- Create: `frontend/src/SettingsPanel.test.tsx`
- Create: `frontend/src/desktopState.ts`
- Modify: `frontend/src/App.tsx`

- [ ] **Step 1: Test one in-flight preview**

Use a function injection rather than a real collector:

```go
func TestPreviewCoordinatorCoalescesConcurrentCalls(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	coordinator := newPreviewCoordinator(func(ctx context.Context) (ResourcePreview, error) {
		if calls.Add(1) == 1 {
			close(started)
		}
		select {
		case <-release:
			return ResourcePreview{Files: 8}, nil
		case <-ctx.Done():
			return ResourcePreview{}, ctx.Err()
		}
	})

	results := make(chan ResourcePreview, 2)
	for range 2 {
		go func() {
				preview, err := coordinator.refresh(context.Background())
			if err != nil {
				t.Error(err)
			}
			results <- preview
		}()
	}
	<-started
	close(release)
	if (<-results).Files != 8 || (<-results).Files != 8 {
		t.Fatal("coalesced callers received different previews")
	}
	if calls.Load() != 1 {
		t.Fatalf("collector calls = %d", calls.Load())
	}
}
```

Run:

```powershell
go test ./internal/desktop -run TestPreviewCoordinator -count=1
```

Expected: FAIL because the coordinator does not exist.

- [ ] **Step 2: Implement the coordinator**

Create a coordinator with these fields and method:

```go
type previewResult struct {
	preview ResourcePreview
	err     error
}

type previewCall struct {
	done    chan struct{}
	cancel  context.CancelFunc
	waiters int
	result  previewResult
}

type previewCoordinator struct {
	mu       sync.Mutex
	collect  func(context.Context) (ResourcePreview, error)
	inflight *previewCall
}

func newPreviewCoordinator(collect func(context.Context) (ResourcePreview, error)) *previewCoordinator
func (c *previewCoordinator) refresh(ctx context.Context) (ResourcePreview, error)
```

`previewCall` owns an internal context/cancel function and a waiter count. The
first caller creates it and starts collection in one goroutine. Joiners
increment the waiter count, wait on either `done` or their own context, and
read the immutable result. On return/cancellation each caller decrements the
count; when it reaches zero before completion, cancel the internal collection.
Do not rely on multiple receives from a one-value channel.

Add a second test with two callers: cancel the first and prove collection
continues for the second; cancel both and prove the injected collector receives
`context.Canceled`.

- [ ] **Step 3: Wire preview persistence**

Add `preview *previewCoordinator` to `Service`. Initialize it in `New` with a
closure calling the existing `s.preview`.

Change the APIs to:

```go
func (s *Service) ResourcePreview(ctx context.Context) (ResourcePreview, error)
func (s *WailsService) ResourcePreview(ctx context.Context) (ResourcePreview, error)
```

The coordinator must call
`s.preview(ctx, cfg, providers)` with the signature:

```go
func (s *Service) preview(
	ctx context.Context,
	cfg config.Config,
	providers []provider.Provider,
) (ResourcePreview, error)
```

`ResourcePreview(ctx)` calls `refresh(ctx)`, sets `GeneratedAt = time.Now().UTC()`
only after successful collection, save the summary, and then return it. A
collection or persistence error must leave the previous preview file intact.

`SaveSettings` must clear the persisted preview only after the settings write
succeeds, then queue normal synchronization. It must not synchronously collect.

Add cancellable collection without breaking existing callers:

```go
type InventoryProvider interface {
	InventoryContext(context.Context, resource.Spec) (map[string][]byte, error)
}

func (c *Collector) Collect(specs []resource.Spec) (Result, error) {
	return c.CollectContext(context.Background(), specs)
}

func (c *Collector) CollectContext(
	ctx context.Context,
	specs []resource.Spec,
) (Result, error)
```

`CollectContext` checks `ctx.Err()` before every spec, directory, file read,
projection, inventory call, and stage write. Cancellation returns the context
error joined with stage cleanup and never returns a partial successful result.

Add to `InventoryRegistry`:

```go
func (r *InventoryRegistry) Inventory(spec resource.Spec) (map[string][]byte, error) {
	return r.InventoryContext(context.Background(), spec)
}

func (r *InventoryRegistry) InventoryContext(
	ctx context.Context,
	spec resource.Spec,
) (map[string][]byte, error)
```

`InventoryContext` passes `ctx` to `adapter.Discover` instead of using
`context.Background`. Update fake inventories to accept context. The existing
`Inventory` wrapper preserves manager tests and non-cancellable callers.

Change both `ResourcePreview` and `PreviewCustomResource` service/Wails methods
to accept `context.Context`, and have `s.preview` call `CollectContext`.
Update `resource_wails_test.go` so injected service functions capture the
forwarded context and assert cancellation reaches both Wails methods.

Run:

```powershell
go test ./internal/desktop -run 'TestPreviewCoordinator|TestResourcePreview|TestSaveSettings' -count=1
```

Expected: PASS.

- [ ] **Step 4: Extract SettingsPanel and test immediate opening**

Create `frontend/src/desktopState.ts` and move `AppAgent`, `AppPreview`,
`AppSnapshot`, and `normalizeSnapshot` out of `App.tsx`. These normalized types
must convert every nullable generated slice (`agents`, resources, issues,
conflicts, custom resources, install operations) to an empty array before a
component calls `.map()`.

Move the existing `SettingsPanel` function from `App.tsx` to
`frontend/src/SettingsPanel.tsx`. Its `snapshot` prop is `AppSnapshot`, not the
nullable generated `Snapshot`. Add props:

```ts
type SettingsPanelProps = {
  snapshot: AppSnapshot
  busy: boolean
  previewLoading: boolean
  close: () => void
  refreshPreview: () => Promise<void>
  save: (
    input: SettingsInput,
    startAtLogin: boolean,
    startAtLoginChanged: boolean,
  ) => Promise<unknown>
}
```

Write a test that renders the Go zero-time value
`0001-01-01T00:00:00Z`, verifies the dialog is immediately present, clicks
`Refresh preview`, and verifies the supplied promise is called once. Add a
second test showing a valid persisted `generatedAt` timestamp and file count.

In `App.tsx`, opening Settings sets the panel visible first. A `useEffect`
starts `ResourcePreview()` only when `generatedAt` is missing, invalid, or has
UTC year `<= 1`; when it resolves, normalize its nullable arrays and merge only
the returned preview into the current snapshot. Retain the returned
`CancellablePromise` and call `.cancel()` in effect cleanup or when Settings
closes. Expose the same cancellable call through the explicit refresh button.

- [ ] **Step 5: Run frontend tests and production build**

Regenerate Wails bindings first because the preview method signatures changed:

```powershell
wails3 generate bindings -clean=true -ts -i
npm --prefix frontend test
npm --prefix frontend run build
```

Expected: tests and TypeScript/Vite production build PASS.

- [ ] **Step 6: Commit**

```powershell
git add internal\desktop internal\resourcecollect\collector.go internal\resourcecollect\collector_test.go internal\installplan\inventory.go internal\installplan\manager_test.go frontend\bindings frontend\src\App.tsx frontend\src\desktopState.ts frontend\src\SettingsPanel.tsx frontend\src\SettingsPanel.test.tsx
git commit -m "feat: refresh resource preview on demand" -m "Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
```

## Task 5: Add revision-bound pending conflict batches

**Files:**
- Create: `internal/conflict/resolution.go`
- Create: `internal/conflict/resolution_test.go`
- Modify: `internal/conflict/store.go`
- Modify: `internal/desktop/models.go`
- Modify: `internal/desktop/service.go`
- Modify: `internal/desktop/resources.go`

- [ ] **Step 1: Write batch preflight tests**

Use the existing bundle helper pattern from `store_test.go`:

```go
func TestQueueBatchPreflightsEverySelectionBeforePersisting(t *testing.T) {
	localRoot := t.TempDir()
	repoDir := t.TempDir()
	store := NewStore(localRoot, repoDir, func(record Record, variant string, data []byte) error {
		if record.ID == "blocked" && variant == "merged" {
			return errors.New("secret detected")
		}
		return nil
	})
	createResolutionFixture(t, store, "safe", "one.json")
	createResolutionFixture(t, store, "blocked", "two.json")
	safeRevision := mustConflictRevision(t, store, "safe")
	blockedRevision := mustConflictRevision(t, store, "blocked")

	_, err := store.QueueBatch([]ResolutionSelection{
		{ID: "safe", Revision: safeRevision, Choice: ChoiceLocal},
		{ID: "blocked", Revision: blockedRevision, Choice: ChoiceMerged, Content: []byte(`{"token":"x"}`)},
	})
	if err == nil {
		t.Fatal("QueueBatch() error = nil")
	}
	if pending, pendingErr := store.PendingBatch(); pendingErr != nil || pending != nil {
		t.Fatalf("PendingBatch() = %#v, %v", pending, pendingErr)
	}
}
```

Add tests for duplicate IDs, unsupported choices, missing IDs, stale revisions,
partial selection, deterministic batch ID, and merged content scanning. Add a
concurrency test that creates two `Store` instances for the same `localRoot`,
releases two `QueueBatch` goroutines through one barrier, and proves exactly one
batch succeeds while the other returns `ErrResolutionActive`.

Run:

```powershell
go test ./internal/conflict -run 'TestQueueBatch|TestConflictRevision' -count=1
```

Expected: FAIL because the batch API does not exist.

- [ ] **Step 2: Define revisions and pending models**

Create these exported models in `resolution.go`:

```go
type Choice string

const (
	ChoiceLocal  Choice = "local"
	ChoiceRemote Choice = "remote"
	ChoiceMerged Choice = "merged"
)

type ResolutionSelection struct {
	ID       string `json:"id"`
	Revision string `json:"revision"`
	Choice   Choice `json:"choice"`
	Content  []byte `json:"content,omitempty"`
}

type ResolutionBatch struct {
	ID         string                `json:"id"`
	CreatedAt  time.Time             `json:"createdAt"`
	Status     string                `json:"status"`
	Error      string                `json:"error,omitempty"`
	Selections []ResolutionSelection `json:"selections"`
}

var ErrResolutionActive = errors.New("a conflict resolution batch is already active")

type VisibleConflict struct {
	Record   Record `json:"record"`
	Revision string `json:"revision"`
}
```

The revision is SHA-256 over canonical JSON containing `Record` and SHA-256
hashes of `base`, `local`, and `remote`. Compute it from the local bundle and
return lowercase hex.

Implement:

```go
func (s *Store) Revision(id string) (string, error)
func (s *Store) QueueBatch([]ResolutionSelection) (ResolutionBatch, error)
func (s *Store) PendingBatch() (*ResolutionBatch, error)
func (s *Store) FailedBatch() (*ResolutionBatch, error)
func (s *Store) ResolutionStatus() (*ResolutionBatch, error)
func (s *Store) VisibleConflicts() ([]VisibleConflict, *ResolutionBatch, error)
```

Persist pending state as
the `conflict-resolution\pending.json` sibling of the local conflict directory using the same
owner-only atomic write pattern as conflict bundles. Batch IDs are SHA-256 over
the ordered canonical selections; sort by ID before hashing and persistence.

`NewStore` assigns every instance a process-wide keyed lock set obtained from
the cleaned absolute resolution root:

```go
var resolutionLocks sync.Map

type resolutionLockSet struct {
	metadata    sync.Mutex
	transaction sync.RWMutex
}

func sharedResolutionLocks(root string) *resolutionLockSet
```

The desktop app is single-instance, and every batch operation is in this
process. The metadata mutex protects only short reads/writes of `pending.json`,
`failed.json`, and `applying-view.json`; it is never held across canonical file
writes or scans. The transaction RW mutex protects conflict bundle/repository
mutation. `List`, `Revision`, and queue preflight use a read lock;
`MirrorFromRepo`, `ApplyPendingBatch`, and `RecoverTransactions` use a write
lock and private `listUnlocked`/`revisionUnlocked` helpers; never recursively
acquire the RW mutex.

Always acquire the transaction lock before metadata when both are needed.
`QueueBatch` takes the transaction read lock, performs complete preflight,
takes metadata, checks for an active batch, and atomically writes
`pending.json`. It removes an older `failed.json` only after the new pending
file is durable. Separate `Store` instances therefore cannot overwrite one
another.

`VisibleConflicts` is the nonblocking Snapshot API:

1. Under metadata, if status is `applying`, read and return the immutable
   `applying-view.json`.
2. Otherwise attempt `transaction.TryRLock()`. If a writer won, loop back and
   read the now-visible applying view instead of waiting for the transaction.
3. After acquiring the read lock, recheck status under metadata. Return the
   view if it changed to applying; otherwise list records and revisions while
   holding the read lock.

This gives Snapshot a stable conflict set without blocking behind a full batch
transaction.

- [ ] **Step 3: Add conflict revisions to desktop summaries**

Extend `ConflictSummary`:

```go
type ConflictSummary struct {
	ID          string    `json:"id"`
	Revision    string    `json:"revision"`
	ResourceKey string    `json:"resourceKey"`
	Path        string    `json:"path"`
	CreatedAt   time.Time `json:"createdAt"`
}

type ConflictResolutionStatus struct {
	ID       string `json:"id"`
	Status   string `json:"status"`
	Selected int    `json:"selected"`
	Error    string `json:"error,omitempty"`
}
```

Change `desktopConflicts` to accept `[]conflict.VisibleConflict` and map the
already computed revisions. `Snapshot()` calls `VisibleConflicts`, projects its
returned batch status, and propagates errors rather than omitting a conflict.

Add this field to `Snapshot`:

```go
ConflictResolution *ConflictResolutionStatus `json:"conflictResolution,omitempty"`
```

For no active batch, `VisibleConflicts` also returns the most recent terminal
`failed` record. These are small local JSON reads and remain part of the
lightweight control plane.

- [ ] **Step 4: Run tests**

```powershell
go test ./internal/conflict ./internal/desktop -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```powershell
git add internal\conflict internal\desktop\models.go internal\desktop\service.go internal\desktop\resources.go
git commit -m "feat: queue revision-bound conflict batches" -m "Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
```

## Task 6: Apply conflict batches transactionally after pull

**Files:**
- Modify: `internal/conflict/resolution.go`
- Modify: `internal/conflict/resolution_test.go`
- Modify: `internal/conflict/store.go`
- Modify: `internal/syncengine/engine.go`
- Modify: `internal/syncengine/engine_test.go`

- [ ] **Step 1: Write rollback and recovery tests**

Add package-private file operation injection to `Store` tests and prove:

```go
func TestApplyPendingBatchRollsBackEveryConflictOnWriteFailure(t *testing.T) {
	store, records := queuedResolutionFixture(t, 2)
	originalWrite := store.ops.writeAtomic
	writes := 0
	store.ops.writeAtomic = func(name string, data []byte, mode os.FileMode) error {
		writes++
		if writes == 2 {
			return errors.New("injected write failure")
		}
		return originalWrite(name, data, mode)
	}

	if _, err := store.ApplyPendingBatch(); err == nil {
		t.Fatal("ApplyPendingBatch() error = nil")
	}
	for _, record := range records {
		assertConflictBundleExists(t, store, record.ID)
		assertCanonicalUnchanged(t, store, record)
	}
	if pending, err := store.PendingBatch(); err != nil || pending != nil {
		t.Fatalf("PendingBatch() = %#v, %v", pending, err)
	}
	if failed, err := store.FailedBatch(); err != nil || failed == nil {
		t.Fatalf("FailedBatch() = %#v, %v", failed, err)
	}
}
```

Also test successful partial batches, stale-after-pull batches, pre-commit
tombstone restoration, committed tombstone cleanup, and `List` /
`MirrorFromRepo` ignoring `.resolution-*` directories. Add a blocked-write test:
pause `ApplyPendingBatch` after its first tombstone rename, call
`VisibleConflicts` with a 250 ms deadline, and assert it immediately returns
the complete pre-transaction set plus status `applying`.

Run:

```powershell
go test ./internal/conflict -run 'TestApplyPendingBatch|TestRecoverResolution' -count=1
```

Expected: FAIL.

- [ ] **Step 2: Implement the transaction**

Add:

```go
type ApplyResult struct {
	BatchID  string
	Resolved int
}

type BatchError struct {
	Terminal bool
	Err      error
}

func (e *BatchError) Error() string
func (e *BatchError) Unwrap() error
func IsTerminalBatchError(error) bool
func (s *Store) ApplyPendingBatch() (ApplyResult, error)
func (s *Store) RecoverTransactions() error
```

`ApplyPendingBatch` must:

1. Reload the pending batch and recheck every revision and selected content.
2. While holding the transaction read lock, build the complete
   `[]VisibleConflict`, atomically save it to `applying-view.json`, and then
   atomically change `pending.json` from `queued` to `applying` under metadata.
   Release the read lock and acquire the transaction write lock before any
   mutation.
3. Create a transaction directory with a JSON journal and `committed=false`.
4. Rename both local and repository bundles to transaction tombstones.
5. Copy each existing canonical file into the transaction backup area, recording
   whether it existed and its mode.
6. Write all canonical selections with `writeAtomic`.
7. Set `committed=true` atomically.
8. Remove tombstones. Under metadata, remove `pending.json` and
   `applying-view.json`; then remove the committed journal.

Before the commit marker, any error restores canonical backups and both bundle
roots. A stale/preflight/write failure that rolls back completely moves the
batch to `failed.json`, clears the active pending file, and returns
`BatchError{Terminal: true}`. A rollback failure returns
`BatchError{Terminal: false}` and preserves the journal for recovery. After the
commit marker, cleanup errors retain the journal for startup recovery but do
not recreate conflicts and are nonterminal infrastructure errors.

`RecoverTransactions` takes the transaction write lock. If it finds
`status=applying` plus a view but no journal (a crash before mutation), reset
the batch to `queued` and remove the view. For a pre-commit journal, restore and
move the batch to failed; for a committed journal, finish tombstone cleanup,
then clear pending/view under metadata. Ignore `.resolution-*` names during
directory scans.

- [ ] **Step 3: Recover before Git pull, then integrate batch apply**

In `Engine.syncOnce`, call `RecoverTransactions()` immediately after
`validate()` and before `SetLocalConfig` or `PullRebase`. A crash may have left
canonical worktree edits or transaction tombstones, and recovery must restore
or finish them before Git inspects the worktree.

Add an integration test that seeds a pre-commit journal plus canonical edits,
uses the existing local bare-remote fixture, and calls the existing
`Git.HasChanges()` from the `StagePulling` progress callback. It must observe
`false` before the real fixture's `PullRebase` runs.

Call `ApplyPendingBatch` immediately after
`MirrorFromRepo` and before conflict listing/reconciliation. When a batch
resolves at least one conflict, rerun `SnapshotRepo`, `SplitRemoteSnapshot`,
and `ValidateRemoteResources` before reconciliation so canonical writes and
removed conflict bundles are reflected in `validRemote`. This extra pass occurs
only for a queued batch and will use the indexed path after the second plan.
When `IsTerminalBatchError(err)` is true, append one `resource.Issue` with code
`conflict-resolution-failed`, set `NeedsAttention`, and continue with restored
unresolved conflicts. Infrastructure errors that prevent safe rollback remain
fatal.

Add an engine integration test:

```go
func TestSyncAppliesOneQueuedConflictBatchAfterPull(t *testing.T) {
	engine, store := conflictBatchEngine(t)
	queueTwoConflictSelections(t, store)
	result, err := engine.SyncOnce()
	if err != nil {
		t.Fatal(err)
	}
	if result.Conflicts != 0 {
		t.Fatalf("conflicts = %d", result.Conflicts)
	}
	if pending, err := store.PendingBatch(); err != nil || pending != nil {
		t.Fatalf("pending = %#v, %v", pending, err)
	}
}
```

- [ ] **Step 4: Run focused engine tests**

```powershell
go test ./internal/conflict ./internal/syncengine -run 'Conflict|Resolution' -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```powershell
git add internal\conflict internal\syncengine
git commit -m "feat: apply conflict batches transactionally" -m "Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
```

## Task 7: Expose one batch request and regenerate Wails bindings

**Files:**
- Modify: `internal/desktop/resources.go`
- Modify: `internal/desktop/resource_service_test.go`
- Modify: `internal/desktop/wails.go`
- Modify: `internal/desktop/resource_wails_test.go`
- Regenerate: `frontend/bindings`

- [ ] **Step 1: Test one Trigger for an accepted batch**

Add a trigger function seam to `Service`:

```go
queueSync func() error
```

Move the existing scheduler lookup into `triggerScheduler`, initialize
`queueSync = service.triggerScheduler` in `New`, and make public `Trigger`
delegate to `queueSync`. Use it in tests:

```go
func TestResolveConflictsQueuesOneBatchAndTriggersOnce(t *testing.T) {
	service, store := desktopConflictService(t, 2)
	var triggers int
	service.queueSync = func() error {
		triggers++
		return nil
	}
	input := ConflictBatchInput{Selections: desktopSelections(t, store)}
	if err := service.ResolveConflicts(input); err != nil {
		t.Fatal(err)
	}
	if triggers != 1 {
		t.Fatalf("triggers = %d", triggers)
	}
}

func TestResolveConflictsDoesNotTriggerRejectedBatch(t *testing.T) {
	service, _ := desktopConflictService(t, 1)
	service.queueSync = func() error {
		t.Fatal("Trigger called")
		return nil
	}
	err := service.ResolveConflicts(ConflictBatchInput{
		Selections: []ConflictResolution{{ID: "missing", Revision: "bad", Choice: "local"}},
	})
	if err == nil {
		t.Fatal("ResolveConflicts() error = nil")
	}
}
```

Run:

```powershell
go test ./internal/desktop -run TestResolveConflicts -count=1
```

Expected: FAIL.

- [ ] **Step 2: Replace the singular desktop API**

Define:

```go
type ConflictResolution struct {
	ID       string `json:"id"`
	Revision string `json:"revision"`
	Choice   string `json:"choice"`
	Content  string `json:"content,omitempty"`
}

type ConflictBatchInput struct {
	Selections []ConflictResolution `json:"selections"`
}
```

Implement:

```go
func (s *Service) ResolveConflicts(input ConflictBatchInput) error
func (s *WailsService) ResolveConflicts(input ConflictBatchInput) error
```

Convert all entries to `conflict.ResolutionSelection`, call `QueueBatch` once,
and invoke `s.Trigger()` once only after persistence succeeds. Remove the
old Wails `ResolveConflict` method; keep the store's legacy single-resolution
methods until migration tests no longer need them.

- [ ] **Step 3: Verify Wails delegation**

Add a Wails test that passes two selections and verifies both reach the core
batch method. Run:

```powershell
go test ./internal/desktop -run 'TestResolveConflicts|TestWails' -count=1
```

Expected: PASS.

- [ ] **Step 4: Regenerate bindings**

```powershell
wails3 generate bindings -clean=true -ts -i
```

Expected: `ConflictBatchInput`, revised `ConflictSummary`, and
`ConflictResolutionStatus`, `ResolveConflicts` appear under
`frontend\bindings`; `ResolveConflict` is gone.

- [ ] **Step 5: Continue directly to the frontend migration**

Do not commit the regenerated bindings yet: removal of `ResolveConflict`
intentionally makes the old `App.tsx` uncompilable. Complete Task 8 and commit
the backend, bindings, and frontend migration together.

## Task 8: Stage conflict selections in the React panel

**Files:**
- Modify: `frontend/src/resources/ConflictPanel.tsx`
- Create: `frontend/src/resources/ConflictPanel.test.tsx`
- Modify: `frontend/src/App.tsx`
- Modify: `frontend/src/style.css`

- [ ] **Step 1: Write interaction tests**

Test no backend call before Apply, select-all, partial Apply, failure retention,
and success clearing:

```tsx
it('submits all staged choices once', async () => {
  const user = userEvent.setup()
  const resolve = vi.fn().mockResolvedValue(true)
  render(
    <ConflictPanel
      conflicts={conflicts}
      busy={false}
      resolve={resolve}
    />,
  )

  await user.click(screen.getByRole('button', { name: 'Use remote for all' }))
  expect(resolve).not.toHaveBeenCalled()
  await user.click(screen.getByRole('button', { name: 'Apply 2 selected' }))

  expect(resolve).toHaveBeenCalledTimes(1)
  expect(resolve).toHaveBeenCalledWith({
    selections: [
      { id: 'one', revision: 'rev-one', choice: 'remote' },
      { id: 'two', revision: 'rev-two', choice: 'remote' },
    ],
  })
})
```

Run:

```powershell
npm --prefix frontend test -- ConflictPanel.test.tsx
```

Expected: FAIL because each row currently resolves immediately.

- [ ] **Step 2: Implement local selection state**

Use:

```ts
type Selection = {
  revision: string
  choice: 'local' | 'remote' | 'merged'
  content?: string
}

const [selections, setSelections] = useState<Record<string, Selection>>({})
```

Local/Remote buttons copy the conflict's currently displayed `revision` into
the record and update selected styling. Saving merged content records that same
observed revision and closes the editor. Select-all replaces the map with one
Local or Remote selection and observed revision for every current conflict.

In an effect, remove selections whose conflict IDs are no longer present or
whose stored revision differs from the latest conflict revision. A changed
variant therefore requires the user to review and select it again.
`Apply selected` sorts current conflicts by ID, builds one
`ConflictBatchInput` from the stored revisions, and awaits `resolve`. The prop
contract is:

```ts
resolve: (input: ConflictBatchInput) => Promise<boolean>
```

Clear the accepted IDs only when the returned value is `true`; retain them when
it is `false` or rejects.

The Apply label is `Apply N selected`, and the button is disabled for
`busy || N === 0 || batch status is queued or applying`. Pass
`snapshot.conflictResolution` into the panel. Show queued/applying status and
selected count above the list. Show a terminal failed batch's exact diagnostic
as an alert, but allow a new selection and submission.

Extend `AppSnapshot`/`normalizeSnapshot` in `desktopState.ts` with the optional
`conflictResolution` field when regenerating the model; preserve `undefined`
when no active or failed batch exists.

- [ ] **Step 3: Wire App to the generated batch method**

Import `ResolveConflicts` instead of `ResolveConflict` and use:

```tsx
resolve={(input) => perform(
  () => ResolveConflicts(input),
  'Conflict decisions saved; one synchronization queued',
)}
```

Because `Snapshot()` is now lightweight, keep `perform`'s post-action refresh.
It updates queued/failed metadata without blocking on collection.

The interaction tests use `mockResolvedValue(true)` for success and
`mockResolvedValue(false)` for the caught-backend-error path. Assert that false
preserves selections. This matches `perform`, which catches, displays the
error, and returns `false`.

- [ ] **Step 4: Style and verify**

Add styles for `.conflict-toolbar`, `.conflict-choice.selected`, and
`.conflict-selection-count` using existing button colors and focus styles.

Run sequentially:

```powershell
npm --prefix frontend test -- ConflictPanel.test.tsx
npm --prefix frontend run build
```

Expected: PASS.

- [ ] **Step 5: Commit**

```powershell
git add internal\desktop frontend\src frontend\bindings
git commit -m "feat: apply staged conflict choices once" -m "Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
```

## Task 9: Persist structured install failures and revoke approval

**Files:**
- Modify: `internal/installplan/model.go`
- Modify: `internal/installplan/store.go`
- Modify: `internal/installplan/executor.go`
- Modify: `internal/installplan/executor_test.go`
- Modify: `internal/installplan/store_test.go`
- Modify: `internal/installplan/manager.go`
- Modify: `internal/installplan/manager_test.go`
- Modify: `internal/syncengine/engine.go`
- Modify: `internal/syncengine/install_test.go`
- Modify: `internal/desktop/models.go`
- Modify: `internal/desktop/resources.go`
- Modify: `internal/desktop/resource_service_test.go`
- Regenerate: `frontend/bindings`

- [ ] **Step 1: Write failure-classification tests**

Add:

```go
func TestExecutorRequiresManualRetryAfterWindowsAccessDenied(t *testing.T) {
	store := NewStore(t.TempDir())
	operation := testOperation()
	if err := store.Approve(operation); err != nil {
		t.Fatal(err)
	}
	plan := Plan{ID: "plan-1", Operations: []Operation{operation}, Approved: true}
	runner := &fakeRunner{err: errors.New("exit status 1"), stderr: []byte(
		"Failed to install plugin: Access is denied. (os error 5)",
	)}
	executor := Executor{Runner: runner, Store: store}

	result, err := executor.Execute(context.Background(), plan)
	if err != nil {
		t.Fatal(err)
	}
	failure := result.Failures[operation.ID]
	if failure.Code != FailureLockedResource || !failure.ManualRetry {
		t.Fatalf("failure = %#v", failure)
	}
	if store.IsApproved(operation) {
		t.Fatal("failed operation remained approved")
	}
}
```

Add tests for lowercase access-denied variants, unknown errors, failed-first /
successful-second operation continuation, and approval revocation persistence.
Add a security regression where two operations share adapter/source but differ
in kind or argv: approving the first must not approve the second.
Add `NewPlan` validation/test rejecting duplicate operation IDs, because
failure persistence and desktop projection are keyed by operation ID.
Add a manager test that performs a failed reconcile and then a periodic
reconcile with the same desired/current manifests. The second reconcile must
make zero runner calls and retain the same structured failure in
`pending.json`.

Add a concurrency test with two `Store` instances for the same install root:
release `ReconcilePending` for an unchanged failed plan and
`ApprovePendingPlan(plan.ID, true)` through one barrier. If approval succeeds,
the final pending plan must have the same ID, no failures, and all current
operations approved; reconciliation must not restore the old failure or
overwrite approval state.

Add an executor concurrency test that pauses before result persistence, changes
the same pending plan through a second Store instance, then resumes. The
executor must return `ErrPendingPlanChanged` (or merge same-ID approval state as
specified below) and must never overwrite the newer pending file.

Run:

```powershell
go test ./internal/installplan -run 'TestExecutorRequiresManual|TestStoreRevoke|TestReconcilePending|TestApprovePendingPlan' -count=1
```

Expected: FAIL.

- [ ] **Step 2: Add structured failures**

Define:

```go
type FailureCode string

const (
	FailureCommand        FailureCode = "command-failed"
	FailureLockedResource FailureCode = "locked-resource"
)

type Failure struct {
	Code        FailureCode `json:"code"`
	Error       string      `json:"error"`
	Recovery    string      `json:"recovery,omitempty"`
	FailedAt    time.Time   `json:"failedAt"`
	ManualRetry bool        `json:"manualRetry"`
}

type Plan struct {
	ID         string             `json:"id"`
	Operations []Operation        `json:"operations"`
	Approved   bool               `json:"approved"`
	Failures   map[string]Failure `json:"failures,omitempty"`
}
```

Replace `ExecutionResult.Errors` with `Failures map[string]Failure`. Replace
`installplan.ReconcileResult.Errors` with
`Failures map[string]Failure`, and update `Manager.Reconcile` and the sync
engine's issue projection to compile in the same task.

Classify `strings.ToLower(message)` containing `access is denied` or
`os error 5` as `locked-resource`, with recovery:

```text
Close Copilot CLI, VS Code, and other agent processes that may be using this Plugin, then retry.
```

Unknown command failures use `command-failed`, preserve the combined exit /
stderr diagnostic, and require manual retry as well.

- [ ] **Step 3: Revoke approval on every command failure**

Add:

```go
func (s *Store) Revoke(operation Operation) error
func (s *Store) ReconcilePending(plan Plan) (Plan, error)
func (s *Store) ApprovePendingPlan(id string, requireFailures bool) (Plan, error)
func (s *Store) CommitExecution(
	id string,
	pending []Operation,
	failures map[string]Failure,
) error

var ErrPendingPlanChanged = errors.New("pending install plan changed")
```

`NewStore` canonicalizes the absolute install root and obtains a process-wide
shared `*sync.Mutex` from a keyed registry. Every Store method uses that shared
lock and private unlocked JSON/approval helpers, so the desktop service and
sync manager cannot interleave compound pending-plan changes.

`Revoke` validates the same identity as `Approve`, removes
`approvalKey(operation)` from `approvals.json`, and writes atomically.

Change `approvalKey` to SHA-256 of the full `operationIdentity` (adapter, ID,
source, kind, executable, working directory, and every argv element), matching
the exact command shown to the user. Existing legacy adapter/source-only
approval keys are intentionally ignored and require one safe reapproval.

`ReconcilePending` holds the shared lock once while it loads the existing plan,
preserves same-ID structured failures by full `operationIdentity`, recomputes
approval from `approvals.json`, and atomically saves/clears `pending.json`.
Change `Manager.Reconcile` to call this single API instead of separate
`Pending`, approval-status, and `SavePending` calls.

`ApprovePendingPlan` holds the shared lock once while it loads `pending.json`,
rejects a mismatched ID, optionally requires at least one failure, validates
every current operation, writes all approval keys to `approvals.json` once,
clears failures, marks the same plan approved, and atomically saves it. Approval
keys are published before the pending rewrite; if the second write fails, the
method returns the persistence error, but any later execution is still
authorized by the user's exact-plan action. Change `ApproveInstallPlan` to use
this method with `requireFailures=false`, so a multi-operation plan cannot be
partially approved or swapped by another `Store` instance.

`CommitExecution` holds the shared lock, reloads the current pending plan, and
rejects a different/missing plan ID with `ErrPendingPlanChanged` without
writing. For the same ID, retain only the supplied pending operations, preserve
current failures for untouched operation identities, overlay fresh failures,
and recompute `Approved` from the current approval file. Clear pending only
when no operations remain. Thus a user approval that races after the executor's
earlier check is retained rather than rewritten as unapproved. The runner and
backup hooks execute outside the shared lock.

In `Executor.Execute`, after building a failure, call `Revoke`. If revocation
fails, return that persistence error rather than claiming the operation is safe
from automatic retry. Continue to later operations only after revocation
succeeds. Initialize the execution result with a clone of `plan.Failures`;
delete an operation's old failure only after it executes successfully; persist
the remaining result once through `CommitExecution` instead of `SavePending`.
Do not hold the Store lock while invoking external commands.

When `ReconcilePending` sees the same plan ID, copy failures only for operations
whose full `operationIdentity` is still present. If the plan ID changed, carry
no failures. A failed operation remains unapproved, so the executor skips it
and retains its failure. This prevents each periodic reconcile from overwriting
the Retry diagnostic.

In `syncengine.Engine`, iterate `installResult.Failures` and append each
failure's original `Error` as the `install-failed` issue message. Keep the
structured fields in the pending plan for the desktop UI.

In the same task, extend desktop `InstallOperation` with:

```go
FailureCode string `json:"failureCode,omitempty"`
Recovery    string `json:"recovery,omitempty"`
ManualRetry bool   `json:"manualRetry"`
```

Change `desktopInstallPlan` to project `plan.Failures` (including the original
error) instead of the removed `plan.Errors`, update its tests, and regenerate
Wails bindings. This keeps the Go repository and generated model buildable at
the Task 9 commit boundary; Task 10 only adds the Retry action and rendering.

- [ ] **Step 4: Run all install-plan tests**

```powershell
go test ./internal/installplan ./internal/syncengine ./internal/desktop -count=1
wails3 generate bindings -clean=true -ts -i
npm --prefix frontend run build
```

Expected: PASS.

- [ ] **Step 5: Commit**

```powershell
git add internal\installplan internal\syncengine\engine.go internal\syncengine\install_test.go internal\desktop frontend\bindings
git commit -m "fix: require manual retry after install failure" -m "Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
```

## Task 10: Add exact-plan Retry to desktop and UI

**Files:**
- Modify: `internal/desktop/resources.go`
- Modify: `internal/desktop/resource_service_test.go`
- Modify: `internal/desktop/wails.go`
- Modify: `internal/desktop/resource_wails_test.go`
- Modify: `frontend/src/resources/InstallPlanPanel.tsx`
- Modify: `frontend/src/resources/InstallPlanPanel.test.tsx`
- Modify: `frontend/src/App.tsx`
- Modify: `frontend/src/style.css`
- Regenerate: `frontend/bindings`

- [ ] **Step 1: Test exact-plan retry and one trigger**

```go
func TestRetryInstallPlanRejectsStalePlan(t *testing.T) {
	service := installPlanService(t, pendingFailedPlan(t))
	if err := service.RetryInstallPlan("old-plan"); err == nil {
		t.Fatal("RetryInstallPlan() error = nil")
	}
}

func TestRetryInstallPlanApprovesCurrentPlanAndTriggersOnce(t *testing.T) {
	service := installPlanService(t, pendingFailedPlan(t))
	var triggers int
	service.queueSync = func() error {
		triggers++
		return nil
	}
	if err := service.RetryInstallPlan("plan-1"); err != nil {
		t.Fatal(err)
	}
	if triggers != 1 {
		t.Fatalf("triggers = %d", triggers)
	}
}
```

Run:

```powershell
go test ./internal/desktop -run TestRetryInstallPlan -count=1
```

Expected: FAIL.

- [ ] **Step 2: Add exact-plan Retry**

Use the structured `InstallOperation` failure fields projected in Task 9; keep
`Error` as the original diagnostic.

Implement:

```go
func (s *Service) RetryInstallPlan(id string) error
func (s *WailsService) RetryInstallPlan(id string) error
```

Call the single atomic `ApprovePendingPlan(id, true)` Store API, which loads
`pending.json` and rejects missing/mismatched IDs or a plan with no failures;
then trigger once. Do not compose `Pending`, approval, and `SavePending` in the
service.

Keep `ApproveInstallPlan` for a newly generated plan with no failures; it calls
`ApprovePendingPlan(id, false)`. Both methods validate the displayed plan ID
inside the same shared-root critical section that writes approval.

- [ ] **Step 3: Regenerate bindings**

```powershell
wails3 generate bindings -clean=true -ts -i
```

Expected: `RetryInstallPlan` and structured operation fields appear.

- [ ] **Step 4: Implement and test recovery UI**

For any operation with `manualRetry`, render its original error and recovery
message. Change the panel eyebrow to `ACTION REQUIRED` and the primary button
to `Retry after closing apps`. Call `retry(plan.id)`.

For a new plan without failures, retain `APPROVAL REQUIRED` and
`Approve and synchronize`.

Extend `InstallPlanPanel.test.tsx`:

```tsx
it('uses explicit retry for a locked Plugin operation', async () => {
  const user = userEvent.setup()
  const approve = vi.fn()
  const retry = vi.fn().mockResolvedValue(undefined)
  render(
    <InstallPlanPanel
      plan={lockedPlan}
      busy={false}
      approve={approve}
      retry={retry}
    />,
  )
  expect(screen.getByText(/Close Copilot CLI/)).toBeInTheDocument()
  await user.click(screen.getByRole('button', { name: 'Retry after closing apps' }))
  expect(retry).toHaveBeenCalledWith(lockedPlan.id)
  expect(approve).not.toHaveBeenCalled()
})
```

Wire App to generated `RetryInstallPlan` through `perform`.

- [ ] **Step 5: Run focused and frontend tests**

```powershell
go test ./internal/installplan ./internal/desktop -count=1
npm --prefix frontend test -- InstallPlanPanel.test.tsx
npm --prefix frontend run build
```

Expected: PASS.

- [ ] **Step 6: Commit**

```powershell
git add internal\desktop frontend
git commit -m "feat: explain and retry locked plugin updates" -m "Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
```

## Task 11: Validate the fast-control-plane release slice

**Files:**
- Create: `internal/desktop/performance_test.go`
- Modify only if validation exposes defects directly caused by Tasks 1–10.

- [ ] **Step 1: Run frontend tests and build before Go reads embedded dist**

```powershell
npm --prefix frontend test
npm --prefix frontend run build
```

Expected: all component tests and the production build PASS.

- [ ] **Step 2: Run all Go tests and vet sequentially**

```powershell
go test ./... -count=1
go vet ./...
```

Expected: PASS. Do not run these concurrently with the frontend build because
Go embeds `frontend\dist` while Vite replaces it.

- [ ] **Step 3: Measure lightweight Snapshot**

Create `internal/desktop/performance_test.go`:

```go
func TestSnapshotPerformanceRealProfile(t *testing.T) {
	if os.Getenv("ACSYNC_REAL_PROFILE") != "1" {
		t.Skip("set ACSYNC_REAL_PROFILE=1 to measure the live profile")
	}
	userHome, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	service, err := New(filepath.Join(userHome, ".acsync"), runtime.GOOS)
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	var maximum time.Duration
	for range 20 {
		started := time.Now()
		if _, err := service.Snapshot(); err != nil {
			t.Fatal(err)
		}
		elapsed := time.Since(started)
		if elapsed > maximum {
			maximum = elapsed
		}
	}
	t.Logf("maximum Snapshot duration: %s", maximum)
	if maximum >= time.Second {
		t.Fatalf("maximum Snapshot duration = %s, want < 1s", maximum)
	}
}
```

Run the desktop service probe 20 times in one process and record the maximum
duration:

```powershell
go test ./internal/desktop -run TestSnapshotPerformanceRealProfile -count=1 -v
```

The test must be opt-in through `ACSYNC_REAL_PROFILE=1`; without the variable it
skips. With the real profile:

```powershell
$env:ACSYNC_REAL_PROFILE = '1'
go test ./internal/desktop -run TestSnapshotPerformanceRealProfile -count=1 -v
```

Expected: every call is below one second and no `acsync-resources-*` preview
stage is created.

- [ ] **Step 4: Verify one batch queues one sync**

With a disposable copy of the real conflict directory and repository, select
all conflicts in the UI, choose `Use remote for all`, and click
`Apply N selected`. Verify the daemon log contains one transition into
`updating` for that batch, not one per conflict. Do not run this destructive
choice against the live 78 conflicts until the user has reviewed the choices.

- [ ] **Step 5: Verify Plugin failure isolation with a fake runner**

Run:

```powershell
go test ./internal/installplan ./internal/syncengine -run 'Install|Plugin|ManualRetry' -count=1 -v
```

Expected: the locked operation remains pending and unapproved, independent
operations complete, and a later periodic cycle does not execute it.

- [ ] **Step 6: Commit any direct validation fixes, otherwise record no commit**

If fixes were required:

```powershell
git add internal\desktop\performance_test.go
git commit -m "fix: complete fast desktop validation" -m "Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
```

Commit the opt-in performance probe even if no defect fix was required:

```powershell
git add internal\desktop\performance_test.go
git commit -m "test: enforce desktop snapshot performance" -m "Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
```

Then continue to the indexed sync performance plan.
