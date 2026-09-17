// @vitest-environment node
import { expect, it } from "vitest";
import * as container from "../scripts/repair-container.mjs";

it("contains repair verification without exposing host credentials or Docker control", () => {
  expect(container.repairContainerArguments).toBeTypeOf("function");
  const args = container.repairContainerArguments({
    source: "C:\\repo", output: "C:\\reports\\run-1",
    image: `sha256:${"a".repeat(64)}`, name: "synchub-repair-test", uid: 1001, gid: 1001,
  });
  expect(args).toContain("--read-only");
  expect(args).toContain("--rm");
  expect(args).toContain("--cap-drop=ALL");
  expect(args).toContain("--security-opt=no-new-privileges");
  expect(args).toContain("--pids-limit=512");
  expect(args).toContain("--memory=6g");
  expect(args).toContain("--cpus=2");
  expect(args).toContain("1001:1001");
  expect(args).toContain("type=bind,source=C:\\repo,target=/source,readonly");
  expect(args).toContain("type=bind,source=C:\\reports\\run-1,target=/results");
  expect(args).not.toContain("--privileged");
  expect(args.join(" ")).not.toContain("docker.sock");
  expect(args.join(" ")).not.toContain("--network=host");
  expect(args).toContain("HOME=/tmp/synchub-home");
  expect(args).toContain("GIT_CONFIG_COUNT=2");
  expect(args).toContain("GIT_CONFIG_KEY_0=safe.directory");
  expect(args).toContain("GIT_CONFIG_VALUE_0=/source");
  expect(args).toContain("GIT_CONFIG_KEY_1=safe.directory");
  expect(args).toContain("GIT_CONFIG_VALUE_1=/source/.git");
  expect(args.join(" ")).not.toContain("safe.directory=*");
  expect(args.slice(-4)).toEqual(["node", "/source/scripts/repair-loop.mjs", "/source", "/results/proof"]);
});

it("rejects root execution and ambiguous mount specifications", () => {
  expect(container.repairContainerArguments).toBeTypeOf("function");
  const options = {
    source: "C:\\repo", output: "C:\\reports",
    image: `sha256:${"a".repeat(64)}`, name: "synchub-repair-test", uid: 1000, gid: 1000,
  };
  expect(() => container.repairContainerArguments({ ...options, uid: 0 })).toThrow();
  expect(() => container.repairContainerArguments({ ...options, source: "C:\\repo,readonly=false" })).toThrow();
  expect(() => container.repairContainerArguments({ ...options, image: "mutable:latest" })).toThrow();
});
