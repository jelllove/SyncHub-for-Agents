# Reset Local Setup Implementation Plan

**Goal:** Safely clear local SyncHub setup and clone and return to onboarding.

**Architecture:** A focused desktop reset module owns preview and path validation,
staging/rollback and credential deletion. Existing scheduler PauseIfIdle and a
restartable desktop lifecycle prevent file writes during reset. The Wails
boundary serializes mutations, resets onboarding state, and exposes preview and
confirm APIs. A shared React confirmation panel serves Settings and onboarding.

## Tasks

1. Add failing synthetic reset tests in `internal/desktop/reset_test.go`: exact
   RESET requirement, expected artifact removal, preservation of provider/log
   files, clone origin/path checks, staging rollback, and fake credential errors.
   Run `go test ./internal/desktop -run TestReset -count=1`.
2. Implement `ResetPreview` and `ResetLocalSetup` in
   `internal/desktop/reset.go`. Delete only staged named targets, never the home
   itself. Use local Git metadata and isolated tests, not network sync.
3. Add idle-stop/reconfiguration tests and make `internal/desktop/lifecycle.go`
   wait for subsequent daemons after reset. Guard reset against an active cycle.
4. Add onboarding `Reset` tests, implement memory reset and OAuth cancellation,
   and expose reset operations in `internal/desktop/wails.go`. Serialize mutation
   APIs and regenerate Wails bindings using the existing generator.
5. Add failing React tests for the shared reset panel, then implement the
   confirmation/path/error UX in `frontend/src/ResetPanel.tsx`. Wire both
   Settings and onboarding and reset App state only after successful backend
   completion.
6. Update `docs/install.md`, run targeted tests, then
   `node scripts/dev.mjs check` and `node scripts/dev.mjs verify`. Review generated
   diffs and preserve failure reports. Build, but do not run reset on real data.

## Completion Gate

Tests must demonstrate actual removal and source preservation, not merely a
successful return value. Explicit failure paths must not transition to Welcome.
No real user credentials, agent homes, or repositories are test fixtures.
