import { createHash } from "node:crypto";
import { closeSync, fstatSync, lstatSync, openSync, readSync } from "node:fs";
import path from "node:path";
import { ownedFile, repositoryFiles, run } from "./dev-lib.mjs";

function sameFileIdentity(left, right) {
  return right && ["dev", "ino", "mode"].every(key => left[key] === right[key]);
}

function sameFileState(left, right) {
  return sameFileIdentity(left, right) && left.size === right.size && left.mtimeNs === right.mtimeNs;
}

function fileEntry(root, file, buffer) {
  const filename = path.join(root, file);
  const initial = lstatSync(filename, { throwIfNoEntry: false, bigint: true });
  if (!initial) return { path: file, kind: "missing" };
  if (!ownedFile(root, file)) throw new Error(`Cannot snapshot non-file source: ${file}`);
  const descriptor = openSync(filename, "r");
  try {
    const opened = fstatSync(descriptor, { bigint: true });
    // Windows path and handle change-times can differ without content changing.
    if (!sameFileIdentity(initial, opened)) throw new Error(`Source changed while opening: ${file}`);
    if (opened.size > BigInt(Number.MAX_SAFE_INTEGER)) throw new Error(`Source is too large to snapshot: ${file}`);
    const hash = createHash("sha256");
    let bytes = 0;
    let count;
    while ((count = readSync(descriptor, buffer, 0, buffer.length, null)) !== 0) {
      hash.update(buffer.subarray(0, count));
      bytes += count;
    }
    if (BigInt(bytes) !== opened.size || !sameFileState(opened, fstatSync(descriptor, { bigint: true })) ||
      !sameFileIdentity(opened, lstatSync(filename, { throwIfNoEntry: false, bigint: true })) ||
      !ownedFile(root, file)) {
      throw new Error(`Source changed while reading: ${file}`);
    }
    return { path: file, kind: "file", mode: Number(opened.mode & 0o777n), bytes, sha256: hash.digest("hex") };
  } finally {
    closeSync(descriptor);
  }
}

export function sourceSnapshot(root) {
  const head = run("git", ["rev-parse", "HEAD"], root).trim();
  const files = repositoryFiles(root).sort();
  const buffer = Buffer.alloc(64 * 1024);
  const entries = files.map(file => fileEntry(root, file, buffer));
  if (head !== run("git", ["rev-parse", "HEAD"], root).trim() ||
    JSON.stringify(files) !== JSON.stringify(repositoryFiles(root).sort())) {
    throw new Error("Repository revision or file inventory changed while capturing source");
  }
  // Hash framed metadata, not concatenated names/contents that could be ambiguous.
  const sha256 = createHash("sha256").update("synchub-source-v1\0").update(JSON.stringify(entries)).digest("hex");
  return {
    entries,
    summary: { sha256, head, files: entries.length, bytes: entries.reduce((total, entry) => total + (entry.bytes || 0), 0) },
  };
}

export function changedSourcePaths(before, after) {
  const oldFiles = new Map(before.entries.map(entry => [entry.path, JSON.stringify(entry)]));
  const newFiles = new Map(after.entries.map(entry => [entry.path, JSON.stringify(entry)]));
  return [...new Set([...oldFiles.keys(), ...newFiles.keys()])]
    .filter(file => oldFiles.get(file) !== newFiles.get(file)).sort();
}
