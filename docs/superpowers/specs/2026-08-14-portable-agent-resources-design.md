# AgentConfigSync Portable Agent Resources Design

- Date: 2026-08-14
- Status: Approved
- Scope: Portable configuration, instructions, Skills, and Plugins for supported agents

## 1. Goal

AgentConfigSync currently synchronizes a safe subset of agent configuration and
session files. This change expands it into a portable agent-environment backup:
after connecting a new computer, a user can restore agent settings,
instructions, Skills, Plugin declarations, and sessions without copying
credentials, machine state, caches, downloaded runtimes, or platform-specific
binaries.

The supported logical resource categories are:

1. `sessions`: conversation and recovery records.
2. `config`: portable application and agent settings.
3. `instructions`: global `README.md`, `AGENTS.md`, `CLAUDE.md`, prompts, and
   similar instruction files.
4. `skills`: Skill source, documentation, scripts, assets, and dependency
   manifests.
5. `plugins`: installed Plugin declarations, configuration, and user-authored
   Plugin source.

Agent-specific resources and common resources such as `~/.agents` are both in
scope. Files inside project repositories are out of scope because the project's
own version control should manage them. Users may add explicit custom resource
directories.

## 2. Confirmed Product Decisions

- Use a resource manifest and typed adapters instead of mirroring whole agent
  directories.
- Synchronize Plugin declarations and user-authored source. Reinstall Plugins
  on a new machine instead of copying downloaded payloads.
- Synchronize Skill source and dependency manifests. Rebuild dependencies
  instead of copying `node_modules`, test runtimes, or compiled binaries.
- Synchronize safe fields from mixed configuration files while preserving each
  computer's local credentials.
- Synchronize portable settings only. Normalize known home-directory paths and
  keep machine-specific paths and state local.
- Use three-way merging for concurrently edited files. Preserve both versions
  and notify the user when automatic merging is not safe.
- Show and require confirmation for the first Plugin or Skill dependency
  installation plan on a computer. Later updates from the same approved source
  may run automatically.
- Keep deletion propagation and the existing 30-day recovery period.
- Default all safe resource categories to enabled, with category-level controls
  in the desktop settings UI.

## 3. Non-Goals

- Copying credentials, OAuth state, API keys, browser sessions, or login
  databases.
- Copying entire agent homes, package caches, active databases, logs, downloaded
  application runtimes, `node_modules`, virtual environments, or test SDKs.
- Scanning arbitrary project repositories for instruction files.
- Executing arbitrary install commands supplied by a synchronized repository or
  a custom provider.
- Guaranteeing byte-identical agent directories across operating systems.
- Replacing a project's Git repository or an agent vendor's account sync.

## 4. Architecture

### 4.1 Typed resources

Provider manifests move from one root with `include` and `sessions` globs to a
versioned list of typed resources. Each resource has one responsibility and one
restore strategy.

Conceptually, a resource declaration contains:

```yaml
schema_version: 2
name: claude
resources:
  - id: settings
    category: config
    paths:
      windows: "%USERPROFILE%\\.claude"
      darwin: "~/.claude"
      linux: "~/.claude"
    include: ["settings.json", "settings.local.json"]
    exclude: ["**/.credentials.json", "**/*token*"]
    strategy: structured-merge
    transformer: claude-settings

  - id: global-instructions
    category: instructions
    paths:
      windows: "%USERPROFILE%\\.claude"
      darwin: "~/.claude"
      linux: "~/.claude"
    include: ["CLAUDE.md", "commands/**", "prompts/**"]
    strategy: text-tree

  - id: skills
    category: skills
    paths:
      windows: "%USERPROFILE%\\.claude\\skills"
      darwin: "~/.claude/skills"
      linux: "~/.claude/skills"
    strategy: source-tree
    shared_as: common-skills

  - id: plugins
    category: plugins
    paths:
      windows: "%USERPROFILE%\\.claude\\plugins"
      darwin: "~/.claude/plugins"
      linux: "~/.claude/plugins"
    strategy: install-manifest
    installer: claude-plugin
```

