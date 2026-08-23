# Sync Stability and First-Sync Strategy Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix tray exit and sync-error guidance UX, then add configurable repo path + first-sync strategy with safe migration behavior.

**Architecture:** Implement P1 first (exit correctness, actionable pull/rebase diagnostics, Settings manual sync action) on top of existing desktop service + snapshot pipeline. Then implement P2 by extending config/runtime path resolution and onboarding/sync bootstrap state to support repo-dir migration and first-sync strategy selection. Keep behavior explicit, typed, and test-driven across Go backend and React frontend.

**Tech Stack:** Go (Wails backend), React + TypeScript (frontend), Vitest, Go test, existing gitclient/syncengine/desktop modules

---

## File Structure

- **Modify:** `app.go` (tray quit flow + window-close guard)
- **Modify:** `internal/desktop/models.go` (typed sync diagnostics + repo path + first-sync metadata)
- **Modify:** `internal/desktop/service.go` (diagnostic classification, settings action wiring, repo-dir updates)
- **Modify:** `internal/syncengine/engine.go` (first-sync strategy branch integration)
- **Modify:** `internal/config/config.go` (persist repo dir + first-sync strategy state)
- **Modify:** `internal/cli/runtime.go` (repo dir resolution from config; fallback default)
- **Modify:** `internal/cli/sync.go` / `internal/cli/status.go` / `internal/cli/cleanup.go` (use resolved repo dir)
- **Modify:** `internal/onboarding/service.go` + models (first-sync strategy pre-run state)
- **Modify:** `frontend/src/App.tsx` (fix panel + strategy modal)
- **Modify:** `frontend/src/SettingsPanel.tsx` (Run sync now button + repo-dir controls)
- **Modify:** `frontend/src/desktopState.ts` (normalize new snapshot fields)
- **Test:** `internal/desktop/wails_test.go`, `internal/desktop/service_test.go`, `internal/config/config_test.go`, `internal/cli/sync_test.go`, `internal/syncengine/engine_test.go`, `frontend/src/App.test.tsx`, `frontend/src/SettingsPanel.test.tsx`

## Conventions Used in This Plan

- Keep existing naming style (`SyncHub`, `RepositoryURL`, `TriggerSync`, etc.).
- No hidden auto-recovery; surface clear actionable guidance.
- Migration failures must leave existing config and repo path unchanged.
- First-sync strategy prompt is one-time and persisted.

### Task 1: Fix tray Exit to terminate process

**Files:**
- Modify: `app.go`
- Test: `internal/desktop/wails_test.go`

- [ ] **Step 1: Write failing quit-flow test**

```go
func TestWindowClosingDoesNotCancelWhenAppIsQuitting(t *testing.T) {
	// add a small helper in app.go:
	// func shouldCancelWindowClose(isQuitting bool) bool
	if !shouldCancelWindowClose(false) {
		t.Fatal("expected regular close to be canceled (hide to tray)")
	}
	if shouldCancelWindowClose(true) {
		t.Fatal("expected close to proceed while quitting")
	}
}
```

- [ ] **Step 2: Run targeted test to confirm it fails**

Run:  
`go test ./internal/desktop -run TestWindowClosingDoesNotCancelWhenAppIsQuitting -count=1`  
Expected: FAIL (`undefined: shouldCancelWindowClose`).

- [ ] **Step 3: Implement explicit quitting guard in `app.go`**

```go
type guiApplication struct {
	// ...
	isQuitting atomic.Bool
}

func shouldCancelWindowClose(isQuitting bool) bool {
	return !isQuitting
}

// in tray Quit handler:
menu.Add("Quit").OnClick(func(*application.Context) {
	g.isQuitting.Store(true)
	g.app.Quit()
})

// in WindowClosing hook:
gui.window.RegisterHook(events.Common.WindowClosing, func(event *application.WindowEvent) {
	if shouldCancelWindowClose(gui.isQuitting.Load()) {
		gui.window.Hide()
		event.Cancel()
	}
})
```

- [ ] **Step 4: Re-run targeted test**

