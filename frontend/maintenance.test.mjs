// @vitest-environment node
import { afterEach, describe, expect, it } from "vitest";
import { existsSync, mkdtempSync, mkdirSync, readFileSync, readdirSync, rmSync, symlinkSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { checkReference, referenceEnd, referenceStart, run } from "../scripts/dev-lib.mjs";
import * as maintenance from "../scripts/maintenance.mjs";

const roots = [];
function put(root, file, content) {
  const filename = path.join(root, file);
  mkdirSync(path.dirname(filename), { recursive: true });
  writeFileSync(filename, content);
}
function fixture() {
  const root = mkdtempSync(path.join(tmpdir(), "synchub-maintenance-"));
  roots.push(root);
  put(root, ".gitignore", ".artifacts/\n");
  put(root, ".node-version", "24.17.0\n");
  put(root, "go.mod", "module example.test/app\n\ngo 1.26.6\nrequire (\n github.com/wailsapp/wails/v3 v3.0.0-beta.8\n)\n");
  put(root, "frontend/package.json", JSON.stringify({ scripts: { test: "vitest run" } }));
  put(root, "docs/development.md", `Human guidance stays.\n${referenceStart}\n${referenceEnd}\nMore guidance.\n`);
  put(root, "main.go", "package example\n\nvar answer = 1\n");
  checkReference(root, true);
  run("git", ["init", "--quiet"], root);
  run("git", ["add", "."], root);
  run("git", [
    "-c", "user.name=Maintenance test", "-c", "user.email=maintenance@example.invalid",
    "-c", "commit.gpgsign=false", "-c", "core.hooksPath=",
    "commit", "--quiet", "-m", "fixture",
  ], root);
  return root;
}
function propose(root, options) {
  expect(maintenance.proposeMaintenance).toBeTypeOf("function");
  return maintenance.proposeMaintenance(root, options);
}
afterEach(() => {
  for (const root of roots.splice(0)) rmSync(root, { recursive: true, force: true });
});

// Leave headroom for native Git/Go startup on busy Windows hosts.
describe("review-only maintenance proposals", { timeout: 20_000 }, () => {
  it("records an honest no-change result for an already canonical checkout", () => {
    const root = fixture();
    const { report, directory } = propose(root);
    expect(report.status).toBe("no-changes");
    expect(report.changes).toEqual([]);
    expect(existsSync(path.join(directory, "repair.patch"))).toBe(false);
    expect(JSON.parse(readFileSync(path.join(directory, "proposal.json"), "utf8"))).toEqual(report);
    expect(run("git", ["status", "--porcelain=v1"], root)).toBe("");
  });

  it("produces an applicable, reversible patch without modifying existing work or unrelated files", () => {
    const root = fixture();
    const originalGo = "package example\r\nvar  answer=2\r\n";
    const originalDoc = readFileSync(path.join(root, "docs", "development.md"), "utf8");
    put(root, "main.go", originalGo);
    put(root, ".node-version", "24.18.0\n");
    put(root, "notes.txt", "unrelated uncommitted work");
    const before = run("git", ["status", "--porcelain=v1"], root);
    const { report, directory } = propose(root);
    expect(report.status).toBe("proposed");
    expect(report.changes.map(change => change.path).sort()).toEqual(["docs/development.md", "main.go"]);
    expect(report.changes.every(change => /^[a-f0-9]{64}$/.test(change.beforeSha256) && /^[a-f0-9]{64}$/.test(change.afterSha256))).toBe(true);
    expect(report.verification).toEqual({ patchApplies: true, canonical: true, scope: "format-and-generated-reference" });
    expect(readFileSync(path.join(root, "main.go"), "utf8")).toBe(originalGo);
    expect(readFileSync(path.join(root, "docs", "development.md"), "utf8")).toBe(originalDoc);
    expect(run("git", ["status", "--porcelain=v1"], root)).toBe(before);
    const patch = path.join(directory, "repair.patch");
    run("git", ["apply", "--check", patch], root);
    run("git", ["apply", patch], root);
    expect(readFileSync(path.join(root, "main.go"), "utf8").replaceAll("\r\n", "\n")).toBe("package example\n\nvar answer = 2\n");
    expect(() => checkReference(root)).not.toThrow();
    expect(propose(root).report.status).toBe("no-changes");
    run("git", ["apply", "--reverse", "--check", patch], root);
    run("git", ["apply", "--reverse", patch], root);
    expect(readFileSync(path.join(root, "main.go"), "utf8").replaceAll("\r\n", "\n")).toBe(originalGo.replaceAll("\r\n", "\n"));
    expect(readFileSync(path.join(root, "notes.txt"), "utf8")).toBe("unrelated uncommitted work");
  }, 20_000);

  it("enforces exact file-count and patch-size limits without publishing an oversized repair", () => {
    const root = fixture();
    put(root, "main.go", "package example\nvar  answer=2\n");
    put(root, ".node-version", "24.18.0\n");
    const tooMany = propose(root, { maxFiles: 1 });
    expect(tooMany.report.status).toBe("failed");
    expect(tooMany.report.error).toContain("file limit");
    expect(existsSync(path.join(tooMany.directory, "repair.patch"))).toBe(false);
    const allowed = propose(root, { maxFiles: 2 });
    expect(allowed.report.status).toBe("proposed");
    const size = readFileSync(path.join(allowed.directory, "repair.patch")).length;
    expect(propose(root, { maxPatchBytes: size }).report.status).toBe("proposed");
    const tooLarge = propose(root, { maxPatchBytes: size - 1 });
    expect(tooLarge.report.status).toBe("failed");
    expect(tooLarge.report.error).toContain("patch size limit");
    expect(existsSync(path.join(tooLarge.directory, "repair.patch"))).toBe(false);
  }, 20_000);

  it("reports unsupported syntax failures instead of claiming to repair arbitrary code", () => {
    const root = fixture();
    put(root, "main.go", "this is not Go code\n");
    const { report, directory } = propose(root);
    expect(report.status).toBe("failed");
    expect(report.error).toContain("gofmt");
    expect(existsSync(path.join(directory, "repair.patch"))).toBe(false);
    expect(readFileSync(path.join(root, "main.go"), "utf8")).toBe("this is not Go code\n");
  });

  it("produces a plain patch even when the developer forces Git color output", () => {
    const root = fixture();
    run("git", ["config", "color.ui", "always"], root);
    put(root, "main.go", "package example\nvar  answer=2\n");
    const { report, directory } = propose(root);
    expect(report.status).toBe("proposed");
    const patch = path.join(directory, "repair.patch");
    expect(readFileSync(patch, "utf8")).not.toContain("\u001b[");
    run("git", ["apply", "--check", patch], root);
  });

  it("refuses invalid UTF-8 without replacing or changing the original bytes", () => {
    const root = fixture();
    const bytes = Buffer.concat([Buffer.from("package example\n// "), Buffer.from([255]), Buffer.from("\nvar  answer=2\n")]);
    put(root, "main.go", bytes);
    const { report, directory } = propose(root);
    expect(report.status).toBe("failed");
    expect(report.error).toContain("non-UTF-8");
    expect(readFileSync(path.join(root, "main.go"))).toEqual(bytes);
    expect(existsSync(path.join(directory, "repair.patch"))).toBe(false);
  });

  it("rejects invalid limits and refuses linked artifact directories", () => {
    const root = fixture();
    expect(() => propose(root, { maxFiles: 0 })).toThrow();
    expect(() => propose(root, { maxPatchBytes: -1 })).toThrow();
    const outside = fixture();
    symlinkSync(outside, path.join(root, ".artifacts"), process.platform === "win32" ? "junction" : "dir");
    const before = readdirSync(outside);
    expect(() => propose(root)).toThrow("artifact directory");
    expect(readdirSync(outside)).toEqual(before);
  });
});
