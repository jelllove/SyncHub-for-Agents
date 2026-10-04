# Composite 80+ D4 Improvement Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Improve the current Composite 80.5 result by strengthening D4 continuous-improvement evidence without claiming closed-loop autonomy.

**Architecture:** Add governed learning-lifecycle evidence around the already-recognized recurring Copilot review capability. Keep the existing required review checks and branch protection intact, and make the evidence durable through doc-drift and repocheck guards.

**Tech Stack:** GitHub Actions YAML, GitHub rulesets, Node.js documentation drift checker, Go repocheck tests, Markdown operational docs.

---

## File structure

- `docs/operations/agentic-learning-lifecycle.md`: New lifecycle document describing candidate, active, retired, and rejected rule states.
- `docs/operations/agentic-observability.md`: Link the lifecycle document from the existing observability surface.
- `docs/reports/agentic-review-findings.md`: New evidence index for actual review findings and follow-up validation links.
- `scripts/doc-drift.mjs`: Require the new lifecycle and findings index to remain documented.
- `frontend/doc-drift.test.mjs`: Update synthetic fixtures so drift checks guard the new documents.
- `tools/repocheck/workflow_test.go`: Add tests for the D4 lifecycle and recurring review evidence.

---

### Task 1: D4 lifecycle evidence documents

**Files:**
- Create: `docs/operations/agentic-learning-lifecycle.md`
- Create: `docs/reports/agentic-review-findings.md`
- Modify: `docs/operations/agentic-observability.md`
- Modify: `tools/repocheck/workflow_test.go`

- [ ] **Step 1: Write failing repocheck test**

Add this test to `tools/repocheck/workflow_test.go` after `TestRecurringCopilotReviewWorkflowIsStaticallyDiscoverable`:

```go
func TestAgenticLearningLifecycleIsDocumented(t *testing.T) {
	for _, relative := range []string{
		"docs/operations/agentic-learning-lifecycle.md",
		"docs/reports/agentic-review-findings.md",
	} {
		if _, err := os.Stat(filepath.Join("..", "..", relative)); err != nil {
			t.Fatalf("%s must exist: %v", relative, err)
		}
	}
	lifecycle, err := os.ReadFile(filepath.Join("..", "..", "docs", "operations", "agentic-learning-lifecycle.md"))
	if err != nil {
		t.Fatal(err)
	}
	lifeText := string(lifecycle)
	for _, required := range []string{
		"candidate",
		"active",
		"retired",
		"rejected",
		"Recurring Copilot review",
		"regression validation",
		"agentic-review-findings.md",
	} {
		if !strings.Contains(lifeText, required) {
			t.Fatalf("agentic learning lifecycle must mention %q", required)
		}
	}
	findings, err := os.ReadFile(filepath.Join("..", "..", "docs", "reports", "agentic-review-findings.md"))
	if err != nil {
		t.Fatal(err)
	}
	findingsText := string(findings)
	for _, required := range []string{
		"PR #16",
		"Copilot review finding",
		"follow-up validation",
		"Recurring Copilot review",
	} {
		if !strings.Contains(findingsText, required) {
			t.Fatalf("agentic review findings index must mention %q", required)
		}
	}
}
```

- [ ] **Step 2: Run failing test**

Run:

```powershell
$env:PATH='C:\Program Files\Go\bin;' + $env:PATH
Push-Location C:\XQQ\SyncHub-for-Agents-qinqingxu
go test ./tools/repocheck -run TestAgenticLearningLifecycleIsDocumented -count=1
Pop-Location
```

Expected: FAIL because the lifecycle documents do not exist.

- [ ] **Step 3: Create lifecycle document**

Create `docs/operations/agentic-learning-lifecycle.md` with:

```markdown
# Agentic learning lifecycle

SyncHub uses recurring Copilot review as a supervised learning loop for
repository maintenance. This document is evidence for review adoption and rule
promotion; it is not a claim of autonomous production repair.

## States

- `candidate`: a finding or review pattern has been observed but has not been
  accepted into repository guidance.
- `active`: the pattern has a documented rule, an owner, and regression
  validation in CI or local checks.
- `retired`: the rule no longer applies because the underlying workflow or
  risk changed.
- `rejected`: the finding was reviewed and intentionally not adopted.

## Promotion rules

1. Capture the Copilot review finding in `docs/reports/agentic-review-findings.md`.
2. Link the finding to a pull request, follow-up validation, and the affected
   recurring review or validation surface.
3. Promote a candidate to active only after a maintainer-reviewed change lands
   and `node scripts/dev.mjs verify` or hosted required checks pass.
4. Retire or reject stale entries with a note explaining why the rule no longer
   applies.

## Active rules

| ID | Source | Rule | Validation |
| --- | --- | --- | --- |
| D4-001 | PR #16 Copilot review finding | Local recurring review actions must avoid unresolved shell expansion in the evaluator-visible contract. | `go test ./tools/repocheck -count=1`, `npm --prefix frontend run test:lint`, hosted Recurring Copilot review |

## Ownership

The maintainer reviews every lifecycle change. Recurring Copilot review can
surface candidates, but humans promote, retire, or reject rules.
```

- [ ] **Step 4: Create findings index**

Create `docs/reports/agentic-review-findings.md` with:

```markdown
# Agentic review findings

This index records review findings that changed repository guidance, tests, or
workflows. It provides human-readable evidence for continuous improvement loops;
it does not claim closed-loop autonomy.

| Finding | Source | Outcome | Follow-up validation |
| --- | --- | --- | --- |
| Copilot review finding: the POSIX PR validation workflow needed Linux desktop dependencies. | PR #16 Copilot review | Added the dependency install step before `go test ./...` and `npm test`. | PR #16 required checks passed. |
| Copilot review finding: the shipped MCP server should delegate to the existing implementation instead of duplicating protocol code. | PR #16 Copilot review | `mcp/synchub-validation/server.mjs` delegates to `tools/mcp/validation-server.mjs`. | `npm --prefix frontend run test:lint` covers the shipped wrapper. |
| Recurring Copilot review contract should stay statically discoverable. | PR #17 and Recurring Copilot review | Added `.github/actions/recurring-copilot-review/action.yml` and guarded it with repocheck/doc-drift tests. | Composite 80.5 assessment and hosted Recurring Copilot review. |
```

- [ ] **Step 5: Link lifecycle from observability docs**

In `docs/operations/agentic-observability.md`, add under Artifact contracts:

```markdown
- Repository path `docs/operations/agentic-learning-lifecycle.md`
  ([lifecycle](agentic-learning-lifecycle.md)) and repository path
  `docs/reports/agentic-review-findings.md`
  ([findings](../reports/agentic-review-findings.md)) describe how recurring
  Copilot review findings become candidate, active, retired, or rejected rules.
```

- [ ] **Step 6: Run test and commit**

Run:

```powershell
$env:PATH='C:\Program Files\Go\bin;' + $env:PATH
Push-Location C:\XQQ\SyncHub-for-Agents-qinqingxu
go test ./tools/repocheck -run TestAgenticLearningLifecycleIsDocumented -count=1
Pop-Location
```

Expected: PASS.

Commit:

```powershell
git -C C:\XQQ\SyncHub-for-Agents-qinqingxu add docs\operations\agentic-learning-lifecycle.md docs\reports\agentic-review-findings.md docs\operations\agentic-observability.md tools\repocheck\workflow_test.go
git -C C:\XQQ\SyncHub-for-Agents-qinqingxu commit -m "docs: add agentic learning lifecycle evidence" -m "Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
```

---

### Task 2: Drift guards and validation

**Files:**
- Modify: `scripts/doc-drift.mjs`
- Modify: `frontend/doc-drift.test.mjs`
- Modify: `frontend/validation.test.mjs`
- Modify: `frontend/repair-loop.test.mjs`

- [ ] **Step 1: Extend drift requirements**

In `scripts/doc-drift.mjs`, add these entries to `requiredGuideEntries`:

```js
"docs/operations/agentic-learning-lifecycle.md",
"docs/reports/agentic-review-findings.md",
"candidate",
"active",
"retired",
"rejected",
"Recurring Copilot review",
```

Add explicit file checks near the other `requireContains` calls:

```js
const lifecycle = read(root, "docs/operations/agentic-learning-lifecycle.md");
for (const state of ["candidate", "active", "retired", "rejected"]) {
  requireContains(lifecycle, state, "agentic-learning-lifecycle.md");
}
requireContains(read(root, "docs/reports/agentic-review-findings.md"), "Copilot review finding", "agentic-review-findings.md");
```

- [ ] **Step 2: Update synthetic fixtures**

In `frontend/doc-drift.test.mjs`, `frontend/validation.test.mjs`, and
`frontend/repair-loop.test.mjs`, update the synthetic `agentic-observability.md`
strings so they mention:

```text
docs/operations/agentic-learning-lifecycle.md docs/reports/agentic-review-findings.md candidate active retired rejected Recurring Copilot review
```

Create synthetic files with these contents:

```js
"docs/operations/agentic-learning-lifecycle.md": "candidate active retired rejected Recurring Copilot review regression validation agentic-review-findings.md\n",
"docs/reports/agentic-review-findings.md": "PR #16 Copilot review finding follow-up validation Recurring Copilot review\n",
```

- [ ] **Step 3: Run targeted tests**

Run:

```powershell
npm --prefix C:\XQQ\SyncHub-for-Agents-qinqingxu\frontend run test:lint
npm --prefix C:\XQQ\SyncHub-for-Agents-qinqingxu\frontend test -- validation.test.mjs repair-loop.test.mjs
$env:PATH='C:\Program Files\Go\bin;' + $env:PATH
Push-Location C:\XQQ\SyncHub-for-Agents-qinqingxu
go test ./tools/repocheck -count=1
node scripts/dev.mjs docs
Pop-Location
```

Expected: all PASS.

- [ ] **Step 4: Commit drift guards**

Commit:

```powershell
git -C C:\XQQ\SyncHub-for-Agents-qinqingxu add scripts\doc-drift.mjs frontend\doc-drift.test.mjs frontend\validation.test.mjs frontend\repair-loop.test.mjs
git -C C:\XQQ\SyncHub-for-Agents-qinqingxu commit -m "test: guard agentic learning evidence" -m "Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
```

---

### Task 3: Score verification and push

**Files:**
- No source files beyond Tasks 1 and 2.

- [ ] **Step 1: Run full local verification**

Run:

```powershell
$env:PATH='C:\Program Files\Go\bin;' + $env:PATH
Push-Location C:\XQQ\SyncHub-for-Agents-qinqingxu
node scripts/dev.mjs check
node scripts/dev.mjs verify
Pop-Location
```

Expected: both commands pass.

- [ ] **Step 2: Run CodeBlend once**

Run:

```powershell
$TargetPath=(git -C C:\XQQ\SyncHub-for-Agents-qinqingxu rev-parse --show-toplevel).Trim()
& C:\Users\qinqiangxu\.agents\skills\codeblend-ai-composite\ai-readiness-eval.exe eval $TargetPath --no-cache
```

Expected: command completes. Report the actual Composite/Operation result. A successful target is Composite above 80.5 or D4 remains at least 2/4.

- [ ] **Step 3: Push**

Run:

```powershell
git -C C:\XQQ\SyncHub-for-Agents-qinqingxu push origin main
```

Expected: push succeeds.
