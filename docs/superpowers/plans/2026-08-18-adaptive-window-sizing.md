# Adaptive Main Window Sizing Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Size and center the AgentConfigSync main window from the primary display's DPI-aware work area so large screens show the complete dashboard and small screens keep the whole native window visible with scrollable content.

**Architecture:** Add a pure sizing policy in the root desktop package, then use a small window-options factory to map that policy into Wails `WebviewWindowOptions`. The frontend retains its existing responsive layout but explicitly permits document-level vertical scrolling. No window geometry is persisted or synchronized.

**Tech Stack:** Go, Wails v3 `application.Screen` and `WebviewWindowOptions`, React CSS, Go testing, Vite, NSIS

---

## File structure

- Create `window_size.go`: constants, result type, pure sizing calculation, and Wails main-window options factory.
- Create `window_size_test.go`: table-driven sizing tests plus Wails option-mapping tests.
- Modify `app.go`: obtain the primary screen, log invalid display information, and create the main window from the options factory.
- Modify `frontend/src/style.css`: explicitly allow the document root to grow and scroll vertically in short windows.
- Verify `scripts/smoke/windows.ps1`: reuse the existing clean install, launch, single-instance, and uninstall smoke workflow without modifying it.

### Task 1: Add the adaptive sizing policy

**Files:**
- Create: `window_size.go`
- Create: `window_size_test.go`

- [ ] **Step 1: Write the failing table-driven sizing test**

Create `window_size_test.go`:

```go
package main

import "testing"

func TestCalculateMainWindowSize(t *testing.T) {
	tests := []struct {
		name                  string
		workWidth, workHeight int
		want                  mainWindowSize
	}{
		{
			name:       "large display uses preferred size",
			workWidth:  2560,
			workHeight: 1400,
			want:       mainWindowSize{Width: 1200, Height: 850, MinWidth: 640, MinHeight: 480},
		},
		{
			name:       "full HD work area uses preferred size",
			workWidth:  1920,
			workHeight: 1040,
			want:       mainWindowSize{Width: 1200, Height: 850, MinWidth: 640, MinHeight: 480},
		},
		{
			name:       "laptop work area reduces height",
			workWidth:  1366,
			workHeight: 728,
			want:       mainWindowSize{Width: 1200, Height: 632, MinWidth: 640, MinHeight: 480},
		},
		{
			name:       "compact work area lowers initial and minimum dimensions",
			workWidth:  600,
			workHeight: 450,
			want:       mainWindowSize{Width: 504, Height: 354, MinWidth: 504, MinHeight: 354},
		},
		{
			name:       "work area smaller than margins still stays visible",
			workWidth:  80,
			workHeight: 70,
			want:       mainWindowSize{Width: 80, Height: 70, MinWidth: 80, MinHeight: 70},
		},
		{
			name:       "invalid work area uses fallback",
			workWidth:  0,
			workHeight: 1080,
			want:       mainWindowSize{Width: 1080, Height: 720, MinWidth: 640, MinHeight: 480},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := calculateMainWindowSize(test.workWidth, test.workHeight)
			if got != test.want {
				t.Fatalf("calculateMainWindowSize(%d, %d) = %+v, want %+v",
					test.workWidth, test.workHeight, got, test.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run the test and verify RED**

Run:

```powershell
go test . -run TestCalculateMainWindowSize -count=1
```

Expected: compilation fails because `mainWindowSize` and `calculateMainWindowSize` do not exist.

- [ ] **Step 3: Implement the minimal pure sizing policy**

Create `window_size.go`:

```go
package main

const (
	preferredWindowWidth  = 1200
	preferredWindowHeight = 850
	fallbackWindowWidth   = 1080
	fallbackWindowHeight  = 720
	normalMinWindowWidth  = 640
	normalMinWindowHeight = 480
	windowSafeMargin      = 48
)

type mainWindowSize struct {
	Width     int
	Height    int
	MinWidth  int
	MinHeight int
}

func calculateMainWindowSize(workWidth, workHeight int) mainWindowSize {
	if workWidth <= 0 || workHeight <= 0 {
		return mainWindowSize{
			Width:     fallbackWindowWidth,
			Height:    fallbackWindowHeight,
			MinWidth:  normalMinWindowWidth,
			MinHeight: normalMinWindowHeight,
		}
	}

	width := availableWindowDimension(workWidth, preferredWindowWidth)
	height := availableWindowDimension(workHeight, preferredWindowHeight)
	return mainWindowSize{
		Width:     width,
		Height:    height,
		MinWidth:  min(normalMinWindowWidth, width),
		MinHeight: min(normalMinWindowHeight, height),
	}
}

func availableWindowDimension(workDimension, preferred int) int {
	available := workDimension - 2*windowSafeMargin
	if available <= 0 {
		available = workDimension
	}
	return min(preferred, available)
}
```

- [ ] **Step 4: Run the targeted test and verify GREEN**

Run:

```powershell
go test . -run TestCalculateMainWindowSize -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit the sizing policy**

