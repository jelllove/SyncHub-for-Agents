# AI Readiness Evidence Pack Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add truthful, non-blocking AI-readiness evidence for environment setup, report contracts, workflow observability, and documentation drift without changing SyncHub application behavior.

**Architecture:** Keep runtime validation unchanged and add a read-only documentation/evidence layer around it. A small Node checker verifies that committed specs, labels, workflow artifacts, and documentation references stay aligned; Go repocheck tests guard workflow/configuration contracts.

**Tech Stack:** Node.js ESM scripts, Node's built-in test runner, Go repocheck tests, GitHub Actions YAML, Markdown, JSON Schema.

---

## File structure

- Create `.env.example` to document optional non-secret development inputs.
- Create `.github/labels.yml` to declare workflow/agent routing labels.
- Create `docs/specs/validation-receipt.v1.schema.json` to version validation receipts.
- Create `docs/specs/repair-proof.v1.schema.json` to version contained repair proof receipts.
- Create `docs/operations/agentic-observability.md` to explain workflows, artifacts, labels, and human handoff.
- Create `scripts/doc-drift.mjs` as a focused, read-only documentation/evidence drift checker.
- Modify `scripts/dev-lib.mjs` to expose `checkEvidenceDrift(root)`.
- Modify `scripts/dev.mjs` to run `checkEvidenceDrift(root)` in `check` and `docs`.
- Create `frontend/doc-drift.test.mjs` with synthetic fixtures for the checker.
- Modify `frontend/package.json` so `test:lint` includes the new Node test.
- Modify `frontend/validation.test.mjs` fixture-copy list if the new checker is imported by scripts copied into synthetic repositories.
- Modify `tools/repocheck/workflow_test.go` to guard committed labels/specs and documented artifact contracts.
- Modify `docs/development.md` to document the evidence contracts and drift check.

## Task 1: Write failing drift-checker tests

**Files:**
- Create: `frontend/doc-drift.test.mjs`

- [ ] **Step 1: Add the failing Node tests**

Create `frontend/doc-drift.test.mjs` with this content:

