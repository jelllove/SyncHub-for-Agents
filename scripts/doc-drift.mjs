import path from "node:path";
import { readFileSync } from "node:fs";

const requiredEnvKeys = [
  "SYNCHUB_GITHUB_CLIENT_ID=",
  "ACSYNC_GITHUB_CLIENT_ID=",
  "SYNCHUB_RUN_HELPER_INTEGRATION=0",
];
const requiredLabels = ["ai-readiness", "validation", "repair-proof", "agent-review", "documentation-drift"];
const requiredGuideEntries = [
  "docs/specs/validation-receipt.v1.schema.json",
  "docs/specs/repair-proof.v1.schema.json",
  "docs/specs/README.md",
  "docs/specs/agentic-validation.v1.md",
  "docs/adr/0001-validation-evidence.md",
  "docs/reports/agentic-validation-reports.md",
  "docs/dashboards/agentic-readiness-dashboard.json",
  "docs/runbooks/ci-failure-response.md",
  ".github/workflows/ci.yml",
  ".github/workflows/maintenance.yml",
  ".github/workflows/repair-verification.yml",
  ".github/workflows/self-healing.yml",
  ".github/workflows/codeql.yml",
  ".github/ISSUE_TEMPLATE/config.yml",
  ".vscode/mcp.json",
  "CODEOWNERS",
  "tools/mcp/validation-server.mjs",
  "`repository-validation`",
  "`maintenance-proposal`",
  "`repair-verification`",
  "`ci-failure-response`",
  "node scripts/dev.mjs verify",
  "node scripts/dev.mjs repair:verify",
  "node scripts/dev.mjs propose",
];

function read(root, relative) {
  return readFileSync(path.join(root, relative), "utf8");
}

function requireContains(content, needle, source) {
  if (!content.includes(needle)) {
    throw new Error(`${source} must mention "${needle}"`);
  }
}

function requireLabel(labels, label) {
  if (!new RegExp(String.raw`(?:^|\n)\s*-\s*name:\s*["']?${label}["']?(?:\s|\n|$)`).test(labels)) {
    throw new Error(`labels.yml must define label "${label}"`);
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
  const envExample = read(root, ".env.example");
  for (const key of requiredEnvKeys) requireContains(envExample, key, ".env.example");

  const labels = read(root, ".github/labels.yml");
  for (const label of requiredLabels) requireLabel(labels, label);
  requireContains(read(root, "CODEOWNERS"), "@jelllove", "CODEOWNERS");
  requireContains(read(root, ".github/ISSUE_TEMPLATE/config.yml"), "AI readiness evidence report", "issue template config");

  requireJsonSchema(root, "docs/specs/validation-receipt.v1.schema.json", "SyncHub validation receipt v1", [
    "schemaVersion",
    "status",
    "checks",
    "source",
  ]);
  requireJsonSchema(root, "docs/specs/repair-proof.v1.schema.json", "SyncHub contained repair proof v1", [
    "schemaVersion",
    "scenario",
    "status",
    "steps",
    "snapshots",
    "originalSourceUnchanged",
  ]);

  const guide = read(root, "docs/operations/agentic-observability.md");
  for (const entry of requiredLabels.concat(requiredGuideEntries)) {
    requireContains(guide, entry, "agentic-observability.md");
  }

  const dashboard = JSON.parse(read(root, "docs/dashboards/agentic-readiness-dashboard.json"));
  if (dashboard.schemaVersion !== 1 || !dashboard.signals?.includes("ci-failure-response")) {
    throw new Error("agentic-readiness-dashboard.json must list the ci-failure-response signal");
  }
  const mcp = JSON.parse(read(root, ".vscode/mcp.json"));
  const server = mcp.servers?.["synchub-validation"];
  if (server?.command !== "node" || !server.args?.includes("tools/mcp/validation-server.mjs")) {
    throw new Error(".vscode/mcp.json must expose tools/mcp/validation-server.mjs");
  }

  requireContains(read(root, ".github/workflows/ci.yml"), "repository-validation", "ci.yml");
  requireContains(read(root, ".github/workflows/ci.yml"), "maintenance-proposal", "ci.yml");
  requireContains(read(root, ".github/workflows/repair-verification.yml"), "repair-verification", "repair-verification.yml");
  requireContains(read(root, ".github/workflows/self-healing.yml"), "workflow_run", "self-healing.yml");
  requireContains(read(root, ".github/workflows/self-healing.yml"), "node scripts/dev.mjs propose", "self-healing.yml");
  requireContains(read(root, ".github/workflows/self-healing.yml"), "ci-failure-response", "self-healing.yml");
  requireContains(read(root, ".pre-commit-config.yaml"), "node scripts/dev.mjs check", ".pre-commit-config.yaml");
  requireContains(read(root, ".pre-commit-config.yaml"), "node scripts/dev.mjs docs", ".pre-commit-config.yaml");
  requireContains(read(root, ".github/workflows/codeql.yml"), "javascript-typescript", "codeql.yml");
}