Version 2 manifests use this structure. `category` is one of the five categories
in Section 1. `strategy` is one of `file-tree`, `text-tree`,
`structured-merge`, `source-tree`, or `install-manifest`. `transformer`,
`installer`, and `shared_as` are optional stable adapter IDs, but are required
when their corresponding strategy needs them. Unknown categories, strategies,
and adapter IDs fail provider loading instead of falling back to raw copying.

The built-in adapters are:

- file-tree adapter for sessions and instructions;
- structured configuration adapter for JSON, YAML, and TOML;
- source-tree adapter for Skills and user-authored Plugins;
- install-manifest adapter for vendor Plugins and Skill dependencies;
- common-resource adapter for shared resources and aliases.

Custom directories may use file-tree, text-tree, or structured configuration
strategies. They cannot define executable installer commands. Installers are
compiled, trusted adapters shipped with AgentConfigSync.

### 4.2 Common resources and aliases

The same resource can appear at multiple agent paths. On the current machine,
both `~/.claude/skills` and `~/.agents/skills` are symbolic links to the same
directory. The collector must resolve the root link, identify the canonical
filesystem object, and synchronize it once as `common-skills`.

Independent directories with the same content may be deduplicated using a tree
hash, but path identity is the primary and cheaper signal. Nested links that
escape an approved resource root are not followed unless the user approves the
external target.

Restore behavior is:

1. Keep an existing valid local link and its target.
2. On a new computer, materialize common Skills at `~/.agents/skills`.
3. Create agent-specific links to the common path when the operating system
   permits it.
4. On Windows systems without symbolic-link permission, create managed copies
   and record their alias relationship so subsequent syncs do not upload
   duplicates.

The repository never stores a machine's absolute link target.

### 4.3 Backward-compatible repository layout

Existing `agents/<provider>/config` and `agents/<provider>/sessions` paths remain
valid. They are not moved during the initial migration.

Older AgentConfigSync versions classify unexpected top-level paths as unsafe
and may delete them. New portable metadata and resources therefore live under
an internal pseudo-provider that old clients treat as an unknown, disabled
provider and leave untouched:

```text
agents/
  claude/
    config/
    sessions/
  copilot/
    config/
    sessions/
  _portable/
    config/
      schema.json
      providers/
        claude/
          config/
          instructions/
          plugins/
        copilot/
          config/
          instructions/
          plugins/
      common/
        skills/
        instructions/
      install/
        claude-plugins.json
        copilot-plugins.json
        skill-dependencies.json
      conflicts/
```

The `_portable` provider is internal and cannot be disabled independently. Its
files are still scanned, allowlisted, reconciled, and covered by deletion
protection. New clients generate the portable manifest without destructively
rewriting existing session history. `_portable` is a reserved provider ID and
cannot be used by custom provider definitions.

### 4.4 Initial built-in coverage

The first implementation covers these portable sources:

| Provider | Config | Instructions | Skills | Plugins |
| --- | --- | --- | --- | --- |
| Claude | `settings.json`, `settings.local.json`, `config.json`, and known portable fields from related JSON config | `CLAUDE.md`, commands, prompts, and Markdown documentation under the Claude home | `~/.claude/skills`, normally aliased to common Skills | declarations and origin metadata discovered from `~/.claude/plugins`; user-authored source when explicitly identified |
| Copilot CLI | `config.json`, `settings.json`, and portable permission preferences | global Markdown instructions and prompts under the Copilot home | `~/.copilot/skills`, normally aliased to common Skills | declarations from `~/.copilot/installed-plugins`; user-authored source when explicitly identified |
| Gemini CLI | portable fields from `settings.json`; project trust, account, installation, and state files remain local | `GEMINI.md`, commands, prompts, and Markdown documentation under the Gemini home | a detected Gemini Skills root when supported by the installed version | declarations from a detected Gemini extension or Plugin inventory when the installed version exposes trustworthy origin metadata |
| VS Code Copilot | user `settings.json`, `keybindings.json`, `mcp.json`, portable profile settings, and snippets | `User/prompts` and profile prompts/instructions | common Skills referenced by Copilot, without copying extension global storage | no raw `globalStorage` copy; only a future trusted Copilot Plugin inventory exposed by the installed product |
| Cursor | portable fields from Cursor settings | global rules, prompts, and Markdown instructions under the Cursor home | a detected Cursor Skills root | declarations only when Cursor exposes trustworthy origin metadata |
| Common | portable Skill lock and manifest fields | global instructions and hooks under `~/.agents` | `~/.agents/skills` as the default canonical common root | common Plugin declarations supplied by a trusted built-in adapter |

