# NPM desktop installer specification

## Goals

Provide `npm install -g synchub-for-agents` and a `synchub-for-agents` command
that installs and launches the official prebuilt SyncHub desktop application.
The user requested a usable NPM package and publication to the public registry.
The package must not require Go, Wails, a C compiler or frontend compilation.

## Scope

The initial wrapper version is 0.1.0 and selects the already published desktop
release v0.3.5. This is an implementation choice, not approval to create another
GitHub release. Unreleased worktree improvements are not part of those binaries.
Windows x64, macOS x64/ARM64 Universal, and Linux x64 are covered. Unsupported
platforms and architectures fail explicitly.

## Functional requirements

- Select an exact platform archive and checksum from the shipped release manifest.
- Download only from official GitHub HTTPS hosts with bounded size, redirects,
  retry count and time budgets. Never send repository credentials.
- Check both exact archive size and SHA-256 before extraction or launching.
- Reject traversal, absolute paths, links, special files and oversized extraction.
- Stage installation inside the NPM package, then publish a complete native
  directory and integrity receipt. Never launch from a partial installation.
- Install without opening the app, registering startup or editing user settings.
- Provide a foreground launcher, download-only `--install`, and native `--version`.
  Preserve argument boundaries and child exit status without using a command shell.
- Support installation with lifecycle scripts disabled: `--install` performs the
  explicit download later. Do not quietly substitute an unsupported binary.
- Keep publishing public, with a narrow files whitelist and no credentials,
  user sync data, logs, development dependencies or downloaded binaries.

## Interactions

NPM postinstall downloads the native package but does not run it. The command
launches the verified executable and waits for its exit. `--install` verifies or
downloads without launching. `--version` executes only the native version query.
The normal desktop settings, sync repository, tray and authentication behavior
belong to the desktop binary, not the NPM wrapper.

## Edge cases

Network, checksum, extraction, filesystem and process failures exit nonzero and
identify the failed operation. Temporary installation files are removed. A
corrupted or partially missing installed executable is not launched.
Concurrent installations cannot replace an in-use package-owned native tree.
Missing WebView2 or Linux graphical libraries remain explicit OS prerequisites.
macOS ad-hoc signing does not become Developer ID signing or notarization.

## Non-goals

No new GitHub release, automatic application launch, OS installer invocation,
Start menu or Applications shortcut creation, privilege elevation, automatic
credential setup, live synchronization during tests, or security-policy bypass.
Do not mutate startup/update preferences or delete user data during uninstall.
The existing Windows updater remains installer-based; users who want only NPM
management must disable desktop automatic updates and update the NPM package.

## Acceptance criteria

- The package packs into an installable `.tgz` with only the documented whitelist.
- Archive download, verification, extraction, receipt and launcher tests pass.
- A real downloaded Windows archive installs in an isolated local NPM fixture
  and its executable reports desktop version 0.3.5 without opening the GUI.
- Traversal, links, wrong hashes, unsupported platforms, tampered installed
  binaries, network failures and child launch failures fail deterministically.
- `npm test` also checks complete spec/architecture/guidance documents and links,
  including isolated failure fixtures for missing sections and unfinished markers.
- Publication occurs only with authenticated official-registry authorization.
  A blocked authentication attempt is reported, never described as publication.
