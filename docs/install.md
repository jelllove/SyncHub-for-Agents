# Installing SyncHub

SyncHub is a tray application. It synchronizes supported AI-agent
configuration and session files through a private GitHub repository.

## Before installing

### Headless CLI option

Use the separate [CLI guide](cli.md) for command-line installation, JSON output
and headless daemon operation. Starting with v0.3.6, CLI archives are published
separately from Desktop packages and do not require GUI runtimes. They share the
Desktop profile by default; quit Desktop from its tray before CLI operations on
that profile. Earlier releases do not include the new headless executable.

### NPM installation option

The [NPM installer package](../packages/npm-installer/README.md) provides a
download-verifying launcher rather than compiling the desktop app through Node.
Wrapper 0.1.1 pins desktop v0.3.7 and supports Windows x64, macOS
Universal x64/ARM64, and Linux x64. It downloads only official release archives,
validates size/SHA-256 and rejects unsafe extraction. Installing does not open
the app or modify startup settings. NPM publication requires an authenticated
official-registry account; a local tarball is not evidence of publication.

After publication, install with:

```text
npm install -g synchub-for-agents --registry=https://registry.npmjs.org/
synchub-for-agents --version
synchub-for-agents
```

For a verified local tarball:

```text
npm install -g ./synchub-for-agents-0.1.1.tgz --registry=https://registry.npmjs.org/
```

The existing platform prerequisites below still apply. NPM does not supply
WebView2 or Linux GTK4/WebKitGTK, notarize macOS binaries, or create OS shortcuts.
The launcher stays in the foreground until you quit the desktop app. Disable
desktop automatic updates if you want to remain NPM-managed, because current
Windows updates use NSIS instead of NPM. Quit and disable start-at-login before
uninstalling the NPM package; user data is never deleted by this wrapper.

Create an empty **private** GitHub repository. Do not add a README or other
files; SyncHub can initialize the repository itself.

For SSH authentication, add an SSH key to GitHub and verify it:

```powershell
ssh -T git@github.com
```

GitHub should report that authentication succeeded.

## Package matrix and validation scope

