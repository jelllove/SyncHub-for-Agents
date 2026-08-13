# AgentConfigSync Cross-Platform Desktop App Design

- **Date:** 2026-08-13
- **Status:** Approved design, pending implementation plan
- **Platforms:** Windows, macOS, Linux

## 1. Problem

AgentConfigSync currently exposes a capable sync engine through `acsync` CLI
commands and a tray process. This works for development and headless machines,
but it does not behave like a normal desktop application:

- users must install and invoke a command manually;
- `acsync install` only registers login startup and does not explain that it
  will not launch the application immediately;
- there is no installer, Start menu or Applications entry, main window, first
  run flow, or integrated authentication experience;
- repository and credential errors are shown as terminal output rather than
  actionable application UI;
- an empty GitHub repository fails on the first pull because it has no default
  branch.

The product should install, launch, configure, and update like a conventional
desktop application while preserving the existing cross-platform Go sync core.

## 2. Product Goals

1. A non-technical user can install and launch AgentConfigSync without a
   terminal.
2. The first-run wizard connects a private GitHub repository using browser
   login or an existing SSH key and verifies unattended access.
3. A dashboard shows sync health, schedule, repository, and enabled agents.
4. Closing the main window keeps synchronization running in the system tray.
   The process exits only through the tray's Quit action.
5. Windows, macOS, and Linux share one product and UI, with native packaging
   and OS integration per platform.
6. Existing CLI commands remain available for headless hosts, automation, and
   diagnostics, but are not the normal desktop entry point.

## 3. Chosen Approach

Use **Wails v3** as the desktop shell:

- the existing Go packages remain the application backend;
- a shared web frontend renders the desktop UI in each platform's system
  WebView;
- the Wails application calls Go APIs directly rather than spawning `acsync`
  for ordinary operations;
- platform-specific packaging and system integration are thin adapters around
  the shared application.

Wails v3 is currently beta, but its desktop API is documented as stable and it
provides native system tray, window lifecycle, typed bindings, and packaging
support. The implementation plan must pin a tested release rather than tracking
`latest`. Upgrades occur only after the native smoke suite passes on all three
platforms.

Alternatives considered:

- **Fyne:** simpler all-Go distribution, but weaker visual fidelity and layout
  flexibility for a polished dashboard and onboarding wizard.
- **Tauri:** excellent packaging and UI performance, but introduces Rust and a
  second backend boundary around an already complete Go core.

Wails provides the best balance of reuse, maintainability, and desktop UX.

## 4. Architecture

```text
┌────────────────────────────────────────────────────────────┐
│ Wails Desktop Application                                  │
│  Dashboard │ First-run Wizard │ Settings │ Activity/Errors │
└──────────────────────────┬─────────────────────────────────┘
                           │ typed Wails bindings/events
┌──────────────────────────▼─────────────────────────────────┐
│ Desktop Application Service                               │
│  lifecycle │ single instance │ auth/repo setup │ settings │
└───────────┬──────────────────────┬─────────────────────────┘
            │                      │
┌───────────▼──────────┐  ┌────────▼────────────────────────┐
│ Existing Go Core     │  │ Platform Adapters              │
│ syncengine, config,  │  │ tray, startup, credential      │
│ daemon, scheduler,   │  │ discovery, packaging, open URL │
│ provider, gitclient  │  └─────────────────────────────────┘
└──────────────────────┘
```

### 4.1 Process model

- One application process owns the scheduler, tray, settings, and main window.
- A single-instance guard prevents a second scheduler from starting.
- Launching the app again activates the existing main window.
- Closing the main window hides it and leaves the scheduler running.
- Tray **Quit** cancels the scheduler, shuts down services, and exits.
- The existing `daemon` command remains a separate headless mode and must not
  run concurrently with the desktop application for the same acsync home.

### 4.2 Reuse and boundaries

- `syncengine`, `provider`, `secret`, `state`, and `config` stay UI-independent.
- `daemon`/`scheduler` expose observable status and live interval updates.
- GitHub and SSH onboarding live in a dedicated repository setup service.
- The Wails frontend never receives tokens, private key contents, or raw
  credential values.
- The existing local web settings server is retired from the normal desktop
  flow after feature parity is reached. It may remain temporarily for CLI or
  compatibility use during migration.

## 5. Desktop Experience

### 5.1 Installation and launch

The application installs under the product name **AgentConfigSync**, creates a
normal launcher, and can be opened by double-clicking:

- Windows: installer/MSIX, Start menu entry, uninstall entry;
- macOS: signed `.app` distributed in a DMG;
- Linux: AppImage and `.deb`, with a desktop entry.

The installer offers:

- launch AgentConfigSync after installation;
- start AgentConfigSync automatically at login.

Login startup launches the desktop application directly, not a visible console
or batch file. The first installation starts the app immediately when selected;
enabling startup alone is never presented as though it launched the app.

### 5.2 First-run wizard

The wizard contains these steps:

1. **Welcome** — explains what will and will not be synchronized.
2. **Repository** — enter or select a private GitHub repository.
3. **Authentication** — choose GitHub browser login or SSH.
4. **Connectivity check** — verify read/write access without prompting.
5. **Agent discovery** — show detected Claude, Copilot, Gemini, Cursor, and
   custom providers; all detected agents are enabled by default.
6. **Review and start** — save settings and run the initial sync.

#### GitHub browser login

