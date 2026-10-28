// @vitest-environment node
import { afterEach, describe, expect, it } from "vitest";
import { existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, symlinkSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { checkReference, referenceEnd, referenceStart, run } from "../scripts/dev-lib.mjs";
import * as loop from "../scripts/repair-loop.mjs";

const directories = [];
function temporary(prefix) {
  const directory = mkdtempSync(path.join(tmpdir(), prefix));
  directories.push(directory);
  return directory;
}
function put(root, file, value) {
  const filename = path.join(root, file);
  mkdirSync(path.dirname(filename), { recursive: true });
  writeFileSync(filename, value);
}
function evidenceFixture(root) {
  put(root, ".env.example", [
    "SYNCHUB_GITHUB_CLIENT_ID=",
    "ACSYNC_GITHUB_CLIENT_ID=",
    "SYNCHUB_RUN_HELPER_INTEGRATION=0",
    "",
  ].join("\n"));
  put(root, "CODEOWNERS", "* @jelllove\n");
  put(root, ".github/copilot-instructions.md", "node scripts/dev.mjs setup\nnode scripts/dev.mjs check\nnode scripts/dev.mjs verify\nnode scripts/dev.mjs docs\nDo not run application sync\nDo not create releases or tags\n");
  put(root, ".agents/skills/synchub-validation/SKILL.md", "# SyncHub Validation\n");
  put(root, ".pre-commit-config.yaml", "repos:\n  - repo: local\n    hooks:\n      - entry: node scripts/dev.mjs check\n        pass_filenames: false\n      - entry: node scripts/dev.mjs docs\n        pass_filenames: false\n");
  put(root, ".github/labels.yml", [
    "- name: ai-readiness",
    "- name: validation",
    "- name: repair-proof",
    "- name: agent-review",
    "- name: documentation-drift",
    "",
  ].join("\n"));
  put(root, ".github/ISSUE_TEMPLATE/config.yml", [
    "blank_issues_enabled: true",
    "contact_links:",
    "  - name: AI readiness evidence report",
    "",
  ].join("\n"));
  put(root, "docs/specs/validation-receipt.v1.schema.json", JSON.stringify({
    "$schema": "https://json-schema.org/draft/2020-12/schema",
    title: "SyncHub validation receipt v1",
    required: ["schemaVersion", "status", "checks", "source"],
  }) + "\n");
  put(root, "docs/specs/repair-proof.v1.schema.json", JSON.stringify({
    "$schema": "https://json-schema.org/draft/2020-12/schema",
    title: "SyncHub contained repair proof v1",
    required: ["schemaVersion", "scenario", "status", "steps", "snapshots", "originalSourceUnchanged"],
  }) + "\n");
  put(root, ".github/workflows/ci.yml", "name: CI\nDocumentation drift\nnode scripts/dev.mjs docs\nrepository-validation\nmaintenance-proposal\n");
  put(root, ".github/workflows/maintenance.yml", "name: Repository maintenance\n");
  put(root, ".github/workflows/repair-verification.yml", "name: Repair verification\nrepair-verification\n");
  put(root, ".github/workflows/self-healing.yml", "name: Self-healing diagnostics\nworkflow_run\nnode scripts/dev.mjs rollback:verify\nrollback-verification\nnode scripts/dev.mjs propose\nci-failure-response\n");
  put(root, ".github/workflows/auto-revert.yml", "name: Auto-revert failed main\nauto-revert\n");
  put(root, ".github/workflows/pr-validation.yml", "name: PR validation\ngo test ./...\nnpm test\n");
  put(root, ".github/workflows/codeql.yml", "name: CodeQL\njavascript-typescript\n");
  put(root, ".github/workflows/copilot-agent-review.yml", "name: Copilot agent review\nnpm install --global @github/copilot@1.0.84\nYou are reviewing SyncHub for Agents\nDo not modify files\ncopilot-agent-review\n");
  put(root, ".github/workflows/recurring-copilot-review.yml", "name: Recurring Copilot review\npull_request\n./.github/actions/recurring-copilot-review\n");
  put(root, ".github/actions/recurring-copilot-review/action.yml", "name: Recurring Copilot review\nnpm install --global @github/copilot@1.0.84\ncopilot -p \"Review pull request changed files and publish review output with scoped guards\"\n");
  put(root, ".github/workflows/copilot-setup-steps.yml", "name: Copilot Setup Steps\ncopilot-setup-steps\nnode scripts/dev.mjs setup\n");
  put(root, "docs/specs/README.md", "# SyncHub versioned specifications\n");
  put(root, "docs/specs/agentic-validation.v1.md", "# Agentic validation specification v1\n");
  put(root, "docs/adr/0001-validation-evidence.md", "# ADR 0001: Version repository validation evidence\n");
  put(root, "docs/reports/agentic-validation-reports.md", "`repository-validation` `maintenance-proposal` `repair-verification` `ci-failure-response` `rollback-verification` `copilot-agent-review`\n");
  put(root, "docs/reports/agentic-review-findings.md", "PR #16 Copilot review finding follow-up validation Recurring Copilot review\n");
  put(root, "docs/dashboards/agentic-readiness-dashboard.json", JSON.stringify({
    schemaVersion: 1,
    signals: ["ci-failure-response", "rollback-verification", "copilot-agent-review"],
  }) + "\n");
  put(root, "docs/runbooks/ci-failure-response.md", "detection containment remediation validation rollback\n");
  put(root, "docs/operations/agentic-learning-lifecycle.md", "candidate active retired rejected Recurring Copilot review regression validation agentic-review-findings.md\n");
  put(root, ".vscode/mcp.json", JSON.stringify({
    servers: { "synchub-validation": { command: "node", args: ["tools/mcp/synchub-mcp-server.mjs"] } },
  }) + "\n");
  put(root, "tools/mcp/validation-server.mjs", "export const name = 'synchub-validation';\n");
  put(root, "tools/mcp/synchub-mcp-server.mjs", "import './validation-server.mjs';\n");
  put(root, "package.json", JSON.stringify({
    scripts: {
      verify: "node scripts/dev.mjs verify",
      "rollback:verify": "node scripts/dev.mjs rollback:verify",
    },
  }) + "\n");
  put(root, "Makefile", "verify:\n\tnode scripts/dev.mjs verify\n\nrollback-verify:\n\tnode scripts/dev.mjs rollback:verify\n");
  put(root, ".mcp.json", JSON.stringify({
    mcpServers: { "synchub-validation": { command: "node", args: ["mcp/synchub-validation/server.mjs"] } },
  }) + "\n");
  put(root, "mcp/synchub-validation/server.mjs", "export const name = 'synchub-validation';\n");
  put(root, "docs/operations/agentic-observability.md", [
    "# Agentic observability",
    "ai-readiness validation repair-proof agent-review documentation-drift",
    ".github/workflows/ci.yml .github/workflows/maintenance.yml .github/workflows/repair-verification.yml .github/workflows/self-healing.yml .github/workflows/auto-revert.yml .github/workflows/pr-validation.yml .github/workflows/codeql.yml .github/workflows/copilot-agent-review.yml .github/workflows/recurring-copilot-review.yml .github/workflows/copilot-setup-steps.yml Documentation drift",
    "`repository-validation` `maintenance-proposal` `repair-verification` `ci-failure-response` `rollback-verification` `copilot-agent-review`",
    "docs/specs/validation-receipt.v1.schema.json docs/specs/repair-proof.v1.schema.json docs/specs/README.md docs/specs/agentic-validation.v1.md docs/adr/0001-validation-evidence.md",
    "docs/reports/agentic-validation-reports.md docs/reports/agentic-review-findings.md docs/dashboards/agentic-readiness-dashboard.json docs/operations/agentic-learning-lifecycle.md docs/runbooks/ci-failure-response.md",
    "CODEOWNERS .github/copilot-instructions.md .github/actions/recurring-copilot-review/action.yml .agents/skills/synchub-validation/SKILL.md .pre-commit-config.yaml .github/ISSUE_TEMPLATE/config.yml .vscode/mcp.json Makefile package.json tools/mcp/validation-server.mjs tools/mcp/synchub-mcp-server.mjs .mcp.json mcp/synchub-validation/server.mjs .github/workflows/codeql.yml",
    "candidate active retired rejected Recurring Copilot review",
    "node scripts/dev.mjs verify node scripts/dev.mjs repair:verify node scripts/dev.mjs rollback:verify node scripts/dev.mjs propose go test ./... npm test",
    "",
  ].join("\n"));
}
function fixture() {
  const root = temporary("synchub-repair-source-");
  const scripts = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..", "scripts");
  for (const name of ["dev.mjs", "dev-lib.mjs", "validation.mjs", "maintenance.mjs", "source-snapshot.mjs", "reporting.mjs", "repair-container.mjs", "doc-drift.mjs"]) {
    put(root, `scripts/${name}`, readFileSync(path.join(scripts, name)));
  }
  put(root, ".gitignore", ".artifacts/\nfrontend/node_modules/\n");
  put(root, ".node-version", "24.17.0\n");
  put(root, "local-wails/go.mod", "module github.com/wailsapp/wails/v3\n\ngo 1.26.6\n");
  put(root, "go.mod", "module example.test/repair\n\ngo 1.26.6\nrequire (\n github.com/wailsapp/wails/v3 v3.0.0\n)\nreplace github.com/wailsapp/wails/v3 => ./local-wails\n");
  put(root, "app.go", "package repair\n\nvar Value = 1\n");
  put(root, "frontend/package.json", JSON.stringify({
    name: "repair-fixture", version: "1.0.0",
    scripts: Object.fromEntries(["build", "lint", "typecheck", "test:lint", "test"].map(name => [name, 'node -e "process.exit(0)"'])),
  }));
  put(root, "frontend/package-lock.json", JSON.stringify({
    name: "repair-fixture", version: "1.0.0", lockfileVersion: 3, requires: true,
    packages: { "": { name: "repair-fixture", version: "1.0.0" } },
  }));
  put(root, "docs/development.md", `${referenceStart}\n${referenceEnd}\n`);
  evidenceFixture(root);
  checkReference(root, true);
  run("git", ["init", "--quiet"], root);
  run("git", ["add", "."], root);
  run("git", [
    "-c", "user.name=Repair test", "-c", "user.email=repair@example.invalid",
    "-c", "commit.gpgsign=false", "-c", "core.hooksPath=",
    "commit", "--quiet", "-m", "fixture",
  ], root);
  return root;
}
afterEach(() => {
  for (const directory of directories.splice(0)) rmSync(directory, { recursive: true, force: true });
});

describe("isolated native repair verification", { timeout: 120_000 }, () => {
  it("proves failure, repair, full revalidation and rollback without changing the source", () => {
    const root = fixture();
    const output = path.join(temporary("synchub-repair-report-"), "proof");
    const before = readFileSync(path.join(root, "app.go"));
    expect(loop.verifyRepairProbe).toBeTypeOf("function");
    const report = loop.verifyRepairProbe(root, output);
    expect(report.status, report.error).toBe("passed");
    expect(report.scenario).toBe("diagnostic-go-format");
    expect(report.productionIncident).toBe(false);
    expect(report.steps.map(step => [step.id, step.outcome])).toEqual([
      ["baseline", "passed"], ["injected-failure", "failed"],
      ["repaired", "passed"], ["rollback", "failed"],
    ]);
    expect(report.snapshots.repaired).toBe(report.snapshots.baseline);
    expect(report.snapshots.rollback).toBe(report.snapshots.injected);
    expect(report.originalSourceUnchanged).toBe(true);
    expect(readFileSync(path.join(root, "app.go"))).toEqual(before);
    expect(run("git", ["status", "--porcelain=v1"], root)).toBe("");
    expect(existsSync(path.join(output, "repair.patch"))).toBe(true);
    expect(JSON.parse(readFileSync(path.join(output, "repair.json"), "utf8")).status).toBe("passed");
  });

  it("refuses uncommitted source and output locations within the source tree", () => {
    const root = fixture();
    expect(loop.verifyRepairProbe).toBeTypeOf("function");
    expect(() => loop.verifyRepairProbe(root, path.join(root, ".artifacts", "proof"))).toThrow();
    put(root, "app.go", "package repair\n\nvar Value = 2\n");
    const output = path.join(temporary("synchub-repair-report-"), "proof");
    expect(() => loop.verifyRepairProbe(root, output)).toThrow("committed");
    expect(readFileSync(path.join(root, "app.go"), "utf8")).toContain("Value = 2");
  });

  it("rejects an output-directory alias that would write into the source checkout", () => {
    const root = fixture();
    const parent = temporary("synchub-repair-alias-");
    const linked = path.join(parent, "source-link");
    symlinkSync(root, linked, process.platform === "win32" ? "junction" : "dir");
    expect(() => loop.verifyRepairProbe(root, path.join(linked, "proof"))).toThrow();
    expect(existsSync(path.join(root, "proof"))).toBe(false);
  });

  it("retains baseline failures instead of manufacturing a successful diagnostic repair", () => {
    const root = fixture();
    const manifestPath = path.join(root, "frontend", "package.json");
    const manifest = JSON.parse(readFileSync(manifestPath, "utf8"));
    manifest.scripts.test = 'node -e "process.exit(7)"';
    writeFileSync(manifestPath, JSON.stringify(manifest));
    checkReference(root, true);
    run("git", ["add", "."], root);
    run("git", [
      "-c", "user.name=Repair test", "-c", "user.email=repair@example.invalid",
      "-c", "commit.gpgsign=false", "-c", "core.hooksPath=",
      "commit", "--quiet", "-m", "failing baseline",
    ], root);
    const before = readFileSync(path.join(root, "app.go"));
    const output = path.join(temporary("synchub-repair-report-"), "proof");
    const report = loop.verifyRepairProbe(root, output);
    expect(report.status).toBe("failed");
    expect(report.error).toContain("Clean baseline validation failed");
    expect(report.steps.map(step => step.id)).toEqual(["baseline"]);
    expect(existsSync(path.join(output, "repair.patch"))).toBe(false);
    expect(report.originalSourceUnchanged).toBe(true);
    expect(readFileSync(path.join(root, "app.go"))).toEqual(before);
  });
});
