# Repository guidance

SyncHub for Agents is a Wails v3 desktop/tray application with a Go backend
and a React/TypeScript frontend. It synchronizes portable agent resources
through a user-owned private Git repository.

## Start here

- Read [contribution guidance](CONTRIBUTING.md) and the
  [development reference](docs/development.md).
- Read the [architecture guide](docs/architecture.md) before changing core dependencies.
- Use the Go minimum in [go.mod](go.mod) and Node 24 (at least 24.13.0).
  Setup enforces the exact Node version in [.node-version](.node-version).
- From the repository root, run `node scripts/dev.mjs setup` to install locked
  frontend dependencies, download Go modules, and build embedded frontend assets.
  Setup does not install global tools or change Git configuration.
- Follow [backend guidance](internal/AGENTS.md) for `internal/` and
  [frontend guidance](frontend/AGENTS.md) for `frontend/`.

## Map

- `main.go`, `app.go`: desktop composition and lifecycle; `cmd/`: CLI entry points.
- `internal/desktop`: Wails service boundary and UI models.
- `internal/cli`, `internal/daemon`, `internal/scheduler`: sync orchestration.
- `internal/syncengine`: resource synchronization, restore, deletion, and cleanup.
- `internal/portableconfig`, `internal/secret`, `internal/resourcecollect`:
  portable-field projection and safety filtering.
- `internal/conflict`, `internal/installplan`: conflict resolution and approved installs.
- `frontend/src`: UI; `frontend/bindings`: generated Wails API.
- `build/`, `Taskfile.yml`: platform-specific desktop build/package tasks.

## Change boundaries

- Keep changes scoped; preserve compatibility with existing SyncHub identifiers.
  The module path `github.com/qinqingxu/synchub-for-agents` intentionally differs
  from the repository owner. Do not rename it to match GitHub.
- Follow [.editorconfig](.editorconfig); avoid repository-wide cosmetic changes.
- Never hand-edit generated `frontend/bindings`; update the Go service/models
  and regenerate via the existing Wails binding task.
- Do not use real agent homes, credentials, synced repositories, or user sync data
  as test fixtures. Use isolated synthetic fixtures and mocked external commands.
- Preserve credential exclusion, path containment, conflict protection, and
  explicit installation approval. See [portable-resource rules](docs/portable-resources.md).
- Repository maintenance must not invoke application sync/trash cleanup or mutate
  user data. `node scripts/dev.mjs cleanup` only repairs Go formatting; it deletes
  nothing. The weekly repository maintenance audit is read-only.
- Report vulnerabilities privately using [SECURITY.md](SECURITY.md).

## Verify and hand off

- Run the smallest relevant existing tests while developing. Before handoff:
  `node scripts/dev.mjs check`, `go test ./...`,
  `npm --prefix frontend test`, and `npm --prefix frontend run build`.
- `node scripts/dev.mjs verify` builds frontend assets and runs shared checks,
  all current-host Go tests, and frontend tests with actual results and logs.
  Preserve failed output, add a regression test, and include
  failing/passing run links in the PR; never edit a report to make a check pass.
- Verification records source fingerprints and rejects observed source/revision
  changes during checks. Do not edit the checkout during a run or rely on only
  individual passed checks when the overall report failed.
- `check` checks Go formatting, runs `go vet`, frontend lint/typechecking, and docs
  drift checks. Repair Go formatting with `node scripts/dev.mjs format`.
- `node scripts/dev.mjs propose` prepares bounded review-only
  formatting/reference repair and rollback patches without changing source or
  the index. Review the report and patches; never treat a proposal as a passing
  full validation or apply a report marked failed/running.
- `node scripts/dev.mjs repair:verify` checks a diagnostic repair/rollback cycle
  in a restricted Linux container using committed inputs. It does not repair the
  developer checkout, merge a PR, or establish production autonomy.
- `node scripts/dev.mjs rollback:verify` checks the lightweight review-only
  maintenance repair/rollback handoff in an isolated fixture.
- For documentation-only edits, run `node scripts/dev.mjs docs`. This checks
  relative link target existence and generated versions/command references,
  not semantic accuracy; review the prose against code.
- Use `node scripts/dev.mjs docs:write` only to regenerate the command reference
  in `docs/development.md`. Review every generated diff.
- Summarize changed behavior, validation results, and any untested platform
  behavior. Do not claim CI or branch protection is configured merely from docs.
