# AgentConfigSync Packaging and Release Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Ship AgentConfigSync as a normal signed desktop application: Windows NSIS installer, macOS universal DMG, and Linux AppImage plus `.deb`, with native login startup, old-autostart migration, CI artifacts, and release smoke tests.

**Architecture:** Use Wails v3.0.0-beta.8 generated build assets and native OS runners. The desktop application owns Wails' native autostart manager and migrates legacy `.cmd`/LaunchAgent/systemd entries idempotently. Separate GitHub Actions jobs build and test each native package; signing and release upload occur only in protected release environments.

**Tech Stack:** Wails v3.0.0-beta.8, NSIS, Apple codesign/notarytool/DMG, Wails AppImage packaging, nfpm-backed deb packaging, GitHub Actions.

---

## File Structure

```text
internal/startup/manager.go              # app-facing startup interface
internal/startup/migration.go            # remove legacy autostart entries
internal/startup/migration_test.go
internal/desktop/wails.go                # Wails autostart bindings
frontend/src/components/StartupSetting.tsx
build/config.yml
build/windows/nsis/project.nsi           # finish-page launch + metadata
build/windows/Taskfile.yml               # corrected installer signing path
build/darwin/Info.plist
build/darwin/entitlements.plist
build/linux/nfpm/nfpm.yaml
build/linux/appimage/**
.github/workflows/ci.yml
.github/workflows/release.yml
scripts/smoke/windows.ps1
scripts/smoke/macos.sh
scripts/smoke/linux.sh
docs/install.md
```

---

### Task 1: Migrate legacy startup entries safely

**Files:**
- Create: `internal/startup/migration.go`
- Create: `internal/startup/migration_test.go`

- [ ] **Step 1: Write migration tests**

Test each OS with a temporary home:

```go
func TestRemoveLegacyWindowsStartup(t *testing.T) {
	home := t.TempDir()
	appData := filepath.Join(home, "AppData", "Roaming")
	t.Setenv("APPDATA", appData)
	path := filepath.Join(appData, "Microsoft", "Windows", "Start Menu", "Programs", "Startup", "acsync.cmd")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil { t.Fatal(err) }
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil { t.Fatal(err) }
	removed, err := RemoveLegacy("windows", home)
	if err != nil || len(removed) != 1 { t.Fatalf("removed=%v err=%v", removed, err) }
	if _, err := os.Stat(path); !os.IsNotExist(err) { t.Fatalf("legacy file still exists: %v", err) }
	if _, err := RemoveLegacy("windows", home); err != nil { t.Fatal(err) }
}
```

Also cover:

```text
darwin: ~/Library/LaunchAgents/com.acsync.agent.plist
linux:  ~/.config/systemd/user/acsync.service
```

Linux/macOS tests inject an unregister runner and assert it is called before
deletion.

- [ ] **Step 2: Implement migration**

```go
type Runner func(name string, args ...string) error

func RemoveLegacy(goos, home string, run Runner) ([]string, error)
```

Rules:

- remove only the exact known legacy path;
- unregister launchd/systemd before removal;
- missing entries are success;
- return removed paths for logging;
- no wildcard or recursive deletion.

- [ ] **Step 3: Verify**

```powershell
go test ./internal/startup -run TestRemoveLegacy -race -v
git add internal/startup
git commit -m "feat: migrate legacy login startup entries"
```

---

### Task 2: Wrap Wails native autostart

**Files:**
- Create: `internal/startup/manager.go`
- Create: `internal/startup/manager_test.go`
- Modify: `internal/desktop/wails.go`

- [ ] **Step 1: Define testable interface**

```go
type Backend interface {
	EnableWithOptions(application.AutostartOptions) error
	Disable() error
	IsEnabled() bool
}

type Manager struct {
	Backend Backend
	Identifier string
	Arguments []string
}
```

Test `Enable`, `Disable`, and `IsEnabled` with a fake backend.

- [ ] **Step 2: Implement Wails adapter**

Use:

```go
app.Autostart.EnableWithOptions(application.AutostartOptions{
	Identifier: "io.github.qinqingxu.agentconfigsync",
	Arguments:  []string{"--hidden"},
})
```

