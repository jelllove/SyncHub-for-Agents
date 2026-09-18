# AI-Ready Agent Review Design

## Goal

Raise the Operation axis to AI-Ready level by adding a real, enforced,
agent-assisted pull-request review surface while preserving SyncHub application
behavior and avoiding fabricated agent-authored PR history.

## Current state

The clean codeblend assessment for commit `cdc66f2` reported a composite score
of 81.0, but AI-Ready remained `no` because Operation was 72.5. The remaining
gap is operational: the report shows no enforced agent review, no
API-confirmed agent-authored merged PRs, and no local-loop verification profile.

## Selected route

Add a GitHub Actions workflow named `Copilot agent review` that runs on pull
requests and manual dispatch. The workflow installs a pinned GitHub Copilot CLI,
asks it to perform a read-only review of the repository/PR context, stores the
review output as an artifact, and fails closed if no review report is produced.
After the workflow is merged and passes, add the `Copilot agent review` status
check to the existing main ruleset.

This creates real runtime evidence for enforced agent review without claiming
that the repository has autonomous production repair or fake agent-authored
merged pull requests.

## Constraints

- Do not modify desktop application behavior.
- Do not create GitHub issues as part of the review workflow.
- Do not write comments automatically; keep the first version artifact-only.
- Do not use repository write permissions in the workflow.
- Do not change existing required checks except to add the new agent review check
  after it has passed on a pull request.
- Do not add required human reviews in this step; this repository has one
  collaborator and that could block owner-authored maintenance work.

## Validation

- Add workflow regression tests before creating the workflow.
- Run `go test ./tools/repocheck -count=1`.
- Run `node scripts/dev.mjs check` and `node scripts/dev.mjs verify`.
- Open a PR, confirm hosted CI, CodeQL, repair verification, and the new Copilot
  agent review pass.
- Merge the PR, update the ruleset to require `Copilot agent review`, and run a
  fresh clean codeblend evaluation.

## Rollback

If the Copilot CLI is unavailable in Actions, do not require the new check.
Instead, remove the workflow from the branch or leave it non-required and report
the external entitlement blocker.
