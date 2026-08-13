# AgentConfigSync Desktop Shell Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the CLI-first tray experience with a single-instance Wails desktop application that owns the scheduler, dashboard, settings, and system tray while preserving the existing headless CLI.

**Architecture:** Add a canonical root Wails v3 application and keep `cmd/acsync` as the separate CLI. A UI-independent `desktop` service wraps the existing daemon/config/provider packages and publishes typed snapshots; Wails owns windows, tray, native lifecycle, and frontend bindings. The scheduler gains multi-subscriber state events and live interval replacement so the daemon, desktop window, and tray can observe one scheduler safely.

**Tech Stack:** Go 1.26.5, Wails v3.0.0-beta.8 (pinned), React 19, TypeScript, Vite, Vitest, existing Go sync packages.

---

## File Structure

```text
main.go                              # Wails GUI entrypoint only
app.go                               # Wails app/window/tray composition
assets.go                            # frontend/dist embed
build/config.yml                     # Wails product metadata
build/appicon.png                    # generated product icon
Taskfile.yml                         # Wails dev/build tasks
internal/scheduler/scheduler.go      # live interval + state subscribers
internal/scheduler/scheduler_test.go
internal/daemon/daemon.go            # observable sync result/error
internal/daemon/daemon_test.go
internal/desktop/models.go           # frontend-safe DTOs
internal/desktop/service.go          # UI-independent desktop facade
internal/desktop/service_test.go
internal/desktop/lifecycle.go        # start/stop, snapshot broadcast
internal/desktop/lifecycle_test.go
internal/desktop/wails.go            # Wails binding adapter
internal/tray/icon.go                # reuse generated status icons
frontend/package.json
frontend/package-lock.json
frontend/index.html
frontend/vite.config.ts
frontend/tsconfig.json
frontend/src/main.tsx
frontend/src/App.tsx
frontend/src/api.ts
frontend/src/models.ts
frontend/src/styles.css
frontend/src/components/StatusCard.tsx
frontend/src/components/AgentGrid.tsx
frontend/src/components/ActivityPanel.tsx
frontend/src/components/SettingsPanel.tsx
frontend/src/components/*.test.tsx
```

The existing `cmd/acsync` CLI remains independent. Do not initialize Wails or
take the GUI single-instance lock before Cobra selects a command.

---

### Task 1: Add live scheduler interval updates and state subscriptions

**Files:**
- Modify: `internal/scheduler/scheduler.go`
- Modify: `internal/scheduler/scheduler_test.go`

- [ ] **Step 1: Add failing tests for interval replacement and subscribers**

Append:

```go
func TestSetIntervalReplacesTickerWithoutRestart(t *testing.T) {
	var runs atomic.Int32
	s := New(time.Hour, func() error {
		runs.Add(1)
		return nil
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go s.Run(ctx)

	s.SetInterval(10 * time.Millisecond)
	deadline := time.After(time.Second)
	for runs.Load() == 0 {
		select {
		case <-deadline:
			t.Fatal("new interval did not trigger a run")
		default:
			time.Sleep(5 * time.Millisecond)
		}
	}
	if got := s.IntervalDuration(); got != 10*time.Millisecond {
		t.Fatalf("interval = %v", got)
	}
}

func TestSubscribeReceivesStateAndUnsubscribeStopsIt(t *testing.T) {
	s := New(time.Hour, func() error { return nil })
	var got []State
	var mu sync.Mutex
	unsubscribe := s.Subscribe(func(st State) {
		mu.Lock()
		got = append(got, st)
		mu.Unlock()
	})
	s.Trigger()
	ctx, cancel := context.WithCancel(context.Background())
	go s.Run(ctx)
	time.Sleep(50 * time.Millisecond)
	unsubscribe()
	s.Pause()
	cancel()

	mu.Lock()
	defer mu.Unlock()
	if len(got) < 2 || got[0] != StateUpdating || got[1] != StateIdle {
		t.Fatalf("states = %v", got)
	}
	for _, st := range got {
		if st == StatePaused {
			t.Fatalf("received state after unsubscribe: %v", got)
		}
	}
}
```

Add imports:

```go
import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)
```

- [ ] **Step 2: Run the focused tests**

Run:

```powershell
go test ./internal/scheduler -run "TestSetInterval|TestSubscribe" -v
```

Expected: build failure because `SetInterval`, `IntervalDuration`, and
`Subscribe` do not exist.

- [ ] **Step 3: Replace the scheduler's mutable callback contract**

Change `Scheduler` to:

```go
type Scheduler struct {
	Job Job

	mu          sync.Mutex
	interval    time.Duration
	paused      bool
	state       State
	nextSubID   uint64
	subscribers map[uint64]func(State)

	trigger        chan struct{}
	intervalChange chan time.Duration
}
```

Change `New` to:

```go
func New(interval time.Duration, job Job) *Scheduler {
	if interval <= 0 {
		interval = 10 * time.Minute
	}
	return &Scheduler{
		Job:            job,
		interval:       interval,
		state:          StateIdle,
		subscribers:    map[uint64]func(State){},
		trigger:        make(chan struct{}, 1),
		intervalChange: make(chan time.Duration, 1),
	}
}
```

Add:

```go
func (s *Scheduler) IntervalDuration() time.Duration {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.interval
}

func (s *Scheduler) SetInterval(interval time.Duration) {
	if interval <= 0 {
		interval = 10 * time.Minute
	}
	s.mu.Lock()
	s.interval = interval
	s.mu.Unlock()
	select {
	case s.intervalChange <- interval:
	default:
		select {
		case <-s.intervalChange:
		default:
		}
		s.intervalChange <- interval
	}
}

func (s *Scheduler) Subscribe(fn func(State)) func() {
	s.mu.Lock()
	id := s.nextSubID
	s.nextSubID++
	s.subscribers[id] = fn
	s.mu.Unlock()
	return func() {
		s.mu.Lock()
		delete(s.subscribers, id)
		s.mu.Unlock()
	}
}
```

