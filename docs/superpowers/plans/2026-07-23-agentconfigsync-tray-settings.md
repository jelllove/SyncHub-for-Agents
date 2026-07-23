# AgentConfigSync Tray & Settings UI Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Give `acsync` a desktop presence — a system-tray icon that reflects sync state (idle/updating/error/paused), a right-click menu (sync now / pause / settings / open logs / quit) with per-agent enable toggles, and a local web settings page for editing config.

**Architecture:** All GUI-independent logic is pure and unit-tested: tray icons are generated in-process (solid-color PNG, wrapped in ICO on Windows), and the settings page is a stdlib `net/http` handler tested via `httptest`. The thin GUI glue (`fyne.io/systray`) lives in one file and builds an `App` that wraps the Plan 2 `daemon.Daemon`: it starts the daemon in a goroutine, overrides `Scheduler.OnState` to repaint the icon, serves the settings page on `127.0.0.1`, and maps menu clicks to `Trigger`/`Pause`/`Resume`, config writes, and browser/log launches.

**Tech Stack:** Go 1.22+, `fyne.io/systray` (tray), stdlib `net/http` + `html/template` + `image/png` (settings + icons). Builds on Plan 1 (`config`, `cli`, `provider`) and Plan 2 (`daemon`, `scheduler`).

**Prerequisite:** Plans 1 and 2 must be fully implemented with green tests. This plan imports `internal/cli`, `internal/config`, `internal/daemon`, and `internal/scheduler`.

> **Platform note:** `fyne.io/systray` is pure Go on **Windows** (no cgo) but requires cgo and native toolchains/libraries on **macOS** (Cocoa) and **Linux** (GTK/AppIndicator: `libgtk-3-dev`, `libayatana-appindicator3-dev`). Therefore the tray binary must be built natively on each OS; the from-Windows three-way cross-compile from Plans 1–2 does **not** apply to the tray. `go build ./...` / `go test ./...` work on Windows as-is.

---

## File Structure

```
AgentConfigSync/
  cmd/acsync/
    main.go                   # MODIFY: add `tray` command
  internal/
    autostart/
      autostart.go            # MODIFY: launch `acsync tray` instead of `daemon`
      autostart_test.go       # MODIFY: expectations daemon -> tray
    settings/
      settings.go             # NEW: ViewModel + http.Handler (GET page, POST /save)
      settings_test.go
      serve.go                # NEW: Serve — bind 127.0.0.1 background server
      serve_test.go
    tray/
      icon.go                 # NEW: state -> color -> PNG -> (ICO on Windows)
      icon_test.go
      menu.go                 # NEW: pure helpers (pauseTitle, openCommand)
      menu_test.go
      tray.go                 # NEW: App + systray glue (build-only, no unit test)
```

**Responsibilities & boundaries:**
- `settings` — pure config view-model + HTTP handlers; the only new config-writing UI. No tray/daemon knowledge.
- `tray/icon.go` — deterministic image generation from a `scheduler.State`; no I/O.
- `tray/menu.go` — pure decision helpers (menu titles, per-OS open command).
- `tray/tray.go` — the *only* file importing `fyne.io/systray`; wires an `App` to a `daemon.Daemon`. Not unit-tested (GUI); covered by build + manual smoke.
- `autostart` (from Plan 2) — retargeted to launch the tray so the desktop icon appears at login.

---

## Task 1: Settings view-model and HTTP handler

**Files:**
- Create: `internal/settings/settings.go`
- Test: `internal/settings/settings_test.go`

`BuildViewModel` merges the saved config with the known providers (builtins + user) so every agent gets a checkbox. `Handler` renders that page at `/` and applies edits at `POST /save`. Config read/write reuses Plan 1's `config` package; the agent list comes from `cli.LoadProviders`.

- [ ] **Step 1: Write the failing test**

Create `internal/settings/settings_test.go`:

