# Sync Stability + First Sync Strategy Design

Date: 2026-08-21  
Product: SyncHub for Agents

## Goals

Address five requested improvements with low regression risk:

1. Tray `Exit` must terminate the process (not just hide window).
2. `git pull --rebase` unstaged-change errors should show actionable UI guidance.
3. Local clone directory should be configurable in Settings, with keep-or-migrate options.
4. After initial setup, Settings should provide a manual “run one sync now” action.
5. First sync should support strategy selection: **Use cloud**, **Merge cloud + local**, **Use local**.

## Scope and Phasing

### Phase P1 (stability-first)

- Fix tray exit behavior.
- Add pull/rebase guidance UI for common local-dirty error.
- Add “Run sync now” in Settings.

### Phase P2 (new sync flows)

- Add configurable local repo path in Settings.
- Add migration mode when changing repo path:
  - Re-clone into new path (keep old path untouched), or
  - Migrate existing repo directory to new path.
- Add first-sync strategy selection before the first real sync cycle starts.

This sequencing minimizes risk: P1 removes user-facing blockers first, P2 introduces higher-impact data-path changes with focused safeguards.

## Architecture

## A) Tray exit correctness

- File focus: `app.go` (Wails tray flow).
- Current `Quit` menu calls `g.app.Quit()` but window close hook always cancels close by hiding.
- Change: add explicit app-level quitting guard so `WindowClosing` does not cancel during true app quit.
- Expected behavior: tray `Quit` performs full shutdown path (`ServiceShutdown`, daemon stop, settings server shutdown, process exit).

## B) Pull/rebase error guidance

- File focus: `internal/syncengine/engine.go`, `internal/desktop/service.go`, `internal/desktop/models.go`, `frontend/src/App.tsx`.
- Add a typed sync diagnostic model in desktop snapshot for known Git failure patterns.
- Pattern detection target (P1): `cannot pull with rebase: You have unstaged changes`.
- UI behavior:
  - Show a dedicated “Fix sync issue” panel when this diagnostic is present.
  - Display local repo path (default `~/.synchub/repo` on Windows resolves to `%USERPROFILE%\.synchub\repo`).
  - Provide one-click “Open repo folder”.
  - Show three command options:
    1) Commit local changes, then pull.
    2) Stash local changes, pull, then pop.
    3) Discard local changes, then pull (explicit caution text).

## C) Settings “Run sync now”

- File focus: `frontend/src/SettingsPanel.tsx`, `frontend/src/App.tsx`.
- Add a non-destructive action button in settings drawer that calls existing `TriggerSync`.
- Keep existing hero button unchanged; settings button is an additional path.

## D) Configurable repo path + migration options (P2)

- File focus: `internal/cli/runtime.go` (`RepoDir` contract), `internal/desktop/models.go`, `internal/desktop/service.go`, `internal/repository/setup.go`, frontend settings/onboarding.
- Extend config to include repository directory override (portable path tokenized by home when persisted).
- Settings flow when path changes:
  - User selects mode:
    - **Re-clone at new path** (old path kept),
    - **Migrate existing repo to new path**.
  - Validation before saving:
    - target path safety checks,
    - source/target accessibility,
    - remote consistency.
  - If migration fails: abort and keep old config/path unchanged (as requested).

## E) First-sync strategy selection (P2)

- File focus: onboarding state + desktop sync trigger path + syncengine first-cycle gate.
- On first configured run, before sync starts, show strategy modal:
  - **Use cloud**: remote snapshot becomes authority.
  - **Merge cloud + local**: existing reconcile behavior.
  - **Use local**: local snapshot initializes/overwrites remote where needed.
- Persist selected strategy and “first-sync-completed” marker to avoid re-prompt.

## Data Flow

1. UI action (tray/settings/onboarding) calls Wails service.
2. Desktop service updates config/state and triggers daemon/scheduler.
3. Sync engine executes and emits progress + result.
4. Desktop snapshot includes:
   - state/progress,
   - last error,
   - structured sync diagnostics (if detected),
   - repo path and first-sync strategy metadata (P2).
5. Frontend renders action-oriented guidance and controls.

## Error Handling

- Do not swallow sync errors; preserve raw error in `LastError`.
- Add deterministic classification for known actionable failures (initially rebase dirty-worktree).
- Migration operations use fail-fast semantics with rollback by “no config write before successful move/clone”.
- User-triggered destructive path (discard local changes) is never automatic; UI only provides instructions.

## Testing Strategy

### Backend tests (Go)

- `app.go`/window-close behavior test for explicit quit path.
- `desktop/service` tests:
  - snapshot includes diagnostic for rebase-dirty error,
  - settings-triggered sync action wiring.
- Repo path migration tests:
  - re-clone mode success/failure,
  - migrate mode success/failure with no partial config write.
- First-sync strategy tests:
  - each strategy branch executes expected reconcile direction,
  - prompt is one-time.

### Frontend tests (Vitest)

- `App` renders fix panel when diagnostic exists.
- Fix panel open-folder action and command rendering.
- `SettingsPanel` includes and triggers “Run sync now”.
- First-sync strategy prompt flow and selected strategy submission.

## Non-Goals

- Automatic stash/commit/discard execution without user confirmation.
- Bulk redesign of settings UI layout.
- Changing conflict-resolution semantics beyond first-sync bootstrap strategy.

## Rollout Notes

- Ship P1 first behind normal release path.
- After P1 validation, ship P2 with migration safeguards and explicit UX copy.
