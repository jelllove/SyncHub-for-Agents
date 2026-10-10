# SyncHub command-line interface

## Goals and scope

The medium-scope CLI keeps the existing Desktop UI and uses the same
configuration, resource policies and synchronization engine. It adds a standalone
headless entry point for terminals, scripts and AI tools. Desktop/CLI coexistence
uses explicit busy errors instead of a local Desktop control protocol.

## Build, install and quick start

From the repository root, with the Go version in [go.mod](../go.mod):

```powershell
$env:CGO_ENABLED = "0"
go build -o .artifacts\cli\synchub.exe ./cmd/synchub
.\.artifacts\cli\synchub.exe --help
.\.artifacts\cli\synchub.exe version --json
```

On macOS/Linux, use `CGO_ENABLED=0 go build -o .artifacts/cli/synchub ./cmd/synchub`.
The CLI does not require Wails, WebView2, GTK, WebKitGTK or Node. Install Git
separately. Existing Desktop OAuth authentication also requires the operating
system credential store; new CLI initialization uses existing Git/SSH credentials
instead of introducing a login wizard.

Starting with v0.3.6, releases publish these separate archives:

| Platform | Archive |
| --- | --- |
| Windows x64 | `SyncHub-CLI-windows-x64.zip` |
| macOS Apple Silicon | `SyncHub-CLI-macos-arm64.tar.gz` |
| macOS Intel x64 | `SyncHub-CLI-macos-x64.tar.gz` |
| Linux x64 | `SyncHub-CLI-linux-x64.tar.gz` |

Verify the archive using the release's `SHA256SUMS.txt`, extract `synchub-cli`,
and put that directory on your user `PATH`, or invoke the executable by its full
path. This does not register startup tasks, create GUI shortcuts or modify your
agent resources. CLI packages are unsigned/not notarized; Desktop signing does
not imply CLI signing. Existing v0.3.5 assets and the initial NPM desktop wrapper
do not include this new headless executable.

With the extracted executable on `PATH`:

```text
synchub version
synchub init --repo git@github.com:example-user/private-agent-sync.git --first-sync merge-cloud-local
synchub doctor --json
synchub plan --json
synchub sync --json
synchub status --json
synchub daemon
```

For an existing Desktop profile, omit `init` and quit Desktop from its tray before
inspecting or synchronizing. Do not run `init` to change an already configured
repository. `--agents claude,copilot` restricts initial enabled agents; omitting
the flag enables all known agents. Configure categories/custom resources using
Desktop or the existing [configuration format](install.md).

## Command contract

The intended executable is `synchub` (`synchub.exe` on Windows). Commands are:

- `init --repo <URL> --first-sync <strategy>`: configure through existing Git/SSH
  authentication, with an explicit first-sync strategy and no reinitialization of
  an existing profile.
- `status`: report configured agents, last recorded state and pending actions.
- `doctor`: check configuration, resource declarations, Git and local repository
  availability without repairing anything or contacting the network.
- `plan`: show the guarded plan against the existing local clone, without fetching,
  applying resource changes, committing, pushing or approving installations.
- `sync`: run one synchronization using existing safety and approval rules.
- `daemon`: run headlessly until interrupted, retaining existing scheduled
  synchronization and trash-retention behavior.

Finite commands support `--json` with a versioned result/error envelope and
documented nonzero exit codes. `--home` selects an explicit SyncHub profile;
omitting it uses the Desktop profile at `~/.synchub`. Different profiles must not
share a synchronization repository or overlapping resource targets.

First-sync choices are `merge-cloud-local`, `use-cloud` and `use-local`.
The latter two can plan deletions or replacements to favor one side; inspect
`plan` before execution. The initial choice is recorded once and the existing
engine marks it complete after a successful sync.

`plan` is a local estimate, not a promise about later execution: remote changes
are not fetched, and conflict recovery/merge/install execution can change the
outcome. Use Desktop to select an initial strategy if its onboarding left that
choice pending. `status.lastSync` reflects the state-file modification time, not
a live query of Desktop or an independently verified last successful upload.

## JSON and exit codes

Successful finite commands emit one JSON object:

```json
{"schemaVersion":1,"ok":true,"data":{"version":"0.3.6"}}
```

Errors emit `ok:false` and an `error` with `code` and `message`. Attention-required
results also retain their summary in `data`. Output never includes synchronized
file bodies or raw engine result objects. It can include repository URLs,
resource paths and diagnostic messages; review/redact those before sharing logs.

| Exit | Meaning | Error code |
| --- | --- | --- |
| 0 | Command completed; no attention required | None |
| 1 | Runtime, profile I/O, Git or output failure | `runtime_error` |
| 2 | Invalid command/flags, URL or first-sync option | `invalid_input` |
| 3 | Another cooperating process owns the profile | `busy` |
| 4 | Failed diagnostic checks, blocked preview, conflicts or pending approval | `needs_attention` |