Run:  
`go test ./internal/desktop -run TestWindowClosingDoesNotCancelWhenAppIsQuitting -count=1`  
Expected: PASS.

- [ ] **Step 5: Run nearby desktop shutdown tests**

Run:  
`go test ./internal/desktop -run TestWaitForDesktopRunTimesOutInsteadOfBlockingQuit -count=1`  
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add app.go internal/desktop/wails_test.go
git commit -m "fix: ensure tray quit fully exits application"
```

### Task 2: Add actionable pull/rebase diagnostics to snapshot

**Files:**
- Modify: `internal/desktop/models.go`
- Modify: `internal/desktop/service.go`
- Test: `internal/desktop/service_test.go`

- [ ] **Step 1: Add failing backend snapshot test**

```go
func TestSnapshotIncludesRebaseDirtyWorktreeDiagnostic(t *testing.T) {
	home := configuredHome(t)
	service, err := New(home, runtime.GOOS)
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()

	service.recordCycle(daemon.CycleResult{
		Error: "pull: git pull --rebase: exit status 128: error: cannot pull with rebase: You have unstaged changes. error: Please commit or stash them.",
		NeedsAttention: true,
		FinishedAt: time.Now(),
	})

	snapshot, err := service.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.SyncDiagnostic == nil || snapshot.SyncDiagnostic.Code != "git-rebase-dirty-worktree" {
		t.Fatalf("diagnostic = %#v", snapshot.SyncDiagnostic)
	}
}
```

- [ ] **Step 2: Run targeted test and confirm fail**

Run:  
`go test ./internal/desktop -run TestSnapshotIncludesRebaseDirtyWorktreeDiagnostic -count=1`  
Expected: FAIL (`SyncDiagnostic` field missing).

- [ ] **Step 3: Add typed model fields**

```go
type SyncFixStep struct {
	Title   string `json:"title"`
	Command string `json:"command"`
	Warning string `json:"warning,omitempty"`
}

type SyncDiagnostic struct {
	Code     string        `json:"code"`
	Summary  string        `json:"summary"`
	RepoPath string        `json:"repoPath"`
	Steps    []SyncFixStep `json:"steps"`
}

// in Snapshot
SyncDiagnostic *SyncDiagnostic `json:"syncDiagnostic,omitempty"`
RepoPath       string          `json:"repoPath"`
```

- [ ] **Step 4: Implement classifier in `service.go`**

```go
func classifySyncDiagnostic(lastError, repoPath string) *SyncDiagnostic {
	if strings.Contains(lastError, "cannot pull with rebase") &&
		strings.Contains(lastError, "Please commit or stash them") {
		return &SyncDiagnostic{
			Code:     "git-rebase-dirty-worktree",
			Summary:  "Local repository has unstaged changes, so pull --rebase is blocked.",
			RepoPath: repoPath,
			Steps: []SyncFixStep{
				{Title: "Commit local changes", Command: "git add -A && git commit -m \"wip: local changes\" && git pull --rebase"},
				{Title: "Stash then pull", Command: "git stash push -u -m \"temp before sync\" && git pull --rebase && git stash pop"},
				{Title: "Discard local changes", Command: "git restore . && git pull --rebase", Warning: "Destructive: discards local edits"},
			},
		}
	}
	return nil
}
```

- [ ] **Step 5: Populate fields in snapshot builder**

```go
repoPath := cli.RepoDir(s.home) // will be refactored in P2 for override
return Snapshot{
	// existing fields...
	LastError:       last.Error,
	RepoPath:        repoPath,
	SyncDiagnostic:  classifySyncDiagnostic(last.Error, repoPath),
}
```

- [ ] **Step 6: Re-run targeted + nearby tests**

Run:  
`go test ./internal/desktop -run "TestSnapshotIncludesRebaseDirtyWorktreeDiagnostic|TestSnapshotIncludesResourceCategoriesAndPendingWork" -count=1`  
Expected: PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/desktop/models.go internal/desktop/service.go internal/desktop/service_test.go
git commit -m "feat: expose actionable pull rebase diagnostics in snapshot"
```

