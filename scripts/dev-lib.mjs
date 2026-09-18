import { spawnSync } from "node:child_process";
import { readFileSync, realpathSync, lstatSync, writeFileSync, mkdirSync, mkdtempSync, rmSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { checkEvidenceDrift } from "./doc-drift.mjs";

export const commands = {
  setup: "Restore locked dependencies and build embedded frontend assets.",
  check: "Check Go formatting/vet, frontend ESLint/TypeScript, and documentation without rewriting files.",
  verify: "Build frontend assets, run shared checks and all Go/frontend tests, and save results and logs.",
  propose: "Prepare bounded, review-only Go formatting/reference repair and rollback patches; never apply or commit them.",
  "repair:verify": "Verify a diagnostic failure/repair/rollback cycle in a restricted Linux container.",
  format: "Apply gofmt to repository-owned Go files only; never stage or commit.",
  cleanup: "Run one bounded formatting repair pass; never delete files or touch sync data.",
  docs: "Check local Markdown file links and the generated development reference.",
  "docs:write": "Refresh only the generated development reference.",
};

export function run(command, args, root, { timeout = 600_000, input, allowedExitCodes = [0], includeStderr = false } = {}) {
  // npm.cmd needs cmd.exe on Windows; npm arguments here are fixed by this tool.
  const windowsNpm = process.platform === "win32" && command === "npm";
  const executable = windowsNpm ? (process.env.ComSpec || "cmd.exe") : command;
  const argv = windowsNpm ? ["/d", "/s", "/c", "npm", ...args] : args;
  const result = spawnSync(executable, argv, {
    cwd: root,
    encoding: "utf8",
    timeout,
    maxBuffer: 16 * 1024 * 1024,
    windowsHide: true,
    input,
  });
  if (result.error || result.status === null || !allowedExitCodes.includes(result.status)) {
    throw new Error(
      `${command} ${args.join(" ")} failed: ${result.error?.message || `exit ${result.status}`}\n` +
      (result.stdout || "") + (result.stderr || ""),
    );
  }
  return result.stdout + (includeStderr ? result.stderr : "");
}

export function repositoryFiles(root) {
  return run("git", ["ls-files", "-z", "--cached", "--others", "--exclude-standard"], root)
    .split("\0").filter(Boolean).filter((file, index, files) => files.indexOf(file) === index);
}

function withinRoot(root, target) {
  const relative = path.relative(root, target);
  return relative !== ".." && !relative.startsWith(`..${path.sep}`) && !path.isAbsolute(relative);
}

export function ownedFile(root, file) {
  const target = path.resolve(root, file);
  if (!withinRoot(root, target)) throw new Error(`Path escapes repository: ${file}`);
  const stat = lstatSync(target, { throwIfNoEntry: false });
  if (!stat) return false; // A tracked deletion is not a current source file.
  if (stat.isSymbolicLink() || !withinRoot(realpathSync(root), realpathSync(target))) {
    throw new Error(`Refusing linked source file: ${file}`);
  }
  return stat.isFile();
}

export function artifactDirectory(root, group) {
  if (typeof group !== "string" || !/^[a-z]+(?:-[a-z]+)*$/.test(group)) {
    throw new Error("Invalid artifact directory group");
  }
  let directory = realpathSync(root);
  for (const part of [".artifacts", group]) {
    directory = path.join(directory, part);
    const entry = lstatSync(directory, { throwIfNoEntry: false });
    if (entry && (entry.isSymbolicLink() || !entry.isDirectory())) {
      throw new Error(`Refusing unsafe artifact directory: ${directory}`);
    }
    if (!entry) mkdirSync(directory);
  }
  return mkdtempSync(path.join(directory, "run-"));
}

export function formattedGo(original, root) {
  // Validate formatting rather than Windows checkout line endings.
  const normalized = original.replaceAll("\r\n", "\n");
  const formatted = run("gofmt", [], root, { input: normalized });
  if (formatted === normalized) return original;
  return original.includes("\r\n") ? formatted.replaceAll("\n", "\r\n") : formatted;
}

export function readText(filename) {
  const bytes = readFileSync(filename);
  const text = bytes.toString("utf8");
  if (!Buffer.from(text).equals(bytes)) throw new Error(`Refusing non-UTF-8 source: ${filename}`);
  return text;
}

export function goFormattingChanges(root, files) {
  const sources = [...new Set(files)]
    .filter(file => file.endsWith(".go") && ownedFile(root, file))
    .map((file, index) => ({ path: file, before: readText(path.join(root, file)), slot: `source-${index}.go` }));
  if (sources.length === 0) return [];
  const scratch = mkdtempSync(path.join(tmpdir(), "synchub-gofmt-"));
  const paths = new Map(sources.map(source => [source.slot, source.path]));
  try {
    for (const source of sources) {
      writeFileSync(path.join(scratch, source.slot), source.before.replaceAll("\r\n", "\n"));
    }
    // Short, generated names bound Windows command length and keep diagnostics mappable.
    for (let index = 0; index < sources.length; index += 128) {
      try {
        run("gofmt", ["-w", ...sources.slice(index, index + 128).map(source => source.slot)], scratch);
      } catch (error) {
        const message = error instanceof Error ? error.message : String(error);
        throw new Error(message.replace(/\bsource-\d+\.go\b/g, slot => paths.get(slot) ?? slot), { cause: error });
      }
    }
    const changes = [];
    for (const source of sources) {
      if (!ownedFile(root, source.path) || readText(path.join(root, source.path)) !== source.before) {
        throw new Error(`Source changed while checking Go formatting: ${source.path}`);
      }
      const formatted = readText(path.join(scratch, source.slot));
      if (formatted !== source.before.replaceAll("\r\n", "\n")) {
        changes.push({
          path: source.path,
          before: source.before,
          after: source.before.includes("\r\n") ? formatted.replaceAll("\n", "\r\n") : formatted,
        });
      }
    }
    return changes;
  } finally {
    rmSync(scratch, { recursive: true, force: true });
  }
}

export function formatGo(root, files, write = false) {
  const changes = goFormattingChanges(root, files);
  if (!write && changes.length) {
    throw new Error(`Go formatting differs; run node scripts/dev.mjs format:\n${changes.map(change => change.path).join("\n")}`);
  }
  if (!write) return;
  for (const change of changes) {
    if (!ownedFile(root, change.path) || readText(path.join(root, change.path)) !== change.before) {
      throw new Error(`Source changed before applying Go formatting: ${change.path}`);
    }
  }
  for (const change of changes) {
    writeFileSync(path.join(root, change.path), change.after);
  }
}

function withoutCode(markdown) {
  let fence = null;
  return markdown.split(/\r?\n/).map(line => {
    const match = line.match(/^\s{0,3}(`{3,}|~{3,})/);
    if (match) {
      if (!fence) fence = match[1];
      else if (match[1][0] === fence[0] && match[1].length >= fence.length) fence = null;
      return "";
    }
    return fence ? "" : line.replace(/(`+)[\s\S]*?\1/g, "");
  }).join("\n").replace(/<!--[\s\S]*?-->/g, "");
}

export function markdownTargets(markdown) {
  const text = withoutCode(markdown);
  const targets = [];
  // Inline links/images and reference definitions. Fragments are checked only
  // for file existence, not renderer-specific heading anchor semantics.
  const inline = /!?\[[^\]\n]*\]\(\s*(?:<([^>\n]+)>|((?:[^()\s\\]|\\.|(?:\([^()\n]*\)))+))(?:\s+["'][^"\n]*["'])?\s*\)/g;
  const references = /^\s{0,3}\[[^\]\n]+\]:\s*(?:<([^>\n]+)>|(\S+))/gm;
  for (const pattern of [inline, references]) {
    for (const match of text.matchAll(pattern)) targets.push(match[1] || match[2]);
  }
  return targets;
}

export function checkMarkdownLinks(root, files) {
  const errors = [];
  const available = new Set(files.filter(file => ownedFile(root, file)));
  for (const file of available) {
    if (!file.toLowerCase().endsWith(".md")) continue;
    for (const target of markdownTargets(readText(path.join(root, file)))) {
      if (/^(?:[a-z][a-z\d+.-]*:|\/\/|#)/i.test(target)) continue;
      const destination = decodeURIComponent(target.split(/[?#]/)[0]).replace(/\\([\\()[\] ])/g, "$1");
      const absolute = destination.startsWith("/")
        ? path.resolve(root, `.${destination}`)
        : path.resolve(root, path.dirname(file), destination);
      const relative = path.relative(root, absolute).split(path.sep).join("/");
      if (!withinRoot(root, absolute)) {
        errors.push(`${file}: link escapes repository: ${target}`);
      } else if (!available.has(relative) &&
        ![...available].some(candidate => relative === "" || candidate.startsWith(`${relative}/`))) {
        errors.push(`${file}: missing repository link target: ${target}`);
      }
    }
  }
  if (errors.length) throw new Error(errors.join("\n"));
}

export const referenceStart = "<!-- dev-reference:start -->";
export const referenceEnd = "<!-- dev-reference:end -->";

export function developmentReference(root) {
  const goMod = readText(path.join(root, "go.mod"));
  const goVersion = goMod.match(/^go\s+(\S+)$/m)?.[1];
  const wailsVersion = goMod.match(/^\s*github\.com\/wailsapp\/wails\/v3\s+(\S+)/m)?.[1];
  if (!goVersion || !wailsVersion) throw new Error("Missing Go or Wails version in go.mod");
  const nodeVersion = readText(path.join(root, ".node-version")).trim();
  const frontend = JSON.parse(readText(path.join(root, "frontend", "package.json")));
  const escape = value => value.replaceAll("|", "\\|");
  return [
    referenceStart,
    "## Generated tool and command reference",
    "",
    "Regenerate with `node scripts/dev.mjs docs:write`; CI rejects stale content.",
    "",
    `- Go minimum: \`${goVersion}\` (from \`go.mod\`).`,
    `- Node.js: \`${nodeVersion}\` (from \`.node-version\`).`,
    `- Wails CLI: \`${wailsVersion}\` (from \`go.mod\`).`,
    "",
    "| Repository command | Purpose |",
    "| --- | --- |",
    ...Object.entries(commands).map(([name, description]) => `| \`node scripts/dev.mjs ${name}\` | ${description} |`),
    "",
    "| Frontend command | Executed script |",
    "| --- | --- |",
    ...Object.entries(frontend.scripts).map(([name, script]) => `| \`npm --prefix frontend run ${name}\` | \`${escape(script)}\` |`),
    referenceEnd,
  ].join("\n");
}

export function replaceReference(content, generated) {
  if (content.split(referenceStart).length !== 2 || content.split(referenceEnd).length !== 2) {
    throw new Error("Development documentation must contain exactly one reference marker pair");
  }
  const start = content.indexOf(referenceStart);
  const end = content.indexOf(referenceEnd);
  if (end < start) throw new Error("Development reference markers are reversed");
  return content.slice(0, start) + generated + content.slice(end + referenceEnd.length);
}

export function checkReference(root, write = false) {
  const file = "docs/development.md";
  if (!ownedFile(root, file)) throw new Error(`Missing ${file}`);
  const filename = path.join(root, file);
  const original = readText(filename).replaceAll("\r\n", "\n");
  const expected = replaceReference(original, developmentReference(root));
  if (original !== expected) {
    if (!write) throw new Error("Development reference is stale; run node scripts/dev.mjs docs:write");
    writeFileSync(filename, expected);
  }
}

export { checkEvidenceDrift };
