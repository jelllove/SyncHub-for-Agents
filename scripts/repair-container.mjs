import { randomUUID } from "node:crypto";
import { writeFileSync } from "node:fs";
import path from "node:path";
import { artifactDirectory, readText, run } from "./dev-lib.mjs";
import { sourceSnapshot } from "./source-snapshot.mjs";

export function repairContainerArguments({ source, output, image, name, uid, gid }) {
  if (![source, output].every(value => typeof value === "string" && value && !/[\r\n,]/.test(value)) ||
    !/^sha256:[a-f0-9]{64}$/.test(image) || !/^synchub-repair-[a-z0-9-]+$/.test(name) ||
    !Number.isSafeInteger(uid) || uid <= 0 || !Number.isSafeInteger(gid) || gid <= 0) {
    throw new Error("Invalid repair container configuration");
  }
  return [
    "run", "--rm", "--name", name, "--read-only",
    "--cap-drop=ALL", "--security-opt=no-new-privileges", "--pids-limit=512",
    "--memory=6g", "--cpus=2", "--user", `${uid}:${gid}`,
    "--tmpfs", "/tmp:rw,exec,nosuid,size=4g",
    "--env", "HOME=/tmp/synchub-home", "--env", "USERPROFILE=/tmp/synchub-home",
    "--env", "XDG_CONFIG_HOME=/tmp/synchub-home/config", "--env", "XDG_CACHE_HOME=/tmp/synchub-cache",
    "--env", "GOPATH=/tmp/synchub-go", "--env", "GOCACHE=/tmp/synchub-gocache",
    "--env", "npm_config_cache=/tmp/synchub-npm",
    "--env", "GIT_CONFIG_COUNT=2", "--env", "GIT_CONFIG_KEY_0=safe.directory", "--env", "GIT_CONFIG_VALUE_0=/source",
    "--env", "GIT_CONFIG_KEY_1=safe.directory", "--env", "GIT_CONFIG_VALUE_1=/source/.git",
    "--mount", `type=bind,source=${source},target=/source,readonly`,
    "--mount", `type=bind,source=${output},target=/results`,
    image, "node", "/source/scripts/repair-loop.mjs", "/source", "/results/proof",
  ];
}

export function runContainerRepairProbe(root) {
  try {
    run("git", ["diff", "--quiet", "HEAD", "--"], root);
  } catch (error) {
    throw new Error("Container repair verification requires committed source changes", { cause: error });
  }
  const before = sourceSnapshot(root).summary;
  const directory = artifactDirectory(root, "repair-verification");
  const name = `synchub-repair-${randomUUID()}`;
  const report = {
    schemaVersion: 1, kind: "contained-repair-verification", scenario: "diagnostic-go-format",
    productionIncident: false, commit: before.head, status: "running",
    startedAt: new Date().toISOString(), finishedAt: null,
    limits: {
      cpus: 2, memoryGiB: 6, temporaryGiB: 4, processes: 512, nonRoot: true, readOnlySource: true,
      imageBuildTimeoutMs: 1_200_000, verificationTimeoutMs: 1_200_000,
    },
    image: null, originalSourceUnchanged: false, errors: [],
  };
  const save = () => writeFileSync(path.join(directory, "container.json"), JSON.stringify(report, null, 2) + "\n");
  save();
  let attemptedContainer = false;
  try {
    if (run("docker", ["info", "--format", "{{.OSType}}"], root).trim() !== "linux") {
      throw new Error("Linux containers are required for contained repair verification");
    }
    const tag = `synchub-repair-dev:${before.head.slice(0, 12)}`;
    const build = run("docker", ["build", "--progress=plain", "--tag", tag, ".devcontainer"], root, {
      includeStderr: true, timeout: report.limits.imageBuildTimeoutMs,
    });
    writeFileSync(path.join(directory, "image-build.log"), build);
    const image = run("docker", ["image", "inspect", tag, "--format", "{{.Id}}"], root).trim();
    report.image = image;
    const uid = process.getuid?.() > 0 ? process.getuid() : 1000;
    const gid = process.getgid?.() > 0 ? process.getgid() : 1000;
    const args = repairContainerArguments({ source: path.resolve(root), output: directory, image, name, uid, gid });
    attemptedContainer = true;
    const output = run("docker", args, root, { includeStderr: true, timeout: report.limits.verificationTimeoutMs });
    writeFileSync(path.join(directory, "worker.log"), output);
    const proof = JSON.parse(readText(path.join(directory, "proof", "repair.json")));
    if (proof.status !== "passed" || proof.commit !== before.head || !proof.originalSourceUnchanged ||
      proof.scenario !== "diagnostic-go-format" || proof.productionIncident !== false) {
      throw new Error("The contained repair proof did not satisfy the expected contract");
    }
    report.status = "passed";
  } catch (error) {
    report.status = "failed";
    const message = error instanceof Error ? error.message : String(error);
    report.errors.push(message);
    writeFileSync(path.join(directory, "failure.log"), message);
  } finally {
    if (attemptedContainer) {
      try {
        run("docker", ["container", "rm", "--force", name], root);
      } catch (error) {
        const message = error instanceof Error ? error.message : String(error);
        if (!message.includes(`No such container: ${name}`)) {
          report.status = "failed";
          report.errors.push(`Container cleanup failed: ${message}`);
        }
      }
    }
    try {
      const after = sourceSnapshot(root).summary;
      report.originalSourceUnchanged = after.head === before.head && after.sha256 === before.sha256;
      if (!report.originalSourceUnchanged) {
        report.status = "failed";
        report.errors.push("Original source changed during contained verification");
      }
    } catch (error) {
      report.status = "failed";
      report.errors.push(`Cannot verify original source: ${error instanceof Error ? error.message : String(error)}`);
    }
    report.finishedAt = new Date().toISOString();
    save();
  }
  return { report, directory };
}
