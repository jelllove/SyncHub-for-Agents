# Operation Closed-Loop Evidence Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add real, repository-owned Operation evidence for rollback handoff, validation command discovery, and MCP server discoverability without changing SyncHub desktop runtime behavior.

**Architecture:** Extend the existing review-only maintenance proposal path so it emits and verifies a rollback patch alongside the repair patch. Wire that evidence into existing read-only workflows, Taskfile validation tasks, MCP server naming, and documentation so both humans and automated evidence collectors can discover the same commands and artifacts.

**Tech Stack:** Node.js ESM scripts, Vitest, Go `node:test`-style repository checks through `go test`, GitHub Actions YAML, Taskfile v3, MCP JSON-RPC stdio server.

---

## File structure

- `scripts/maintenance.mjs`: Generate `rollback.patch`, verify it, and record structured rollback metadata in `proposal.json`.
- `frontend/maintenance.test.mjs`: Assert rollback artifacts are present, applicable, reversible, size-bounded, and absent on no-change/failure reports.
- `.github/workflows/self-healing.yml`: Rename the proposal step/artifact language to bounded repair plus rollback handoff while keeping read-only permissions.
- `Taskfile.yml`: Add repository validation tasks that expose `setup`, `check`, `verify`, `docs`, and `repair:verify` commands.
- `tools/mcp/synchub-mcp-server.mjs`: Add a committed wrapper whose filename makes the shipped repository MCP server discoverable while delegating to the existing implementation.
- `.vscode/mcp.json`: Point the MCP config at the discoverable wrapper.
- `tools/mcp/validation-server.mjs`: Keep the existing server implementation; no runtime logic changes unless tests require command text updates.
- `frontend/mcp-server.test.mjs`: Assert the MCP config points to the wrapper and the listed validation commands remain complete.
- `tools/repocheck/workflow_test.go`: Assert the self-healing workflow publishes rollback handoff evidence and Taskfile exposes repository validation commands.
- `docs/development.md`, `docs/operations/agentic-observability.md`, `docs/runbooks/ci-failure-response.md`, `.agents/skills/synchub-validation/SKILL.md`, `AGENTS.md`: Keep human-facing command and artifact contracts aligned.

---

### Task 1: Maintenance rollback artifact

**Files:**
- Modify: `frontend/maintenance.test.mjs`
- Modify: `scripts/maintenance.mjs`

- [ ] **Step 1: Write the failing rollback artifact test**

Replace the assertion block in `frontend/maintenance.test.mjs` inside the test named `produces an applicable, reversible patch without modifying existing work or unrelated files` with:

```js
    expect(report.verification).toEqual({
      patchApplies: true,
      canonical: true,
      rollbackApplies: true,
      rollbackRestoresOriginal: true,
      scope: "format-and-generated-reference",
    });
    const rollback = path.join(directory, "rollback.patch");
    expect(existsSync(rollback)).toBe(true);
```

Then after the existing `const patch = path.join(directory, "repair.patch");` line, add:

```js
    run("git", ["apply", "--check", patch], root);
    run("git", ["apply", patch], root);
    run("git", ["apply", "--check", rollback], root);
    run("git", ["apply", rollback], root);
    expect(readFileSync(path.join(root, "main.go"), "utf8").replaceAll("\r\n", "\n")).toBe(originalGo.replaceAll("\r\n", "\n"));
    run("git", ["apply", patch], root);
```

Also update the `no-change` and failure tests so they assert `rollback.patch` is absent:

```js
    expect(existsSync(path.join(directory, "rollback.patch"))).toBe(false);
```

- [ ] **Step 2: Run the targeted test and confirm failure**

Run:

```powershell
npm --prefix frontend test -- maintenance.test.mjs
```

Expected: FAIL because `report.verification` does not include rollback fields and `rollback.patch` is missing.

- [ ] **Step 3: Generate and verify rollback.patch**

In `scripts/maintenance.mjs`, change `preparePatch` so it returns both patch strings:

```js
function preparePatch(root, scratch, changes, maxPatchBytes) {
  const beforeRoot = path.join(scratch, "a");
  const afterRoot = path.join(scratch, "b");
  for (const change of changes) {
    for (const [tree, text] of [["a", change.before], ["b", change.after]]) {
      const filename = path.join(scratch, tree, change.path);
      mkdirSync(path.dirname(filename), { recursive: true });
      writeFileSync(filename, text);
    }
  }
  const patch = run("git", [
    "--no-pager", "-c", "core.autocrlf=false", "diff", "--no-color", "--no-index", "--no-prefix", "--binary",
    "--no-ext-diff", "--no-textconv", "--", "a", "b",
  ], scratch, { allowedExitCodes: [0, 1] });
  const rollback = run("git", [
    "--no-pager", "-c", "core.autocrlf=false", "diff", "--no-color", "--no-index", "--no-prefix", "--binary",
    "--no-ext-diff", "--no-textconv", "--", "b", "a",
  ], scratch, { allowedExitCodes: [0, 1] });
  if (!patch || Buffer.byteLength(patch) > maxPatchBytes || !rollback || Buffer.byteLength(rollback) > maxPatchBytes) {
    throw new Error(`Maintenance patch size limit exceeded or empty patch (${maxPatchBytes} bytes)`);
  }
  const patchFile = path.join(scratch, "candidate.patch");
  const rollbackFile = path.join(scratch, "rollback.patch");
  writeFileSync(patchFile, patch);
  writeFileSync(rollbackFile, rollback);
  run("git", ["init", "--quiet"], beforeRoot);
  run("git", ["-c", "core.autocrlf=false", "apply", "--check", patchFile], beforeRoot);
  run("git", ["-c", "core.autocrlf=false", "apply", patchFile], beforeRoot);
  for (const change of changes) {
    const applied = readText(path.join(beforeRoot, change.path));
    if (applied !== change.after) throw new Error(`Patch verification differs for ${change.path}`);
    const canonical = change.kind === "gofmt"
      ? formattedGo(applied, root)
      : replaceReference(applied.replaceAll("\r\n", "\n"), developmentReference(root));
    if (canonical.replaceAll("\r\n", "\n") !== applied.replaceAll("\r\n", "\n")) {
      throw new Error(`Repair is not canonical for ${change.path}`);
    }
    if (!ownedFile(root, change.path) || readText(path.join(root, change.path)) !== change.before) {
      throw new Error(`Source changed while preparing proposal: ${change.path}`);
    }
  }
  run("git", ["-c", "core.autocrlf=false", "apply", "--check", rollbackFile], beforeRoot);
  run("git", ["-c", "core.autocrlf=false", "apply", rollbackFile], beforeRoot);
  for (const change of changes) {
    const restored = readText(path.join(beforeRoot, change.path));
    if (restored !== change.before) throw new Error(`Rollback verification differs for ${change.path}`);
  }
  run("git", ["-c", "core.autocrlf=false", "apply", "--check", patchFile], beforeRoot);
  return { patch, rollback };
}
```

Update the initial `report.verification` object:

```js
    verification: {
      patchApplies: false,
      canonical: false,
      rollbackApplies: false,
      rollbackRestoresOriginal: false,
      scope: "format-and-generated-reference",
    },
```

Update the success branch:

```js
      const { patch, rollback } = preparePatch(root, scratch, changes, maxPatchBytes);
      writeFileSync(path.join(directory, "repair.patch"), patch);
      writeFileSync(path.join(directory, "rollback.patch"), rollback);
      report.verification.patchApplies = true;
      report.verification.canonical = true;
      report.verification.rollbackApplies = true;
      report.verification.rollbackRestoresOriginal = true;
      report.status = "proposed";
```

- [ ] **Step 4: Run the targeted test and confirm pass**

Run:

```powershell
npm --prefix frontend test -- maintenance.test.mjs
```

Expected: PASS.

- [ ] **Step 5: Commit rollback artifact support**

Run:

```powershell
git add scripts/maintenance.mjs frontend/maintenance.test.mjs
git commit -m "feat: publish rollback handoff artifacts" -m "Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
```

---

### Task 2: Validation and MCP discoverability surfaces

**Files:**
- Modify: `Taskfile.yml`
- Create: `tools/mcp/synchub-mcp-server.mjs`
- Modify: `.vscode/mcp.json`
- Modify: `frontend/mcp-server.test.mjs`
- Modify: `tools/repocheck/workflow_test.go`

- [ ] **Step 1: Write failing tests for validation tasks and MCP wrapper**

In `frontend/mcp-server.test.mjs`, add:

```js
import { readFileSync } from "node:fs";
```

Then add this test before the existing MCP server behavior test:

```js
test("repository MCP config points at the shipped SyncHub validation server wrapper", () => {
  const config = JSON.parse(readFileSync(path.join(root, ".vscode", "mcp.json"), "utf8"));
  assert.equal(config.servers["synchub-validation"].command, "node");
  assert.deepEqual(config.servers["synchub-validation"].args, ["tools/mcp/synchub-mcp-server.mjs"]);
});
```

In `tools/repocheck/workflow_test.go`, add imports already present are enough. Add this test after `TestEvidenceConfigurationFilesAreCommitted`:

```go
func TestRepositoryValidationTasksAreDiscoverable(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "Taskfile.yml"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, required := range []string{
		"repo:setup:",
		"node scripts/dev.mjs setup",
		"repo:check:",
		"node scripts/dev.mjs check",
		"repo:verify:",
		"node scripts/dev.mjs verify",
		"repo:docs:",
		"node scripts/dev.mjs docs",
		"repo:repair:verify:",
		"node scripts/dev.mjs repair:verify",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("Taskfile.yml must expose repository validation command %q", required)
		}
	}
}
```

Update `TestEvidenceConfigurationFilesAreCommitted` to include:

```go
		"tools/mcp/synchub-mcp-server.mjs",
```

- [ ] **Step 2: Run targeted tests and confirm failure**

Run:

```powershell
npm --prefix frontend run test:lint
go test ./tools/repocheck -count=1
```

Expected: FAIL because the wrapper file and Taskfile tasks do not exist.

- [ ] **Step 3: Add validation tasks to Taskfile**

Append to `Taskfile.yml` under `tasks:`:

