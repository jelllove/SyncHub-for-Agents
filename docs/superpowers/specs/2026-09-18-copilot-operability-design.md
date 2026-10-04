# Copilot Operability Design

## Goal

Improve SyncHub's real GitHub Copilot cloud-agent operability without changing
desktop application behavior or fabricating closed-loop evidence.

## Current state

The latest clean codeblend assessment for `qinqingxu/SyncHub-for-Agents` reports
82.3 composite, Substrate 90.4, Operation 75.0, and `AI-Ready: no`. The
repository now has three API-confirmed `app/copilot-swe-agent` merged pull
requests, required reviews, required checks, and a read-only Copilot agent review
workflow. Remaining evidence gaps include empty `copilotInstructionPaths`, empty
`mcpServersShipped`, empty `validationCommands`, and documentation drift controls
reported as advisory/partial.

## Selected approach

Add a focused Copilot operability layer:

1. Commit `.github/copilot-instructions.md` so Copilot cloud agent and Copilot
   code review get repository-specific setup, validation, safety, and handoff
   instructions.
2. Commit `.github/workflows/copilot-setup-steps.yml` so Copilot cloud agent has
   deterministic setup steps on the default branch before future tasks.
3. Add `annotations.readOnlyHint: true` to every tool returned by
   `tools/mcp/validation-server.mjs`, matching GitHub's MCP guidance for Copilot
   code review.
4. Add a dedicated `Documentation drift` CI job that runs
   `node scripts/dev.mjs docs` as its own fail-closed status. After the PR is
   merged and the check has passed, add that status to the main ruleset.

This is useful even if the evaluator still requires an official local closed-loop
profile for `AI-Ready: yes`.

## Alternatives considered

- Create more routine Copilot PRs. Rejected as the main strategy because the
  evaluator already recognizes three agent-authored merged PRs and still caps
  Operation at 75 without a score-4 closed loop.
- Fabricate rollback or local-loop evidence. Rejected because the evaluator
  requires approved verifier/profile evidence and unsupported profiles are
  rejected.
- Disable review requirements to make merging faster. Rejected because the
  current target is operational readiness, not convenience.

## Safety constraints

- Do not modify application source behavior.
- Do not change release tags or create a release in this optimization.
- Do not reduce existing required checks or review protection.
- Do not store secrets in Copilot instructions, setup steps, or MCP config.
- Keep MCP tools read-only and make the read-only hint executable in tests.
- Keep documentation drift checks deterministic and fail-closed.

## Validation

- Add failing tests before changing workflow/MCP behavior.
- Run `npm --prefix frontend run test:lint`.
- Run `go test ./tools/repocheck -count=1`.
- Run `node scripts/dev.mjs check` and `node scripts/dev.mjs verify`.
- Open a PR, confirm hosted CI/CodeQL/repair/Copilot review pass, approve with
  the separate collaborator account, and merge normally.
- Add `Documentation drift` to the main ruleset only after its hosted status has
  passed.
- Run one fresh codeblend assessment and report the actual result.
