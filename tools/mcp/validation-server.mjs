#!/usr/bin/env node
import { spawnSync } from "node:child_process";
import { createInterface } from "node:readline";
import path from "node:path";
import { fileURLToPath } from "node:url";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..", "..");
const tools = [
  {
    name: "repository_docs_check",
    description: "Run SyncHub's read-only documentation and evidence drift checks.",
    inputSchema: { type: "object", additionalProperties: false },
    annotations: { readOnlyHint: true },
  },
  {
    name: "repository_validation_commands",
    description: "List the repository validation commands an agent should run before handoff.",
    inputSchema: { type: "object", additionalProperties: false },
    annotations: { readOnlyHint: true },
  },
];

function write(message) {
  process.stdout.write(`${JSON.stringify(message)}\n`);
}

function result(id, value) {
  write({ jsonrpc: "2.0", id, result: value });
}

function failure(id, code, message) {
  write({ jsonrpc: "2.0", id, error: { code, message } });
}

function textResult(text, isError = false) {
  return { content: [{ type: "text", text }], isError };
}

function validationCommands() {
  return [
    "node scripts/dev.mjs check",
    "node scripts/dev.mjs verify",
    "node scripts/dev.mjs repair:verify",
  ].join("\n");
}

function docsCheck() {
  const output = spawnSync(process.execPath, ["scripts/dev.mjs", "docs"], {
    cwd: root,
    encoding: "utf8",
    timeout: 120_000,
    windowsHide: true,
  });
  const text = `${output.stdout || ""}${output.stderr || ""}`.trim() || "docs check produced no output";
  return textResult(text, Boolean(output.error || output.status));
}

function callTool(id, params) {
  if (params?.name === "repository_validation_commands") {
    result(id, textResult(validationCommands()));
  } else if (params?.name === "repository_docs_check") {
    result(id, docsCheck());
  } else {
    failure(id, -32602, `Unknown tool: ${params?.name || ""}`);
  }
}

const reader = createInterface({ input: process.stdin, output: process.stdout, terminal: false });
reader.on("line", line => {
  let message;
  try {
    message = JSON.parse(line);
  } catch {
    failure(null, -32700, "Parse error");
    return;
  }
  if (!message.id && message.method?.startsWith("notifications/")) return;
  switch (message.method) {
    case "initialize":
      result(message.id, {
        protocolVersion: message.params?.protocolVersion || "2024-11-05",
        capabilities: { tools: { listChanged: false } },
        serverInfo: { name: "synchub-validation", version: "1.0.0" },
      });
      break;
    case "tools/list":
      result(message.id, { tools });
      break;
    case "tools/call":
      callTool(message.id, message.params);
      break;
    default:
      failure(message.id, -32601, `Method not found: ${message.method || ""}`);
  }
});
