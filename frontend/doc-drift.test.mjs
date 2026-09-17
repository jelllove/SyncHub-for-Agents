import test, { afterEach } from "node:test";
import assert from "node:assert/strict";
import path from "node:path";
import { mkdtempSync, mkdirSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";

import { checkEvidenceDrift } from "../scripts/dev-lib.mjs";

const roots = [];

function fixtureRoot() {
  const root = mkdtempSync(path.join(tmpdir(), "synchub-doc-drift-"));
  roots.push(root);
  return root;
}

afterEach(() => {
  for (const root of roots.splice(0)) rmSync(root, { recursive: true, force: true });
});

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
      "- name: documentation-drift",
      "  color: \"c5def5\"",
      "  description: Documentation and executable behavior alignment",
      "",
    ].join("\n"),
    "docs/specs/validation-receipt.v1.schema.json": JSON.stringify({
      "$schema": "https://json-schema.org/draft/2020-12/schema",
      "$id": "https://github.com/jelllove/SyncHub-for-Agents/schemas/validation-receipt.v1.schema.json",
      title: "SyncHub validation receipt v1",
      type: "object",
      required: ["schemaVersion", "status", "checks", "source"],
      properties: {
        schemaVersion: { const: 1 },
        status: { enum: ["passed", "failed", "running"] },
        checks: { type: "array" },
        source: { type: "object" },
      },
    }, null, 2) + "\n",
    "docs/specs/repair-proof.v1.schema.json": JSON.stringify({
      "$schema": "https://json-schema.org/draft/2020-12/schema",
      "$id": "https://github.com/jelllove/SyncHub-for-Agents/schemas/repair-proof.v1.schema.json",
      title: "SyncHub contained repair proof v1",
      type: "object",
      required: ["schemaVersion", "scenario", "status", "steps", "snapshots", "originalSourceUnchanged"],
      properties: {
        schemaVersion: { const: 1 },
        scenario: { const: "diagnostic-go-format" },
        status: { enum: ["passed", "failed"] },
        steps: { type: "array" },
        snapshots: { type: "object" },
        originalSourceUnchanged: { type: "boolean" },
      },
    }, null, 2) + "\n",
    "docs/operations/agentic-observability.md": [
      "# Agentic observability",
      "",
      "Labels: ai-readiness, validation, repair-proof, agent-review, documentation-drift.",
      "",
      "Workflows: .github/workflows/ci.yml, .github/workflows/maintenance.yml, .github/workflows/repair-verification.yml.",
      "",
      "Artifacts: `repository-validation`, `maintenance-proposal`, `repair-verification`.",
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
      "          name: repair-verification",
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
  const root = fixtureRoot();
  writeFixture(root);
  assert.doesNotThrow(() => checkEvidenceDrift(root));
});

test("checkEvidenceDrift rejects missing label configuration", () => {
  const root = fixtureRoot();
  writeFixture(root, { ".github/labels.yml": "- name: ai-readiness\n" });
  assert.throws(
    () => checkEvidenceDrift(root),
    /labels.yml must define label "validation"/,
  );
});

test("checkEvidenceDrift rejects incomplete environment template", () => {
  const root = fixtureRoot();
  writeFixture(root, { ".env.example": "SYNCHUB_GITHUB_CLIENT_ID=\n" });
  assert.throws(
    () => checkEvidenceDrift(root),
    /.env.example must mention "ACSYNC_GITHUB_CLIENT_ID="/,
  );
});

test("checkEvidenceDrift rejects undocumented repair artifact", () => {
  const root = fixtureRoot();
  writeFixture(root, {
    "docs/operations/agentic-observability.md": [
      "# Agentic observability",
      "Labels: ai-readiness, validation, repair-proof, agent-review, documentation-drift.",
      "Workflows: .github/workflows/ci.yml, .github/workflows/maintenance.yml, .github/workflows/repair-verification.yml.",
      "Artifacts: `repository-validation` and `maintenance-proposal`.",
      "Schemas: docs/specs/validation-receipt.v1.schema.json and docs/specs/repair-proof.v1.schema.json.",
      "Commands: node scripts/dev.mjs verify and node scripts/dev.mjs repair:verify.",
      "",
    ].join("\n"),
  });
  assert.throws(
    () => checkEvidenceDrift(root),
    /agentic-observability.md must mention "`repair-verification`"/,
  );
});
