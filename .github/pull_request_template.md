## Summary

Describe the change, motivation, and any compatibility or migration impact.

## Validation

List commands and results; explain skipped checks and untested platforms.

For an automated finding, link the failing run, the fix/regression test, and the
passing validation run. `node scripts/dev.mjs verify` creates a local JSON/log
report; distinguish local evidence from hosted CI results.

- [ ] Relevant regression tests added/updated, or not applicable explained.
- [ ] `node scripts/dev.mjs check`
- [ ] `go test ./...` (for code changes)
- [ ] `npm --prefix frontend test` (for code changes)
- [ ] `npm --prefix frontend run build` (for code changes)
- [ ] Documentation-only changes: `node scripts/dev.mjs docs` and manual accuracy review.

## Safety and review

- [ ] No credentials, private session data, or real sync repositories in fixtures/logs.
- [ ] User data, path validation, conflict protection, and install approval remain safe.
- [ ] Generated bindings were regenerated rather than hand-edited, if affected.
- [ ] Documentation and generated command references updated where relevant.

For UI changes, include redacted screenshots and note native desktop testing.
Report vulnerabilities through [private security reporting](../SECURITY.md),
not this public pull request.
