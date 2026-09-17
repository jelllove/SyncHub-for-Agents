# Backend architecture and dependency boundaries

SyncHub is a Wails desktop/tray application and CLI that synchronizes portable
agent resources through a user-owned Git repository. The Go module remains
`github.com/qinqingxu/synchub-for-agents`; its spelling intentionally differs
from the GitHub repository owner.

## Package roles

All package names below are under `internal/` unless stated otherwise.

| Area | Packages | Responsibility |
| --- | --- | --- |
| Foundations | `resource`, `config`, `secret`, `state`, `pathresolver` | Resource models and filtering; persisted user settings; path/content exclusion; sync snapshots and merge bases; filesystem path resolution and containment. `secret` filters resource content, while authentication credentials belong to `auth`. |
| Resource preparation | `provider`, `resourcecollect`, `portableconfig` | Load provider/resource definitions, discover and stage safe resources, and project portable configuration fields through codecs/policies. |
| Reconciliation and application | `portablemerge`, `conflict`, `installplan`, `syncengine` | Merge portable content, persist/resolve conflicts, require explicit installation approval, and execute synchronization, restore, deletion, and trash cleanup. |
| Repository access | `gitclient`, `repository` | Run Git operations and validate/prepare the configured synchronization repository. |
| Application composition | `cli`, `daemon`, `desktop`, `settings`, `tray` | Assemble backend dependencies and resource specifications; wire scheduled sync/cleanup; expose Wails services/UI models, HTTP settings, and the native tray. `cli` is shared application orchestration, not just command-line argument parsing. |
| Other support | `auth`, `onboarding`, `scheduler`, `processattr`, `autostart`, `startup`, `updater`, `sshprobe`, `appversion` | Authentication/onboarding, reusable scheduling, process/platform integration, startup/update behavior, SSH probing, and version metadata. These packages are not classified by the two guards below. |

Root `main.go`/`app.go` compose the desktop lifecycle; `cmd/` contains command
entry points. The React frontend consumes the desktop service boundary through
generated Wails bindings. These entry points and frontend code are outside the
import scan.

## Two enforced directions

[The architecture guard](../tools/archcheck/boundaries_test.go) groups the current
packages by the responsibilities above, rather than recording every currently
allowed dependency.

1. **`foundations-below-sync-domain`**: `config`, `pathresolver`, `resource`,
   `secret`, and `state` must not import `conflict`, `gitclient`, `installplan`,
   `portableconfig`, `portablemerge`, `provider`, `repository`, `resourcecollect`,
   or `syncengine`. Resource/settings/snapshot abstractions should not acquire
   the workflows that consume them.
2. **`core-independent-of-composition`**: those five foundations and nine
   sync-domain/repository packages must not import `cli`, `daemon`, `desktop`,
   `settings`, or `tray`. Synchronization and safety policies should remain
   reusable without depending on an application host or its wiring.

Both source and destination rules include subpackages, using path-segment
matching: `internal/desktop/models` is covered, but `internal/client` is not
mistaken for `internal/cli`.

These directions reflect existing production dependencies:

- [Configuration](../internal/config/config.go) uses `resource` category and
  strategy types. Imports within the foundation group are allowed.
- `resourcecollect` uses `resource`, `secret`, and `state`; `portablemerge` uses
  `portableconfig`. Sync-domain packages may use foundations and one another.
- [The engine](../internal/syncengine/engine.go) consumes Git, conflict,
  installation, collection, codec, and state services. The guard preserves
  these existing dependencies instead of demanding a production refactor.
- [The daemon](../internal/daemon/daemon.go) intentionally imports `cli` to wire
  `RunSyncWithProgress` and `RunCleanup` into the reusable `scheduler`.
  `desktop` and `tray` also compose other application packages. These are
  allowed directions, not exceptions to an otherwise frozen import graph.

For runtime safety requirements, see the
[portable-resource rules](portable-resources.md) and
[backend guidance](../internal/AGENTS.md). An import check does not verify those
behavioral invariants.

## Planning versus execution

[Resource planning](../internal/syncengine/planning.go) is a pure stage shared by
[CLI status](../internal/cli/status.go) and the
[sync engine](../internal/syncengine/engine.go). It prepares independent snapshot
views, filters the stored base by resource ownership, and excludes skipped
resource prefixes using path-segment boundaries. It also
protects internal metadata from accidental restoration, and retains blocked
remote paths for removal. It does not mutate the caller's snapshots or perform I/O.

The engine applies explicit first-sync choices only when `FirstSyncRun` is true,
then executes the resulting actions through the existing applier. Pull/push retry
handling, conflict persistence, installation approval, state updates, and progress
events remain execution responsibilities. Status uses the common plan against
its current snapshots; it is not a guarantee about later network changes or
execution outcomes.

[Planning regression tests](../internal/syncengine/planning_test.go) characterize
action ordering, blocked/skipped precedence, input preservation, first-sync
mapping, and real-Git first-run gating. Keep those policies in the common planner
rather than adding a second copy to status or execution code.

## Running the check and understanding its limits

From the repository root, with the Go minimum in [go.mod](../go.mod):

```text
go test ./tools/archcheck -count=1
go test ./tools/...
```

The test uses only the Go standard library. It walks `internal/` and parses
imports in every `.go` file except `_test.go`, without evaluating build tags or
selecting the current operating system. Integration tests may import application
wiring from domain test packages; their imports are deliberately excluded.
The check fails on file/directory I/O errors, malformed import declarations, or
an empty source scan. Boundary errors identify the source file, import location,
violated rule, and forbidden import.

In-memory synthetic fixtures exercise forbidden imports, real allowed edges,
source/destination subpackages, lookalike names, tagged files, aliased/raw-string
imports, integration-test exclusion, and parse/read/walk failures. They neither
compile the synthetic imports nor use real agent homes, repositories, or
credentials.

This is a small **direct-import** guard, not complete architectural verification:

- It does not type-check source bodies, compile every platform, validate runtime
  behavior, or replace the normal Go tests/build.
- Third-party imports, unclassified packages, and dependencies within a group
  are not restricted. Indirect paths through unclassified packages are not
  analyzed.
- A new top-level package requires a deliberate classification decision if it
  should be protected; descendants of classified packages inherit the rules.
  Harmless growth does not require maintaining an exhaustive allowed-import
  list.
