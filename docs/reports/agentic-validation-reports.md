# Agentic validation report index

SyncHub's repository automation publishes machine-readable reports through
GitHub Actions artifacts and local ignored `.artifacts/` directories.

| Artifact | Producer | Purpose |
| --- | --- | --- |
| `repository-validation` | `.github/workflows/ci.yml` | Full source-bound validation receipt, logs, JUnit XML, and offline HTML. |
| `maintenance-proposal` | `.github/workflows/ci.yml` | Bounded review-only patch proposal after failed validation. |
| `repair-verification` | `.github/workflows/repair-verification.yml` | Contained diagnostic failure, repair, revalidation, and rollback proof. |
| `ci-failure-response` | `.github/workflows/self-healing.yml` | Follow-up proposal artifact created only after CI failure or manual dispatch. |
| `rollback-verification` | `.github/workflows/self-healing.yml` | Lightweight review-only maintenance repair and rollback patch verification. |
| `copilot-agent-review` | `.github/workflows/copilot-agent-review.yml` | Read-only Copilot CLI PR audit output for correctness, safety, and validation evidence. |

The dashboard data in
[agentic-readiness-dashboard.json](../dashboards/agentic-readiness-dashboard.json)
lists the same signals for automated readers. These reports are evidence for
repository maintenance and review handoff, not a claim that SyncHub repairs user
data or production incidents autonomously.
