# AgentConfigSync Core Sync (MVP) Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the core one-shot sync engine of `acsync` — a Go single binary that mirrors selected AI-agent config/session files between machines through a private GitHub repo, with secret filtering, three-way delete propagation, soft-delete, and `init`/`sync`/`status` CLI commands.

**Architecture:** A local "sync workspace" (a clone of the private GitHub repo at `~/.acsync/repo`) bridges the agent directories and the remote. One `sync` pass does: `git pull --rebase` → three-way reconcile against a saved snapshot (`state.json`) → apply remote changes to agent dirs → collect local changes (secret-filtered) → soft-delete removed files into `.trash/` → commit → push (retry via pull-rebase on non-fast-forward).

**Tech Stack:** Go 1.22+, standard library, `gopkg.in/yaml.v3` for provider/config YAML, `github.com/spf13/cobra` for CLI, `git` invoked via `os/exec`. Tests use Go's `testing` package with local bare git repos as a fake remote.

---

## File Structure

This plan (Plan 1 of 3) builds only the core sync MVP. Scheduler/daemon/autostart (Plan 2) and tray/settings UI (Plan 3) come later and are out of scope here.

```
AgentConfigSync/
  go.mod
  go.sum
  cmd/
    acsync/
      main.go                 # cobra root command wiring
  internal/
    pathresolver/
      pathresolver.go         # cross-platform ~ / %USERPROFILE% expansion
      pathresolver_test.go
    secret/
      secret.go               # Scanner: exclude globs + secret key patterns
      secret_test.go
    provider/
      provider.go             # Provider types + Load(YAML) + Builtins(embed)
      provider_test.go
      builtin/                # embedded YAML definitions
        claude.yaml
        copilot.yaml
        gemini.yaml
        cursor.yaml
    state/
      state.go                # Snapshot, FileMeta, HashFile, Load/Save
      state_test.go
    gitclient/
      gitclient.go            # Client wrapping git via os/exec
      gitclient_test.go
    config/
      config.go               # Config load/save (~/.acsync/config.yaml)
      config_test.go
    syncengine/
      collect.go              # walk agent dirs → Snapshot (secret-filtered)
      collect_test.go
      reconcile.go            # three-way diff → []Action (LWW, delete propagation)
      reconcile_test.go
      apply.go                # apply actions to agent dirs / repo / .trash
      apply_test.go
      engine.go               # SyncOnce: orchestrates a full sync pass
      engine_test.go
    cli/
      runtime.go              # Home/paths, LoadProviders, BuildSpecs, Cloner
      init.go                 # acsync init
      sync.go                 # acsync sync
      status.go               # acsync status
```

**Responsibilities & boundaries:**
- `pathresolver` — pure functions, no I/O beyond reading env; deterministic per (goos, home).
- `secret` — pure predicate logic over relative paths + byte content; no I/O.
- `provider` — parse declarative YAML; expose builtin defs via `embed`.
- `state` — snapshot data model + file hashing + JSON persistence.
- `gitclient` — the *only* place that shells out to `git`.
- `config` — user settings persistence.
- `syncengine` — orchestration; depends on all of the above; the only package that knows the full flow.
- `cli` — thin command handlers that wire config + engine.

---

## Task 0: Environment setup and Go module scaffold

**Files:**
- Create: `go.mod`
- Create: `cmd/acsync/main.go`
- Create: `.gitignore`

- [ ] **Step 1: Install Go (if not present)**

Check whether Go is installed:

Run: `go version`
Expected: `go version go1.22.x ...`. If the command is not found, install via winget:

Run: `winget install --id GoLang.Go -e --accept-source-agreements --accept-package-agreements`

Then open a new shell and re-run `go version` to confirm it is on PATH.

- [ ] **Step 2: Initialize the Go module**

Run: `go mod init github.com/qinqingxu/acsync`
Expected: creates `go.mod` containing `module github.com/qinqingxu/acsync` and a `go 1.22` line.

- [ ] **Step 3: Add a `.gitignore`**

Create `.gitignore`:

```gitignore
/acsync
/acsync.exe
/dist/
*.test
*.out
```

- [ ] **Step 4: Create a minimal `main.go` that builds**

Create `cmd/acsync/main.go`:

```go
package main

import "fmt"

func main() {
	fmt.Println("acsync")
}
```

- [ ] **Step 5: Verify it builds and runs**

Run: `go build ./... && go run ./cmd/acsync`
Expected: prints `acsync` with no build errors.

- [ ] **Step 6: Commit**

```bash
git add go.mod .gitignore cmd/acsync/main.go
git commit -m "chore: scaffold Go module and acsync entrypoint"
```

---

## Task 1: pathresolver — cross-platform path expansion

**Files:**
- Create: `internal/pathresolver/pathresolver.go`
- Test: `internal/pathresolver/pathresolver_test.go`

Provider YAML declares OS-specific paths like `%USERPROFILE%\.claude` (Windows) or `~/.claude` (unix). This package expands those to absolute paths. `ResolveFor` is a pure function taking `goos` and `home` explicitly so it is testable on any OS; `Resolve` calls it with the real runtime values.

- [ ] **Step 1: Write the failing test**

Create `internal/pathresolver/pathresolver_test.go`:

```go
package pathresolver

import (
	"path/filepath"
	"testing"
)

func TestResolveForUnixTilde(t *testing.T) {
	got, err := ResolveFor("~/.claude", "linux", "/home/alice")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := filepath.FromSlash("/home/alice/.claude")
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestResolveForWindowsUserProfile(t *testing.T) {
	got, err := ResolveFor("%USERPROFILE%\\.claude", "windows", `C:\Users\alice`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := filepath.FromSlash(`C:\Users\alice\.claude`)
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestResolveForTildeOnWindows(t *testing.T) {
	got, err := ResolveFor("~/.gemini", "windows", `C:\Users\alice`)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := filepath.FromSlash(`C:\Users\alice\.gemini`)
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestResolveForEmptyHome(t *testing.T) {
	if _, err := ResolveFor("~/.claude", "linux", ""); err == nil {
		t.Fatal("expected error when home is empty, got nil")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/pathresolver/ -run TestResolveFor -v`
Expected: FAIL — `undefined: ResolveFor`.

- [ ] **Step 3: Write minimal implementation**

Create `internal/pathresolver/pathresolver.go`:

```go
// Package pathresolver expands OS-specific path templates from provider
// definitions into concrete absolute paths.
package pathresolver

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Resolve expands raw using the current runtime OS and user home directory.
func Resolve(raw string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return ResolveFor(raw, runtime.GOOS, home)
}

// ResolveFor expands raw for the given goos and home directory. It handles a
// leading "~", "%USERPROFILE%", and "$HOME", then normalizes separators for
// the target OS.
func ResolveFor(raw, goos, home string) (string, error) {
	if home == "" {
		return "", fmt.Errorf("pathresolver: empty home directory")
	}

	p := raw
	switch {
	case strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`):
		p = home + string(os.PathSeparator) + p[2:]
	case p == "~":
		p = home
	}

	p = strings.ReplaceAll(p, "%USERPROFILE%", home)
	p = strings.ReplaceAll(p, "$HOME", home)

	// Normalize slashes to the target OS separator.
	if goos == "windows" {
		p = strings.ReplaceAll(p, "/", `\`)
	} else {
		p = strings.ReplaceAll(p, `\`, "/")
	}

	return filepath.Clean(p), nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/pathresolver/ -v`
Expected: PASS (all four tests).

- [ ] **Step 5: Commit**

```bash
git add internal/pathresolver/
git commit -m "feat: add cross-platform pathresolver"
```

---

## Task 2: secret — exclude globs and secret-content detection

**Files:**
- Create: `internal/secret/secret.go`
- Test: `internal/secret/secret_test.go`

A `Scanner` decides whether a file should be kept out of the repo. Two mechanisms: (1) the relative path matches an exclude glob, or (2) the file is JSON whose keys match secret patterns like `apiKey`/`token`. `ShouldBlock` combines both.

- [ ] **Step 1: Write the failing test**

Create `internal/secret/secret_test.go`:

```go
package secret

import "testing"

func newTestScanner() *Scanner {
	return NewScanner(
		[]string{"**/.credentials.json", "**/*token*", "**/shell-snapshots/**"},
		[]string{"apiKey", "token", "secret", "password", "oauth", "refresh_token"},
	)
}

func TestIsExcludedByGlob(t *testing.T) {
	s := newTestScanner()
	cases := map[string]bool{
		"projects/a/.credentials.json":     true,
		"config/access_token.txt":          true,
		"shell-snapshots/snap-1.sh":        true,
		"config/settings.json":             false,
		"sessions/abc/uuid.jsonl":          false,
	}
	for rel, want := range cases {
		if got := s.IsExcluded(rel); got != want {
			t.Errorf("IsExcluded(%q) = %v, want %v", rel, got, want)
		}
	}
}

func TestHasSecretContent(t *testing.T) {
	s := newTestScanner()
	secretJSON := []byte(`{"model":"x","apiKey":"sk-123"}`)
	if !s.HasSecretContent(secretJSON) {
		t.Error("expected secret content to be detected")
	}
	cleanJSON := []byte(`{"model":"x","theme":"dark"}`)
	if s.HasSecretContent(cleanJSON) {
		t.Error("clean JSON should not be flagged")
	}
	notJSON := []byte("apiKey is a word in prose")
	if s.HasSecretContent(notJSON) {
		t.Error("non-JSON content must not be treated as secret JSON")
	}
}

func TestShouldBlock(t *testing.T) {
	s := newTestScanner()
	if !s.ShouldBlock("projects/.credentials.json", []byte("{}")) {
		t.Error("excluded path must be blocked regardless of content")
	}
	if !s.ShouldBlock("config/settings.json", []byte(`{"token":"abc"}`)) {
		t.Error("secret content must be blocked")
	}
	if s.ShouldBlock("config/settings.json", []byte(`{"theme":"dark"}`)) {
		t.Error("clean file must not be blocked")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/secret/ -v`
Expected: FAIL — `undefined: NewScanner`.

- [ ] **Step 3: Write minimal implementation**

Create `internal/secret/secret.go`:

```go
// Package secret decides which files must be kept out of the sync repo,
// either by path (exclude globs) or by detecting secret-looking JSON keys.
package secret

import (
	"encoding/json"
	"path"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

// Scanner evaluates files against exclude globs and secret key patterns.
type Scanner struct {
	excludeGlobs []string
	keyPatterns  []string
}

// NewScanner builds a Scanner. keyPatterns are matched case-insensitively as
// substrings of JSON object keys.
func NewScanner(excludeGlobs, keyPatterns []string) *Scanner {
	lowered := make([]string, len(keyPatterns))
	for i, p := range keyPatterns {
		lowered[i] = strings.ToLower(p)
	}
	return &Scanner{excludeGlobs: excludeGlobs, keyPatterns: lowered}
}

// IsExcluded reports whether the forward-slash relative path matches any
// exclude glob.
func (s *Scanner) IsExcluded(rel string) bool {
	rel = path.Clean(strings.ReplaceAll(rel, "\\", "/"))
	for _, g := range s.excludeGlobs {
		if ok, _ := doublestar.Match(g, rel); ok {
			return true
		}
	}
	return false
}

// HasSecretContent reports whether data parses as JSON and contains a key
// matching any secret pattern (searched recursively).
func (s *Scanner) HasSecretContent(data []byte) bool {
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return false
	}
	return s.walk(v)
}

func (s *Scanner) walk(v any) bool {
	switch t := v.(type) {
	case map[string]any:
		for k, child := range t {
			if s.keyMatches(k) {
				return true
			}
			if s.walk(child) {
				return true
			}
		}
	case []any:
		for _, child := range t {
			if s.walk(child) {
				return true
			}
		}
	}
	return false
}

func (s *Scanner) keyMatches(key string) bool {
	lk := strings.ToLower(key)
	for _, p := range s.keyPatterns {
		if strings.Contains(lk, p) {
			return true
		}
	}
	return false
}

// ShouldBlock returns true if the file must not be uploaded.
func (s *Scanner) ShouldBlock(rel string, data []byte) bool {
	if s.IsExcluded(rel) {
		return true
	}
	return s.HasSecretContent(data)
}
```

- [ ] **Step 4: Add the doublestar dependency**

Run: `go get github.com/bmatcuk/doublestar/v4@v4.6.1`
Expected: adds the module to `go.mod`/`go.sum`.

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/secret/ -v`
Expected: PASS (all three tests).

- [ ] **Step 6: Commit**

```bash
git add internal/secret/ go.mod go.sum
git commit -m "feat: add secret scanner for path and content filtering"
```

---

## Task 3: provider — declarative agent adapters (YAML + embedded builtins)

**Files:**
- Create: `internal/provider/provider.go`
- Create: `internal/provider/builtin/claude.yaml`
- Create: `internal/provider/builtin/copilot.yaml`
- Create: `internal/provider/builtin/gemini.yaml`
- Create: `internal/provider/builtin/cursor.yaml`
- Test: `internal/provider/provider_test.go`

Each agent is described by a declarative YAML. `Load` parses one file; `Builtins` returns the embedded set. Users can later drop extra YAML into `~/.acsync/providers/` (loaded via `Load`), so no code change is needed to add agents.

- [ ] **Step 1: Create the builtin YAML definitions**

Create `internal/provider/builtin/claude.yaml`:

```yaml
name: claude
config:
  paths:
    windows: "%USERPROFILE%\\.claude"
    darwin: "~/.claude"
    linux: "~/.claude"
  include:
    - "settings.json"
    - "CLAUDE.md"
  sessions:
    - "projects/**/*.jsonl"
  exclude:
    - "**/.credentials.json"
    - "**/*token*"
    - "**/shell-snapshots/**"
secrets:
  key_patterns: ["apiKey", "token", "secret", "password", "oauth", "refresh_token"]
```

Create `internal/provider/builtin/copilot.yaml`:

```yaml
name: copilot
config:
  paths:
    windows: "%USERPROFILE%\\.copilot"
    darwin: "~/.copilot"
    linux: "~/.copilot"
  include:
    - "config.json"
  sessions:
    - "history/**/*.json"
  exclude:
    - "**/*token*"
    - "**/hosts.json"
secrets:
  key_patterns: ["apiKey", "token", "secret", "password", "oauth", "refresh_token"]
```

Create `internal/provider/builtin/gemini.yaml`:

```yaml
name: gemini
config:
  paths:
    windows: "%USERPROFILE%\\.gemini"
    darwin: "~/.gemini"
    linux: "~/.gemini"
  include:
    - "settings.json"
  sessions:
    - "sessions/**/*.json"
  exclude:
    - "**/*token*"
    - "**/oauth_creds.json"
secrets:
  key_patterns: ["apiKey", "token", "secret", "password", "oauth", "refresh_token"]
```

Create `internal/provider/builtin/cursor.yaml`:

```yaml
name: cursor
config:
  paths:
    windows: "%USERPROFILE%\\.cursor"
    darwin: "~/.cursor"
    linux: "~/.cursor"
  include:
    - "settings.json"
  sessions:
    - "chats/**/*.json"
  exclude:
    - "**/*token*"
secrets:
  key_patterns: ["apiKey", "token", "secret", "password", "oauth", "refresh_token"]
```

- [ ] **Step 2: Write the failing test**

Create `internal/provider/provider_test.go`:

```go
package provider

import "testing"

func TestBuiltinsLoaded(t *testing.T) {
	ps, err := Builtins()
	if err != nil {
		t.Fatalf("Builtins() error: %v", err)
	}
	byName := map[string]Provider{}
	for _, p := range ps {
		byName[p.Name] = p
	}
	for _, want := range []string{"claude", "copilot", "gemini", "cursor"} {
		if _, ok := byName[want]; !ok {
			t.Errorf("missing builtin provider %q", want)
		}
	}
	claude := byName["claude"]
	if claude.Config.Paths["linux"] != "~/.claude" {
		t.Errorf("claude linux path = %q", claude.Config.Paths["linux"])
	}
	if len(claude.Config.Sessions) == 0 {
		t.Error("claude should declare session globs")
	}
	if len(claude.Secrets.KeyPatterns) == 0 {
		t.Error("claude should declare secret key patterns")
	}
}

func TestLoadFromBytes(t *testing.T) {
	data := []byte(`
name: demo
config:
  paths:
    linux: "~/.demo"
  include: ["settings.json"]
  sessions: ["s/**/*.json"]
  exclude: ["**/*token*"]
secrets:
  key_patterns: ["token"]
`)
	p, err := Parse(data)
	if err != nil {
		t.Fatalf("Parse() error: %v", err)
	}
	if p.Name != "demo" {
		t.Errorf("name = %q, want demo", p.Name)
	}
	if p.Config.Include[0] != "settings.json" {
		t.Errorf("include = %v", p.Config.Include)
	}
}

func TestParseRejectsMissingName(t *testing.T) {
	if _, err := Parse([]byte(`config: {}`)); err == nil {
		t.Fatal("expected error for provider without name")
	}
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `go test ./internal/provider/ -v`
Expected: FAIL — `undefined: Builtins` / `undefined: Parse`.

- [ ] **Step 4: Write minimal implementation**

Create `internal/provider/provider.go`:

```go
// Package provider models declarative agent adapters and loads builtin and
// user-supplied definitions.
package provider

import (
	"embed"
	"fmt"
	"os"
	"path"

	"gopkg.in/yaml.v3"
)

//go:embed builtin/*.yaml
var builtinFS embed.FS

// ConfigSpec describes which files of an agent are synced.
type ConfigSpec struct {
	Paths    map[string]string `yaml:"paths"`
	Include  []string          `yaml:"include"`
	Sessions []string          `yaml:"sessions"`
	Exclude  []string          `yaml:"exclude"`
}

// SecretSpec lists JSON key substrings that mark a file as secret.
type SecretSpec struct {
	KeyPatterns []string `yaml:"key_patterns"`
}

// Provider is a declarative adapter for one agent.
type Provider struct {
	Name    string     `yaml:"name"`
	Config  ConfigSpec `yaml:"config"`
	Secrets SecretSpec `yaml:"secrets"`
}

// Parse decodes a single provider definition from YAML bytes.
func Parse(data []byte) (Provider, error) {
	var p Provider
	if err := yaml.Unmarshal(data, &p); err != nil {
		return Provider{}, fmt.Errorf("provider: parse: %w", err)
	}
	if p.Name == "" {
		return Provider{}, fmt.Errorf("provider: missing name")
	}
	return p, nil
}

// Load reads and parses a provider definition from a file path.
func LoadFile(filename string) (Provider, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return Provider{}, err
	}
	return Parse(data)
}

// Builtins returns all embedded provider definitions.
func Builtins() ([]Provider, error) {
	entries, err := builtinFS.ReadDir("builtin")
	if err != nil {
		return nil, err
	}
	var out []Provider
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		data, err := builtinFS.ReadFile(path.Join("builtin", e.Name()))
		if err != nil {
			return nil, err
		}
		p, err := Parse(data)
		if err != nil {
			return nil, fmt.Errorf("builtin %s: %w", e.Name(), err)
		}
		out = append(out, p)
	}
	return out, nil
}
```

- [ ] **Step 5: Add the yaml dependency**

Run: `go get gopkg.in/yaml.v3@v3.0.1`
Expected: adds the module to `go.mod`/`go.sum`.

- [ ] **Step 6: Run test to verify it passes**

Run: `go test ./internal/provider/ -v`
Expected: PASS (all three tests).

- [ ] **Step 7: Commit**

```bash
git add internal/provider/ go.mod go.sum
git commit -m "feat: add provider model with embedded builtin agents"
```

---

## Task 4: state — snapshot model, file hashing, persistence

**Files:**
- Create: `internal/state/state.go`
- Test: `internal/state/state_test.go`

`Snapshot` maps a repo-relative path (forward slashes) to `FileMeta{Hash, ModTime, Size}`. This is the "base" recorded after each successful sync and is the third input to three-way reconciliation. `HashFile` produces a stable SHA-256 hex digest.

- [ ] **Step 1: Write the failing test**

Create `internal/state/state_test.go`:

```go
package state

import (
	"os"
	"path/filepath"
	"testing"
)

func TestHashFileStable(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(f, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	h1, err := HashFile(f)
	if err != nil {
		t.Fatalf("HashFile error: %v", err)
	}
	h2, _ := HashFile(f)
	if h1 != h2 {
		t.Errorf("hash not stable: %q vs %q", h1, h2)
	}
	// Known SHA-256 of "hello".
	want := "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
	if h1 != want {
		t.Errorf("HashFile = %q, want %q", h1, want)
	}
}

func TestSaveAndLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")

	snap := Snapshot{
		"agents/claude/config/settings.json": {Hash: "abc", ModTime: 100, Size: 12},
	}
	if err := Save(path, snap); err != nil {
		t.Fatalf("Save error: %v", err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatalf("Load error: %v", err)
	}
	fm, ok := got["agents/claude/config/settings.json"]
	if !ok {
		t.Fatal("missing entry after round trip")
	}
	if fm.Hash != "abc" || fm.ModTime != 100 || fm.Size != 12 {
		t.Errorf("round trip mismatch: %+v", fm)
	}
}

func TestLoadMissingReturnsEmpty(t *testing.T) {
	got, err := Load(filepath.Join(t.TempDir(), "nope.json"))
	if err != nil {
		t.Fatalf("Load of missing file should not error, got %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty snapshot, got %d entries", len(got))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/state/ -v`
Expected: FAIL — `undefined: HashFile` / `undefined: Snapshot`.

- [ ] **Step 3: Write minimal implementation**

Create `internal/state/state.go`:

```go
// Package state persists the snapshot of files recorded after the last
// successful sync, used as the base for three-way reconciliation.
package state

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
)

// FileMeta records identity of a file at snapshot time.
type FileMeta struct {
	Hash    string `json:"hash"`
	ModTime int64  `json:"mod_time"`
	Size    int64  `json:"size"`
}

// Snapshot maps repo-relative (forward-slash) paths to metadata.
type Snapshot map[string]FileMeta

// HashFile returns the SHA-256 hex digest of the file at path.
func HashFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// Load reads a snapshot from path. A missing file yields an empty snapshot.
func Load(path string) (Snapshot, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return Snapshot{}, nil
	}
	if err != nil {
		return nil, err
	}
	snap := Snapshot{}
	if len(data) == 0 {
		return snap, nil
	}
	if err := json.Unmarshal(data, &snap); err != nil {
		return nil, err
	}
	return snap, nil
}

// Save writes the snapshot to path atomically (temp file + rename).
func Save(path string, snap Snapshot) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(snap, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/state/ -v`
Expected: PASS (all three tests).

- [ ] **Step 5: Commit**

```bash
git add internal/state/
git commit -m "feat: add state snapshot with hashing and persistence"
```

---

## Task 5: gitclient — git wrapper via os/exec

**Files:**
- Create: `internal/gitclient/gitclient.go`
- Test: `internal/gitclient/gitclient_test.go`

The only package that shells out to `git`. Tests use a local bare repo as the "remote", so no network or GitHub credentials are needed.

- [ ] **Step 1: Write the failing test**

Create `internal/gitclient/gitclient_test.go`:

```go
package gitclient

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func mustGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}

// setupBareRemote creates a bare repo with one initial commit and returns its path.
func setupBareRemote(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	bare := filepath.Join(root, "remote.git")
	mustGit(t, root, "init", "--bare", "-b", "main", bare)

	// Seed the bare repo via a temporary working clone.
	seed := filepath.Join(root, "seed")
	mustGit(t, root, "clone", bare, seed)
	mustGit(t, seed, "config", "user.email", "t@example.com")
	mustGit(t, seed, "config", "user.name", "tester")
	if err := os.WriteFile(filepath.Join(seed, "README.md"), []byte("seed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGit(t, seed, "add", ".")
	mustGit(t, seed, "commit", "-m", "seed")
	mustGit(t, seed, "push", "origin", "main")
	return bare
}

func TestCloneAndHasChanges(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	bare := setupBareRemote(t)
	work := filepath.Join(t.TempDir(), "work")

	c := &Client{Dir: work}
	if err := c.Clone(bare, work); err != nil {
		t.Fatalf("Clone error: %v", err)
	}
	mustGit(t, work, "config", "user.email", "t@example.com")
	mustGit(t, work, "config", "user.name", "tester")

	changed, err := c.HasChanges()
	if err != nil {
		t.Fatalf("HasChanges error: %v", err)
	}
	if changed {
		t.Error("fresh clone should have no changes")
	}

	if err := os.WriteFile(filepath.Join(work, "new.txt"), []byte("hi\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	changed, err = c.HasChanges()
	if err != nil {
		t.Fatalf("HasChanges error: %v", err)
	}
	if !changed {
		t.Error("expected changes after writing a new file")
	}
}

func TestCommitPushPullRoundTrip(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	bare := setupBareRemote(t)

	// Machine A clones, commits, pushes.
	workA := filepath.Join(t.TempDir(), "A")
	a := &Client{Dir: workA}
	if err := a.Clone(bare, workA); err != nil {
		t.Fatal(err)
	}
	mustGit(t, workA, "config", "user.email", "a@example.com")
	mustGit(t, workA, "config", "user.name", "A")
	if err := os.WriteFile(filepath.Join(workA, "fromA.txt"), []byte("A\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := a.AddAll(); err != nil {
		t.Fatalf("AddAll: %v", err)
	}
	if err := a.Commit("add fromA"); err != nil {
		t.Fatalf("Commit: %v", err)
	}
	if err := a.Push(); err != nil {
		t.Fatalf("Push: %v", err)
	}

	// Machine B clones and should see A's file after pull.
	workB := filepath.Join(t.TempDir(), "B")
	b := &Client{Dir: workB}
	if err := b.Clone(bare, workB); err != nil {
		t.Fatal(err)
	}
	mustGit(t, workB, "config", "user.email", "b@example.com")
	mustGit(t, workB, "config", "user.name", "B")
	if err := b.PullRebase(); err != nil {
		t.Fatalf("PullRebase: %v", err)
	}
	if _, err := os.Stat(filepath.Join(workB, "fromA.txt")); err != nil {
		t.Errorf("B should have fromA.txt after pull: %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/gitclient/ -v`
Expected: FAIL — `undefined: Client`.

- [ ] **Step 3: Write minimal implementation**

Create `internal/gitclient/gitclient.go`:

```go
// Package gitclient wraps the git CLI. It is the only package that shells
// out to git.
package gitclient

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// Client operates on a git working directory at Dir.
type Client struct {
	Dir string
}

func (c *Client) run(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	if c.Dir != "" && (len(args) == 0 || args[0] != "clone") {
		cmd.Dir = c.Dir
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return stdout.String(), fmt.Errorf("git %s: %w: %s",
			strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}

// Clone clones url into dir. Dir on the client should match dir.
func (c *Client) Clone(url, dir string) error {
	_, err := c.run("clone", url, dir)
	return err
}

// PullRebase runs git pull --rebase from origin.
func (c *Client) PullRebase() error {
	_, err := c.run("pull", "--rebase")
	return err
}

// AddAll stages all changes including deletions.
func (c *Client) AddAll() error {
	_, err := c.run("add", "-A")
	return err
}

// Commit records staged changes. It is a no-op error if nothing is staged;
// callers should check HasChanges first.
func (c *Client) Commit(message string) error {
	_, err := c.run("commit", "-m", message)
	return err
}

// Push pushes the current branch to origin.
func (c *Client) Push() error {
	_, err := c.run("push")
	return err
}

// HasChanges reports whether the working tree has any staged or unstaged
// changes (including untracked files).
func (c *Client) HasChanges() (bool, error) {
	out, err := c.run("status", "--porcelain")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) != "", nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/gitclient/ -v`
Expected: PASS (both tests; skipped only if git is absent, which it is not here).

- [ ] **Step 5: Commit**

```bash
git add internal/gitclient/
git commit -m "feat: add gitclient wrapper over git CLI"
```

---

## Task 6: config — user settings persistence

**Files:**
- Create: `internal/config/config.go`
- Test: `internal/config/config_test.go`

Holds user settings persisted to `~/.acsync/config.yaml`: remote repo URL, sync interval, trash grace period, and per-agent enable flags. `Default` produces sensible defaults (all agents enabled).

- [ ] **Step 1: Write the failing test**

Create `internal/config/config_test.go`:

```go
package config

import (
	"path/filepath"
	"testing"
)

func TestDefault(t *testing.T) {
	c := Default([]string{"claude", "copilot"})
	if c.SyncIntervalMinutes != 10 {
		t.Errorf("interval = %d, want 10", c.SyncIntervalMinutes)
	}
	if c.TrashGraceDays != 30 {
		t.Errorf("grace = %d, want 30", c.TrashGraceDays)
	}
	if !c.Agents["claude"] || !c.Agents["copilot"] {
		t.Errorf("all agents should default to enabled: %+v", c.Agents)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	in := Config{
		RepoURL:             "git@github.com:me/acsync-data.git",
		SyncIntervalMinutes: 15,
		TrashGraceDays:      7,
		Agents:              map[string]bool{"claude": true, "gemini": false},
	}
	if err := Save(path, in); err != nil {
		t.Fatalf("Save: %v", err)
	}
	out, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if out.RepoURL != in.RepoURL || out.SyncIntervalMinutes != 15 || out.TrashGraceDays != 7 {
		t.Errorf("round trip mismatch: %+v", out)
	}
	if out.Agents["claude"] != true || out.Agents["gemini"] != false {
		t.Errorf("agents mismatch: %+v", out.Agents)
	}
}

func TestLoadMissingIsError(t *testing.T) {
	if _, err := Load(filepath.Join(t.TempDir(), "nope.yaml")); err == nil {
		t.Fatal("expected error loading missing config")
	}
}

func TestEnabledAgents(t *testing.T) {
	c := Config{Agents: map[string]bool{"claude": true, "gemini": false, "copilot": true}}
	got := c.EnabledAgents()
	if len(got) != 2 {
		t.Fatalf("expected 2 enabled, got %v", got)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/config/ -v`
Expected: FAIL — `undefined: Default`.

- [ ] **Step 3: Write minimal implementation**

Create `internal/config/config.go`:

```go
// Package config persists user settings for acsync.
package config

import (
	"os"
	"path/filepath"
	"sort"

	"gopkg.in/yaml.v3"
)

// Config is the user-editable settings model.
type Config struct {
	RepoURL             string          `yaml:"repo_url"`
	SyncIntervalMinutes int             `yaml:"sync_interval_minutes"`
	TrashGraceDays      int             `yaml:"trash_grace_days"`
	Agents              map[string]bool `yaml:"agents"`
}

// Default returns a Config with all provided agent names enabled.
func Default(agentNames []string) Config {
	agents := make(map[string]bool, len(agentNames))
	for _, n := range agentNames {
		agents[n] = true
	}
	return Config{
		SyncIntervalMinutes: 10,
		TrashGraceDays:      30,
		Agents:              agents,
	}
}

// EnabledAgents returns the sorted names of enabled agents.
func (c Config) EnabledAgents() []string {
	var out []string
	for name, on := range c.Agents {
		if on {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

// Load reads a config from path. A missing file is an error (run init first).
func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	var c Config
	if err := yaml.Unmarshal(data, &c); err != nil {
		return Config{}, err
	}
	if c.Agents == nil {
		c.Agents = map[string]bool{}
	}
	return c, nil
}

// Save writes the config to path (creating parent dirs).
func Save(path string, c Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/config/ -v`
Expected: PASS (all four tests).

- [ ] **Step 5: Commit**

```bash
git add internal/config/
git commit -m "feat: add config persistence with per-agent toggles"
```

---

## Task 7: syncengine collect — walk agent dirs into a repo snapshot

**Files:**
- Create: `internal/syncengine/collect.go`
- Test: `internal/syncengine/collect_test.go`

`Collect` walks each enabled agent's directory, matches `include` globs into `agents/<name>/config/...` and `sessions` globs into `agents/<name>/sessions/...`, applies the secret `Scanner`, and returns the desired repo snapshot plus a map of repo-path → local source path (for later copying).

- [ ] **Step 1: Write the failing test**

Create `internal/syncengine/collect_test.go`:

```go
package syncengine

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/qinqingxu/acsync/internal/secret"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCollectMapsAndFilters(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "settings.json"), `{"theme":"dark"}`)
	writeFile(t, filepath.Join(root, "projects", "foo", "abc.jsonl"), `{"m":1}`)
	writeFile(t, filepath.Join(root, ".credentials.json"), `{"token":"x"}`)
	writeFile(t, filepath.Join(root, "secret.json"), `{"apiKey":"sk-1"}`)

	sc := secret.NewScanner(
		[]string{"**/.credentials.json"},
		[]string{"apiKey", "token"},
	)
	spec := AgentSpec{
		Name:     "claude",
		Root:     root,
		Include:  []string{"settings.json", "secret.json"},
		Sessions: []string{"projects/**/*.jsonl"},
		Scanner:  sc,
	}

	c, err := Collect([]AgentSpec{spec})
	if err != nil {
		t.Fatalf("Collect error: %v", err)
	}

	if _, ok := c.Snapshot["agents/claude/config/settings.json"]; !ok {
		t.Error("expected settings.json in config")
	}
	if _, ok := c.Snapshot["agents/claude/sessions/projects/foo/abc.jsonl"]; !ok {
		t.Error("expected session file mapped under sessions/")
	}
	if _, ok := c.Snapshot["agents/claude/config/.credentials.json"]; ok {
		t.Error(".credentials.json should be excluded by glob")
	}
	if _, ok := c.Snapshot["agents/claude/config/secret.json"]; ok {
		t.Error("secret.json should be blocked by content scan")
	}
	if src := c.Sources["agents/claude/config/settings.json"]; src == "" {
		t.Error("expected a source path for settings.json")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/syncengine/ -run TestCollect -v`
Expected: FAIL — `undefined: AgentSpec` / `undefined: Collect`.

- [ ] **Step 3: Write minimal implementation**

Create `internal/syncengine/collect.go`:

```go
// Package syncengine orchestrates a full sync pass: collect, reconcile, apply.
package syncengine

import (
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/qinqingxu/acsync/internal/secret"
	"github.com/qinqingxu/acsync/internal/state"
)

// AgentSpec is a provider resolved to a concrete local directory for the
// current machine.
type AgentSpec struct {
	Name     string
	Root     string
	Include  []string // globs relative to Root -> agents/<name>/config/
	Sessions []string // globs relative to Root -> agents/<name>/sessions/
	Scanner  *secret.Scanner
}

// Collected is the result of walking all agent directories.
type Collected struct {
	Snapshot state.Snapshot    // repo-relative path -> metadata
	Sources  map[string]string // repo-relative path -> local absolute path
	Blocked  []string          // repo-relative paths blocked by the scanner
}

// Collect walks each spec's directory and builds the desired repo snapshot.
func Collect(specs []AgentSpec) (Collected, error) {
	out := Collected{
		Snapshot: state.Snapshot{},
		Sources:  map[string]string{},
	}
	for _, spec := range specs {
		if err := collectOne(spec, &out); err != nil {
			return Collected{}, err
		}
	}
	return out, nil
}

func collectOne(spec AgentSpec, out *Collected) error {
	info, err := os.Stat(spec.Root)
	if err != nil || !info.IsDir() {
		return nil // missing agent dir: nothing to collect
	}

	return filepath.WalkDir(spec.Root, func(abs string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		relOS, err := filepath.Rel(spec.Root, abs)
		if err != nil {
			return err
		}
		rel := filepath.ToSlash(relOS)

		sub, ok := classify(rel, spec.Include, spec.Sessions)
		if !ok {
			return nil
		}
		repoRel := path.Join("agents", spec.Name, sub, rel)

		data, err := os.ReadFile(abs)
		if err != nil {
			return err
		}
		if spec.Scanner != nil && spec.Scanner.ShouldBlock(rel, data) {
			out.Blocked = append(out.Blocked, repoRel)
			return nil
		}

		hash, err := state.HashFile(abs)
		if err != nil {
			return err
		}
		fi, err := d.Info()
		if err != nil {
			return err
		}
		out.Snapshot[repoRel] = state.FileMeta{
			Hash:    hash,
			ModTime: fi.ModTime().Unix(),
			Size:    fi.Size(),
		}
		out.Sources[repoRel] = abs
		return nil
	})
}

// classify returns the repo subdirectory ("config" or "sessions") for a file,
// or ok=false if it matches no glob. Session globs take precedence.
func classify(rel string, include, sessions []string) (string, bool) {
	for _, g := range sessions {
		if matchGlob(g, rel) {
			return "sessions", true
		}
	}
	for _, g := range include {
		if matchGlob(g, rel) {
			return "config", true
		}
	}
	return "", false
}

func matchGlob(glob, rel string) bool {
	glob = strings.ReplaceAll(glob, "\\", "/")
	ok, _ := doublestar.Match(glob, rel)
	return ok
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/syncengine/ -run TestCollect -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/syncengine/collect.go internal/syncengine/collect_test.go
git commit -m "feat: add syncengine collect for agent directories"
```

---

## Task 8: syncengine reconcile — three-way diff with LWW and delete propagation

**Files:**
- Create: `internal/syncengine/reconcile.go`
- Test: `internal/syncengine/reconcile_test.go`

`Reconcile(base, local, remote)` compares each path across the three snapshots and emits `Action`s. New/modified files propagate in the direction they changed; deletions propagate (delete/modify conflicts resurrect the surviving copy to avoid data loss); double edits resolve by last-writer-wins on `ModTime`.

- [ ] **Step 1: Write the failing test**

Create `internal/syncengine/reconcile_test.go`:

```go
package syncengine

import (
	"testing"

	"github.com/qinqingxu/acsync/internal/state"
)

func meta(hash string, mtime int64) state.FileMeta {
	return state.FileMeta{Hash: hash, ModTime: mtime, Size: 1}
}

// find returns the action for a path, or nil.
func find(actions []Action, repoRel string) *Action {
	for i := range actions {
		if actions[i].RepoRel == repoRel {
			return &actions[i]
		}
	}
	return nil
}

func TestReconcileLocalNewPushes(t *testing.T) {
	base := state.Snapshot{}
	local := state.Snapshot{"agents/c/config/a.json": meta("h1", 10)}
	remote := state.Snapshot{}
	a := find(Reconcile(base, local, remote), "agents/c/config/a.json")
	if a == nil || a.Type != PushToRemote {
		t.Fatalf("expected PushToRemote, got %+v", a)
	}
}

func TestReconcileRemoteNewPulls(t *testing.T) {
	base := state.Snapshot{}
	local := state.Snapshot{}
	remote := state.Snapshot{"agents/c/config/a.json": meta("h1", 10)}
	a := find(Reconcile(base, local, remote), "agents/c/config/a.json")
	if a == nil || a.Type != PullToLocal {
		t.Fatalf("expected PullToLocal, got %+v", a)
	}
}

func TestReconcileLocalDeletePropagates(t *testing.T) {
	base := state.Snapshot{"agents/c/config/a.json": meta("h1", 10)}
	local := state.Snapshot{}                                             // locally deleted
	remote := state.Snapshot{"agents/c/config/a.json": meta("h1", 10)}    // unchanged remote
	a := find(Reconcile(base, local, remote), "agents/c/config/a.json")
	if a == nil || a.Type != DeleteRemote {
		t.Fatalf("expected DeleteRemote, got %+v", a)
	}
}

func TestReconcileRemoteDeletePropagates(t *testing.T) {
	base := state.Snapshot{"agents/c/config/a.json": meta("h1", 10)}
	local := state.Snapshot{"agents/c/config/a.json": meta("h1", 10)}     // unchanged local
	remote := state.Snapshot{}                                            // deleted remote
	a := find(Reconcile(base, local, remote), "agents/c/config/a.json")
	if a == nil || a.Type != DeleteLocal {
		t.Fatalf("expected DeleteLocal, got %+v", a)
	}
}

func TestReconcileConflictLastWriterWins(t *testing.T) {
	base := state.Snapshot{"p": meta("h0", 5)}
	// Local newer.
	local := state.Snapshot{"p": meta("hL", 20)}
	remote := state.Snapshot{"p": meta("hR", 10)}
	a := find(Reconcile(base, local, remote), "p")
	if a == nil || a.Type != PushToRemote {
		t.Fatalf("expected PushToRemote (local newer), got %+v", a)
	}
	// Remote newer.
	local2 := state.Snapshot{"p": meta("hL", 10)}
	remote2 := state.Snapshot{"p": meta("hR", 20)}
	a2 := find(Reconcile(base, local2, remote2), "p")
	if a2 == nil || a2.Type != PullToLocal {
		t.Fatalf("expected PullToLocal (remote newer), got %+v", a2)
	}
}

func TestReconcileUnchangedNoAction(t *testing.T) {
	base := state.Snapshot{"p": meta("h1", 10)}
	local := state.Snapshot{"p": meta("h1", 10)}
	remote := state.Snapshot{"p": meta("h1", 10)}
	if got := Reconcile(base, local, remote); len(got) != 0 {
		t.Fatalf("expected no actions, got %v", got)
	}
}

func TestReconcileDeleteModifyResurrects(t *testing.T) {
	// Local deleted, remote modified -> keep remote copy (pull), avoid data loss.
	base := state.Snapshot{"p": meta("h0", 5)}
	local := state.Snapshot{}
	remote := state.Snapshot{"p": meta("hR", 20)}
	a := find(Reconcile(base, local, remote), "p")
	if a == nil || a.Type != PullToLocal {
		t.Fatalf("expected PullToLocal (resurrect), got %+v", a)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/syncengine/ -run TestReconcile -v`
Expected: FAIL — `undefined: Reconcile` / `undefined: PushToRemote`.

- [ ] **Step 3: Write minimal implementation**

Create `internal/syncengine/reconcile.go`:

```go
package syncengine

import (
	"sort"

	"github.com/qinqingxu/acsync/internal/state"
)

// ActionType enumerates the operations a sync pass can perform on one path.
type ActionType int

const (
	// PushToRemote copies the local file into the repo working tree.
	PushToRemote ActionType = iota
	// PullToLocal copies the repo file into the local agent directory.
	PullToLocal
	// DeleteRemote soft-deletes the repo file (moves it to .trash).
	DeleteRemote
	// DeleteLocal removes the local agent file.
	DeleteLocal
)

func (t ActionType) String() string {
	switch t {
	case PushToRemote:
		return "push"
	case PullToLocal:
		return "pull"
	case DeleteRemote:
		return "delete-remote"
	case DeleteLocal:
		return "delete-local"
	default:
		return "unknown"
	}
}

// Action is a single reconciliation operation on a repo-relative path.
type Action struct {
	Type    ActionType
	RepoRel string
}

// Reconcile compares base, local, and remote snapshots and returns the actions
// needed to converge. base is the snapshot recorded after the last successful
// sync; local is the current agent-dir snapshot; remote is the pulled repo
// snapshot.
func Reconcile(base, local, remote state.Snapshot) []Action {
	keys := unionKeys(base, local, remote)
	var actions []Action

	for _, p := range keys {
		b, inB := base[p]
		l, inL := local[p]
		r, inR := remote[p]

		localChanged := sideChanged(inB, b, inL, l)
		remoteChanged := sideChanged(inB, b, inR, r)

		switch {
		case !localChanged && !remoteChanged:
			// converged; nothing to do
		case localChanged && !remoteChanged:
			if inL {
				actions = append(actions, Action{PushToRemote, p})
			} else {
				actions = append(actions, Action{DeleteRemote, p})
			}
		case remoteChanged && !localChanged:
			if inR {
				actions = append(actions, Action{PullToLocal, p})
			} else {
				actions = append(actions, Action{DeleteLocal, p})
			}
		default:
			if a, ok := resolveConflict(p, inL, l, inR, r); ok {
				actions = append(actions, a)
			}
		}
	}
	return actions
}

// sideChanged reports whether a side differs from base.
func sideChanged(inBase bool, base state.FileMeta, inSide bool, side state.FileMeta) bool {
	if inBase != inSide {
		return true
	}
	if !inBase {
		return false
	}
	return base.Hash != side.Hash
}

// resolveConflict handles the both-sides-changed case.
func resolveConflict(p string, inL bool, l state.FileMeta, inR bool, r state.FileMeta) (Action, bool) {
	switch {
	case inL && inR:
		if l.Hash == r.Hash {
			return Action{}, false // same content, already converged
		}
		if l.ModTime >= r.ModTime {
			return Action{PushToRemote, p}, true
		}
		return Action{PullToLocal, p}, true
	case inL && !inR:
		// local modified, remote deleted: resurrect on remote (no data loss)
		return Action{PushToRemote, p}, true
	case !inL && inR:
		// local deleted, remote modified: resurrect locally (no data loss)
		return Action{PullToLocal, p}, true
	default:
		// both deleted
		return Action{}, false
	}
}

func unionKeys(snaps ...state.Snapshot) []string {
	set := map[string]struct{}{}
	for _, s := range snaps {
		for k := range s {
			set[k] = struct{}{}
		}
	}
	out := make([]string, 0, len(set))
	for k := range set {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/syncengine/ -run TestReconcile -v`
Expected: PASS (all cases).

- [ ] **Step 5: Commit**

```bash
git add internal/syncengine/reconcile.go internal/syncengine/reconcile_test.go
git commit -m "feat: add three-way reconcile with LWW and delete propagation"
```

---

## Task 9: syncengine apply — perform actions, soft-delete, repo snapshot

**Files:**
- Create: `internal/syncengine/apply.go`
- Test: `internal/syncengine/apply_test.go`

`Applier.Apply` executes the reconcile actions: copy files between local and repo, and for deletions move repo files into `.trash/files/...` while recording the deletion time in `.trash/index.json`. `SnapshotRepo` hashes the repo's `agents/**` tree — used both as the reconcile "remote" input and to recompute the post-sync base snapshot.

- [ ] **Step 1: Write the failing test**

Create `internal/syncengine/apply_test.go`:

```go
package syncengine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestApplyPushPullDelete(t *testing.T) {
	repoDir := t.TempDir()
	localRoot := t.TempDir()

	specs := map[string]AgentSpec{"c": {Name: "c", Root: localRoot}}

	// Prepare a local source file for push.
	localSrc := filepath.Join(localRoot, "a.json")
	writeFile(t, localSrc, `{"v":1}`)

	// Prepare a repo file for pull.
	writeFile(t, filepath.Join(repoDir, "agents", "c", "config", "b.json"), `{"v":2}`)

	// Prepare a repo file to delete.
	writeFile(t, filepath.Join(repoDir, "agents", "c", "config", "d.json"), `{"v":3}`)

	// Prepare a local file to delete.
	writeFile(t, filepath.Join(localRoot, "e.json"), `{"v":4}`)

	ap := &Applier{
		RepoDir: repoDir,
		Specs:   specs,
		Sources: map[string]string{"agents/c/config/a.json": localSrc},
		Now:     time.Unix(1000, 0),
	}
	actions := []Action{
		{PushToRemote, "agents/c/config/a.json"},
		{PullToLocal, "agents/c/config/b.json"},
		{DeleteRemote, "agents/c/config/d.json"},
		{DeleteLocal, "agents/c/config/e.json"},
	}
	if err := ap.Apply(actions); err != nil {
		t.Fatalf("Apply error: %v", err)
	}

	// Push landed in repo.
	if _, err := os.Stat(filepath.Join(repoDir, "agents", "c", "config", "a.json")); err != nil {
		t.Errorf("pushed file missing in repo: %v", err)
	}
	// Pull landed locally.
	if _, err := os.Stat(filepath.Join(localRoot, "b.json")); err != nil {
		t.Errorf("pulled file missing locally: %v", err)
	}
	// Delete-remote removed from repo and moved to trash.
	if _, err := os.Stat(filepath.Join(repoDir, "agents", "c", "config", "d.json")); !os.IsNotExist(err) {
		t.Errorf("deleted repo file should be gone, err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(repoDir, ".trash", "files", "agents", "c", "config", "d.json")); err != nil {
		t.Errorf("deleted file should be in trash: %v", err)
	}
	// Trash index records deletion time.
	idxData, err := os.ReadFile(filepath.Join(repoDir, ".trash", "index.json"))
	if err != nil {
		t.Fatalf("trash index missing: %v", err)
	}
	var idx map[string]int64
	if err := json.Unmarshal(idxData, &idx); err != nil {
		t.Fatal(err)
	}
	if idx["agents/c/config/d.json"] != 1000 {
		t.Errorf("trash index time = %d, want 1000", idx["agents/c/config/d.json"])
	}
	// Delete-local removed the local file.
	if _, err := os.Stat(filepath.Join(localRoot, "e.json")); !os.IsNotExist(err) {
		t.Errorf("deleted local file should be gone, err=%v", err)
	}
}

func TestSnapshotRepoSkipsTrashAndGit(t *testing.T) {
	repoDir := t.TempDir()
	writeFile(t, filepath.Join(repoDir, "agents", "c", "config", "a.json"), `{"v":1}`)
	writeFile(t, filepath.Join(repoDir, ".trash", "files", "agents", "c", "config", "old.json"), `{"v":9}`)
	writeFile(t, filepath.Join(repoDir, ".git", "HEAD"), "ref: refs/heads/main")
	writeFile(t, filepath.Join(repoDir, "manifest.json"), `{}`)

	snap, err := SnapshotRepo(repoDir)
	if err != nil {
		t.Fatalf("SnapshotRepo error: %v", err)
	}
	if _, ok := snap["agents/c/config/a.json"]; !ok {
		t.Error("expected agents file in snapshot")
	}
	if len(snap) != 1 {
		t.Errorf("snapshot should only include agents/** files, got %v", snap)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/syncengine/ -run "TestApply|TestSnapshotRepo" -v`
Expected: FAIL — `undefined: Applier` / `undefined: SnapshotRepo`.

- [ ] **Step 3: Write minimal implementation**

Create `internal/syncengine/apply.go`:

```go
package syncengine

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/qinqingxu/acsync/internal/state"
)

// Applier executes reconcile actions against the repo and local agent dirs.
type Applier struct {
	RepoDir string
	Specs   map[string]AgentSpec // by agent name, for local path resolution
	Sources map[string]string    // repo-relative path -> local absolute source
	Now     time.Time
}

// Apply runs all actions in order.
func (a *Applier) Apply(actions []Action) error {
	for _, act := range actions {
		if err := a.applyOne(act); err != nil {
			return fmt.Errorf("apply %s %s: %w", act.Type, act.RepoRel, err)
		}
	}
	return nil
}

func (a *Applier) applyOne(act Action) error {
	switch act.Type {
	case PushToRemote:
		src := a.Sources[act.RepoRel]
		if src == "" {
			return fmt.Errorf("no local source for push")
		}
		return copyFile(src, filepath.Join(a.RepoDir, filepath.FromSlash(act.RepoRel)))
	case PullToLocal:
		dst, err := a.localPathFor(act.RepoRel)
		if err != nil {
			return err
		}
		return copyFile(filepath.Join(a.RepoDir, filepath.FromSlash(act.RepoRel)), dst)
	case DeleteRemote:
		return a.softDeleteRemote(act.RepoRel)
	case DeleteLocal:
		dst, err := a.localPathFor(act.RepoRel)
		if err != nil {
			return err
		}
		if err := os.Remove(dst); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	default:
		return fmt.Errorf("unknown action type %v", act.Type)
	}
}

// localPathFor maps a repo-relative path back to the local agent file path.
// Expects "agents/<name>/<sub>/<rest...>".
func (a *Applier) localPathFor(repoRel string) (string, error) {
	parts := strings.Split(repoRel, "/")
	if len(parts) < 4 || parts[0] != "agents" {
		return "", fmt.Errorf("cannot map repo path %q", repoRel)
	}
	name := parts[1]
	rest := parts[3:]
	spec, ok := a.Specs[name]
	if !ok {
		return "", fmt.Errorf("no spec for agent %q", name)
	}
	return filepath.Join(spec.Root, filepath.FromSlash(strings.Join(rest, "/"))), nil
}

func (a *Applier) softDeleteRemote(repoRel string) error {
	repoPath := filepath.Join(a.RepoDir, filepath.FromSlash(repoRel))
	trashPath := filepath.Join(a.RepoDir, ".trash", "files", filepath.FromSlash(repoRel))

	if _, err := os.Stat(repoPath); os.IsNotExist(err) {
		return nil // already gone
	}
	if err := os.MkdirAll(filepath.Dir(trashPath), 0o755); err != nil {
		return err
	}
	if err := os.Rename(repoPath, trashPath); err != nil {
		// Cross-device fallback: copy then remove.
		if cerr := copyFile(repoPath, trashPath); cerr != nil {
			return cerr
		}
		if rerr := os.Remove(repoPath); rerr != nil {
			return rerr
		}
	}
	return a.recordTrash(repoRel)
}

func (a *Applier) recordTrash(repoRel string) error {
	idxPath := filepath.Join(a.RepoDir, ".trash", "index.json")
	idx := map[string]int64{}
	if data, err := os.ReadFile(idxPath); err == nil {
		_ = json.Unmarshal(data, &idx)
	}
	idx[repoRel] = a.Now.Unix()
	data, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(idxPath), 0o755); err != nil {
		return err
	}
	return os.WriteFile(idxPath, data, 0o644)
}

func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	tmp := dst + ".tmp"
	out, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	if err := out.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, dst)
}

// SnapshotRepo hashes every file under the repo's agents/ tree, keyed by
// repo-relative forward-slash path. It skips .git, .trash, and top-level
// metadata files.
func SnapshotRepo(repoDir string) (state.Snapshot, error) {
	snap := state.Snapshot{}
	agentsRoot := filepath.Join(repoDir, "agents")
	if _, err := os.Stat(agentsRoot); os.IsNotExist(err) {
		return snap, nil
	}

	err := filepath.WalkDir(agentsRoot, func(abs string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		relOS, err := filepath.Rel(repoDir, abs)
		if err != nil {
			return err
		}
		rel := filepath.ToSlash(relOS)
		if !strings.HasPrefix(rel, "agents/") {
			return nil
		}
		hash, err := state.HashFile(abs)
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		snap[path.Clean(rel)] = state.FileMeta{
			Hash:    hash,
			ModTime: info.ModTime().Unix(),
			Size:    info.Size(),
		}
		return nil
	})
	return snap, err
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/syncengine/ -run "TestApply|TestSnapshotRepo" -v`
Expected: PASS (both tests).

- [ ] **Step 5: Commit**

```bash
git add internal/syncengine/apply.go internal/syncengine/apply_test.go
git commit -m "feat: add apply with soft-delete trash and repo snapshot"
```

---

## Task 10: syncengine engine — SyncOnce orchestration + integration test

**Files:**
- Create: `internal/syncengine/engine.go`
- Test: `internal/syncengine/engine_test.go`

`Engine.SyncOnce` runs a complete pass: pull → snapshot remote → collect local → reconcile → apply → commit/push (with pull-rebase retry) → recompute and save the base snapshot. The integration test uses a local bare git repo as a fake GitHub and two working clones to prove push, pull, and delete propagation across "machines".

- [ ] **Step 1: Write the failing test**

Create `internal/syncengine/engine_test.go`:

```go
package syncengine

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/qinqingxu/acsync/internal/gitclient"
)

func git(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func newBareRemote(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	bare := filepath.Join(root, "remote.git")
	git(t, root, "init", "--bare", "-b", "main", bare)
	seed := filepath.Join(root, "seed")
	git(t, root, "clone", bare, seed)
	git(t, seed, "config", "user.email", "s@e.com")
	git(t, seed, "config", "user.name", "seed")
	os.WriteFile(filepath.Join(seed, "manifest.json"), []byte("{}\n"), 0o644)
	git(t, seed, "add", ".")
	git(t, seed, "commit", "-m", "seed")
	git(t, seed, "push", "origin", "main")
	return bare
}

func cloneWorkspace(t *testing.T, bare, dir string) *gitclient.Client {
	t.Helper()
	c := &gitclient.Client{Dir: dir}
	if err := c.Clone(bare, dir); err != nil {
		t.Fatalf("clone: %v", err)
	}
	git(t, dir, "config", "user.email", "m@e.com")
	git(t, dir, "config", "user.name", "machine")
	return c
}

func engineFor(client *gitclient.Client, repoDir, statePath, agentRoot string) *Engine {
	return &Engine{
		Git:       client,
		RepoDir:   repoDir,
		StatePath: statePath,
		Specs: map[string]AgentSpec{
			"demo": {Name: "demo", Root: agentRoot, Include: []string{"settings.json"}},
		},
		PushRetries: 3,
		Now:         func() time.Time { return time.Unix(1000, 0) },
	}
}

func TestSyncOncePropagatesCreateAndDelete(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	bare := newBareRemote(t)

	// Machine A.
	repoA := filepath.Join(t.TempDir(), "repoA")
	clientA := cloneWorkspace(t, bare, repoA)
	rootA := t.TempDir()
	stateA := filepath.Join(t.TempDir(), "stateA.json")
	engA := engineFor(clientA, repoA, stateA, rootA)

	// Machine B.
	repoB := filepath.Join(t.TempDir(), "repoB")
	clientB := cloneWorkspace(t, bare, repoB)
	rootB := t.TempDir()
	stateB := filepath.Join(t.TempDir(), "stateB.json")
	engB := engineFor(clientB, repoB, stateB, rootB)

	// A creates a config file and syncs (push).
	writeFile(t, filepath.Join(rootA, "settings.json"), `{"theme":"dark"}`)
	if _, err := engA.SyncOnce(); err != nil {
		t.Fatalf("A first sync: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repoA, "agents", "demo", "config", "settings.json")); err != nil {
		t.Fatalf("A repo should contain settings.json: %v", err)
	}

	// B syncs (pull) and should receive the file locally.
	if _, err := engB.SyncOnce(); err != nil {
		t.Fatalf("B first sync: %v", err)
	}
	if _, err := os.Stat(filepath.Join(rootB, "settings.json")); err != nil {
		t.Fatalf("B should have pulled settings.json locally: %v", err)
	}

	// A deletes the file and syncs (delete propagation + soft delete).
	if err := os.Remove(filepath.Join(rootA, "settings.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := engA.SyncOnce(); err != nil {
		t.Fatalf("A delete sync: %v", err)
	}
	if _, err := os.Stat(filepath.Join(repoA, ".trash", "files", "agents", "demo", "config", "settings.json")); err != nil {
		t.Fatalf("deleted file should be in A's trash: %v", err)
	}

	// B syncs and should delete its local copy.
	if _, err := engB.SyncOnce(); err != nil {
		t.Fatalf("B delete sync: %v", err)
	}
	if _, err := os.Stat(filepath.Join(rootB, "settings.json")); !os.IsNotExist(err) {
		t.Fatalf("B local settings.json should be deleted, err=%v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/syncengine/ -run TestSyncOnce -v`
Expected: FAIL — `undefined: Engine`.

- [ ] **Step 3: Write minimal implementation**

Create `internal/syncengine/engine.go`:

```go
package syncengine

import (
	"fmt"
	"time"

	"github.com/qinqingxu/acsync/internal/gitclient"
	"github.com/qinqingxu/acsync/internal/state"
)

// Engine orchestrates a single sync pass over a cloned repo workspace.
type Engine struct {
	Git         *gitclient.Client
	RepoDir     string
	StatePath   string
	Specs       map[string]AgentSpec
	PushRetries int
	Now         func() time.Time
}

// Result summarizes what a sync pass did.
type Result struct {
	Actions []Action
	Blocked []string
	Pushed  bool
}

// SyncOnce performs one full pull → reconcile → apply → push cycle.
func (e *Engine) SyncOnce() (Result, error) {
	now := time.Now
	if e.Now != nil {
		now = e.Now
	}

	if err := e.Git.PullRebase(); err != nil {
		return Result{}, fmt.Errorf("pull: %w", err)
	}

	remote, err := SnapshotRepo(e.RepoDir)
	if err != nil {
		return Result{}, fmt.Errorf("snapshot remote: %w", err)
	}

	specs := make([]AgentSpec, 0, len(e.Specs))
	for _, s := range e.Specs {
		specs = append(specs, s)
	}
	collected, err := Collect(specs)
	if err != nil {
		return Result{}, fmt.Errorf("collect: %w", err)
	}

	base, err := state.Load(e.StatePath)
	if err != nil {
		return Result{}, fmt.Errorf("load state: %w", err)
	}

	actions := Reconcile(base, collected.Snapshot, remote)

	ap := &Applier{
		RepoDir: e.RepoDir,
		Specs:   e.Specs,
		Sources: collected.Sources,
		Now:     now(),
	}
	if err := ap.Apply(actions); err != nil {
		return Result{}, fmt.Errorf("apply: %w", err)
	}

	pushed := false
	if err := e.Git.AddAll(); err != nil {
		return Result{}, fmt.Errorf("git add: %w", err)
	}
	changed, err := e.Git.HasChanges()
	if err != nil {
		return Result{}, err
	}
	if changed {
		msg := fmt.Sprintf("acsync sync %s", now().UTC().Format(time.RFC3339))
		if err := e.Git.Commit(msg); err != nil {
			return Result{}, fmt.Errorf("commit: %w", err)
		}
		if err := e.pushWithRetry(); err != nil {
			return Result{}, fmt.Errorf("push: %w", err)
		}
		pushed = true
	}

	newBase, err := SnapshotRepo(e.RepoDir)
	if err != nil {
		return Result{}, fmt.Errorf("snapshot new base: %w", err)
	}
	if err := state.Save(e.StatePath, newBase); err != nil {
		return Result{}, fmt.Errorf("save state: %w", err)
	}

	return Result{Actions: actions, Blocked: collected.Blocked, Pushed: pushed}, nil
}

func (e *Engine) pushWithRetry() error {
	retries := e.PushRetries
	if retries < 1 {
		retries = 1
	}
	var err error
	for i := 0; i < retries; i++ {
		if err = e.Git.Push(); err == nil {
			return nil
		}
		if perr := e.Git.PullRebase(); perr != nil {
			return perr
		}
	}
	return err
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/syncengine/ -run TestSyncOnce -v`
Expected: PASS.

- [ ] **Step 5: Run the whole syncengine package**

Run: `go test ./internal/syncengine/ -v`
Expected: PASS (collect, reconcile, apply, engine).

- [ ] **Step 6: Commit**

```bash
git add internal/syncengine/engine.go internal/syncengine/engine_test.go
git commit -m "feat: add SyncOnce engine orchestration with integration test"
```

---

## Task 11: cli runtime helpers + `acsync init`

**Files:**
- Create: `internal/cli/runtime.go`
- Create: `internal/cli/init.go`
- Test: `internal/cli/runtime_test.go`
- Test: `internal/cli/init_test.go`

`runtime.go` centralizes acsync paths, provider loading (builtins + user YAML), and building `AgentSpec`s from config. `init.go`'s `RunInit` scaffolds `~/.acsync`, clones the repo (via an injected cloner so tests avoid network), detects builtin agents, and writes a default config.

- [ ] **Step 1: Write the failing tests**

Create `internal/cli/runtime_test.go`:

```go
package cli

import (
	"testing"

	"github.com/qinqingxu/acsync/internal/config"
	"github.com/qinqingxu/acsync/internal/provider"
)

func TestBuildSpecsEnabledOnly(t *testing.T) {
	providers := []provider.Provider{
		{
			Name: "claude",
			Config: provider.ConfigSpec{
				Paths:    map[string]string{"linux": "~/.claude"},
				Include:  []string{"settings.json"},
				Sessions: []string{"projects/**/*.jsonl"},
				Exclude:  []string{"**/*token*"},
			},
			Secrets: provider.SecretSpec{KeyPatterns: []string{"token"}},
		},
		{
			Name: "gemini",
			Config: provider.ConfigSpec{
				Paths: map[string]string{"linux": "~/.gemini"},
			},
		},
	}
	cfg := config.Config{Agents: map[string]bool{"claude": true, "gemini": false}}

	specs, err := BuildSpecs(cfg, providers, "linux", "/home/alice")
	if err != nil {
		t.Fatalf("BuildSpecs error: %v", err)
	}
	if _, ok := specs["gemini"]; ok {
		t.Error("disabled agent should be excluded")
	}
	s, ok := specs["claude"]
	if !ok {
		t.Fatal("claude spec missing")
	}
	if s.Root != "/home/alice/.claude" {
		t.Errorf("root = %q", s.Root)
	}
	if s.Scanner == nil {
		t.Error("scanner should be set")
	}
}
```

Create `internal/cli/init_test.go`:

```go
package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/qinqingxu/acsync/internal/config"
)

type fakeCloner struct{ called bool }

func (f *fakeCloner) Clone(url, dir string) error {
	f.called = true
	return os.MkdirAll(dir, 0o755)
}

func TestRunInitScaffolds(t *testing.T) {
	home := filepath.Join(t.TempDir(), ".acsync")
	fc := &fakeCloner{}

	if err := RunInit(home, "https://example.com/me/data.git", fc); err != nil {
		t.Fatalf("RunInit error: %v", err)
	}
	if !fc.called {
		t.Error("cloner should have been called")
	}
	cfg, err := config.Load(ConfigPath(home))
	if err != nil {
		t.Fatalf("config not written: %v", err)
	}
	if cfg.RepoURL != "https://example.com/me/data.git" {
		t.Errorf("repo url = %q", cfg.RepoURL)
	}
	for _, name := range []string{"claude", "copilot", "gemini", "cursor"} {
		if !cfg.Agents[name] {
			t.Errorf("agent %q should be enabled by default", name)
		}
	}
	if _, err := os.Stat(ProvidersDir(home)); err != nil {
		t.Errorf("providers dir missing: %v", err)
	}
	attr, err := os.ReadFile(filepath.Join(RepoDir(home), ".gitattributes"))
	if err != nil {
		t.Fatalf(".gitattributes not written: %v", err)
	}
	if string(attr) != "* -text\n" {
		t.Errorf(".gitattributes = %q, want %q", string(attr), "* -text\n")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/cli/ -v`
Expected: FAIL — `undefined: BuildSpecs` / `undefined: RunInit`.

- [ ] **Step 3: Write runtime.go**

Create `internal/cli/runtime.go`:

```go
// Package cli implements acsync commands.
package cli

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/qinqingxu/acsync/internal/config"
	"github.com/qinqingxu/acsync/internal/pathresolver"
	"github.com/qinqingxu/acsync/internal/provider"
	"github.com/qinqingxu/acsync/internal/secret"
	"github.com/qinqingxu/acsync/internal/syncengine"
)

// Home returns the acsync home directory (~/.acsync).
func Home() (string, error) {
	h, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(h, ".acsync"), nil
}

// ConfigPath returns the config file path for a given home.
func ConfigPath(home string) string { return filepath.Join(home, "config.yaml") }

// RepoDir returns the local repo clone directory.
func RepoDir(home string) string { return filepath.Join(home, "repo") }

// StatePath returns the snapshot state file path.
func StatePath(home string) string { return filepath.Join(home, "state.json") }

// ProvidersDir returns the user provider directory.
func ProvidersDir(home string) string { return filepath.Join(home, "providers") }

// LogsDir returns the log directory.
func LogsDir(home string) string { return filepath.Join(home, "logs") }

// LoadProviders returns builtin providers merged with user YAML in
// ProvidersDir(home). User definitions with a duplicate name override builtins.
func LoadProviders(home string) ([]provider.Provider, error) {
	builtins, err := provider.Builtins()
	if err != nil {
		return nil, err
	}
	byName := map[string]provider.Provider{}
	order := []string{}
	for _, p := range builtins {
		if _, ok := byName[p.Name]; !ok {
			order = append(order, p.Name)
		}
		byName[p.Name] = p
	}

	entries, err := os.ReadDir(ProvidersDir(home))
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		p, err := provider.LoadFile(filepath.Join(ProvidersDir(home), e.Name()))
		if err != nil {
			return nil, err
		}
		if _, ok := byName[p.Name]; !ok {
			order = append(order, p.Name)
		}
		byName[p.Name] = p
	}

	out := make([]provider.Provider, 0, len(order))
	for _, name := range order {
		out = append(out, byName[name])
	}
	return out, nil
}

// BuildSpecs resolves enabled providers into AgentSpecs for the given OS/home.
func BuildSpecs(cfg config.Config, providers []provider.Provider, goos, home string) (map[string]syncengine.AgentSpec, error) {
	specs := map[string]syncengine.AgentSpec{}
	for _, p := range providers {
		if !cfg.Agents[p.Name] {
			continue
		}
		raw, ok := p.Config.Paths[goos]
		if !ok || raw == "" {
			continue
		}
		root, err := pathresolver.ResolveFor(raw, goos, home)
		if err != nil {
			return nil, err
		}
		specs[p.Name] = syncengine.AgentSpec{
			Name:     p.Name,
			Root:     root,
			Include:  p.Config.Include,
			Sessions: p.Config.Sessions,
			Scanner:  secret.NewScanner(p.Config.Exclude, p.Secrets.KeyPatterns),
		}
	}
	return specs, nil
}
```

- [ ] **Step 4: Write init.go**

Create `internal/cli/init.go`:

```go
package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/qinqingxu/acsync/internal/config"
)

// Cloner clones a git URL into a directory.
type Cloner interface {
	Clone(url, dir string) error
}

// RunInit scaffolds the acsync home, clones the data repo, detects agents, and
// writes a default config.
func RunInit(home, repoURL string, cloner Cloner) error {
	for _, dir := range []string{home, ProvidersDir(home), LogsDir(home)} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}

	repo := RepoDir(home)
	empty, err := isEmptyOrMissing(repo)
	if err != nil {
		return err
	}
	if empty {
		if err := os.RemoveAll(repo); err != nil {
			return err
		}
		if err := cloner.Clone(repoURL, repo); err != nil {
			return fmt.Errorf("clone %s: %w", repoURL, err)
		}
	}

	// Store agent files byte-for-byte: never let git rewrite line endings.
	if err := ensureGitAttributes(repo); err != nil {
		return err
	}

	providers, err := LoadProviders(home)
	if err != nil {
		return err
	}
	names := make([]string, 0, len(providers))
	for _, p := range providers {
		names = append(names, p.Name)
	}

	cfg := config.Default(names)
	cfg.RepoURL = repoURL
	return config.Save(ConfigPath(home), cfg)
}

func isEmptyOrMissing(dir string) (bool, error) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	return len(entries) == 0, nil
}

// ensureGitAttributes writes a `.gitattributes` into the data repo (if the repo
// directory exists) so git treats every stored file as binary and never
// rewrites CRLF/LF. acsync copies agent files byte-for-byte, so git must not
// touch their bytes. Idempotent: a no-op if the file already exists.
func ensureGitAttributes(repo string) error {
	if _, err := os.Stat(repo); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	path := filepath.Join(repo, ".gitattributes")
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	return os.WriteFile(path, []byte("* -text\n"), 0o644)
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/cli/ -v`
Expected: PASS (BuildSpecs + RunInit).

- [ ] **Step 6: Commit**

```bash
git add internal/cli/runtime.go internal/cli/init.go internal/cli/runtime_test.go internal/cli/init_test.go
git commit -m "feat: add cli runtime helpers and init command core"
```

---

## Task 12: `acsync sync`

**Files:**
- Create: `internal/cli/sync.go`
- Test: `internal/cli/sync_test.go`

`RunSync` loads config + providers, builds specs for the current OS, constructs the `Engine`, and runs one `SyncOnce`. The test wires the whole path end-to-end against a bare git remote.

- [ ] **Step 1: Write the failing test**

Create `internal/cli/sync_test.go`:

```go
package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/qinqingxu/acsync/internal/config"
	"github.com/qinqingxu/acsync/internal/gitclient"
	"github.com/qinqingxu/acsync/internal/provider"
	"gopkg.in/yaml.v3"
)

func gitCmd(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

func bareRemote(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	bare := filepath.Join(root, "remote.git")
	gitCmd(t, root, "init", "--bare", "-b", "main", bare)
	seed := filepath.Join(root, "seed")
	gitCmd(t, root, "clone", bare, seed)
	gitCmd(t, seed, "config", "user.email", "s@e.com")
	gitCmd(t, seed, "config", "user.name", "seed")
	os.WriteFile(filepath.Join(seed, "manifest.json"), []byte("{}\n"), 0o644)
	gitCmd(t, seed, "add", ".")
	gitCmd(t, seed, "commit", "-m", "seed")
	gitCmd(t, seed, "push", "origin", "main")
	return bare
}

func TestRunSyncPushesConfig(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	home := filepath.Join(t.TempDir(), ".acsync")
	if err := os.MkdirAll(ProvidersDir(home), 0o755); err != nil {
		t.Fatal(err)
	}
	bare := bareRemote(t)

	// Clone repo workspace and set identity.
	client := &gitclient.Client{Dir: RepoDir(home)}
	if err := client.Clone(bare, RepoDir(home)); err != nil {
		t.Fatalf("clone: %v", err)
	}
	gitCmd(t, RepoDir(home), "config", "user.email", "m@e.com")
	gitCmd(t, RepoDir(home), "config", "user.name", "machine")

	// A user provider pointing at a temp agent root.
	agentRoot := t.TempDir()
	os.WriteFile(filepath.Join(agentRoot, "settings.json"), []byte(`{"theme":"dark"}`), 0o644)
	p := provider.Provider{
		Name: "demo",
		Config: provider.ConfigSpec{
			Paths:   map[string]string{runtime.GOOS: agentRoot},
			Include: []string{"settings.json"},
		},
	}
	data, _ := yaml.Marshal(p)
	if err := os.WriteFile(filepath.Join(ProvidersDir(home), "demo.yaml"), data, 0o644); err != nil {
		t.Fatal(err)
	}

	// Config enabling only demo.
	cfg := config.Config{
		RepoURL:             bare,
		SyncIntervalMinutes: 10,
		TrashGraceDays:      30,
		Agents:              map[string]bool{"demo": true},
	}
	if err := config.Save(ConfigPath(home), cfg); err != nil {
		t.Fatal(err)
	}

	res, err := RunSync(home, runtime.GOOS)
	if err != nil {
		t.Fatalf("RunSync error: %v", err)
	}
	if !res.Pushed {
		t.Error("expected a push")
	}
	if _, err := os.Stat(filepath.Join(RepoDir(home), "agents", "demo", "config", "settings.json")); err != nil {
		t.Errorf("repo should contain the pushed file: %v", err)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/cli/ -run TestRunSync -v`
Expected: FAIL — `undefined: RunSync`.

- [ ] **Step 3: Write minimal implementation**

Create `internal/cli/sync.go`:

```go
package cli

import (
	"time"

	"github.com/qinqingxu/acsync/internal/config"
	"github.com/qinqingxu/acsync/internal/gitclient"
	"github.com/qinqingxu/acsync/internal/syncengine"
)

// RunSync loads settings from home and performs one sync pass for goos.
func RunSync(home, goos string) (syncengine.Result, error) {
	cfg, err := config.Load(ConfigPath(home))
	if err != nil {
		return syncengine.Result{}, err
	}
	providers, err := LoadProviders(home)
	if err != nil {
		return syncengine.Result{}, err
	}
	specs, err := BuildSpecs(cfg, providers, goos, home)
	if err != nil {
		return syncengine.Result{}, err
	}

	client := &gitclient.Client{Dir: RepoDir(home)}
	eng := &syncengine.Engine{
		Git:         client,
		RepoDir:     RepoDir(home),
		StatePath:   StatePath(home),
		Specs:       specs,
		PushRetries: 5,
		Now:         time.Now,
	}
	return eng.SyncOnce()
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/cli/ -run TestRunSync -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/cli/sync.go internal/cli/sync_test.go
git commit -m "feat: add acsync sync command core"
```

---

## Task 13: `acsync status`

**Files:**
- Create: `internal/cli/status.go`
- Test: `internal/cli/status_test.go`

`RunStatus` reports the repo URL, enabled agents, last sync time (state file mtime), and the number of pending actions computed without pushing (compares base vs current local vs current repo working tree). Needs no network.

- [ ] **Step 1: Write the failing test**

Create `internal/cli/status_test.go`:

```go
package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/qinqingxu/acsync/internal/config"
	"github.com/qinqingxu/acsync/internal/provider"
	"gopkg.in/yaml.v3"
)

func TestRunStatusCountsPending(t *testing.T) {
	home := filepath.Join(t.TempDir(), ".acsync")
	if err := os.MkdirAll(ProvidersDir(home), 0o755); err != nil {
		t.Fatal(err)
	}

	agentRoot := t.TempDir()
	os.WriteFile(filepath.Join(agentRoot, "settings.json"), []byte(`{"a":1}`), 0o644)
	p := provider.Provider{
		Name: "demo",
		Config: provider.ConfigSpec{
			Paths:   map[string]string{runtime.GOOS: agentRoot},
			Include: []string{"settings.json"},
		},
	}
	data, _ := yaml.Marshal(p)
	os.WriteFile(filepath.Join(ProvidersDir(home), "demo.yaml"), data, 0o644)

	cfg := config.Config{
		RepoURL: "https://example.com/data.git",
		Agents:  map[string]bool{"demo": true},
	}
	if err := config.Save(ConfigPath(home), cfg); err != nil {
		t.Fatal(err)
	}

	st, err := RunStatus(home, runtime.GOOS)
	if err != nil {
		t.Fatalf("RunStatus error: %v", err)
	}
	if st.RepoURL != "https://example.com/data.git" {
		t.Errorf("repo url = %q", st.RepoURL)
	}
	if len(st.EnabledAgents) != 1 || st.EnabledAgents[0] != "demo" {
		t.Errorf("enabled agents = %v", st.EnabledAgents)
	}
	if st.PendingActions != 1 {
		t.Errorf("pending = %d, want 1", st.PendingActions)
	}
	if !st.LastSync.IsZero() {
		t.Errorf("last sync should be zero when no state file")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/cli/ -run TestRunStatus -v`
Expected: FAIL — `undefined: RunStatus`.

- [ ] **Step 3: Write minimal implementation**

Create `internal/cli/status.go`:

```go
package cli

import (
	"os"
	"time"

	"github.com/qinqingxu/acsync/internal/config"
	"github.com/qinqingxu/acsync/internal/state"
	"github.com/qinqingxu/acsync/internal/syncengine"
)

// Status is a snapshot of the current sync situation.
type Status struct {
	RepoURL        string
	EnabledAgents  []string
	LastSync       time.Time
	PendingActions int
}

// RunStatus computes status for home without contacting the remote.
func RunStatus(home, goos string) (Status, error) {
	cfg, err := config.Load(ConfigPath(home))
	if err != nil {
		return Status{}, err
	}
	providers, err := LoadProviders(home)
	if err != nil {
		return Status{}, err
	}
	specs, err := BuildSpecs(cfg, providers, goos, home)
	if err != nil {
		return Status{}, err
	}

	specList := make([]syncengine.AgentSpec, 0, len(specs))
	for _, s := range specs {
		specList = append(specList, s)
	}
	collected, err := syncengine.Collect(specList)
	if err != nil {
		return Status{}, err
	}

	remote, err := syncengine.SnapshotRepo(RepoDir(home))
	if err != nil {
		return Status{}, err
	}
	base, err := state.Load(StatePath(home))
	if err != nil {
		return Status{}, err
	}

	actions := syncengine.Reconcile(base, collected.Snapshot, remote)

	var last time.Time
	if info, err := os.Stat(StatePath(home)); err == nil {
		last = info.ModTime()
	}

	return Status{
		RepoURL:        cfg.RepoURL,
		EnabledAgents:  cfg.EnabledAgents(),
		LastSync:       last,
		PendingActions: len(actions),
	}, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/cli/ -run TestRunStatus -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/cli/status.go internal/cli/status_test.go
git commit -m "feat: add acsync status command core"
```

---

## Task 14: Wire the cobra CLI and verify the whole build

**Files:**
- Modify: `cmd/acsync/main.go` (replace the Task 0 stub)

- [ ] **Step 1: Add the cobra dependency**

Run: `go get github.com/spf13/cobra@v1.8.1`
Expected: adds cobra (and its deps) to `go.mod`/`go.sum`.

- [ ] **Step 2: Replace `cmd/acsync/main.go`**

Replace the entire contents of `cmd/acsync/main.go` with:

```go
package main

import (
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/qinqingxu/acsync/internal/cli"
	"github.com/qinqingxu/acsync/internal/gitclient"
	"github.com/spf13/cobra"
)

func main() {
	root := &cobra.Command{
		Use:   "acsync",
		Short: "Sync AI agent config and session files across machines via a private GitHub repo",
	}
	root.AddCommand(initCmd(), syncCmd(), statusCmd())
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func initCmd() *cobra.Command {
	var repo string
	cmd := &cobra.Command{
		Use:   "init",
		Short: "Initialize acsync: bind a private repo, clone it, detect agents",
		RunE: func(cmd *cobra.Command, args []string) error {
			home, err := cli.Home()
			if err != nil {
				return err
			}
			if err := cli.RunInit(home, repo, &gitclient.Client{}); err != nil {
				return err
			}
			fmt.Printf("Initialized acsync at %s (repo: %s)\n", home, repo)
			return nil
		},
	}
	cmd.Flags().StringVar(&repo, "repo", "", "private git repo URL (required)")
	_ = cmd.MarkFlagRequired("repo")
	return cmd
}

func syncCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "sync",
		Short: "Run one sync pass now",
		RunE: func(cmd *cobra.Command, args []string) error {
			home, err := cli.Home()
			if err != nil {
				return err
			}
			res, err := cli.RunSync(home, runtime.GOOS)
			if err != nil {
				return err
			}
			fmt.Printf("Sync complete: %d actions, %d blocked, pushed=%v\n",
				len(res.Actions), len(res.Blocked), res.Pushed)
			return nil
		},
	}
}

func statusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show current sync status",
		RunE: func(cmd *cobra.Command, args []string) error {
			home, err := cli.Home()
			if err != nil {
				return err
			}
			st, err := cli.RunStatus(home, runtime.GOOS)
			if err != nil {
				return err
			}
			fmt.Printf("Repo:            %s\n", st.RepoURL)
			fmt.Printf("Enabled agents:  %v\n", st.EnabledAgents)
			if st.LastSync.IsZero() {
				fmt.Println("Last sync:       never")
			} else {
				fmt.Printf("Last sync:       %s\n", st.LastSync.Format(time.RFC3339))
			}
			fmt.Printf("Pending actions: %d\n", st.PendingActions)
			return nil
		},
	}
}
```

- [ ] **Step 3: Tidy modules**

Run: `go mod tidy`
Expected: `go.mod`/`go.sum` reflect exactly the used dependencies (cobra, yaml.v3, doublestar).

- [ ] **Step 4: Vet and build**

Run: `go vet ./... && go build ./...`
Expected: no output (success).

- [ ] **Step 5: Run the full test suite**

Run: `go test ./...`
Expected: all packages `ok` (pathresolver, secret, provider, state, gitclient, config, syncengine, cli).

- [ ] **Step 6: Verify the CLI help works**

Run: `go run ./cmd/acsync --help`
Expected: usage listing `init`, `sync`, and `status` commands.

- [ ] **Step 7: Cross-compile smoke test for all three platforms**

Run (PowerShell):

```powershell
$env:GOOS="windows"; go build -o dist/acsync-windows.exe ./cmd/acsync
$env:GOOS="darwin";  go build -o dist/acsync-darwin ./cmd/acsync
$env:GOOS="linux";   go build -o dist/acsync-linux ./cmd/acsync
Remove-Item Env:\GOOS
```

Expected: three binaries produced in `dist/` with no build errors.

- [ ] **Step 8: Commit**

```bash
git add cmd/acsync/main.go go.mod go.sum
git commit -m "feat: wire cobra CLI with init, sync, and status commands"
```

---

## Done Criteria (Plan 1)

- `go test ./...` passes, including the syncengine integration test that proves create/pull/delete propagation across two workspaces sharing one bare remote.
- `acsync init --repo <url>` scaffolds `~/.acsync`, clones the repo, and writes a default config with all agents enabled.
- `acsync sync` performs a full pull → reconcile → apply → push pass, filtering secrets and soft-deleting into `.trash/`.
- `acsync status` reports repo URL, enabled agents, last sync time, and pending action count.
- Binaries cross-compile for Windows, macOS, and Linux.

**Out of scope (later plans):**
- Plan 2: `scheduler` (10-minute timer), `daemon` command, `autostart` (registry / LaunchAgent / systemd), `.trash` grace-period cleanup.
- Plan 3: `tray` (system tray + status icons) and `settings` (local web settings UI + per-agent toggles from the tray menu).

**Deliberately deferred spec details (tracked, not lost):**
- **Newline/LF normalization** (spec §6, §4.2): files are hashed and copied byte-for-byte in this plan. Cross-platform CRLF↔LF divergence therefore surfaces as a normal LWW edit rather than being normalized to LF-in-repo. A dedicated text-detection + safe-restore pass is deferred to a later plan; `acsync init` writes a `.gitattributes` (`* -text`) into the data repo (Task 11) so git itself never rewrites bytes in the meantime.
- **`manifest.json` machine registry** (spec §6): the file exists as top-level repo metadata (and `SnapshotRepo` intentionally skips it), but populating a per-machine registry is not required for core sync and is deferred.

---
