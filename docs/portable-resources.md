# Portable agent resources

SyncHub synchronizes portable agent data through your private Git
repository. It scans every file before staging it and keeps credentials and
machine-specific state on the computer where they were created.

## Resource categories

- **Sessions** preserve supported conversation histories as file trees.
- **Config** projects only approved portable fields from JSON, YAML, and TOML.
  Local credentials and machine identifiers remain untouched during restore.
- **Instructions** synchronize Markdown and other supported text instructions
  with three-way merging.
- **Skills** synchronize source files and trusted dependency manifests. Generated
  dependencies are rebuilt locally.
- **Plugins** synchronize declarations only. Plugin payloads and executables are
  installed locally after approval.
- **Common resources** represent shared instructions or Skills restored to
  multiple supported agents.

The opt-in [shared library](shared-library.md) adds canonical sources under
`~/.agents/library` with previewed, per-target adoption. Its managed names are
reserved from legacy alias restore/collection; local backups and target approval
remain on each computer. Enable common Skills/Instructions to synchronize the
corresponding sources. This is distinct from automatically sharing whole skill
directories.

You can disable an entire agent or an individual category in **Sync settings**.
Use the restore preview to review source paths, restore targets, file counts,
excluded content, and total portable data before saving changes.

## Content that never synchronizes

SyncHub excludes:

- access tokens, OAuth data, passwords, API keys, and credential files;
- machine IDs and other machine-local configuration fields;
- databases, caches, logs, temporary files, and test runtimes;
- generated dependencies such as `node_modules`, virtual environments, and
  `__pycache__`;
- platform binaries such as `.exe`, `.dll`, `.so`, `.dylib`, and `.node`;
- files that are 50 MiB or larger;
- unapproved symbolic links and files that fail safety projection or scanning.

The Git repository stores only projected configuration, safe source files,
session files, and declarative installation metadata.

## First computer

1. Connect an empty private Git repository.
2. Select the agents and categories to synchronize.
3. Review the restore preview.
4. Start synchronization.

The first computer publishes its safe portable resources. Policy-excluded files
remain local without routine skip reminders.

## Sync diagnostics and acknowledgement

The dashboard no longer displays **Skipped** or **Blocked** cards. Normal
exclusions do not produce **Needs attention**, tray warnings, review buttons, or
diagnostic-dialog entries. This covers generated content, platform binaries,
oversized files, detected credentials and remote machine-local fields. Missing
resource roots are automatic exclusions only when that resource has no files
in the remote or saved synchronization base. Missing roots for previously
synchronized resources, access failures and invalid configuration remain
actionable. Older `root-unavailable` records cannot safely distinguish an absent
directory from an access failure, so they remain visible until a new cycle
records the precise cause.

When actionable records exist, the keyboard-accessible **Issues** button opens
the **Sync diagnostics log** for the last successful sync-and-cleanup cycle.
Filter by collection issues, safety blocks, or operational issues to inspect
the resource, reported path, reason, and code. File issues use the relative path
reported by the cycle; resource-level failures may report the source root.
The displayed count is the number of visible issue records, including
duplicates, not the number of failed files. Original counts and every record,
including automatically excluded items, remain in local cycle history and
daemon logs; filtering the UI does not change collection or synchronization.

A failed later cycle leaves the successful-cycle log available and its current
error visible. Older summaries saved without details explicitly report that
historical records are unavailable; they cannot be acknowledged and do not
invent an issue count. The log never substitutes a newer settings preview for
historical cycle details.

Choose **Review notices** beside the dashboard status, or open **Issues**.
Choose **Review safety notices** to show the recorded actionable set, then
**Confirm acknowledgement**. The log scrolls independently of its header and
confirmation controls, so the buttons remain visible even with many records.
Acknowledgement is local to this SyncHub setup and
persists across restarts. A versioned fingerprint identifies the visible records,
counts, and reasons, including duplicates. Unchanged safety blocks remain reviewed
when timestamps, record order, or file sizes change. Updated sizes remain in the
log; new paths, reasons, codes, or counts require review.
If the set changes before confirmation, the operation fails visibly: close and
reopen the log to review the current records. Load and persistence failures are
reported rather than silently treating notices as reviewed.

