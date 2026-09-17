# Frontend guidance

Inherit the [root guidance](../AGENTS.md). Commands below run from the repository
root. This is React/TypeScript with Vite, Vitest, and Testing Library.

## Boundaries

- `src/App.tsx` coordinates desktop state and Wails events;
  `src/desktopState.ts` normalizes backend snapshots.
- `src/onboarding` handles first-run setup; `src/resources` handles resource
  previews, conflicts, and installation plans. Settings and updates have separate
  panels and adjacent tests.
- Import backend APIs/types from `bindings`. Those files are generated from Go:
  change `internal/desktop` and use the existing `generate:bindings` task in
  `build/Taskfile.yml`, never patch generated output manually.
- Keep filesystem access, credential handling, sync policy, and installer
  execution behind the Go service boundary.

## UI changes

- Preserve explicit approvals for installation plans and visible retry/error
  states. Never make a failed request look like successful sync or installation.
- Clean up event subscriptions and cancel obsolete requests on unmount or
  replacement. Follow existing request/state normalization patterns.
- Use accessible names, labels, keyboard-operable controls, and clear loading
  states. Avoid exposing credentials or private session contents in messages.
- Add adjacent component tests using existing Wails/runtime mocks and synthetic
  data; do not require a running desktop app, live network, or real agent home.

## Validation

- Install dependencies with root `node scripts/dev.mjs setup` (uses `npm ci`).
- Run `npm --prefix frontend run lint`, `npm --prefix frontend run typecheck`,
  `npm --prefix frontend test`, and `npm --prefix frontend run build` for frontend
  changes. For quick iteration, pass related test filenames after
  `npm --prefix frontend test --`.
- Run `npm --prefix frontend run test:lint` after changing lint dependencies or
  configuration. These Node tests exercise the actual configuration with
  in-memory TypeScript/TSX fixtures and check the ESLint CLI's exit status.
  Vitest retains default discovery for UI tests and root-level repository tooling
  suites. Only the standalone `lint.test.mjs`, `doc-drift.test.mjs`, and
  `mcp-server.test.mjs` suites are additionally excluded from Vitest and run
  separately through `test:lint`.
- Keep `package-lock.json` in sync with intentional dependency changes. Do not
  commit `node_modules` or `dist`; the build produces assets embedded by Go.
- Browser/component tests do not prove native tray/window behavior. For desktop
  changes, validate with the pinned optional Wails CLI and platform prerequisites
  described in the [development reference](../docs/development.md).

## ESLint compatibility and scope

- `eslint.config.mjs` checks handwritten `src` files, `vite.config.ts`, the icon
  generator, and its own config/tests. Generated `bindings`, `dist`,
  `node_modules`, and other root-level `*.test.mjs` tooling suites are excluded.
  Lint is read-only, rejects warnings, and reports unused suppression comments.
- Use the standard ESLint recommended defect checks plus `eqeqeq`,
  `no-unreachable-loop`, `no-var`, `prefer-const`, and
  `react-hooks/rules-of-hooks`. Deliberate `== null` / `!= null` checks are allowed.
  There are no formatting rules or automatic hook-dependency changes;
  `exhaustive-deps` and the broader React compiler preset are not enabled.
- The TypeScript compiler remains at 7.0.2. `typescript-eslint` 8.70.0 declares
  a TypeScript peer range of `>=4.8.4 <6.1.0`, so it is not compatible. Instead,
  Babel 8's ESLint parser reads TypeScript/TSX syntax without depending on the
  TypeScript compiler. Do not force unsupported peer versions or mistake this
  syntax-only lint for type-aware analysis; keep running `typecheck`.
- Only for TypeScript files, leave `no-undef`, `no-unused-vars`, `no-redeclare`,
  and `no-dupe-class-members` to the compiler: core JavaScript rules do not model
  type-only names, declaration merging, or overloads correctly. JavaScript
  tooling retains these checks. Preserve the existing compiler policy allowing
  implicit/explicit `any` and unused parameters; changing that policy is a
  separate task. Revisit typed lint rules when their published peers support
  this repository's compiler.
