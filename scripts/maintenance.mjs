import { createHash } from "node:crypto";
import { mkdirSync, mkdtempSync, rmSync, writeFileSync } from "node:fs";
import path from "node:path";
import {
  artifactDirectory, developmentReference, formattedGo, goFormattingChanges, ownedFile,
  readText, replaceReference, repositoryFiles, run,
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
