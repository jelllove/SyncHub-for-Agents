# SyncHub Full Rename Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rename active product/runtime/build surfaces from AgentConfigSync/acsync to SyncHub naming, while automatically migrating existing `~/.acsync` users to new paths safely.

**Architecture:** Apply rename in layers: (1) module/import + command/runtime identity, (2) migration-safe data and startup transition, (3) packaging/workflow/documentation convergence. Keep legacy strings only in explicit migration recognition paths. Validate with targeted unit tests first, then full build/package smoke checks.

**Tech Stack:** Go 1.26, Cobra CLI, Wails v3, React/TypeScript (Vite), NSIS, GitHub Actions, PowerShell/bash smoke scripts.

---

## File structure and responsibilities

- `go.mod` — canonical Go module identity.
- `cmd/synchub/main.go` (new) — renamed CLI root command and user-facing CLI text.
- `internal/cli/runtime.go` + `internal/cli/home_migration.go` (new) — runtime home path and legacy home migration.
- `internal/startup/{manager,migration}.go` + `internal/autostart/autostart.go` — startup identifiers and legacy startup entry cleanup.
- `internal/auth/keyring.go` — OAuth keyring service namespace and token migration.
- `app.go`, `main.go` — desktop app identifiers, single-instance key, OAuth env key name.
- `build/*`, `Taskfile.yml`, `.github/workflows/*`, `scripts/smoke/*` — build outputs, installer naming, release paths.
- `README.md`, `docs/install.md` — user-facing download/install naming.
- `assets/icons/` + `frontend/scripts/generate-icons.mjs` — renamed icon source reference.

### Naming targets

- Executable: `SyncHub.exe`
- Installer: `SyncHub-for-Agents-Setup-x64.exe`
- CLI command: `synchub`
- Module path: `github.com/qinqingxu/synchub-for-agents`
- Runtime home: `~/.synchub`
- Startup identifier: `io.github.qinqingxu.synchub`

Only migration-recognition code may reference legacy strings (`.acsync`, `acsync.cmd`, `com.acsync.agent`, etc.).

---

### Task 1: Rename Go module and import root

**Files:**
- Modify: `go.mod`
- Modify: `main.go`
- Modify: `app.go`
- Modify: `internal/**/*.go` (all active files importing old module path)
- Modify: `*_test.go` files with old import path

- [ ] **Step 1: Write the failing change by updating module path first**

```go
module github.com/qinqingxu/synchub-for-agents
```

- [ ] **Step 2: Run targeted compile test to verify import breakage appears**

Run:
```powershell
go test ./internal/cli -run TestBuildResourceSpecsSkipsDisabledCategoriesAndNormalizesV1
```
Expected: FAIL with unresolved imports from `github.com/qinqingxu/acsync/...`.

- [ ] **Step 3: Replace import root everywhere in active Go code**

```go
// before
import "github.com/qinqingxu/acsync/internal/cli"

// after
import "github.com/qinqingxu/synchub-for-agents/internal/cli"
```

Use one repository-wide replacement for active code (exclude `docs/superpowers/**` historical files).

- [ ] **Step 4: Run targeted tests after import migration**

Run:
```powershell
go test ./internal/cli ./internal/startup ./internal/auth
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add go.mod main.go app.go internal cmd
git commit -m "refactor: rename go module path to synchub-for-agents"
```

---

### Task 2: Rename CLI command to `synchub` and update desktop OAuth env key

**Files:**
- Create: `cmd/synchub/main.go`
- Delete: `cmd/acsync/main.go`
- Modify: `main.go`
- Test: `cmd/synchub/main_test.go` (new)

- [ ] **Step 1: Write failing tests for command naming**

```go
func TestRootCommandUsesSynchub(t *testing.T) {
	root := newRootCommand()
	if root.Use != "synchub" {
		t.Fatalf("Use = %q, want synchub", root.Use)
	}
}
```

- [ ] **Step 2: Run test to verify it fails before implementation**

Run:
```powershell
go test ./cmd/synchub -run TestRootCommandUsesSynchub
```
Expected: FAIL because `cmd/synchub` and `newRootCommand` do not exist yet.

- [ ] **Step 3: Implement renamed command package and testable constructor**

```go
func newRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:   "synchub",
		Short: "Sync AI agent config and session files across machines via a private GitHub repo",
	}
	root.AddCommand(initCmd(), syncCmd(), statusCmd(), daemonCmd(), trayCmd(), installCmd(), uninstallCmd(), credentialCmd())
	return root
}
```

Also in desktop `main.go`:

```go
if clientID == "" {
	clientID = os.Getenv("SYNCHUB_GITHUB_CLIENT_ID")
}
```

and keep a migration fallback:

```go
if clientID == "" {
	clientID = os.Getenv("ACSYNC_GITHUB_CLIENT_ID") // legacy fallback only
}
```

