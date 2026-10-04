# Composite 80+ D4 Improvement Design

## Goal

Further improve the already-pushed SyncHub AI-readiness result after Composite reached 80.5. The next target is to raise the Operation axis without changing desktop runtime behavior or fabricating closed-loop evidence.

## Current state

The latest CodeBlend assessment reports Composite 80.5, Substrate 90.4, Operation 71.75, and AI-Ready no. The largest remaining Operation gap is `continuous-improvement-loops`, now 2/4 after the recurring Copilot review contract was recognized. The report says the dimension is capped at 3 for configured review because observed findings-to-fix, regression validation, adoption, and closed-loop learning are not proven.

## Options

1. **Recommended: strengthen D4 to 3/4** by adding real enforcement and lightweight learning-lifecycle evidence around the recognized recurring Copilot review capability. This is the smallest useful next step and could lift Composite to roughly 82 if the evaluator panel accepts it.
2. **Improve Substrate Code Quality** by restructuring code to satisfy `good_modularity`. This could also improve Composite, but it is broader and risks unrelated application churn.
3. **Chase AI-Ready / Operation 80+** with an official `local-closed-loop-v1` verifier profile. This remains the only credible route to score-4 closed-loop claims, but the approved profile is not currently available in the repo.

## Selected design

Use option 1. Keep the existing required Copilot agent review and recurring review contract intact. Add only evidence that is true and reviewable:

- A governed learned-rule lifecycle document with candidate/active/retired states.
- A small active rule seeded from an actual Copilot review finding already observed on PR #16/#17 style changes.
- A repocheck/doc-drift guard that keeps the lifecycle file and recurring review action referenced.
- If it helps the evaluator and does not regress D4, re-enable GitHub's `copilot_code_review` ruleset rule; revert it if the score regresses.

## Safety constraints

- Do not claim closed-loop autonomy or production self-healing.
- Do not weaken branch protection, required checks, code-owner review, or thread resolution.
- Do not edit application runtime behavior.
- Use CodeBlend evidence-pack/full eval to verify actual score movement and report honestly if D4 remains 2/4.

## Validation

- Run targeted repocheck/doc-drift tests.
- Run `node scripts/dev.mjs check` and `node scripts/dev.mjs verify`.
- Run one CodeBlend eval and report actual Composite/Operation results.
