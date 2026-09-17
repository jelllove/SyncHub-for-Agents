function escapeMarkup(value) {
  return String(value)
    .replace(/[\u0000-\u0008\u000b\u000c\u000e-\u001f\uD800-\uDFFF\uFFFE\uFFFF]/gu, "")
    .replaceAll("&", "&amp;").replaceAll("<", "&lt;").replaceAll(">", "&gt;")
    .replaceAll('"', "&quot;").replaceAll("'", "&apos;");
}

function reportCases(report) {
  if (!["passed", "failed"].includes(report.status) || !Array.isArray(report.checks) || !report.source) {
    throw new Error("Only completed validation receipts with source evidence can be exported");
  }
  const cases = report.checks.map(check => {
    if (!["passed", "failed", "not-run"].includes(check.status) ||
      !/^[a-z0-9]+(?:-[a-z0-9]+)*$/.test(check.id)) {
      throw new Error("Invalid check outcome in validation receipt");
    }
    if (check.durationMs !== null && (!Number.isFinite(check.durationMs) || check.durationMs < 0)) {
      throw new Error("Invalid check duration in validation receipt");
    }
    if (check.log && !/^[a-z0-9-]+\.log$/.test(check.log)) throw new Error("Invalid validation log path");
    return {
      name: check.id, status: check.status, durationMs: check.durationMs,
      message: check.status === "failed" ? `Command failed; see ${check.log}` : "",
      log: check.status === "not-run" ? null : check.log,
    };
  });
  const sourceStatus = { stable: "passed", changed: "failed", error: "error" }[report.source.status];
  if (!sourceStatus) throw new Error("Source evidence is incomplete");
  cases.push({
    name: "source-integrity", status: sourceStatus, durationMs: null,
    message: report.source.error || "", log: null,
  });
  const successful = cases.every(check => check.status === "passed");
  if (successful !== (report.status === "passed")) throw new Error("Contradictory validation verdict");
  return cases;
}

export function renderJUnit(report) {
  const cases = reportCases(report);
  const count = status => cases.filter(check => check.status === status).length;
  const seconds = cases.reduce((total, check) => total + (check.durationMs || 0), 0) / 1000;
  const elements = cases.map(check => {
    const time = check.durationMs === null ? "" : ` time="${(check.durationMs / 1000).toFixed(3)}"`;
    let result = "";
    if (check.status === "failed" || check.status === "error") {
      const tag = check.status === "failed" ? "failure" : "error";
      result = `<${tag} message="${escapeMarkup(check.message)}">${escapeMarkup(check.message)}</${tag}>`;
    } else if (check.status === "not-run") {
      result = '<skipped message="Check was not executed"/>';
    }
    return `  <testcase classname="repository-validation" name="${escapeMarkup(check.name)}"${time}>${result}</testcase>`;
  });
  return [
    '<?xml version="1.0" encoding="UTF-8"?>',
    `<testsuite name="repository-validation" tests="${cases.length}" failures="${count("failed")}" errors="${count("error")}" skipped="${count("not-run")}" time="${seconds.toFixed(3)}">`,
    ...elements,
    "</testsuite>",
    "",
  ].join("\n");
}

export function renderHTML(report) {
  const cases = reportCases(report);
  const rows = cases.map(check => {
    const log = check.log ? `<a href="${encodeURIComponent(check.log)}">log</a>` : "-";
    return `<tr><td>${escapeMarkup(check.name)}</td><td>${check.status}</td><td>${check.durationMs ?? "-"}</td><td>${log}</td><td>${escapeMarkup(check.message)}</td></tr>`;
  });
  return [
    "<!doctype html>",
    '<html lang="en"><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1">',
    "<title>Repository validation</title>",
    "<style>body{font:16px system-ui;margin:2rem;max-width:80rem}table{border-collapse:collapse;width:100%}th,td{border:1px solid #ccc;padding:.5rem;text-align:left}code{overflow-wrap:anywhere}</style>",
    `<h1>Repository validation: ${report.status}</h1>`,
    `<p>Commit: <code>${escapeMarkup(report.commit)}</code></p>`,
    "<p>Rows represent validation check groups, not individual application test cases. This is not production qualification.</p>",
    '<p><a href="validation.json">JSON receipt</a> | <a href="junit.xml">JUnit XML</a></p>',
    "<table><thead><tr><th>Check</th><th>Outcome</th><th>Duration (ms)</th><th>Output</th><th>Details</th></tr></thead><tbody>",
    ...rows,
    "</tbody></table></html>",
    "",
  ].join("\n");
}