- [ ] **Step 4: Run command tests**

Run:
```powershell
go test ./cmd/synchub
```
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add cmd/synchub main.go
git rm cmd/acsync/main.go
git commit -m "refactor: rename CLI command to synchub"
```

---

### Task 3: Migrate runtime home from `~/.acsync` to `~/.synchub` with rollback safety

**Files:**
- Modify: `internal/cli/runtime.go`
- Create: `internal/cli/home_migration.go`
- Create: `internal/cli/home_migration_test.go`
- Modify: `internal/cli/init_test.go`
- Modify: `internal/cli/sync_test.go`
- Modify: `internal/cli/status_test.go`
- Modify: `internal/desktop/service_test.go` (path fixtures)

- [ ] **Step 1: Write failing migration tests**

```go
func TestEnsureHomeMigratesLegacyDirectory(t *testing.T) {
	user := t.TempDir()
	legacy := filepath.Join(user, ".acsync")
	current := filepath.Join(user, ".synchub")
	if err := os.MkdirAll(legacy, 0o755); err != nil { t.Fatal(err) }
	if err := os.WriteFile(filepath.Join(legacy, "config.yaml"), []byte("version: 3\n"), 0o644); err != nil { t.Fatal(err) }

	got, err := ensureHome(user)
	if err != nil { t.Fatal(err) }
	if got != current { t.Fatalf("home = %q, want %q", got, current) }
	if _, err := os.Stat(filepath.Join(current, "config.yaml")); err != nil { t.Fatal(err) }
}
```

- [ ] **Step 2: Run tests to verify failure**

Run:
```powershell
go test ./internal/cli -run "TestEnsureHomeMigratesLegacyDirectory|TestEnsureHomeKeepsLegacyOnMigrationFailure"
```
Expected: FAIL because `ensureHome` is not implemented.

- [ ] **Step 3: Implement migration helper and wire `Home()`**

```go
func Home() (string, error) {
	userHome, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return ensureHome(userHome)
}
```

```go
func ensureHome(userHome string) (string, error) {
	current := filepath.Join(userHome, ".synchub")
	legacy := filepath.Join(userHome, ".acsync")
	if _, err := os.Stat(current); err == nil {
		return current, nil
	}
	if _, err := os.Stat(legacy); os.IsNotExist(err) {
		return current, nil
	}
	// copy-verify-switch with checkpoint; if any step fails, keep legacy intact
	return migrateLegacyHome(legacy, current)
}
```

- [ ] **Step 4: Update path-dependent tests from `.acsync` to `.synchub`**

```go
home := filepath.Join(t.TempDir(), ".synchub")
```

- [ ] **Step 5: Run targeted tests**

Run:
```powershell
go test ./internal/cli ./internal/desktop -run "TestEnsureHome|TestRunInitScaffolds|TestRunStatus|TestSnapshot"
```
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/cli internal/desktop
git commit -m "feat: migrate runtime home from .acsync to .synchub"
```

---

### Task 4: Rename startup/autostart identities and keep legacy startup cleanup

**Files:**
- Modify: `app.go`
- Modify: `internal/autostart/autostart.go`
- Modify: `internal/autostart/autostart_test.go`
- Modify: `internal/startup/migration.go`
- Modify: `internal/startup/migration_test.go`
- Modify: `internal/startup/manager_test.go`

- [ ] **Step 1: Write failing tests for new startup entry names**

```go
func TestNewWindowsManagerUsesSynchubCmd(t *testing.T) {
	m, _ := New("windows", `C:\Users\alice`)
	if m.File != "synchub.cmd" {
		t.Fatalf("file = %q, want synchub.cmd", m.File)
	}
}
```

- [ ] **Step 2: Run autostart tests and confirm failures**

Run:
```powershell
go test ./internal/autostart -run "TestNewWindowsManagerUsesSynchubCmd|TestContentGenerators"
```
Expected: FAIL because code still emits `acsync.*`.

- [ ] **Step 3: Implement new startup names**

```go
const label = "com.synchub.agent"
```

```go
// windows
m.File = "synchub.cmd"
// linux
m.File = "synchub.service"
```

```go
// app.go
ProgramName: "synchub",
UniqueID: "com.qinqingxu.synchub",
Identifier: "io.github.qinqingxu.synchub",
```

- [ ] **Step 4: Keep legacy startup remover for old entry names**

```go
// migration.go remains responsible for removing:
// acsync.cmd, com.acsync.agent.plist, acsync.service
```

Do not rename legacy matcher values; they are migration-only constants.

- [ ] **Step 5: Run targeted tests**

