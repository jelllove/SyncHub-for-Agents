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

## Artifact contracts

- Repository path `docs/specs/validation-receipt.v1.schema.json`
  ([schema](../specs/validation-receipt.v1.schema.json)) describes validation
  receipts written by `node scripts/dev.mjs verify`.
- Repository path `docs/specs/repair-proof.v1.schema.json`
  ([schema](../specs/repair-proof.v1.schema.json)) describes diagnostic repair
  proof receipts written by `node scripts/dev.mjs repair:verify`.
- `repository-validation`, `maintenance-proposal`, and `repair-verification`
  artifacts are retained by GitHub Actions for bounded review windows.

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