```go
package settings

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/qinqingxu/acsync/internal/cli"
	"github.com/qinqingxu/acsync/internal/config"
)

func setupHome(t *testing.T) string {
	t.Helper()
	home := filepath.Join(t.TempDir(), ".acsync")
	if err := os.MkdirAll(cli.ProvidersDir(home), 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(cli.ProvidersDir(home), "demo.yaml"),
		[]byte("name: demo\nconfig:\n  paths:\n    linux: ~/.demo\n  include:\n    - settings.json\n  exclude:\n    - \"**/secret*\"\n"), 0o644)
	if err := config.Save(cli.ConfigPath(home), config.Config{
		RepoURL:             "https://example.com/data.git",
		SyncIntervalMinutes: 10,
		TrashGraceDays:      30,
		Agents:              map[string]bool{"demo": true},
	}); err != nil {
		t.Fatal(err)
	}
	return home
}

func TestHandlerGETRendersAgents(t *testing.T) {
	home := setupHome(t)
	srv := httptest.NewServer(Handler(home))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	s := string(body)
	if !strings.Contains(s, "demo") {
		t.Error("page should list the demo agent")
	}
	if !strings.Contains(s, "https://example.com/data.git") {
		t.Error("page should show the repo URL")
	}
	if !strings.Contains(s, "**/secret*") {
		t.Error("page should show the exclude rule")
	}
}

func TestHandlerSaveUpdatesConfig(t *testing.T) {
	home := setupHome(t)
	srv := httptest.NewServer(Handler(home))
	defer srv.Close()

	form := url.Values{}
	form.Set("repo_url", "https://example.com/new.git")
	form.Set("sync_interval_minutes", "15")
	form.Set("trash_grace_days", "45")
	// no agent_demo field => demo becomes disabled

	resp, err := http.PostForm(srv.URL+"/save", form)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status after save = %d", resp.StatusCode)
	}

	cfg, err := config.Load(cli.ConfigPath(home))
	if err != nil {
		t.Fatal(err)
	}
	if cfg.RepoURL != "https://example.com/new.git" {
		t.Errorf("repo url = %q", cfg.RepoURL)
	}
	if cfg.SyncIntervalMinutes != 15 {
		t.Errorf("interval = %d", cfg.SyncIntervalMinutes)
	}
	if cfg.TrashGraceDays != 45 {
		t.Errorf("grace = %d", cfg.TrashGraceDays)
	}
	if cfg.Agents["demo"] {
		t.Error("demo should be disabled after saving with its checkbox unchecked")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/settings/ -run TestHandler -v`
Expected: FAIL — `undefined: Handler`.

- [ ] **Step 3: Write minimal implementation**

Create `internal/settings/settings.go`:

