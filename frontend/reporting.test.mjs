// @vitest-environment node
import { expect, it } from "vitest";
import { JSDOM } from "jsdom";
import * as reporting from "../scripts/reporting.mjs";

function receipt(overrides = {}) {
  return {
    schemaVersion: 1,
    kind: "repository-validation",
    commit: "a".repeat(40),
    startedAt: "2026-09-17T00:00:00.000Z",
    finishedAt: "2026-09-17T00:00:01.500Z",
    status: "passed",
    source: { status: "stable", before: { sha256: "b".repeat(64) }, after: { sha256: "b".repeat(64) } },
    checks: [{ id: "build", status: "passed", durationMs: 1200, log: "build.log" }],
    ...overrides,
  };
}

function junit(report) {
  expect(reporting.renderJUnit).toBeTypeOf("function");
  return new JSDOM(reporting.renderJUnit(report), { contentType: "text/xml" }).window.document;
}

it("exports real check and source-integrity outcomes as valid JUnit", () => {
  const document = junit(receipt());
  const suite = document.querySelector("testsuite");
  expect(suite.getAttribute("tests")).toBe("2");
  expect(suite.getAttribute("failures")).toBe("0");
  expect(suite.getAttribute("errors")).toBe("0");
  expect(document.querySelector('testcase[name="build"]').getAttribute("time")).toBe("1.200");
  expect(document.querySelector('testcase[name="source-integrity"]')).not.toBeNull();
});

it("does not hide source mutations behind successful commands", () => {
  const document = junit(receipt({
    status: "failed",
    source: { status: "changed", error: "source <changed> & must rerun" },
  }));
  expect(document.querySelector("testsuite").getAttribute("failures")).toBe("1");
  expect(document.querySelector('testcase[name="source-integrity"] failure').textContent).toContain("source <changed> & must rerun");
});

it("represents unexecuted checks and capture errors without reporting success", () => {
  const document = junit(receipt({
    status: "failed",
    source: { status: "error", error: "capture failed" },
    checks: [{ id: "build", status: "not-run", durationMs: null, log: "build.log" }],
  }));
  expect(document.querySelector("testsuite").getAttribute("skipped")).toBe("1");
  expect(document.querySelector("testsuite").getAttribute("errors")).toBe("1");
  expect(document.querySelector('testcase[name="build"] skipped')).not.toBeNull();
});

it("escapes XML/HTML and strips invalid control characters in diagnostics", () => {
  const report = receipt({
    status: "failed",
    source: { status: "error", error: '<script>alert("x")</script>&\u0000\u001b' },
  });
  const document = junit(report);
  expect(document.querySelector("error").textContent).toContain('<script>alert("x")</script>&');
  expect(reporting.renderHTML).toBeTypeOf("function");
  const html = new JSDOM(reporting.renderHTML(report)).window.document;
  expect(html.querySelector("script")).toBeNull();
  expect(html.body.textContent).toContain('<script>alert("x")</script>&');
  expect(html.querySelector('a[href="validation.json"]')).not.toBeNull();
});

it("rejects unfinished or contradictory receipts", () => {
  expect(reporting.renderJUnit).toBeTypeOf("function");
  expect(() => reporting.renderJUnit(receipt({ status: "running" }))).toThrow();
  expect(() => reporting.renderJUnit(receipt({ status: "failed" }))).toThrow();
});
