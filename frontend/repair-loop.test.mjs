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
  put(root, ".github/labels.yml", [
    "- name: ai-readiness",
    "- name: validation",
    "- name: repair-proof",
    "- name: agent-review",
    "- name: documentation-drift",
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
  put(root, ".github/workflows/ci.yml", "name: CI\nrepository-validation\nmaintenance-proposal\n");
  put(root, ".github/workflows/maintenance.yml", "name: Repository maintenance\n");
  put(root, ".github/workflows/repair-verification.yml", "name: Repair verification\nrepair-verification\n");
  put(root, "docs/operations/agentic-observability.md", [
    "# Agentic observability",
    "ai-readiness validation repair-proof agent-review documentation-drift",
    ".github/workflows/ci.yml .github/workflows/maintenance.yml .github/workflows/repair-verification.yml",
    "`repository-validation` `maintenance-proposal` `repair-verification`",
    "docs/specs/validation-receipt.v1.schema.json docs/specs/repair-proof.v1.schema.json",
    "node scripts/dev.mjs verify node scripts/dev.mjs repair:verify",
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
