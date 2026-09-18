# SyncHub Validation

Use this skill when changing SyncHub source, workflows, validation scripts,
development evidence, packaging, or agent handoff documentation.

## Repository commands

Run commands from the repository root:

- `node scripts/dev.mjs setup` restores locked Go and frontend dependencies and
  builds embedded frontend assets.
- `node scripts/dev.mjs check` runs Go formatting checks, `go vet`, frontend
  lint/typechecking, Markdown links, generated command references, and evidence
  drift checks.
- `node scripts/dev.mjs verify` runs the full source-bound validation suite and
  writes JSON, JUnit XML, HTML, and command logs under ignored `.artifacts/`.
- `node scripts/dev.mjs repair:verify` runs the contained diagnostic repair and
  rollback proof against committed inputs.
- `node scripts/dev.mjs docs` checks documentation links, generated references,
  and evidence cross-references without rewriting files.
- Pull requests also run the `Copilot agent review` workflow, which uploads the
  `copilot-agent-review` artifact and must not modify files or create GitHub
  resources.
- GitHub Copilot cloud-agent tasks use `.github/copilot-instructions.md` and
  `.github/workflows/copilot-setup-steps.yml` for repository-specific setup.
- The `Documentation drift` status runs `node scripts/dev.mjs docs` as a
  dedicated fail-closed PR check.

## Safety rules

- Do not run application sync, trash cleanup, installer smoke tests, or repair
  loops against real user agent homes, credentials, or sync repositories.
- Treat `node scripts/dev.mjs propose` output as a review-only patch proposal,
  not an automatic source repair.
- Preserve source-bound validation failures and report artifacts exactly; never
  edit reports to make checks look successful.
- Keep releases, tags, merges, and stricter branch protection changes separate
  unless the maintainer explicitly asks for them.

## Handoff

Before handing off a change, include the local command results and hosted GitHub
Actions run links when available. If the codeblend assessment is requested,
report the generated score honestly and call out whether local uncommitted files
were included in the evaluated checkout.
