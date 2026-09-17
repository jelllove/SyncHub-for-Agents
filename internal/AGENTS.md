# Backend guidance

Inherit the [root guidance](../AGENTS.md). Go packages here own user data safety;
keep policy enforcement in the backend rather than relying on the UI.

## Trace the boundary before changing it

- `desktop` exposes Wails services/models; `cli` builds dependencies and resource
  specifications; `daemon` schedules jobs; `syncengine` executes synchronization.
- `provider`, `resource`, `pathresolver`, and `resourcecollect` define/discover
  resources and safe filesystem paths.
- `portableconfig` projects portable fields; `portablemerge`, `state`, and
  `conflict` support three-way merging and conflict persistence.
- `auth` and `secret` handle credentials and filtering; `installplan` owns
  approval/execution; `gitclient` owns Git process interaction.
- Prefer existing injected runners, clocks, homes, and platform values in tests.
  Do not run against the developer's actual agent directories, keyring, or remote.

## Invariants to preserve

- Scan/project before staging or restoring; preserve machine-local configuration
  and credentials. Do not broaden include rules to make a failing test pass.
- Validate repository-relative paths, restore targets, and symlink handling.
  Cover traversal and out-of-root paths when changing file operations.
- Preserve unresolved conflict data and deletion protection. Test both local
  and remote changes, retries, and errors around partial work.
- Installation declarations are not permission to execute. Retain explicit
  approval, argument-vector execution, and actionable retry/error states.
- Keep cancellation, scheduler state, and UI progress consistent with actual
  completion; do not hide failed I/O or publish success before cleanup completes.

## Application cleanup is not repository maintenance

`daemon.New` wires a scheduled sync job to `cli.RunCleanup`, which calls
`syncengine.CleanupTrash`. After a sync without an error, expired soft-deleted
entries are purged using `TrashGraceDays` (30 days when nonpositive), and changes
may be committed/pushed to the user's sync repository. Sync errors skip cleanup;
cleanup errors propagate through the daemon result. A needs-attention result
without a sync error still runs cleanup.

Preserve this grace-window/error behavior. Never call this path from development
setup, hooks, generic maintenance, or CI against user data. The development
`cleanup` command is separate and only formats repository Go files.

## Validation

Run affected packages together, for example:
`go test ./internal/syncengine ./internal/cli ./internal/daemon`.
Add regression tests beside the code; cover rejected inputs as well as success.
Run `node scripts/dev.mjs check` and `go test ./...` before handoff for Go changes.
If Wails contracts change, regenerate bindings through `build/Taskfile.yml` and
run the frontend typecheck, tests, and build too. Keep OS-specific code/tests in
the existing platform files; report platforms not validated locally.
