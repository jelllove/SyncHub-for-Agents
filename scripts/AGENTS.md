# Script and maintenance guidance

Inherit the [root guidance](../AGENTS.md).

- [dev.mjs](dev.mjs) is the public setup/check/verify entry point;
  [dev-lib.mjs](dev-lib.mjs) contains shared execution and documentation checks;
  [validation.mjs](validation.mjs) records real validation outcomes.
- [source-snapshot.mjs](source-snapshot.mjs) hashes repository inputs using a
  bounded buffer without following linked source paths. Capture errors or observed
  changes must fail verification, not yield a success-shaped fallback.
- [maintenance.mjs](maintenance.mjs) prepares review-only formatting/reference
  proposals in isolated snapshots; it must never apply patches to the real tree.
- [repair-container.mjs](repair-container.mjs) contains the diagnostic native
  repair proof; [repair-loop.mjs](repair-loop.mjs) runs only against disposable
  clones and records failure, repair, revalidation and rollback.
- [reporting.mjs](reporting.mjs) renders completed receipts as JUnit and offline
  HTML. Do not turn incomplete/error/source-changed receipts into success.
- Keep the opt-in hook fast and non-mutating. Dependency installation belongs
  to setup; report generation belongs to explicit verification.
- Propagate native command failures. Validation may finish independent checks
  to collect evidence, but any failure must make the overall command fail.
- Reuse strict text decoding for source transformations. Never replace invalid
  UTF-8 bytes implicitly when formatting or regenerating documentation.
- Reuse batched Go formatting for checks, explicit formatting, and repair
  proposals. Keep temporary copies isolated, map diagnostics back to source
  paths, and preflight all inputs before modifying real files.
- Bound commands and refuse linked output directories. Never clean broad paths,
  format external sources, change shared Git configuration, or invoke application
  synchronization against a developer's data.
- Store generated output under ignored `.artifacts/validation/`. Preserve older
  receipts and distinguish incomplete, failed, and passed runs.
- Proposal reports live under `.artifacts/maintenance/`. Keep the 50-file and
  1-MiB patch limits, source digests, applicability checks and review requirement.
- Publish artifact directories only after creating them safely. CI must not
  upload arbitrary pre-existing `.artifacts` paths after initialization failures.
- Maintenance audits must remain read-only with respect to source and user data.
  Do not add automatic commits, issue/PR creation, or merge permissions.
- Diagnostic repair proofs must stay clearly labeled as fault injection, use
  committed inputs and immutable container identities, and never mount host homes
  or the Docker socket or run with privileged/root container permissions.
- Add regression tests using synthetic repositories in
  [validation.test.mjs](../frontend/validation.test.mjs) and
  [tooling.test.mjs](../frontend/tooling.test.mjs). CI wiring is tested in
  [workflow_test.go](../tools/repocheck/workflow_test.go).
- Packaging and smoke scripts have platform-specific side effects. Do not run
  installer smoke tests against an ordinary developer account during generic checks.