```yaml
  repo:setup:
    summary: Restores locked dependencies and embedded frontend assets
    cmds:
      - node scripts/dev.mjs setup

  repo:check:
    summary: Runs repository formatting, vet, lint, type, docs, and evidence checks
    cmds:
      - node scripts/dev.mjs check

  repo:verify:
    summary: Runs source-bound repository validation and writes report artifacts
    cmds:
      - node scripts/dev.mjs verify

  repo:docs:
    summary: Checks Markdown links, generated references, and evidence drift
    cmds:
      - node scripts/dev.mjs docs

  repo:repair:verify:
    summary: Verifies the diagnostic repair and rollback cycle in containment
    cmds:
      - node scripts/dev.mjs repair:verify
```

- [ ] **Step 4: Add the shipped MCP wrapper**

Create `tools/mcp/synchub-mcp-server.mjs`:

```js
#!/usr/bin/env node
import "./validation-server.mjs";
```

Change `.vscode/mcp.json`:

```json
{
  "servers": {
    "synchub-validation": {
      "type": "stdio",
      "command": "node",
      "args": ["tools/mcp/synchub-mcp-server.mjs"]
    }
  }
}
```

- [ ] **Step 5: Run targeted tests and confirm pass**

Run:

```powershell
npm --prefix frontend run test:lint
go test ./tools/repocheck -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit validation surface changes**

Run:

```powershell
git add Taskfile.yml .vscode/mcp.json tools/mcp/synchub-mcp-server.mjs frontend/mcp-server.test.mjs tools/repocheck/workflow_test.go
git commit -m "ci: expose repository validation surfaces" -m "Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
```

---

### Task 3: Workflow/docs alignment and final verification

**Files:**
- Modify: `.github/workflows/self-healing.yml`
- Modify: `tools/repocheck/workflow_test.go`
- Modify: `docs/development.md`
- Modify: `docs/operations/agentic-observability.md`
- Modify: `docs/runbooks/ci-failure-response.md`
- Modify: `.agents/skills/synchub-validation/SKILL.md`
- Modify: `AGENTS.md`

- [ ] **Step 1: Write failing workflow contract test**

In `tools/repocheck/workflow_test.go`, update `TestSelfHealingDiagnosticsWorkflowIsReadOnlyAndReviewOnly` so the final assertion block becomes:

```go
	if !strings.Contains(joined, "node scripts/dev.mjs propose") ||
		!strings.Contains(joined, "bounded repair and rollback handoff") ||
		!strings.Contains(joined, "ci-failure-response") {
		t.Fatal("self-healing diagnostics must publish the existing bounded repair and rollback handoff")
	}
```

- [ ] **Step 2: Run the targeted Go test and confirm failure**

Run:

```powershell
go test ./tools/repocheck -run TestSelfHealingDiagnosticsWorkflowIsReadOnlyAndReviewOnly -count=1
```

Expected: FAIL because the workflow does not use the required handoff wording yet.

- [ ] **Step 3: Update workflow wording**

In `.github/workflows/self-healing.yml`, rename:

```yaml
      - name: Prepare bounded review-only repair proposal
```

to:

```yaml
      - name: Prepare bounded repair and rollback handoff
```

Keep `run: node scripts/dev.mjs propose` and permissions unchanged.

- [ ] **Step 4: Update docs and skill guidance**

Ensure each changed document says `node scripts/dev.mjs propose` emits both `repair.patch` and `rollback.patch` when changes are proposed, and that both are review-only:

```md
`node scripts/dev.mjs propose` prepares a bounded review-only repair patch and rollback patch.
```

Use this exact meaning in:

- `AGENTS.md`
- `.agents/skills/synchub-validation/SKILL.md`
- `docs/development.md`
- `docs/operations/agentic-observability.md`
- `docs/runbooks/ci-failure-response.md`

- [ ] **Step 5: Run docs generation/checks**

Run:

```powershell
node scripts/dev.mjs docs:write
node scripts/dev.mjs docs
go test ./tools/repocheck -count=1
```

Expected: PASS.

- [ ] **Step 6: Run full verification**

Run:

```powershell
npm --prefix frontend run test:lint
npm --prefix frontend test -- maintenance.test.mjs validation.test.mjs repair-loop.test.mjs
node scripts/dev.mjs check
node scripts/dev.mjs verify
```

Expected: PASS.

- [ ] **Step 7: Run one CodeBlend evaluation**

Run:

```powershell
& C:\Users\qinqiangxu\.agents\skills\codeblend-ai-composite\ai-readiness-eval.exe eval C:\XQQ\SyncHub-for-Agents-qinqingxu --no-cache
```

Expected: The command completes and prints a run directory. Report the actual score and evidence fields; do not claim AI-Ready if the evaluator still says no.

- [ ] **Step 8: Commit final alignment**

Run:

```powershell
git add .github/workflows/self-healing.yml AGENTS.md .agents/skills/synchub-validation/SKILL.md docs/development.md docs/operations/agentic-observability.md docs/runbooks/ci-failure-response.md tools/repocheck/workflow_test.go
git commit -m "docs: align operation rollback evidence" -m "Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
```
