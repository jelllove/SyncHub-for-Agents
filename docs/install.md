# Installing SyncHub

SyncHub is a tray application. It synchronizes supported AI-agent
configuration and session files through a private GitHub repository.

## Before installing

Create an empty **private** GitHub repository. Do not add a README or other
files; SyncHub can initialize the repository itself.

For SSH authentication, add an SSH key to GitHub and verify it:

```powershell
ssh -T git@github.com
```

GitHub should report that authentication succeeded.

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

## macOS

1. Download and open `SyncHub.dmg`.
2. Drag SyncHub to Applications.
3. Open it from Applications.

The standard `SyncHub.dmg` release path requires Developer ID signing and
notarization. To uninstall, quit the app from its
menu-bar icon and move it from Applications to Trash. Settings remain in
`~/.synchub`.

An explicitly named `SyncHub-macOS-universal-adhoc.dmg` is instead an **ad-hoc
signed test build, not Developer ID signed or notarized**. It includes Apple
Silicon and Intel binaries. Verify its download with `SHA256SUMS-macOS.txt`;
the separate Windows checksum manifest does not cover this asset. Gatekeeper
may block its first launch. Only approve it in **System Settings > Privacy &
Security** if you trust the source; do not disable system-wide security.
The minimum build target is macOS 12; automated native UI checks run on macOS 15.
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

Use **Reset and start over** at the bottom of **Sync settings**, or in the
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

Open the dashboard from the tray icon. Settings let you:

- enable or disable each detected agent;
- enable or disable individual resource categories;
- preview portable and excluded files;
- add validated custom resource directories;
- change the synchronization interval;
- enable or disable **Start at login**;
- trigger a synchronization immediately.

The installer migrates the legacy `synchub` startup entry when the desktop app
first launches. Existing configuration, repository checkout, and session data
in `~/.synchub` are reused.

## Automatic software updates

From v0.3.0 onward, release builds check the public
`jelllove/SyncHub-for-Agents` GitHub repository at startup and every six hours.
Only newer, stable releases are accepted: drafts, prereleases, equal versions,
and downgrades are not installed. Update checks do not use or send the
credentials for your private synchronization repository.

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
