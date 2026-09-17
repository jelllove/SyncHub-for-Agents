# Development and repository maintenance

## Reproducible setup

Install Git, the Go version declared in [go.mod](../go.mod), and the exact Node
version in [.node-version](../.node-version). Put Go's `bin` directory on `PATH`
(typically `C:\Program Files\Go\bin` on Windows). Run from the repository root:

```powershell
node scripts/dev.mjs setup
node scripts/dev.mjs verify
```

Setup restores Go modules and the npm lockfile and builds frontend assets required
by Go's embed directive. It does not install global tools, change Git configuration,
start a daemon, or access a user's synchronization repository.
For a pinned Linux environment, use the [development container](devcontainer.md).

For desktop development, install the Wails CLI at the version in the generated
reference below using `go install github.com/wailsapp/wails/v3/cmd/wails3@<version>`
and put `$(go env GOPATH)/bin` on `PATH`. Run `wails3 dev`.
Windows uses WebView2; installer packaging also needs NSIS.
macOS requires Xcode command-line tools. Ubuntu 24.04 needs
`gcc libgtk-4-dev libwebkitgtk-6.0-dev` for Go desktop checks; install these through
your OS package manager before setup/check. Packaging dependencies are listed in
[CI](../.github/workflows/ci.yml).

## Local feedback and cleanup

The shared check uses gofmt, go vet, ESLint, and TypeScript. It checks
all repository-owned Go files and all packages, not just the staged diff.
Checks report command output on failure and do not rewrite source files.
Go formatting is computed on normalized temporary copies, in batches of at most
128 files per native `gofmt` process. Temporary files are removed on success or
failure; diagnostics name the original source paths. Files must be complete Go
source files, not the standalone fragments accepted by `gofmt` on stdin.
Explicit formatting validates every input before writing changes back, rechecks
the source before applying, and preserves existing LF/CRLF conventions.
It is not an atomic multi-file transaction; inspect the diff if a write fails.
The frontend [ESLint configuration](../frontend/eslint.config.mjs) uses a
syntax parser compatible with the current code; `tsc` remains responsible for
TypeScript semantic checks. Lint configuration regressions run separately with
`npm --prefix frontend run test:lint`; they do not replace the UI/tooling test suite.

Enable the hook for an individual commit without changing configuration shared
by other worktrees:

```powershell
git -c core.hooksPath=.githooks commit
```

The hook validates the working tree, not an isolated index snapshot. Do not
partially stage a file and assume the staged version was validated. PR CI is the
authoritative clean-checkout check. Dependency installation belongs to setup,
not to the hook.

`node scripts/dev.mjs cleanup` (also available as `format`) performs one gofmt
repair pass over Git-listed Go files. It includes non-ignored untracked files,
rejects symlinks, never deletes files, and never stages, commits, or pushes.
Each subprocess has a ten-minute timeout. Review the resulting diff.
The weekly/manual [maintenance workflow](../.github/workflows/maintenance.yml)
audits the same checks without applying repairs or opening issues or PRs.
There is deliberately no unattended source modification.

This is separate from application trash cleanup. The existing daemon wires
`cli.RunCleanup` into its scheduled sync job, which calls
`syncengine.CleanupTrash` with the configured recovery window, commits the purge,
and pushes with bounded retries. See [daemon wiring](../internal/daemon/daemon.go)
and [cleanup driver](../internal/cli/cleanup.go).
Repository maintenance must never invoke that driver against real user data.
Its schedule, pause behavior, retention policy, and safety filters are unchanged.

## Validation reports and the maintenance loop

`node scripts/dev.mjs verify` builds frontend assets first, runs the same local
`check`, then the full current-host Go suite (including workflow and architecture
guards), lint-configuration regressions, and all frontend/UI/tooling tests.
It does not install dependencies; run setup first.
The existing Linux CI job also runs the Go suite with `-race`.
Use `check` for quick feedback and `docs` for documentation-only changes.

Each verification creates a new `.artifacts/validation/run-*` directory containing
`validation.json` and one log per check. The JSON records the actual commit,
dirty-worktree flag, platform, command arguments, status, and elapsed time.
It also captures SHA-256 source snapshots before and after the check commands.
The fingerprint includes Git-listed tracked and non-ignored untracked files,
raw bytes (including binary files), paths, and file permissions. Tracked deletions
are represented explicitly. Ignored dependencies, build output, and reports are
not included; linked or otherwise unsupported source paths fail closed.

Two dirty worktrees at the same commit can therefore be distinguished. If a
source file or the Git revision changes between the snapshots, the overall
verification fails even when every command exits successfully. The report's
`source` section lists changed paths or capture errors. A failed initial snapshot
leaves checks `not-run` rather than executing against unidentified inputs.
Consumers must check the overall `status`, not just individual command results.

