import { createHash } from "node:crypto";
import { copyFileSync, mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import path from "node:path";
import {
  artifactDirectory, developmentReference, formattedGo, goFormattingChanges, ownedFile,
  readText, referenceEnd, referenceStart, replaceReference, repositoryFiles, run,
} from "./dev-lib.mjs";

const digest = text => createHash("sha256").update(text).digest("hex");

function collectChanges(root, maxFiles) {
  const changes = [];
  function add(file, before, after, kind) {
    if (before === after) return;
    changes.push({ path: file, before, after, kind });
    if (changes.length > maxFiles) throw new Error(`Maintenance file limit exceeded (${maxFiles})`);
  }
  for (const change of goFormattingChanges(root, repositoryFiles(root).sort())) {
    add(change.path, change.before, change.after, "gofmt");
  }
  const file = "docs/development.md";
  if (!ownedFile(root, file)) throw new Error(`Missing generated-reference document: ${file}`);
  const before = readText(path.join(root, file));
  const normalized = before.replaceAll("\r\n", "\n");
  const updated = replaceReference(normalized, developmentReference(root));
  if (updated !== normalized) {
    add(file, before, before.includes("\r\n") ? updated.replaceAll("\n", "\r\n") : updated, "generated-reference");
  }
  return changes;
}

function preparePatch(root, scratch, changes, maxPatchBytes) {
  const beforeRoot = path.join(scratch, "a");
  for (const change of changes) {
    for (const [tree, text] of [["a", change.before], ["b", change.after]]) {
      const filename = path.join(scratch, tree, change.path);
      mkdirSync(path.dirname(filename), { recursive: true });
      writeFileSync(filename, text);
    }
  }
  // Relative a/ and b/ trees produce standard patches without temporary paths.
  const patch = run("git", [
    "--no-pager", "-c", "core.autocrlf=false", "diff", "--no-color", "--no-index", "--no-prefix", "--binary",
    "--no-ext-diff", "--no-textconv", "--", "a", "b",
  ], scratch, { allowedExitCodes: [0, 1] });
  const rollback = run("git", [
    "--no-pager", "-c", "core.autocrlf=false", "diff", "--no-color", "--no-index", "--no-prefix", "--binary",
    "--no-ext-diff", "--no-textconv", "--", "b", "a",
  ], scratch, { allowedExitCodes: [0, 1] });
  if (!patch || !rollback || Buffer.byteLength(patch) > maxPatchBytes || Buffer.byteLength(rollback) > maxPatchBytes) {
    throw new Error(`Maintenance patch size limit exceeded or empty patch (${maxPatchBytes} bytes)`);
  }
  const patchFile = path.join(scratch, "candidate.patch");
  const rollbackFile = path.join(scratch, "rollback.patch");
  writeFileSync(patchFile, patch);
  writeFileSync(rollbackFile, rollback);
  // Apply only to an isolated snapshot, never the developer's working tree/index.
  run("git", ["init", "--quiet"], beforeRoot);
  run("git", ["-c", "core.autocrlf=false", "apply", "--check", patchFile], beforeRoot);
  run("git", ["-c", "core.autocrlf=false", "apply", patchFile], beforeRoot);
  for (const change of changes) {
    const applied = readText(path.join(beforeRoot, change.path));
    if (applied !== change.after) throw new Error(`Patch verification differs for ${change.path}`);
    const canonical = change.kind === "gofmt"
      ? formattedGo(applied, root)
      : replaceReference(applied.replaceAll("\r\n", "\n"), developmentReference(root));
    if (canonical.replaceAll("\r\n", "\n") !== applied.replaceAll("\r\n", "\n")) {
      throw new Error(`Repair is not canonical for ${change.path}`);
    }
    if (!ownedFile(root, change.path) || readText(path.join(root, change.path)) !== change.before) {
      throw new Error(`Source changed while preparing proposal: ${change.path}`);
    }
  }
  run("git", ["-c", "core.autocrlf=false", "apply", "--check", rollbackFile], beforeRoot);
  run("git", ["-c", "core.autocrlf=false", "apply", rollbackFile], beforeRoot);
  for (const change of changes) {
    const restored = readText(path.join(beforeRoot, change.path));
    if (restored !== change.before) throw new Error(`Rollback verification differs for ${change.path}`);
  }
  run("git", ["-c", "core.autocrlf=false", "apply", "--check", patchFile], beforeRoot);
  return { patch, rollback };
}

export function proposeMaintenance(root, { maxFiles = 50, maxPatchBytes = 1024 * 1024 } = {}) {
  if (!Number.isSafeInteger(maxFiles) || maxFiles < 1 ||
    !Number.isSafeInteger(maxPatchBytes) || maxPatchBytes < 1) {
    throw new Error("Maintenance limits must be positive integers");
  }
  const directory = artifactDirectory(root, "maintenance");
  const report = {
    schemaVersion: 1,
    kind: "maintenance-proposal",
    status: "running",
    startedAt: new Date().toISOString(),
    finishedAt: null,
    commit: null,
    limits: { maxFiles, maxPatchBytes },
    changes: [],
    verification: {
      patchApplies: false,
      canonical: false,
      rollbackApplies: false,
      rollbackRestoresOriginal: false,
      scope: "format-and-generated-reference",
    },
  };
  const save = () => writeFileSync(path.join(directory, "proposal.json"), JSON.stringify(report, null, 2) + "\n");
  save();
  const scratch = mkdtempSync(path.join(directory, "scratch-"));
  try {
    report.commit = run("git", ["rev-parse", "HEAD"], root).trim();
    const changes = collectChanges(root, maxFiles);
    report.changes = changes.map(change => ({
      path: change.path, kind: change.kind,
      beforeSha256: digest(change.before), afterSha256: digest(change.after),
    }));
    if (changes.length === 0) {
      report.status = "no-changes";
    } else {
      const { patch, rollback } = preparePatch(root, scratch, changes, maxPatchBytes);
      writeFileSync(path.join(directory, "repair.patch"), patch);
      writeFileSync(path.join(directory, "rollback.patch"), rollback);
      report.verification.patchApplies = true;
      report.verification.canonical = true;
      report.verification.rollbackApplies = true;
      report.verification.rollbackRestoresOriginal = true;
      report.status = "proposed";
    }
  } catch (error) {
    report.status = "failed";
    report.error = error instanceof Error ? error.message : String(error);
  } finally {
    rmSync(scratch, { recursive: true, force: true });
    report.finishedAt = new Date().toISOString();
    save();
  }
  return { report, directory };
}

function writeFixtureFile(root, file, content) {
  const filename = path.join(root, file);
  mkdirSync(path.dirname(filename), { recursive: true });
  writeFileSync(filename, content);
}

function createRollbackFixture(root) {
  writeFixtureFile(root, ".gitignore", ".artifacts/\n");
  writeFixtureFile(root, ".node-version", "24.17.0\n");
  writeFixtureFile(root, "go.mod", [
    "module example.test/rollback",
    "",
    "go 1.26.6",
    "require (",
    " github.com/wailsapp/wails/v3 v3.0.0-beta.8",
    ")",
    "",
  ].join("\n"));
  writeFixtureFile(root, "frontend/package.json", JSON.stringify({ scripts: { test: "vitest run" } }) + "\n");
  const referenceShell = [
    "Rollback verification fixture.",
    referenceStart,
    referenceEnd,
    "End fixture.",
    "",
  ].join("\n");
  writeFixtureFile(root, "docs/development.md", replaceReference(referenceShell, developmentReference(root)));
  writeFixtureFile(root, "main.go", "package example\n\nvar answer = 1\n");
  run("git", ["init", "--quiet"], root);
  run("git", ["add", "."], root);
  run("git", [
    "-c", "user.name=Rollback verification", "-c", "user.email=rollback@example.invalid",
    "-c", "commit.gpgsign=false", "-c", "core.hooksPath=",
    "commit", "--quiet", "-m", "fixture",
  ], root);
}

export function verifyMaintenanceRollback(root) {
  const directory = artifactDirectory(root, "rollback-verification");
  const report = {
    schemaVersion: 1,
    kind: "maintenance-rollback-verification",
    scenario: "review-only-maintenance-rollback",
    productionIncident: false,
    status: "running",
    startedAt: new Date().toISOString(),
    finishedAt: null,
    proposal: null,
    verification: {
      repairPatchApplies: false,
      rollbackPatchApplies: false,
      rollbackRestoresOriginal: false,
    },
    error: null,
  };
  const save = () => writeFileSync(path.join(directory, "rollback-verification.json"), JSON.stringify(report, null, 2) + "\n");
  save();
  const fixture = mkdtempSync(path.join(directory, "fixture-"));
  try {
    createRollbackFixture(fixture);
    const original = "package example\r\nvar  answer=2\r\n";
    writeFixtureFile(fixture, "main.go", original);
    const { report: proposal, directory: proposalDirectory } = proposeMaintenance(fixture);
    report.proposal = {
      status: proposal.status,
      changes: proposal.changes,
      verification: proposal.verification,
    };
    if (proposal.status !== "proposed") {
      throw new Error(`Expected a proposed maintenance rollback fixture, got ${proposal.status}`);
    }
    const repairPatch = path.join(proposalDirectory, "repair.patch");
    const rollbackPatch = path.join(proposalDirectory, "rollback.patch");
    const localRepair = path.join(directory, "repair.patch");
    const localRollback = path.join(directory, "rollback.patch");
    copyFileSync(repairPatch, localRepair);
    copyFileSync(rollbackPatch, localRollback);
    run("git", ["apply", "--check", localRepair], fixture);
    run("git", ["apply", localRepair], fixture);
    report.verification.repairPatchApplies = true;
    const repaired = readText(path.join(fixture, "main.go")).replaceAll("\r\n", "\n");
    if (repaired !== "package example\n\nvar answer = 2\n") {
      throw new Error("Repair patch did not produce canonical Go formatting");
    }
    run("git", ["apply", "--check", localRollback], fixture);
    run("git", ["apply", localRollback], fixture);
    report.verification.rollbackPatchApplies = true;
    const restored = readText(path.join(fixture, "main.go")).replaceAll("\r\n", "\n");
    if (restored !== original.replaceAll("\r\n", "\n")) {
      throw new Error("Rollback patch did not restore the original source");
    }
    report.verification.rollbackRestoresOriginal = true;
    report.status = "passed";
  } catch (error) {
    report.status = "failed";
    report.error = error instanceof Error ? error.message : String(error);
  } finally {
    rmSync(fixture, { recursive: true, force: true });
    report.finishedAt = new Date().toISOString();
    save();
  }
  return { report, directory };
}
