# AgentConfigSync Scheduler, Daemon & Autostart Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Turn the one-shot `acsync sync` (Plan 1) into a long-running background service — a scheduler that syncs every N minutes with manual trigger and pause, trash grace-period cleanup, a `daemon` command, and cross-platform autostart (`install`/`uninstall`).

**Architecture:** A `scheduler.Scheduler` owns an interval loop plus manual `Trigger`/`Pause`/`Resume` and emits a `State` (Idle/Updating/Error/Paused) via a callback. A `daemon.Daemon` wires the scheduler's job to `cli.RunSync` + trash cleanup, logs to `~/.acsync/logs`, and runs until the process is signalled. `autostart.Manager` writes an OS-appropriate startup entry (Windows Startup-folder `.cmd`, macOS LaunchAgent plist, Linux systemd user unit). All state-machine and content-generation logic is pure and unit-tested; OS side effects are thin and injectable.

**Tech Stack:** Go 1.22+ standard library only (`context`, `time`, `os/signal`, `os/exec`, `text/template`); builds on Plan 1 packages (`syncengine`, `cli`, `config`, `gitclient`). `github.com/spf13/cobra` (already a dependency) for the new subcommands.

**Prerequisite:** Plan 1 (`2026-07-23-agentconfigsync-core-sync.md`) must be fully implemented and its tests green. This plan imports `internal/cli`, `internal/syncengine`, `internal/config`, and `internal/gitclient` from Plan 1.

---

## File Structure

```
AgentConfigSync/
  cmd/acsync/
    main.go                   # MODIFY: add daemon / install / uninstall subcommands
  internal/
    syncengine/
      trash.go                # NEW: CleanupTrash(repoDir, now, graceDays)
      trash_test.go
    scheduler/
      scheduler.go            # NEW: State, Scheduler (loop, trigger, pause/resume)
      scheduler_test.go
    autostart/
      autostart.go            # NEW: Manager (per-OS startup entry, pure content)
      autostart_test.go
    daemon/
      daemon.go               # NEW: Daemon wiring scheduler + sync job + cleanup + logging
      daemon_test.go
    cli/
      cleanup.go              # NEW: RunCleanup (load grace, cleanup trash, commit/push)
      cleanup_test.go
      install.go              # NEW: RunInstall / RunUninstall / RunDaemon entrypoints
      install_test.go
```

**Responsibilities & boundaries:**
- `syncengine.CleanupTrash` — pure-ish file op over `.trash/`; the only new repo-mutating logic.
- `scheduler` — pure state machine + loop; no knowledge of sync, git, or agents. Depends only on stdlib.
- `autostart` — computes startup-entry paths + content from `(goos, home, execPath)`; writes files and runs `launchctl`/`systemctl` through an injectable runner.
- `daemon` — orchestration: builds the scheduler, defines the sync job (`RunSync` + `RunCleanup`), owns the log file and signal handling.
- `cli` — thin command handlers (`RunCleanup`, `RunInstall`, `RunUninstall`, `RunDaemon`).

---

## Task 1: Trash grace-period cleanup

**Files:**
- Create: `internal/syncengine/trash.go`
- Test: `internal/syncengine/trash_test.go`

