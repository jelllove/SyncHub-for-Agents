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

macOS packages, when available, are signed and notarized. To uninstall, quit the app from its
menu-bar icon and move it from Applications to Trash. Settings remain in
`~/.synchub`.

## Linux

### Debian or Ubuntu

```bash
sudo apt install ./synchub_*.deb
```

Start SyncHub from the application menu.

### AppImage

```bash
chmod +x SyncHub-amd64.AppImage
./SyncHub-amd64.AppImage
```

Keep the AppImage at a permanent path before enabling **Start at login**.
SyncHub records that stable path, not the temporary AppImage mount.

## First launch

1. Enter the private repository URL:
   - SSH: `git@github.com:your-name/agent-sync.git`
   - HTTPS: `https://github.com/your-name/agent-sync.git`
2. Authenticate:
   - SSH verifies your local key, `ssh-agent`, GitHub host key, and access to
     the selected repository.
   - HTTPS opens GitHub Device Flow. The OAuth token is stored only in the
     operating-system keyring.
3. Choose agents to synchronize. Detected agents are enabled by default.
4. Select **Start synchronizing**.
5. Open **Sync settings** and review the resource categories, source-to-target
   mappings, excluded files, and restore totals.
6. If the repository contains Plugin declarations or Skill dependencies,
   review the exact executable and arguments in the installation plan before
   approving it.

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
