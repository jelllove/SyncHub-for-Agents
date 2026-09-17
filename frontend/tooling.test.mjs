// @vitest-environment node
import { afterEach, describe, expect, it } from "vitest";
import { mkdtempSync, mkdirSync, readFileSync, writeFileSync, rmSync, symlinkSync } from "node:fs";
import { tmpdir } from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";
import sharp from "sharp";
import {
  markdownTargets, checkMarkdownLinks, replaceReference, referenceStart, referenceEnd,
  developmentReference, checkReference, ownedFile, repositoryFiles, run, formatGo,
} from "../scripts/dev-lib.mjs";

const roots = [];
function fixture() {
  const root = mkdtempSync(path.join(tmpdir(), "synchub-tooling-"));
  roots.push(root);
  return root;
}
function put(root, file, content = "") {
  const filename = path.join(root, file);
  mkdirSync(path.dirname(filename), { recursive: true });
  writeFileSync(filename, content);
}
afterEach(() => {
  for (const root of roots.splice(0)) rmSync(root, { recursive: true, force: true });
});

describe("repository documentation checks", () => {
  it("extracts inline/image/reference targets but ignores fenced code and comments", () => {
    const input = [
      "[guide](docs/guide.md#setup)", "![picture](assets/picture.png)",
      "[a][ref]", "[ref]: <docs/a b.md>", "[paren](docs/a(b).md)",
      "~~~markdown", "[example](missing.md)", "~~~",
      "`[inline](missing.md)`", "<!-- [hidden](missing.md) -->",
    ].join("\n");
    expect(markdownTargets(input)).toEqual([
      "docs/guide.md#setup", "assets/picture.png", "docs/a(b).md", "docs/a b.md",
    ]);
  });

  it("checks all Markdown paths including historical specs and directory links", () => {
    const root = fixture();
    put(root, "README.md", "[guide](docs/guide.md#setup)\n[docs](docs)\n[web](https://example.org/missing)");
    put(root, "docs/guide.md", "[root](/README.md)\n[space](a%20b.md)\n[back](../README.md)");
    put(root, "docs/a b.md");
    put(root, "docs/superpowers/specs/old.md", "[bad](gone.md)");
    const files = ["README.md", "docs/guide.md", "docs/a b.md", "docs/superpowers/specs/old.md"];
    expect(() => checkMarkdownLinks(root, files)).toThrow("old.md: missing repository link target");
    put(root, "docs/superpowers/specs/old.md", "[guide](../../guide.md)");
    expect(() => checkMarkdownLinks(root, files)).not.toThrow();
  });

  it("rejects case mismatches, ignored targets, deleted targets, and repository escapes", () => {
    const root = fixture();
    put(root, "guide.md");
    put(root, "ignored.md");
    for (const target of ["Guide.md", "ignored.md", "deleted.md", "../outside.md"]) {
      put(root, "README.md", `[bad](${target})`);
      expect(() => checkMarkdownLinks(root, ["README.md", "guide.md", "deleted.md"])).toThrow();
    }
  });

  it("requires exactly one ordered reference marker pair and preserves surrounding prose", () => {
    const generated = `${referenceStart}\nnew\n${referenceEnd}`;
    expect(replaceReference(`intro\n${referenceStart}\nold\n${referenceEnd}\noutro`, generated))
      .toBe(`intro\n${generated}\noutro`);
    expect(() => replaceReference("no markers", generated)).toThrow();
    expect(() => replaceReference(`${referenceEnd}${referenceStart}`, generated)).toThrow();
    expect(() => replaceReference(`${generated}${generated}`, generated)).toThrow();
  });

  it("detects version and command drift, and regenerates only the marked section", () => {
    const root = fixture();
    put(root, "go.mod", "module example.test/app\n\ngo 1.26.5\nrequire (\n github.com/wailsapp/wails/v3 v3.0.0-beta.8\n)\n");
    put(root, ".node-version", "24.17.0\n");
    put(root, "frontend/package.json", JSON.stringify({ scripts: { test: "vitest run" } }));
    put(root, "docs/development.md", `intro\n${referenceStart}\n${referenceEnd}\noutro\n`);
    expect(() => checkReference(root)).toThrow("stale");
    checkReference(root, true);
    expect(() => checkReference(root)).not.toThrow();
    const content = readFileSync(path.join(root, "docs/development.md"), "utf8");
    expect(content).toBe(`intro\n${developmentReference(root)}\noutro\n`);
    put(root, ".node-version", "24.18.0\n");
    expect(() => checkReference(root)).toThrow("stale");
    checkReference(root, true);
    put(root, "frontend/package.json", JSON.stringify({ scripts: { test: "vitest run --changed" } }));
    expect(() => checkReference(root)).toThrow("stale");
  });

  it("refuses to rewrite a reference document containing invalid UTF-8", () => {
    const root = fixture();
    put(root, "go.mod", "module example.test/app\n\ngo 1.26.6\nrequire (\n github.com/wailsapp/wails/v3 v3.0.0-beta.8\n)\n");
    put(root, ".node-version", "24.17.0\n");
    put(root, "frontend/package.json", JSON.stringify({ scripts: { test: "vitest run" } }));
    const bytes = Buffer.concat([
      Buffer.from("Human notes "), Buffer.from([255]),
      Buffer.from(`\n${referenceStart}\n${referenceEnd}\n`),
    ]);
    put(root, "docs/development.md", bytes);
    expect(() => checkReference(root, true)).toThrow("non-UTF-8");
    expect(readFileSync(path.join(root, "docs/development.md"))).toEqual(bytes);
  });
});

