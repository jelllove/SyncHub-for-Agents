# Batch Conflict Resolution and Sync Performance Design

## Goal

Make AgentConfigSync responsive during routine desktop actions, resolve many
conflicts with one user confirmation and one synchronization, reduce a
no-change synchronization from roughly three minutes to less than 30 seconds,
and turn locked Plugin updates into actionable manual-retry work instead of a
repeating sync failure.

The measured baseline on the real profile is:

- A desktop status collection takes about 85 seconds.
- `Snapshot()` performs both status collection and resource preview collection.
- A complete synchronization takes about three minutes.
- Resolving one conflict immediately queues a complete synchronization.
- There are currently 78 conflicts, including 73 Common Skill conflicts.

## Scope

This change includes:

- Staged per-conflict choices and one batch Apply action.
- Safe global selection controls for choosing Local or Remote for all visible
  conflicts.
- A lightweight desktop snapshot that never performs a full resource scan.
- Persisted, on-demand resource preview summaries.
- Correctness-preserving incremental local and Git repository scans.
- Phase timing and measurable performance acceptance criteria.
- Structured Plugin failure recovery and an explicit retry action.
- Completion of the already implemented adaptive-window package and installed
  smoke validation.

This change does not:

- Add real-time filesystem monitoring.
- Use the Windows USN journal.
- Automatically terminate Copilot, VS Code, Claude, or Plugin processes.
- Silently ignore install failures.
- Weaken deletion detection, secret scanning, path ownership, or remote
  validation.
- Change the configured periodic synchronization interval.

## User experience

### Conflict selection

Choosing `Use local` or `Use remote` on a conflict changes only the selection
shown in the desktop UI. It does not call the backend and does not start a
synchronization.

The conflict panel provides:

- `Use local for all`
- `Use remote for all`
- `Apply selected`
- A selected count and total conflict count

`Apply selected` may submit a subset of the visible conflicts. Unselected
conflicts remain unresolved. The button is disabled when no conflict is
selected, while a submission is active, or while synchronization is already
applying a previously queued batch.

If submission fails, the selections remain in the UI so the user can correct
or retry them. A successful submission clears only the accepted selections and
shows that one synchronization was queued.

### Fast desktop actions

Pause, Resume, Sync now, Save settings, Approve, Retry, and Apply selected wait
only for their local state change to be persisted and, when applicable, for one
sync request to be queued. They do not wait for a complete resource scan or
synchronization before returning.

The main dashboard continues to show live scheduler progress. Routine snapshot
refreshes have a one-second target and do not execute agent inventory commands.

### Resource preview

The Settings panel opens immediately with the latest persisted preview summary.
The summary records when it was produced. If no preview exists yet, the panel
shows a loading state and requests one background refresh without blocking the
rest of the UI.

An explicit preview refresh starts or joins the one in-flight preview job.
Concurrent callers never launch duplicate scans.

### Plugin recovery

An install operation that fails because Windows reports `Access is denied` or
`os error 5` is marked as requiring manual retry. The UI explains that the
Plugin files are probably in use and instructs the user to close Copilot CLI,
VS Code, and related agent processes before selecting `Retry`.

Other synchronization work and other install operations continue. The failed
operation is no longer approved, so periodic synchronization does not execute
it again until the user explicitly retries it. The original command error
remains visible for diagnosis.

## Architecture

### Fast control plane

`desktop.Service.Snapshot()` becomes a lightweight control-plane read. It is
assembled from:

- The scheduler's in-memory state and next-run time.
- The latest in-memory or persisted compact cycle result.
- Configuration and enabled provider declarations.
- Pending conflict-resolution metadata.
- Conflict records.
- The pending install plan and its structured failures.
- The latest persisted resource preview summary.

It must not call `cli.RunStatus()`, `resourcecollect.Collect()`, Plugin
inventory adapters, Git fetch, or resource preview collection.

Scheduler state transitions publish this lightweight snapshot. The frontend's
generic action wrapper may refresh it after a mutation without turning a fast
button into a full scan.

The compact cycle result and preview summary are written atomically under
`~/.acsync`. They contain counts, timestamps, issue summaries, and per-resource
preview metadata, but not synchronized file contents or credentials.

### Background data plane

Full collection remains in the synchronization data plane. A preview-only job
uses the same indexed collector but runs independently, is cancellable, and is
coalesced so only one preview collection can exist at a time.

