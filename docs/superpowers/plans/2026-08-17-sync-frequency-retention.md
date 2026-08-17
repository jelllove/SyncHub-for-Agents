# Configurable Sync Frequency and Archive Retention Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let users choose a 1–1440 minute sync frequency and 1–365 day archive retention period, defaulting to 10 minutes and 30 days, and display both values on the dashboard.

**Architecture:** Keep the existing configuration and snapshot fields as the source of truth. Strengthen desktop-service validation before persistence or scheduler mutation, then replace the frontend presets with bounded numeric inputs and render the snapshot values in the dashboard metrics.

**Tech Stack:** Go, Wails v3, React 19, TypeScript, CSS, Go testing.

---

## File Structure

- `internal/desktop/service.go`: validates timing settings before loading, saving, or applying them.
- `internal/desktop/service_test.go`: proves accepted boundaries, rejected boundaries, and no mutation on rejection.
- `frontend/src/App.tsx`: renders the timing metrics and bounded numeric settings inputs.
- `docs/superpowers/specs/2026-08-17-sync-frequency-retention-design.md`: approved behavior contract.

### Task 1: Enforce timing boundaries without partial updates

**Files:**
- Modify: `internal/desktop/service.go:359-398`
- Modify: `internal/desktop/service_test.go:193-220`

- [ ] **Step 1: Write failing table-driven validation tests**

Add a test that starts from the configured 10-minute/30-day state, attempts every invalid boundary, and verifies the error, saved configuration, and live scheduler remain unchanged:

```go
func TestSaveSettingsRejectsTimingOutsideSupportedRanges(t *testing.T) {
	tests := []struct {
		name     string
		interval int
		grace    int
		want     string
	}{
		{name: "zero interval", interval: 0, grace: 30, want: "sync interval must be between 1 and 1440 minutes"},
		{name: "interval above one day", interval: 1441, grace: 30, want: "sync interval must be between 1 and 1440 minutes"},
		{name: "zero retention", interval: 10, grace: 0, want: "archive retention must be between 1 and 365 days"},
		{name: "retention above one year", interval: 10, grace: 366, want: "archive retention must be between 1 and 365 days"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			home := configuredHome(t)
			service, err := New(home, runtime.GOOS)
			if err != nil {
				t.Fatal(err)
			}
			defer service.Close()

			err = service.SaveSettings(SettingsInput{
				RepositoryURL:   "git@github.com:owner/changed.git",
				IntervalMinutes: test.interval,
				TrashGraceDays:  test.grace,
				Agents:          map[string]bool{"claude": false},
			})
			if err == nil || err.Error() != test.want {
				t.Fatalf("error = %v, want %q", err, test.want)
			}

			cfg, err := config.Load(cli.ConfigPath(home))
			if err != nil {
				t.Fatal(err)
			}
			if cfg.RepoURL != "git@github.com:owner/repo.git" ||
				cfg.SyncIntervalMinutes != 10 ||
				cfg.TrashGraceDays != 30 {
				t.Fatalf("config changed after rejected settings: %#v", cfg)
			}
			if got := service.Daemon().Scheduler.IntervalDuration(); got != 10*time.Minute {
				t.Fatalf("interval = %v, want unchanged 10m", got)
			}
		})
	}
}
```

- [ ] **Step 2: Extend the accepted-settings test to cover both upper boundaries**

Change the existing success test input and assertions:

```go
err = service.SaveSettings(SettingsInput{
	RepositoryURL:   "git@github.com:owner/new.git",
	IntervalMinutes: 1440,
	TrashGraceDays:  365,
	Agents:          map[string]bool{"claude": false},
})
```

```go
if cfg.RepoURL != "git@github.com:owner/new.git" ||
	cfg.SyncIntervalMinutes != 1440 ||
	cfg.TrashGraceDays != 365 {
	t.Fatalf("config = %#v", cfg)
}
if got := service.Daemon().Scheduler.IntervalDuration(); got != 1440*time.Minute {
	t.Fatalf("interval = %v, want 1440m", got)
}
```

- [ ] **Step 3: Run the targeted tests and verify RED**

Run:

```powershell
go test ./internal/desktop -run 'TestSaveSettings(UpdatesConfigAndLiveInterval|RejectsTimingOutsideSupportedRanges)' -count=1
```

Expected: FAIL because values above the maximum and zero-day retention are currently accepted, and current error strings do not match the bounded contract.

- [ ] **Step 4: Implement bounded validation before persistence**