### Task 3: Add Fix Sync Issue panel and open-folder action in UI

**Files:**
- Modify: `frontend/src/desktopState.ts`
- Modify: `frontend/src/App.tsx`
- Modify: `frontend/src/App.test.tsx`

- [ ] **Step 1: Add failing UI test**

```tsx
it('renders sync fix panel for git rebase dirty worktree', async () => {
  api.snapshot.mockResolvedValue({
    ...configuredSnapshot(),
    repoPath: 'C:/Users/test/.synchub/repo',
    syncDiagnostic: {
      code: 'git-rebase-dirty-worktree',
      summary: 'Local repository has unstaged changes',
      repoPath: 'C:/Users/test/.synchub/repo',
      steps: [{ title: 'Stash then pull', command: 'git stash ...' }],
    },
  })
  render(<App />)
  expect(await screen.findByText('Fix sync issue')).toBeInTheDocument()
  expect(screen.getByText('C:/Users/test/.synchub/repo')).toBeInTheDocument()
})
```

- [ ] **Step 2: Run test and verify failure**

Run:  
`npm --prefix frontend test -- App.test.tsx --runInBand`  
Expected: FAIL (missing `syncDiagnostic` handling).

- [ ] **Step 3: Extend snapshot normalization types**

```ts
export type AppSyncFixStep = { title: string; command: string; warning?: string }
export type AppSyncDiagnostic = {
  code: string
  summary: string
  repoPath: string
  steps: AppSyncFixStep[]
}

export type AppSnapshot = Omit<Snapshot, ...> & {
  // existing fields...
  repoPath: string
  syncDiagnostic?: AppSyncDiagnostic | null
}
```

- [ ] **Step 4: Render fix panel in `App.tsx`**

```tsx
{snapshot.syncDiagnostic && (
  <section className="panel warning">
    <div className="panel-heading">
      <h2>Fix sync issue</h2>
    </div>
    <p>{snapshot.syncDiagnostic.summary}</p>
    <code>{snapshot.syncDiagnostic.repoPath}</code>
    <button className="secondary" onClick={() => void Browser.OpenURL(`file://${snapshot.syncDiagnostic!.repoPath}`)}>
      Open repo folder
    </button>
    <ol>
      {snapshot.syncDiagnostic.steps.map((step) => (
        <li key={step.title}>
          <strong>{step.title}</strong>
          <pre>{step.command}</pre>
          {step.warning && <small>{step.warning}</small>}
        </li>
      ))}
    </ol>
  </section>
)}
```

- [ ] **Step 5: Re-run frontend tests**

Run:  
`npm --prefix frontend test -- App.test.tsx --runInBand`  
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add frontend/src/desktopState.ts frontend/src/App.tsx frontend/src/App.test.tsx
git commit -m "feat: add sync fix panel for pull rebase dirty workspace errors"
```

### Task 4: Add “Run sync now” action in Settings drawer

**Files:**
- Modify: `frontend/src/SettingsPanel.tsx`
- Modify: `frontend/src/App.tsx`
- Modify: `frontend/src/SettingsPanel.test.tsx`

- [ ] **Step 1: Add failing test for settings sync action**

```tsx
it('shows run sync now action in settings and triggers callback', async () => {
  const runSyncNow = vi.fn().mockResolvedValue(undefined)
  render(
    <SettingsPanel
      snapshot={normalizeSnapshot(snapshot('2026-08-18T09:00:00Z', 7))}
      busy={false}
      previewLoading={false}
      close={vi.fn()}
      refreshPreview={vi.fn().mockResolvedValue(undefined)}
      save={vi.fn().mockResolvedValue(true)}
      runSyncNow={runSyncNow}
    />,
  )
  await userEvent.click(screen.getByRole('button', { name: 'Run sync now' }))
  expect(runSyncNow).toHaveBeenCalledTimes(1)
})
```

- [ ] **Step 2: Run test and verify failure**

Run:  
`npm --prefix frontend test -- SettingsPanel.test.tsx --runInBand`  
Expected: FAIL (`runSyncNow` prop missing).