Existing session rules for Claude, Copilot CLI, Gemini CLI, VS Code Copilot, and
Cursor remain in force. A source listed above is still omitted when it is absent
or when its installed product version does not expose enough metadata to restore
it safely.

## 5. Collection and Filtering

### 5.1 Collection pipeline

For each enabled resource, a sync pass:

1. resolves the declared path for the current platform;
2. resolves and validates root links;
3. discovers matching files without following unapproved escaping links;
4. excludes generated, local-only, or unsupported content;
5. reads each selected file once into immutable staging;
6. transforms structured configuration into a portable projection;
7. scans the exact staged bytes that will be committed;
8. records hashes, resource identity, aliases, restore strategy, and install
   declarations in the portable manifest;
9. reconciles the staged snapshot with local state and the pulled repository.

The existing immutable staging and remote re-scan protections remain mandatory.

### 5.2 Default generated-content exclusions

Source-tree resources exclude at least:

- `.git`, `.svn`, and other VCS internals;
- `node_modules`;
- `.venv`, `venv`, Python caches, and compiled Python files;
- `.vscode-test` and downloaded editor/test runtimes;
- package-manager caches;
- temporary, log, lock, and active database files;
- OS-specific executables, shared libraries, and native modules that a declared
  dependency can rebuild or reinstall;
- generated coverage and test-output directories.

Build output is not excluded solely because its directory is named `dist` or
`build`; some Skills ship required generated JavaScript. A built-in adapter must
understand the corresponding ecosystem, or the file remains subject to normal
safety scanning and size limits.

Files at or above 50 MiB are blocked with an actionable message. This stays
below GitHub's individual-file limit and catches accidental runtime archives.
The UI reports excluded file counts and bytes before the first upload.

### 5.3 Plugin inventory

Install-manifest adapters store portable declarations, not installed payloads.
A declaration contains the provider, stable package or Plugin identifier,
source registry or repository, requested version or revision, integrity
information when available, enabled state, and portable settings.

User-authored local Plugins may additionally use the source-tree adapter. Git
metadata, dependencies, build caches, and secrets are excluded.

An adapter must not infer an install declaration from an opaque downloaded
directory when it cannot identify a trustworthy source. Such a Plugin is shown
as "not portable" with a reason instead of being silently copied.

## 6. Portable Configuration

### 6.1 Safe projections

A structured configuration adapter never writes the original mixed file to the
repository. It parses the local file and produces a portable projection that
contains only approved settings.

Each built-in transformer defines:

- portable field paths;
- credential and authentication field paths;
- machine-local state field paths;
- portable path fields;
- schema-specific validation.

Credential fields are excluded even when their names do not match the generic
secret patterns. Machine-local fields include installation IDs, window state,
workspace trust, caches, recent-file lists, process state, and absolute paths
that cannot be portably mapped.

Known user-home path prefixes are represented using a platform-neutral home
token. Only transformer-declared path fields are normalized. Other absolute
paths remain local.

The existing generic secret scanner runs after projection. Logs and manifests
may identify a blocked field path but never include its value.

### 6.2 Restore merge

On restore, the adapter parses the current local file and applies the portable
projection. Local credential and machine-state fields remain unchanged.

If no local file exists, the adapter creates one from the portable projection
without authentication fields and reports that the agent may require login.