```powershell
git add window_size.go window_size_test.go
git commit -m "feat: calculate adaptive window size" -m "Keep the preferred dashboard size on large displays and clamp initial and minimum dimensions to small display work areas." -m "Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
```

### Task 2: Apply adaptive sizing to the Wails main window

**Files:**
- Modify: `window_size.go`
- Modify: `window_size_test.go`
- Modify: `app.go:103-112`

- [ ] **Step 1: Write failing tests for Wails option mapping**

Append to `window_size_test.go`:

```go
func TestMainWindowOptionsTargetsAndCentersPrimaryScreen(t *testing.T) {
	screen := &application.Screen{
		WorkArea: application.Rect{Width: 1366, Height: 728},
	}

	got := mainWindowOptions(true, screen)

	if got.Width != 1200 || got.Height != 632 {
		t.Fatalf("window size = %dx%d, want 1200x632", got.Width, got.Height)
	}
	if got.MinWidth != 640 || got.MinHeight != 480 {
		t.Fatalf("minimum size = %dx%d, want 640x480", got.MinWidth, got.MinHeight)
	}
	if got.Screen != screen {
		t.Fatal("window did not retain the selected primary screen")
	}
	if got.InitialPosition != application.WindowCentered {
		t.Fatalf("initial position = %v, want WindowCentered", got.InitialPosition)
	}
	if !got.Hidden {
		t.Fatal("hidden startup flag was not preserved")
	}
}

func TestMainWindowOptionsFallsBackWithoutAValidScreen(t *testing.T) {
	got := mainWindowOptions(false, nil)

	if got.Width != 1080 || got.Height != 720 {
		t.Fatalf("fallback window size = %dx%d, want 1080x720", got.Width, got.Height)
	}
	if got.MinWidth != 640 || got.MinHeight != 480 {
		t.Fatalf("fallback minimum = %dx%d, want 640x480", got.MinWidth, got.MinHeight)
	}
	if got.Screen != nil {
		t.Fatal("fallback options unexpectedly target a screen")
	}
	if got.Hidden {
		t.Fatal("visible startup unexpectedly became hidden")
	}
}
```

Add this import:

```go
import (
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"
)
```

- [ ] **Step 2: Run the option tests and verify RED**

Run:

```powershell
go test . -run 'TestMainWindowOptions' -count=1
```

Expected: compilation fails because `mainWindowOptions` does not exist.

- [ ] **Step 3: Implement the window-options factory**

Add to `window_size.go`:

```go
import "github.com/wailsapp/wails/v3/pkg/application"

func mainWindowOptions(hidden bool, screen *application.Screen) application.WebviewWindowOptions {
	workWidth, workHeight := 0, 0
	if screen != nil {
		workWidth = screen.WorkArea.Width
		workHeight = screen.WorkArea.Height
	}
	size := calculateMainWindowSize(workWidth, workHeight)

	return application.WebviewWindowOptions{
		Name:            "main",
		Title:           "AgentConfigSync",
		URL:             "/",
		Width:           size.Width,
		Height:          size.Height,
		MinWidth:        size.MinWidth,
		MinHeight:       size.MinHeight,
		InitialPosition: application.WindowCentered,
		Screen:          screen,
		Hidden:          hidden,
	}
}
```

- [ ] **Step 4: Run the option tests and verify GREEN**

Run:

```powershell
go test . -run 'TestMainWindowOptions' -count=1
```

Expected: PASS.

- [ ] **Step 5: Wire the factory into `guiApplication.configure`**

Replace the inline `WebviewWindowOptions` block in `app.go` with:

```go
	screen := gui.app.Screen.GetPrimary()
	if screen == nil || screen.WorkArea.Width <= 0 || screen.WorkArea.Height <= 0 {
		log.Printf("primary screen work area unavailable; using default window size")
	}
	gui.window = gui.app.Window.NewWithOptions(mainWindowOptions(hidden, screen))
```

Do not change activation, close-to-tray, or single-instance hooks.

- [ ] **Step 6: Run focused desktop tests**

Run:

```powershell
go test . -run 'TestCalculateMainWindowSize|TestMainWindowOptions|TestActivationQueue' -count=1
```

Expected: PASS.

- [ ] **Step 7: Commit Wails integration**

```powershell
git add app.go window_size.go window_size_test.go
git commit -m "feat: adapt main window to display" -m "Center the window on the primary work area and use safe fallback dimensions when display data is unavailable." -m "Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
```

### Task 3: Make short-window content explicitly scrollable

**Files:**
- Modify: `frontend/src/style.css:13-20`

- [ ] **Step 1: Update root overflow behavior**

Replace the current body rule with:

```css
html, body, #root { min-height: 100%; }
body { margin: 0; min-width: 320px; min-height: 100vh; overflow-y: auto; }
```

Keep the existing 800-pixel responsive breakpoint unchanged. Do not scale text or controls and do not add horizontal page scrolling.

- [ ] **Step 2: Build the frontend**

Run:

```powershell
npm --prefix frontend run build
```

Expected: TypeScript and Vite production build succeeds.

- [ ] **Step 3: Run the focused Go tests after the embedded frontend changes**

Run:

```powershell
go test . -run 'TestCalculateMainWindowSize|TestMainWindowOptions|TestActivationQueue' -count=1
```

Expected: PASS.

- [ ] **Step 4: Commit responsive overflow behavior**

```powershell
git add frontend/src/style.css
git commit -m "fix: keep compact windows scrollable" -m "Allow the dashboard document to scroll vertically when adaptive sizing selects a short window." -m "Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
```

`frontend/dist` is ignored and is rebuilt during validation and packaging, so do not force-add it.

### Task 4: Run full validation and package Windows x64

**Files:**
- Verify: `bin/AgentConfigSync-amd64-installer.exe`
- Verify: `scripts/smoke/windows.ps1`
- Verify: `C:\Users\qinqiangxu\.acsync\config.yaml`

- [ ] **Step 1: Run the complete Go test suite**

Run:

```powershell
go test ./... -count=1
```

Expected: all packages PASS.

- [ ] **Step 2: Run Go static analysis**

Run:

```powershell
go vet ./...
```

Expected: exit code 0 with no diagnostics.

- [ ] **Step 3: Rebuild production frontend assets**

Run:

```powershell
npm --prefix frontend run build
```

Expected: Vite production build succeeds.

- [ ] **Step 4: Build the Windows x64 per-user installer**

Run:

```powershell
$env:PATH = 'C:\Users\qinqiangxu\.copilot\session-state\be3cd41a-d061-4f74-9a7c-9024015f2234\files\nsis-3.12\nsis-3.12\Bin;' + $env:PATH
wails3 package GOOS=windows ARCH=amd64 INSTALL_SCOPE=user
```

Expected: NSIS writes `bin\AgentConfigSync-amd64-installer.exe`.

- [ ] **Step 5: Preserve the real profile and stop only the installed app**

Run:

```powershell
$installedExe = 'C:\Users\qinqiangxu\AppData\Local\Programs\AgentConfigSync\AgentConfigSync.exe'
$config = 'C:\Users\qinqiangxu\.acsync\config.yaml'
$configHash = (Get-FileHash -Algorithm SHA256 $config).Hash
$running = @(Get-CimInstance Win32_Process | Where-Object { $_.ExecutablePath -eq $installedExe })
foreach ($process in $running) {
    Stop-Process -Id $process.ProcessId
    Wait-Process -Id $process.ProcessId -ErrorAction SilentlyContinue
}
```

Expected: only processes whose executable path exactly matches the installed AgentConfigSync binary are stopped.

- [ ] **Step 6: Run the existing clean installer smoke test**

Run:

```powershell
powershell -NoProfile -ExecutionPolicy Bypass -File scripts\smoke\windows.ps1 -Installer bin\AgentConfigSync-amd64-installer.exe
```

Expected: `Windows installer smoke test passed`.

- [ ] **Step 7: Restore the installed application and verify profile preservation**

Run:

```powershell
$install = Start-Process -FilePath (Resolve-Path 'bin\AgentConfigSync-amd64-installer.exe') -ArgumentList '/S' -Wait -PassThru
if ($install.ExitCode -ne 0) { throw "Installer exited with code $($install.ExitCode)" }
$started = Start-Process -FilePath $installedExe -ArgumentList '--hidden' -PassThru
Start-Sleep -Seconds 5
if ((Get-FileHash -Algorithm SHA256 $config).Hash -ne $configHash) {
    throw 'AgentConfigSync profile changed during packaging verification'
}
Get-Process -Id $started.Id -ErrorAction Stop
```

Expected: the installed process remains running and the configuration hash is unchanged.

- [ ] **Step 8: Verify actual adaptive window behavior**

Open AgentConfigSync from its tray icon and verify:

- On the current large display, the window is centered, not maximized, and approximately 1200 by 850 logical pixels.
- The dashboard's main content is visible without native-window clipping.
- Reducing the window height keeps lower content reachable through vertical scrolling.
- A second application launch still hands off to the same process.

- [ ] **Step 9: Record the installer hash and final source state**

Run:

```powershell
Get-Item bin\AgentConfigSync-amd64-installer.exe | Select-Object FullName,Length,LastWriteTime
Get-FileHash -Algorithm SHA256 bin\AgentConfigSync-amd64-installer.exe
git status --short --branch
```

Expected: an installer path, size, SHA-256 hash, and a clean source worktree. The generated ignored installer does not require a source commit.