Run:
```powershell
go test ./internal/autostart ./internal/startup
```
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add app.go internal/autostart internal/startup
git commit -m "refactor: rename startup identifiers to synchub"
```

---

### Task 5: Rename keyring namespace and internal sentinel markers

**Files:**
- Modify: `internal/auth/keyring.go`
- Modify: `internal/auth/keyring_test.go`
- Modify: `internal/resource/filter.go`
- Modify: `internal/resource/filter_test.go`
- Modify: `internal/resourcecollect/collector.go`
- Modify: `internal/resourcecollect/collector_test.go`

- [ ] **Step 1: Write failing test for keyring migration fallback**

```go
func TestTokenFallsBackToLegacyService(t *testing.T) {
	backend := memoryBackend{}
	_ = backend.Set("io.github.qinqingxu.acsync/github-oauth", "42", "legacy-token")
	store := NewStore(backend)
	token, err := store.Token(42)
	if err != nil { t.Fatal(err) }
	if token != "legacy-token" { t.Fatalf("token = %q", token) }
}
```

- [ ] **Step 2: Run auth test to verify failure**

Run:
```powershell
go test ./internal/auth -run TestTokenFallsBackToLegacyService
```
Expected: FAIL before fallback exists.

- [ ] **Step 3: Implement new primary service + legacy read fallback**

```go
const (
	keyringService       = "io.github.qinqingxu.synchub/github-oauth"
	legacyKeyringService = "io.github.qinqingxu.acsync/github-oauth"
)
```

```go
func (s *Store) Token(id int64) (string, error) {
	key := accountKey(id)
	value, err := s.backend.Get(keyringService, key)
	if err == nil { return value, nil }
	if !errors.Is(err, ErrNotFound) { return "", err }
	return s.backend.Get(legacyKeyringService, key)
}
```

- [ ] **Step 4: Rename synthetic marker from `.acsync-entry` to `.synchub-entry`**

```go
probe := strings.TrimSuffix(rel, "/") + "/.synchub-entry"
```

and in collector:

```go
scanner.IsExcluded(path.Join(logicalPath, ".synchub-entry"))
```

- [ ] **Step 5: Run targeted tests**

Run:
```powershell
go test ./internal/auth ./internal/resource ./internal/resourcecollect
```
Expected: PASS.

- [ ] **Step 6: Commit**

```bash
git add internal/auth internal/resource internal/resourcecollect
git commit -m "refactor: rename keyring and sentinel namespaces to synchub"
```

---

### Task 6: Rename packaging outputs and installer artifact names

**Files:**
- Modify: `Taskfile.yml`
- Modify: `build/config.yml`
- Modify: `build/windows/wails.exe.manifest`
- Modify: `build/windows/info.json`
- Modify: `build/windows/nsis/project.nsi`
- Modify: `build/windows/msix/template.xml`
- Modify: `build/windows/msix/app_manifest.xml`
- Modify: `build/darwin/Info.plist`
- Modify: `build/darwin/Info.dev.plist`
- Modify: `build/ios/Info.plist`
- Modify: `build/ios/Info.dev.plist`
- Modify: `build/linux/desktop`
- Modify: `build/linux/nfpm/nfpm.yaml`
- Modify: `.gitignore`
- Rename: `assets/icons/agentconfigsync.svg` -> `assets/icons/synchub.svg`
- Modify: `frontend/scripts/generate-icons.mjs`
- Modify: `frontend/package.json`
- Modify: `frontend/package-lock.json`

- [ ] **Step 1: Write failing packaging expectation check**

Add/update assertion in Windows smoke script test target to expect new installer name:

```powershell
if (!(Test-Path ".\bin\SyncHub-for-Agents-Setup-x64.exe")) {
  throw "renamed installer missing"
}
```

- [ ] **Step 2: Run packaging command to verify it currently fails expectation**

Run:
```powershell
wails3 package GOOS=windows ARCH=amd64 INSTALL_SCOPE=user
Test-Path .\bin\SyncHub-for-Agents-Setup-x64.exe
```
Expected: `False` (old name still produced).

- [ ] **Step 3: Implement build identity rename**

Key edits:

```yaml
# Taskfile.yml
APP_NAME: "SyncHub"
```

```yaml
# build/config.yml
productIdentifier: "io.github.qinqingxu.synchub"
```

```nsi
; build/windows/nsis/project.nsi
OutFile "..\..\..\bin\SyncHub-for-Agents-Setup-x64.exe"
DeleteRegValue HKCU "Software\Microsoft\Windows\CurrentVersion\Run" "io.github.qinqingxu.synchub"
```

```js
// frontend/scripts/generate-icons.mjs
await sharp(path.join(sources, 'synchub.svg'), { density: 384 })
```

- [ ] **Step 4: Update Linux/macOS/iOS packaging descriptors**

Examples:

```yaml
# build/linux/nfpm/nfpm.yaml
name: "synchub"
vendor: "SyncHub for Agents"
homepage: "https://github.com/qinqingxu/SyncHub-for-Agents"
```

```desktop
Name=SyncHub for Agents
Exec=/usr/local/bin/SyncHub %u
Icon=SyncHub
```

- [ ] **Step 5: Regenerate icons and package assets**

Run:
```powershell
npm --prefix frontend run generate:icons
wails3 task common:generate:icons
```
Expected: PASS and updated app/tray icon outputs.

- [ ] **Step 6: Commit**

```bash
git add Taskfile.yml build assets/icons frontend/scripts frontend/package*.json .gitignore
git commit -m "build: rename packaging outputs to SyncHub artifacts"
```

---

### Task 7: Update workflows, smoke scripts, and user docs for new names

**Files:**
- Modify: `.github/workflows/ci.yml`
- Modify: `.github/workflows/release.yml`
- Modify: `scripts/smoke/windows.ps1`
- Modify: `scripts/smoke/macos.sh`
- Modify: `scripts/smoke/linux.sh`
- Modify: `README.md`
- Modify: `docs/install.md`

- [ ] **Step 1: Write failing check for stale release paths**

Run:
```powershell
rg -n "AgentConfigSync-amd64-installer\\.exe|AgentConfigSync\\.dmg|bin/AgentConfigSync" .github\workflows scripts\smoke README.md docs\install.md
```
Expected: Matches found (pre-change).

- [ ] **Step 2: Implement workflow and smoke-script path updates**

Examples:

```yaml
# .github/workflows/ci.yml
run: ./scripts/smoke/windows.ps1 -Installer ./bin/SyncHub-for-Agents-Setup-x64.exe
```

```pwsh
# scripts/smoke/windows.ps1
$installDir = Join-Path $env:TEMP "SyncHub-Smoke-$PID"
$exe = Join-Path $installDir "SyncHub.exe"
$shortcut = Join-Path $env:APPDATA "Microsoft\Windows\Start Menu\Programs\SyncHub for Agents.lnk"
$runName = "io.github.qinqingxu.synchub"
```

- [ ] **Step 3: Update README/install docs asset names**

```md
- `SyncHub-for-Agents-Setup-x64.exe`
```

- [ ] **Step 4: Re-run stale-name grep**

Run:
```powershell
rg -n "AgentConfigSync|agentconfigsync|acsync" README.md docs\install.md .github\workflows scripts\smoke
```
Expected: no hits, except intentionally documented migration wording if present.

- [ ] **Step 5: Commit**

```bash
git add .github/workflows scripts/smoke README.md docs/install.md
git commit -m "docs: align workflows and install docs with SyncHub naming"
```

---

### Task 8: Full validation, legacy-name audit, package, and release

**Files:**
- Verify: whole repository active surfaces
- Produce: `bin/SyncHub-for-Agents-Setup-x64.exe`
- Publish: GitHub release asset + checksum

- [ ] **Step 1: Run full test/build suite**

Run:
```powershell
npm --prefix frontend run build
go test ./...
go vet ./...
```
Expected: PASS.

- [ ] **Step 2: Build Windows installer with renamed artifact**

Run:
```powershell
$env:Path = "C:\Users\qinqiangxu\.copilot\session-state\be3cd41a-d061-4f74-9a7c-9024015f2234\files\nsis-3.12\nsis-3.12\Bin;$env:Path"
wails3 package GOOS=windows ARCH=amd64 INSTALL_SCOPE=user
```
Expected: `bin\SyncHub-for-Agents-Setup-x64.exe` exists.

- [ ] **Step 3: Smoke test renamed installer**

Run:
```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\smoke\windows.ps1 -Installer .\bin\SyncHub-for-Agents-Setup-x64.exe
```
Expected: `Windows installer smoke test passed`.

- [ ] **Step 4: Audit active code for legacy names**

Run:
```powershell
rg -n "AgentConfigSync|agentconfigsync|acsync" . --glob "!docs/superpowers/**"
```
Expected: hits only in explicit migration-recognition code/tests (for example `internal/cli/home_migration*`, `internal/startup/migration*`, `internal/auth/keyring.go` legacy fallback, `main.go` legacy env fallback), and nowhere else.

- [ ] **Step 5: Publish release with renamed installer**

Run:
```powershell
$hash = (Get-FileHash -Algorithm SHA256 .\bin\SyncHub-for-Agents-Setup-x64.exe).Hash
gh release create v0.2.0 .\bin\SyncHub-for-Agents-Setup-x64.exe --repo qinqingxu/SyncHub-for-Agents --title "SyncHub for Agents v0.2.0" --notes "SHA256: $hash"
```
Expected: release created with renamed asset.

- [ ] **Step 6: Commit final rename adjustments**

```bash
git add .
git commit -m "chore: complete SyncHub full rename and migration rollout"
```