Replace the initial validation in `SaveSettings` with:

```go
const (
	minSyncIntervalMinutes = 1
	maxSyncIntervalMinutes = 24 * 60
	minArchiveRetentionDays = 1
	maxArchiveRetentionDays = 365
)

func validateTimingSettings(intervalMinutes, retentionDays int) error {
	if intervalMinutes < minSyncIntervalMinutes || intervalMinutes > maxSyncIntervalMinutes {
		return fmt.Errorf(
			"sync interval must be between %d and %d minutes",
			minSyncIntervalMinutes,
			maxSyncIntervalMinutes,
		)
	}
	if retentionDays < minArchiveRetentionDays || retentionDays > maxArchiveRetentionDays {
		return fmt.Errorf(
			"archive retention must be between %d and %d days",
			minArchiveRetentionDays,
			maxArchiveRetentionDays,
		)
	}
	return nil
}

func (s *Service) SaveSettings(input SettingsInput) error {
	if err := validateTimingSettings(input.IntervalMinutes, input.TrashGraceDays); err != nil {
		return err
	}
```

Add `fmt` to the imports and remove `errors` only if no other code in `service.go` uses it.

- [ ] **Step 5: Run targeted tests and verify GREEN**

Run:

```powershell
go test ./internal/desktop -run 'TestSaveSettings(UpdatesConfigAndLiveInterval|RejectsTimingOutsideSupportedRanges)' -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit the service validation**

```powershell
git add internal\desktop\service.go internal\desktop\service_test.go
git commit -m "feat: validate configurable sync timing"
```

### Task 2: Add numeric controls and dashboard metrics

**Files:**
- Modify: `frontend/src/App.tsx:249-254`
- Modify: `frontend/src/App.tsx:459-476`

- [ ] **Step 1: Add concise duration formatters**

Add these pure helpers beside `formatTime`:

```tsx
function formatSyncFrequency(minutes: number) {
  return `Every ${minutes} ${minutes === 1 ? 'minute' : 'minutes'}`
}

function formatArchiveRetention(days: number) {
  return `${days} ${days === 1 ? 'day' : 'days'}`
}
```

- [ ] **Step 2: Replace the two dashboard metrics**

Replace Pending changes and Protected agents:

```tsx
<section className="metrics" aria-label="Synchronization details">
  <Metric label="Last sync" value={formatTime(snapshot.lastSync)} />
  <Metric label="Next sync" value={paused ? 'Paused' : formatTime(snapshot.nextSync)} />
  <Metric label="Sync frequency" value={formatSyncFrequency(snapshot.intervalMinutes)} />
  <Metric label="Archive retention" value={formatArchiveRetention(snapshot.trashGraceDays)} />
</section>
```

- [ ] **Step 3: Replace preset selects with bounded numeric inputs**

Use native numeric constraints and explanatory text:

```tsx
<div className="field-grid">
  <label>
    Sync frequency (minutes)
    <input
      required
      type="number"
      min={1}
      max={1440}
      step={1}
      value={intervalMinutes}
      onChange={(event) => setIntervalMinutes(Number(event.target.value))}
    />
    <small>Runs every 1–1440 minutes.</small>
  </label>
  <label>
    Archive retention (days)
    <input
      required
      type="number"
      min={1}
      max={365}
      step={1}
      value={trashGraceDays}
      onChange={(event) => setTrashGraceDays(Number(event.target.value))}
    />
    <small>Deleted files remain recoverable for 1–365 days.</small>
  </label>
</div>
```

- [ ] **Step 4: Run the production frontend build**

Run:

```powershell
npm --prefix frontend run build
```

Expected: TypeScript and Vite production build PASS.

- [ ] **Step 5: Commit the dashboard and settings UI**

```powershell
git add frontend\src\App.tsx
git commit -m "feat: show configurable sync timing"
```

### Task 3: Run regression validation

**Files:**
- Verify: all changed files

- [ ] **Step 1: Run full Go tests**

Run:

```powershell
go test ./... -count=1
```

Expected: PASS.

- [ ] **Step 2: Run Go static analysis**

Run:

```powershell
go vet ./...
```

Expected: PASS with no diagnostics.

- [ ] **Step 3: Re-run the frontend production build**

Run:

```powershell
npm --prefix frontend run build
```

Expected: PASS.

- [ ] **Step 4: Confirm repository state**

Run:

```powershell
git status --short
git log --oneline -3
```

Expected: clean worktree with the service-validation and UI commits at HEAD.
