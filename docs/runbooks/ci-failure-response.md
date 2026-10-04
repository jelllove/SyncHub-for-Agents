# CI failure response

This runbook defines the bounded response for repository validation failures.
It uses existing read-only repository tooling and keeps source modification under
human review.

1. **Detection:** required PR checks and scheduled maintenance identify failed
   validation.
2. **Containment:** failed required checks block merge, while logs and
   `repository-validation` artifacts preserve the failing evidence.
3. **Remediation proposal:** `node scripts/dev.mjs propose` may prepare a
   bounded `maintenance-proposal` or `ci-failure-response` artifact for Go
   formatting and generated-reference drift only. When it proposes changes, it
   emits both `repair.patch` and `rollback.patch`; both patches are review-only
   and must not be applied automatically to user data or production incidents.
4. **Review:** a maintainer inspects the patch and report before applying any
   source change.
5. **Validation:** `node scripts/dev.mjs verify` and hosted checks must pass
   before the patch is merged.
6. **Rollback handoff:** reviewers can check and apply `rollback.patch` to
   reverse the exact proposed repair patch, while `node scripts/dev.mjs
   repair:verify` demonstrates the diagnostic repair cycle in an isolated clone
   and leaves the original source unchanged.
   The self-healing workflow also runs `node scripts/dev.mjs rollback:verify`
   and publishes `rollback-verification` as a lightweight proof of this
   review-only handoff path.

The `.github/workflows/self-healing.yml` workflow is intentionally read-only. It
does not push commits, create issues, create pull requests, merge branches, or
claim production self-healing.