If parsing, validation, or projection fails, that resource fails closed. The
original local file remains untouched. Unknown custom structured resources use
generic secret scanning; if safe field-level handling cannot be proven, the
whole file is blocked.

## 7. Synchronization, Conflict, and Deletion Semantics

### 7.1 Three-way merge

Every resource compares:

- the current local portable representation;
- the last successful portable snapshot;
- the pulled remote portable representation.

Merge behavior is strategy-specific:

- structured configuration merges independent keys;
- text files use a three-way text merge;
- file trees merge independent paths;
- binary assets merge only when one side changed;
- sessions retain their existing union and deletion behavior, with conflicting
  edits to the same record treated as a file conflict.

If both sides change the same key, text hunk, or binary file incompatibly,
AgentConfigSync does not choose a winner by modification time.

### 7.2 Conflict storage and resolution

An unresolved conflict stores the base, local, and remote safe representations
under the `_portable` conflict area and in the local AgentConfigSync conflict
store. These copies pass the same secret scan as normal synchronized content.

The active local file is not overwritten, and the remote canonical file is not
advanced until resolution. Other unrelated resources continue syncing.

The desktop conflict panel allows the user to choose local, choose remote, or
save an edited merged result. Resolution creates one new canonical version and
removes the conflict record. Conflict artifacts are never placed beside agent
files where an agent could interpret them as live configuration.

### 7.3 Deletion

Deletion tombstones remain part of the three-way state model. A deletion on one
computer propagates to other computers and enters the existing 30-day remote
trash.

For Skills and Plugins:

- deleting a common Skill removes its aliases on other computers;
- removing a Plugin declaration disables the Plugin and moves its managed local
  payload into an AgentConfigSync local recovery area;
- restorable source and declarations remain recoverable during the grace
  period;
- generated dependencies and downloaded payloads may be physically removed
  after the grace period because they can be rebuilt.

Deletion is suspended for a resource with an unresolved conflict.

## 8. Restore and Installation

### 8.1 New-computer restore

After a new computer connects to a repository, the desktop app presents a
restore preview containing:

- detected agents and target paths;
- resources and categories to restore;
- file counts and estimated bytes;
- paths that require mapping;
- Plugins and Skill dependencies to install;
- resources blocked or unavailable on this platform.

Safe file resources may be restored together. Plugin and dependency
installation requires a separate first-run confirmation.

### 8.2 Trusted installation plans

Installation commands are generated by built-in adapters from validated
declarations. A remote file cannot supply a shell command.

The confirmation view displays the provider, source, version, target, and exact
command or API operation. Approval is stored locally using the provider and
source identity. Later version updates from the same approved source may run in
the background. A source or registry change requires new approval.

Installers run without interactive elevation. A dependency or Plugin that
requires administrator access is reported and left pending.

An install failure affects only that resource. It is shown as failed, retried on
request or a later cycle, and never reported as a successful restore.

## 9. Desktop Settings and Status

Each agent row in Settings expands into category toggles for Sessions, Config,
Instructions, Skills, and Plugins. All safe, supported categories are enabled
by default. Unsupported categories are visible but disabled with an explanation.

Each category displays:

- detected source and restore target;
- file count and estimated synchronized bytes;
- safety exclusions and excluded bytes;
- current portability or install status.

Settings also contains:

- a Common Resources section;
- an Add Custom Directory workflow;
- a safety preview before enabling a custom resource;
- restore-preview and trusted-installation panels;
- a conflict-resolution panel.

A custom directory requires a stable logical ID, category, current-platform
source, and portable target mapping. It cannot define an installer. Targets
outside the user profile require explicit confirmation.

Sync progress and the final result separately report:

- synchronized;
- restored;
- reinstalled;
- skipped;
- blocked;
- conflicts.

A partial failure leaves safe unrelated resources synchronized but sets the
overall state to "Needs attention" and retains actionable per-resource errors.

## 10. Configuration and Migration

The local configuration schema moves from a provider-level Boolean map to
provider and category settings while retaining a provider master switch.
Loading a version 1 config creates enabled category defaults for every supported
built-in resource and preserves existing provider enablement.