describe("bounded repository maintenance", () => {
  it("includes tracked and non-ignored untracked files, without duplicates", () => {
    const root = fixture();
    run("git", ["init", "--quiet"], root);
    put(root, ".gitignore", "ignored.go\n");
    put(root, "tracked.go", "package example\n");
    put(root, "new.go", "package example\n");
    put(root, "ignored.go", "package example\n");
    run("git", ["add", ".gitignore", "tracked.go"], root);
    expect(repositoryFiles(root).sort()).toEqual([".gitignore", "new.go", "tracked.go"]);
  });

  it("rejects escaping paths and handles tracked deletions", () => {
    const root = fixture();
    expect(() => ownedFile(root, "../outside.go")).toThrow("escapes");
    expect(ownedFile(root, "deleted.go")).toBe(false);
  });

  it("refuses source files reached through an out-of-repository directory link", () => {
    const root = fixture();
    const external = fixture();
    put(external, "external.go", "package example\n");
    symlinkSync(external, path.join(root, "linked"), process.platform === "win32" ? "junction" : "dir");
    expect(() => ownedFile(root, "linked/external.go")).toThrow("Refusing linked");
    expect(() => formatGo(root, ["linked/external.go"], true)).toThrow("Refusing linked");
    expect(readFileSync(path.join(external, "external.go"), "utf8")).toBe("package example\n");
  });

  it("surfaces subprocess failure and timeout rather than returning success", () => {
    const root = fixture();
    expect(() => run(process.execPath, ["-e", "console.error('failure detail'); process.exit(7)"], root))
      .toThrow("failure detail");
    expect(() => run(process.execPath, ["-e", "setTimeout(() => {}, 10000)"], root, { timeout: 100 }))
      .toThrow();
  });

  it("accepts only explicitly allowed nonzero exits for diff commands", () => {
    const root = fixture();
    expect(run(process.execPath, ["-e", "console.log('diff'); process.exit(1)"], root, { allowedExitCodes: [0, 1] })).toBe("diff\n");
    expect(() => run(process.execPath, ["-e", "process.exit(2)"], root, { allowedExitCodes: [0, 1] })).toThrow("exit 2");
    expect(() => run("missing-synchub-test-tool", [], root, { allowedExitCodes: [0, 1] })).toThrow();
  });

  it("checks and repairs Go formatting without EOL churn or changing unrelated files", () => {
    const root = fixture();
    const good = "package example\r\n\r\nvar value = 1\r\n";
    put(root, "good.go", good);
    put(root, "bad.go", "package example\nvar  value=1\n");
    put(root, "notes.txt", "leave me alone");
    expect(() => formatGo(root, ["good.go"])).not.toThrow();
    expect(() => formatGo(root, ["bad.go"])).toThrow("formatting differs");
    formatGo(root, ["good.go", "bad.go", "notes.txt"], true);
    expect(readFileSync(path.join(root, "good.go"), "utf8")).toBe(good);
    expect(readFileSync(path.join(root, "bad.go"), "utf8")).toBe("package example\n\nvar value = 1\n");
    expect(readFileSync(path.join(root, "notes.txt"), "utf8")).toBe("leave me alone");
    expect(() => formatGo(root, ["good.go", "bad.go", "notes.txt"])).not.toThrow();
  });

  it("refuses malformed UTF-8 during manual formatting without changing the bytes", () => {
    const root = fixture();
    const bytes = Buffer.concat([
      Buffer.from("package example\n// "), Buffer.from([255]), Buffer.from("\nvar  value=1\n"),
    ]);
    put(root, "invalid.go", bytes);
    expect(() => formatGo(root, ["invalid.go"], true)).toThrow("non-UTF-8");
    expect(readFileSync(path.join(root, "invalid.go"))).toEqual(bytes);
  });

  it.each(["syntax", "encoding"])("does not partially format files when a later input has invalid %s", kind => {
    const root = fixture();
    const original = "package example\nvar  value=1\n";
    put(root, "first.go", original);
    const invalid = kind === "syntax" ? Buffer.from("this is not Go\n") : Buffer.from([255]);
    put(root, "last.go", invalid);
    expect(() => formatGo(root, ["first.go", "last.go"], true)).toThrow();
    expect(readFileSync(path.join(root, "first.go"), "utf8")).toBe(original);
    expect(readFileSync(path.join(root, "last.go"))).toEqual(invalid);
  });

  it("handles multiple batches and original paths with spaces without touching unrelated files", () => {
    const root = fixture();
    const files = Array.from({ length: 130 }, (_, index) => `nested dir/file-${index}.go`);
    for (const [index, file] of files.entries()) {
      put(root, file, `package example\n\nvar Value${index} = ${index}\n`);
    }
    const last = files[files.length - 1];
    const original = "package example\r\nvar  Value129=129\r\n";
    put(root, last, original);
    put(root, "notes.txt", "unchanged");
    expect(() => formatGo(root, files)).toThrow(last);
    expect(readFileSync(path.join(root, last), "utf8")).toBe(original);
    formatGo(root, files, true);
    expect(readFileSync(path.join(root, last), "utf8")).toBe("package example\r\n\r\nvar Value129 = 129\r\n");
    expect(readFileSync(path.join(root, "notes.txt"), "utf8")).toBe("unchanged");
    expect(() => formatGo(root, files)).not.toThrow();
  }, 60_000);

  it("reports syntax errors against the original file instead of accepting a source fragment", () => {
    const root = fixture();
    put(root, "nested dir/fragment.go", "var value = 1\n");
    expect(() => formatGo(root, ["nested dir/fragment.go"])).toThrow("nested dir/fragment.go");
    expect(readFileSync(path.join(root, "nested dir/fragment.go"), "utf8")).toBe("var value = 1\n");
  });
});

it("renders the existing icon inputs with the patched sharp dependency without writing artifacts", async () => {
  const root = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
  const inputs = ["synchub", ...["ready", "updating", "done", "error", "paused"].map(state => `tray-${state}`)];
  for (const name of inputs) {
    const sizes = name === "synchub" ? [1024] : [16, 20, 24, 32];
    for (const size of sizes) {
      const { info } = await sharp(path.join(root, "assets", "icons", `${name}.svg`), { density: 384 })
        .resize(size, size).png({ compressionLevel: 9 }).toBuffer({ resolveWithObject: true });
      expect(info.format).toBe("png");
      expect(info.width).toBe(size);
      expect(info.height).toBe(size);
    }
  }
});