For AppImage, use the stable `APPIMAGE` path. If it is absent, return an
actionable error rather than writing an entry pointing into the temporary mount.

- [ ] **Step 3: Add Wails bindings**

Expose:

```go
StartAtLogin() bool
SetStartAtLogin(enabled bool) error
```

On first desktop startup:

1. call `RemoveLegacy`;
2. preserve the old enabled preference if a legacy entry existed;
3. enable Wails autostart only after migration succeeds;
4. log migrated paths.

- [ ] **Step 4: Add settings toggle**

Create a tested `StartupSetting.tsx` checkbox. Saving calls
`SetStartAtLogin`. Display an explicit message:

```text
"Starts AgentConfigSync the next time you sign in. It does not restart the app now."
```

- [ ] **Step 5: Verify and commit**

```powershell
go test ./internal/startup ./internal/desktop -race
npm --prefix frontend test -- --run
wails3 build
git add internal/startup internal/desktop frontend
git commit -m "feat: add native login startup controls"
```

---

### Task 3: Configure Windows NSIS installer

**Files:**
- Modify: `build/config.yml`
- Modify: `build/windows/nsis/project.nsi`
- Modify: `build/windows/Taskfile.yml`
- Create: `scripts/smoke/windows.ps1`

- [ ] **Step 1: Install and verify NSIS**

```powershell
winget install NSIS.NSIS
makensis /VERSION
```

Expected: NSIS version prints.

- [ ] **Step 2: Configure per-user installer**

Set installer metadata and use:

```text
INSTALL_SCOPE=user
```

The NSIS script must:

- install `AgentConfigSync.exe`;
- create Start menu shortcut;
- create uninstaller registration;
- offer `MUI_FINISHPAGE_RUN` to launch after install;
- never create `acsync.cmd`;
- never require admin for per-user installation.

Add:

```nsi
!define MUI_FINISHPAGE_RUN "$INSTDIR\AgentConfigSync.exe"
!define MUI_FINISHPAGE_RUN_TEXT "Launch AgentConfigSync"
!insertmacro MUI_PAGE_FINISH
```

- [ ] **Step 3: Correct beta.8 signing output path**

Ensure `windows:sign:installer` signs:

```text
bin/AgentConfigSync-amd64-installer.exe
```

not the stale `build/windows/nsis/...` documentation path.

- [ ] **Step 4: Build installer**

```powershell
wails3 package GOOS=windows GOARCH=amd64 INSTALL_SCOPE=user
```

Expected:

```text
bin/AgentConfigSync-amd64-installer.exe
```

- [ ] **Step 5: Add smoke script**

`scripts/smoke/windows.ps1` must:

1. install silently into a temporary test-user location;
2. verify Start menu shortcut and executable;
3. launch and verify a single process;
4. launch again and verify still one process;
5. close window and verify process remains;
6. uninstall;
7. verify files and autostart registration are removed.

Use process IDs captured from the installed executable; never kill by name.

- [ ] **Step 6: Commit**

```powershell
git add build scripts/smoke/windows.ps1
git commit -m "build: add Windows desktop installer"
```

---

### Task 4: Configure macOS universal app and DMG

**Files:**
- Modify: `build/darwin/Info.plist`
- Modify: `build/darwin/entitlements.plist`
- Create: `scripts/smoke/macos.sh`

- [ ] **Step 1: Set bundle identity and capabilities**

Use bundle identifier:

```text
io.github.qinqingxu.agentconfigsync
```

Set minimum supported macOS version consistent with Wails v3 and enable only
required entitlements. Do not add broad filesystem/network entitlements beyond
normal desktop access.

- [ ] **Step 2: Build universal app**

```bash
wails3 task darwin:package:universal
```

Expected:

```text
bin/AgentConfigSync.app
```

- [ ] **Step 3: Sign and notarize app**

```bash
xcrun notarytool store-credentials acsync-notary \
  --apple-id "$APPLE_ID" \
  --team-id "$APPLE_TEAM_ID" \
  --password "$APPLE_APP_PASSWORD"

wails3 tool sign \
  --input bin/AgentConfigSync.app \
  --identity "$SIGN_IDENTITY" \
  --entitlements build/darwin/entitlements.plist \
  --hardened-runtime \
  --notarize \
  --keychain-profile acsync-notary
```