```go
// Package settings serves a local web page for editing acsync configuration.
package settings

import (
	"html/template"
	"net/http"
	"sort"
	"strconv"

	"github.com/qinqingxu/acsync/internal/cli"
	"github.com/qinqingxu/acsync/internal/config"
)

// AgentView is one agent row in the settings page.
type AgentView struct {
	Name    string
	Enabled bool
	Exclude []string
}

// ViewModel is the data rendered by the settings page.
type ViewModel struct {
	RepoURL             string
	SyncIntervalMinutes int
	TrashGraceDays      int
	Agents              []AgentView
}

// BuildViewModel merges the saved config with the known providers.
func BuildViewModel(home string) (ViewModel, error) {
	cfg, err := config.Load(cli.ConfigPath(home))
	if err != nil {
		return ViewModel{}, err
	}
	providers, err := cli.LoadProviders(home)
	if err != nil {
		return ViewModel{}, err
	}
	vm := ViewModel{
		RepoURL:             cfg.RepoURL,
		SyncIntervalMinutes: cfg.SyncIntervalMinutes,
		TrashGraceDays:      cfg.TrashGraceDays,
	}
	for _, p := range providers {
		vm.Agents = append(vm.Agents, AgentView{
			Name:    p.Name,
			Enabled: cfg.Agents[p.Name],
			Exclude: p.Config.Exclude,
		})
	}
	sort.Slice(vm.Agents, func(i, j int) bool { return vm.Agents[i].Name < vm.Agents[j].Name })
	return vm, nil
}

// Handler returns the settings HTTP handler for the given acsync home.
func Handler(home string) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		vm, err := BuildViewModel(home)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := pageTemplate.Execute(w, vm); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})
	mux.HandleFunc("/save", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if err := saveForm(home, r); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		vm, err := BuildViewModel(home)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		if err := pageTemplate.Execute(w, vm); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
		}
	})
	return mux
}

func saveForm(home string, r *http.Request) error {
	if err := r.ParseForm(); err != nil {
		return err
	}
	cfg, err := config.Load(cli.ConfigPath(home))
	if err != nil {
		return err
	}
	cfg.RepoURL = r.FormValue("repo_url")
	if v, err := strconv.Atoi(r.FormValue("sync_interval_minutes")); err == nil {
		cfg.SyncIntervalMinutes = v
	}
	if v, err := strconv.Atoi(r.FormValue("trash_grace_days")); err == nil {
		cfg.TrashGraceDays = v
	}
	providers, err := cli.LoadProviders(home)
	if err != nil {
		return err
	}
	if cfg.Agents == nil {
		cfg.Agents = map[string]bool{}
	}
	for _, p := range providers {
		cfg.Agents[p.Name] = r.FormValue("agent_"+p.Name) == "on"
	}
	return config.Save(cli.ConfigPath(home), cfg)
}

var pageTemplate = template.Must(template.New("page").Parse(`<!doctype html>
<html>
<head><meta charset="utf-8"><title>acsync settings</title></head>
<body>
<h1>AgentConfigSync Settings</h1>
<form method="POST" action="/save">
<p><label>Repo URL: <input name="repo_url" value="{{.RepoURL}}" size="60"></label></p>
<p><label>Sync interval (minutes): <input name="sync_interval_minutes" type="number" min="1" value="{{.SyncIntervalMinutes}}"></label></p>
<p><label>Trash grace (days): <input name="trash_grace_days" type="number" min="0" value="{{.TrashGraceDays}}"></label></p>
<h2>Agents</h2>
{{range .Agents}}
<fieldset>
<label><input type="checkbox" name="agent_{{.Name}}" {{if .Enabled}}checked{{end}}> {{.Name}}</label>
{{if .Exclude}}<div>Excluded: {{range .Exclude}}<code>{{.}}</code> {{end}}</div>{{end}}
</fieldset>
{{end}}
<p><button type="submit">Save</button></p>
</form>
</body>
</html>
`))
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/settings/ -run TestHandler -v`
Expected: PASS (both tests).

- [ ] **Step 5: Commit**

```bash
git add internal/settings/settings.go internal/settings/settings_test.go
git commit -m "feat: add settings web page handler"
```

---

## Task 2: `settings.Serve` — background localhost server

**Files:**
- Create: `internal/settings/serve.go`
- Test: `internal/settings/serve_test.go`

`Serve` binds a `net/http.Server` to `127.0.0.1:0` (a free port), serves in a goroutine, and returns the bound address plus a shutdown function. The tray uses the returned URL for its "Settings…" menu item. It reuses the `setupHome` helper from `settings_test.go` (same package).

- [ ] **Step 1: Write the failing test**

Create `internal/settings/serve_test.go`:

```go
package settings

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestServeRespondsThenShutsDown(t *testing.T) {
	home := setupHome(t)

	addr, shutdown, err := Serve(home)
	if err != nil {
		t.Fatalf("Serve error: %v", err)
	}
	if !strings.HasPrefix(addr, "127.0.0.1:") {
		t.Fatalf("addr = %q, want 127.0.0.1:<port>", addr)
	}

	resp, err := http.Get("http://" + addr + "/")
	if err != nil {
		t.Fatalf("GET error: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if !strings.Contains(string(body), "AgentConfigSync Settings") {
		t.Error("body should contain the settings heading")
	}

	if err := shutdown(context.Background()); err != nil {
		t.Fatalf("shutdown error: %v", err)
	}
	if _, err := http.Get("http://" + addr + "/"); err == nil {
		t.Error("server should be down after shutdown")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/settings/ -run TestServe -v`
Expected: FAIL — `undefined: Serve`.

- [ ] **Step 3: Write minimal implementation**

Create `internal/settings/serve.go`:

```go
package settings

import (
	"context"
	"net"
	"net/http"
)

// Serve starts the settings server on 127.0.0.1 (a free port). It returns the
// bound address and a shutdown function.
func Serve(home string) (string, func(context.Context) error, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", nil, err
	}
	srv := &http.Server{Handler: Handler(home)}
	go func() { _ = srv.Serve(ln) }()
	return ln.Addr().String(), srv.Shutdown, nil
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/settings/ -run TestServe -v`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/settings/serve.go internal/settings/serve_test.go
git commit -m "feat: add settings background server"
```

---

## Task 3: Tray icon generation

**Files:**
- Create: `internal/tray/icon.go`
- Test: `internal/tray/icon_test.go`

Icons are generated in-process — no binary assets. Each `scheduler.State` maps to a color; `renderPNG` draws a solid square; `pngToICO` wraps it in a single-image ICO container (Windows needs ICO, other OSes accept PNG). `Icon(state, goos)` returns the right format.

- [ ] **Step 1: Write the failing test**

Create `internal/tray/icon_test.go`:

```go
package tray

import (
	"bytes"
	"image/color"
	"image/png"
	"testing"

	"github.com/qinqingxu/acsync/internal/scheduler"
)

func TestRenderPNGIsSolidColor(t *testing.T) {
	data := renderPNG(color.RGBA{10, 20, 30, 255}, 8)
	img, err := png.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if b := img.Bounds(); b.Dx() != 8 || b.Dy() != 8 {
		t.Fatalf("size = %dx%d, want 8x8", b.Dx(), b.Dy())
	}
	r, g, b, a := img.At(4, 4).RGBA()
	if uint8(r>>8) != 10 || uint8(g>>8) != 20 || uint8(b>>8) != 30 || uint8(a>>8) != 255 {
		t.Errorf("center pixel = %d,%d,%d,%d", uint8(r>>8), uint8(g>>8), uint8(b>>8), uint8(a>>8))
	}
}

func TestPngToICOHeader(t *testing.T) {
	ico := pngToICO(renderPNG(colorFor(scheduler.StateIdle), 32), 32)
	// ICONDIR: reserved=0, type=1 (icon), count=1
	if ico[0] != 0 || ico[1] != 0 || ico[2] != 1 || ico[3] != 0 || ico[4] != 1 || ico[5] != 0 {
		t.Fatalf("bad ICO header: % x", ico[:6])
	}
}

func TestIconDiffersByState(t *testing.T) {
	idle := Icon(scheduler.StateIdle, "linux")
	fail := Icon(scheduler.StateError, "linux")
	if bytes.Equal(idle, fail) {
		t.Error("idle and error icons should differ")
	}
}