Reviewed safety blocks no longer cause **Needs attention** by themselves;
their records remain available with a reviewed indicator. Missing synchronized
sources, unavailable links, failed configuration projection, I/O, scanner,
write and unknown failures cannot be dismissed by acknowledgement. Actual sync
errors, unresolved conflicts and pending/failed installation plans still
require attention. Conflicts retain their local/remote/merged-content choices;
automatic exclusion never chooses a side. Acknowledgement neither approves installation nor
resolves conflicts. Opening, filtering, or acknowledging the log never triggers
sync or changes secret, path, symbolic-link, or size protections. **Reset local
setup** removes both the retained summaries and acknowledgement.

## A new computer

1. Install SyncHub and connect the same private repository.
2. Review the restore preview and target paths.
3. Save your agent and category choices.
4. Review any Plugin or Skill dependency installation plan.
5. Approve the plan only after checking every executable and argument.

Safe files restore immediately. Existing credentials on the new computer are
preserved. SyncHub executes approved installers directly with fixed
argument lists; it does not construct shell command strings or request elevated
permissions.

Failed commands remain pending. Fix the reported problem, such as a missing
`npm`, `uv`, `go`, Claude, or Copilot executable, then run synchronization
again.

## Conflicts

Independent text and structured configuration edits merge automatically.
Conflicting edits to the same value create a conflict bundle while unrelated
files continue synchronizing.

The dashboard offers:

- **Use local** to keep this computer's complete version;
- **Use remote** to keep the repository version;
- **Edit merged** to provide reviewed merged content.

Merged content is scanned before it can replace the repository file. Deletion
of an unresolved conflicted file is suspended until the conflict is resolved.
Queued resolutions also check the current local resources before changing any
files. If a selected path is blocked or skipped, the entire batch stays queued,
the local files and conflict bundles are preserved, and the log explains why it
is waiting. Address the collection issue and sync again to apply the batch.

## Custom resources

The custom resource editor requires:

- a stable ID containing letters, numbers, `.`, `_`, or `-`;
- one category;
- a source and restore target for the current platform;
- at least one include glob;
- a supported strategy.

Custom Sessions must use `file-tree`. Custom Plugin source must use
`source-tree`. `install-manifest` is reserved for built-in trusted adapters and
cannot be selected for a custom directory. Preview every candidate before
adding it.

Custom resources use the same credential scanner, generated-content filters,
size limit, executable restrictions, and symbolic-link approval rules as
built-in resources.

## Deletion and recovery

A deletion is synchronized to other computers after the next successful cycle.
Deleted repository files move to `.trash` before removal. The default recovery
window is 30 days and can be changed in settings. Shared resources are deleted
from every configured alias.

Plugin uninstall operations preserve a local recovery copy before execution.
Recovery copies and `.trash` are not treated as portable agent resources.

## Troubleshooting

### A file is blocked

Open the safety notices in the restore preview. Remove credential content,
select a safer include glob, or keep the file local. SyncHub fails
closed and never uploads a blocked file.

### A symbolic link is unavailable

Generated-directory links are always excluded. Other links must resolve inside
an approved safe location. Replace an unnecessary link with a normal directory
or explicitly approve the target through a supported workflow.

### An install command fails

The exact executable, arguments, working directory, and error remain in the
pending installation plan. Install or repair the required tool, then
synchronize again. SyncHub does not silently mark failed operations as
complete.

### Synchronization says Needs attention

Open the dashboard **Issues** log and inspect the reason and path.
After acknowledgement, an otherwise successful cycle no longer needs attention
for the same reviewed safety blocks. Normal exclusions need no acknowledgement.
Fix actual sync/operational errors, resolve conflicts,
and review pending installation plans separately; acknowledging notices cannot
clear these gates.