```javascript
import test from "node:test";
import assert from "node:assert/strict";
import path from "node:path";
import { mkdtempSync, mkdirSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";

import { checkEvidenceDrift } from "../scripts/dev-lib.mjs";

function writeFixture(root, overrides = {}) {
  const files = {
    ".env.example": [
      "# SyncHub development environment",
      "SYNCHUB_GITHUB_CLIENT_ID=",
      "ACSYNC_GITHUB_CLIENT_ID=",
      "SYNCHUB_RUN_HELPER_INTEGRATION=0",
      "",
    ].join("\n"),
    ".github/labels.yml": [
      "- name: ai-readiness",
      "  color: \"5319e7\"",
      "  description: AI readiness evaluation or evidence work",
      "- name: validation",
      "  color: \"0e8a16\"",
      "  description: Native validation, CI, or test evidence",
      "- name: repair-proof",
      "  color: \"d93f0b\"",
      "  description: Diagnostic repair and rollback verification evidence",
      "- name: agent-review",
      "  color: \"1d76db\"",
      "  description: Agent-assisted review, prompt, or handoff workflow",
      "",
    ].join("\n"),
    "docs/specs/validation-receipt.v1.schema.json": JSON.stringify({
      "$schema": "https://json-schema.org/draft/2020-12/schema",
      "$id": "https://github.com/jelllove/SyncHub-for-Agents/schemas/validation-receipt.v1.schema.json",
      "title": "SyncHub validation receipt v1",
      "type": "object",
      "required": ["version", "status", "checks", "source"],
      "properties": {
        "version": { "const": 1 },
        "status": { "enum": ["passed", "failed", "running"] },
        "checks": { "type": "array" },
        "source": { "type": "object" }
      }
    }, null, 2) + "\n",
    "docs/specs/repair-proof.v1.schema.json": JSON.stringify({
      "$schema": "https://json-schema.org/draft/2020-12/schema",
      "$id": "https://github.com/jelllove/SyncHub-for-Agents/schemas/repair-proof.v1.schema.json",
      "title": "SyncHub contained repair proof v1",
      "type": "object",
      "required": ["scenario", "status", "baseline", "repair", "rollback"],
      "properties": {
        "scenario": { "const": "diagnostic-go-format" },
        "status": { "enum": ["passed", "failed"] },
        "baseline": { "type": "object" },
        "repair": { "type": "object" },
        "rollback": { "type": "object" }
      }
    }, null, 2) + "\n",
    "docs/operations/agentic-observability.md": [
      "# Agentic observability",
      "",
      "Labels: ai-readiness, validation, repair-proof, agent-review.",
      "",
      "Workflows: .github/workflows/ci.yml, .github/workflows/maintenance.yml, .github/workflows/repair-verification.yml.",
      "",
      "Artifacts: repository-validation, maintenance-proposal, native-repair-proof.",
      "",
      "Schemas: docs/specs/validation-receipt.v1.schema.json and docs/specs/repair-proof.v1.schema.json.",
      "",
      "Commands: node scripts/dev.mjs verify and node scripts/dev.mjs repair:verify.",
      "",
    ].join("\n"),
    ".github/workflows/ci.yml": [
      "name: CI",
      "jobs:",
      "  repository:",
      "    steps:",
      "      - run: node scripts/dev.mjs verify",
      "      - uses: actions/upload-artifact@ea165f8d65b6e75b540449e92b4886f43607fa02",
      "        with:",
      "          name: repository-validation",
      "      - uses: actions/upload-artifact@ea165f8d65b6e75b540449e92b4886f43607fa02",
      "        with:",
      "          name: maintenance-proposal",
      "",
    ].join("\n"),
    ".github/workflows/maintenance.yml": "name: Repository maintenance\n",
    ".github/workflows/repair-verification.yml": [
      "name: Repair verification",
      "jobs:",
      "  repair:",
      "    steps:",
      "      - run: node scripts/dev.mjs repair:verify",
      "      - uses: actions/upload-artifact@ea165f8d65b6e75b540449e92b4886f43607fa02",
      "        with:",
      "          name: native-repair-proof",
      "",
    ].join("\n"),
  };
  for (const [relative, content] of Object.entries({ ...files, ...overrides })) {
    const full = path.join(root, relative);
    mkdirSync(path.dirname(full), { recursive: true });
    writeFileSync(full, content);
  }
}

test("checkEvidenceDrift accepts complete evidence documentation", () => {
  const root = mkdtempSync(path.join(tmpdir(), "synchub-doc-drift-"));
  writeFixture(root);
  assert.doesNotThrow(() => checkEvidenceDrift(root));
});

test("checkEvidenceDrift rejects missing label configuration", () => {
  const root = mkdtempSync(path.join(tmpdir(), "synchub-doc-drift-"));
  writeFixture(root, { ".github/labels.yml": "- name: ai-readiness\n" });
  assert.throws(
    () => checkEvidenceDrift(root),
    /labels.yml must define label "validation"/,
  );
});

test("checkEvidenceDrift rejects undocumented repair artifact", () => {
  const root = mkdtempSync(path.join(tmpdir(), "synchub-doc-drift-"));
  writeFixture(root, {
    "docs/operations/agentic-observability.md": [
      "# Agentic observability",
      "Labels: ai-readiness, validation, repair-proof, agent-review.",
      "Workflows: .github/workflows/ci.yml, .github/workflows/maintenance.yml, .github/workflows/repair-verification.yml.",
      "Artifacts: repository-validation and maintenance-proposal.",
      "Schemas: docs/specs/validation-receipt.v1.schema.json and docs/specs/repair-proof.v1.schema.json.",
      "Commands: node scripts/dev.mjs verify and node scripts/dev.mjs repair:verify.",
      "",
    ].join("\n"),
  });
  assert.throws(
    () => checkEvidenceDrift(root),
    /agentic-observability.md must mention "native-repair-proof"/,
  );
});
```

