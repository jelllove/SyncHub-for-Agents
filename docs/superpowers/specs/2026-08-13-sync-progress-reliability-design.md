# Sync Progress and Reliability Design

## Problem

Desktop synchronization currently reports success while collecting no files
because provider paths are resolved relative to `~/.acsync` instead of the
user profile. Git subprocesses also create visible console windows on Windows,
and the dashboard only confirms that a request was queued rather than showing
the active work or final result.

## Design

### Correct source paths

The runtime will keep the application data directory and operating-system user
home as separate values. Provider templates such as `%USERPROFILE%\.claude`
and `~/.gemini` will always resolve from the actual user home. The repository,
state, logs, and configuration remain under `~/.acsync`.

### Hidden subprocesses

All production Git and SSH subprocesses will pass through one platform helper.
On Windows it will set `CREATE_NO_WINDOW`; other platforms will leave process
attributes unchanged. Command output and errors continue to be captured.

### Structured progress

The sync engine will publish these ordered stages:

1. Pulling remote changes
2. Scanning local files
3. Comparing changes
4. Applying changes
5. Uploading changes
6. Complete

Each update contains a stage, user-facing label, percentage, processed action
count, total action count, and blocked-file count. The daemon forwards progress
to the desktop service, which emits updated snapshots through the existing
Wails event.

The dashboard shows a determinate segmented progress bar while synchronization
is active. On completion it shows one of:

- synchronized and uploaded a specific number of changes;
- checked successfully with no changes needed;
- failed with the actionable backend error.

The Sync Now button only queues the operation; it does not claim completion.

## Error handling

Path resolution errors stop the cycle and appear in the dashboard. A missing
agent directory remains a valid empty source. Git and SSH errors retain stderr
details without exposing credentials. A failed sync leaves the previous
successful timestamp intact.

## Testing

- A runtime regression test proves `~/.acsync` data storage does not alter
  provider source paths.
- Platform process tests prove Windows commands are hidden and non-Windows
  commands are unchanged.
- Engine and desktop tests verify ordered progress, action counts, completion,
  and errors.
- Frontend production build verifies generated models and progress UI.
- A real Windows installer test verifies no console windows and confirms files
  are committed and pushed to the configured private repository.