func TestIconWindowsIsICO(t *testing.T) {
	ico := Icon(scheduler.StateIdle, "windows")
	if len(ico) < 6 || ico[2] != 1 {
		t.Error("windows icon should be ICO format")
	}
	png := Icon(scheduler.StateIdle, "linux")
	if len(png) < 8 || png[0] != 0x89 || png[1] != 'P' {
		t.Error("non-windows icon should be PNG format")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tray/ -run "TestRenderPNG|TestPngToICO|TestIcon" -v`
Expected: FAIL — `undefined: renderPNG`.

- [ ] **Step 3: Write minimal implementation**

Create `internal/tray/icon.go`:

```go
// Package tray renders status icons and runs the system-tray application.
package tray

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"

	"github.com/qinqingxu/acsync/internal/scheduler"
)

func colorFor(s scheduler.State) color.RGBA {
	switch s {
	case scheduler.StateUpdating:
		return color.RGBA{0x1e, 0x90, 0xff, 0xff} // blue
	case scheduler.StateError:
		return color.RGBA{0xd3, 0x2f, 0x2f, 0xff} // red
	case scheduler.StatePaused:
		return color.RGBA{0x9e, 0x9e, 0x9e, 0xff} // gray
	default:
		return color.RGBA{0x2e, 0x7d, 0x32, 0xff} // green (idle)
	}
}

func renderPNG(c color.RGBA, size int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			img.SetRGBA(x, y, c)
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}

// pngToICO wraps a square PNG (side=size) in a single-image ICO container.
// Windows Vista+ accepts PNG-compressed icon images.
func pngToICO(pngData []byte, size int) []byte {
	var buf bytes.Buffer
	// ICONDIR
	binary.Write(&buf, binary.LittleEndian, uint16(0)) // reserved
	binary.Write(&buf, binary.LittleEndian, uint16(1)) // type: 1 = icon
	binary.Write(&buf, binary.LittleEndian, uint16(1)) // image count
	// ICONDIRENTRY
	dim := byte(size)
	if size >= 256 {
		dim = 0 // 0 means 256
	}
	buf.WriteByte(dim)                                            // width
	buf.WriteByte(dim)                                            // height
	buf.WriteByte(0)                                             // palette size
	buf.WriteByte(0)                                             // reserved
	binary.Write(&buf, binary.LittleEndian, uint16(1))           // color planes
	binary.Write(&buf, binary.LittleEndian, uint16(32))          // bits per pixel
	binary.Write(&buf, binary.LittleEndian, uint32(len(pngData))) // image size
	binary.Write(&buf, binary.LittleEndian, uint32(22))          // offset (6 + 16)
	buf.Write(pngData)
	return buf.Bytes()
}

// Icon returns the tray icon bytes for a state, in the format the OS expects
// (ICO on Windows, PNG elsewhere).
func Icon(s scheduler.State, goos string) []byte {
	const size = 32
	pngData := renderPNG(colorFor(s), size)
	if goos == "windows" {
		return pngToICO(pngData, size)
	}
	return pngData
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/tray/ -run "TestRenderPNG|TestPngToICO|TestIcon" -v`
Expected: PASS (all four tests).

- [ ] **Step 5: Commit**

```bash
git add internal/tray/icon.go internal/tray/icon_test.go
git commit -m "feat: add generated tray status icons"
```

---

## Task 4: Tray pure menu helpers

**Files:**
- Create: `internal/tray/menu.go`
- Test: `internal/tray/menu_test.go`

Two pure helpers the GUI glue depends on: the pause/resume menu label and the per-OS command to open a URL or folder.

- [ ] **Step 1: Write the failing test**

Create `internal/tray/menu_test.go`:

```go
package tray

import (
	"reflect"
	"testing"
)

func TestPauseTitle(t *testing.T) {
	if got := pauseTitle(false); got != "Pause" {
		t.Errorf("running -> %q, want Pause", got)
	}
	if got := pauseTitle(true); got != "Resume" {
		t.Errorf("paused -> %q, want Resume", got)
	}
}

func TestOpenCommand(t *testing.T) {
	cases := []struct {
		goos   string
		target string
		name   string
		args   []string
	}{
		{"windows", "http://x/", "rundll32", []string{"url.dll,FileProtocolHandler", "http://x/"}},
		{"darwin", "/logs", "open", []string{"/logs"}},
		{"linux", "/logs", "xdg-open", []string{"/logs"}},
	}
	for _, c := range cases {
		name, args := openCommand(c.goos, c.target)
		if name != c.name || !reflect.DeepEqual(args, c.args) {
			t.Errorf("openCommand(%q) = %q %v, want %q %v", c.goos, name, args, c.name, c.args)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/tray/ -run "TestPauseTitle|TestOpenCommand" -v`
Expected: FAIL — `undefined: pauseTitle`.

- [ ] **Step 3: Write minimal implementation**

Create `internal/tray/menu.go`:

```go
package tray

func pauseTitle(paused bool) string {
	if paused {
		return "Resume"
	}
	return "Pause"
}

// openCommand returns the command + args to open a URL or folder on goos.
func openCommand(goos, target string) (string, []string) {
	switch goos {
	case "windows":
		return "rundll32", []string{"url.dll,FileProtocolHandler", target}
	case "darwin":
		return "open", []string{target}
	default:
		return "xdg-open", []string{target}
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/tray/ -run "TestPauseTitle|TestOpenCommand" -v`
Expected: PASS (both tests).

- [ ] **Step 5: Commit**

```bash
git add internal/tray/menu.go internal/tray/menu_test.go
git commit -m "feat: add tray menu helper functions"
```

---

## Task 5: Tray application (systray glue)

**Files:**
- Create: `internal/tray/tray.go`

This is the only file importing `fyne.io/systray`. It is **not** unit-tested (systray needs a real desktop session); correctness is covered by `go build`/`go vet` and the manual smoke test in Task 6. It builds an `App` around a Plan 2 `daemon.Daemon`: starts the daemon in a goroutine, repaints the icon on every scheduler state change, serves the settings page, and maps menu clicks.

- [ ] **Step 1: Add the systray dependency**

Run: `go get fyne.io/systray@v1.12.2`
Expected: adds `fyne.io/systray` to `go.mod`/`go.sum`.

- [ ] **Step 2: Write the implementation**

Create `internal/tray/tray.go`:

```go
package tray

import (
	"context"
	"os/exec"

	"fyne.io/systray"

	"github.com/qinqingxu/acsync/internal/cli"
	"github.com/qinqingxu/acsync/internal/config"
	"github.com/qinqingxu/acsync/internal/daemon"
	"github.com/qinqingxu/acsync/internal/scheduler"
	"github.com/qinqingxu/acsync/internal/settings"
)

// App binds a daemon to a system-tray UI.
type App struct {
	Home string
	GOOS string

	d                *daemon.Daemon
	cancel           context.CancelFunc
	settingsURL      string
	shutdownSettings func(context.Context) error
}

type agentItem struct {
	name string
	item *systray.MenuItem
}

// Run builds a daemon and runs the tray until the user quits.
func Run(home, goos string) error {
	d, err := daemon.New(home, goos)
	if err != nil {
		return err
	}
	app := &App{Home: home, GOOS: goos, d: d}
	systray.Run(app.onReady, app.onExit)
	return nil
}

func (a *App) onReady() {
	systray.SetTitle("acsync")
	systray.SetTooltip("AgentConfigSync")
	systray.SetIcon(Icon(scheduler.StateIdle, a.GOOS))

	// Repaint the icon (and keep logging) on every state change.
	a.d.Scheduler.OnState = func(s scheduler.State) {
		a.d.Logger.Printf("state: %s", s)
		systray.SetIcon(Icon(s, a.GOOS))
	}

	if addr, shutdown, err := settings.Serve(a.Home); err == nil {
		a.settingsURL = "http://" + addr + "/"
		a.shutdownSettings = shutdown
	} else {
		a.d.Logger.Printf("settings server error: %v", err)
	}

	mSync := systray.AddMenuItem("Sync now", "Trigger a sync immediately")
	mPause := systray.AddMenuItem("Pause", "Pause syncing")
	mSettings := systray.AddMenuItem("Settings…", "Open the settings page")
	mLogs := systray.AddMenuItem("Open logs", "Open the log folder")
	systray.AddSeparator()
	agents := a.addAgentItems()
	systray.AddSeparator()
	mQuit := systray.AddMenuItem("Quit", "Stop acsync")

	ctx, cancel := context.WithCancel(context.Background())
	a.cancel = cancel
	go a.d.Run(ctx)
	go a.handleMenu(mSync, mPause, mSettings, mLogs, mQuit, agents)
}

func (a *App) addAgentItems() []agentItem {
	cfg, err := config.Load(cli.ConfigPath(a.Home))
	if err != nil {
		a.d.Logger.Printf("load config for menu: %v", err)
		return nil
	}
	providers, err := cli.LoadProviders(a.Home)
	if err != nil {
		a.d.Logger.Printf("load providers for menu: %v", err)
		return nil
	}
	var items []agentItem
	for _, p := range providers {
		it := systray.AddMenuItemCheckbox(p.Name, "Sync "+p.Name, cfg.Agents[p.Name])
		items = append(items, agentItem{name: p.Name, item: it})
	}
	return items
}

func (a *App) handleMenu(mSync, mPause, mSettings, mLogs, mQuit *systray.MenuItem, agents []agentItem) {
	for _, ai := range agents {
		ai := ai
		go func() {
			for range ai.item.ClickedCh {
				on := !ai.item.Checked()
				if on {
					ai.item.Check()
				} else {
					ai.item.Uncheck()
				}
				a.toggleAgent(ai.name, on)
			}
		}()
	}
	for {
		select {
		case <-mSync.ClickedCh:
			a.d.Scheduler.Trigger()
		case <-mPause.ClickedCh:
			if a.d.Scheduler.State() == scheduler.StatePaused {
				a.d.Scheduler.Resume()
			} else {
				a.d.Scheduler.Pause()
			}
			mPause.SetTitle(pauseTitle(a.d.Scheduler.State() == scheduler.StatePaused))
		case <-mSettings.ClickedCh:
			a.openTarget(a.settingsURL)
		case <-mLogs.ClickedCh:
			a.openTarget(cli.LogsDir(a.Home))
		case <-mQuit.ClickedCh:
			systray.Quit()
			return
		}
	}
}

func (a *App) toggleAgent(name string, on bool) {
	cfg, err := config.Load(cli.ConfigPath(a.Home))
	if err != nil {
		a.d.Logger.Printf("toggle load error: %v", err)
		return
	}
	if cfg.Agents == nil {
		cfg.Agents = map[string]bool{}
	}
	cfg.Agents[name] = on
	if err := config.Save(cli.ConfigPath(a.Home), cfg); err != nil {
		a.d.Logger.Printf("toggle save error: %v", err)
	}
}

func (a *App) openTarget(target string) {
	if target == "" {
		return
	}
	name, args := openCommand(a.GOOS, target)
	if err := exec.Command(name, args...).Start(); err != nil {
		a.d.Logger.Printf("open %q error: %v", target, err)
	}
}

func (a *App) onExit() {
	if a.cancel != nil {
		a.cancel()
	}
	if a.shutdownSettings != nil {
		_ = a.shutdownSettings(context.Background())
	}
}
```

- [ ] **Step 3: Verify it builds and vets**

Run: `go build ./internal/tray/ && go vet ./internal/tray/`
Expected: no output (success). On Windows this compiles without cgo.

- [ ] **Step 4: Commit**

```bash
git add internal/tray/tray.go go.mod go.sum
git commit -m "feat: add system tray application"
```

---

## Task 6: Wire the `tray` command, retarget autostart, and verify

**Files:**
- Modify: `cmd/acsync/main.go` (add the `tray` command)
- Modify: `internal/autostart/autostart.go` (launch `acsync tray` instead of `daemon`)
- Modify: `internal/autostart/autostart_test.go` (update expectations)

- [ ] **Step 1: Add the `tray` import and command to `cmd/acsync/main.go`**

Add the import (in the existing import block):

```go
	"github.com/qinqingxu/acsync/internal/tray"
```

Change the `root.AddCommand(...)` line to include the tray command:

```go
	root.AddCommand(initCmd(), syncCmd(), statusCmd(), daemonCmd(), trayCmd(), installCmd(), uninstallCmd())
```

Add this command constructor alongside the others:

```go
func trayCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "tray",
		Short: "Run acsync with a system-tray icon (daemon + UI)",
		RunE: func(cmd *cobra.Command, args []string) error {
			home, err := cli.Home()
			if err != nil {
				return err
			}
			return tray.Run(home, runtime.GOOS)
		},
	}
}
```

- [ ] **Step 2: Retarget autostart to launch the tray**

In `internal/autostart/autostart.go`, change the three content generators from launching `daemon` to launching `tray`.

In `windowsCmd`, replace:

```go
	return "@echo off\r\nstart \"\" \"" + execPath + "\" daemon\r\n"
```

with:

```go
	return "@echo off\r\nstart \"\" \"" + execPath + "\" tray\r\n"
```

In `launchAgentPlist`, replace the argument line:

```go
		<string>daemon</string>
```

with:

```go
		<string>tray</string>
```

In `systemdUnit`, replace:

```go
ExecStart=` + execPath + ` daemon
```

with:

```go
ExecStart=` + execPath + ` tray
```

- [ ] **Step 3: Update the autostart test expectations**

In `internal/autostart/autostart_test.go`, update the four `daemon` expectations to `tray`:

- In `TestContentGenerators`: `start "" "C:\acsync.exe" daemon` → `start "" "C:\acsync.exe" tray`, and `ExecStart=/usr/local/bin/acsync daemon` → `ExecStart=/usr/local/bin/acsync tray`.
- In `TestEnableDisableLinux`: `ExecStart=/opt/acsync daemon` → `ExecStart=/opt/acsync tray`.
- In `TestEnableWindowsWritesCmd`: `"C:\acsync.exe" daemon` → `"C:\acsync.exe" tray`.

The exact replacements:

```go
	// TestContentGenerators
	if got := windowsCmd(`C:\acsync.exe`); !strings.Contains(got, `start "" "C:\acsync.exe" tray`) {
		t.Errorf("windows cmd = %q", got)
	}
	...
	unit := systemdUnit("/usr/local/bin/acsync")
	if !strings.Contains(unit, "ExecStart=/usr/local/bin/acsync tray") {
		t.Errorf("unit = %q", unit)
	}
```

```go
	// TestEnableDisableLinux
	if !strings.Contains(string(data), "ExecStart=/opt/acsync tray") {
		t.Errorf("unit body = %q", string(data))
	}
```

```go
	// TestEnableWindowsWritesCmd
	if !strings.Contains(string(data), `"C:\acsync.exe" tray`) {
		t.Errorf("cmd body = %q", string(data))
	}
```

- [ ] **Step 4: Tidy, vet, and build**

Run: `go mod tidy && go vet ./... && go build ./...`
Expected: no output (success) on Windows.

- [ ] **Step 5: Run the full test suite**

Run: `go test ./...`
Expected: all packages `ok`, including `internal/settings`, `internal/tray`, and the updated `internal/autostart`.

- [ ] **Step 6: Verify the `tray` command is registered**

Run: `go run ./cmd/acsync --help`
Expected: usage now lists `tray` alongside `init`, `sync`, `status`, `daemon`, `install`, `uninstall`.

- [ ] **Step 7: Manual desktop smoke test (interactive — run on a machine with a desktop session)**

Run: `go run ./cmd/acsync tray`
Expected:
- A tray icon appears (green when idle).
- Right-click shows: Sync now, Pause, Settings…, Open logs, per-agent checkboxes, Quit.
- "Sync now" turns the icon blue briefly (updating); errors turn it red.
- "Settings…" opens the browser to the local settings page; toggling an agent + Save updates `~/.acsync/config.yaml`.
- "Pause" flips to "Resume" and the icon turns gray; "Quit" exits.

- [ ] **Step 8: Commit**

```bash
git add cmd/acsync/main.go internal/autostart/autostart.go internal/autostart/autostart_test.go
git commit -m "feat: wire tray command and launch tray at login"
```

---

## Done Criteria (Plan 3)

- `go test ./...` passes for all Plan 1 + Plan 2 + Plan 3 packages (tray icons, menu helpers, settings handler/server, updated autostart).
- `acsync tray` shows a status icon that changes with sync state (green/blue/red/gray), a right-click menu (sync now / pause-resume / settings / open logs / quit), and per-agent enable checkboxes that persist to config.
- The "Settings…" item opens a local web page (`127.0.0.1`) for editing repo URL, sync interval, trash grace, and per-agent enable — saved back to `~/.acsync/config.yaml`.
- `acsync install` now registers `acsync tray` for login start (desktop), so the icon appears automatically.

**Deliberately deferred spec details (tracked, not lost):**
- **Interval change without restart:** the daemon reads `SyncIntervalMinutes` once at startup, so editing the interval in Settings applies on the next `acsync tray`/`daemon` restart. Agent enable/disable and repo URL take effect on the next sync cycle (each cycle reloads config). Live interval reconfiguration is a small future enhancement (recreate the scheduler ticker on change) and is out of scope here.
- **Headless hosts:** autostart launches `acsync tray`, which needs a desktop session. On a headless Linux host, run `acsync daemon` (still available) or edit the generated unit's `ExecStart` from `tray` to `daemon`.
- **Secret exclude rule editing:** the settings page shows each agent's exclude globs read-only (spec §5.8 lists "查看" / viewing). Editing exclude rules is done by editing the provider YAML directly; an in-UI editor is a future enhancement.

---

## Project Complete

With Plans 1–3 implemented, `acsync` delivers the full design: multi-agent config/session sync through a private GitHub repo, secret filtering, three-way delete propagation with a 30-day trash grace period, a 10-minute scheduler with manual trigger/pause, cross-platform autostart, a status-icon system tray, and a local settings UI.
