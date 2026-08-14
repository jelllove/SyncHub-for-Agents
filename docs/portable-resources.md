# Portable agent resources

AgentConfigSync synchronizes portable agent data through your private Git
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

You can disable an entire agent or an individual category in **Sync settings**.
Use the restore preview to review source paths, restore targets, file counts,
excluded content, and total portable data before saving changes.

## Content that never synchronizes

AgentConfigSync excludes:

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

The first computer publishes its safe portable resources. Blocked and skipped
files appear as safety notices and remain local.

## A new computer

1. Install AgentConfigSync and connect the same private repository.
2. Review the restore preview and target paths.
3. Save your agent and category choices.
4. Review any Plugin or Skill dependency installation plan.
5. Approve the plan only after checking every executable and argument.

Safe files restore immediately. Existing credentials on the new computer are
preserved. AgentConfigSync executes approved installers directly with fixed
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
select a safer include glob, or keep the file local. AgentConfigSync fails
closed and never uploads a blocked file.

### A symbolic link is unavailable

Generated-directory links are always excluded. Other links must resolve inside
an approved safe location. Replace an unnecessary link with a normal directory
or explicitly approve the target through a supported workflow.

### An install command fails

The exact executable, arguments, working directory, and error remain in the
pending installation plan. Install or repair the required tool, then
synchronize again. AgentConfigSync does not silently mark failed operations as
complete.

### Synchronization says Needs attention

Review blocked files, pending installation plans, and conflicts. The state
returns to **Up to date** only after a later clean synchronization cycle.