`daemon` is long-running, not a finite JSON command; `daemon --json` is rejected.
Its cycle errors and state are recorded in the profile's `logs/daemon.log`.
Stopping it does not erase prior errors or mean that every scheduled cycle passed.
`--help`/`--version` use Cobra's text output; use `version --json` for JSON metadata.

## Interactions and safety

Desktop and headless daemon hold a shared OS-backed profile ownership lock for
their lifetime. Finite CLI commands acquire that same lock before reading
consistent state, scanning or mutating profile data. Contention reports busy and
does not interrupt the owner. Quit Desktop from its tray, not just its window,
before using commands that need profile ownership.

The lock is released by the operating system after process exit, including an
unexpected crash; a leftover lock file is not itself evidence of contention.
Do not delete a lock file to bypass ownership. Locks do not retroactively protect
older versions of SyncHub that do not participate in this protocol.

Resource credential exclusion, path containment, conflict persistence and
explicit installation approval stay in the existing backend. JSON output must
not contain file contents, credentials or full internal synchronization results.
Planning may create and clean isolated temporary staging, but cannot change
configuration, agent resources or synchronization state.

## Architecture and execution

The headless command entry point composes `internal/cli`, `internal/daemon` and
the shared profile-lock package. It must not import Wails, WebView or Fyne tray
dependencies. Desktop keeps its existing executable and UI, acquiring ownership
after Wails single-instance handling and before backend initialization.

Inspection reuses the shared resource planner and first-sync mapping rather than
implementing a different synchronization policy. Headless packages are built
separately for Windows x64, macOS ARM64/Intel and Linux x64, with version metadata
and checksums. This change does not authorize a release, NPM publication or MCP.

The hidden credential-helper protocol is deliberately outside the profile lock:
Git invokes it as a child while its parent owns the profile. OAuth Git commands
pass the selected profile through a scoped helper environment, so `--home`
does not accidentally read credentials from the default profile.

Default builds exclude legacy Fyne tray dependencies. The old CLI
`tray`/`install`/`uninstall` commands remain available only in an explicit
`go build -tags legacytray ./cmd/synchub` build. Default headless binaries return
a clear error for those commands. Use the existing Desktop app for its tray and
start-at-login settings; the optional legacy build is not the headless package.
Legacy tray honors `--home`, but legacy autostart only supports the default profile
and rejects `--home` rather than registering the wrong profile at login.

## Edge cases and non-goals

- Missing or malformed configuration, unknown agents/strategies, lock contention
  and I/O failures are explicit errors.
- First initialization uses existing Git/SSH credentials; no new OAuth login
  wizard or credential-management interface is introduced.
- Conflicts and pending approvals are reported as attention required, not complete
  success. Complex conflict editing stays in Desktop.
- Daemon interruption stops scheduling and waits for the active cycle under the
  existing scheduler behavior; it does not force-kill Git mid-write.
- Finite operations also finish their active work after a normal interrupt;
  this version does not introduce per-command timeouts or forceful cancellation.
  Failed initialization can leave a partial clone; inspect it manually rather
  than bypassing the nonempty-repository guard.
- No Desktop RPC, MCP tools, interactive TUI, automatic repair or GUI redesign.

## Acceptance criteria and validation

1. Desktop UI and existing synchronization behavior pass repository regression tests.
2. Two cooperating processes cannot own the same profile; failed acquisition does
   not modify configuration or resources. Process death releases ownership.
3. CLI help/version work without accessing user data or initializing any GUI.
4. JSON output is deterministic in shape; busy, invalid input, runtime errors and
   attention-required results have documented distinguishable exits.
5. Initialization refuses existing profiles and records an explicit first-sync
   policy. Plan uses that policy but does not execute it.
6. Headless builds have no GUI dependency and cover the specified OS/CPU matrix.
7. Tests use isolated homes, synthetic resources and mocked external checks.

Run targeted Go tests during development, then `node scripts/dev.mjs verify` from
the repository root. Cross-compilation alone is not native installation evidence;
report unexercised host behavior and preserve failed validation logs.

To build one versioned package from the repository root:

```text
go run ./tools/clipack --os windows --arch amd64 --version 0.0.0 --out .artifacts/cli-package
```

Use `darwin/arm64`, `darwin/amd64` or `linux/amd64` for the other targets. The
packager refuses existing output names, sets `CGO_ENABLED=0`, verifies executable
build metadata, and smokes the version only when the target matches the host.
Each archive contains only the executable, license and this guide. Retain the
per-target checksum manifest and build receipt; local dirty builds are not
immutable-release evidence.

[Native CLI CI](../.github/workflows/cli.yml) runs command/ownership tests and
packaging on Windows, both macOS architectures and Ubuntu without GUI libraries.
Both [CI](../.github/workflows/ci.yml) and the
[release workflow](../.github/workflows/release.yml) reuse it. Release publication
requires all four CLI jobs and verifies their manifests before creating combined
checksums. Workflow configuration alone is not evidence of a successful hosted
run; cross-compilation on Windows does not prove native macOS/Linux behavior.
