# Copilot instructions for SyncHub

Use the repository-local validation guidance in
`.agents/skills/synchub-validation/SKILL.md` before changing SyncHub.

## Setup and validation

- Run `node scripts/dev.mjs setup` before full validation in a fresh checkout.
- Run `node scripts/dev.mjs check` for Go formatting, `go vet`, frontend lint,
  typechecking, Markdown links, generated references, and evidence drift checks.
- Run `node scripts/dev.mjs verify` before proposing code changes that affect
  application behavior, workflows, scripts, or validation evidence.
- Run `node scripts/dev.mjs docs` for documentation-only changes.
- Treat `node scripts/dev.mjs propose` output as review-only repair and
  rollback patch proposals; do not apply, commit, or push them automatically.
- Treat `node scripts/dev.mjs repair:verify` as a diagnostic proof, not as
  production self-healing.
- Use `node scripts/dev.mjs rollback:verify` for a lightweight isolated check of
  review-only maintenance repair and rollback patches.

## Safety boundaries

- Do not run application sync, trash cleanup, installer smoke tests, or repair
  loops against real user agent homes, credentials, or sync repositories.
- Do not create releases or tags unless the maintainer explicitly asks for a
  release.
- Do not edit generated `frontend/bindings` by hand.
- Do not make failed checks look successful. Preserve validation artifacts and
  report the failing command output.

## Handoff

Summaries should include changed behavior, validation commands, hosted run links
when available, and any platform behavior that was not exercised.