The synchronization engine records elapsed time for:

- Pull
- Local collection
- Remote indexing and validation
- Comparison
- Conflict resolution
- Resource application
- Install reconciliation
- Commit and push
- State and cache persistence

The phase timing is logged and included in the compact cycle result so slow
phases can be diagnosed without adding temporary instrumentation.

## Batch conflict-resolution data flow

The desktop API accepts an ordered batch containing a conflict ID, a Local,
Remote, or merged choice, and the conflict revision observed by the UI.

The API preflights the complete batch before persisting anything:

1. Reject duplicate or unsupported selections.
2. Confirm every conflict still exists.
3. Confirm the submitted revision matches the current conflict record and
   variants.
4. Load the selected bytes.
5. Apply the existing secret and resource-policy scanner to merged content and
   selected variants.

After successful preflight, the service atomically writes one local pending
resolution file and calls `Trigger()` once. It does not modify the managed Git
worktree before pull. This avoids creating unstaged repository changes that
would prevent `git pull --rebase`.

During synchronization, after pull and remote conflict mirroring, the engine
revalidates the pending revisions. It then applies the complete queued batch as
one logical transaction:

1. Rename affected local and repository conflict bundles to transaction
   tombstones.
2. Save the previous canonical file state and write each selected canonical
   value atomically.
3. If any write fails, restore canonical files and conflict bundles.
4. On success, remove the pending batch and delete tombstones.

Tombstones are excluded from conflict listing and mirroring. Startup recovery
either completes cleanup for a committed transaction or restores a transaction
that did not commit. A cleanup failure is reported and retried; it is not
silently treated as a new conflict.

The normal reconcile, commit, and push stages then distribute the chosen
values. A failed or stale batch transitions to a terminal failed record,
releases the active-batch slot, and leaves the original conflicts unresolved.
Its exact error remains visible, but periodic synchronization does not attempt
it again. The user can make new selections and submit a new batch.

## Incremental collection

### Local resources

Every synchronization still enumerates all in-scope paths. This preserves
deletion detection and applies directory exclusions before descending into
known dependency, cache, build, and runtime trees.

For each included file:

1. Apply size and type policy before reading.
2. Read the file once and compute SHA-256.
3. Reuse a projection and scan verdict only when the source hash, transformer
   version, filter policy, secret rules, resource specification, OS, and cache
   schema all match.
4. Store portable output in the content-addressed cache only after projection
   and secret scanning succeed.

The collector does not trust size and modification time as proof that content
is unchanged. A same-size change with a preserved timestamp is detected by its
content hash.

The content-addressed store replaces per-cycle staging of every safe file.
Only files needed for push or merge are read from the store during apply. Cache
files use owner-only permissions and contain only projected portable content
that already passed the scanner. Raw source bytes and blocked content are never
stored.

File work uses a small bounded worker pool. Results, issues, and actions are
sorted before persistence so output remains deterministic.

### Git repository resources

The Git client provides an index view containing path, stage, and blob ID.
Tracked files whose blob ID has already been validated reuse the cached
SHA-256, size, ownership result, and secret-scan result.

Files with a new blob ID are read and validated once. Untracked, staged, or
unstaged worktree changes are detected from Git status and read directly rather
than trusting the index entry.

Remote snapshot construction and remote resource validation become one pass.
They no longer hash and then reopen the same changed file in separate
operations.

The cache key includes repository identity, object format, resource-policy
fingerprint, and cache schema. It therefore cannot reuse an entry across an
incompatible repository or policy.

### Cache persistence and failure

Indexed metadata and safe portable objects live under `~/.acsync/cache` and are
never synchronized. Metadata updates use atomic replacement.

Missing or incompatible caches produce a normal full scan. A malformed or
unreadable cache produces a visible warning, is quarantined, and is rebuilt
from source data. A cache error must never turn an unvalidated resource into an
accepted resource.

Content objects no longer referenced by the current index are cleaned using
the existing archive-retention setting. Cleanup is bounded and occurs after
the sync result is safely persisted.

## Install-plan failures

Pending install operations gain structured failure fields:

- Failure code
- Original error
- Recovery message
- Failed-at timestamp
- Manual-retry requirement

The executor classifies only known signatures. Windows access-denied output is
mapped to a locked-resource failure. Unknown failures retain a generic code and
the complete sanitized command error.