- [ ] **Step 2: Run the new test and verify RED**

Run:

```powershell
npm --prefix frontend exec -- node --test doc-drift.test.mjs
```

Expected: FAIL with an import error or `The requested module '../scripts/dev-lib.mjs' does not provide an export named 'checkEvidenceDrift'`.

## Task 2: Implement the read-only drift checker

**Files:**
- Create: `scripts/doc-drift.mjs`
- Modify: `scripts/dev-lib.mjs`
- Modify: `scripts/dev.mjs`
- Modify: `frontend/package.json`

- [ ] **Step 1: Create `scripts/doc-drift.mjs`**

Add this content:

```javascript
import path from "node:path";
import { readFileSync } from "node:fs";

const requiredLabels = ["ai-readiness", "validation", "repair-proof", "agent-review"];
const requiredDocs = [
  "docs/specs/validation-receipt.v1.schema.json",
  "docs/specs/repair-proof.v1.schema.json",
  ".github/workflows/ci.yml",
  ".github/workflows/maintenance.yml",
  ".github/workflows/repair-verification.yml",
  "repository-validation",
  "maintenance-proposal",
  "native-repair-proof",
  "node scripts/dev.mjs verify",
  "node scripts/dev.mjs repair:verify",
];

function read(root, relative) {
  return readFileSync(path.join(root, relative), "utf8");
}

function requireContains(content, needle, source) {
  if (!content.includes(needle)) {
    throw new Error(`${source} must mention "${needle}"`);
  }
}

function requireJsonSchema(root, relative, title, requiredKeys) {
  const parsed = JSON.parse(read(root, relative));
  if (parsed.$schema !== "https://json-schema.org/draft/2020-12/schema") {
    throw new Error(`${relative} must use JSON Schema draft 2020-12`);
  }
  if (parsed.title !== title) {
    throw new Error(`${relative} must have title "${title}"`);
  }
  for (const key of requiredKeys) {
    if (!parsed.required?.includes(key)) {
      throw new Error(`${relative} must require "${key}"`);
    }
  }
}

export function checkEvidenceDrift(root) {
  const labels = read(root, ".github/labels.yml");
  for (const label of requiredLabels) {
    requireContains(labels, `name: ${label}`, "labels.yml");
  }

  requireJsonSchema(root, "docs/specs/validation-receipt.v1.schema.json", "SyncHub validation receipt v1", [
    "version",
    "status",
    "checks",
    "source",
  ]);
  requireJsonSchema(root, "docs/specs/repair-proof.v1.schema.json", "SyncHub contained repair proof v1", [
    "scenario",
    "status",
    "baseline",
    "repair",
    "rollback",
  ]);

  const guide = read(root, "docs/operations/agentic-observability.md");
  for (const entry of requiredLabels.concat(requiredDocs)) {
    requireContains(guide, entry, "agentic-observability.md");
  }

  requireContains(read(root, ".github/workflows/ci.yml"), "repository-validation", "ci.yml");
  requireContains(read(root, ".github/workflows/ci.yml"), "maintenance-proposal", "ci.yml");
  requireContains(read(root, ".github/workflows/repair-verification.yml"), "native-repair-proof", "repair-verification.yml");
}
```

- [ ] **Step 2: Export the checker from `scripts/dev-lib.mjs`**

Add this import near the other imports:

```javascript
import { checkEvidenceDrift } from "./doc-drift.mjs";
```

Add this export near the bottom with the existing exports:

```javascript
export { checkEvidenceDrift };
```

- [ ] **Step 3: Wire the checker into `scripts/dev.mjs`**

Add `checkEvidenceDrift` to the import from `./dev-lib.mjs`.