- [ ] **Step 4: Create, sign, notarize, and staple DMG**

```bash
wails3 task darwin:create:dmg
codesign --force --timestamp --sign "$SIGN_IDENTITY" bin/AgentConfigSync.dmg
xcrun notarytool submit bin/AgentConfigSync.dmg --keychain-profile acsync-notary --wait
xcrun stapler staple bin/AgentConfigSync.dmg
xcrun stapler validate bin/AgentConfigSync.dmg
spctl --assess --verbose=2 bin/AgentConfigSync.app
```

- [ ] **Step 5: Add macOS smoke script**

Verify mount, copy to temporary Applications directory, first launch, close to
tray/menu bar, second-launch focus, login-item toggle, Quit, and clean removal.

- [ ] **Step 6: Commit**

```bash
git add build/darwin scripts/smoke/macos.sh
git commit -m "build: add signed macOS DMG packaging"
```

---

### Task 5: Configure Linux AppImage and deb

**Files:**
- Modify: `build/linux/nfpm/nfpm.yaml`
- Modify: `build/linux/Taskfile.yml`
- Create: `scripts/smoke/linux.sh`

- [ ] **Step 1: Install native build dependencies**

On Ubuntu 24.04:

```bash
sudo apt-get update
sudo apt-get install -y build-essential pkg-config libgtk-4-dev libwebkitgtk-6.0-dev
```

- [ ] **Step 2: Set package metadata and runtime dependencies**

The deb must declare:

```yaml
depends:
  - libgtk-4-1
  - libwebkitgtk-6.0-4
```

Install the executable and desktop entry at stable system paths. Advertise the
first `.deb` baseline as Ubuntu 24.04+/Debian 13+.

- [ ] **Step 3: Pin AppImage helper downloads**

Replace mutable `continuous` helper downloads with versioned URLs and committed
SHA-256 checks. Fail the build when a checksum differs.

- [ ] **Step 4: Build requested formats only**

```bash
wails3 task linux:create:appimage ARCH=amd64
wails3 task linux:create:deb ARCH=amd64
```

Expected:

```text
bin/AgentConfigSync-x86_64.AppImage
bin/AgentConfigSync.deb
```

- [ ] **Step 5: Add Linux smoke script**

Under Xvfb or a desktop runner:

- launch each package;
- verify single instance;
- verify tray when StatusNotifier is available;
- verify XDG autostart points to a stable path;
- verify AppImage uses `$APPIMAGE`, never the temporary mounted executable;
- install/remove deb and verify desktop entry cleanup.

- [ ] **Step 6: Commit**

```bash
git add build/linux scripts/smoke/linux.sh
git commit -m "build: add Linux AppImage and deb packages"
```

---

### Task 6: Add native CI validation

**Files:**
- Create: `.github/workflows/ci.yml`

- [ ] **Step 1: Define least-privilege workflow**

Use:

```yaml
name: CI
on:
  push:
    branches: [master]
  pull_request:
permissions:
  contents: read
```

Pin actions to full commit SHAs. Create:

- `core`: Windows, `go test ./... -race`, frontend test/build;
- `windows-package`: `windows-latest`, NSIS package + smoke;
- `macos-package`: `macos-latest`, unsigned app/DMG smoke for PRs;
- `linux-package`: `ubuntu-24.04`, AppImage/deb + smoke.

Install:

```text
github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.8
```

Upload artifacts with `actions/upload-artifact@v4` pinned by SHA.

- [ ] **Step 2: Validate workflow syntax**

Run the repository's existing workflow linter if present. Otherwise inspect
with:

```powershell
gh workflow view .github/workflows/ci.yml --yaml
```

and open a draft PR to execute all jobs.

- [ ] **Step 3: Commit**

```powershell
git add .github/workflows/ci.yml
git commit -m "ci: build desktop packages on native platforms"
```

---

### Task 7: Add protected signed release workflow

**Files:**
- Create: `.github/workflows/release.yml`

- [ ] **Step 1: Define tag-only build jobs**

Trigger:

```yaml
on:
  push:
    tags: ["v*"]
```

