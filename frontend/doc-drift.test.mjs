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
    "CODEOWNERS": "* @jelllove\n",
    ".agents/skills/synchub-validation/SKILL.md": [
      "# SyncHub Validation",
      "",
      "Use for repository setup, check, verify, repair:verify, and docs handoff.",
      "",
    ].join("\n"),
    ".pre-commit-config.yaml": [
      "repos:",
      "  - repo: local",
      "    hooks:",
      "      - id: synchub-check",
      "        entry: node scripts/dev.mjs check",
      "        pass_filenames: false",
      "      - id: synchub-docs",
      "        entry: node scripts/dev.mjs docs",
      "        pass_filenames: false",
      "",
    ].join("\n"),
    ".github/ISSUE_TEMPLATE/config.yml": [
      "blank_issues_enabled: true",
      "contact_links:",
      "  - name: AI readiness evidence report",
      "    url: https://github.com/jelllove/SyncHub-for-Agents/actions/workflows/maintenance.yml",
      "    about: Run maintenance evidence before filing stale validation issues.",
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
      "Workflows: .github/workflows/ci.yml, .github/workflows/maintenance.yml, .github/workflows/repair-verification.yml, .github/workflows/self-healing.yml, .github/workflows/codeql.yml, .github/workflows/copilot-agent-review.yml.",
      "",
      "Artifacts: `repository-validation`, `maintenance-proposal`, `repair-verification`, `ci-failure-response`, `copilot-agent-review`.",
      "",
      "Schemas: docs/specs/validation-receipt.v1.schema.json and docs/specs/repair-proof.v1.schema.json.",
      "",
      "Surfaces: CODEOWNERS, .agents/skills/synchub-validation/SKILL.md, .pre-commit-config.yaml, .github/ISSUE_TEMPLATE/config.yml, .vscode/mcp.json, docs/specs/README.md, docs/specs/agentic-validation.v1.md, docs/adr/0001-validation-evidence.md, docs/reports/agentic-validation-reports.md, docs/dashboards/agentic-readiness-dashboard.json, docs/runbooks/ci-failure-response.md, tools/mcp/validation-server.mjs.",
      "",
      "Commands: node scripts/dev.mjs verify, node scripts/dev.mjs repair:verify, and node scripts/dev.mjs propose.",
      "",
    ].join("\n"),
    "docs/specs/README.md": "# SyncHub versioned specifications\n",
    "docs/specs/agentic-validation.v1.md": "# Agentic validation specification v1\n",
    "docs/adr/0001-validation-evidence.md": "# ADR 0001: Version repository validation evidence\n",
    "docs/reports/agentic-validation-reports.md": "# Reports\n\n`repository-validation` `maintenance-proposal` `repair-verification` `ci-failure-response` `copilot-agent-review`\n",
    "docs/dashboards/agentic-readiness-dashboard.json": JSON.stringify({
      schemaVersion: 1,
      title: "SyncHub agentic readiness dashboard",
      signals: ["repository-validation", "maintenance-proposal", "repair-verification", "ci-failure-response", "copilot-agent-review"],
    }) + "\n",
    "docs/runbooks/ci-failure-response.md": "# CI failure response\n\nDetection, containment, remediation, validation, and rollback stay review-only.\n",
    ".vscode/mcp.json": JSON.stringify({
      servers: {
        "synchub-validation": {
          command: "node",
          args: ["tools/mcp/validation-server.mjs"],
        },
      },
    }) + "\n",
    "tools/mcp/validation-server.mjs": "export const name = 'synchub-validation';\n",
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
    ".github/workflows/self-healing.yml": [
      "name: Self-healing diagnostics",
      "on:",
      "  workflow_run:",
      "    workflows: [CI]",
      "    types: [completed]",
      "  workflow_dispatch:",
      "permissions:",
      "  actions: read",
      "  contents: read",
      "jobs:",
      "  response:",
      "    steps:",
      "      - run: node scripts/dev.mjs propose",
      "      - uses: actions/upload-artifact@ea165f8d65b6e75b540449e92b4886f43607fa02",
      "        with:",
      "          name: ci-failure-response",
      "",
    ].join("\n"),
    ".github/workflows/codeql.yml": [
      "name: CodeQL",
      "jobs:",
      "  analyze:",
      "    steps:",
      "      - uses: github/codeql-action/init@b96794f015dfd88f77b49b1c93e0fa7110f94c63",
      "        with:",
      "          languages: javascript-typescript",
      "",
    ].join("\n"),
    ".github/workflows/copilot-agent-review.yml": [
      "name: Copilot agent review",
      "jobs:",
      "  review:",
      "    name: Copilot agent review",
      "    steps:",
      "      - run: npm install --global @github/copilot@1.0.84",
      "      - run: You are reviewing SyncHub for Agents. Do not modify files.",
      "      - run: copilot -C \"$GITHUB_WORKSPACE\" --no-ask-user -p \"$prompt\"",
      "      - uses: actions/upload-artifact@ea165f8d65b6e75b540449e92b4886f43607fa02",
      "        with:",
      "          name: copilot-agent-review",
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
      "Workflows: .github/workflows/ci.yml, .github/workflows/maintenance.yml, .github/workflows/repair-verification.yml, .github/workflows/self-healing.yml, .github/workflows/codeql.yml, .github/workflows/copilot-agent-review.yml.",
      "Artifacts: `repository-validation` and `maintenance-proposal`.",
      "Schemas: docs/specs/validation-receipt.v1.schema.json and docs/specs/repair-proof.v1.schema.json.",
      "Surfaces: CODEOWNERS, .agents/skills/synchub-validation/SKILL.md, .pre-commit-config.yaml, .github/ISSUE_TEMPLATE/config.yml, .vscode/mcp.json, docs/specs/README.md, docs/specs/agentic-validation.v1.md, docs/adr/0001-validation-evidence.md, docs/reports/agentic-validation-reports.md, docs/dashboards/agentic-readiness-dashboard.json, docs/runbooks/ci-failure-response.md, tools/mcp/validation-server.mjs.",
      "Commands: node scripts/dev.mjs verify, node scripts/dev.mjs repair:verify, and node scripts/dev.mjs propose.",
      "",
    ].join("\n"),
  });
  assert.throws(
    () => checkEvidenceDrift(root),
    /agentic-observability.md must mention "`repair-verification`"/,
  );
});

