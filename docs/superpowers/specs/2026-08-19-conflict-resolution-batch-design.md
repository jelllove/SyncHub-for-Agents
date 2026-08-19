# Conflict Resolution Batch Design

## Goal

Replace the desktop conflict-by-conflict apply flow with one atomic,
revision-bound batch flow. The UI must require a choice for every currently
visible conflict before submission. A batch is accepted only when every
selection is valid against the conflict revision visible to the user.

## Architecture

`internal/conflict` owns conflict revisions, batch validation, durable batch
metadata, and process-wide locks keyed by the conflict root. `QueueBatch`
performs a complete read-only preflight, then atomically writes `pending.json`;
it never partially persists selections. `VisibleConflicts` returns a stable
record plus revision and does not block behind an applying transaction.

The batch supports `local`, `remote`, and `merged` choices. Merged content is
required for `merged` and is scanned with the same conflict scanner before the
batch is persisted. Selection IDs must be unique, complete, and match the
visible conflict set. Revisions hash the canonical record and the base/local/
remote bundle bytes, so edits invalidate stale UI selections.

Batch metadata uses owner-only atomic JSON files beside the local conflict
bundles:

- `pending.json`: validated work waiting to apply
- `applying-view.json`: immutable conflict view published while applying
- `failed.json`: durable failure and selections for retry

The transaction lock protects bundle and repository mutations. A metadata
mutex protects short metadata reads/writes. When both are needed, transaction
locking is acquired first.

## Desktop integration

`Snapshot` will expose conflict summaries containing `id`, `revision`,
resource key, path, and creation time, plus an optional resolution status with
batch ID, state, selected count, and error. Snapshot projection will use
`VisibleConflicts` and propagate errors instead of silently omitting conflicts.
The existing singular resolve method remains until the later conflict API/UI
tasks migrate Wails bindings and React controls to one batch submission.

Because this task requires all conflicts to be selected, an empty or partial
selection list is rejected. The UI migration will provide select-all and
per-conflict choices, then submit one batch.

## Error handling and recovery

Invalid IDs, duplicate IDs, unsupported choices, missing merged content, stale
revisions, scanner failures, and an already-active batch are explicit errors.
No pending batch is written when any preflight step fails. Applying and
recovery operations use the transaction lock and preserve enough metadata for
the next startup to expose a failed batch rather than losing user choices.

## Testing

Tests will cover:

- deterministic revisions and batch IDs;
- complete-selection enforcement and duplicate/missing/unsupported inputs;
- stale revisions and merged-content scanning;
- all-or-nothing preflight persistence;
- concurrent stores where exactly one queue operation succeeds;
- stable visible conflict projections and resolution status;
- malformed or missing metadata behavior;
- desktop Snapshot propagation of revisions and batch state.

