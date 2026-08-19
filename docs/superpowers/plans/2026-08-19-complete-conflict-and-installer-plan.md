# Complete Conflict and Installer Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Complete atomic conflict resolution, install retry UX, and produce a validated Windows installer.

**Architecture:** Build conflict revisions and durable resolution batches in `internal/conflict`, then integrate them with the desktop snapshot and transaction engine. Migrate the React conflict panel to submit one complete batch. Preserve durable install failures and add exact-plan retry. Finish with frontend generation, Go validation, Windows packaging, and installer smoke checks.

**Tech Stack:** Go, React/TypeScript, Wails v3, Vitest, Go test, Taskfile, NSIS.

---

### Task 1: Add revision-bound conflict batches

**Files:**
- Create: `internal/conflict/resolution.go`
- Create: `internal/conflict/resolution_test.go`
- Modify: `internal/conflict/store.go`

- [ ] Write failing tests for deterministic revisions, complete selections, stale revisions, merged-content scanning, atomic preflight, duplicate IDs, and concurrent stores.
- [ ] Run `go test ./internal/conflict -run 'TestQueueBatch|TestConflictRevision' -count=1` and verify failure.
- [ ] Implement `Choice`, `ResolutionSelection`, `ResolutionBatch`, `VisibleConflict`, revision hashing, keyed lock sets, metadata persistence, `QueueBatch`, `PendingBatch`, `FailedBatch`, `ResolutionStatus`, and `VisibleConflicts`.
- [ ] Run the focused conflict tests and then `go test ./internal/conflict -count=1`.
- [ ] Commit `feat: add revision-bound conflict batches`.

### Task 2: Integrate conflict revisions into desktop snapshots

**Files:**
- Modify: `internal/desktop/models.go`
- Modify: `internal/desktop/service.go`
- Modify: `internal/desktop/resources.go`
- Modify: `internal/desktop/service_test.go`
- Modify: `internal/desktop/resource_service_test.go`

- [ ] Add `Revision` to `ConflictSummary` and `ConflictResolutionStatus` to `Snapshot`.
- [ ] Change snapshot projection to use `VisibleConflicts` and return errors instead of silently dropping conflicts.
- [ ] Add tests for revisions, pending status, failed status, and propagation of malformed conflict metadata.
- [ ] Run `go test ./internal/desktop -run 'TestSnapshot|TestConflict' -count=1`.
- [ ] Commit `feat: expose conflict batch status in desktop snapshots`.

### Task 3: Add transactional conflict apply and recovery

**Files:**
- Create: `internal/conflict/transaction.go`
- Create: `internal/conflict/transaction_test.go`
- Modify: `internal/conflict/store.go`
- Modify: `internal/syncengine/...`
- Modify: `internal/daemon/...`

- [ ] Write failing tests for apply success, rollback after a failed selection, pre-pull recovery, and durable failed batches.
- [ ] Implement applying-view publication, transaction journal, rollback, `ApplyPendingBatch`, and `RecoverTransactions` under the transaction write lock.
- [ ] Integrate recovery before pull and apply pending batches after conflict-aware sync.
- [ ] Run focused conflict, syncengine, and daemon tests.
- [ ] Commit `feat: apply conflict batches transactionally`.

### Task 4: Replace singular conflict API with batch API

**Files:**
- Modify: `internal/desktop/models.go`
- Modify: `internal/desktop/service.go`
- Modify: `internal/desktop/resources.go`
- Modify: `internal/desktop/wails.go`
- Modify: `frontend/src/...`
- Regenerate: `frontend/bindings/...`

- [ ] Add `ConflictSelection`, `QueueConflictBatch`, `RetryConflictBatch`, and status projection.
- [ ] Reject empty or partial selections and validate every revision server-side.
- [ ] Regenerate Wails bindings.
- [ ] Run Go and TypeScript compile tests.
- [ ] Commit `feat: expose atomic conflict batch API`.

### Task 5: Add one-click conflict UI

**Files:**
- Modify: `frontend/src/resources/ConflictPanel.tsx`
- Create/modify: `frontend/src/resources/ConflictPanel.test.tsx`
- Modify: `frontend/src/App.tsx`
- Modify: `frontend/src/desktopState.ts`

- [ ] Write failing tests for local/remote/merged selection, select-all, disabled partial submit, and one Apply action.
- [ ] Implement controlled selections, merged editor, batch submission, pending/applying/failed status, and retry.
- [ ] Run `npm --prefix frontend test -- --run`.
- [ ] Commit `feat: apply all conflict resolutions at once`.

### Task 6: Persist and retry install failures

**Files:**
- Modify: `internal/installplan/store.go`
- Modify: `internal/installplan/executor.go`
- Modify: `internal/installplan/manager.go`
- Modify: `internal/desktop/models.go`
- Modify: `internal/desktop/resources.go`
- Add focused tests beside existing installplan tests.

- [ ] Add structured failure records tied to exact plan and operation IDs.
- [ ] Persist failures atomically and expose retryable operations without re-approving unrelated work.
- [ ] Implement exact-plan retry with actionable access-denied errors.
- [ ] Run `go test ./internal/installplan ./internal/desktop -run 'Test.*Install|Test.*Retry' -count=1`.
- [ ] Commit `feat: persist and retry install failures`.

### Task 7: Add install retry UI

**Files:**
- Modify: `frontend/src/resources/InstallPlanPanel.tsx`
- Add/modify: `frontend/src/resources/InstallPlanPanel.test.tsx`
- Modify: `frontend/src/App.tsx`
- Regenerate: `frontend/bindings/...`

- [ ] Write failing tests for exact retry, error display, and disabled duplicate retry.
- [ ] Implement retry button and structured failure rendering.
- [ ] Run frontend tests and production build.
- [ ] Commit `feat: add install failure retry UI`.

### Task 8: Validate and package Windows installer

**Files:**
- Modify only generated build outputs if packaging requires it.

- [ ] Run `npm --prefix frontend test -- --run` and `npm --prefix frontend run build` sequentially.
- [ ] Run `go test ./...` and `go vet ./...`.
- [ ] Run the existing Windows packaging task from `build/Taskfile.yml`.
- [ ] Verify installer exists, is non-empty, and embeds the expected executable/icon.
- [ ] Install into a clean temporary Windows profile, launch the app, verify no console flash, tray startup, settings opening, and single-instance handoff.
- [ ] Commit packaging metadata only if required, then report installer path and validation results.