`CleanupTrash` reads `.trash/index.json` (the `map[string]int64` of repo-relative path → deletion unix time written by Plan 1's `Applier.recordTrash`), permanently deletes backing files whose deletion is older than `graceDays`, updates the index, and returns the purged paths. It reuses the `writeFile` test helper defined in Plan 1's `apply_test.go` (same package).

- [ ] **Step 1: Write the failing test**

Create `internal/syncengine/trash_test.go`:

```go
package syncengine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCleanupTrashRemovesExpired(t *testing.T) {
	repo := t.TempDir()
	writeFile(t, filepath.Join(repo, ".trash", "files", "agents", "a", "config", "old.json"), "old")
	writeFile(t, filepath.Join(repo, ".trash", "files", "agents", "a", "sessions", "new.jsonl"), "new")

	now := time.Date(2026, 1, 31, 0, 0, 0, 0, time.UTC)
	idx := map[string]int64{
		"agents/a/config/old.json":    now.AddDate(0, 0, -40).Unix(),
		"agents/a/sessions/new.jsonl": now.AddDate(0, 0, -5).Unix(),
	}
	data, _ := json.MarshalIndent(idx, "", "  ")
	writeFile(t, filepath.Join(repo, ".trash", "index.json"), string(data))

	purged, err := CleanupTrash(repo, now, 30)
	if err != nil {
		t.Fatalf("CleanupTrash error: %v", err)
	}
	if len(purged) != 1 || purged[0] != "agents/a/config/old.json" {
		t.Fatalf("purged = %v, want [agents/a/config/old.json]", purged)
	}
	if _, err := os.Stat(filepath.Join(repo, ".trash", "files", "agents", "a", "config", "old.json")); !os.IsNotExist(err) {
		t.Error("expired file should be removed")
	}
	if _, err := os.Stat(filepath.Join(repo, ".trash", "files", "agents", "a", "sessions", "new.jsonl")); err != nil {
		t.Error("recent file should remain")
	}

	raw, _ := os.ReadFile(filepath.Join(repo, ".trash", "index.json"))
	got := map[string]int64{}
	if err := json.Unmarshal(raw, &got); err != nil {
		t.Fatal(err)
	}
	if _, ok := got["agents/a/config/old.json"]; ok {
		t.Error("expired entry should be gone from index")
	}
	if _, ok := got["agents/a/sessions/new.jsonl"]; !ok {
		t.Error("recent entry should remain in index")
	}
}

func TestCleanupTrashNoIndex(t *testing.T) {
	purged, err := CleanupTrash(t.TempDir(), time.Now(), 30)
	if err != nil {
		t.Fatalf("CleanupTrash error: %v", err)
	}
	if purged != nil {
		t.Errorf("purged = %v, want nil when no trash index", purged)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/syncengine/ -run TestCleanupTrash -v`
Expected: FAIL — `undefined: CleanupTrash`.

- [ ] **Step 3: Write minimal implementation**

Create `internal/syncengine/trash.go`:

```go
package syncengine

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// CleanupTrash permanently removes soft-deleted files whose deletion time is
// older than graceDays. It updates .trash/index.json and deletes the backing
// files under .trash/files/. It returns the repo-relative paths that were
// purged (sorted). A missing trash index is treated as empty (no-op).
func CleanupTrash(repoDir string, now time.Time, graceDays int) ([]string, error) {
	idxPath := filepath.Join(repoDir, ".trash", "index.json")
	data, err := os.ReadFile(idxPath)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	idx := map[string]int64{}
	if len(data) > 0 {
		if err := json.Unmarshal(data, &idx); err != nil {
			return nil, err
		}
	}

	cutoff := now.Add(-time.Duration(graceDays) * 24 * time.Hour).Unix()
	var purged []string
	for repoRel, deletedAt := range idx {
		if deletedAt <= cutoff {
			filePath := filepath.Join(repoDir, ".trash", "files", filepath.FromSlash(repoRel))
			if err := os.Remove(filePath); err != nil && !os.IsNotExist(err) {
				return nil, err
			}
			purged = append(purged, repoRel)
		}
	}
	if len(purged) == 0 {
		return nil, nil
	}

	for _, repoRel := range purged {
		delete(idx, repoRel)
	}
	out, err := json.MarshalIndent(idx, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(idxPath, out, 0o644); err != nil {
		return nil, err
	}

	sort.Strings(purged)
	return purged, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/syncengine/ -run TestCleanupTrash -v`
Expected: PASS (both tests).

- [ ] **Step 5: Commit**

```bash
git add internal/syncengine/trash.go internal/syncengine/trash_test.go
git commit -m "feat: add trash grace-period cleanup"
```

---

## Task 2: Scheduler (interval loop, manual trigger, pause/resume)

**Files:**
- Create: `internal/scheduler/scheduler.go`
- Test: `internal/scheduler/scheduler_test.go`

The scheduler is a pure state machine: it runs `Job` on a ticker and on manual `Trigger`, can be paused/resumed, and reports every transition through `OnState`. It knows nothing about git or agents. Tests are white-box (`package scheduler`) so they can drive `runCycle` directly for determinism and use the loop only for trigger/pause behavior.

- [ ] **Step 1: Write the failing test**

Create `internal/scheduler/scheduler_test.go`:

```go
package scheduler

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type recorder struct {
	mu     sync.Mutex
	states []State
}

func (r *recorder) add(s State) {
	r.mu.Lock()
	r.states = append(r.states, s)
	r.mu.Unlock()
}

func (r *recorder) snapshot() []State {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]State, len(r.states))
	copy(out, r.states)
	return out
}

func waitFor(t *testing.T, cond func() bool, timeout time.Duration, msg string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for %s", msg)
}

func TestRunCycleReportsUpdatingThenIdle(t *testing.T) {
	var runs int64
	rec := &recorder{}
	s := New(time.Hour, func() error {
		atomic.AddInt64(&runs, 1)
		return nil
	})
	s.OnState = rec.add

	s.runCycle()

	if got := atomic.LoadInt64(&runs); got != 1 {
		t.Fatalf("runs = %d, want 1", got)
	}
	states := rec.snapshot()
	if len(states) != 2 || states[0] != StateUpdating || states[1] != StateIdle {
		t.Fatalf("states = %v, want [updating idle]", states)
	}
}

func TestRunCycleReportsErrorOnJobFailure(t *testing.T) {
	rec := &recorder{}
	s := New(time.Hour, func() error { return errors.New("boom") })
	s.OnState = rec.add

	s.runCycle()

	states := rec.snapshot()
	if len(states) != 2 || states[0] != StateUpdating || states[1] != StateError {
		t.Fatalf("states = %v, want [updating error]", states)
	}
}

func TestPausedRunCycleSkips(t *testing.T) {
	var runs int64
	s := New(time.Hour, func() error {
		atomic.AddInt64(&runs, 1)
		return nil
	})
	s.Pause()
	s.runCycle()

	if got := atomic.LoadInt64(&runs); got != 0 {
		t.Fatalf("runs = %d, want 0 while paused", got)
	}
	if s.State() != StatePaused {
		t.Fatalf("state = %v, want paused", s.State())
	}
}

func TestTriggerRunsViaLoop(t *testing.T) {
	var runs int64
	s := New(time.Hour, func() error {
		atomic.AddInt64(&runs, 1)
		return nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.Run(ctx)

	s.Trigger()
	waitFor(t, func() bool { return atomic.LoadInt64(&runs) == 1 }, time.Second, "first run")
}

func TestPauseBlocksTriggerThenResume(t *testing.T) {
	var runs int64
	s := New(time.Hour, func() error {
		atomic.AddInt64(&runs, 1)
		return nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.Run(ctx)

	s.Pause()
	s.Trigger()
	time.Sleep(50 * time.Millisecond)
	if got := atomic.LoadInt64(&runs); got != 0 {
		t.Fatalf("runs = %d, want 0 while paused", got)
	}

	s.Resume()
	s.Trigger()
	waitFor(t, func() bool { return atomic.LoadInt64(&runs) == 1 }, time.Second, "run after resume")
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/scheduler/ -v`
Expected: FAIL — `undefined: New` / `undefined: State`.

- [ ] **Step 3: Write minimal implementation**

Create `internal/scheduler/scheduler.go`:

```go
// Package scheduler runs a job periodically with manual trigger and pause.
package scheduler

import (
	"context"
	"sync"
	"time"
)

// State is the scheduler's current status, surfaced to the UI.
type State int

const (
	StateIdle State = iota
	StateUpdating
	StateError
	StatePaused
)

func (s State) String() string {
	switch s {
	case StateIdle:
		return "idle"
	case StateUpdating:
		return "updating"
	case StateError:
		return "error"
	case StatePaused:
		return "paused"
	default:
		return "unknown"
	}
}

// Job performs one unit of work (a full sync pass). Returning an error moves the
// scheduler into StateError for that cycle.
type Job func() error

// Scheduler runs Job every Interval, with manual Trigger and Pause/Resume.
// OnState (if set) is called on every state transition. All methods are safe
// for concurrent use.
type Scheduler struct {
	Interval time.Duration
	Job      Job
	OnState  func(State)

	mu     sync.Mutex
	paused bool
	state  State

	trigger chan struct{}
}

// New returns a Scheduler that runs job every interval.
func New(interval time.Duration, job Job) *Scheduler {
	return &Scheduler{
		Interval: interval,
		Job:      job,
		state:    StateIdle,
		trigger:  make(chan struct{}, 1),
	}
}

// Trigger requests an immediate run. Non-blocking; coalesces a pending request.
func (s *Scheduler) Trigger() {
	select {
	case s.trigger <- struct{}{}:
	default:
	}
}

// Pause stops periodic and triggered runs until Resume.
func (s *Scheduler) Pause() {
	s.mu.Lock()
	s.paused = true
	s.mu.Unlock()
	s.setState(StatePaused)
}

// Resume re-enables runs.
func (s *Scheduler) Resume() {
	s.mu.Lock()
	s.paused = false
	s.mu.Unlock()
	s.setState(StateIdle)
}

// State returns the current state.
func (s *Scheduler) State() State {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state
}

func (s *Scheduler) isPaused() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.paused
}

func (s *Scheduler) setState(st State) {
	s.mu.Lock()
	s.state = st
	cb := s.OnState
	s.mu.Unlock()
	if cb != nil {
		cb(st)
	}
}

// runCycle runs the job once and updates state, unless paused.
func (s *Scheduler) runCycle() {
	if s.isPaused() {
		return
	}
	s.setState(StateUpdating)
	err := s.Job()
	if s.isPaused() {
		return // a Pause arrived during the run; keep the paused state
	}
	if err != nil {
		s.setState(StateError)
		return
	}
	s.setState(StateIdle)
}

// Run blocks executing the schedule until ctx is cancelled.
func (s *Scheduler) Run(ctx context.Context) {
	ticker := time.NewTicker(s.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.runCycle()
		case <-s.trigger:
			s.runCycle()
		}
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/scheduler/ -v`
Expected: PASS (all five tests).

- [ ] **Step 5: Commit**

```bash
git add internal/scheduler/
git commit -m "feat: add scheduler with interval loop, trigger, and pause"
```

---

## Task 3: Autostart (Windows Startup / macOS LaunchAgent / Linux systemd)

**Files:**
- Create: `internal/autostart/autostart.go`
- Test: `internal/autostart/autostart_test.go`

One `Manager` handles all three platforms with no build tags: it computes the entry directory/filename from `(goos, home)`, generates the file body with pure functions, and — on macOS/Linux — registers with `launchctl`/`systemctl` through an injectable `Runner` (nil on Windows). Everything is testable on any OS.

- [ ] **Step 1: Write the failing test**

Create `internal/autostart/autostart_test.go`:

```go
package autostart

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewWindowsManager(t *testing.T) {
	t.Setenv("APPDATA", `C:\Users\alice\AppData\Roaming`)
	m, err := New("windows", `C:\Users\alice`)
	if err != nil {
		t.Fatal(err)
	}
	wantDir := filepath.Join(`C:\Users\alice\AppData\Roaming`, "Microsoft", "Windows", "Start Menu", "Programs", "Startup")
	if m.Dir != wantDir {
		t.Errorf("dir = %q, want %q", m.Dir, wantDir)
	}
	if m.File != "acsync.cmd" {
		t.Errorf("file = %q", m.File)
	}
}

func TestNewUnsupportedOS(t *testing.T) {
	if _, err := New("plan9", "/home/x"); err == nil {
		t.Fatal("expected error for unsupported OS")
	}
}

func TestContentGenerators(t *testing.T) {
	if got := windowsCmd(`C:\acsync.exe`); !strings.Contains(got, `start "" "C:\acsync.exe" daemon`) {
		t.Errorf("windows cmd = %q", got)
	}
	plist := launchAgentPlist("/usr/local/bin/acsync")
	if !strings.Contains(plist, "<string>/usr/local/bin/acsync</string>") || !strings.Contains(plist, "com.acsync.agent") {
		t.Errorf("plist = %q", plist)
	}
	unit := systemdUnit("/usr/local/bin/acsync")
	if !strings.Contains(unit, "ExecStart=/usr/local/bin/acsync daemon") {
		t.Errorf("unit = %q", unit)
	}
}

func TestEnableDisableLinux(t *testing.T) {
	home := t.TempDir()
	m, err := New("linux", home)
	if err != nil {
		t.Fatal(err)
	}
	var calls [][]string
	m.Run = func(name string, args ...string) error {
		calls = append(calls, append([]string{name}, args...))
		return nil
	}

	if err := m.Enable("/opt/acsync"); err != nil {
		t.Fatal(err)
	}
	if en, _ := m.IsEnabled(); !en {
		t.Error("should be enabled")
	}
	data, _ := os.ReadFile(m.Path())
	if !strings.Contains(string(data), "ExecStart=/opt/acsync daemon") {
		t.Errorf("unit body = %q", string(data))
	}
	if len(calls) != 1 || calls[0][0] != "systemctl" {
		t.Errorf("register calls = %v", calls)
	}

	if err := m.Disable(); err != nil {
		t.Fatal(err)
	}
	if en, _ := m.IsEnabled(); en {
		t.Error("should be disabled after Disable")
	}
}

func TestEnableWindowsWritesCmd(t *testing.T) {
	home := t.TempDir()
	t.Setenv("APPDATA", filepath.Join(home, "AppData", "Roaming"))
	m, err := New("windows", home)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Enable(`C:\acsync.exe`); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(m.Path())
	if !strings.Contains(string(data), `"C:\acsync.exe" daemon`) {
		t.Errorf("cmd body = %q", string(data))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/autostart/ -v`
Expected: FAIL — `undefined: New` / `undefined: windowsCmd`.

- [ ] **Step 3: Write minimal implementation**

Create `internal/autostart/autostart.go`:

```go
// Package autostart manages an OS-appropriate "run at login" entry.
package autostart

import (
	"encoding/xml"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const label = "com.acsync.agent"

// Runner executes an external command (launchctl/systemctl). Injectable for tests.
type Runner func(name string, args ...string) error

// Manager writes and removes an OS-appropriate autostart entry.
type Manager struct {
	GOOS string
	Dir  string // directory holding the entry
	File string // entry filename
	Run  Runner // used on darwin/linux to (de)register; nil on windows
}

// New builds a Manager for goos using home as the base for user directories.
func New(goos, home string) (*Manager, error) {
	m := &Manager{GOOS: goos, Run: execRunner}
	switch goos {
	case "windows":
		appData := os.Getenv("APPDATA")
		if appData == "" {
			appData = filepath.Join(home, "AppData", "Roaming")
		}
		m.Dir = filepath.Join(appData, "Microsoft", "Windows", "Start Menu", "Programs", "Startup")
		m.File = "acsync.cmd"
		m.Run = nil
	case "darwin":
		m.Dir = filepath.Join(home, "Library", "LaunchAgents")
		m.File = label + ".plist"
	case "linux":
		m.Dir = filepath.Join(home, ".config", "systemd", "user")
		m.File = "acsync.service"
	default:
		return nil, fmt.Errorf("autostart: unsupported OS %q", goos)
	}
	return m, nil
}

// Path returns the full path of the autostart entry file.
func (m *Manager) Path() string { return filepath.Join(m.Dir, m.File) }

func (m *Manager) content(execPath string) string {
	switch m.GOOS {
	case "windows":
		return windowsCmd(execPath)
	case "darwin":
		return launchAgentPlist(execPath)
	case "linux":
		return systemdUnit(execPath)
	default:
		return ""
	}
}

// Enable writes the autostart entry pointing at execPath and, on darwin/linux,
// registers it with the user service manager.
func (m *Manager) Enable(execPath string) error {
	if err := os.MkdirAll(m.Dir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(m.Path(), []byte(m.content(execPath)), 0o644); err != nil {
		return err
	}
	if m.Run == nil {
		return nil
	}
	switch m.GOOS {
	case "darwin":
		return m.Run("launchctl", "load", "-w", m.Path())
	case "linux":
		return m.Run("systemctl", "--user", "enable", "acsync.service")
	}
	return nil
}

// Disable removes the autostart entry (and unregisters it on darwin/linux).
func (m *Manager) Disable() error {
	if m.Run != nil {
		switch m.GOOS {
		case "darwin":
			_ = m.Run("launchctl", "unload", "-w", m.Path())
		case "linux":
			_ = m.Run("systemctl", "--user", "disable", "acsync.service")
		}
	}
	err := os.Remove(m.Path())
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// IsEnabled reports whether the autostart entry file exists.
func (m *Manager) IsEnabled() (bool, error) {
	_, err := os.Stat(m.Path())
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func execRunner(name string, args ...string) error {
	return exec.Command(name, args...).Run()
}

func windowsCmd(execPath string) string {
	return "@echo off\r\nstart \"\" \"" + execPath + "\" daemon\r\n"
}

func launchAgentPlist(execPath string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>` + label + `</string>
	<key>ProgramArguments</key>
	<array>
		<string>` + xmlEscape(execPath) + `</string>
		<string>daemon</string>
	</array>
	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	<true/>
</dict>
</plist>
`
}

