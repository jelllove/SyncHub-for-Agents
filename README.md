# SyncHub for Agents

SyncHub for Agents is a desktop tray app that keeps AI agent configuration and session data synchronized across multiple computers.

It uses a private Git repository you control as the synchronization bridge, with strict safety filters so credentials and machine-specific state stay local.

## Core capabilities

- Synchronize agent resources (sessions, config, instructions, skills, plugin declarations).
- Run in the Windows system tray with clear status states: Ready, Updating, Sync complete, Paused, Needs attention.
- Sync periodically in the background and on-demand.
- Propagate deletions across machines with a configurable recovery window.
- Resolve conflicts with local/remote/merged choices and batch apply.
- Review and approve plugin/skill install operations before execution, with explicit retry for failed operations.
- Select which agents and resource categories are included, plus custom resource directories.
- Start automatically at login.

## What is synchronized

- Portable session files
- Safe projected configuration fields
- Instructions and skills source files
- Plugin manifests/declarations
- Shared common resources configured for supported agents

## What is never synchronized

- Credentials, API keys, OAuth tokens, keyrings
- Machine IDs and machine-local config fields
- Caches, logs, temp files, test runtimes
- Generated dependencies (for example `node_modules`, virtual environments, `__pycache__`)
- Platform binaries and unsafe large files

See [docs/portable-resources.md](docs/portable-resources.md) for detailed rules.

## Quick start (Windows)

1. Create an empty private GitHub repository.
2. Download the installer from the latest release:
   - `SyncHub-for-Agents-Setup-x64.exe`
3. Install and launch **SyncHub for Agents**.
4. Complete onboarding:
   - connect your private repository,
   - authenticate with SSH or GitHub Device Flow,
   - choose agents to synchronize.
5. Keep the app running in the tray on each computer you want to sync.

Detailed install and onboarding guide: [docs/install.md](docs/install.md).

## Build from source

### Prerequisites

- Go 1.26+
- Node.js + npm
- Wails v3 CLI (`wails3`)
- NSIS (`makensis`) for Windows installer packaging

### Useful commands

```powershell
npm --prefix frontend install
npm --prefix frontend run build
go test ./...
wails3 dev
wails3 package GOOS=windows ARCH=amd64 INSTALL_SCOPE=user
```

## Project notes

- Product brand: **SyncHub for Agents**
- Some internal identifiers and file names still use `SyncHub`/`synchub` for compatibility with existing installs and startup registrations.