Set `contents: read` for builders. Each builder uses the same pinned source tag,
tests before packaging, signs with secrets from a protected `release`
environment, and uploads artifacts plus SHA-256 digests.

- [ ] **Step 2: Add final release job**

The final job:

- depends on all platform jobs;
- has `contents: write`;
- downloads all artifacts;
- verifies recorded digests;
- creates the GitHub release from the existing tag;
- uploads only signed/notarized deliverables;
- optionally emits build provenance attestations.

- [ ] **Step 3: Document required secrets**

Use:

```text
WINDOWS_CERTIFICATE_BASE64
WINDOWS_CERTIFICATE_PASSWORD
APPLE_CERTIFICATE_BASE64
APPLE_CERTIFICATE_PASSWORD
APPLE_ID
APPLE_TEAM_ID
APPLE_APP_PASSWORD
SIGN_IDENTITY
```

Never expose secrets to pull-request workflows.

- [ ] **Step 4: Commit**

```powershell
git add .github/workflows/release.yml
git commit -m "ci: add signed multi-platform desktop releases"
```

---

### Task 8: Write installation and migration documentation

**Files:**
- Create: `docs/install.md`
- Modify: `README.md` if present

- [ ] **Step 1: Document ordinary user flow**

Cover:

- Windows installer;
- drag macOS app from DMG;
- run AppImage or install deb;
- first-run OAuth/SSH wizard;
- login-start toggle;
- close-to-tray versus Quit;
- uninstall.

CLI instructions move to an “Advanced/headless use” section.

- [ ] **Step 2: Document migration**

Explain that the packaged app reuses:

```text
~/.acsync/config.yaml
~/.acsync/repo
~/.acsync/state.json
~/.acsync/providers
~/.acsync/logs
```

and automatically removes the legacy login entry. Existing users do not
re-clone or lose state.

- [ ] **Step 3: Commit**

```powershell
git add docs/install.md README.md
git commit -m "docs: add desktop installation and migration guide"
```

---

### Task 9: Final cross-platform release candidate verification

**Files:**
- Modify only files required by failures found during verification.

- [ ] **Step 1: Run core validation**

```powershell
go test ./... -race
go vet ./...
npm --prefix frontend test -- --run
npm --prefix frontend run build
```

- [ ] **Step 2: Run native package jobs**

Produce:

```text
AgentConfigSync-amd64-installer.exe
AgentConfigSync.dmg
AgentConfigSync-x86_64.AppImage
AgentConfigSync.deb
```

- [ ] **Step 3: Execute acceptance matrix**

On clean test users/machines:

1. install and open without terminal;
2. complete OAuth onboarding with empty private repo;
3. complete SSH onboarding;
4. restart and reuse existing config;
5. launch twice and confirm one process;
6. close to tray, reopen, pause/resume, Quit;
7. enable startup, log out/in, verify hidden launch;
8. uninstall and verify package files/startup registration removed;
9. verify `~/.acsync` user data is preserved unless user explicitly removes it;
10. verify headless `acsync daemon` still works.

- [ ] **Step 4: Verify signatures and checksums**

Windows:

```powershell
Get-AuthenticodeSignature .\bin\AgentConfigSync-amd64-installer.exe
```

macOS:

```bash
codesign --verify --deep --strict bin/AgentConfigSync.app
spctl --assess --verbose=2 bin/AgentConfigSync.app
xcrun stapler validate bin/AgentConfigSync.dmg
```

Linux:

```bash
sha256sum bin/AgentConfigSync-x86_64.AppImage bin/AgentConfigSync.deb
dpkg-deb --info bin/AgentConfigSync.deb
```

- [ ] **Step 5: Commit verification fixes**

```powershell
git add -A
git commit -m "fix: address desktop release verification findings"
```

Skip this commit when verification requires no code changes.

---

## Done Criteria

- Normal users install and launch without a terminal.
- Windows has a signed per-user NSIS installer and Start menu entry.
- macOS has a signed/notarized universal app and DMG.
- Linux has tested x86-64 AppImage and deb artifacts.
- Native login startup replaces legacy entries and never launches a console.
- CI builds/tests packages on their native OS.
- Release workflow protects signing secrets and publishes verified artifacts.
- Existing user data and headless CLI behavior remain compatible.