Replace `setState`:

```go
func (s *Scheduler) setState(st State) {
	s.mu.Lock()
	s.state = st
	callbacks := make([]func(State), 0, len(s.subscribers))
	for _, callback := range s.subscribers {
		callbacks = append(callbacks, callback)
	}
	s.mu.Unlock()
	for _, callback := range callbacks {
		callback(st)
	}
}
```

Replace `Run`:

```go
func (s *Scheduler) Run(ctx context.Context) {
	ticker := time.NewTicker(s.IntervalDuration())
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case interval := <-s.intervalChange:
			ticker.Reset(interval)
		case <-ticker.C:
			s.runCycle()
		case <-s.trigger:
			s.runCycle()
		}
	}
}
```

Remove the exported `Interval` and `OnState` fields. Update existing tests to
use `IntervalDuration()` and `Subscribe`.

- [ ] **Step 4: Update daemon and tray callers**

In `internal/daemon/daemon.go`, replace:

```go
d.Scheduler.OnState = d.logState
```

with:

```go
d.Scheduler.Subscribe(d.logState)
```

Replace `d.Scheduler.Interval` in log output with:

```go
d.Scheduler.IntervalDuration()
```

In `internal/tray/tray.go`, replace assignment to `OnState` with:

```go
a.d.Scheduler.Subscribe(func(s scheduler.State) {
	a.d.Logger.Printf("state: %s", s)
	systray.SetIcon(Icon(s, a.GOOS))
})
```

Update daemon tests to assert `IntervalDuration()`.

- [ ] **Step 5: Verify scheduler, daemon, and legacy tray**

Run:

```powershell
go test ./internal/scheduler ./internal/daemon ./internal/tray -race
```

Expected: all packages pass.

- [ ] **Step 6: Commit**

```powershell
git add internal/scheduler internal/daemon internal/tray/tray.go
git commit -m "feat: support live scheduler updates and subscribers"
```

---

### Task 2: Expose structured daemon cycle results

**Files:**
- Modify: `internal/daemon/daemon.go`
- Modify: `internal/daemon/daemon_test.go`

- [ ] **Step 1: Add a failing observer test**

Append:

```go
func TestDaemonPublishesCycleErrors(t *testing.T) {
	home := filepath.Join(t.TempDir(), ".acsync")
	writeConfig(t, home, config.Config{SyncIntervalMinutes: 60, Agents: map[string]bool{}})
	d, err := New(home, runtime.GOOS)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()

	events := make(chan CycleResult, 1)
	d.OnCycle = func(result CycleResult) { events <- result }
	if err := d.syncJob(); err == nil {
		t.Fatal("expected missing repository error")
	}
	select {
	case result := <-events:
		if result.Error == "" || result.FinishedAt.IsZero() {
			t.Fatalf("result = %#v", result)
		}
	case <-time.After(time.Second):
		t.Fatal("cycle result not published")
	}
}
```

- [ ] **Step 2: Run the failing test**

```powershell
go test ./internal/daemon -run TestDaemonPublishesCycleErrors -v
```

Expected: build failure because `CycleResult` and `OnCycle` do not exist.

- [ ] **Step 3: Add the cycle DTO and publish every outcome**

Add:

```go
type CycleResult struct {
	Actions    int
	Blocked    int
	Pushed     bool
	Purged     int
	Error      string
	FinishedAt time.Time
}
```

Add to `Daemon`:

```go
OnCycle func(CycleResult)
```

Replace `syncJob` with:

```go
func (d *Daemon) syncJob() error {
	result := CycleResult{}
	defer func() {
		result.FinishedAt = time.Now()
		if d.OnCycle != nil {
			d.OnCycle(result)
		}
	}()

	res, err := cli.RunSync(d.Home, d.GOOS)
	if err != nil {
		result.Error = err.Error()
		d.Logger.Printf("sync error: %v", err)
		return err
	}
	result.Actions = len(res.Actions)
	result.Blocked = len(res.Blocked)
	result.Pushed = res.Pushed
	d.Logger.Printf("sync ok: %d actions, %d blocked, pushed=%v",
		result.Actions, result.Blocked, result.Pushed)

	purged, err := cli.RunCleanup(d.Home, time.Now())
	if err != nil {
		result.Error = err.Error()
		d.Logger.Printf("cleanup error: %v", err)
		return err
	}
	result.Purged = len(purged)
	if result.Purged > 0 {
		d.Logger.Printf("purged %d expired trash entries", result.Purged)
	}
	return nil
}
```

- [ ] **Step 4: Run tests**

```powershell
go test ./internal/daemon -race
```

Expected: pass.

- [ ] **Step 5: Commit**

```powershell
git add internal/daemon
git commit -m "feat: publish structured daemon cycle results"
```

---

### Task 3: Build the UI-independent desktop service

**Files:**
- Create: `internal/desktop/models.go`
- Create: `internal/desktop/service.go`
- Create: `internal/desktop/service_test.go`

- [ ] **Step 1: Write failing desktop service tests**

Create `internal/desktop/service_test.go`:

```go
package desktop

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/qinqingxu/acsync/internal/cli"
	"github.com/qinqingxu/acsync/internal/config"
)

func testHome(t *testing.T) string {
	t.Helper()
	home := filepath.Join(t.TempDir(), ".acsync")
	if err := os.MkdirAll(home, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := config.Save(cli.ConfigPath(home), config.Config{
		RepoURL:             "git@github.com:owner/repo.git",
		SyncIntervalMinutes: 10,
		TrashGraceDays:      30,
		Agents:              map[string]bool{"claude": true},
	}); err != nil {
		t.Fatal(err)
	}
	return home
}

func TestSnapshotMapsConfigAndStatus(t *testing.T) {
	service, err := New(testHome(t), runtime.GOOS)
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	got, err := service.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	if got.RepositoryURL != "git@github.com:owner/repo.git" ||
		got.IntervalMinutes != 10 ||
		len(got.Agents) == 0 {
		t.Fatalf("snapshot = %#v", got)
	}
}

func TestSaveSettingsUpdatesConfigAndLiveInterval(t *testing.T) {
	home := testHome(t)
	service, err := New(home, runtime.GOOS)
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	err = service.SaveSettings(SettingsInput{
		RepositoryURL:  "git@github.com:owner/new.git",
		IntervalMinutes: 3,
		TrashGraceDays:  45,
		Agents:          map[string]bool{"claude": false},
	})
	if err != nil {
		t.Fatal(err)
	}
	cfg, _ := config.Load(cli.ConfigPath(home))
	if cfg.RepoURL != "git@github.com:owner/new.git" ||
		cfg.SyncIntervalMinutes != 3 ||
		service.Daemon().Scheduler.IntervalDuration() != 3*time.Minute {
		t.Fatalf("config = %#v interval=%v", cfg, service.Daemon().Scheduler.IntervalDuration())
	}
}
```

- [ ] **Step 2: Run the failing tests**

```powershell
go test ./internal/desktop -v
```

Expected: package/types missing.

- [ ] **Step 3: Create frontend-safe models**

Create `internal/desktop/models.go`:

```go
package desktop

import "time"

type Agent struct {
	Name    string   `json:"name"`
	Enabled bool     `json:"enabled"`
	Exclude []string `json:"exclude"`
}

type Snapshot struct {
	Configured       bool      `json:"configured"`
	State            string    `json:"state"`
	RepositoryURL    string    `json:"repositoryUrl"`
	IntervalMinutes  int       `json:"intervalMinutes"`
	TrashGraceDays   int       `json:"trashGraceDays"`
	Agents           []Agent   `json:"agents"`
	LastSync         time.Time `json:"lastSync"`
	NextSync         time.Time `json:"nextSync"`
	PendingActions   int       `json:"pendingActions"`
	BlockedFiles     int       `json:"blockedFiles"`
	LastError        string    `json:"lastError"`
}

type SettingsInput struct {
	RepositoryURL   string          `json:"repositoryUrl"`
	IntervalMinutes int             `json:"intervalMinutes"`
	TrashGraceDays  int             `json:"trashGraceDays"`
	Agents          map[string]bool `json:"agents"`
}
```

- [ ] **Step 4: Implement the service**

Create `internal/desktop/service.go`:

```go
package desktop

import (
	"errors"
	"runtime"
	"sync"
	"time"

	"github.com/qinqingxu/acsync/internal/cli"
	"github.com/qinqingxu/acsync/internal/config"
	"github.com/qinqingxu/acsync/internal/daemon"
)

type Service struct {
	home   string
	goos   string
	daemon *daemon.Daemon
	mu     sync.Mutex
	last   daemon.CycleResult
}

func New(home, goos string) (*Service, error) {
	if goos == "" {
		goos = runtime.GOOS
	}
	d, err := daemon.New(home, goos)
	if err != nil {
		return nil, err
	}
	s := &Service{home: home, goos: goos, daemon: d}
	d.OnCycle = func(result daemon.CycleResult) {
		s.mu.Lock()
		s.last = result
		s.mu.Unlock()
	}
	return s, nil
}

func (s *Service) Daemon() *daemon.Daemon { return s.daemon }

func (s *Service) Close() error { return s.daemon.Close() }

func (s *Service) Snapshot() (Snapshot, error) {
	cfg, err := config.Load(cli.ConfigPath(s.home))
	if err != nil {
		return Snapshot{}, err
	}
	providers, err := cli.LoadProviders(s.home)
	if err != nil {
		return Snapshot{}, err
	}
	status, err := cli.RunStatus(s.home, s.goos)
	if err != nil {
		return Snapshot{}, err
	}
	agents := make([]Agent, 0, len(providers))
	for _, provider := range providers {
		agents = append(agents, Agent{
			Name: provider.Name, Enabled: cfg.Agents[provider.Name],
			Exclude: provider.Config.Exclude,
		})
	}
	s.mu.Lock()
	last := s.last
	s.mu.Unlock()
	next := time.Time{}
	if !last.FinishedAt.IsZero() {
		next = last.FinishedAt.Add(s.daemon.Scheduler.IntervalDuration())
	}
	return Snapshot{
		Configured: true, State: s.daemon.Scheduler.State().String(),
		RepositoryURL: cfg.RepoURL, IntervalMinutes: cfg.SyncIntervalMinutes,
		TrashGraceDays: cfg.TrashGraceDays, Agents: agents,
		LastSync: status.LastSync, NextSync: next,
		PendingActions: status.PendingActions, BlockedFiles: last.Blocked,
		LastError: last.Error,
	}, nil
}

func (s *Service) SaveSettings(input SettingsInput) error {
	if input.IntervalMinutes < 1 {
		return errors.New("sync interval must be at least one minute")
	}
	if input.TrashGraceDays < 0 {
		return errors.New("trash grace days cannot be negative")
	}
	cfg, err := config.Load(cli.ConfigPath(s.home))
	if err != nil {
		return err
	}
	cfg.RepoURL = input.RepositoryURL
	cfg.SyncIntervalMinutes = input.IntervalMinutes
	cfg.TrashGraceDays = input.TrashGraceDays
	cfg.Agents = input.Agents
	if err := config.Save(cli.ConfigPath(s.home), cfg); err != nil {
		return err
	}
	s.daemon.Scheduler.SetInterval(time.Duration(input.IntervalMinutes) * time.Minute)
	return nil
}

func (s *Service) Trigger() { s.daemon.Scheduler.Trigger() }
func (s *Service) Pause()   { s.daemon.Scheduler.Pause() }
func (s *Service) Resume()  { s.daemon.Scheduler.Resume() }
```