func systemdUnit(execPath string) string {
	return `[Unit]
Description=AgentConfigSync daemon
After=network-online.target

[Service]
ExecStart=` + execPath + ` daemon
Restart=on-failure

[Install]
WantedBy=default.target
`
}

func xmlEscape(s string) string {
	var b strings.Builder
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/autostart/ -v`
Expected: PASS (all five tests).

- [ ] **Step 5: Commit**

```bash
git add internal/autostart/
git commit -m "feat: add cross-platform autostart manager"
```

---

## Task 4: `cli.RunCleanup` — purge trash and publish

**Files:**
- Create: `internal/cli/cleanup.go`
- Test: `internal/cli/cleanup_test.go`

`RunCleanup` loads the config for `TrashGraceDays` (defaulting to 30), runs `syncengine.CleanupTrash`, and — if anything was purged — commits and pushes with a small pull-rebase retry. The test reuses `bareRemote`/`gitCmd` from Plan 1's `sync_test.go` (same package).

- [ ] **Step 1: Write the failing test**

Create `internal/cli/cleanup_test.go`:

```go
package cli

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/qinqingxu/acsync/internal/config"
	"github.com/qinqingxu/acsync/internal/gitclient"
)

func gitOut(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

func TestRunCleanupPurgesExpiredAndCommits(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	home := filepath.Join(t.TempDir(), ".acsync")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	remote := bareRemote(t)
	repo := RepoDir(home)

	client := &gitclient.Client{Dir: repo}
	if err := client.Clone(remote, repo); err != nil {
		t.Fatal(err)
	}
	gitCmd(t, repo, "config", "user.email", "t@e.com")
	gitCmd(t, repo, "config", "user.name", "t")

	// Seed a trashed file as if a prior sync recorded it, then commit+push.
	trashFile := filepath.Join(repo, ".trash", "files", "agents", "a", "config", "old.json")
	if err := os.MkdirAll(filepath.Dir(trashFile), 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(trashFile, []byte("old"), 0o644)
	idx := map[string]int64{"agents/a/config/old.json": time.Now().AddDate(0, 0, -40).Unix()}
	data, _ := json.MarshalIndent(idx, "", "  ")
	os.WriteFile(filepath.Join(repo, ".trash", "index.json"), data, 0o644)
	gitCmd(t, repo, "add", ".")
	gitCmd(t, repo, "commit", "-m", "seed trash")
	gitCmd(t, repo, "push", "origin", "main")

	if err := config.Save(ConfigPath(home), config.Config{
		RepoURL:        remote,
		TrashGraceDays: 30,
		Agents:         map[string]bool{},
	}); err != nil {
		t.Fatal(err)
	}

	purged, err := RunCleanup(home, time.Now())
	if err != nil {
		t.Fatalf("RunCleanup error: %v", err)
	}
	if len(purged) != 1 || purged[0] != "agents/a/config/old.json" {
		t.Fatalf("purged = %v", purged)
	}
	if _, err := os.Stat(trashFile); !os.IsNotExist(err) {
		t.Error("expired trash file should be gone")
	}
	if s := strings.TrimSpace(gitOut(t, repo, "status", "--porcelain")); s != "" {
		t.Errorf("worktree should be clean after cleanup commit, got %q", s)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/cli/ -run TestRunCleanup -v`
Expected: FAIL — `undefined: RunCleanup`.

- [ ] **Step 3: Write minimal implementation**

Create `internal/cli/cleanup.go`:

```go
package cli

import (
	"fmt"
	"time"

	"github.com/qinqingxu/acsync/internal/config"
	"github.com/qinqingxu/acsync/internal/gitclient"
	"github.com/qinqingxu/acsync/internal/syncengine"
)

// RunCleanup purges expired trash from the repo and commits/pushes if anything
// was removed. It returns the purged repo-relative paths.
func RunCleanup(home string, now time.Time) ([]string, error) {
	cfg, err := config.Load(ConfigPath(home))
	if err != nil {
		return nil, err
	}
	grace := cfg.TrashGraceDays
	if grace <= 0 {
		grace = 30
	}

	repo := RepoDir(home)
	purged, err := syncengine.CleanupTrash(repo, now, grace)
	if err != nil {
		return nil, err
	}
	if len(purged) == 0 {
		return nil, nil
	}

	client := &gitclient.Client{Dir: repo}
	if err := client.AddAll(); err != nil {
		return purged, err
	}
	changed, err := client.HasChanges()
	if err != nil {
		return purged, err
	}
	if !changed {
		return purged, nil
	}
	if err := client.Commit(fmt.Sprintf("chore: purge %d expired trash entries", len(purged))); err != nil {
		return purged, err
	}
	if err := pushWithRebase(client, 3); err != nil {
		return purged, err
	}
	return purged, nil
}

// pushWithRebase pushes, and on failure pull-rebases and retries up to attempts.
func pushWithRebase(client *gitclient.Client, attempts int) error {
	var err error
	for i := 0; i < attempts; i++ {
		if err = client.Push(); err == nil {
			return nil
		}
		if rerr := client.PullRebase(); rerr != nil {
			return rerr
		}
	}
	return err
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/cli/ -run TestRunCleanup -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/cli/cleanup.go internal/cli/cleanup_test.go
git commit -m "feat: add cli cleanup command for expired trash"
```

---

## Task 5: `cli.RunInstall` / `RunUninstall` — autostart wrappers

**Files:**
- Create: `internal/cli/install.go`
- Test: `internal/cli/install_test.go`

Thin handlers that build an `autostart.Manager` from `(goos, userHome)` and enable/disable the entry for a given executable path. `userHome` and `execPath` are passed in (computed in `main.go` from `os.UserHomeDir()` / `os.Executable()`), keeping these functions pure and testable. The test uses `goos="windows"` so no external service manager is invoked.

- [ ] **Step 1: Write the failing test**

Create `internal/cli/install_test.go`:

```go
package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunInstallUninstallWindows(t *testing.T) {
	userHome := t.TempDir()
	t.Setenv("APPDATA", filepath.Join(userHome, "AppData", "Roaming"))

	path, err := RunInstall("windows", userHome, `C:\Program Files\acsync\acsync.exe`)
	if err != nil {
		t.Fatalf("RunInstall error: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("entry not written: %v", err)
	}
	if !strings.Contains(string(data), `"C:\Program Files\acsync\acsync.exe" daemon`) {
		t.Errorf("entry body = %q", string(data))
	}

	if err := RunUninstall("windows", userHome); err != nil {
		t.Fatalf("RunUninstall error: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("entry should be removed after uninstall")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/cli/ -run TestRunInstallUninstall -v`
Expected: FAIL — `undefined: RunInstall`.

- [ ] **Step 3: Write minimal implementation**

Create `internal/cli/install.go`:

```go
package cli

import "github.com/qinqingxu/acsync/internal/autostart"

// RunInstall enables run-at-login for execPath and returns the entry path.
func RunInstall(goos, userHome, execPath string) (string, error) {
	m, err := autostart.New(goos, userHome)
	if err != nil {
		return "", err
	}
	if err := m.Enable(execPath); err != nil {
		return "", err
	}
	return m.Path(), nil
}

// RunUninstall removes the run-at-login entry.
func RunUninstall(goos, userHome string) error {
	m, err := autostart.New(goos, userHome)
	if err != nil {
		return err
	}
	return m.Disable()
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/cli/ -run TestRunInstallUninstall -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/cli/install.go internal/cli/install_test.go
git commit -m "feat: add cli install/uninstall for autostart"
```

---

## Task 6: Daemon (scheduler + sync job + cleanup + logging)

**Files:**
- Create: `internal/daemon/daemon.go`
- Test: `internal/daemon/daemon_test.go`

The `Daemon` reads the configured interval, builds a `scheduler.Scheduler` whose job runs one `cli.RunSync` followed by `cli.RunCleanup`, logs to `~/.acsync/logs/daemon.log`, triggers an immediate sync on startup, and runs until its context is cancelled. It exposes `Scheduler` so Plan 3's tray can observe state and drive `Trigger`/`Pause`/`Resume`.

- [ ] **Step 1: Write the failing test**

Create `internal/daemon/daemon_test.go`:

```go
package daemon

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/qinqingxu/acsync/internal/cli"
	"github.com/qinqingxu/acsync/internal/config"
)

func writeConfig(t *testing.T, home string, cfg config.Config) {
	t.Helper()
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := config.Save(cli.ConfigPath(home), cfg); err != nil {
		t.Fatal(err)
	}
}

func TestNewUsesConfiguredInterval(t *testing.T) {
	home := filepath.Join(t.TempDir(), ".acsync")
	writeConfig(t, home, config.Config{SyncIntervalMinutes: 3, Agents: map[string]bool{}})

	d, err := New(home, runtime.GOOS)
	if err != nil {
		t.Fatal(err)
	}
	if d.Scheduler.Interval != 3*time.Minute {
		t.Errorf("interval = %v, want 3m", d.Scheduler.Interval)
	}
}

func TestNewDefaultsIntervalTo10m(t *testing.T) {
	home := filepath.Join(t.TempDir(), ".acsync")
	writeConfig(t, home, config.Config{SyncIntervalMinutes: 0, Agents: map[string]bool{}})

	d, err := New(home, runtime.GOOS)
	if err != nil {
		t.Fatal(err)
	}
	if d.Scheduler.Interval != 10*time.Minute {
		t.Errorf("interval = %v, want 10m", d.Scheduler.Interval)
	}
}

func TestRunStopsOnContextCancelAndLogs(t *testing.T) {
	home := filepath.Join(t.TempDir(), ".acsync")
	// Long interval so only the startup trigger fires; the sync job may error
	// (no repo) — the lifecycle must still complete cleanly.
	writeConfig(t, home, config.Config{SyncIntervalMinutes: 60, Agents: map[string]bool{}})

	d, err := New(home, runtime.GOOS)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- d.Run(ctx) }()

	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not stop after context cancel")
	}

	data, err := os.ReadFile(filepath.Join(cli.LogsDir(home), "daemon.log"))
	if err != nil {
		t.Fatalf("daemon log missing: %v", err)
	}
	if !strings.Contains(string(data), "daemon started") {
		t.Errorf("log missing startup line: %s", data)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/daemon/ -v`
Expected: FAIL — `undefined: New`.

- [ ] **Step 3: Write minimal implementation**

Create `internal/daemon/daemon.go`:

```go
// Package daemon runs the acsync sync scheduler as a long-lived service.
package daemon

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/qinqingxu/acsync/internal/cli"
	"github.com/qinqingxu/acsync/internal/config"
	"github.com/qinqingxu/acsync/internal/scheduler"
)

// Daemon runs the sync scheduler for a given acsync home.
type Daemon struct {
	Home      string
	GOOS      string
	Scheduler *scheduler.Scheduler
	Logger    *log.Logger

	closeLog func() error
}

// New builds a Daemon: it loads config for the interval and wires a scheduler
// whose job runs one sync pass followed by trash cleanup.
func New(home, goos string) (*Daemon, error) {
	cfg, err := config.Load(cli.ConfigPath(home))
	if err != nil {
		return nil, err
	}
	interval := time.Duration(cfg.SyncIntervalMinutes) * time.Minute
	if interval <= 0 {
		interval = 10 * time.Minute
	}

	logger, closeLog, err := newLogger(home)
	if err != nil {
		return nil, err
	}

	d := &Daemon{Home: home, GOOS: goos, Logger: logger, closeLog: closeLog}
	d.Scheduler = scheduler.New(interval, d.syncJob)
	d.Scheduler.OnState = d.logState
	return d, nil
}

// syncJob runs one full sync followed by a trash cleanup pass.
func (d *Daemon) syncJob() error {
	res, err := cli.RunSync(d.Home, d.GOOS)
	if err != nil {
		d.Logger.Printf("sync error: %v", err)
		return err
	}
	d.Logger.Printf("sync ok: %d actions, %d blocked, pushed=%v",
		len(res.Actions), len(res.Blocked), res.Pushed)

	purged, err := cli.RunCleanup(d.Home, time.Now())
	if err != nil {
		d.Logger.Printf("cleanup error: %v", err)
		return err
	}
	if len(purged) > 0 {
		d.Logger.Printf("purged %d expired trash entries", len(purged))
	}
	return nil
}

func (d *Daemon) logState(s scheduler.State) {
	d.Logger.Printf("state: %s", s)
}

// Run triggers an initial sync then runs the scheduler until ctx is cancelled.
func (d *Daemon) Run(ctx context.Context) error {
	d.Logger.Printf("daemon started (home=%s, interval=%s)", d.Home, d.Scheduler.Interval)
	d.Scheduler.Trigger() // sync promptly on startup
	d.Scheduler.Run(ctx)
	d.Logger.Printf("daemon stopped")
	if d.closeLog != nil {
		return d.closeLog()
	}
	return nil
}

func newLogger(home string) (*log.Logger, func() error, error) {
	dir := cli.LogsDir(home)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, nil, err
	}
	f, err := os.OpenFile(filepath.Join(dir, "daemon.log"),
		os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, nil, err
	}
	return log.New(f, "", log.LstdFlags), f.Close, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/daemon/ -v`
Expected: PASS (all three tests).

- [ ] **Step 5: Commit**

```bash
git add internal/daemon/
git commit -m "feat: add daemon wiring scheduler, sync, and cleanup"
```

---

## Task 7: Wire `daemon` / `install` / `uninstall` commands and verify

**Files:**
- Create: `internal/daemon/run.go`
- Modify: `cmd/acsync/main.go` (add three subcommands to the Plan 1 root)

- [ ] **Step 1: Add signal-aware run helper**

Create `internal/daemon/run.go`:

```go
package daemon

import (
	"context"
	"os"
	"os/signal"
	"syscall"
)

// RunWithSignals builds a daemon for home and runs it until SIGINT/SIGTERM.
func RunWithSignals(home, goos string) error {
	d, err := New(home, goos)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	return d.Run(ctx)
}
```

- [ ] **Step 2: Replace `cmd/acsync/main.go`**

Replace the entire contents of `cmd/acsync/main.go` (extends the Plan 1 version with `daemon`, `install`, `uninstall`):

```go
package main

import (
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/qinqingxu/acsync/internal/cli"
	"github.com/qinqingxu/acsync/internal/daemon"
	"github.com/qinqingxu/acsync/internal/gitclient"
	"github.com/spf13/cobra"
)

func main() {
	root := &cobra.Command{
		Use:   "acsync",
		Short: "Sync AI agent config and session files across machines via a private GitHub repo",
	}
	root.AddCommand(initCmd(), syncCmd(), statusCmd(), daemonCmd(), installCmd(), uninstallCmd())
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

func daemonCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "daemon",
		Short: "Run the sync daemon (periodic sync + trash cleanup) until stopped",
		RunE: func(cmd *cobra.Command, args []string) error {
			home, err := cli.Home()
			if err != nil {
				return err
			}
			return daemon.RunWithSignals(home, runtime.GOOS)
		},
	}
}

func installCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "install",
		Short: "Enable acsync to start automatically at login",
		RunE: func(cmd *cobra.Command, args []string) error {
			exe, err := os.Executable()
			if err != nil {
				return err
			}
			userHome, err := os.UserHomeDir()
			if err != nil {
				return err
			}
			path, err := cli.RunInstall(runtime.GOOS, userHome, exe)
			if err != nil {
				return err
			}
			fmt.Printf("Autostart enabled: %s\n", path)
			return nil
		},
	}
}

func uninstallCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "uninstall",
		Short: "Disable acsync autostart",
		RunE: func(cmd *cobra.Command, args []string) error {
			userHome, err := os.UserHomeDir()
			if err != nil {
				return err
			}
			if err := cli.RunUninstall(runtime.GOOS, userHome); err != nil {
				return err
			}
			fmt.Println("Autostart disabled")
			return nil
		},
	}
}
```

- [ ] **Step 3: Tidy, vet, and build**

Run: `go mod tidy && go vet ./... && go build ./...`
Expected: no output (success).

- [ ] **Step 4: Run the full test suite**

Run: `go test ./...`
Expected: all packages `ok`, including `internal/scheduler`, `internal/autostart`, `internal/daemon`, and the new `internal/cli` and `internal/syncengine` tests.

- [ ] **Step 5: Verify the new commands are registered**

Run: `go run ./cmd/acsync --help`
Expected: usage lists `init`, `sync`, `status`, `daemon`, `install`, `uninstall`.

- [ ] **Step 6: Smoke-test the daemon starts and stops**

Run (PowerShell — starts the daemon, waits, then stops it):

```powershell
$p = Start-Process -FilePath go -ArgumentList 'run','./cmd/acsync','daemon' -PassThru -NoNewWindow
Start-Sleep -Seconds 3
Stop-Process -Id $p.Id
Get-Content "$env:USERPROFILE\.acsync\logs\daemon.log" -Tail 5
```

Expected: `daemon.log` contains a `daemon started` line (a sync error line is fine if no repo is configured yet).

- [ ] **Step 7: Cross-compile smoke test for all three platforms**

Run (PowerShell):

```powershell
$env:GOOS="windows"; go build -o dist/acsync-windows.exe ./cmd/acsync
$env:GOOS="darwin";  go build -o dist/acsync-darwin ./cmd/acsync
$env:GOOS="linux";   go build -o dist/acsync-linux ./cmd/acsync
Remove-Item Env:\GOOS
```

Expected: three binaries produced with no build errors.

- [ ] **Step 8: Commit**

```bash
git add internal/daemon/run.go cmd/acsync/main.go
git commit -m "feat: wire daemon, install, and uninstall commands"
```

---

## Done Criteria (Plan 2)

- `go test ./...` passes for all Plan 1 + Plan 2 packages.
- `acsync daemon` runs a full sync immediately, then every configured interval (default 10 min), and also purges trash older than the grace period; it logs to `~/.acsync/logs/daemon.log` and stops cleanly on Ctrl+C / SIGTERM.
- `acsync install` writes an OS-appropriate autostart entry (Windows Startup `.cmd`, macOS LaunchAgent plist, Linux systemd user unit) pointing at `acsync daemon`; `acsync uninstall` removes it.
- The scheduler supports pause/resume and manual trigger, and reports Idle/Updating/Error/Paused via `Scheduler.OnState` — the seam Plan 3's tray consumes.
- Binaries cross-compile for Windows, macOS, and Linux.

**Out of scope (Plan 3):** `tray` (system tray + status icons, per-agent toggle menu) and `settings` (local web settings UI). The tray will import `internal/daemon` + `internal/scheduler`, build a `daemon.Daemon`, set `Scheduler.OnState` to update the icon, and wire menu items to `Trigger`/`Pause`/`Resume` and per-agent `config` writes.

**Deliberately deferred spec details (tracked, not lost):**
- **Write-in-progress static period** (spec §5.5: skip files whose mtime changed in the last few seconds): the collect step in Plan 1 mirrors files as-is. Adding a "settling" filter is a small, isolated change to `syncengine.collectOne` and is deferred to avoid reopening frozen Plan 1 code here; the 10-minute cadence makes mid-write capture rare, and the next cycle self-heals.
- **Error-state surfacing to the user** beyond the log file (spec §9 mentions the tray turning red) is delivered in Plan 3 via `Scheduler.OnState` → tray icon.