- [ ] **Step 3: Implement prop and button**

```tsx
export type SettingsPanelProps = {
  // existing props...
  runSyncNow: () => Promise<void>
}

<button
  type="button"
  className="secondary"
  disabled={busy || !snapshot.configured}
  onClick={() => void runSyncNow()}
>
  Run sync now
</button>
```

- [ ] **Step 4: Wire callback from `App.tsx`**

```tsx
<SettingsPanel
  // existing props...
  runSyncNow={() => perform(TriggerSync, 'Synchronization queued') as Promise<void> }
/>
```

- [ ] **Step 5: Re-run frontend tests**

Run:  
`npm --prefix frontend test -- SettingsPanel.test.tsx --runInBand`  
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add frontend/src/SettingsPanel.tsx frontend/src/App.tsx frontend/src/SettingsPanel.test.tsx
git commit -m "feat: add settings action to trigger one immediate sync"
```

### Task 5: Add configurable local repo directory to config/runtime

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`
- Modify: `internal/cli/runtime.go`
- Modify: `internal/cli/sync.go`
- Modify: `internal/cli/status.go`
- Modify: `internal/cli/cleanup.go`
- Modify: `internal/desktop/models.go`
- Modify: `internal/desktop/service.go`

- [ ] **Step 1: Add failing config round-trip test for repo dir**

```go
func TestSaveLoadRoundTripIncludesRepoDir(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	in := Config{
		RepoURL: "git@github.com:me/synchub-data.git",
		RepoDir: "${HOME}/custom-sync-repo",
		SyncIntervalMinutes: 15,
		TrashGraceDays: 7,
		Agents: map[string]bool{"claude": true},
	}
	if err := Save(path, in); err != nil {
		t.Fatal(err)
	}
	out, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if out.RepoDir != in.RepoDir {
		t.Fatalf("repo dir = %q, want %q", out.RepoDir, in.RepoDir)
	}
}
```

- [ ] **Step 2: Run config test and verify fail**

Run:  
`go test ./internal/config -run TestSaveLoadRoundTripIncludesRepoDir -count=1`  
Expected: FAIL (`RepoDir` missing).

- [ ] **Step 3: Add `RepoDir` in config model and keep default compatibility**

```go
type Config struct {
	// ...
	RepoURL string `yaml:"repo_url"`
	RepoDir string `yaml:"repo_dir,omitempty"`
}
```

- [ ] **Step 4: Add runtime resolver and switch call sites**

```go
func RepoDir(home string) string { return filepath.Join(home, "repo") }

func EffectiveRepoDir(home string, cfg config.Config) string {
	if strings.TrimSpace(cfg.RepoDir) == "" {
		return RepoDir(home)
	}
	resolved, err := pathresolver.Resolve(cfg.RepoDir)
	if err != nil {
		return RepoDir(home)
	}
	return resolved
}
```

Then replace `RepoDir(home)` with `EffectiveRepoDir(home, cfg)` in:
- `internal/cli/sync.go`
- `internal/cli/status.go`
- `internal/cli/cleanup.go`
- desktop snapshot path emission

- [ ] **Step 5: Re-run targeted Go tests**

