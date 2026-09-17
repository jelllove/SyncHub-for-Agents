# Agentic validation specification v1

## Scope

This specification covers repository validation evidence for SyncHub. It does
not define production incident response or autonomous repair of user sync data.

## Required receipts

- `validation-receipt.v1.schema.json` describes `validation.json`, `junit.xml`,
  and `index.html` emitted by `node scripts/dev.mjs verify`.
- `repair-proof.v1.schema.json` describes `repair.json` emitted by
  `node scripts/dev.mjs repair:verify`.

## Required workflows

- `.github/workflows/ci.yml` publishes `repository-validation` and
  `maintenance-proposal`.
- `.github/workflows/repair-verification.yml` publishes `repair-verification`.
- `.github/workflows/self-healing.yml` publishes `ci-failure-response` from a
  read-only workflow-run diagnostic path.
- `.github/workflows/codeql.yml` runs CodeQL JavaScript/TypeScript analysis.

## Required review surfaces

- `CODEOWNERS` declares the maintainer review route.
- `.github/labels.yml` declares evidence-routing labels.
- `.github/ISSUE_TEMPLATE/config.yml` points stale-validation reports to
  maintenance evidence.
- `.vscode/mcp.json` exposes `tools/mcp/validation-server.mjs` for read-only
  validation command discovery and documentation drift checks.