- Use an AgentConfigSync GitHub OAuth App device authorization flow with the
  private-repository permission required by Git; normal users do not need to
  install GitHub CLI.
- Store the resulting credential in the OS credential store (Windows
  Credential Manager, macOS Keychain, or Linux Secret Service).
- Register an acsync Git credential helper for the local sync repository. The
  helper retrieves credentials from the OS store without exposing them to the
  frontend or writing them into Git URLs/config files.
- If GitHub CLI is already authenticated, the wizard may offer to reuse/import
  that account, but this is an optional shortcut rather than a dependency.
- Verify repository access with a non-destructive remote query followed by the
  initial repository operation.

#### SSH

- Detect existing SSH keys and GitHub host configuration.
- Verify GitHub authentication and repository access.
- If no usable key exists, guide the user through key generation and display
  only the public key for registration.
- Verify that subsequent Git operations can run unattended. A passphrase key
  requires a working OS SSH agent.

#### Empty repository behavior

If the remote repository has no branch:

- clone it successfully;
- create `.gitattributes`;
- create the initial `main` commit;
- push and set upstream;
- continue the first sync.

The UI reports this as normal repository initialization, not as an error.

### 5.3 Main dashboard

The approved layout is a status dashboard:

- prominent current state: idle, updating, error, or paused;
- last successful sync and next scheduled sync;
- pending action and blocked-file counts;
- repository name and connection status;
- enabled agent summary with quick toggles;
- primary actions: **Sync now**, **Pause/Resume**, **Open logs**, **Settings**;
- recent activity and an actionable error panel.

State colors remain consistent between window and tray:

- green: idle/healthy;
- blue: updating;
- red: error;
- gray: paused.

### 5.4 Settings

Settings include:

- repository and authentication status;
- sync interval;
- trash grace period;
- start at login;
- per-agent enable toggles;
- provider/exclusion details;
- logs and diagnostics.

Agent and repository changes apply on the next sync. Sync interval changes
reconfigure the running scheduler immediately without requiring restart.

### 5.5 Tray

The tray menu provides:

- Open AgentConfigSync;
- Sync now;
- Pause/Resume;
- status summary;
- Settings;
- Open logs;
- Quit.

Closing the main window hides it. Tray Quit is the explicit full-exit action.

## 6. Error Handling and Diagnostics

- Authentication, clone, pull, push, config, and filesystem failures surface
  in the dashboard with a concise explanation and a recommended action.
- Detailed errors are written to the existing log directory.
- No broad error swallowing or success-shaped fallback is allowed.
- The app distinguishes authentication failure, missing repository,
  insufficient write permission, offline state, Git conflict, and empty remote.
- Retryable network failures preserve the current local state and retry on the
  next schedule or explicit Sync now.
- The UI and logs redact tokens, private keys, embedded URL credentials, and
  other detected secrets.

## 7. Packaging and Platform Integration

### Windows

- Produce an installer/MSIX with Start menu and uninstall registration.
- Build as a GUI application so no console window appears.
- Use a native login-start mechanism suitable for the packaged application,
  rather than a visible Startup-folder batch file.

### macOS

- Produce a `.app` bundle and DMG.
- Use the platform login-item mechanism.
- Signing/notarization can be configured when certificates are available; local
  development builds remain possible without them.

### Linux

- Produce AppImage and `.deb` packages with desktop files and icons.
- Support common desktop autostart integration.
- Keep `acsync daemon` as the supported option for headless hosts.

GUI builds run natively on each target OS in CI. The project does not rely on
Windows cross-compilation for macOS/Linux desktop artifacts.

## 8. Testing

### Backend

- Preserve all existing Go tests.
- Add tests for empty-repository initialization and idempotent re-entry.
- Test GitHub CLI and SSH detection through injected command runners.
- Test live scheduler interval changes and single-instance coordination.
- Test desktop lifecycle state transitions independently of Wails.

### Frontend

- Component tests for dashboard states, onboarding steps, settings, and errors.
- Binding/service tests use deterministic backend fakes.
- Verify that no secret value is rendered.

### End to end

On each native OS:

- install and launch from the normal application launcher;
- complete onboarding with a private repository;
- initialize an empty repository;
- sync and restart;
- close to tray, reopen, pause/resume, and quit;
- enable/disable login startup;
- uninstall cleanly.

## 9. Migration

- Existing `~/.acsync/config.yaml`, repository clone, state, providers, and logs
  are reused in place.
- Existing users skip onboarding when configuration and repository access are
  valid.
- The app detects and offers to replace the old Windows `acsync.cmd` startup
  entry with the packaged login-start mechanism.
- CLI commands remain compatible throughout the migration.

## 10. Out of Scope for This Iteration

- automatic application self-update;
- a mobile application;
- non-GitHub cloud backends;
- editing arbitrary provider YAML in the GUI;
- resolving same-file semantic merge conflicts beyond the existing policy.

## 11. Acceptance Criteria

The feature is complete when:

1. A new user can install, open, authenticate, choose agents, and complete the
   first sync without a terminal.
2. An empty private GitHub repository is initialized automatically.
3. Reopening the app activates one existing process and never starts a second
   scheduler.
4. Closing hides to tray; Quit exits.
5. The dashboard and tray reflect scheduler state accurately.
6. Settings persist and interval changes apply live.
7. Native Windows, macOS, and Linux packages pass their platform smoke tests.
8. Existing CLI and synchronization tests remain green.