Snapshots use a fixed-size streaming buffer and do not store source contents in
the report. They are endpoint observations, not a filesystem lock or an atomic
snapshot: changes made and reverted between captures may go undetected. They do
not attest installed dependencies, external services, or another operating system.
Avoid editing the checkout during verification and rerun after any source change.

Older reports are preserved and outputs are ignored by Git. A failed command
does not prevent the independent checks from running, but the overall command
still exits nonzero. Timeouts fail rather than being silently retried.
Interrupted reports remain `running`, not `passed`.
The Go log contains `go test -json` events, including skipped platform-dependent
tests; successful stderr diagnostics are retained alongside stdout.
Completed runs also contain `junit.xml` and a self-contained `index.html`.
JUnit cases describe validation command groups plus source integrity, not the
number of individual application tests. Source capture errors and observed
changes remain failures even when the underlying commands passed. The HTML report
links the original logs and JSON; it does not load scripts or remote resources.
Native-process integration tests use a 20-second test budget for Git/Go startup
on busy hosts. Their explicit child-command timeout assertions remain unchanged;
ordinary UI tests retain the default timeout.

The PR workflow and weekly/manual maintenance audit share this runner, append
results to the GitHub job summary, and upload logs/JSON even on failure. Artifacts
expire after 14 days in GitHub; local reports are retained until explicitly removed.
Review failure logs, make a scoped fix with a regression test, then re-run the
same command. Include the failing/passing run links in the PR description.
This is a human-reviewed improvement loop, not unattended self-healing.
Reports contain command output: use synthetic test data and do not log secrets.
No issues or PRs are opened automatically.

## Review-only repair proposals

When recorded validation fails, CI makes one attempt to prepare a proposal with
`node scripts/dev.mjs propose`. It can only fix Go formatting and regenerate the
marked command/version reference below. It does not change dependencies, edit
application logic, or attempt to repair arbitrary test failures.

The command creates `.artifacts/maintenance/run-*` with `proposal.json` and, when
applicable, `repair.patch`. A proposal is limited to 50 changed files and 1 MiB of
patch data. It verifies application and reversal in an isolated snapshot and checks
canonical output before publishing the patch. Source files and the real Git index
are never modified; per-file before/after SHA-256 digests identify the proposed
change, including when the working tree is already dirty.

`proposed` means only that this limited repair is applicable and canonical, not
that the full test suite has passed. `no-changes` means no supported repair was
needed; it does not resolve the original failure. `failed` and `running` are not
approved patches. CI retains the original failed verdict and uploads a proposal
only from the safely created output directory. Both validation and proposal
artifacts expire after 14 days.

Formatting and reference regeneration require valid UTF-8. Invalid byte sequences
produce an explicit error rather than being silently replaced. Convert a file's
encoding deliberately before retrying; the tools do not guess another encoding.

After downloading and inspecting a report with status `proposed`, a maintainer
may explicitly apply its patch and verify the result:

```powershell
git apply --check path-to-repair.patch
git apply path-to-repair.patch
node scripts/dev.mjs verify
```

If that exact patch needs to be reverted, first check for conflicting intervening
edits; never reset the whole worktree:

```powershell
git apply --reverse --check path-to-repair.patch
git apply --reverse path-to-repair.patch
```

Keep the failing run, proposal, regression test, and passing run linked in the PR.
This review-first backstop is not an autonomous production rollback system.

## Contained native repair verification

With Docker running Linux containers and the intended source changes committed:

```powershell
node scripts/dev.mjs repair:verify
```

The command builds the development image and runs a diagnostic proof with the
source mounted read-only. The container is non-root, has a read-only root
filesystem, no added capabilities, no Docker socket or host-home mounts, a
512-process limit, two CPUs, 6 GiB memory, and a 4 GiB temporary filesystem.
Only the new report directory is writable on the host. Named containers are
removed on completion/failure; development image layers remain reusable.
Container-local Git trust is restricted to `/source` and `/source/.git`, which
allows a non-root worker to read Windows bind mounts without changing host Git
configuration or trusting arbitrary repositories.
Image construction and contained verification each have a 20-minute deadline;
the CI job has a 45-minute ceiling. Registry/network failures remain failures,
and TLS, checksum, or package-signature verification must not be disabled.

The worker clones the committed source into a disposable workspace, restores
locked dependencies, and proves this sequence using native project commands:

1. The unmodified baseline passes the full validation suite.
2. A deliberately injected Go-formatting fault fails the normal check.
3. The bounded proposal repairs only that diagnostic file; the full suite passes.
4. Reversing that exact patch restores the failing state and its source digest.

