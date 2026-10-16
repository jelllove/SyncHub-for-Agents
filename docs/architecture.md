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
| Reconciliation and application | `portablemerge`, `conflict`, `installplan`, `sharedlibrary`, `syncengine` | Merge portable content, persist/resolve conflicts, require explicit installation approval, manage approved shared-source projections, and execute synchronization, restore, deletion, and trash cleanup. |
| Repository access | `gitclient`, `repository` | Run Git operations and validate/prepare the configured synchronization repository. |
| Application composition | `cli`, `daemon`, `desktop`, `settings`, `tray` | Assemble backend dependencies and resource specifications; wire scheduled sync/cleanup; expose Wails services/UI models, HTTP settings, and the native tray. `cli` is shared application orchestration, not just command-line argument parsing. |
| Other support | `auth`, `onboarding`, `scheduler`, `processattr`, `autostart`, `startup`, `updater`, `sshprobe`, `appversion` | Authentication/onboarding, reusable scheduling, process/platform integration, startup/update behavior, SSH probing, and version metadata. These packages are not classified by the two guards below. |

Root `main.go`/`app.go` compose the desktop lifecycle; `cmd/` contains command
entry points. The React frontend consumes the desktop service boundary through
generated Wails bindings. These entry points and frontend code are outside the
import scan.

The [CLI contract](cli.md) describes the standalone headless entry point and
[OS-backed profile ownership](../internal/instance/lock.go) shared by Desktop and
CLI. Desktop acquires ownership before backend initialization; CLI holds it for
each finite operation or the daemon lifetime. Busy profiles are rejected instead
of running competing synchronization or settings changes. Different profiles
must not share a clone or overlapping resource targets. The lock file stays in
place after release; the OS lock, not file existence, determines ownership.

The desktop executable uses Wails' native tray. Shared tray icons remain in
`internal/tray`; the legacy `synchub tray` backend is isolated in
`internal/tray/legacy` and requires an explicit `legacytray` CLI build tag.
Default CLI builds exclude both Wails and Fyne so they work without GUI libraries.
Keeping the Fyne tray backend out of the desktop dependency graph prevents
colliding Objective-C menu symbols on macOS. A dependency-graph test guards this
separation; the default headless package does not replace Desktop tray behavior.

Activity feedback is also split at this boundary. The framework-independent
[tray feedback controller](../internal/tray/feedback.go) maps stages and terminal
outcomes, deduplicates tips, handles readiness/dismissal and cancels stale timers.
The [desktop adapter](../internal/tray/desktop/notifier.go) alone imports Wails and
positions the Windows popup through its public system-tray API. It uses native
image-resource PNG data; the legacy backend retains its ICO packaging.
This keeps Wails out of the Fyne legacy executable's dependency graph too.

[Tray tip preferences](../internal/tray/preferences.go) persist the local popup
opt-out independently of synchronized resources. Both the sender-scoped popup
disable event and generated Desktop settings APIs update this same preference;
failed persistence leaves the current value unchanged. The notifier immediately
hides disabled tips while preserving tray icon/tooltip updates. Reenable waits
for future state changes rather than replaying a stale tip. The feedback
controller owns bounded active and terminal timers; the Windows adapter alone
uses the public native window handle for a non-activating show.

[Terminal cycle observers](../internal/desktop/cycle_observer.go) publish after
summary persistence and outside the service lock, including error cycles while
paused that have no completion-progress event. Native tray callbacks consume
this outcome rather than inferring success from the last progress stage.
The [tip view](../frontend/src/TrayTip.tsx) receives a minimal, revisioned status
event and can only request opening the main window or dismissing the tip. The
adapter validates the sending window name, cleans up events/timers at shutdown,
and does not expose raw repository/file/error data in the popup.

## Desktop interface and language

[The language store](../frontend/src/i18n.ts) uses React's external-store
subscription API so English/Simplified Chinese changes update mounted views
without resetting forms, approved previews, or onboarding state. Explicit
bilingual UI strings remain adjacent to their controls; both translations are
required by the typed translator. Backend request identifiers and user-provided
content are never translated. Known status codes have presentation-only labels.

[The language selector](../frontend/src/LanguageSwitcher.tsx) persists the
`en` or `zh-CN` preference under the WebView-local `synchub.ui.language` key.
It defaults to Simplified Chinese for Chinese browser/system languages and to
English otherwise. Storage events update other same-origin WebViews when their
host supports these events. Storage read/write failures are logged and displayed
with an explicit retry; a failed save still permits session-only use.
This preference is not part of Git synchronization or the CLI configuration.
Native tray menus, OS dialogs, and backend diagnostics retain their original
language; the React desktop views and Windows activity-tip controls are bilingual.

[Settings illustrations](../frontend/src/SettingsDiagram.tsx) are local SVG/CSS
diagrams with translated text alternatives, not remote images or live progress.
[The shared-library panel](../frontend/src/SharedLibraryPanel.tsx) separates adding
sources from using them in compatible destinations and reveals recovery controls
only when managed outputs exist. Its existing generation counter and exact
preview-ID approval remain the authority for preventing stale UI writes.
Language changes do not issue backend calls or alter approval validity.

## Two enforced directions

[The architecture guard](../tools/archcheck/boundaries_test.go) groups the current
packages by the responsibilities above, rather than recording every currently
allowed dependency.

1. **`foundations-below-sync-domain`**: `config`, `pathresolver`, `resource`,
   `secret`, and `state` must not import `conflict`, `gitclient`, `installplan`,
   `portableconfig`, `portablemerge`, `provider`, `repository`, `resourcecollect`,
   `sharedlibrary`, or `syncengine`. Resource/settings/snapshot abstractions should not acquire
   the workflows that consume them.
2. **`core-independent-of-composition`**: those five foundations and ten
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

The [shared library](shared-library.md) keeps canonical sources separate from
Harness output directories. Local ownership receipts and backups never enter
Git. A user-home OS lock serializes explicit library operations and sync; source
sync uses the existing engine, followed by drift-checked refresh of approved
outputs after clean cycles. Reserved resource paths are excluded from both
collection and remote ownership/application, including legacy instruction paths.
Whole-directory alias creation is disabled for the affected common skills pool.

## Planning versus execution

[Resource planning](../internal/syncengine/planning.go) is a pure stage shared by
[CLI status](../internal/cli/status.go) and the
[sync engine](../internal/syncengine/engine.go). It prepares independent snapshot
views, filters the stored base by resource ownership, and excludes skipped
resource prefixes using path-segment boundaries. It also
protects internal metadata from accidental restoration, and retains blocked
remote paths for removal. It does not mutate the caller's snapshots or perform I/O.

Local status/CLI plans apply the same explicit first-sync mapping to the prepared
plan when configuration says it is pending. The planner includes protected
metadata and blocked/skipped paths before mapping, as execution does.
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
