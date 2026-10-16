# Shared resource library

## Goals and scope

Manage a personal source of skills and instructions once, then explicitly apply
it to supported Harnesses. Desktop and CLI share the same backend. This is an
opt-in addition to Git synchronization, not a new configuration standard.

The editable source is `~/.agents/library/skills/<name>/SKILL.md` and its
supporting files, or `~/.agents/library/instructions/AGENTS.md`. This separate
namespace avoids taking over an existing `.agents/skills` directory merely by
opening SyncHub. Edit library sources, not generated copies.

| Resource | Target | Discovery |
| --- | --- | --- |
| Skills | `common`: `~/.agents/skills/<name>` | Native personal skills for compatible tools such as Copilot; one output, not an additional `.copilot` copy |
| Skills | `claude`: `~/.claude/skills/<name>` | Claude Code personal skills |
| Global instructions | `claude`: `~/.claude/CLAUDE.md` | Claude Code personal instructions |

Import also accepts skills from `~/.copilot/skills`. Applying to `common`
refuses an existing same-name Copilot private skill to avoid duplicate native
discovery. After importing and reviewing it, move or rename that private copy
yourself before applying. SyncHub never silently deletes it.

All tools that natively discover `.agents/skills` can see its contents; the
`common` target cannot individually disable those tools. Mapping support does
not establish semantic compatibility with every tool-specific frontmatter
extension. Reload/restart the Harness when its own discovery requires it.
Discovery paths follow [Copilot skills documentation](https://docs.github.com/en/copilot/concepts/agents/about-agent-skills)
and [Claude Code skills documentation](https://code.claude.com/docs/en/skills).

Non-goals for this version: project-level overrides, plugin installation, MCP
configuration conversion, model/permission unification, automatic semantic
rewriting, and universal Harness support.

## Interactions

Open **Settings > Library**. The illustrated flow distinguishes resources
already on this computer, the shared source, and compatible tool destinations.
An empty library starts with **Add existing resources**, rather than an unusable
application form. Choose a source, preview its import, then explicitly confirm.
Use **Use shared resources** to select a shared source and a compatible location.
Both steps are separate approvals; importing never implicitly approves a target.
If no eligible resource exists, the page explains where to put a supported skill
or instruction and offers a rescan, instead of an empty target dropdown.

The task-oriented controls correspond to these backend operations:

- **Import** copies a reviewed candidate into the canonical source. It does not
  delete or take ownership of the import origin. A same-name replacement is
  shown explicitly in the preview.
- **Apply** previews the complete target change, including removal of files
  not present in the source. Confirming initially adopts only that target.
- **Restore** returns an unchanged managed target to its pre-adoption content.
  It refuses intervening user edits. Empty directories may remain.
- **Detach** removes ownership without changing target contents. Use this to
  retain a private edit instead of restoring a backup.

Recovery controls are grouped under **Manage existing copies**, only when
managed copies exist. Blocked resources retain their diagnostic reasons and
never become selectable. Configuration and plugin conversion remain outside
this library's scope; the illustration does not promise universal compatibility.

Every mutation requires its exact preview ID. Source, destination or ownership
changes invalidate the ID. Previews include before/after text: do not share
them publicly without reviewing their contents.

For target drift, import the target's edits if they should become shared,
then detach and preview apply again. Otherwise detach to retain the private
copy, or manually reconcile it before restoring. There is no force-overwrite
or automatic bidirectional merge.

## Synchronization and ownership

Enable **common / Skills** and **common / Instructions** to synchronize the
corresponding canonical sources through the existing private Git repository.
They have distinct portable resource IDs, `library-skills` and
`library-instructions`. On a new computer, review and approve targets locally;
pulling a source never grants target-write approval.

After a synchronization without attention-required results, previously approved
targets in enabled common categories refresh from their sources. Disabling
common Skills or Instructions also disables that category's automatic refresh;
explicit preview/apply remains available. Changed targets, missing sources, blocked
files and incomplete operations fail visibly instead of overwriting data.
Sources arriving during a conflicted/attention-required sync are not
automatically projected in that cycle. The next clean cycle or an explicit
preview/apply can update them.

Names present in the library, or still owned locally, are reserved from legacy
skill alias synchronization. Legacy remote entries remain unowned, rather than
being restored or deleted. Unrelated resources retain their old behavior.
Remote library names are reserved before local collection on a fresh computer,
not only after their canonical sources have been restored. Absent canonical
category directories are empty local sources, not failed scans, so first restore
does not require creating those directories manually.
While library skill names exist, legacy shared skills use directory copies,
not new whole-root symlinks. Custom resources overlapping canonical sources or
known output paths are also protected.

Keeping a detached, same-name resource private does not re-enable its old
alias synchronization while that name remains in the library. Rename the
private resource if it should have a separate identity.

## State, backups and failure handling

Local ownership and pre-adoption backups live in
`~/.synchub/library-ownership.json`; they are shared across profiles for this
OS user, never synchronized. Imports retain their previous canonical content
in `~/.synchub/library-import-backups/<preview-id>.json` (file bytes are encoded
by JSON as base64). These backups have no automatic retention/deletion policy.
Resetting repository setup does not erase library sources or ownership.

Before writing managed files, SyncHub durably records the original, previous
and intended trees with a pending marker. A failed operation remains pending;
preview **Restore** to recover only if current files still match the recorded
operation. New external modifications prevent recovery from overwriting them.
Import write failures attempt to restore canonical content and return any
rollback error. If the process terminates during an import, inspect and recover
the retained import backup before importing again.

A user-home OS lock serializes library operations with synchronization, in
addition to the existing Desktop/CLI profile lock. Busy errors do not interrupt
another owner. Older SyncHub versions do not honor these reservations or locks;
do not run them concurrently against managed directories.

## Safety and limits

- No commands, installers, dependency managers or scripts are executed by library
  operations. Existing sync dependency plans still require approval.
- Only regular UTF-8 source files, at most 1 MiB each, 20 MiB total and 2,000
  files per resource are accepted. A skill requires `SKILL.md`.
- Names use lowercase letters, digits, hyphens or underscores (1-64 characters);
  Windows device names are rejected.
- Generated dependencies, binaries, recognized credential paths and
  secret-looking content are rejected, not silently omitted.
- The canonical source policy also runs before Git staging, remote validation
  and restoration, not only when applying a target.
- Links and linked ancestors are rejected. Existing legacy aliases appear as
  blocked; convert them to reviewed real directories before adoption. SyncHub
  does not automatically unlink or migrate an existing alias.
- Safety scanning is conservative, not a guarantee that arbitrary prose cannot
  contain a secret. Review both import and application previews.

## Architecture and validation

### Settings UX and language delivery

The approved settings redesign uses compact, accessible flow diagrams for
repository connections, sync/recovery, resource filtering, the shared library,
desktop startup/notifications, and reset. Diagrams explain relationships, not
live progress, and never replace the underlying approval or risk disclosure.
Existing settings drafts and keyboard-operable category tabs remain intact.

Settings use a centered workspace with a persistent left category sidebar.
Up/Down, Left/Right, Home and End navigate the categories; changing categories
starts at the top without clearing drafts. The sidebar remains vertical at
narrow widths, with compact labeled icons rather than hiding category names.
The Library page has one heading and a compact flow diagram. Resource paths
and supplementary safety details are expandable; compatibility warnings,
blocked reasons and actual change previews remain available. Recovery controls
follow the normal resource task instead of interrupting first use.

The Library and Advanced tabs do not offer the general **Save settings** action:
library writes and reset retain their own confirmation barriers. Other settings
drafts remain available when returning to their tabs. Resource lists are bounded
so the destination and preview action remain easy to reach. Browser verification
covers the native fallback window size (1080 x 720), preferred size (1200 x 850),
and smaller windows, not only oversized presentation screenshots.

English and Simplified Chinese are selectable during first-run setup and in the
settings header. After setup, the home screen has no language selector; open
settings to change the language from any category. The interface changes
immediately without reloading, resubmitting
forms, rescanning resources, or invalidating file previews. A device-local
WebView preference remembers the choice; it is not synchronized through Git.
Without a saved choice, Chinese system/browser languages use Simplified Chinese
and other languages use English. Persistence failures are shown explicitly.
User resource content, identifiers, paths, and backend diagnostics are not
translated or rewritten.

Implementation order: test the persistent language store and selector, replace
the library operation form with guided task controls, add category illustrations,
translate the remaining desktop surfaces, then run the normal full verification
and an isolated browser fixture in both languages. The existing React/TypeScript
toolchain is sufficient; no translation service or external font/image download
is required.

Additional acceptance criteria:

- An empty library leads to adding resources; no blank destination selector.
- Import, application, recovery, and detach all retain exact preview approval.
- Switching language preserves drafts, selected resources and pending approvals.
- Both languages have clear blocked/empty/error states and readable narrow layouts.
- Each settings category has a labeled diagram that reflects supported behavior.
- A fresh mount restores the selected language; storage errors do not look saved.

[The shared library backend](../internal/sharedlibrary/) owns adapter paths,
bounded scanning, previews, local receipts, drift checks and recovery.
[CLI composition](../internal/cli/library.go) provides the common locked API;
[Desktop methods](../internal/desktop/library.go) expose it through generated
Wails bindings. [Resource ownership](../internal/resource/spec.go) is enforced
in local collection, remote planning and restore/delete application so generated
outputs cannot become competing sources.

Verifiable acceptance criteria:

1. Preview does not change source/target content; stale approval changes nothing.
2. Import preserves its origin and retains a canonical recovery backup.
3. Apply updates only the selected target; native discovery is not duplicated.
4. Subsequent target edits block refresh and restore; detach preserves them.
5. Original files are restored after ordinary and interrupted operations.
6. Legacy sync cannot collect or restore reserved output names.
7. Desktop and CLI use the same request and preview contract.

Run focused checks from the repository root:

```text
go test ./internal/sharedlibrary ./internal/cli ./internal/desktop ./internal/resourcecollect ./internal/syncengine ./internal/provider ./cmd/synchub
npm --prefix frontend test -- src/SharedLibraryPanel.test.tsx src/SettingsPanel.test.tsx
node scripts/dev.mjs verify
```

Tests use isolated synthetic homes. File-mapping/component tests do not prove
that an installed external Harness loads the resource, or that every native
OS UI has been exercised.