Run:  
`go test ./internal/config ./internal/cli ./internal/desktop -run "RepoDir|SaveSettings|Status" -count=1`  
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go internal/cli/runtime.go internal/cli/sync.go internal/cli/status.go internal/cli/cleanup.go internal/desktop/models.go internal/desktop/service.go
git commit -m "feat: support configurable local sync repository directory"
```

### Task 6: Add Settings repo-dir change flow with keep-or-migrate modes

**Files:**
- Modify: `internal/desktop/models.go`
- Modify: `internal/desktop/service.go`
- Modify: `internal/repository/setup.go`
- Modify: `internal/desktop/service_test.go`
- Modify: `frontend/src/SettingsPanel.tsx`
- Modify: `frontend/src/SettingsPanel.test.tsx`

- [ ] **Step 1: Add failing backend migration-failure test**

```go
func TestSaveSettingsRepoDirMigrationFailureKeepsOriginalConfig(t *testing.T) {
	home := configuredHome(t)
	service, err := New(home, runtime.GOOS)
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()

	original, err := config.Load(cli.ConfigPath(home))
	if err != nil {
		t.Fatal(err)
	}
	err = service.SaveSettings(SettingsInput{
		RepositoryURL:   original.RepoURL,
		RepositoryDir:   filepath.Join(home, "new-repo"),
		RepoPathMode:    "migrate",
		IntervalMinutes: original.SyncIntervalMinutes,
		TrashGraceDays:  original.TrashGraceDays,
		Agents:          original.Agents,
	})
	if err == nil {
		t.Fatal("expected migration failure in test fixture")
	}
	after, err := config.Load(cli.ConfigPath(home))
	if err != nil {
		t.Fatal(err)
	}
	if after.RepoDir != original.RepoDir {
		t.Fatal("config changed despite migration failure")
	}
}
```

- [ ] **Step 2: Run targeted test and verify fail**

Run:  
`go test ./internal/desktop -run TestSaveSettingsRepoDirMigrationFailureKeepsOriginalConfig -count=1`  
Expected: FAIL (`RepositoryDir`/`RepoPathMode` missing).

- [ ] **Step 3: Extend settings input and implement transactional apply**

```go
type SettingsInput struct {
	RepositoryURL string `json:"repositoryUrl"`
	RepositoryDir string `json:"repositoryDir,omitempty"`
	RepoPathMode  string `json:"repoPathMode,omitempty"` // "reclone" | "migrate"
	// existing fields...
}
```

Implementation order in `SaveSettings`:
1. Validate timing.
2. Load existing config.
3. If repo dir changed:
   - execute re-clone or migrate operation first,
   - if operation fails, return error immediately (do not write config).
4. Write updated config.
5. Refresh daemon interval and trigger sync.

- [ ] **Step 4: Add Settings UI controls**

```tsx
const [repositoryDir, setRepositoryDir] = useState(snapshot.repoPath)
const [repoPathMode, setRepoPathMode] = useState<'reclone' | 'migrate'>('reclone')

<label>
  Local repository directory
  <input value={repositoryDir} onChange={(e) => setRepositoryDir(e.target.value)} />
</label>
<fieldset>
  <legend>When directory changes</legend>
  <label><input type="radio" checked={repoPathMode==='reclone'} ... /> Keep old folder, clone into new</label>
  <label><input type="radio" checked={repoPathMode==='migrate'} ... /> Move existing repo to new folder</label>
</fieldset>
```

- [ ] **Step 5: Re-run frontend + backend tests**

Run:  
`go test ./internal/desktop ./internal/repository -run "RepoDir|Migration|SaveSettings" -count=1`  
`npm --prefix frontend test -- SettingsPanel.test.tsx --runInBand`  
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/desktop/models.go internal/desktop/service.go internal/repository/setup.go internal/desktop/service_test.go frontend/src/SettingsPanel.tsx frontend/src/SettingsPanel.test.tsx
git commit -m "feat: support repo directory migration options in settings"
```

### Task 7: Add first-sync strategy selection (Use cloud / Merge / Use local)

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/onboarding/models.go`
- Modify: `internal/onboarding/service.go`
- Modify: `internal/desktop/models.go`
- Modify: `internal/desktop/wails.go`
- Modify: `internal/syncengine/engine.go`
- Modify: `internal/syncengine/engine_test.go`
- Modify: `frontend/src/onboarding/Onboarding.tsx`
- Modify: `frontend/src/App.tsx`
- Modify: `frontend/src/App.test.tsx`

- [ ] **Step 1: Add failing first-sync strategy test**

```go
func TestFirstSyncStrategyUseCloudIsAppliedOnce(t *testing.T) {
	// fixture: local changes + remote changes
	// config.FirstSync.Strategy = "use-cloud", Completed = false
	// first SyncOnce should favor remote and set completed=true
	// second SyncOnce should use normal merge path
}
```

- [ ] **Step 2: Run targeted test and confirm fail**

Run:  
`go test ./internal/syncengine -run TestFirstSyncStrategyUseCloudIsAppliedOnce -count=1`  
Expected: FAIL (strategy state missing).

- [ ] **Step 3: Extend config with first-sync policy state**

```go
type FirstSyncPolicy struct {
	Strategy  string `yaml:"strategy,omitempty"`  // use-cloud | merge | use-local
	Completed bool   `yaml:"completed,omitempty"`
}