Migration is idempotent. It does not:

- re-enable a provider the user disabled;
- rewrite credential-bearing local files;
- move current session paths;
- delete old repository data;
- execute installation plans.

The first upgraded sync performs a resource preview and records repository
schema metadata under `_portable`. Existing clients can continue synchronizing
known configuration and sessions without deleting the new pseudo-provider.
The UI warns when repository metadata indicates another registered computer has
not yet used the portable-resource schema, but this does not block safe sync.

## 11. Error Handling

- Missing optional resource directory: mark unavailable and continue.
- Unreadable selected file: fail that resource and preserve its previous remote
  version.
- Malformed structured configuration or JSONL: fail closed; do not upload or
  overwrite.
- Secret or credential detection: block the portable artifact, remove any
  unsafe remote copy through the existing secure-delete path, and notify.
- Escaping symbolic link: require explicit trust or skip it.
- Unsupported Plugin source: keep the local installation and report it as not
  portable.
- Install or rebuild failure: retain a pending plan and error details.
- Merge conflict: preserve all safe variants and require resolution.
- Network or Git failure: retain staged state and retry through the existing
  synchronization lifecycle.

Errors are associated with stable resource IDs. User-visible messages do not
contain secret values or raw file contents.

## 12. Validation

### 12.1 Unit tests

Tests cover:

- provider schema v1 compatibility and v2 resource parsing;
- config migration and per-category settings;
- built-in resource discovery;
- generated-content exclusions and the file-size limit;
- root-link resolution, escaping-link rejection, alias detection, and
  copy fallback;
- JSON, YAML, and TOML projection and restore;
- preservation of local credentials and machine state;
- home-path normalization across Windows, macOS, and Linux;
- Plugin inventory and deterministic trusted install plans;
- rejection of repository-supplied executable commands;
- structured and text three-way merges;
- conflict bundle creation and resolution;
- common-resource and Plugin deletion behavior;
- portable repository allowlisting and secret scanning.

### 12.2 Integration tests

Local bare Git repositories and isolated home directories simulate:

1. two computers changing different resources;
2. two computers changing the same text file without overlapping edits;
3. a true conflict that preserves both versions;
4. safe configuration restore while each computer retains different
   credentials;
5. a shared Skill linked into multiple agent homes;
6. Windows link-unavailable copy fallback;
7. Plugin discovery, first-run approval, install success, and install failure
   using command stubs;
8. Plugin and Skill deletion with recovery;
9. version 1 local and remote migration;
10. an older-client-compatible repository layout;
11. malformed and secret-bearing local and remote artifacts.

### 12.3 End-to-end acceptance

Use an isolated temporary profile as a simulated new computer:

1. run the current installed profile through collection and safety preview;
2. synchronize to a local test remote;
3. connect the temporary profile;
4. approve its generated installation plan;
5. restore all supported portable resources;
6. compare portable projections, instruction trees, Skill source, Plugin
   declarations, and sessions;
7. verify credentials and machine state were not copied;
8. verify the repository contains no blocked credentials, active databases,
   caches, downloaded runtimes, or oversized generated files;
9. run the existing Go tests, Go vet, frontend production build, and desktop
   smoke tests.

Acceptance requires successful and persistent restore, not only a generated
plan or a successful upload.

## 13. Delivery Boundaries

Implementation should proceed in dependency order:

1. typed resource model, local config migration, and backward-compatible
   repository namespace;
2. collection filters, common-resource identity, and safe projections;
3. three-way resource merge, conflict storage, and deletion behavior;
4. built-in Claude, Copilot, Gemini, VS Code Copilot, Cursor, and Common
   resource adapters;
5. Plugin and Skill dependency inventory and trusted install plans;
6. desktop category settings, preview, progress, conflict, and install UI;
7. migration and end-to-end restore validation.

The feature is complete only when a fresh isolated profile can restore the safe
portable environment and all exclusion assertions pass.
