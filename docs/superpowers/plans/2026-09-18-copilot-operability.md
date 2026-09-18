# Copilot Operability Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add repository-specific Copilot cloud-agent setup, instructions, MCP read-only annotations, and an independently required documentation drift gate.

**Architecture:** Keep application behavior unchanged and add only repository-operability surfaces. Existing `node scripts/dev.mjs docs` remains the single implementation of documentation drift detection; workflows and tests make that check visible as its own gate.

**Tech Stack:** GitHub Actions YAML, GitHub Copilot cloud-agent conventions, Node.js ESM, Node's built-in test runner, Go repocheck tests, Markdown.

---

## File structure

- Create `.github/copilot-instructions.md` for cloud-agent and code-review instructions.
- Create `.github/workflows/copilot-setup-steps.yml` for deterministic cloud-agent setup.
- Modify `tools/mcp/validation-server.mjs` to annotate every tool as read-only.
- Modify `frontend/mcp-server.test.mjs` to assert read-only annotations.
- Modify `.github/workflows/ci.yml` to add a `Documentation drift` job that runs `node scripts/dev.mjs docs`.
- Modify `tools/repocheck/workflow_test.go` to guard the new Copilot setup and docs gate contracts.
- Modify `scripts/doc-drift.mjs`, `frontend/doc-drift.test.mjs`, `frontend/validation.test.mjs`, and `frontend/repair-loop.test.mjs` to include the new committed evidence files.
- Modify `docs/operations/agentic-observability.md`, `docs/specs/agentic-validation.v1.md`, and `.agents/skills/synchub-validation/SKILL.md` to document the new surfaces.

## Task 1: Copilot setup and instructions

**Files:**
- Create: `.github/copilot-instructions.md`
- Create: `.github/workflows/copilot-setup-steps.yml`
- Modify: `tools/repocheck/workflow_test.go`

- [ ] **Step 1: Write failing repocheck tests**

Add tests that require `.github/copilot-instructions.md` and `.github/workflows/copilot-setup-steps.yml` to exist. The setup workflow must have exactly one `copilot-setup-steps` job, use `contents: read`, install Go and Node from repository pins, run `node scripts/dev.mjs setup`, and stay under the 59-minute Copilot setup limit.

- [ ] **Step 2: Verify RED**

Run:

```powershell
go test ./tools/repocheck -run 'TestCopilotInstructionsAreCommitted|TestCopilotSetupStepsAreBoundedAndDeterministic' -count=1
```

Expected: FAIL because the files do not exist.

- [ ] **Step 3: Implement the two files**

Create `.github/copilot-instructions.md` with specific instructions to use `node scripts/dev.mjs setup`, `check`, `verify`, and `docs`; preserve user-data safety; avoid releases/tags unless asked; and report validation links.

Create `.github/workflows/copilot-setup-steps.yml` with one `copilot-setup-steps` job, `runs-on: ubuntu-24.04`, `timeout-minutes: 30`, read-only contents permission, checkout, setup-go from `go.mod`, setup-node from `.node-version`, Linux desktop prerequisites, and `node scripts/dev.mjs setup`.

- [ ] **Step 4: Verify GREEN**

Run the same Go test command and expect PASS.

## Task 2: MCP read-only annotations

**Files:**
- Modify: `tools/mcp/validation-server.mjs`
- Modify: `frontend/mcp-server.test.mjs`

- [ ] **Step 1: Write failing MCP test**

Update `frontend/mcp-server.test.mjs` to assert every returned tool includes:

```javascript
annotations: { readOnlyHint: true }
```

- [ ] **Step 2: Verify RED**

Run:

```powershell
npm --prefix frontend exec -- node --test mcp-server.test.mjs
```

Expected: FAIL because tools currently have no annotations.

- [ ] **Step 3: Add annotations**

Add `annotations: { readOnlyHint: true }` to each MCP tool object.

- [ ] **Step 4: Verify GREEN**

Run:

```powershell
npm --prefix frontend run test:lint
```

Expected: PASS.

## Task 3: Documentation drift gate

**Files:**
- Modify: `.github/workflows/ci.yml`
- Modify: `tools/repocheck/workflow_test.go`
- Modify: `scripts/doc-drift.mjs`
- Modify: `frontend/doc-drift.test.mjs`
- Modify: `frontend/validation.test.mjs`
- Modify: `frontend/repair-loop.test.mjs`

- [ ] **Step 1: Write failing tests**

Add a repocheck test requiring a job named `Documentation drift` that is unconditional, read-only, time-bounded, and runs `node scripts/dev.mjs docs` after setup. Add drift test fixtures that mention `.github/workflows/copilot-setup-steps.yml`, `.github/copilot-instructions.md`, and `Documentation drift`.

- [ ] **Step 2: Verify RED**

Run:

```powershell
go test ./tools/repocheck -run TestDocumentationDriftGateIsDedicatedAndFailClosed -count=1
npm --prefix frontend run test:lint
```

Expected: FAIL until the workflow and drift checker are updated.

- [ ] **Step 3: Add the job and drift references**

Add a `documentation-drift` job to `.github/workflows/ci.yml` with `name: Documentation drift`, Ubuntu runner, timeout, checkout, setup-go, setup-node, Linux desktop prerequisites, `node scripts/dev.mjs setup`, and `node scripts/dev.mjs docs`.

Update drift fixtures and documentation references so the new surfaces are described.

- [ ] **Step 4: Verify GREEN**

Run:

```powershell
go test ./tools/repocheck -count=1
npm --prefix frontend run test:lint
```

Expected: PASS.

## Task 4: Full validation, PR, ruleset, and evaluation

**Files:**
- All changed files from prior tasks.

- [ ] **Step 1: Run local validation**

Run:

```powershell
node scripts/dev.mjs check
node scripts/dev.mjs verify
```

Expected: PASS.

- [ ] **Step 2: Commit and push**

Commit with:

```powershell
git -c core.hooksPath=.githooks commit -m "ci: improve Copilot operability gates" -m "Add Copilot setup instructions, read-only MCP annotations, and a dedicated documentation drift gate." -m "Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
```

Push a branch and open a PR.

- [ ] **Step 3: Hosted validation and merge**

Wait for CI, CodeQL, Repair verification, and Copilot agent review. Approve with the separate collaborator account and merge normally. Do not create a release.

- [ ] **Step 4: Update ruleset**

After the `Documentation drift` check has passed on the PR/main, add `Documentation drift` to the main ruleset required status checks.

- [ ] **Step 5: Re-evaluate once**

Run:

```powershell
C:\Users\qinqiangxu\.agents\skills\codeblend-ai-composite\ai-readiness-eval.exe eval C:\XQQ\SyncHub-for-Agents-qinqingxu --no-cache
```

Report the actual score and AI-Ready verdict.

## Self-review

- Spec coverage: Covers Copilot instructions, setup steps, MCP read-only annotations, dedicated documentation drift gate, hosted merge, ruleset update, and fresh evaluation.
- Placeholder scan: No placeholders or deferred implementation details remain.
- Type consistency: The only MCP annotation property is `annotations.readOnlyHint`.
