# Reset Local Setup

## Goal and Scope

Provide a Reset and start over action in Settings and onboarding so users can
discard SyncHub setup and its local clone, then repeat onboarding without
restarting the application.

## Interaction

Show the resolved local clone path before confirmation. Require the exact text
`RESET`, and disable confirmation while an operation or synchronization is
active. Failed reset stays on the current screen with an explicit error.
Successful reset clears frontend state and returns to the Welcome screen.

## Data Boundaries

Remove only named SyncHub artifacts: configuration, sync snapshot, merge bases,
local conflicts and resolution metadata, pending install plans and approvals,
desktop summaries, and SyncHub OAuth metadata/credential. Remove the selected
local clone after validation. Preserve remote Git data, agent source files,
SSH configuration and keys, provider definitions, logs, update preferences, and
start-at-login registration.

Do not remove the application home itself. Reject a clone that is a filesystem
root, the user home, an ancestor of application data, a resource source/target,
a symlink, not a standalone Git clone, or contains
unexpected top-level files. Missing artifacts are valid, including incomplete
onboarding with no clone. Custom clones must have an origin exactly matching
the selected repository URL. The dedicated default clone does not require an
origin match: changed URLs, SSH aliases, missing origins, and incomplete
onboarding must not prevent resetting its local setup.

## Execution and Failure Handling

Serialize mutating desktop operations with reset. Use the scheduler's
PauseIfIdle guard, cancel its runner and wait for completion before touching
files. Invalidate preview generations before removing summaries. Keep the
desktop runner available to start a fresh daemon after onboarding.

Move validated, named data into a temporary staging directory under the
application home, and stage the clone in a temporary sibling directory on its
own volume before deleting it. Roll back moves if staging or credential
removal fails. Snapshot credentials in memory before deleting either current
or legacy entries and restore them if deletion fails; report restoration
failures explicitly. After successful file rollback, recreate the daemon,
preserve its paused state, and suppress an automatic startup sync. If final
cleanup fails, report its exact path and do not return a successful reset.
Never change the remote or run application sync in tests.

Reset onboarding memory and local UI state only after reset succeeds.

## Verification and Acceptance

- Synthetic fixture tests verify data removal, retained source/remote files,
  credential errors, invalid paths, and no deletion without RESET.
- Lifecycle tests prove idle stop and subsequent reconfiguration, and reject
  reset during an active job.
- UI tests verify confirmation gating, explicit errors, and Welcome transition.
- Run repository check and source-bound verify before handoff.
