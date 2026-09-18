# Contributing

Keep changes focused and explain the user-visible outcome. For security concerns,
use [private reporting](SECURITY.md), not a public issue or pull request.

## Development

Use the Go minimum in [go.mod](go.mod) and Node 24 (at least 24.13.0), with npm and Git available.
Select the exact Node version in [.node-version](.node-version) before setup;
the setup command enforces that pin. From the repository root:

```text
node scripts/dev.mjs setup
node scripts/dev.mjs check
go test ./...
npm --prefix frontend test
npm --prefix frontend run build
```

Setup runs `npm ci`, `go mod download`, and the frontend build; it does not
install global tools or change Git configuration. The frontend build supplies
assets embedded by Go. Native GUI dependencies vary by OS. Desktop development
optionally requires the Wails CLI pinned to `v3.0.0-beta.8`; follow the
[development reference](docs/development.md) for installation and platform
prerequisites rather than installing an unpinned latest version.

Read [repository guidance](AGENTS.md) and the scoped
[backend](internal/AGENTS.md)/[frontend](frontend/AGENTS.md) guidance. The Go
module path intentionally remains `github.com/qinqingxu/synchub-for-agents`.
Regenerate Wails bindings from Go changes; do not hand-edit `frontend/bindings`.

## Validation and review

- Add regression tests for behavior changes; prefer targeted existing tests
  during development and run the commands above before submitting code changes.
- `check` runs Go formatting checks, `go vet`, frontend lint/typechecking, and docs
  drift checks. `node scripts/dev.mjs format` repairs Go formatting only.
- For documentation-only changes, run `node scripts/dev.mjs docs`. It checks
  local Markdown link target existence and generated tool versions/commands,
  not full semantic correctness. Verify prose against code yourself.
- Use `node scripts/dev.mjs docs:write` to regenerate only the development
  command reference after tool/script changes; review the resulting diff.
- State what was tested, what was not, and platform-specific limitations. Include
  safe screenshots for UI changes and note compatibility or migration impact.
- For a recorded validation run, use `node scripts/dev.mjs verify`. It produces
  ignored JSON/log artifacts for the frontend build, shared checks, full Go suite,
  and frontend tests, and is also used by PR CI and weekly maintenance.
  Link the failing and passing workflow runs when fixing an automated finding;
  do not discard the failure or present a local result as a hosted CI run.
- Use synthetic fixtures. Never attach credentials, private sync repositories,
  real session contents, or unredacted logs to a contribution.

The optional pre-commit hook can be enabled for a single commit without changing
configuration shared by linked worktrees:

```text
git -c core.hooksPath=.githooks commit
```

Hooks are opt-in and do not replace CI or review. Do not set shared local Git
hook configuration as part of repository setup.

## Maintenance and merge policy

`node scripts/dev.mjs cleanup` is a bounded Go formatting repair, equivalent to
`format`: it deletes nothing and never touches user sync data. The weekly CI
maintenance audit is read-only. This is distinct from the application's scheduled
trash cleanup and retention window.

Failed validation can produce review-only repair and rollback patches through
`node scripts/dev.mjs propose`. Follow the [review and reverse-check procedure](docs/development.md)
before applying either patch. The original CI failure is never changed to
success by generating a proposal, and no source changes are applied
automatically.

Dependabot proposes weekly GitHub Actions, Go, and frontend npm updates with
bounded open PRs. Updates require human review and passing checks; no automatic
merging is configured by these files.

After integration, a maintainer must configure branch protection/rulesets to
require the CI workflow's **Repository checks** and **Security checks** and
appropriate review approval before merging. Adding workflow files does not
configure those repository settings. Review permissions and exception policies
in GitHub; this document does not assert that protection is already enabled.