In the `check` case, after `checkReference(root);`, add:

```javascript
checkEvidenceDrift(root);
```

In the `docs` case, after `checkReference(root);`, add:

```javascript
checkEvidenceDrift(root);
```

- [ ] **Step 4: Include the drift test in `frontend/package.json`**

Change the `test:lint` script from:

```json
"test:lint": "node --test lint.test.mjs"
```

to:

```json
"test:lint": "node --test lint.test.mjs doc-drift.test.mjs"
```

- [ ] **Step 5: Run the new test and verify it still fails for missing evidence files**

Run:

```powershell
npm --prefix frontend exec -- node --test doc-drift.test.mjs
```

Expected: FAIL because the repository root lacks `.env.example`, labels, schemas, or observability guide.

## Task 3: Add committed evidence files

**Files:**
- Create: `.env.example`
- Create: `.github/labels.yml`
- Create: `docs/specs/validation-receipt.v1.schema.json`
- Create: `docs/specs/repair-proof.v1.schema.json`
- Create: `docs/operations/agentic-observability.md`
- Modify: `docs/development.md`

- [ ] **Step 1: Add `.env.example`**

Create:

```text
# SyncHub development environment
# Copy to .env.local or export values in your shell if you need local overrides.
# Do not put secrets in this template.

# Optional GitHub OAuth client ID for local development builds.
SYNCHUB_GITHUB_CLIENT_ID=

# Legacy compatibility name read by older local builds.
ACSYNC_GITHUB_CLIENT_ID=

# Windows updater helper integration tests are opt-in because they spawn helper
# processes and require an isolated temporary root.
SYNCHUB_RUN_HELPER_INTEGRATION=0
```

- [ ] **Step 2: Add `.github/labels.yml`**

Create:

```yaml
- name: ai-readiness
  color: "5319e7"
  description: AI readiness evaluation, evidence, or methodology work
- name: validation
  color: "0e8a16"
  description: Native validation, CI, test, or packaging evidence
- name: repair-proof
  color: "d93f0b"
  description: Diagnostic repair and rollback verification evidence
- name: agent-review
  color: "1d76db"
  description: Agent-assisted review, prompt, or supervised handoff workflow
- name: documentation-drift
  color: "c5def5"
  description: Documentation and executable behavior alignment
```

- [ ] **Step 3: Add `docs/specs/validation-receipt.v1.schema.json`**

Use a compact JSON Schema with required keys:

```json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "https://github.com/jelllove/SyncHub-for-Agents/schemas/validation-receipt.v1.schema.json",
  "title": "SyncHub validation receipt v1",
  "type": "object",
  "additionalProperties": true,
  "required": ["version", "status", "checks", "source"],
  "properties": {
    "version": { "const": 1 },
    "status": { "enum": ["passed", "failed", "running"] },
    "checks": {
      "type": "array",
      "items": {
        "type": "object",
        "required": ["id", "status"],
        "properties": {
          "id": { "type": "string" },
          "status": { "enum": ["passed", "failed", "not-run"] },
          "durationMs": { "type": ["integer", "null"], "minimum": 0 },
          "log": { "type": "string" }
        },
        "additionalProperties": true
      }
    },
    "source": {
      "type": "object",
      "required": ["status"],
      "properties": {
        "status": { "enum": ["unchanged", "changed", "error", "not-captured"] },
        "changedPaths": { "type": "array", "items": { "type": "string" } },
        "error": { "type": "string" }
      },
      "additionalProperties": true
    }
  }
}
```

- [ ] **Step 4: Add `docs/specs/repair-proof.v1.schema.json`**

Use a compact JSON Schema with required keys:

```json
{
  "$schema": "https://json-schema.org/draft/2020-12/schema",
  "$id": "https://github.com/jelllove/SyncHub-for-Agents/schemas/repair-proof.v1.schema.json",
  "title": "SyncHub contained repair proof v1",
  "type": "object",
  "additionalProperties": true,
  "required": ["scenario", "status", "baseline", "repair", "rollback"],
  "properties": {
    "scenario": { "const": "diagnostic-go-format" },
    "status": { "enum": ["passed", "failed"] },
    "productionIncident": { "const": false },
    "baseline": { "type": "object" },
    "fault": { "type": "object" },
    "repair": { "type": "object" },
    "rollback": { "type": "object" },
    "originalSourceUnchanged": { "type": "boolean" },
    "errors": { "type": "array", "items": { "type": "string" } }
  }
}
```

- [ ] **Step 5: Add `docs/operations/agentic-observability.md`**

Create a concise guide that explicitly mentions:

```markdown
# Agentic observability

SyncHub records agent-assisted repository work through pull requests, hosted
checks, validation artifacts, labels, and human review notes. These surfaces make
the work observable; they do not claim production autonomous repair.

## Workflows and commands

- `.github/workflows/ci.yml` runs `node scripts/dev.mjs verify`, publishes
  `repository-validation`, and publishes `maintenance-proposal` only after a
  failed validation run produces a bounded review-only patch.
- `.github/workflows/maintenance.yml` schedules the same repository/security and
  Linux test checks without write permissions or packaging.
- `.github/workflows/repair-verification.yml` runs
  `node scripts/dev.mjs repair:verify` and publishes `native-repair-proof`.

## Artifact contracts

- `docs/specs/validation-receipt.v1.schema.json` describes validation receipts
  written by `node scripts/dev.mjs verify`.
- `docs/specs/repair-proof.v1.schema.json` describes diagnostic repair proof
  receipts written by `node scripts/dev.mjs repair:verify`.
- `repository-validation`, `maintenance-proposal`, and `native-repair-proof`
  artifacts are retained by GitHub Actions for bounded review windows.

## Labels and handoff

- `ai-readiness`: score reports, evidence gaps, and methodology work.
- `validation`: native checks, CI failures, and packaging evidence.
- `repair-proof`: diagnostic fault injection, repair validation, and rollback evidence.
- `agent-review`: agent-assisted review comments, prompts, and supervised handoff.
- `documentation-drift`: documentation/reference mismatches found by checks.

Human reviewers should link failed and passing workflow runs in pull requests,
verify the report `status` before trusting individual command rows, and treat
repair proposals as patches requiring review rather than automatic fixes.
```

- [ ] **Step 6: Update `docs/development.md`**

Add one paragraph under the validation/reporting section:

```markdown
The committed evidence contracts live in [validation receipt schema](specs/validation-receipt.v1.schema.json),
[repair proof schema](specs/repair-proof.v1.schema.json), and
[agentic observability](operations/agentic-observability.md). `node scripts/dev.mjs docs`
checks that those files still reference the workflows, labels, artifact names,
and command entry points that publish the evidence.
```

## Task 4: Make tests green and add Go workflow guards

**Files:**
- Modify: `tools/repocheck/workflow_test.go`

- [ ] **Step 1: Run the drift test**

Run:

```powershell
npm --prefix frontend exec -- node --test doc-drift.test.mjs
```

Expected: PASS.

- [ ] **Step 2: Add Go workflow/config guards**

Append tests that:

```go
func TestEvidenceConfigurationFilesAreCommitted(t *testing.T) {
	for _, relative := range []string{
		".env.example",
		".github/labels.yml",
		"docs/specs/validation-receipt.v1.schema.json",
		"docs/specs/repair-proof.v1.schema.json",
		"docs/operations/agentic-observability.md",
	} {
		if _, err := os.Stat(filepath.Join("..", "..", relative)); err != nil {
			t.Fatalf("%s must exist: %v", relative, err)
		}
	}
}

func TestEvidenceArtifactsRemainDocumentedAndPublished(t *testing.T) {
	ciData, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", "ci.yml"))
	if err != nil {
		t.Fatal(err)
	}
	repairData, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", "repair-verification.yml"))
	if err != nil {
		t.Fatal(err)
	}
	guide, err := os.ReadFile(filepath.Join("..", "..", "docs", "operations", "agentic-observability.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"repository-validation", "maintenance-proposal"} {
		if !strings.Contains(string(ciData), required) || !strings.Contains(string(guide), required) {
			t.Fatalf("%s must be published by CI and documented", required)
		}
	}
	if !strings.Contains(string(repairData), "native-repair-proof") || !strings.Contains(string(guide), "native-repair-proof") {
		t.Fatal("native repair proof must be published and documented")
	}
}
```

- [ ] **Step 3: Run Go repocheck**

Run:

```powershell
go test ./tools/repocheck -count=1
```

Expected: PASS.

## Task 5: Full local validation and commit

**Files:**
- All files changed by Tasks 1-4.

- [ ] **Step 1: Run repository-standard checks**

Run:

```powershell
node scripts/dev.mjs check
```

Expected: PASS.

- [ ] **Step 2: Inspect the deterministic evidence pack**

Run:

```powershell
C:\Users\qinqiangxu\.agents\skills\codeblend-ai-composite\ai-readiness-eval.exe evidence-pack C:\XQQ\SyncHub-for-Agents --output C:\Users\qinqiangxu\.copilot\session-state\0b92b33a-6e67-4e1d-aa94-33813aa07b76\files\evidence-after-pack.json
```

Expected: evidence pack should now report `files.devcontainer=true`, `governance.labelConfigPaths` containing `.github/labels.yml`, `observability` may remain conservative, and `validationCommands` may still be empty.

- [ ] **Step 3: Commit implementation**

Run:

```powershell
git add .env.example .github/labels.yml docs/specs/validation-receipt.v1.schema.json docs/specs/repair-proof.v1.schema.json docs/operations/agentic-observability.md docs/development.md scripts/doc-drift.mjs scripts/dev-lib.mjs scripts/dev.mjs frontend/doc-drift.test.mjs frontend/package.json tools/repocheck/workflow_test.go
git -c core.hooksPath=.githooks commit -m "docs: add AI readiness evidence contracts" -m "Expose non-secret development inputs, version validation and repair receipts, document observable workflow artifacts, and guard the documentation with executable drift checks." -m "Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
```

Expected: commit succeeds and hook reports `check: passed`.

## Task 6: Publish, verify hosted checks, and re-evaluate

**Files:**
- No source edits expected.

- [ ] **Step 1: Push branch**

Run:

```powershell
git push
```

Expected: branch updates without force-push.

- [ ] **Step 2: Update PR #11**

Use `gh pr edit 11` to add the new summary and local validation evidence. Do not create a release or tag.

- [ ] **Step 3: Wait for hosted checks**

Run:

```powershell
gh run list --repo jelllove/SyncHub-for-Agents --branch agents/readiness-linux-validation-20260917 --limit 10 --json databaseId,name,event,status,conclusion,headSha,url
```

Watch the CI and repair runs for the new head SHA until they pass.

- [ ] **Step 4: Re-evaluate once**

Run:

```powershell
C:\Users\qinqiangxu\.agents\skills\codeblend-ai-composite\ai-readiness-eval.exe eval C:\XQQ\SyncHub-for-Agents --no-cache
```

Expected: report the actual score. If it is below 80, explain that the remaining gap requires recognized self-healing/agent-operation evidence rather than more local documentation.

## Self-review

- Spec coverage: The plan covers every selected design file and explicitly excludes releases, app behavior changes, fake evidence, and stricter review rules.
- Placeholder scan: No `TBD`, `TODO`, `implement later`, or unexpanded edge-case instructions remain.
- Type consistency: The single new public function is `checkEvidenceDrift(root)` in both tests and implementation.