Download packages and `SHA256SUMS.txt` from the
[official release page](https://github.com/jelllove/SyncHub-for-Agents/releases/latest).
The workflows produce this matrix; an older release may not contain every format.
macOS and Linux publishing remain opt-in, so only assets actually attached to a
release are available for download.

| Platform | CPU | Primary package | Alternative packages |
| --- | --- | --- | --- |
| Windows | x64 (amd64) | `SyncHub-for-Agents-Setup-x64.exe` | `SyncHub-for-Agents-Windows-x64.zip` |
| macOS | ARM64 + Intel x64 | Universal `.dmg` | Universal `.zip`; ad-hoc assets carry `-adhoc` in their names |
| Linux / Ubuntu | x64 (amd64) | `SyncHub.deb` | `SyncHub-x86_64.AppImage`, `SyncHub-linux-x64.tar.gz` |
| Linux / RPM distributions | x64 (amd64) | `SyncHub.rpm`, optional alternative | Fedora 43 container installation check; not Fedora desktop verification |

**Linux desktop validation target: Ubuntu 24.04 x64 only.** The
[CI packaging job](../.github/workflows/ci.yml) checks the DEB structure, extracts
and starts the AppImage in an isolated home under Xvfb, and checks the tar archive
contents and executable version. It does not install the DEB into a clean OS or
prove interactive desktop behavior. A distribution is verified for a particular
revision only when that native job passes; inspect the corresponding CI run.
For `v0.3.5`, [Ubuntu/Fedora package validation](https://github.com/jelllove/SyncHub-for-Agents/actions/runs/37797776658)
passed for release commit `f84e964`. Ubuntu checks verified visible native startup
of the extracted AppImage, DEB and RPM, plus tar.gz content equality with the DEB.
Fedora 43 checks verified RPM dependencies, installation and executable version in
a container, not a graphical Fedora desktop.
[macOS native validation](https://github.com/jelllove/SyncHub-for-Agents/actions/runs/37796990437)
passed on macOS 15 ARM64 and Intel, including DMG installation, native onboarding
UI tests, and complete ZIP app-content equality. These results do not extend to
other distributions, Gatekeeper acceptance or live synchronization.

Debian 13 has the expected GTK4/WebKitGTK 6.0 package family but is not a verified
target. Ubuntu 22.04, other Debian/Ubuntu versions, Fedora desktop, RHEL, AlmaLinux and
Rocky Linux are not claimed as verified. In particular, RPM generation on Ubuntu
does not establish compatibility with any RPM distribution. AppImage and tar.gz
do not guarantee compatibility with older glibc or missing system libraries.
Windows ARM64 and Linux ARM64 packages are outside this release matrix.

Git must be installed separately and available on `PATH` on every platform.
Compare each downloaded asset with its entry in `SHA256SUMS.txt`; checksums detect
corruption, not publisher identity.

## Windows

1. Download `SyncHub-for-Agents-Setup-x64.exe` from the latest release.
2. Run the installer. It installs for the current user and does not require
   administrator access.
3. Start **SyncHub** from the Start menu.

Download from the
[official release page](https://github.com/jelllove/SyncHub-for-Agents/releases/latest).
Releases include `SHA256SUMS.txt` for download verification. Windows builds may
be unsigned when no code-signing certificate is configured; check the release
notes before installing. Windows SmartScreen may warn about an unsigned build.

Uninstall it from **Settings > Apps > Installed apps**. Your synchronized data
and settings in `%USERPROFILE%\.synchub` are retained so an uninstall cannot
delete your sessions accidentally.

### Portable ZIP

Extract `SyncHub-for-Agents-Windows-x64.zip` to a permanent directory and run
`SyncHub.exe`. The ZIP includes the executable and license, not an installer.
Microsoft Edge WebView2 Runtime must already be installed; unlike the installer,
the ZIP does not bootstrap it.

"Portable" means no installation is required, not isolated data storage:
settings and synchronization data still use `%USERPROFILE%\.synchub`.
Keep the extracted directory stable before enabling **Start at login**.
Automatic Windows updates still use the NSIS installer; disable automatic updates
in settings and replace the ZIP manually if you want to remain installation-free.
Do not run an installed and extracted copy simultaneously against the same data.

## macOS

1. Download and open `SyncHub.dmg`.
2. Drag SyncHub to Applications.
3. Open it from Applications.

The standard `SyncHub.dmg` release path requires Developer ID signing and
notarization. To uninstall, quit the app from its
menu-bar icon and move it from Applications to Trash. Settings remain in
`~/.synchub`.

Release DMG and ZIP packages contain a Universal app for both Apple Silicon ARM64
and Intel x64. The deployment target is macOS 12 or later; CI packaging/smoke tests
are configured on macOS 15 ARM64 and Intel hosts, not on every supported OS version.

For the ZIP alternative, extract `SyncHub-macos-universal.zip` and move
`SyncHub.app` to Applications before launching. The standard signed packages require the
configured Developer ID signing and Apple notarization credentials. The workflow
notarizes and staples the app before creating either final archive. Local/PR
packages are only ad-hoc signed, not notarized release builds; do not bypass
Gatekeeper for an untrusted download.

An explicitly named `SyncHub-macOS-universal-adhoc.dmg` or
`SyncHub-macOS-universal-adhoc.zip` is instead an **ad-hoc
signed test build, not Developer ID signed or notarized**. It includes Apple
Silicon and Intel binaries. Verify its download with `SHA256SUMS-macOS.txt`;
older Windows-only checksum manifests do not cover this asset. The `v0.3.5`
combined `SHA256SUMS.txt` covers every package, alongside the platform manifests. Gatekeeper
may block its first launch. Only approve it in **System Settings > Privacy &
Security** if you trust the source; do not disable system-wide security.
The minimum build target is macOS 12; automated native UI checks run on macOS 15.
For the ad-hoc ZIP, extract and move the app to Applications. Native validation
checks that its complete contents match the app installed from the tested DMG.
See the [macOS installer pipeline](development.md#macos-installed-app-validation)
for screenshots, tested behavior, and limitations.

## Linux

Linux x64 packaging supports `SyncHub-x86_64.AppImage`, `SyncHub.deb`, and
`SyncHub.rpm`. Download only assets actually listed in the chosen release;
older releases may not include RPM.
Separately added Linux assets have their own `SHA256SUMS-Linux.txt`; verify those
downloads against that manifest rather than an older Windows-only checksum file.
The [Linux package workflow](development.md#linux-release-package-validation)
checks extracted-package startup on Ubuntu 24.04 under Xvfb, not every Linux
distribution or desktop environment.

### Debian or Ubuntu

```bash
sudo apt install ./SyncHub.deb
```

Start SyncHub from the application menu.

Ubuntu 24.04 x64 is the configured validation target. DEB dependencies include
`libgtk-4-1` and `libwebkitgtk-6.0-4`. Install Git separately:

```bash
sudo apt install git
```


### Fedora and compatible RPM distributions

```bash
sudo dnf install ./SyncHub.rpm
```

The RPM declares `gtk4` and `webkitgtk6.0` dependencies. Use a distribution
that provides GTK4 and WebKitGTK 6.0; do not assume older RHEL/CentOS releases
can install it. The validation workflow includes a Fedora 43 container
installation/dependency check, plus extracted-RPM startup on Ubuntu.
Check the chosen release's evidence before treating those checks as passed
for that release. Fedora desktop UI, login items, and live sync are not covered
by the container's package-installation check.

Start SyncHub from the application menu. Uninstall a DEB with
`sudo apt remove synchub` or an RPM with `sudo dnf remove synchub`;
user data in `~/.synchub` is retained.

### AppImage

```bash
chmod +x SyncHub-x86_64.AppImage
./SyncHub-x86_64.AppImage
```

Keep the AppImage at a permanent path before enabling **Start at login**.
SyncHub records that stable path, not the temporary AppImage mount.

On Ubuntu 24.04, install `libfuse2t64` if FUSE 2 is missing. AppImage extraction
can avoid the FUSE mount requirement, but does not remove other runtime-library
requirements.

### tar.gz

The archive contains `SyncHub/SyncHub`, a desktop entry, icon, license and
installation guide. It is not a self-contained AppImage; install the native
runtime dependencies first. On Ubuntu 24.04:

```bash
sudo apt install git libgtk-4-1 libwebkitgtk-6.0-4
mkdir -p "$HOME/.local/opt"
tar -xzf SyncHub-linux-x64.tar.gz -C "$HOME/.local/opt"
"$HOME/.local/opt/SyncHub/SyncHub"
```

Keep that directory stable for **Start at login**. The included desktop entry
uses `Exec=SyncHub`; copying it to an application-menu directory also requires
putting the executable on `PATH` or updating `Exec` and `Icon` to absolute paths.
Extracting the archive alone does not register a menu item.

### Optional RPM

RPM is an alternative package format; see
[native packaging and release controls](development.md#native-desktop-packages).
RPM metadata requires `gtk4` and `webkitgtk6.0`, but the binary is built on Ubuntu
24.04. The Fedora 43 container checks installation and dependencies, not a
Fedora graphical desktop. RHEL-family installation and launch are not verified.
Use it only after validating those dependencies and binary compatibility on the
specific distribution; report the distribution/version and architecture with
the validation result.

## First launch

1. Enter the private repository URL:
   - SSH: `git@github.com:your-name/agent-sync.git`
   - SSH alias: `git@github-work:your-name/agent-sync.git`
   - HTTPS: `https://github.com/your-name/agent-sync.git`
   SSH aliases may select a different GitHub key through `~/.ssh/config`. The
   alias must resolve to `HostName github.com`; SyncHub verifies the effective
   host before contacting the repository.
2. Authenticate:
   - SSH verifies your local key, `ssh-agent`, GitHub host key, and access to
     the selected repository.
   - HTTPS opens GitHub Device Flow. The OAuth token is stored only in the
     operating-system keyring.
3. Choose agents to synchronize. Detected agents are enabled by default.
4. Select **Start synchronizing**.
   If a first-sync choice is required, a **Choose first sync strategy** dialog
   opens automatically. Choose **Use cloud**, **Merge cloud + local**, or
   **Use local**, then confirm with **Start first sync**. No strategy is
   preselected; local/cloud preferences can replace or remove differing files
   on the other side. **Choose later** or Escape closes the dialog without
   starting a sync; **Sync now** on the dashboard or in settings opens it again.
   Confirmation also resumes synchronization if it was paused. Save failures
   remain visible in the dialog so you can retry.
5. Open **Sync settings** and review the resource categories, source-to-target
   mappings, excluded files, and restore totals.
6. If the repository contains Plugin declarations or Skill dependencies,
   review the exact executable and arguments in the installation plan before
   approving it.

### Reset and start over

Use **Reset and start over** under **Sync settings > Advanced**, or in the
onboarding steps if setup was not completed. Review the local clone path and
type `RESET`, then choose **Delete local setup**.

This removes SyncHub configuration, synchronization history/merge bases,
pending conflicts and installation approvals, its saved OAuth login, and the
selected local Git clone, including any unpushed changes. It returns to the
Welcome screen without requiring an app restart. The operation cannot be
undone.

The remote repository, agent source files, SSH configuration/keys, custom
provider definitions, logs, automatic update preferences, and start-at-login
registration remain unchanged. Reset does not uninstall plugins or integrations.

Reset is refused during a running synchronization or when the selected clone
has an unsafe path, links, or unexpected top-level files. Custom clones must
also have an origin exactly matching the current settings. The dedicated
default clone can be reset even if its origin differs, is missing, or setup
was not completed, so changing a repository URL or SSH alias does not prevent
starting over.
Move unrelated files out of that clone and retry; do not select your agent
directory or home directory as the local clone. Custom clones are staged in
their own parent directory, so resetting a clone on another volume does not
require copying it. Inside the SyncHub data directory, only its default `repo`
subdirectory is allowed as a reset clone.

If staging or credential removal fails, SyncHub attempts to restore the local
files and credentials. After file rollback succeeds, synchronization is
available again with its previous paused state and no automatic startup sync.
Recovery failures and any remaining staging paths are shown explicitly; do not
delete recovery data until the error has been resolved.

The app runs an initial synchronization and then checks every ten minutes.
Closing the window keeps it running in the system tray. Use **Quit** from the
tray menu to stop it completely.

On a new computer, safe configuration, instructions, Skills, and sessions
restore from the connected repository. Existing local credentials remain
local. See [Portable agent resources](portable-resources.md) for category
details, exclusions, conflict handling, and recovery behavior.

## Settings and startup

### Tray activity tips

While the desktop app is running, its native system-tray icon remains registered,
including when the main window is closed to the tray. Only **Quit** exits it.
Windows controls whether the icon is in the visible notification area or the
overflow menu; pin SyncHub through Windows taskbar settings if you want it always
visible. SyncHub does not override your taskbar preferences.

Windows displays a small, non-activating activity tip beside the tray during
synchronization. Its status icon and text change for **Pulling**, **Scanning**,
**Comparing**, **Applying** and **Pushing**. A push is only reported when the
engine actually reaches a Git push, not for an unchanged cycle.
The same tip is updated in place; per-file progress does not create extra tips.
It does not steal focus or add a taskbar button.

Successful completion stays visible for four seconds. Errors and items needing
attention stay visible for twelve seconds; the error/attention tray icon and
hover text remain after the tip disappears. Errors are also reported if a cycle
fails while future automatic syncing is paused. **Open SyncHub for Agents**
opens the main window for details. **Dismiss activity tip** hides tips for the
current run, but a terminal error/attention result is still shown.

Tips use generic operation text, not repository URLs, file paths or raw command
output. They are application-owned tips, not Windows Notification Center history.
macOS/Linux receive the same status-specific tray icon and hover text but do not
automatically show this Windows popup. The legacy `synchub tray` CLI is unchanged.

### Settings categories and login startup

Open the dashboard from the tray icon, then **Sync settings**. Settings are
organized into keyboard-accessible tabs:

| Tab | Controls |
| --- | --- |
| Repository | Private repository URL, local clone directory, and clone/move policy. |
| Sync | Synchronization frequency, archive retention and **Run sync now**. |
| Resources | Agents, resource categories, preview/restore totals and custom resources. |
| Desktop | **Start with Windows** / **Start at login**, automatic updates and update checks. |
| Advanced | Review and confirm **Reset and start over**. |

Configured installations open on **Sync**; an unconfigured settings panel opens
on **Repository**. Arrow keys, Home and End move between tabs. Unsaved values
survive tab changes, and **Save settings** saves all categories together.
If a required field in another tab is invalid, saving reveals and focuses that
field. Automatic-update preferences are still saved immediately, independently
of the synchronization form. Reset still requires an explicit review and `RESET`.

On Windows, the desktop app registers the current executable with `--hidden`
under `HKCU\Software\Microsoft\Windows\CurrentVersion\Run`, using the existing
`io.github.qinqingxu.synchub` identifier. Startup is enabled once on the first
release-build launch with no recorded startup preference, including older Windows installations
that have not recorded a choice. It starts in the tray at the next Windows login,
not immediately. Turn it off under **Desktop > Start with Windows**, then choose
**Save settings**.

The choice is kept locally in `~/.synchub/startup-settings.json`. Later launches
do not override a saved opt-out or recreate an entry removed outside SyncHub.
Resetting synchronization setup preserves this file and the registration.
Developer builds do not automatically register temporary executables; an explicit
startup choice is still available. macOS/Linux startup defaults are unchanged. Registry/preference failures remain
visible in the Desktop tab with **Retry startup status**; they do not prevent
opening the application or get reported as a successful registration.

The installer migrates the legacy `synchub` startup entry when the desktop app
first launches. Existing configuration, repository checkout, and session data
in `~/.synchub` are reused.

## Automatic software updates

From v0.3.0 onward, release builds check the public
`jelllove/SyncHub-for-Agents` GitHub repository at startup and every six hours.
Only newer, stable releases are accepted: drafts, prereleases, equal versions,
and downgrades are not installed. Update checks do not use or send the
credentials for your private synchronization repository.

Transient request timeouts (including TLS handshake timeouts), connection EOFs
and HTTP 500/502/503/504 responses get at most three attempts with bounded,
cancellable delays. Release metadata has a 90-second total budget; individual
TLS handshakes have a 20-second limit and response headers a 30-second limit.
The normal system certificate trust and HTTP(S) proxy environment settings are
preserved. Certificate errors and permanent HTTP errors are not retried or
bypassed; installer size, source and SHA-256 checks remain mandatory.

If requests still fail, **Desktop > Check now** shows the error and guidance to
check the connection or proxy. Retry after restoring connectivity, or open the
official release page for manual installation. Retries cannot make a blocked
GitHub endpoint reachable.

On Windows x64, a matching `SyncHub-for-Agents-Setup-x64.exe` and
`SHA256SUMS.txt` must both be present on the release. The download is size-limited
and its SHA-256 is verified before it can be installed. If the release only
contains source code, a download fails, or a checksum is missing or incorrect,
settings show an error and the running application is not replaced.
HTTPS and the official GitHub repository are the download trust boundary;
checksums detect corruption but do not replace publisher code signing.

The update is staged while the app keeps running. Choose **Quit** from the
tray to install it after the process exits, or **Restart to update** in settings
to install and reopen the app. Restart is refused while a synchronization is
active. Closing the main window only hides it and does not trigger installation.
Settings and synchronized files are retained. A custom installation directory
is reused; a directory requiring administrator access must be updated manually.
Installation errors are recorded locally and displayed on the next launch.

Use **Check for updates** to check immediately. Turning off automatic updates
stops periodic checks and installation on Quit; a manual check can still download
an update, and **Restart to update** explicitly installs it.
The preference is local to this machine, in `~/.synchub/update-settings.json`.
Development builds (`dev`) never auto-update.

Automatic installation currently supports Windows x64 only. macOS, Linux, and
Windows ARM64 provide a release-page link for manual installation. Users on
v0.2.3 or earlier need to install a newer release manually once: those versions
cannot discover or install the updater themselves.

## Advanced: headless CLI

The `synchub` command remains available for servers and scripted environments.
Desktop users normally do not need it.

```powershell
synchub init --repo git@github.com:your-name/agent-sync.git
synchub sync
synchub status
```

Use the same private repository on each computer. Do not run the desktop app
and the headless daemon simultaneously for the same user account.
