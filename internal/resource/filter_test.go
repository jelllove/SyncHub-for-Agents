package resource

import "testing"

func TestDefaultFilterPolicyBlocksGeneratedAndOversizedFiles(t *testing.T) {
	policy := DefaultFilterPolicy()
	cases := map[string]bool{
		"skill/SKILL.md":                      true,
		"skill/dist/index.js":                 true,
		"skill/build/generated.go":            true,
		"skill/node_modules/pkg/index.js":     false,
		"skill/scripts/.vscode-test/code.exe": false,
		"skill/.venv/pyvenv.cfg":              false,
		"skill/venv/bin/python":               false,
		"skill/__pycache__/module.pyc":        false,
		"skill/.git/config":                   false,
		"skill/.svn/entries":                  false,
		"skill/coverage/coverage.json":        false,
		"skill/.cache/index":                  false,
		"skill/state.db":                      false,
		"skill/state.db-wal":                  false,
		"skill/state.sqlite":                  false,
		"skill/state.sqlite-shm":              false,
		"skill/logs/current.log":              false,
		"skill/package.lock":                  false,
		"skill/output.tmp":                    false,
	}
	for rel, want := range cases {
		if got := policy.Allows(rel, 1024); got != want {
			t.Errorf("Allows(%q) = %v, want %v", rel, got, want)
		}
	}

	if !policy.Allows("skill/archive.bin", (49<<20)+1024) {
		t.Fatal("49 MiB source asset must be allowed")
	}
	if policy.Allows("skill/archive.bin", DefaultMaxFileSize) {
		t.Fatal("50 MiB artifact must be blocked")
	}
	if allowed, reason := policy.Check("skill/archive.bin", DefaultMaxFileSize, StrategySourceTree); allowed || reason != "file-too-large" {
		t.Fatalf("Check oversized = (%v, %q), want (false, file-too-large)", allowed, reason)
	}
}

func TestFilterPolicyBlocksExecutablesOnlyForSourceTrees(t *testing.T) {
	policy := DefaultFilterPolicy()
	for _, rel := range []string{
		"bin/tool.exe",
		"bin/library.dll",
		"lib/library.so",
		"lib/library.dylib",
		"node/addon.node",
	} {
		if allowed, reason := policy.Check(rel, 1024, StrategySourceTree); allowed || reason != "platform-binary" {
			t.Errorf("source Check(%q) = (%v, %q)", rel, allowed, reason)
		}
		if allowed, reason := policy.Check(rel, 1024, StrategyFileTree); !allowed || reason != "" {
			t.Errorf("file-tree Check(%q) = (%v, %q)", rel, allowed, reason)
		}
	}
}

func TestFilterPolicyFailsClosedOnInvalidPaths(t *testing.T) {
	policy := DefaultFilterPolicy()
	if !policy.Allows(`nested\file.txt`, 1) {
		t.Fatal("relative backslashes must be normalized before filtering")
	}
	for _, rel := range []string{
		"",
		"/etc/passwd",
		`C:\Windows\system.ini`,
		"../secret.txt",
		"./SKILL.md",
		"dir/../SKILL.md",
		"dir//SKILL.md",
	} {
		if allowed, reason := policy.Check(rel, 1, StrategySourceTree); allowed || reason != "invalid-path" {
			t.Errorf("Check(%q) = (%v, %q), want (false, invalid-path)", rel, allowed, reason)
		}
	}
}
