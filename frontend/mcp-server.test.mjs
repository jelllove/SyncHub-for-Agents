import test from "node:test";
import assert from "node:assert/strict";
import { spawn } from "node:child_process";
import path from "node:path";
import { fileURLToPath } from "node:url";

const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const server = path.join(root, "tools", "mcp", "validation-server.mjs");

function request(process, message) {
  return new Promise((resolve, reject) => {
    const timer = setTimeout(() => reject(new Error(`Timed out waiting for ${message.method}`)), 5_000);
    const onData = chunk => {
      const lines = String(chunk).trim().split(/\r?\n/).filter(Boolean);
      for (const line of lines) {
        const response = JSON.parse(line);
        if (response.id === message.id) {
          clearTimeout(timer);
          process.stdout.off("data", onData);
          resolve(response);
        }
      }
    };
    process.stdout.on("data", onData);
    process.stdin.write(`${JSON.stringify(message)}\n`, error => {
      if (error) {
        clearTimeout(timer);
        process.stdout.off("data", onData);
        reject(error);
      }
    });
  });
}

test("validation MCP server exposes read-only repository tools", async () => {
  const child = spawn(process.execPath, [server], { cwd: root, stdio: ["pipe", "pipe", "pipe"] });
  try {
    const initialized = await request(child, {
      jsonrpc: "2.0",
      id: 1,
      method: "initialize",
      params: { protocolVersion: "2024-11-05", capabilities: {}, clientInfo: { name: "test", version: "1" } },
    });
    assert.equal(initialized.result.serverInfo.name, "synchub-validation");
    assert.equal(initialized.result.capabilities.tools.listChanged, false);

    const tools = await request(child, { jsonrpc: "2.0", id: 2, method: "tools/list", params: {} });
    assert.deepEqual(tools.result.tools.map(tool => tool.name), [
      "repository_docs_check",
      "repository_validation_commands",
    ]);

    const commands = await request(child, {
      jsonrpc: "2.0",
      id: 3,
      method: "tools/call",
      params: { name: "repository_validation_commands", arguments: {} },
    });
    assert.equal(commands.result.isError, false);
    assert.match(commands.result.content[0].text, /node scripts\/dev\.mjs verify/);
    assert.match(commands.result.content[0].text, /node scripts\/dev\.mjs repair:verify/);
  } finally {
    child.kill();
  }
});
