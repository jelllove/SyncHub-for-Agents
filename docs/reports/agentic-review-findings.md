# Agentic review findings

This index records review findings that changed repository guidance, tests, or
workflows. It provides human-readable evidence for continuous improvement loops;
it does not claim closed-loop autonomy.

| Finding | Source | Outcome | follow-up validation |
| --- | --- | --- | --- |
| Copilot review finding: the POSIX PR validation workflow needed Linux desktop dependencies. | PR #16 Copilot review | Added the dependency install step before `go test ./...` and `npm test`. | PR #16 required checks passed. |
| Copilot review finding: the shipped MCP server should delegate to the existing implementation instead of duplicating protocol code. | PR #16 Copilot review | `mcp/synchub-validation/server.mjs` delegates to `tools/mcp/validation-server.mjs`. | `npm --prefix frontend run test:lint` covers the shipped wrapper. |
| Recurring Copilot review contract should stay statically discoverable. | PR #17 and Recurring Copilot review | Added `.github/actions/recurring-copilot-review/action.yml` and guarded it with repocheck/doc-drift tests. | Composite 80.5 assessment and hosted Recurring Copilot review. |