- [ ] **Step 5: Run tests**

```powershell
go test ./internal/desktop -race
```

Expected: pass.

- [ ] **Step 6: Commit**

```powershell
git add internal/desktop
git commit -m "feat: add desktop application service"
```

---

### Task 4: Add desktop lifecycle and observable snapshots

**Files:**
- Create: `internal/desktop/lifecycle.go`
- Create: `internal/desktop/lifecycle_test.go`

- [ ] **Step 1: Write a failing lifecycle test**

Create:

```go
package desktop

import (
	"context"
	"runtime"
	"testing"
	"time"
)

func TestRunPublishesSnapshotsAndStops(t *testing.T) {
	service, err := New(testHome(t), runtime.GOOS)
	if err != nil {
		t.Fatal(err)
	}
	updates := make(chan Snapshot, 8)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- service.Run(ctx, func(s Snapshot) { updates <- s }) }()
	service.Trigger()
	select {
	case <-updates:
	case <-time.After(2 * time.Second):
		t.Fatal("no snapshot published")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("service did not stop")
	}
}
```

- [ ] **Step 2: Run it**

```powershell
go test ./internal/desktop -run TestRunPublishesSnapshotsAndStops -v
```

Expected: `Service.Run` missing.

- [ ] **Step 3: Implement lifecycle**

Create `internal/desktop/lifecycle.go`:

```go
package desktop

import (
	"context"

	"github.com/qinqingxu/acsync/internal/scheduler"
)

func (s *Service) Run(ctx context.Context, publish func(Snapshot)) error {
	unsubscribe := s.daemon.Scheduler.Subscribe(func(scheduler.State) {
		if snapshot, err := s.Snapshot(); err == nil {
			publish(snapshot)
		}
	})
	defer unsubscribe()
	previous := s.daemon.OnCycle
	s.daemon.OnCycle = func(result daemon.CycleResult) {
		if previous != nil {
			previous(result)
		}
		if snapshot, err := s.Snapshot(); err == nil {
			publish(snapshot)
		}
	}
	return s.daemon.Run(ctx)
}
```

Add the missing import:

```go
import "github.com/qinqingxu/acsync/internal/daemon"
```

- [ ] **Step 4: Run tests**

```powershell
go test ./internal/desktop -race
```

Expected: pass.

- [ ] **Step 5: Commit**

```powershell
git add internal/desktop/lifecycle*
git commit -m "feat: add desktop service lifecycle"
```

---

### Task 5: Scaffold and pin Wails v3 with React TypeScript

**Files:**
- Create: `Taskfile.yml`
- Create: `build/config.yml`
- Create: `build/**`
- Create: `frontend/**`
- Modify: `go.mod`
- Modify: `go.sum`

- [ ] **Step 1: Install the pinned Wails CLI**

```powershell
go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.8
wails3 doctor
```

Expected: Wails v3.0.0-beta.8 and Windows WebView2/toolchain checks pass.

- [ ] **Step 2: Generate a disposable canonical React template**

Run outside the repository:

```powershell
$tmp = Join-Path $env:TEMP "acsync-wails-template"
if (Test-Path $tmp) { Remove-Item -Recurse -Force $tmp }
wails3 init -n AgentConfigSync -t react-ts -d $tmp
```

Expected: canonical Wails root files, `build/`, and `frontend/` are created.

- [ ] **Step 3: Copy only Wails build/frontend assets**

```powershell
Copy-Item "$tmp\Taskfile.yml" .
Copy-Item "$tmp\build" . -Recurse
Copy-Item "$tmp\frontend" . -Recurse
```

Do not copy the template `main.go`, service, `go.mod`, or README.

- [ ] **Step 4: Pin Go and npm dependencies**

```powershell
go get github.com/wailsapp/wails/v3@v3.0.0-beta.8
cd frontend
npm install
npm install --save-exact react@19.1.1 react-dom@19.1.1
npm install --save-dev --save-exact typescript@5.9.2 vite@7.1.2 vitest@3.2.4 jsdom@26.1.0 @testing-library/react@16.3.0 @testing-library/jest-dom@6.6.4
cd ..
go mod tidy
```

Commit `frontend/package-lock.json`; never leave `@wailsio/runtime` floating
without a lockfile.

- [ ] **Step 5: Set product metadata**

Set `build/config.yml`:

```yaml
version: "3"
name: "AgentConfigSync"
info:
  companyName: "AgentConfigSync"
  productName: "AgentConfigSync"
  productIdentifier: "io.github.qinqingxu.agentconfigsync"
  description: "Synchronize AI agent configuration and sessions"
  copyright: "Copyright © 2026"
  version: "0.1.0"
```

Preserve the generated platform sections below `info`.

- [ ] **Step 6: Verify the untouched scaffold**

```powershell
wails3 generate bindings -ts
npm --prefix frontend test -- --run
wails3 build
```

Expected: template frontend and Wails build succeed.

- [ ] **Step 7: Commit**

```powershell
git add Taskfile.yml build frontend go.mod go.sum
git commit -m "build: scaffold pinned Wails desktop application"
```

---

### Task 6: Compose the Wails window, service, and single instance

**Files:**
- Create: `assets.go`
- Create: `app.go`
- Create: `main.go`
- Create: `internal/desktop/wails.go`

- [ ] **Step 1: Add the embedded assets**

Create `assets.go`:

```go
package main

import "embed"

//go:embed all:frontend/dist
var frontendAssets embed.FS
```

- [ ] **Step 2: Add the Wails binding adapter**

Create `internal/desktop/wails.go`:

```go
package desktop

import (
	"context"

	"github.com/wailsapp/wails/v3/pkg/application"
)

const SnapshotEvent = "desktop:snapshot"

type WailsService struct {
	app     *application.App
	core    *Service
	ctx     context.Context
	cancel  context.CancelFunc
}

func NewWailsService(app *application.App, core *Service) *WailsService {
	return &WailsService{app: app, core: core}
}

func (s *WailsService) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	s.ctx, s.cancel = context.WithCancel(ctx)
	go func() {
		_ = s.core.Run(s.ctx, func(snapshot Snapshot) {
			s.app.Event.Emit(SnapshotEvent, snapshot)
		})
	}()
	return nil
}

func (s *WailsService) ServiceShutdown() error {
	if s.cancel != nil {
		s.cancel()
	}
	return s.core.Close()
}

func (s *WailsService) Snapshot() (Snapshot, error) { return s.core.Snapshot() }
func (s *WailsService) TriggerSync()                { s.core.Trigger() }
func (s *WailsService) Pause()                      { s.core.Pause() }
func (s *WailsService) Resume()                     { s.core.Resume() }
func (s *WailsService) SaveSettings(input SettingsInput) error {
	return s.core.SaveSettings(input)
}
```

- [ ] **Step 3: Compose the Wails application**

Create `app.go`:

```go
package main

import (
	"fmt"
	"runtime"

	"github.com/qinqingxu/acsync/internal/cli"
	"github.com/qinqingxu/acsync/internal/desktop"
	"github.com/qinqingxu/acsync/internal/scheduler"
	legacytray "github.com/qinqingxu/acsync/internal/tray"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

type gui struct {
	app     *application.App
	window  *application.WebviewWindow
	tray    *application.SystemTray
	service *desktop.Service
}

func newGUI() (*gui, error) {
	home, err := cli.Home()
	if err != nil {
		return nil, err
	}
	core, err := desktop.New(home, runtime.GOOS)
	if err != nil {
		return nil, err
	}
	g := &gui{service: core}
	g.app = application.New(application.Options{
		Name: "AgentConfigSync",
		Description: "Synchronize AI agent configuration and sessions",
		Assets: application.AssetOptions{
			Handler: application.AssetFileServerFS(frontendAssets),
		},
		Windows: application.WindowsOptions{DisableQuitOnLastWindowClosed: true},
		Linux: application.LinuxOptions{DisableQuitOnLastWindowClosed: true},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: false,
		},
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID: "io.github.qinqingxu.agentconfigsync",
			OnSecondInstanceLaunch: func(application.SecondInstanceData) {
				if g.window != nil {
					g.show()
				}
			},
		},
	})
	g.app.RegisterService(application.NewService(desktop.NewWailsService(g.app, core)))
	g.window = g.app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name: "main", Title: "AgentConfigSync", URL: "/",
		Width: 980, Height: 700, MinWidth: 760, MinHeight: 560,
	})
	g.window.RegisterHook(events.Common.WindowClosing, func(event *application.WindowEvent) {
		g.window.Hide()
		event.Cancel()
	})
	g.configureTray()
	return g, nil
}

func (g *gui) show() {
	g.window.Show()
	g.window.Restore()
	g.window.Focus()
}

func (g *gui) configureTray() {
	g.tray = g.app.SystemTray.New()
	setState := func(state scheduler.State) {
		icon := legacytray.Icon(state, runtime.GOOS)
		if runtime.GOOS == "darwin" {
			g.tray.SetTemplateIcon(icon)
		} else {
			g.tray.SetIcon(icon)
		}
		g.tray.SetTooltip(fmt.Sprintf("AgentConfigSync — %s", state))
	}
	setState(scheduler.StateIdle)
	g.service.Daemon().Scheduler.Subscribe(setState)
	menu := g.app.NewMenu()
	menu.Add("Open AgentConfigSync").OnClick(func(*application.Context) { g.show() })
	menu.Add("Sync now").OnClick(func(*application.Context) { g.service.Trigger() })
	menu.Add("Pause").OnClick(func(*application.Context) { g.service.Pause() })
	menu.AddSeparator()
	menu.Add("Quit").OnClick(func(*application.Context) { g.app.Quit() })
	g.tray.SetMenu(menu)
	g.tray.OnClick(g.show)
}
```

- [ ] **Step 4: Add the GUI entrypoint**

Create root `main.go`:

```go
package main

import "log"

func main() {
	gui, err := newGUI()
	if err != nil {
		log.Fatal(err)
	}
	if err := gui.app.Run(); err != nil {
		log.Fatal(err)
	}
}
```

- [ ] **Step 5: Generate bindings and build**

```powershell
wails3 generate bindings -ts
wails3 build
go test ./internal/desktop ./internal/scheduler ./internal/daemon -race
```

Expected: GUI binary under `bin/` and tests pass.

- [ ] **Step 6: Commit**

```powershell
git add main.go app.go assets.go internal/desktop/wails.go frontend/bindings
git commit -m "feat: add single-instance Wails desktop shell"
```

---

### Task 7: Define frontend API and dashboard models

**Files:**
- Create: `frontend/src/models.ts`
- Create: `frontend/src/api.ts`
- Create: `frontend/src/test/setup.ts`
- Modify: `frontend/vite.config.ts`
- Modify: `frontend/package.json`

- [ ] **Step 1: Configure Vitest**

Add to `frontend/vite.config.ts`:

```ts
/// <reference types="vitest/config" />
import { defineConfig } from "vite";
import react from "@vitejs/plugin-react";

export default defineConfig({
  plugins: [react()],
  test: {
    environment: "jsdom",
    setupFiles: ["./src/test/setup.ts"],
  },
});
```

Create `frontend/src/test/setup.ts`:

```ts
import "@testing-library/jest-dom/vitest";
```

Set scripts:

```json
{
  "scripts": {
    "dev": "vite",
    "build": "tsc && vite build",
    "test": "vitest"
  }
}
```

- [ ] **Step 2: Define frontend models**

Create `frontend/src/models.ts`:

```ts
export type SyncState = "idle" | "updating" | "error" | "paused";

export interface Agent {
  name: string;
  enabled: boolean;
  exclude: string[];
}

export interface Snapshot {
  configured: boolean;
  state: SyncState;
  repositoryUrl: string;
  intervalMinutes: number;
  trashGraceDays: number;
  agents: Agent[];
  lastSync: string;
  nextSync: string;
  pendingActions: number;
  blockedFiles: number;
  lastError: string;
}

export interface SettingsInput {
  repositoryUrl: string;
  intervalMinutes: number;
  trashGraceDays: number;
  agents: Record<string, boolean>;
}
```

- [ ] **Step 3: Wrap generated Wails bindings**

Create `frontend/src/api.ts` using the exact generated import path:

```ts
import { Events } from "@wailsio/runtime";
import {
  Pause,
  Resume,
  SaveSettings,
  Snapshot,
  TriggerSync,
} from "../bindings/github.com/qinqingxu/acsync/internal/desktop/wailsservice";
import type { SettingsInput, Snapshot as SnapshotModel } from "./models";

export const api = {
  snapshot: () => Snapshot() as Promise<SnapshotModel>,
  trigger: () => TriggerSync(),
  pause: () => Pause(),
  resume: () => Resume(),
  save: (input: SettingsInput) => SaveSettings(input),
  subscribe: (listener: (snapshot: SnapshotModel) => void) =>
    Events.On("desktop:snapshot", listener),
};
```

If generated filenames differ, inspect `frontend/bindings` and update only the
import path; do not change backend method names.

- [ ] **Step 4: Typecheck**

```powershell
npm --prefix frontend run build
```

Expected: pass.

- [ ] **Step 5: Commit**

```powershell
git add frontend
git commit -m "feat: define typed desktop frontend API"
```

---

### Task 8: Implement the approved dashboard

**Files:**
- Replace: `frontend/src/App.tsx`
- Replace: `frontend/src/main.tsx`
- Create: `frontend/src/components/StatusCard.tsx`
- Create: `frontend/src/components/AgentGrid.tsx`
- Create: `frontend/src/components/ActivityPanel.tsx`
- Create: `frontend/src/App.test.tsx`

- [ ] **Step 1: Write the dashboard test**

Create `frontend/src/App.test.tsx`:

```tsx
import { render, screen } from "@testing-library/react";
import { vi } from "vitest";
import App from "./App";

vi.mock("./api", () => ({
  api: {
    snapshot: vi.fn().mockResolvedValue({
      configured: true, state: "idle",
      repositoryUrl: "git@github.com:owner/repo.git",
      intervalMinutes: 10, trashGraceDays: 30,
      agents: [{ name: "claude", enabled: true, exclude: [] }],
      lastSync: "2026-08-13T10:00:00Z",
      nextSync: "2026-08-13T10:10:00Z",
      pendingActions: 0, blockedFiles: 0, lastError: "",
    }),
    subscribe: vi.fn(() => () => {}),
    trigger: vi.fn(), pause: vi.fn(), resume: vi.fn(), save: vi.fn(),
  },
}));

test("renders repository, state, and agent controls", async () => {
  render(<App />);
  expect(await screen.findByText("All synced")).toBeInTheDocument();
  expect(screen.getByText("owner/repo")).toBeInTheDocument();
  expect(screen.getByText("Claude")).toBeInTheDocument();
  expect(screen.getByRole("button", { name: "Sync now" })).toBeInTheDocument();
});
```

- [ ] **Step 2: Run the failing test**

```powershell
npm --prefix frontend test -- --run
```

Expected: fail because dashboard components are absent.

- [ ] **Step 3: Implement status and agent components**

Create `StatusCard.tsx`:

```tsx
import type { Snapshot } from "../models";

const titles = {
  idle: "All synced",
  updating: "Syncing…",
  error: "Sync needs attention",
  paused: "Sync paused",
};

export function StatusCard({ snapshot, onSync, onPause }: {
  snapshot: Snapshot;
  onSync: () => void;
  onPause: () => void;
}) {
  return <section className={`status-card state-${snapshot.state}`}>
    <div className="status-line">
      <span className="status-dot" />
      <div><p className="eyebrow">Sync status</p><h1>{titles[snapshot.state]}</h1></div>
    </div>
    <div className="actions">
      <button className="primary" onClick={onSync} disabled={snapshot.state === "updating"}>
        Sync now
      </button>
      <button onClick={onPause}>{snapshot.state === "paused" ? "Resume" : "Pause"}</button>
    </div>
  </section>;
}
```

Create `AgentGrid.tsx`:

```tsx
import type { Agent } from "../models";

export function AgentGrid({ agents, onToggle }: {
  agents: Agent[];
  onToggle: (name: string, enabled: boolean) => void;
}) {
  return <section className="panel">
    <div className="panel-heading"><h2>Agents</h2><span>{agents.filter(a => a.enabled).length} enabled</span></div>
    <div className="agent-grid">{agents.map(agent =>
      <label className="agent-card" key={agent.name}>
        <span className="agent-mark">{agent.name.slice(0, 1).toUpperCase()}</span>
        <span>{agent.name[0].toUpperCase() + agent.name.slice(1)}</span>
        <input type="checkbox" checked={agent.enabled}
          onChange={event => onToggle(agent.name, event.target.checked)} />
      </label>
    )}</div>
  </section>;
}
```

Create `ActivityPanel.tsx`:

```tsx
import type { Snapshot } from "../models";

function format(value: string) {
  return value ? new Date(value).toLocaleString() : "Never";
}

export function ActivityPanel({ snapshot }: { snapshot: Snapshot }) {
  return <section className="metrics">
    <article><span>Last sync</span><strong>{format(snapshot.lastSync)}</strong></article>
    <article><span>Next sync</span><strong>{format(snapshot.nextSync)}</strong></article>
    <article><span>Pending</span><strong>{snapshot.pendingActions}</strong></article>
    <article><span>Blocked</span><strong>{snapshot.blockedFiles}</strong></article>
  </section>;
}
```

