# SyncHub Full Rename Design

## Goal

Deliver a full product rename from AgentConfigSync/acsync to SyncHub naming in
one release, while preserving upgrade continuity for existing users through an
automatic, rollback-safe migration.

## Scope

This design covers:

- user-visible names (UI, installer, shortcuts, process names, docs);
- build and packaging outputs;
- Go module path and imports;
- CLI command rename (`acsync` -> `synchub`);
- runtime/config/repository working directories;
- startup registration key rename;
- migration from legacy paths and keys.

This design does **not** require rewriting historical archive documents under
`docs/superpowers` that are kept as project history.

## Architecture

### 1) Naming boundaries

All active runtime and delivery surfaces move to SyncHub naming:

- executable: `SyncHub.exe`;
- installer asset: `SyncHub-for-Agents-Setup-x64.exe`;
- product display name: `SyncHub for Agents`;
- module path: `github.com/qinqingxu/synchub-for-agents`;
- CLI command: `synchub`.

Build scripts, CI workflows, release automation, smoke scripts, and active
product docs are updated to the new names so generated outputs and release
artifacts are consistent.

### 2) Legacy migration contract

A dedicated migration layer runs on first startup after upgrade:

- detects legacy runtime/config directory (for example `~/.acsync`);
- performs atomic migration to new SyncHub path (copy + verify + switch);
- migrates startup Run key from old name to new name;
- preserves repository continuity and existing synchronized data;
- remains idempotent on repeated startups.

Only migration-recognition constants are allowed to contain legacy values.
Every non-migration active surface must be legacy-name-free.

## Data and control flow

1. App start enters migration preflight.
2. If no legacy state exists, continue normal startup.
3. If legacy state exists:
   - create migration checkpoint metadata;
   - copy legacy state to new location;
   - verify copied structure and critical files;
   - switch active state pointer to new location;
   - migrate startup Run key;
   - mark migration complete.
4. On failure at any step:
   - keep legacy state intact;
   - roll back partial new state according to checkpoint;
   - surface explicit error to user.

## Error handling

- No silent fallback from failed migration.
- Partial migration never becomes the active state.
- Startup key migration failure is surfaced and does not corrupt sync state.
- Re-running startup after failure must either complete migration cleanly or
  remain on legacy state without data loss.

## Testing and verification

### Automated

- full `go test ./...`;
- `npm --prefix frontend run build`;
- migration unit tests: success path, failure rollback, idempotent rerun;
- startup key migration tests: old key removal + new key creation;
- packaging/smoke tests updated for new executable and installer names.

### Release checks

- built asset name is `SyncHub-for-Agents-Setup-x64.exe`;
- shortcut and installed binary names reflect SyncHub naming;
- no legacy names on active user-visible surfaces;
- existing installation upgrades without losing sessions/settings/repository.

## Rollout

Release as a single rename version (recommended `v0.2.0`) that includes:

1. full active-surface rename;
2. migration + rollback safeguards;
3. updated release artifact names and checksums;
4. updated README/install docs.
