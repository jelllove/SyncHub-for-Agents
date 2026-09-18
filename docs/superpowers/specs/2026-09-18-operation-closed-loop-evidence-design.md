# Operation Closed-Loop Evidence Design

## Goal

Improve the Operation axis with real, repository-owned evidence that the
CodeBlend evaluator can inspect. The change must not alter SyncHub desktop
runtime behavior, relax branch protection, fabricate hosted evidence, or claim
production autonomous repair.

## Current state

The latest clean assessment for `qinqingxu/SyncHub-for-Agents` reports 82.3
composite, Substrate 90.4, Operation 75.0, and `AI-Ready: no`. The Operation
axis is capped because every dimension is still 3/4. The evidence pack exposes
three repository-local gaps that can be improved without unsafe GitHub policy
changes:

- `selfHealing.hasRollbackPath = false`
- `agentSurfaces.mcpServersShipped = []`
- `validationCommands = []`

Hosted policy evidence also reports `agentCodeReviewEnforced = false`, but the
repository already has a required `Copilot agent review` status and the
evaluator separately rates the PR auditor as strong, merge-gated evidence. This
design therefore avoids weakening or repeatedly reshaping branch protection.

## Approach options

### Recommended: executable closed-loop evidence

Publish explicit repair and rollback artifacts from the existing review-only
maintenance proposal flow, make the self-healing workflow name the rollback
handoff it already verifies, and expose repository validation through
Taskfile/MCP surfaces. This is the best chance to lift at least one Operation
dimension because it changes inspectable machine evidence, not only prose.

### Conservative: workflow and documentation cleanup only

Add clearer runbook prose and workflow step names without changing generated
artifacts. This is low risk but likely repeats the previous result: useful for
humans, weak for an evaluator looking for structured evidence.

### GitHub policy experimentation

Keep changing required review/ruleset settings to try to flip
`agentCodeReviewEnforced`. This is risky because the repo already requires the
agent-review status, and there is no confirmed public API contract for the
reported field.

## Selected design

Implement the recommended path in three small surfaces:

1. Extend `node scripts/dev.mjs propose` so a proposed repair writes both
   `repair.patch` and `rollback.patch`. The rollback patch is generated from the
   repaired snapshot back to the original snapshot, verified with
   `git apply --check`, and recorded in `proposal.json`.
2. Update the self-healing workflow and runbook language so the CI failure
   response explicitly publishes a bounded repair plus rollback handoff artifact
   while remaining read-only.
3. Add repository-level validation tasks to `Taskfile.yml` and improve MCP
   server discoverability with a small committed wrapper named as a repository
   MCP server. These surfaces point to existing commands instead of inventing
   new validation semantics.

## Safety constraints

- Keep all source repair output review-only; do not auto-apply patches, commit,
  push, open issues, open pull requests, merge, or release.
- Keep self-healing workflow permissions read-only.
- Preserve exact file-count and patch-size limits.
- Fail explicitly if a patch or rollback patch cannot be generated or verified.
- Keep generated artifacts under ignored `.artifacts/`.
- Do not modify application sync behavior or user data handling.

## Validation

- Update or add tests for maintenance proposal rollback artifacts.
- Update repository checks that assert the self-healing rollback handoff and
  validation task surfaces stay wired.
- Run targeted tests first, then `node scripts/dev.mjs check` and
  `node scripts/dev.mjs verify`.
- Run a local CodeBlend `evidence-pack` or full `eval --no-cache` once after
  changes to report the actual Operation evidence movement.