- [ ] **Step 4: Implement `App.tsx`**

```tsx
import { useEffect, useState } from "react";
import { api } from "./api";
import type { Snapshot } from "./models";
import { ActivityPanel } from "./components/ActivityPanel";
import { AgentGrid } from "./components/AgentGrid";
import { StatusCard } from "./components/StatusCard";
import "./styles.css";

function repoName(url: string) {
  return url.replace(/\.git$/, "").split(/[/:]/).slice(-2).join("/");
}

export default function App() {
  const [snapshot, setSnapshot] = useState<Snapshot | null>(null);
  const [error, setError] = useState("");
  useEffect(() => {
    api.snapshot().then(setSnapshot).catch(error => setError(String(error)));
    return api.subscribe(setSnapshot);
  }, []);
  if (error) return <main className="shell"><section className="error-panel">{error}</section></main>;
  if (!snapshot) return <main className="shell"><p>Loading AgentConfigSync…</p></main>;

  async function toggle(name: string, enabled: boolean) {
    const agents = Object.fromEntries(snapshot!.agents.map(a => [a.name, a.name === name ? enabled : a.enabled]));
    await api.save({
      repositoryUrl: snapshot!.repositoryUrl,
      intervalMinutes: snapshot!.intervalMinutes,
      trashGraceDays: snapshot!.trashGraceDays,
      agents,
    });
    setSnapshot(await api.snapshot());
  }

  const paused = snapshot.state === "paused";
  return <main className="shell">
    <header><div><span className="brand-mark">A</span><strong>AgentConfigSync</strong></div><span>{repoName(snapshot.repositoryUrl)}</span></header>
    <StatusCard snapshot={snapshot} onSync={() => api.trigger()}
      onPause={() => paused ? api.resume() : api.pause()} />
    {snapshot.lastError && <section className="error-panel"><strong>Sync failed</strong><p>{snapshot.lastError}</p></section>}
    <ActivityPanel snapshot={snapshot} />
    <AgentGrid agents={snapshot.agents} onToggle={toggle} />
  </main>;
}
```

Set `main.tsx`:

```tsx
import React from "react";
import ReactDOM from "react-dom/client";
import App from "./App";

ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode><App /></React.StrictMode>,
);
```

- [ ] **Step 5: Run tests**

```powershell
npm --prefix frontend test -- --run
```

Expected: pass.

- [ ] **Step 6: Commit**

```powershell
git add frontend/src
git commit -m "feat: add desktop sync dashboard"
```

---

### Task 9: Style the dashboard and add settings

**Files:**
- Create: `frontend/src/components/SettingsPanel.tsx`
- Create: `frontend/src/components/SettingsPanel.test.tsx`
- Modify: `frontend/src/App.tsx`
- Replace: `frontend/src/styles.css`

- [ ] **Step 1: Add settings component test**

```tsx
import { fireEvent, render, screen } from "@testing-library/react";
import { vi } from "vitest";
import { SettingsPanel } from "./SettingsPanel";

test("submits validated settings", () => {
  const save = vi.fn();
  render(<SettingsPanel repositoryUrl="https://github.com/o/r.git"
    intervalMinutes={10} trashGraceDays={30} onSave={save} />);
  fireEvent.change(screen.getByLabelText("Sync interval"), { target: { value: "15" } });
  fireEvent.click(screen.getByRole("button", { name: "Save settings" }));
  expect(save).toHaveBeenCalledWith({
    repositoryUrl: "https://github.com/o/r.git",
    intervalMinutes: 15,
    trashGraceDays: 30,
  });
});
```

- [ ] **Step 2: Implement settings panel**

```tsx
import { FormEvent, useState } from "react";

export function SettingsPanel(props: {
  repositoryUrl: string;
  intervalMinutes: number;
  trashGraceDays: number;
  onSave: (value: { repositoryUrl: string; intervalMinutes: number; trashGraceDays: number }) => void;
}) {
  const [repositoryUrl, setRepositoryUrl] = useState(props.repositoryUrl);
  const [intervalMinutes, setIntervalMinutes] = useState(props.intervalMinutes);
  const [trashGraceDays, setTrashGraceDays] = useState(props.trashGraceDays);
  function submit(event: FormEvent) {
    event.preventDefault();
    props.onSave({ repositoryUrl, intervalMinutes, trashGraceDays });
  }
  return <form className="panel settings" onSubmit={submit}>
    <h2>Settings</h2>
    <label>Repository<input value={repositoryUrl} onChange={e => setRepositoryUrl(e.target.value)} /></label>
    <label>Sync interval<input aria-label="Sync interval" type="number" min={1}
      value={intervalMinutes} onChange={e => setIntervalMinutes(Number(e.target.value))} /></label>
    <label>Trash grace days<input type="number" min={0}
      value={trashGraceDays} onChange={e => setTrashGraceDays(Number(e.target.value))} /></label>
    <button className="primary">Save settings</button>
  </form>;
}
```

- [ ] **Step 3: Integrate settings and approved visual style**

Add a Dashboard/Settings segmented navigation to `App.tsx`. On save, preserve
the current agent map and call `api.save`. Replace `styles.css` with:

```css
:root {
  font-family: Inter, ui-sans-serif, system-ui, -apple-system, "Segoe UI", sans-serif;
  color: #172033;
  background: #f4f7fb;
  font-synthesis: none;
}
* { box-sizing: border-box; }
body { margin: 0; min-width: 720px; min-height: 100vh; }
button, input { font: inherit; }
.shell { max-width: 1080px; margin: 0 auto; padding: 28px 36px 48px; }
header { display:flex; align-items:center; justify-content:space-between; margin-bottom:24px; color:#667085; }
header div { display:flex; align-items:center; gap:10px; color:#172033; }
.brand-mark { display:grid; place-items:center; width:32px; height:32px; border-radius:10px; color:white; background:#356ae6; font-weight:800; }
.status-card, .panel, .metrics article, .error-panel { background:white; border:1px solid #e4e9f2; border-radius:18px; box-shadow:0 8px 30px rgba(23,32,51,.06); }
.status-card { display:flex; align-items:center; justify-content:space-between; padding:28px; }
.status-line { display:flex; align-items:center; gap:16px; }
.status-dot { width:18px; height:18px; border-radius:50%; background:#2e7d32; box-shadow:0 0 0 7px #e8f5e9; }
.state-updating .status-dot { background:#1e90ff; box-shadow:0 0 0 7px #e3f2fd; }
.state-error .status-dot { background:#d32f2f; box-shadow:0 0 0 7px #ffebee; }
.state-paused .status-dot { background:#9e9e9e; box-shadow:0 0 0 7px #f2f2f2; }
.eyebrow { margin:0; color:#7c879c; font-size:12px; font-weight:700; text-transform:uppercase; letter-spacing:.08em; }
h1 { margin:4px 0 0; font-size:28px; }
h2 { margin:0; font-size:18px; }
.actions { display:flex; gap:10px; }
button { border:1px solid #d5dce8; border-radius:10px; padding:10px 16px; background:white; cursor:pointer; }
button.primary { color:white; background:#356ae6; border-color:#356ae6; }
button:disabled { opacity:.55; cursor:not-allowed; }
.metrics { display:grid; grid-template-columns:repeat(4,1fr); gap:14px; margin:18px 0; }
.metrics article { padding:18px; }
.metrics span { display:block; color:#7c879c; font-size:12px; margin-bottom:8px; }
.metrics strong { font-size:15px; }
.panel { padding:24px; }
.panel-heading { display:flex; justify-content:space-between; margin-bottom:18px; }
.panel-heading span { color:#7c879c; }
.agent-grid { display:grid; grid-template-columns:repeat(2,1fr); gap:12px; }
.agent-card { display:grid; grid-template-columns:38px 1fr auto; align-items:center; gap:12px; border:1px solid #e4e9f2; border-radius:13px; padding:14px; }
.agent-mark { display:grid; place-items:center; width:36px; height:36px; border-radius:10px; background:#edf2ff; color:#356ae6; font-weight:800; }
.error-panel { margin:16px 0; padding:16px 20px; border-color:#ffcdd2; background:#fff8f8; color:#9f1d1d; }
.settings { display:grid; gap:16px; }
.settings label { display:grid; gap:7px; color:#667085; }
.settings input { border:1px solid #d5dce8; border-radius:10px; padding:11px; }
@media (max-width: 820px) {
  .metrics { grid-template-columns:repeat(2,1fr); }
  .agent-grid { grid-template-columns:1fr; }
}
```

- [ ] **Step 4: Test and build**

```powershell
npm --prefix frontend test -- --run
npm --prefix frontend run build
wails3 build
```

Expected: all pass.

- [ ] **Step 5: Commit**

```powershell
git add frontend
git commit -m "feat: add desktop settings and polished dashboard"
```

---

### Task 10: Replace the legacy tray entrypoint and verify lifecycle

**Files:**
- Modify: `cmd/acsync/main.go`
- Modify: `internal/autostart/autostart.go`
- Modify: `internal/autostart/autostart_test.go`
- Delete: `internal/tray/tray.go`
- Delete: `internal/settings/serve.go`
- Delete: `internal/settings/serve_test.go`
- Modify: `go.mod`
- Modify: `go.sum`

- [ ] **Step 1: Keep CLI explicitly headless**

Remove `trayCmd()` and the `internal/tray` GUI import from `cmd/acsync/main.go`.
Keep `init`, `sync`, `status`, `daemon`, `install`, and `uninstall` until the
packaging plan migrates autostart.

- [ ] **Step 2: Remove obsolete UI glue**

Delete only:

```text
internal/tray/tray.go
internal/settings/serve.go
internal/settings/serve_test.go
```

Keep `internal/tray/icon.go`, its tests, and pure helpers while Wails reuses the
generated icon. Keep the settings handler temporarily only if CLI compatibility
tests still use it; otherwise remove the entire old settings package in a
follow-up cleanup commit.

- [ ] **Step 3: Remove fyne/systray dependency**

```powershell
go mod tidy
```

Expected: `fyne.io/systray` and its no-longer-used indirect dependencies leave
`go.mod`.

- [ ] **Step 4: Run complete validation**

```powershell
go test ./... -race
go vet ./...
npm --prefix frontend test -- --run
npm --prefix frontend run build
wails3 build
go build ./cmd/acsync
```

Expected: all pass.

- [ ] **Step 5: Manual desktop lifecycle smoke**

Run:

```powershell
wails3 dev
```

Verify:

1. dashboard opens without a console window;
2. second launch focuses the first window;
3. closing the window leaves the tray running;
4. tray click reopens the window;
5. Sync now updates window and tray state;
6. Pause/Resume updates immediately;
7. changing interval takes effect without restart;
8. Quit stops the process.

- [ ] **Step 6: Commit**

```powershell
git add -A
git commit -m "refactor: make Wails the desktop application entrypoint"
```

---

## Done Criteria

- The root Wails GUI and `cmd/acsync` headless CLI build independently.
- One Wails process owns one scheduler, one dashboard, and one system tray.
- Second launch focuses the existing window.
- Window close hides to tray; tray Quit exits.
- Dashboard shows live idle/updating/error/paused state and cycle results.
- Settings save through typed Go bindings and interval changes apply live.
- Existing sync behavior and CLI tests remain green.
- Wails and frontend dependencies are pinned and reproducible.