The original checkout must remain unchanged. Results under
`.artifacts/repair-verification/` include the container image identity, resource
limits, native validation receipts/logs, patch, and before/after/rollback digests.
The [repair workflow](../.github/workflows/repair-verification.yml) runs this proof
for PRs and retains artifacts for 14 days without repository write permissions.

This is explicitly a **diagnostic fault-injection scenario**, not a production
incident, an automatic fix to a PR, or proof of recurring autonomous operation.
Uncommitted changes are refused so the source revision is unambiguous; untracked
local tools are not copied into the committed verification workspace.

## Architecture and platform coverage

The [architecture guide](architecture.md) documents the actual module roles and
the small set of enforced core dependency boundaries. The guard parses production
imports across build tags, checks error paths with fixtures, and runs as part of
the Go suite. It does not replace Go compilation, behavioral tests, or review.

Shared-alias integration tests probe the host's actual symlink capability. On a
Windows account without symlink privileges they assert the supported managed-copy
fallback instead of skipping the scenario or incorrectly requiring a link.
An injected successful linker separately checks the success contract; hosts with
symlink support exercise the real-link integration. This does not grant privileges
or change the application's alias policy.

## Documentation drift and CI enforcement

`node scripts/dev.mjs docs` deterministically checks local Markdown link targets
across all Git-listed Markdown files, including historical specs, plus exact
agreement of the generated reference with the Go/Node versions and npm scripts.
External URLs, heading fragments, HTML links, and semantic prose claims are not
validated. A passing check is not proof that all documentation matches runtime
behavior; maintainers must review behavioral documentation with code changes.

Run `node scripts/dev.mjs docs:write` after changing tool versions or npm scripts.
Only the marked section below is regenerated; hand-written guidance is preserved.
CI can propose a reference repair after failed validation, but never applies it
to the checkout or changes a failed check to success.

The `Repository checks` job runs on every PR, without path filters or
`continue-on-error`. It includes tooling regression tests, frontend tests/build,
and the documentation check, with downloadable results on success or failure.
The existing Go test/package jobs remain.
`Security checks` runs dependency vulnerability checks for npm and Go.
The checks execute locally/in GitHub runners; source is not sent to an external
AI reviewer. Weekly Dependabot updates require normal review and are not merged
automatically.

After integration, a repository administrator must configure branch protection
or a ruleset to require `Repository checks`, `Security checks`, and the existing
test/package checks. This worktree does not change remote repository settings.
Until that is done, a failing workflow alone does not guarantee merges are blocked.

<!-- dev-reference:start -->
## Generated tool and command reference

Regenerate with `node scripts/dev.mjs docs:write`; CI rejects stale content.

- Go minimum: `1.26.6` (from `go.mod`).
- Node.js: `24.17.0` (from `.node-version`).
- Wails CLI: `v3.0.0-beta.8` (from `go.mod`).

| Repository command | Purpose |
| --- | --- |
| `node scripts/dev.mjs setup` | Restore locked dependencies and build embedded frontend assets. |
| `node scripts/dev.mjs check` | Check Go formatting/vet, frontend ESLint/TypeScript, and documentation without rewriting files. |
| `node scripts/dev.mjs verify` | Build frontend assets, run shared checks and all Go/frontend tests, and save results and logs. |
| `node scripts/dev.mjs propose` | Prepare a bounded, review-only Go formatting/reference patch; never apply or commit it. |
| `node scripts/dev.mjs repair:verify` | Verify a diagnostic failure/repair/rollback cycle in a restricted Linux container. |
| `node scripts/dev.mjs format` | Apply gofmt to repository-owned Go files only; never stage or commit. |
| `node scripts/dev.mjs cleanup` | Run one bounded formatting repair pass; never delete files or touch sync data. |
| `node scripts/dev.mjs docs` | Check local Markdown file links and the generated development reference. |
| `node scripts/dev.mjs docs:write` | Refresh only the generated development reference. |

| Frontend command | Executed script |
| --- | --- |
| `npm --prefix frontend run dev` | `vite` |
| `npm --prefix frontend run build:dev` | `tsc && vite build --minify false --mode development` |
| `npm --prefix frontend run build` | `tsc && vite build --mode production` |
| `npm --prefix frontend run generate:icons` | `node scripts/generate-icons.mjs` |
| `npm --prefix frontend run preview` | `vite preview` |
| `npm --prefix frontend run lint` | `eslint . --max-warnings 0` |
| `npm --prefix frontend run typecheck` | `tsc --noEmit` |
| `npm --prefix frontend run test` | `vitest run` |
| `npm --prefix frontend run test:lint` | `node --test lint.test.mjs` |
<!-- dev-reference:end -->
