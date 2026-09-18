# Agentic observability

SyncHub records agent-assisted repository work through pull requests, hosted
checks, validation artifacts, labels, and human review notes. These surfaces make
the work observable; they do not claim production autonomous repair.

## Workflows and commands

- `.github/workflows/ci.yml`
  ([workflow](../../.github/workflows/ci.yml)) runs
  `node scripts/dev.mjs verify`, publishes `repository-validation`, and
  publishes `maintenance-proposal` only after a failed validation run produces a
  bounded review-only patch. It also includes the dedicated
  `Documentation drift` status, which runs `node scripts/dev.mjs docs` as a
  fail-closed PR check.
- `.github/workflows/copilot-setup-steps.yml`
  ([workflow](../../.github/workflows/copilot-setup-steps.yml)) prepares the
  GitHub Copilot cloud-agent environment with pinned Go, Node, Linux desktop
  prerequisites, and `node scripts/dev.mjs setup`.
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
  and publishes a bounded repair and rollback handoff as `ci-failure-response`
  without write permissions.
- `.github/workflows/codeql.yml`
  ([workflow](../../.github/workflows/codeql.yml)) runs pinned CodeQL
  JavaScript/TypeScript analysis for pull requests, default-branch pushes,
  schedules, and manual dispatch.
- `.github/workflows/copilot-agent-review.yml`
  ([workflow](../../.github/workflows/copilot-agent-review.yml)) runs the pinned
  GitHub Copilot CLI as a read-only PR auditor and uploads
  `copilot-agent-review`. It is a read-only, artifact-producing review gate, not
  an automatic approval, merge, release, or production self-healing claim.

## Artifact contracts

- Repository path `docs/specs/validation-receipt.v1.schema.json`
  ([schema](../specs/validation-receipt.v1.schema.json)) describes validation
  receipts written by `node scripts/dev.mjs verify`.
- Repository path `docs/specs/repair-proof.v1.schema.json`
  ([schema](../specs/repair-proof.v1.schema.json)) describes diagnostic repair
  proof receipts written by `node scripts/dev.mjs repair:verify`.
- Repository path `docs/specs/README.md`
  ([spec index](../specs/README.md)) and repository path
  `docs/specs/agentic-validation.v1.md`
  ([specification](../specs/agentic-validation.v1.md)) define the versioned
  validation evidence set.
- Repository path `docs/adr/0001-validation-evidence.md`
  ([ADR](../adr/0001-validation-evidence.md)) records why the evidence contracts
  are versioned.
- `repository-validation`, `maintenance-proposal`, and `repair-verification`
  artifacts are retained by GitHub Actions for bounded review windows.
- `ci-failure-response` captures the same bounded proposal format after CI
  failure detection, including review-only `repair.patch` and `rollback.patch`
  artifacts when a supported repair is proposed.
- `copilot-agent-review` captures the read-only Copilot PR auditor output. It is
  designed to become a required status check after a hosted run has passed.
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
trust the overall validation report `status` before individual check rows, and treat
repair proposals as patches requiring review rather than automatic fixes.

## Agent execution surfaces

- `CODEOWNERS` routes repository-wide review ownership to the maintainer without
  requiring a second approver in this single-maintainer repository.
- `.agents/skills/synchub-validation/SKILL.md` provides committed repository
  guidance for setup, check, verify, repair proof, and score-assessment handoff.
- `.github/copilot-instructions.md` gives Copilot cloud agent and Copilot code
  review the repository-specific setup, validation, safety, and handoff rules.
- `.github/ISSUE_TEMPLATE/config.yml` points stale-validation reports toward
  the maintenance evidence workflow.
- `.vscode/mcp.json` exposes the local `synchub-validation` MCP server.
- `tools/mcp/synchub-mcp-server.mjs` is the shipped repository MCP server
  wrapper configured by `.vscode/mcp.json`.
- `tools/mcp/validation-server.mjs` is the read-only stdio MCP implementation
  with tools for listing validation commands and running
  `node scripts/dev.mjs docs`. Its tools set `annotations.readOnlyHint: true`
  for Copilot code review.
- `.pre-commit-config.yaml` offers optional local pre-commit hooks for
  `node scripts/dev.mjs check` and `node scripts/dev.mjs docs`.