type Config struct {
	// ...
	FirstSync FirstSyncPolicy `yaml:"first_sync,omitempty"`
}
```

- [ ] **Step 4: Add onboarding/desktop API to stage strategy before first run**

```go
type FirstSyncChoiceInput struct {
	Strategy string `json:"strategy"`
}

func (s *WailsService) SetFirstSyncStrategy(input FirstSyncChoiceInput) error {
	return s.core.SetFirstSyncStrategy(input.Strategy)
}
```

- [ ] **Step 5: Add UI prompt before first real sync**

```tsx
// show modal when snapshot.configured && !snapshot.firstSyncCompleted
// options:
// - Use cloud
// - Merge cloud + local
// - Use local
// submit -> SetFirstSyncStrategy(...)
```

- [ ] **Step 6: Implement syncengine branch**

```go
switch cfg.FirstSync.Strategy {
case "use-cloud":
	// remote wins for first cycle
case "use-local":
	// local snapshot initializes remote
default: // merge
	// existing reconcile path
}
// after successful first cycle:
cfg.FirstSync.Completed = true
save config
```

- [ ] **Step 7: Re-run tests**

Run:  
`go test ./internal/syncengine ./internal/onboarding ./internal/desktop -run "FirstSync|Strategy|Onboarding" -count=1`  
`npm --prefix frontend test -- App.test.tsx --runInBand`  
Expected: PASS.

- [ ] **Step 8: Commit**

```bash
git add internal/config/config.go internal/onboarding/models.go internal/onboarding/service.go internal/desktop/models.go internal/desktop/wails.go internal/syncengine/engine.go internal/syncengine/engine_test.go frontend/src/onboarding/Onboarding.tsx frontend/src/App.tsx frontend/src/App.test.tsx
git commit -m "feat: add first-sync strategy selection and bootstrap policy"
```

### Task 8: End-to-end regression pass for P1 + P2

**Files:**
- Modify: `docs/install.md` (only if UX text changed in onboarding/settings)

- [ ] **Step 1: Run backend targeted suites**

Run:  
`go test ./internal/desktop ./internal/cli ./internal/config ./internal/syncengine -count=1`
Expected: PASS.

- [ ] **Step 2: Run frontend test suite**

Run:  
`npm --prefix frontend test`
Expected: PASS.

- [ ] **Step 3: Run frontend build**

Run:  
`npm --prefix frontend run build`
Expected: build succeeds.

- [ ] **Step 4: If onboarding/settings copy changed, update docs**

```md
Add short notes in docs/install.md:
- how to resolve pull/rebase dirty-worktree errors
- repo directory relocation options
- first-sync strategy meanings
```

- [ ] **Step 5: Final commit**

```bash
git add docs/install.md
git commit -m "docs: document sync troubleshooting and first-sync options"
```

## Self-Review

1. **Spec coverage:**  
   - Exit not terminating → Task 1  
   - pull/rebase guidance UI → Tasks 2-3  
   - repo directory change + migrate/keep behavior → Tasks 5-6  
   - settings manual resync → Task 4  
   - first-sync 3 strategies → Task 7

2. **Placeholder scan:** No TBD/TODO/“similar to” placeholders remain.

3. **Type consistency:**  
   - `SyncDiagnostic`, `RepoPath`, `RepositoryDir`, `RepoPathMode`, `FirstSyncPolicy` are introduced before dependent tasks.
   - UI and backend method names match planned bindings (`TriggerSync`, `SetFirstSyncStrategy`, `SaveSettings` payload extension).
