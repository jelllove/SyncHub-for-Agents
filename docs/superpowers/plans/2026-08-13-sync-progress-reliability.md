# Sync Progress and Reliability Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Make desktop synchronization collect the real agent files, run without Windows console flashes, and show accurate staged progress and completion results.

**Architecture:** Separate the application data directory from the OS user home at the CLI runtime boundary. Add a platform-specific subprocess configurator used by Git and SSH. Publish typed progress from the sync engine through daemon cycle callbacks into the existing desktop snapshot event and render it in React.

**Tech Stack:** Go 1.26, Wails v3, React 19, TypeScript, Vitest-free TypeScript build validation, Git CLI, PowerShell smoke tests.

---

### Task 1: Resolve providers from the real user profile

**Files:**
- Modify: `internal/cli/runtime.go`
- Modify: `internal/cli/sync.go`
- Test: `internal/cli/runtime_test.go`
- Test: `internal/cli/sync_test.go`

- [ ] **Step 1: Write the failing runtime test**

Add a test that passes separate values for `dataHome` and `userHome`, builds the
Claude spec, and asserts its root is `C:\Users\alice\.claude`, never
`C:\Users\alice\.acsync\.claude`.

- [ ] **Step 2: Run the regression test and verify RED**

Run:

```powershell
go test ./internal/cli -run "TestBuildSpecsUsesUserHome" -count=1
```

Expected: FAIL because `RunSync`/`BuildSpecs` currently use the acsync data
directory as the template home.

- [ ] **Step 3: Separate runtime paths**

Change `BuildSpecs` to accept `userHome` explicitly. In `RunSync`, call
`os.UserHomeDir()` and pass that value to `BuildSpecs`; continue passing the
acsync data directory to config, repository, state, and provider override
functions.

- [ ] **Step 4: Verify GREEN**

Run:

```powershell
go test ./internal/cli -run "TestBuildSpecs|TestRunSync" -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```powershell
git add internal/cli/runtime.go internal/cli/runtime_test.go internal/cli/sync.go internal/cli/sync_test.go
git commit -m "fix: resolve agent paths from user profile"
```

### Task 2: Hide Git and SSH subprocess windows

**Files:**
- Create: `internal/processattr/processattr_windows.go`
- Create: `internal/processattr/processattr_other.go`
- Create: `internal/processattr/processattr_windows_test.go`
- Modify: `internal/gitclient/gitclient.go`
- Modify: `internal/sshprobe/probe.go`
- Test: `internal/gitclient/gitclient_test.go`

- [ ] **Step 1: Write the failing Windows process test**

Construct an `exec.Cmd`, call the wished-for `processattr.HideWindow` helper,
and assert `SysProcAttr.CreationFlags` contains
`windows.CREATE_NO_WINDOW`.

- [ ] **Step 2: Run the Windows test and verify RED**

Run:

```powershell
go test ./internal/processattr -count=1
```

Expected: FAIL because the package/helper does not exist.

- [ ] **Step 3: Implement and wire the platform helper**

On Windows set:

```go
cmd.SysProcAttr = &syscall.SysProcAttr{
    CreationFlags: windows.CREATE_NO_WINDOW,
}
```

On non-Windows, make `HideWindow` a no-op. Call it for every command produced by
`gitclient.Client.command` and `sshprobe.run`.

- [ ] **Step 4: Verify GREEN**

Run:

```powershell
go test ./internal/processattr ./internal/gitclient ./internal/sshprobe -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```powershell
git add internal/processattr internal/gitclient internal/sshprobe
git commit -m "fix: hide desktop subprocess windows"
```

### Task 3: Publish and render structured sync progress

**Files:**
- Modify: `internal/syncengine/engine.go`
- Create: `internal/syncengine/models.go`
- Test: `internal/syncengine/engine_test.go`
- Modify: `internal/daemon/daemon.go`
- Test: `internal/daemon/daemon_test.go`
- Modify: `internal/desktop/models.go`
- Modify: `internal/desktop/service.go`
- Test: `internal/desktop/service_test.go`
- Modify: `frontend/src/App.tsx`
- Modify: `frontend/src/style.css`
- Regenerate: `frontend/bindings/github.com/qinqingxu/acsync/internal/desktop/models.ts`

- [ ] **Step 1: Write failing engine progress tests**

Record progress callbacks during a real temporary-repository sync. Assert the
ordered stages are `pulling`, `scanning`, `comparing`, `applying`, `uploading`,
and `complete`; percentages are monotonic; final counts equal the produced
actions and blocked files.

- [ ] **Step 2: Run engine progress tests and verify RED**

Run:

```powershell
go test ./internal/syncengine -run "TestEnginePublishesProgress" -count=1
```

Expected: FAIL because `Engine` has no progress callback.

- [ ] **Step 3: Add typed progress and daemon forwarding**

Define a `Progress` model with stage, label, percentage, completed actions,
total actions, and blocked files. Emit at each engine boundary. Add
`OnProgress` to `daemon.Daemon`; protect current progress in
`desktop.Service` and include it in `desktop.Snapshot`.

- [ ] **Step 4: Verify backend progress tests GREEN**

Run:

```powershell
go test ./internal/syncengine ./internal/daemon ./internal/desktop -count=1
```

Expected: PASS.

- [ ] **Step 5: Render determinate desktop progress**

Render the snapshot stage label, percentage bar, and action counter while state
is `updating`. Replace “Synchronization started” with “Synchronization queued”
until the first updating snapshot. On completion display either “Uploaded N
changes”, “Synchronization complete; no changes needed”, or the backend error.

- [ ] **Step 6: Regenerate bindings and build frontend**

Run:

```powershell
wails3 generate bindings -clean=true -ts -i
Set-Location frontend
npm run build
```

Expected: TypeScript and Vite production build PASS.

- [ ] **Step 7: Commit**

```powershell
git add internal/syncengine internal/daemon internal/desktop frontend
git commit -m "feat: show structured synchronization progress"
```

### Task 4: Validate the installed application end to end

**Files:**
- Modify: `scripts/smoke/windows.ps1`
- Test: existing Go and frontend suites

- [ ] **Step 1: Extend the smoke test**

Run the installed app against an isolated user profile and temporary bare Git
remote containing representative Claude settings/session files. Trigger one
sync, wait until the repository receives an `agents/claude` commit, and assert
no child process has a visible console window.

- [ ] **Step 2: Run complete validation**

Run:

```powershell
go test ./...
go vet ./...
Set-Location frontend
npm run build
Set-Location ..
wails3 package GOOS=windows ARCH=amd64 INSTALL_SCOPE=user
.\scripts\smoke\windows.ps1 -Installer .\bin\AgentConfigSync-amd64-installer.exe
```

Expected: every command PASS and the smoke remote contains synchronized files.

- [ ] **Step 3: Install and verify the user repository**

Install the new package for the current user, start it, trigger Sync Now, and
wait for the dashboard to report completion. Verify
`~/.acsync/repo/agents` contains collected files and `git log` shows a new sync
commit pushed to `origin/main`.

- [ ] **Step 4: Commit**

```powershell
git add scripts/smoke/windows.ps1
git commit -m "test: cover installed desktop synchronization"
```
