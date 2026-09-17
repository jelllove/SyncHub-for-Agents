import path from "node:path";
import { fileURLToPath } from "node:url";
import { appendFileSync, readFileSync } from "node:fs";
import {
  commands, run, repositoryFiles, formatGo, checkMarkdownLinks, checkReference, readText,
} from "./dev-lib.mjs";
import { runValidation, validationChecks } from "./validation.mjs";
import { proposeMaintenance } from "./maintenance.mjs";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const [command, ...extra] = process.argv.slice(2);

function publishReportDirectory(directory) {
  if (process.env.GITHUB_OUTPUT) {
    appendFileSync(process.env.GITHUB_OUTPUT, `report_directory=${directory}\n`);
  }
}

try {
  if (!Object.hasOwn(commands, command) || extra.length) {
    throw new Error(`Usage: node scripts/dev.mjs <${Object.keys(commands).join("|")}>`);
  }
  if (command === "setup") {
    const expected = readText(path.join(root, ".node-version")).trim();
    if (process.versions.node !== expected) {
      throw new Error(`Setup requires Node ${expected}; found ${process.versions.node}. Select the pinned version first.`);
    }
    run("go", ["version"], root);
    run("go", ["mod", "download"], root);
    run("npm", ["ci", "--prefix", "frontend"], root);
    run("npm", ["--prefix", "frontend", "run", "build"], root);
  } else if (command === "verify") {
    const { report, directory } = runValidation(root, validationChecks(root), {
      summaryPath: process.env.GITHUB_STEP_SUMMARY,
    });
    for (const check of report.checks) {
      const duration = check.durationMs === null ? "not run" : `${check.durationMs} ms`;
      console.log(`${check.id}: ${check.status} (${duration})`);
      if (check.status === "failed") console.error(readFileSync(path.join(directory, check.log), "utf8"));
    }
    console.log(`Source snapshot: ${report.source.status}`);
    if (report.source.error) console.error(report.source.error);
    publishReportDirectory(directory);
    console.log(`Validation artifacts: ${directory}`);
    if (report.status !== "passed") throw new Error("Repository validation failed; see the check logs above.");
  } else if (command === "propose") {
    const { report, directory } = proposeMaintenance(root);
    publishReportDirectory(directory);
    console.log(`Maintenance proposal: ${report.status}; ${report.changes.length} file(s). Artifacts: ${directory}`);
    if (report.status === "failed") throw new Error(report.error);
  } else {
    const files = repositoryFiles(root);
    switch (command) {
      case "check":
        formatGo(root, files);
        run("go", ["vet", "./..."], root);
        run("npm", ["--prefix", "frontend", "run", "lint"], root);
        run("npm", ["--prefix", "frontend", "run", "typecheck"], root);
        checkMarkdownLinks(root, files);
        checkReference(root);
        break;
      case "format":
      case "cleanup":
        formatGo(root, files, true);
        break;
      case "docs":
        checkMarkdownLinks(root, files);
        checkReference(root);
        break;
      case "docs:write":
        checkReference(root, true);
        break;
    }
  }
  console.log(`${command}: passed`);
} catch (error) {
  console.error(error instanceof Error ? error.message : error);
  process.exitCode = 1;
}