After any command failure, the operation remains pending and unapproved.
Execution continues with independent approved operations. Successfully applied
operations retain their applied markers and are not repeated.

`Retry` explicitly approves the current pending plan revision and queues one
synchronization. A stale plan revision is rejected so a retry cannot approve a
different command than the one displayed to the user.

## Error handling and concurrency

- Snapshot reads use immutable copies of cached summaries and do not wait for a
  background scan.
- Sync and preview collection share cache coordination but never write the same
  metadata file concurrently.
- Cancellation stops new file work and prevents publishing partial indexes.
- A new preview request joins the active preview instead of spawning another.
- A sync request coalesces with an already queued or running sync according to
  the existing scheduler behavior.
- Batch conflict resolution has one queued or applying revision at a time. A
  second batch is rejected until the first commits or fails. A failure restores
  the conflicts, records the diagnostic, and releases the active-batch slot.
- Errors identify the operation, conflict, path, or cache involved; no broad
  catch converts failures into success.

## Testing

### Desktop and frontend

- Lightweight snapshot does not call status, collection, Git, or inventory
  adapters.
- Scheduler transitions publish lightweight snapshots.
- Settings opens from persisted preview data without waiting for collection.
- Concurrent preview requests execute one collector call.
- Per-row selection performs no backend call.
- Select-all Local and Remote update every visible row.
- Apply submits exactly the selected rows in one request.
- Backend failure preserves frontend selections.
- Successful Apply and Retry each queue one synchronization.

### Conflict transaction

- Complete preflight failure changes no files and writes no pending batch.
- A stale revision is rejected.
- Successful partial and complete batches commit all chosen values.
- An injected canonical write failure restores earlier files and bundles.
- Startup recovery handles pre-commit and committed tombstones.
- Merged content is scanned before it enters the pending batch.
- No synchronization is triggered for a rejected batch.

### Incremental index

- First collection creates an index and safe content objects.
- An unchanged collection reuses scan verdicts and performs no staging writes.
- Content changes with unchanged size and timestamp are detected.
- Deletions are detected.
- Transformer, filter, secret-rule, resource-spec, OS, and schema changes
  invalidate affected entries.
- Blocked and raw source content never enters the content store.
- Corrupt cache metadata produces a warning and a safe rebuild.
- Git blob reuse avoids reopening unchanged files.
- New blobs, staged changes, unstaged changes, and untracked files are read and
  validated.
- Bounded concurrency produces deterministic snapshots and issues.

### Install operations

- Access-denied output receives the locked-resource code and recovery message.
- The failed operation becomes unapproved and is not retried on a periodic
  synchronization.
- Independent operations continue.
- Explicit Retry approves only the displayed plan revision and queues one sync.
- Unknown errors remain pending with their original diagnostic.

## Performance and release acceptance

Validation is sequential because the frontend build replaces embedded
`frontend/dist`:

1. Frontend production build.
2. Focused and full Go tests.
3. `go vet ./...`.
4. Windows package build.
5. Clean install and installed smoke tests.

Performance is measured on the current real profile after one warm-up sync:

- Lightweight `Snapshot()` and routine UI actions complete within one second.
- Opening Settings does not block on collection.
- One no-change synchronization completes in less than 30 seconds.
- Logs contain timings for every defined phase.
- Applying all current conflict selections queues exactly one synchronization.
- An access-denied Plugin operation does not prevent unrelated resources from
  completing and does not run again without Retry.

The installed smoke test also verifies profile preservation, single-instance
handoff, tray behavior, and the previously implemented adaptive startup window.
On a sufficiently large work area, the installed main window must open centered
at 1200 by 850 logical pixels.

If the measured no-change synchronization remains above 30 seconds, the work
is not complete. The recorded phase timings determine the next targeted
optimization.

## Alternatives considered

### UI-only optimization

Removing full scans from `Snapshot()` would make buttons responsive with the
smallest change, but the three-minute background synchronization would remain.
It does not meet the approved performance goal.

### Metadata-only local cache

Trusting file size and modification time would be faster but could miss edits
that preserve timestamps. The selected design always hashes included local
content and uses metadata only for diagnostics and ordering.

### Windows USN journal

The USN journal could approach a ten-second sync by avoiding enumeration and
hashing, but adds platform-specific permissions, journal-wrap recovery, volume
identity, and cross-platform inconsistency. It is unnecessary for the approved
30-second target.
