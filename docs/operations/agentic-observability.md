# Agentic observability

SyncHub records agent-assisted repository work through pull requests, hosted
checks, validation artifacts, labels, and human review notes. These surfaces make
the work observable; they do not claim production autonomous repair.

## Workflows and commands

- `.github/workflows/ci.yml`
  ([workflow](../../.github/workflows/ci.yml)) runs
  `node scripts/dev.mjs verify`, publishes `repository-validation`, and
  publishes `maintenance-proposal` only after a failed validation run produces a
  bounded review-only patch.
- `.github/workflows/maintenance.yml`
  ([workflow](../../.github/workflows/maintenance.yml)) schedules the same
  repository/security and Linux test checks without write permissions or
  packaging.
- `.github/workflows/repair-verification.yml`
  ([workflow](../../.github/workflows/repair-verification.yml)) runs
  `node scripts/dev.mjs repair:verify` and publishes `repair-verification`.
- `.github/workflows/self-healing.yml`
  ([workflow](../../.github/workflows/self-healing.yml)) listens to failed CI
  `workflow_run` events or manual dispatch, runs `node scripts/dev.mjs propose`,
  and publishes `ci-failure-response` without write permissions.

## Artifact contracts

- Repository path `docs/specs/validation-receipt.v1.schema.json`
  ([schema](../specs/validation-receipt.v1.schema.json)) describes validation
  receipts written by `node scripts/dev.mjs verify`.
- Repository path `docs/specs/repair-proof.v1.schema.json`
  ([schema](../specs/repair-proof.v1.schema.json)) describes diagnostic repair
  proof receipts written by `node scripts/dev.mjs repair:verify`.
- `repository-validation`, `maintenance-proposal`, and `repair-verification`
  artifacts are retained by GitHub Actions for bounded review windows.
- `ci-failure-response` captures the same bounded proposal format after CI
  failure detection.
- Repository path `docs/reports/agentic-validation-reports.md`
  ([report index](../reports/agentic-validation-reports.md)) lists the current
  machine-readable report artifacts.
- Repository path `docs/dashboards/agentic-readiness-dashboard.json`
  ([dashboard](../dashboards/agentic-readiness-dashboard.json)) mirrors the
  workflow and artifact names for automation.
- Repository path `docs/runbooks/ci-failure-response.md`
  ([runbook](../runbooks/ci-failure-response.md)) defines detection,
  containment, remediation proposal, validation, and rollback handoff.

## Labels and handoff

- `ai-readiness`: score reports, evidence gaps, and methodology work.
- `validation`: native checks, CI failures, and packaging evidence.
- `repair-proof`: diagnostic fault injection, repair validation, and rollback
  evidence.
- `agent-review`: agent-assisted review comments, prompts, and supervised
  handoff.
- `documentation-drift`: documentation/reference mismatches found by checks.

Human reviewers should link failed and passing workflow runs in pull requests,
verify the report `status` before trusting individual command rows, and treat
repair proposals as patches requiring review rather than automatic fixes.

## Agent execution surfaces

- `CODEOWNERS` routes repository-wide review ownership to the maintainer without
  requiring a second approver in this single-maintainer repository.
- `.github/ISSUE_TEMPLATE/config.yml` points stale-validation reports toward
  the maintenance evidence workflow.
- `.vscode/mcp.json` exposes the local `synchub-validation` MCP server.
- `tools/mcp/validation-server.mjs` is a read-only stdio MCP server with tools
  for listing validation commands and running `node scripts/dev.mjs docs`.
