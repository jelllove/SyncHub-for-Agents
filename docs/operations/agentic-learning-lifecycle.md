# Agentic learning lifecycle

SyncHub uses recurring Copilot review as a supervised learning loop for
repository maintenance. This document is evidence for review adoption and rule
promotion; it is not a claim of autonomous production repair.

## States

- `candidate`: a finding or review pattern has been observed but has not been
  accepted into repository guidance.
- `active`: the pattern has a documented rule, an owner, and regression validation
  in CI or local checks.
- `retired`: the rule no longer applies because the underlying workflow or risk
  changed.
- `rejected`: the finding was reviewed and intentionally not adopted.

## Promotion rules

1. Capture the Copilot review finding in
   `docs/reports/agentic-review-findings.md`.
2. Link the finding to a pull request, follow-up validation, and the affected
   recurring review or validation surface.
3. Promote a candidate to active only after a maintainer-reviewed change lands
   and `node scripts/dev.mjs verify` or hosted required checks pass.
4. Retire or reject stale entries with a note explaining why the rule no longer
   applies.

## Active rules

| ID | Source | Rule | Validation |
| --- | --- | --- | --- |
| D4-001 | PR #16 Copilot review finding | Local recurring review actions must avoid unresolved shell expansion in the evaluator-visible contract. | `go test ./tools/repocheck -count=1`, `npm --prefix frontend run test:lint`, hosted Recurring Copilot review |

## Ownership

The maintainer reviews every lifecycle change. Recurring Copilot review can
surface candidates, but humans promote, retire, or reject rules.
