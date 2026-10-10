# SyncHub for Agents NPM installer

A lightweight installer and launcher for the prebuilt SyncHub desktop app.
Wrapper 0.1.0 installs desktop **v0.3.5**, not uncommitted development changes.
Public registry availability must be confirmed after publishing.

## Install and run

After publication:

```text
npm install -g synchub-for-agents --registry=https://registry.npmjs.org/
synchub-for-agents --version
synchub-for-agents
```

Installing downloads and verifies the desktop archive but does not open the app.
The launcher runs in the foreground; closing the desktop window keeps its tray
process alive, so the command returns when you quit the app from the tray.
No Start menu or Applications shortcut is created.

If NPM lifecycle scripts are disabled:

```text
npm install -g synchub-for-agents --ignore-scripts --registry=https://registry.npmjs.org/
synchub-for-agents --install
```

## Platforms and prerequisites

Windows supports x64 and needs Microsoft Edge WebView2 Runtime. macOS supports
Intel x64 and Apple Silicon using a Universal app, minimum build target macOS 12.
The v0.3.5 macOS binary is ad-hoc signed, not Developer ID signed or notarized;
do not disable Gatekeeper or other system security to run an untrusted download.
Linux supports x64 and needs Git, GTK4 and WebKitGTK 6.0. Ubuntu 24.04 x64 is the
verified startup target; other distributions are not guaranteed.
Git must be available separately on every platform. Node must be 24.13 or later.

Archives are pinned by size and SHA-256 and downloaded only from the official
`jelllove/SyncHub-for-Agents` GitHub repository. Network/proxy failures are errors,
not successful installations. Node's supported proxy environment mode can be
enabled with `NODE_USE_ENV_PROXY=1` when a proxy is required.

## Updates and uninstall

To remain NPM-managed, disable **Automatic updates** in the desktop settings:
the v0.3.5 Windows desktop updater uses an NSIS installer, not NPM.
Update through NPM after a newer wrapper/native release has been published.
Quit the app and disable its start-at-login setting before uninstalling:

```text
npm uninstall -g synchub-for-agents
```

NPM removes the wrapper and its native files. It does not delete user settings,
agent resources or synchronization data. Moved or modified installed executables
fail integrity checks rather than being silently launched.

## Source verification and publishing

From this package directory:

```text
npm ci --ignore-scripts
npm test
npm pack
npm publish --registry=https://registry.npmjs.org/ --access public
```

Publishing requires the user's official NPM login and any required 2FA approval.
Do not use the internal corporate registry or place tokens in this directory.

See the [specification](docs/spec.md), [architecture](docs/arch.md), and
[project guidance](AGENTS.md) for scope, constraints and acceptance criteria.
