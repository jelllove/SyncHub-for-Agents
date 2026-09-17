// @vitest-environment node
import { afterEach, describe, expect, it } from "vitest";
import { spawnSync } from "node:child_process";
import { existsSync, mkdtempSync, mkdirSync, readFileSync, readdirSync, writeFileSync, rmSync, symlinkSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { checkReference, referenceEnd, referenceStart, run } from "../scripts/dev-lib.mjs";
import * as validation from "../scripts/validation.mjs";

const roots = [];
function fixture() {
  const root = mkdtempSync(path.join(tmpdir(), "synchub-validation-"));
  roots.push(root);
  run("git", ["init", "--quiet"], root);
  writeFileSync(path.join(root, ".gitignore"), ".artifacts/\n");
  run("git", ["add", ".gitignore"], root);
  run("git", [
    "-c", "user.name=Validation test", "-c", "user.email=validation@example.invalid",
    "-c", "commit.gpgsign=false", "-c", "core.hooksPath=",
    "commit", "--quiet", "-m", "fixture",
  ], root);
  return root;
}
function check(id, script, timeout) {
  return { id, command: process.execPath, args: ["-e", script], timeout };
}
function evaluate(root, checks, options) {
  expect(validation.runValidation).toBeTypeOf("function");
  return validation.runValidation(root, checks, options);
}
function commandFixture(root) {
  const scripts = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..", "scripts");
  mkdirSync(path.join(root, "scripts"));
  for (const name of ["dev.mjs", "dev-lib.mjs", "validation.mjs", "maintenance.mjs", "source-snapshot.mjs", "reporting.mjs", "repair-container.mjs"]) {
    writeFileSync(path.join(root, "scripts", name), readFileSync(path.join(scripts, name)));
  }
  const env = { ...process.env };
  delete env.GITHUB_STEP_SUMMARY;
  const outputFile = path.join(root, "workflow-output.txt");
  env.GITHUB_OUTPUT = outputFile;
  return { env, outputFile };
}
afterEach(() => {
  for (const root of roots.splice(0)) rmSync(root, { recursive: true, force: true });
});

// These integration cases launch native processes and create Git repositories.
describe("validation receipts", { timeout: 20_000 }, () => {
  it("persists real successful check output, revision, duration, and a machine-readable result", () => {
    const root = fixture();
    const { report, directory } = evaluate(root, [check("success", "console.log('checked'); console.warn('warning detail')")]);
    expect(report.schemaVersion).toBe(1);
    expect(report.status).toBe("passed");
    expect(report.commit).toMatch(/^[a-f0-9]{40,64}$/);
    expect(report.worktreeDirty).toBe(false);
    expect(report).toHaveProperty("source");
    expect(report.source.status).toBe("stable");
    expect(report.source.before.sha256).toMatch(/^[a-f0-9]{64}$/);
    expect(report.source.after.sha256).toBe(report.source.before.sha256);
    expect(report.checks[0]).toMatchObject({ id: "success", status: "passed", log: "success.log" });
    expect(report.checks[0].durationMs).toBeGreaterThanOrEqual(0);
    expect(Date.parse(report.finishedAt)).toBeGreaterThanOrEqual(Date.parse(report.startedAt));
    expect(readFileSync(path.join(directory, "success.log"), "utf8")).toContain("checked");
    expect(readFileSync(path.join(directory, "success.log"), "utf8")).toContain("warning detail");
    expect(JSON.parse(readFileSync(path.join(directory, "validation.json"), "utf8"))).toEqual(report);
    expect(existsSync(path.join(directory, "junit.xml"))).toBe(true);
    expect(existsSync(path.join(directory, "index.html"))).toBe(true);
  });

  it("retains a failed result and error details while still checking independent commands", () => {
    const root = fixture();
    const { report, directory } = evaluate(root, [
      check("failure", "console.error('failure detail'); process.exit(7)"),
      check("next", "console.log('still checked')"),
    ]);
    expect(report.status).toBe("failed");
    expect(report.checks.map(result => result.status)).toEqual(["failed", "passed"]);
    expect(readFileSync(path.join(directory, "failure.log"), "utf8")).toContain("failure detail");
    expect(readFileSync(path.join(directory, "failure.log"), "utf8")).toContain("exit 7");
    expect(readFileSync(path.join(directory, "next.log"), "utf8")).toContain("still checked");
  });

  it("reports timeouts as failures rather than silently retrying or passing", () => {
    const root = fixture();
    const { report, directory } = evaluate(root, [
      check("timeout", "setTimeout(() => {}, 10000)", 100),
    ]);
    expect(report.status).toBe("failed");
    expect(report.checks[0].status).toBe("failed");
    expect(readFileSync(path.join(directory, "timeout.log"), "utf8")).toContain("ETIMEDOUT");
  });

  it("distinguishes same-size dirty content at the same commit and streams binary inputs", () => {
    const root = fixture();
    const bytes = Buffer.alloc(192 * 1024, 97);
    writeFileSync(path.join(root, "input.bin"), bytes);
    const first = evaluate(root, [check("readonly", "")]).report;
    bytes[bytes.length - 1] = 98;
    writeFileSync(path.join(root, "input.bin"), bytes);
    const second = evaluate(root, [check("readonly", "")]).report;
    expect(first.commit).toBe(second.commit);
    expect(first.worktreeDirty && second.worktreeDirty).toBe(true);
    expect(first).toHaveProperty("source");
    expect(second).toHaveProperty("source");
    expect(first.source.before.sha256).not.toBe(second.source.before.sha256);
    expect(first.source.status).toBe("stable");
    expect(second.source.status).toBe("stable");
  });

  it.each([
    ["edited", "require('node:fs').writeFileSync('source.txt', 'changed')"],
    ["added", "require('node:fs').writeFileSync('added.txt', 'new')"],
    ["deleted", "require('node:fs').unlinkSync('source.txt')"],
  ])("fails the receipt when source is %s despite a successful command", (kind, script) => {
    const root = fixture();
    writeFileSync(path.join(root, "source.txt"), "before");
    run("git", ["add", "source.txt"], root);
    const { report } = evaluate(root, [check("mutating", script)]);
    expect(report.checks[0].status).toBe("passed");
    expect(report.status).toBe("failed");
    expect(report.source.status, report.source.error).toBe("changed");
    expect(report.source.changedPaths).toContain(kind === "added" ? "added.txt" : "source.txt");
    expect(report.source.error).toContain("Source changed during validation");
  });

  it("does not count ignored artifacts as source modifications", () => {
    const root = fixture();
    const { report } = evaluate(root, [
      check("artifact", "require('node:fs').writeFileSync('.artifacts/output.txt', 'generated')"),
    ]);
    expect(report.status).toBe("passed");
    expect(report).toHaveProperty("source");
    expect(report.source.status).toBe("stable");
    expect(report.source.before.sha256).toBe(report.source.after.sha256);
  });

  it("rejects a changed Git revision even when the source bytes are unchanged", () => {
    const root = fixture();
    const args = [
      "-c", "user.name=Validation test", "-c", "user.email=validation@example.invalid",
      "-c", "commit.gpgsign=false", "-c", "core.hooksPath=",
      "commit", "--allow-empty", "--quiet", "-m", "new revision",
    ];
    const { report } = evaluate(root, [
      check("revision", `require('node:child_process').execFileSync('git', ${JSON.stringify(args)})`),
    ]);
    expect(report.status).toBe("failed");
    expect(report.source.status).toBe("changed");
    expect(report.source.changedPaths).toEqual([]);
    expect(report.source.before.head).not.toBe(report.source.after.head);
  });

  it("retains a failed receipt if the after-snapshot can no longer read a source file", () => {
    const root = fixture();
    writeFileSync(path.join(root, "source.txt"), "before");
    run("git", ["add", "source.txt"], root);
    const { report, directory } = evaluate(root, [
      check("replace", "const fs=require('node:fs'); fs.unlinkSync('source.txt'); fs.mkdirSync('source.txt')"),
    ]);
    expect(report.checks[0].status).toBe("passed");
    expect(report.status).toBe("failed");
    expect(report.source.status).toBe("error");
    expect(report.source.error).toContain("after source snapshot");
    expect(report.source.after).toBe(null);
    expect(JSON.parse(readFileSync(path.join(directory, "validation.json"), "utf8")).status).toBe("failed");
  });

  it("fails closed before executing checks when source points outside the checkout", () => {
    const root = fixture();
    const outside = fixture();
    const linked = path.join(root, "linked");
    mkdirSync(linked);
    writeFileSync(path.join(linked, "source.txt"), "inside");
    run("git", ["add", "linked/source.txt"], root);
    rmSync(linked, { recursive: true, force: true });
    writeFileSync(path.join(outside, "source.txt"), "outside stays untouched");
    symlinkSync(outside, linked, process.platform === "win32" ? "junction" : "dir");
    const { report, directory } = evaluate(root, [
      check("never-run", "require('node:fs').writeFileSync('ran.txt', 'unexpected')"),
    ]);
    expect(report.status).toBe("failed");
    expect(report.source.status).toBe("error");
    expect(report.source.error).toContain("linked");
    expect(report.checks[0].status).toBe("not-run");
    expect(existsSync(path.join(root, "ran.txt"))).toBe(false);
    expect(JSON.parse(readFileSync(path.join(directory, "validation.json"), "utf8")).status).toBe("failed");
    expect(readFileSync(path.join(outside, "source.txt"), "utf8")).toBe("outside stays untouched");
  });

  it("preserves older reports and only appends a summary when explicitly requested", () => {
    const root = fixture();
    const summary = path.join(root, "summary.txt");
    writeFileSync(summary, "previous summary\n");
    const first = evaluate(root, [check("success", "console.log('first')")]);
    const original = readFileSync(path.join(first.directory, "validation.json"), "utf8");
    expect(readFileSync(summary, "utf8")).toBe("previous summary\n");
    const second = evaluate(root, [check("failure", "process.exit(2)")], { summaryPath: summary });
    expect(second.directory).not.toBe(first.directory);
    expect(readFileSync(path.join(first.directory, "validation.json"), "utf8")).toBe(original);
    expect(readFileSync(summary, "utf8")).toContain("previous summary\n");
    expect(readFileSync(summary, "utf8")).toContain("Repository validation: failed");
    expect(readFileSync(summary, "utf8")).toContain("| failure | failed |");
  });

  it("rejects empty check sets and unsafe or duplicate log identifiers", () => {
    const root = fixture();
    for (const checks of [
      [],
      [check("../escape", "")],
      [check("same", ""), check("same", "")],
      [check(undefined, "")],
      [check(null, "")],
    ]) {
      expect(() => evaluate(root, checks)).toThrow();
    }
    expect(readdirSync(root)).not.toContain(".artifacts");
  });

  it("refuses linked artifact directories without writing outside the repository", () => {
    const root = fixture();
    const external = fixture();
    symlinkSync(external, path.join(root, ".artifacts"), process.platform === "win32" ? "junction" : "dir");
    const before = readdirSync(external);
    expect(() => evaluate(root, [check("success", "")])).toThrow("artifact directory");
    expect(readdirSync(external)).toEqual(before);
  });

  it("keeps incomplete checks visibly running until their subprocess completes", () => {
    const root = fixture();
    mkdirSync(path.join(root, "scripts"));
    const script = [
      "const fs = require('node:fs');",
      "const path = require('node:path');",
      "const base = path.join('.artifacts', 'validation');",
      "const dir = fs.readdirSync(base)[0];",
      "const report = JSON.parse(fs.readFileSync(path.join(base, dir, 'validation.json'), 'utf8'));",
      "if (report.status !== 'running' || report.checks[0].status !== 'running') process.exit(1);",
    ].join("\n");
    const { report } = evaluate(root, [check("lifecycle", script)]);
    expect(report.status).toBe("passed");
  });

  it("makes the public verify command exit nonzero when validation fails", () => {
    const root = fixture();
    const { env, outputFile } = commandFixture(root);
    const result = spawnSync(process.execPath, [path.join(root, "scripts", "dev.mjs"), "verify"], {
      cwd: root, env, encoding: "utf8", timeout: 25_000,
    });
    expect(result.error).toBeUndefined();
    expect(result.status).toBe(1);
    expect(result.stderr).toContain("Repository validation failed");
    const base = path.join(root, ".artifacts", "validation");
    const report = JSON.parse(readFileSync(path.join(base, readdirSync(base)[0], "validation.json"), "utf8"));
    expect(report.status).toBe("failed");
    expect(report.checks.map(check => check.id)).toEqual([
      "frontend-build", "repository-checks", "go-tests", "lint-tests", "frontend-tests",
    ]);
    expect(readFileSync(outputFile, "utf8")).toContain(`report_directory=${path.join(base, readdirSync(base)[0])}`);
  }, 30_000);

  it("fails public verification on source mutation even when every command succeeds", () => {
    const root = fixture();
    const { env } = commandFixture(root);
    mkdirSync(path.join(root, "frontend"));
    mkdirSync(path.join(root, "docs"));
    writeFileSync(path.join(root, "input.txt"), "before");
    writeFileSync(path.join(root, ".node-version"), "24.17.0\n");
    writeFileSync(path.join(root, "go.mod"), "module example.test/receipt\n\ngo 1.26.6\nrequire (\n github.com/wailsapp/wails/v3 v3.0.0-beta.8\n)\n");
    writeFileSync(path.join(root, "main.go"), "package receipt\n\nvar Value = 1\n");
    writeFileSync(path.join(root, "frontend", "package.json"), JSON.stringify({ scripts: {
      build: `node -e "require('node:fs').writeFileSync('../input.txt','after')"`,
      lint: 'node -e "process.exit(0)"',
      typecheck: 'node -e "process.exit(0)"',
      "test:lint": 'node -e "process.exit(0)"',
      test: 'node -e "process.exit(0)"',
    } }));
    writeFileSync(path.join(root, "docs", "development.md"), `${referenceStart}\n${referenceEnd}\n`);
    checkReference(root, true);
    const result = spawnSync(process.execPath, [path.join(root, "scripts", "dev.mjs"), "verify"], {
      cwd: root, env, encoding: "utf8", timeout: 60_000,
    });
    expect(result.error).toBeUndefined();
    expect(result.status).toBe(1);
    expect(result.stderr).toContain("Source changed during validation");
    const base = path.join(root, ".artifacts", "validation");
    const report = JSON.parse(readFileSync(path.join(base, readdirSync(base)[0], "validation.json"), "utf8"));
    expect(report.checks.every(check => check.status === "passed"), result.stdout + result.stderr).toBe(true);
    expect(report.status).toBe("failed");
    expect(report.source.changedPaths).toContain("input.txt");
  }, 65_000);
});
