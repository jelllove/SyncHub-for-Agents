# Installing AgentConfigSync

AgentConfigSync is a tray application. It synchronizes supported AI-agent
configuration and session files through a private GitHub repository.

## Before installing

Create an empty **private** GitHub repository. Do not add a README or other
files; AgentConfigSync can initialize the repository itself.

For SSH authentication, add an SSH key to GitHub and verify it:

```powershell
ssh -T git@github.com
```

GitHub should report that authentication succeeded.

## Windows

1. Download `AgentConfigSync-amd64-installer.exe` from the latest release.
2. Run the installer. It installs for the current user and does not require
   administrator access.
3. Start **AgentConfigSync** from the Start menu.

Uninstall it from **Settings > Apps > Installed apps**. Your synchronized data
and settings in `%USERPROFILE%\.acsync` are retained so an uninstall cannot
delete your sessions accidentally.

## macOS

1. Download and open `AgentConfigSync.dmg`.
2. Drag AgentConfigSync to Applications.
3. Open it from Applications.

Release builds are signed and notarized. To uninstall, quit the app from its
menu-bar icon and move it from Applications to Trash. Settings remain in
`~/.acsync`.

## Linux

### Debian or Ubuntu

```bash
sudo apt install ./agentconfigsync_*.deb
```

Start AgentConfigSync from the application menu.

### AppImage

```bash
chmod +x AgentConfigSync-amd64.AppImage
./AgentConfigSync-amd64.AppImage
```

Keep the AppImage at a permanent path before enabling **Start at login**.
AgentConfigSync records that stable path, not the temporary AppImage mount.

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

The installer migrates the legacy `acsync` startup entry when the desktop app
first launches. Existing configuration, repository checkout, and session data
in `~/.acsync` are reused.

## Advanced: headless CLI

The `acsync` command remains available for servers and scripted environments.
Desktop users normally do not need it.

```powershell
acsync init --repo git@github.com:your-name/agent-sync.git
acsync sync
acsync status
```

Use the same private repository on each computer. Do not run the desktop app
and the headless daemon simultaneously for the same user account.