test("checkEvidenceDrift rejects undocumented dashboard surface", () => {
  const root = fixtureRoot();
  writeFixture(root, {
    "docs/operations/agentic-observability.md": [
      "# Agentic observability",
      "Labels: ai-readiness, validation, repair-proof, agent-review, documentation-drift.",
      "Workflows: .github/workflows/ci.yml, .github/workflows/maintenance.yml, .github/workflows/repair-verification.yml, .github/workflows/self-healing.yml, .github/workflows/codeql.yml, .github/workflows/copilot-agent-review.yml.",
      "Artifacts: `repository-validation`, `maintenance-proposal`, `repair-verification`, `ci-failure-response`, `copilot-agent-review`.",
      "Schemas: docs/specs/validation-receipt.v1.schema.json and docs/specs/repair-proof.v1.schema.json.",
      "Surfaces: CODEOWNERS, .agents/skills/synchub-validation/SKILL.md, .pre-commit-config.yaml, .github/ISSUE_TEMPLATE/config.yml, .vscode/mcp.json, docs/specs/README.md, docs/specs/agentic-validation.v1.md, docs/adr/0001-validation-evidence.md, docs/reports/agentic-validation-reports.md, docs/runbooks/ci-failure-response.md, tools/mcp/validation-server.mjs.",
      "Commands: node scripts/dev.mjs verify, node scripts/dev.mjs repair:verify, and node scripts/dev.mjs propose.",
      "",
    ].join("\n"),
  });
  assert.throws(
    () => checkEvidenceDrift(root),
    /agentic-observability.md must mention "docs\/dashboards\/agentic-readiness-dashboard.json"/,
  );
});

test("checkEvidenceDrift rejects undocumented Copilot agent review artifact", () => {
  const root = fixtureRoot();
  writeFixture(root, {
    "docs/operations/agentic-observability.md": [
      "# Agentic observability",
      "Labels: ai-readiness, validation, repair-proof, agent-review, documentation-drift.",
      "Workflows: .github/workflows/ci.yml, .github/workflows/maintenance.yml, .github/workflows/repair-verification.yml, .github/workflows/self-healing.yml, .github/workflows/codeql.yml, .github/workflows/copilot-agent-review.yml.",
      "Artifacts: `repository-validation`, `maintenance-proposal`, `repair-verification`, `ci-failure-response`.",
      "Schemas: docs/specs/validation-receipt.v1.schema.json and docs/specs/repair-proof.v1.schema.json.",
      "Surfaces: CODEOWNERS, .agents/skills/synchub-validation/SKILL.md, .pre-commit-config.yaml, .github/ISSUE_TEMPLATE/config.yml, .vscode/mcp.json, docs/specs/README.md, docs/specs/agentic-validation.v1.md, docs/adr/0001-validation-evidence.md, docs/reports/agentic-validation-reports.md, docs/dashboards/agentic-readiness-dashboard.json, docs/runbooks/ci-failure-response.md, tools/mcp/validation-server.mjs.",
      "Commands: node scripts/dev.mjs verify, node scripts/dev.mjs repair:verify, and node scripts/dev.mjs propose.",
      "",
    ].join("\n"),
  });
  assert.throws(
    () => checkEvidenceDrift(root),
    /agentic-observability.md must mention "`copilot-agent-review`"/,
  );
});
