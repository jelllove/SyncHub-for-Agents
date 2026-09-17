import { appendFileSync, writeFileSync } from "node:fs";
import path from "node:path";
import { artifactDirectory, run } from "./dev-lib.mjs";
import { changedSourcePaths, sourceSnapshot } from "./source-snapshot.mjs";
import { renderHTML, renderJUnit } from "./reporting.mjs";

export function validationChecks(root) {
  return [
    { id: "frontend-build", command: "npm", args: ["--prefix", "frontend", "run", "build"] },
    { id: "repository-checks", command: process.execPath, args: [path.join(root, "scripts", "dev.mjs"), "check"] },
    { id: "go-tests", command: "go", args: ["test", "-json", "./..."] },
    { id: "lint-tests", command: "npm", args: ["--prefix", "frontend", "run", "test:lint"] },
    { id: "frontend-tests", command: "npm", args: ["--prefix", "frontend", "test"] },
  ];
}

export function runValidation(root, checks, { summaryPath } = {}) {
  if (!Array.isArray(checks) || checks.length === 0) throw new Error("At least one validation check is required");
  const identifiers = new Set();
  for (const check of checks) {
    if (typeof check?.id !== "string" || !/^[a-z0-9]+(?:-[a-z0-9]+)*$/.test(check.id) || identifiers.has(check.id) ||
      typeof check.command !== "string" || !check.command ||
      !Array.isArray(check.args) || check.args.some(arg => typeof arg !== "string") ||
      (check.timeout !== undefined && (!Number.isFinite(check.timeout) || check.timeout < 1 || check.timeout > 600_000))) {
      throw new Error(`Invalid validation check: ${check?.id}`);
    }
    identifiers.add(check.id);
  }
  const report = {
    schemaVersion: 1,
    kind: "repository-validation",
    commit: run("git", ["rev-parse", "HEAD"], root).trim(),
    worktreeDirty: run("git", ["status", "--porcelain=v1"], root).trim().length > 0,
    platform: process.platform,
    nodeVersion: process.versions.node,
    startedAt: new Date().toISOString(),
    finishedAt: null,
    status: "running",
    source: {
      algorithm: "sha256",
      scope: "git-listed-working-tree-bytes-v1",
      status: "pending",
      before: null,
      after: null,
      changedPaths: [],
    },
    checks: checks.map(check => ({
      id: check.id, command: check.command, args: check.args,
      status: "pending", durationMs: null, log: `${check.id}.log`,
    })),
  };
  const directory = artifactDirectory(root, "validation");
  const save = () => writeFileSync(path.join(directory, "validation.json"), JSON.stringify(report, null, 2) + "\n");
  save();
  function captureSource(phase) {
    try {
      const snapshot = sourceSnapshot(root);
      report.source[phase] = snapshot.summary;
      return snapshot;
    } catch (error) {
      report.source.status = "error";
      report.source.error = `Cannot capture ${phase} source snapshot: ${error instanceof Error ? error.message : String(error)}`;
      return null;
    }
  }
  const before = captureSource("before");
  if (before) {
    report.commit = before.summary.head;
    report.source.status = "running";
    save();
    for (const [index, check] of checks.entries()) {
      const result = report.checks[index];
      result.status = "running";
      save();
      const started = performance.now();
      let output;
      try {
        output = run(check.command, check.args, root, { timeout: check.timeout ?? 600_000, includeStderr: true });
        result.status = "passed";
      } catch (error) {
        output = error instanceof Error ? error.message : String(error);
        result.status = "failed";
      }
      result.durationMs = Math.round(performance.now() - started);
      writeFileSync(path.join(directory, result.log), output);
      save();
    }
    const after = captureSource("after");
    if (after) {
      report.source.changedPaths = changedSourcePaths(before, after);
      report.source.status = before.summary.sha256 === after.summary.sha256 &&
        before.summary.head === after.summary.head ? "stable" : "changed";
      if (report.source.status === "changed") {
        report.source.error = "Source changed during validation; review the changed paths and rerun on a stable checkout.";
      }
    }
  } else {
    for (const result of report.checks) result.status = "not-run";
  }
  report.status = report.source.status === "stable" && report.checks.every(check => check.status === "passed")
    ? "passed" : "failed";
  report.finishedAt = new Date().toISOString();
  const junit = renderJUnit(report);
  const html = renderHTML(report);
  writeFileSync(path.join(directory, "junit.xml"), junit, { flag: "wx" });
  writeFileSync(path.join(directory, "index.html"), html, { flag: "wx" });
  save();
  if (summaryPath) {
    appendFileSync(summaryPath, [
      `\n## Repository validation: ${report.status}`,
      "",
      `Commit: \`${report.commit}\`; uncommitted changes: ${report.worktreeDirty}.`,
      "",
      `Source snapshot: **${report.source.status}**.`,
      `Before: \`${report.source.before?.sha256 || "unavailable"}\`.`,
      `After: \`${report.source.after?.sha256 || "unavailable"}\`.`,
      "",
      "| Check | Result | Duration (ms) |",
      "| --- | --- | ---: |",
      ...report.checks.map(check => `| ${check.id} | ${check.status} | ${check.durationMs ?? "-"} |`),
      "",
      "Download the repository-validation artifact for command output and the JSON receipt.",
      "",
    ].join("\n"));
  }
  return { report, directory };
}
