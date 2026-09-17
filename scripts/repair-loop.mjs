import { copyFileSync, existsSync, mkdirSync, mkdtempSync, realpathSync, rmSync, statSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { pathToFileURL } from "node:url";
import { ownedFile, readText, run } from "./dev-lib.mjs";
import { proposeMaintenance } from "./maintenance.mjs";
import { sourceSnapshot } from "./source-snapshot.mjs";
import { runValidation, validationChecks } from "./validation.mjs";

function outside(source, target) {
  const relative = path.relative(source, target);
  return relative === ".." || relative.startsWith(`..${path.sep}`) || path.isAbsolute(relative);
}

function copyReceipt(result, destination) {
  mkdirSync(destination);
  const files = ["validation.json", "junit.xml", "index.html", ...result.report.checks.map(check => check.log)];
  for (const file of files) {
    if (ownedFile(result.directory, file)) copyFileSync(path.join(result.directory, file), path.join(destination, file));
  }
}

function nativeCheck(root) {
  return [{ id: "native-check", command: process.execPath, args: [path.join(root, "scripts", "dev.mjs"), "check"] }];
}

function expectFormattingFailure(result) {
  if (result.report.status !== "failed" || result.report.source.status !== "stable" ||
    result.report.checks[0].status !== "failed" ||
    !readText(path.join(result.directory, result.report.checks[0].log)).includes("Go formatting differs")) {
    throw new Error("The native check did not detect the controlled formatting fault");
  }
}

export function verifyRepairProbe(sourceRoot, outputRoot) {
  const source = realpathSync(sourceRoot);
  const requestedOutput = path.resolve(outputRoot);
  const output = path.join(realpathSync(path.dirname(requestedOutput)), path.basename(requestedOutput));
  if (!outside(source, output) || existsSync(output)) throw new Error("Use a new report directory outside the source checkout");
  const gitRoot = realpathSync(run("git", ["rev-parse", "--show-toplevel"], source).trim());
  const sourceInfo = statSync(source, { bigint: true });
  const gitInfo = statSync(gitRoot, { bigint: true });
  if (sourceInfo.dev !== gitInfo.dev || sourceInfo.ino !== gitInfo.ino) {
    throw new Error("Repair verification requires a repository root");
  }
  try {
    run("git", ["diff", "--quiet", "HEAD", "--"], source);
  } catch (error) {
    throw new Error("Repair verification requires committed source changes", { cause: error });
  }
  const original = sourceSnapshot(source).summary;
  mkdirSync(output);
  const report = {
    schemaVersion: 1,
    kind: "repair-verification",
    scenario: "diagnostic-go-format",
    sourceScope: "committed-source",
    productionIncident: false,
    commit: original.head,
    status: "running",
    startedAt: new Date().toISOString(),
    finishedAt: null,
    steps: [],
    snapshots: {},
    originalSourceUnchanged: false,
  };
  const save = () => writeFileSync(path.join(output, "repair.json"), JSON.stringify(report, null, 2) + "\n");
  save();
  const temporary = mkdtempSync(path.join(tmpdir(), "synchub-repair-work-"));
  const workspace = path.join(temporary, "checkout");
  try {
    const hooks = path.join(temporary, "empty-hooks");
    mkdirSync(hooks);
    run("git", ["-c", `core.hooksPath=${hooks}`, "clone", "--no-hardlinks", "--", source, workspace], temporary);
    run("git", ["config", "core.autocrlf", "false"], workspace);
    if (run("git", ["rev-parse", "HEAD"], workspace).trim() !== original.head) {
      throw new Error("Source revision changed while cloning the verification workspace");
    }
    const setup = run(process.execPath, [path.join(workspace, "scripts", "dev.mjs"), "setup"], workspace, { includeStderr: true });
    writeFileSync(path.join(output, "setup.log"), setup);
    const baseline = runValidation(workspace, validationChecks(workspace));
    copyReceipt(baseline, path.join(output, "baseline"));
    report.steps.push({ id: "baseline", outcome: baseline.report.status, receipt: "baseline/validation.json" });
    if (baseline.report.status !== "passed") throw new Error("Clean baseline validation failed; no diagnostic repair was attempted");
    report.snapshots.baseline = sourceSnapshot(workspace).summary.sha256;
    save();

    const probe = path.join(workspace, "app.go");
    if (!ownedFile(workspace, "app.go")) throw new Error("The controlled probe source app.go is unavailable");
    const before = readText(probe);
    const fault = before.replace(/^package ([A-Za-z_]\w*)(\r?)$/m, "package  $1$2");
    if (fault === before) throw new Error("Unable to inject the controlled formatting fault");
    writeFileSync(probe, fault);
    report.snapshots.injected = sourceSnapshot(workspace).summary.sha256;
    const failed = runValidation(workspace, nativeCheck(workspace));
    copyReceipt(failed, path.join(output, "injected-failure"));
    report.steps.push({ id: "injected-failure", outcome: failed.report.status, receipt: "injected-failure/validation.json" });
    expectFormattingFailure(failed);
    save();

    const proposal = proposeMaintenance(workspace);
    copyFileSync(path.join(proposal.directory, "proposal.json"), path.join(output, "proposal.json"));
    if (proposal.report.status !== "proposed" || proposal.report.changes.length !== 1 ||
      proposal.report.changes[0].path !== "app.go" || proposal.report.changes[0].kind !== "gofmt") {
      throw new Error("Proposal exceeded the controlled formatting-repair scope");
    }
    const patch = path.join(proposal.directory, "repair.patch");
    const changes = run("git", ["apply", "--numstat", patch], workspace).trim();
    if (!/^\d+\t\d+\tapp\.go$/.test(changes)) throw new Error("Repair patch contains unexpected paths");
    copyFileSync(patch, path.join(output, "repair.patch"));
    run("git", ["apply", "--check", patch], workspace);
    run("git", ["apply", patch], workspace);
    let repaired;
    try {
      repaired = runValidation(workspace, validationChecks(workspace));
      copyReceipt(repaired, path.join(output, "repaired"));
      report.steps.push({ id: "repaired", outcome: repaired.report.status, receipt: "repaired/validation.json" });
      report.snapshots.repaired = sourceSnapshot(workspace).summary.sha256;
      save();
    } finally {
      run("git", ["apply", "--reverse", "--check", patch], workspace);
      run("git", ["apply", "--reverse", patch], workspace);
      const rollback = runValidation(workspace, nativeCheck(workspace));
      copyReceipt(rollback, path.join(output, "rollback"));
      report.steps.push({ id: "rollback", outcome: rollback.report.status, receipt: "rollback/validation.json" });
      report.snapshots.rollback = sourceSnapshot(workspace).summary.sha256;
      expectFormattingFailure(rollback);
    }
    if (repaired.report.status !== "passed" || report.snapshots.repaired !== report.snapshots.baseline ||
      report.snapshots.rollback !== report.snapshots.injected) {
      throw new Error("Repair/revalidation/rollback did not preserve the expected source states");
    }
    report.status = "passed";
  } catch (error) {
    report.status = "failed";
    report.error = error instanceof Error ? error.message : String(error);
  } finally {
    rmSync(temporary, { recursive: true, force: true });
    const current = sourceSnapshot(source).summary;
    report.originalSourceUnchanged = current.head === original.head && current.sha256 === original.sha256;
    if (!report.originalSourceUnchanged) {
      report.status = "failed";
      report.error = "Original source changed during isolated verification";
    }
    report.finishedAt = new Date().toISOString();
    save();
  }
  return report;
}

if (process.argv[1] && import.meta.url === pathToFileURL(path.resolve(process.argv[1])).href) {
  try {
    const [source, output, ...extra] = process.argv.slice(2);
    if (!source || !output || extra.length) throw new Error("Usage: repair-loop.mjs <source-checkout> <new-output-directory>");
    const report = verifyRepairProbe(source, output);
    console.log(JSON.stringify(report, null, 2));
    if (report.status !== "passed") process.exitCode = 1;
  } catch (error) {
    console.error(error instanceof Error ? error.message : error);
    process.exitCode = 1;
  }
}
