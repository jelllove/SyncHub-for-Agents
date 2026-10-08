# SyncHub for Agents

![SyncHub for Agents hackathon poster](backup/hackathon-media-20260821/final/hackathon-poster.png)

SyncHub for Agents is a desktop tray app that keeps AI agent configuration and session data synchronized across multiple computers.

It uses a private Git repository you control as the synchronization bridge, with strict safety filters so credentials and machine-specific state stay local.

## Core capabilities

- Synchronize agent resources (sessions, config, instructions, skills, plugin declarations).
- Run as a desktop/tray app with clear status states: Ready, Updating, Sync complete, Paused, Needs attention.
- Sync periodically in the background and on-demand.
- Propagate deletions across machines with a configurable recovery window.
- Resolve conflicts with local/remote/merged choices and batch apply.
- Review and approve plugin/skill install operations before execution, with explicit retry for failed operations.
- Select which agents and resource categories are included, plus custom resource directories.
- Start automatically at login.
- Check GitHub Releases automatically and securely download Windows x64 updates
  for installation when you quit the app.

## What is synchronized

- Portable session files
- Safe projected configuration fields
- Instructions and skills source files
- Plugin manifests/declarations
- Shared common resources configured for supported agents

Built-in adapters cover Claude Code, GitHub Copilot CLI, Gemini CLI, Cursor,
VS Code/Copilot, and shared resources under the user-level `.agents` directory.
Categories vary by adapter; selecting an agent does not imply every category
is available. Instructions include supported command/prompt files.

Most built-in sources are **user-level directories**, not arbitrary project
directories. Project-local `CLAUDE.md`, `GEMINI.md`, `AGENTS.md`,
`.github/copilot-instructions.md`, and tool-specific project directories can
be configured as explicit custom resources with reviewed source/restore paths.
They are not automatically discovered across all projects. OpenCode, Z-Code,
MiniMax Code, and OpenAI Codex do not yet have built-in adapters.

## What is never synchronized

- Credentials, API keys, OAuth tokens, keyrings
- Machine IDs and machine-local config fields
- Caches, logs, temp files, test runtimes
- Generated dependencies (for example `node_modules`, virtual environments, `__pycache__`)
- Platform binaries and unsafe large files

See [docs/portable-resources.md](docs/portable-resources.md) for detailed rules.

## Downloads and platform support

Use the [official releases](https://github.com/jelllove/SyncHub-for-Agents/releases/latest)
or the [qinqingxu mirror](https://github.com/qinqingxu/SyncHub-for-Agents/releases/latest).
Check each release's asset list and notes; a packaging target does not mean an
installer was published or tested on every OS version.
Linux packaging produces DEB and RPM alongside AppImage. Older releases,
including the original v0.3.4 Linux asset set, may not include an RPM.

| Platform | Package | Notes |
| --- | --- | --- |
| Windows x64 | `SyncHub-for-Agents-Setup-x64.exe` | Per-user installer; may be unsigned. Check SmartScreen/signing notes. |
| macOS Apple Silicon / Intel | `SyncHub-macOS-universal-adhoc.dmg` | Universal ad-hoc build, **not Apple notarized**. Gatekeeper may block first launch. |
| Linux x64 | `SyncHub-x86_64.AppImage`, `SyncHub.deb`, `SyncHub.rpm` | AppImage, Debian/Ubuntu DEB, or Fedora-family RPM; check the release's actual asset list. |

Verify Windows downloads with `SHA256SUMS.txt`, the ad-hoc macOS DMG with
`SHA256SUMS-macOS.txt`, and separately added Linux packages with
`SHA256SUMS-Linux.txt`. Do not assume an older checksum manifest covers assets
added later. A standard Developer ID/notarized macOS build, if published, uses
the separate `SyncHub.dmg` name.

The v0.3.4 ad-hoc DMG passed installed-app native XCTest onboarding and
invalid-repository tests on macOS 15.7.9, Apple Silicon and Intel.
[The validation run](https://github.com/jelllove/SyncHub-for-Agents/actions/runs/37208710481)
retains screenshots and logs as `macos-native-arm64` / `macos-native-x86_64`
artifacts for 30 days. This does not establish Gatekeeper acceptance, live sync,
keychain/login-item behavior, or runtime support on macOS 12.

The v0.3.4 Linux packages passed version/architecture checks and isolated
extracted-package startup on Ubuntu 24.04 x64 under Xvfb. The stricter native
window check uses a private D-Bus session and a test-app-only AppArmor user
namespace allowance for WebKit's sandbox; it does not establish first launch
under every distribution's default security policy.
[The Linux validation run](https://github.com/jelllove/SyncHub-for-Agents/actions/runs/37210714225)
retains packages and startup logs. This is not a full UI test, system-wide
Debian installation test, or a compatibility guarantee for every distribution.

## Quick start

1. Create an empty private GitHub repository.
2. Download and verify the appropriate package from the release page.
3. Install and launch **SyncHub for Agents**.
4. Complete onboarding:
   - connect your private repository,
   - authenticate with SSH or GitHub Device Flow,
   - choose agents to synchronize.
5. Keep the app running in the tray on each computer you want to sync.

Detailed install and onboarding guide: [docs/install.md](docs/install.md).
On macOS, open the DMG and drag the app to Applications. On Linux, install the
DEB/RPM package with the distribution's package manager, or make the AppImage
executable and keep it at a permanent path.
Approve an unnotarized macOS app only if you trust its source; do not disable
system-wide security.

## Software updates

Starting with v0.3.0, release builds check
[GitHub Releases](https://github.com/jelllove/SyncHub-for-Agents/releases/latest)
at startup and every six hours. Windows x64 automatically downloads the latest
stable installer and verifies its SHA-256 checksum before staging it.
The installer runs after you select **Quit** from the tray, without restarting
the app; alternatively, use **Restart to update** in settings.

Settings also provide **Check for updates** and an automatic-update switch.
Closing the window only hides it in the tray and does not install an update.
macOS, Linux, and Windows ARM64 show a link for manual installation instead.
Development builds do not update themselves. Users on v0.2.3 and earlier must
install v0.3.0 or later manually once to enable this feature.

## Build from source

### Prerequisites

- Go version declared in [go.mod](go.mod)
- Node.js version pinned in [.node-version](.node-version), with npm
- Wails v3 CLI (`wails3`)
- NSIS (`makensis`) for Windows installer packaging

### Useful commands

```powershell
node scripts/dev.mjs setup
node scripts/dev.mjs verify
go test ./...
npm --prefix frontend test
wails3 dev
wails3 package GOOS=windows ARCH=amd64 INSTALL_SCOPE=user
```

See the [development guide](docs/development.md) for reproducible setup, hooks,
maintenance, and documentation checks, and [CONTRIBUTING.md](CONTRIBUTING.md)
before submitting a change.

## Project notes

- Product brand: **SyncHub for Agents**
- Some internal identifiers and file names still use `SyncHub`/`synchub` for compatibility with existing installs and startup registrations.

## Code signing policy

SyncHub for Agents intends to use SignPath Foundation for open-source Windows
code signing. See [docs/code-signing-policy.md](docs/code-signing-policy.md) for
release signing scope, team roles, approval requirements, and privacy behavior.

## License

SyncHub for Agents is released under the [MIT License](LICENSE).
