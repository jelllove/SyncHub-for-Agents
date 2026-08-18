# Indexed Synchronization Performance Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Preserve complete deletion, content-change, ownership, and secret-scan correctness while reducing a warmed no-change synchronization on the real profile from roughly three minutes to less than 30 seconds.

**Architecture:** Replace per-cycle temporary staging with an owner-only content-addressed cache of portable bytes that have passed projection and secret scanning. Continue enumerating and hashing every included local source file, but reuse safe projection/scan results by source and policy fingerprint. Build remote snapshots and validation in one Git-aware pass that reuses clean tracked blob verdicts, while dirty and internal metadata paths remain fail-closed.

**Tech Stack:** Go 1.26.5, Wails v3.0.0-beta.8, Git CLI, SHA-256 content addressing, JSON indexes, existing React desktop

**Design:** `docs/superpowers/specs/2026-08-18-batch-conflicts-and-sync-performance-design.md`

**Prerequisite:** Complete `docs/superpowers/plans/2026-08-18-fast-desktop-batch-recovery.md` first. That plan removes scans from routine desktop snapshots and supplies persisted preview/cycle summaries.

---

## File structure

### New files

- `internal/resourcecache/model.go` — versioned local and Git index schemas.
- `internal/resourcecache/store.go` — atomic index storage, safe CAS objects, quarantine, and retention cleanup.
- `internal/resourcecache/store_test.go` — integrity, corruption, and cleanup tests.
- `internal/resourcecache/lock.go` — cancellable index-transaction lease abstraction.
- `internal/resourcecache/lock_windows.go` — Windows `LockFileEx` implementation.
- `internal/resourcecache/lock_unix.go` — Unix `flock` implementation.
- `internal/resource/fingerprint.go` — deterministic Spec and FilterPolicy fingerprints.
- `internal/resource/fingerprint_test.go` — canonical fingerprint tests.
- `internal/secret/fingerprint.go` — scanner-rule fingerprint and schema version.
- `internal/secret/fingerprint_test.go` — normalized rule fingerprint tests.
- `internal/portableconfig/fingerprint.go` — transformer policy fingerprints.
- `internal/portableconfig/fingerprint_test.go` — policy mutation invalidation tests.
- `internal/syncengine/repository_scan.go` — combined Git snapshot, ownership, and validation.
- `internal/syncengine/repository_scan_test.go` — blob reuse and dirty-path validation.
- `internal/syncengine/timing.go` — stable phase names and recorder.
- `internal/syncengine/timing_test.go` — ordered success/failure timing tests.
- `internal/cli/performance_test.go` — opt-in real-profile no-change sync acceptance test.

### Modified files

- `internal/resourcecollect/collector.go` — indexed collection, bounded workers, and cache-backed artifacts.
- `internal/resourcecollect/collector_test.go` — warm reuse, invalidation, deletion, and deterministic concurrency.
- `internal/gitclient/gitclient.go` — NUL-safe index/status/blob/object-format APIs.
- `internal/gitclient/gitclient_test.go` — clean, dirty, untracked, rename, symlink, and SHA format tests.
- `internal/syncengine/resources.go` — wrappers delegate to the combined scanner where appropriate.
- `internal/syncengine/resource_apply.go` — reads immutable cache objects instead of temporary stages.
- `internal/syncengine/engine.go` — cache wiring, combined remote scan, timings, and indexed final persistence.
- `internal/syncengine/engine_test.go` — two-cycle reuse and timings.
- `internal/state/base.go` — skip unchanged base-object rereads.
- `internal/state/base_test.go` — read-counter regression tests.
- `internal/cli/sync.go` — construct one cache and pass retention/timing observers.
- `internal/cli/status.go` — use the same indexed local and remote paths.
- `internal/cli/status_test.go`, `internal/cli/sync_test.go` — shared-cache regression tests.
- `internal/desktop/resources.go` — on-demand preview uses the indexed collector.
- `internal/desktop/resource_service_test.go` — warm preview reuse.
- `internal/daemon/daemon.go` — log and persist phase timings.
- `internal/daemon/daemon_test.go` — success and failure timing logs.
- `scripts/smoke/windows.ps1` — modified only if installed validation exposes a directly related defect.

### Cache layout

```text
~\.acsync\cache\
  objects\{portable-sha256}
  refs\{hash-prefix}\{portable-sha256}\{namespace}.{epoch}
  indexes\local\{local-or-custom-namespace}.json
  indexes\git\{sha256(repository-id,object-format,namespace)}.json
  evicting\{namespace}.{epoch}.json
  orphaned\{namespace}.json
  reference-journal.json
  cleanup-state.json
  index.lock
  quarantine/{timestamp}-{index-name}
```

Objects are projected portable bytes that passed all filters and secret scans.
Raw source bytes, blocked bytes, malformed documents, credentials, and command
output are never placed in `objects`.

## Task 1: Add deterministic policy fingerprints

**Files:**
- Create: `internal/resource/fingerprint.go`
- Create: `internal/resource/fingerprint_test.go`
- Create: `internal/secret/fingerprint.go`
- Create: `internal/secret/fingerprint_test.go`
- Create: `internal/portableconfig/fingerprint.go`
- Create: `internal/portableconfig/fingerprint_test.go`

- [ ] **Step 1: Write fingerprint invariance and invalidation tests**

Add tests with these assertions:

```go
func TestSpecFingerprintIgnoresSliceAndMapConstructionOrder(t *testing.T) {
	left := resource.Spec{
		Key: "claude/settings", Provider: "claude", ID: "settings",
		Include: []string{"*.json", "*.jsonl"},
		Exclude: []string{"cache/**", "logs/**"},
		Targets: []string{`C:\one`, `C:\two`},
	}
	right := left
	right.Include = []string{"*.jsonl", "*.json"}
	right.Exclude = []string{"logs/**", "cache/**"}
	right.Targets = []string{`C:\two`, `C:\one`}
	if FingerprintSpec(left) != FingerprintSpec(right) {
		t.Fatal("equivalent Specs produced different fingerprints")
	}
	right.Transformer = "claude-settings"
	if FingerprintSpec(left) == FingerprintSpec(right) {
		t.Fatal("transformer change did not invalidate fingerprint")
	}
}

func TestScannerFingerprintIncludesSchemaAndNormalizedRules(t *testing.T) {
	left := NewScanner([]string{"logs/**"}, []string{"Token", "password"})
	right := NewScanner([]string{"logs/**"}, []string{"password", "token"})
	if left.Fingerprint() != right.Fingerprint() {
		t.Fatal("equivalent scanner rules produced different fingerprints")
	}
}

func TestRegistryFingerprintChangesWithPolicy(t *testing.T) {
	registry := NewRegistry()
	registry.Register("demo", Policy{Portable: []string{"theme"}})
	first, err := registry.Fingerprint("demo")
	if err != nil {
		t.Fatal(err)
	}
	registry.Register("demo", Policy{Portable: []string{"theme", "language"}})
	second, err := registry.Fingerprint("demo")
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("policy change did not invalidate fingerprint")
	}
}
```

Run:

```powershell
go test ./internal/resource ./internal/secret ./internal/portableconfig -run Fingerprint -count=1
```

Expected: FAIL because the fingerprint APIs do not exist.

- [ ] **Step 2: Implement canonical SHA-256 fingerprints**

Expose:

```go
const ScannerSchemaVersion = 1
const TransformerSchemaVersion = 1

func FingerprintSpec(spec Spec) string
func (p FilterPolicy) Fingerprint() string
func (s *Scanner) Fingerprint() string
func (r *Registry) Fingerprint(transformer string) (string, error)
```

Build small canonical structs and JSON-marshal them before SHA-256 hashing.
Sort copies of order-insensitive slices (`Include`, `Exclude`, `Targets`,
`KeyPatterns`, generated globs, extensions, and policy field paths). Do not
mutate caller-owned slices. Include every `resource.Spec` field that can affect
repo path, projection, scanning, targets, or install behavior.

`Registry.Fingerprint("")` returns the hash for an identity projection.
`generic-safe` includes `TransformerSchemaVersion` and the sorted generic
secret patterns. Unknown transformers return the same error class as `Project`.

- [ ] **Step 3: Run all affected package tests**

```powershell
go test ./internal/resource ./internal/secret ./internal/portableconfig -count=1
```

Expected: PASS.

- [ ] **Step 4: Commit**

```powershell
git add internal\resource internal\secret internal\portableconfig
git commit -m "feat: fingerprint portable resource policies" -m "Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
```

## Task 2: Implement the safe content-addressed cache

**Files:**
- Create: `internal/resourcecache/model.go`
- Create: `internal/resourcecache/store.go`
- Create: `internal/resourcecache/store_test.go`
- Create: `internal/resourcecache/lock.go`
- Create: `internal/resourcecache/lock_windows.go`
- Create: `internal/resourcecache/lock_unix.go`
- Create: `internal/resourcecache/lock_test.go`

- [ ] **Step 1: Write object and index persistence tests**

Define tests against this public contract:

```go
func TestStoreWritesOwnerOnlyDeduplicatedObjects(t *testing.T) {
	store := New(t.TempDir())
	first, err := store.PutObject([]byte("portable"))
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.PutObject([]byte("portable"))
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("object IDs differ: %q %q", first, second)
	}
	data, err := store.ReadObject(first)
	if err != nil || string(data) != "portable" {
		t.Fatalf("ReadObject() = %q, %v", data, err)
	}
	info, err := os.Stat(store.ObjectPath(first))
	if err != nil || info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("object mode = %v, %v", info.Mode().Perm(), err)
	}
}

func TestStoreDoesNotPublishPartialIndex(t *testing.T) {
	store := New(t.TempDir())
	now := time.Date(2026, 8, 18, 0, 0, 0, 0, time.UTC)
	store.now = func() time.Time { return now }
	namespace, err := LocalNamespace("managed", "windows", `C:\Users\Test`)
	if err != nil {
		t.Fatal(err)
	}
	original := LocalIndex{
		Schema: CacheSchemaVersion,
		LastUsed: now,
		Entries: map[string]LocalEntry{"one": {SourceHash: "source"}},
	}
	if _, err := store.SaveLocalContext(context.Background(), namespace, original); err != nil {
		t.Fatal(err)
	}
	persisted, warnings, err := store.LoadLocalContext(context.Background(), namespace)
	if err != nil || len(warnings) != 0 {
		t.Fatalf("initial LoadLocalContext() = %#v, %v", warnings, err)
	}
	originalRename := store.rename
	store.rename = func(oldName, newName string) error {
		if strings.HasSuffix(newName, filepath.Base(store.localIndexPath(namespace))) {
			return errors.New("rename failed")
		}
		return originalRename(oldName, newName)
	}
	if _, err := store.SaveLocalContext(context.Background(), namespace, LocalIndex{Schema: CacheSchemaVersion}); err == nil {
		t.Fatal("SaveLocalContext() error = nil")
	}
	store.rename = originalRename
	got, warnings, err := store.LoadLocalContext(context.Background(), namespace)
	if err != nil || len(warnings) != 0 || !reflect.DeepEqual(got, persisted) {
		t.Fatalf("LoadLocalContext() = %#v, %#v, %v", got, warnings, err)
	}
}
```

Also test missing objects, hash-integrity mismatch, malformed index quarantine,
schema mismatch returning an empty index plus warning, Git-index round-trip,
and retention deleting only unreferenced objects older than the cutoff. Add a
clock-controlled test that creates two custom-preview namespaces, touches only
the second after the retention cutoff, runs bounded cleanup to completion, and
proves the first namespace index and its now-unreferenced object are removed
while the second survives.

Add a corrupt-index reference test: create an active local index plus marker,
corrupt and quarantine its JSON, rebuild the same namespace with a new epoch,
then run bounded cleanup to completion. The old marker/object must be removed
while the rebuilt epoch marker/object survives. Repeat with a parseable but
schema-incompatible index to prove the same orphan-tombstone behavior.

Add concurrency tests with separate `Store` instances and a helper subprocess:

- two writers load the same empty local namespace, add different keys, and
  release their saves through one barrier; both keys survive;
- two writers change the same key from one base; the first committed value is
  retained, the second reports one `cache-index-concurrent-update` issue rather
  than overwriting it;
- a process blocked on `index.lock` returns `context.Canceled` without
  publishing a partial index;
- cleanup with `MaxEntries: 2` returns `Incomplete=true`, inspects no more than
  two index entries/object paths, persists its phase and cursor, and later calls
  resume to completion;
- a barrier race between `PutObjectContext` reusing an old hash and cleanup
  either rewrites after cleanup deleted it or touches it first so cleanup
  retains it until the new reference marker is published;
- after thousands of live objects and partial cleanup,
  `cleanup-state.json` remains below 4 KiB because it contains cursors, not a
  serialized live-hash set.

Run:

```powershell
go test ./internal/resourcecache -count=1
```

Expected: FAIL because the package does not exist.

- [ ] **Step 2: Define versioned index models**

Create:

```go
const CacheSchemaVersion = 1

type Verdict string

const (
	VerdictSafe    Verdict = "safe"
	VerdictBlocked Verdict = "blocked"
	VerdictSkipped Verdict = "skipped"
)

type LocalEntry struct {
	SourceHash  string         `json:"sourceHash"`
	PolicyHash  string         `json:"policyHash"`
	ObjectHash  string         `json:"objectHash,omitempty"`
	PortableSize int64         `json:"portableSize,omitempty"`
	Verdict     Verdict        `json:"verdict"`
	Issue       resource.Issue `json:"issue,omitempty"`
}

type LocalIndex struct {
	Schema   int                   `json:"schema"`
	Epoch    string                `json:"epoch"`
	LastUsed time.Time            `json:"lastUsed"`
	Entries  map[string]LocalEntry `json:"entries"`
}

type CleanupBudget struct {
	MaxEntries  int
	MaxDuration time.Duration
}

type CleanupResult struct {
	Removed    []string
	Inspected  int
	Incomplete bool
}

type GitEntry struct {
	BlobID     string           `json:"blobId"`
	Mode       string           `json:"mode"`
	PolicyHash string           `json:"policyHash"`
	Meta       state.FileMeta   `json:"meta"`
	Owned      bool             `json:"owned"`
	Valid      bool             `json:"valid"`
	Issues     []resource.Issue `json:"issues,omitempty"`
}

type GitIndex struct {
	Schema       int                 `json:"schema"`
	RepositoryID string              `json:"repositoryId"`
	ObjectFormat string              `json:"objectFormat"`
	Namespace    string              `json:"namespace"`
	LastUsed     time.Time           `json:"lastUsed"`
	Entries      map[string]GitEntry `json:"entries"`
}
```

Use a zero-value `resource.Issue` for safe entries. Blocked/skipped entries
store only hashes and issue metadata; they never have `ObjectHash`.

- [ ] **Step 3: Implement safe persistence**

Expose:

```go
func New(home string) *Store
func LocalNamespace(mode, goos, userHome string) (string, error)
func CustomNamespace(mode, goos, userHome, specFingerprint string) (string, error)
func (s *Store) LoadLocalContext(ctx context.Context, namespace string) (LocalIndex, []resource.Issue, error)
func (s *Store) SaveLocalContext(ctx context.Context, namespace string, index LocalIndex) ([]resource.Issue, error)
func (s *Store) LoadGitContext(ctx context.Context, repositoryID, objectFormat, namespace string) (GitIndex, []resource.Issue, error)
func (s *Store) SaveGitContext(ctx context.Context, index GitIndex) ([]resource.Issue, error)
func (s *Store) PutObject([]byte) (string, error)
func (s *Store) PutObjectContext(ctx context.Context, data []byte) (string, error)
func (s *Store) ReadObject(hash string) ([]byte, error)
func (s *Store) LookupObject(hash string, expectedSize int64) (string, bool, error)
func (s *Store) QuarantineObjectContext(ctx context.Context, hash string) error
func (s *Store) ObjectPath(hash string) string
func (s *Store) CleanupUnreferencedContext(ctx context.Context, before time.Time, budget CleanupBudget) (CleanupResult, error)
```

`New(home)` roots the store at the AgentConfigSync home's `cache` directory. Directories are `0700`; objects
and indexes are `0600`. `PutObject` uses SHA-256, verifies an existing object
before reuse, and writes through a synced sibling temporary file. If another
process wins publication between the existence check and rename, verify that
winner's bytes/hash and treat it as success; never overwrite or trust it
without verification. Use a package-private `now func() time.Time` initialized
to `time.Now` for deterministic retention tests.

`PutObject` is the `context.Background()` compatibility wrapper over
`PutObjectContext`. `PutObjectContext` takes the same cross-process lease as
cleanup; when reusing an existing hash it verifies the bytes and touches the
object before returning. Therefore a sweep that wins first causes a normal
rewrite, while PutObject winning first keeps the object newer than the cutoff
until SaveLocal publishes its marker.

`LookupObject` validates hash syntax, uses `Lstat`, rejects links/non-regular
files, checks expected size, and returns `ObjectPath` without opening or hashing
content. A warm local entry is already protected by its active namespace/epoch
reference marker, so collection does not need a second content read.
`ReadObject` remains the full integrity-test/diagnostic helper.
`QuarantineObjectContext` takes the lease, verifies the name, and atomically
moves a corrupt object to quarantine; active index entries then become safe
cache misses on the retry.

Malformed indexes are renamed to
`quarantine\{UTC timestamp}-{index name}` and return an empty index plus one
`cache-index-corrupt` warning. Permission/read/rename errors are returned.
Schema/repository/object-format/namespace mismatch returns an empty index plus a
`cache-index-incompatible` warning without trusting any entry.

Whenever an active local index is quarantined or rejected before its epoch and
entries can be trusted, atomically create
`orphaned\<namespace>.json`. This records that older reference markers for that
namespace require bounded repair; never silently discard the only cleanup
signal.

Every index mutation opens `index.lock` and acquires an OS advisory exclusive
lease: `LockFileEx` on Windows and `flock` on Unix. Acquisition uses
non-blocking retries and checks `ctx`; there is no process-local-only or
last-writer-wins fallback. `LoadLocalContext`/`LoadGitContext` always take the
lease, perform any quarantine while protected, and remember the loaded baseline
in mutex-protected maps keyed by full context: local baselines by namespace and
Git baselines by `{repositoryID, objectFormat, namespace}`. One context can
never replace another's baseline. A save acquires the lease, reloads the latest disk index,
and three-way merges the baseline-to-requested delta:

- disjoint adds/updates/deletes are applied to the fresh disk index;
- if the same key changed since the caller's baseline, retain the fresh disk
  value and return a `cache-index-concurrent-update` warning;
- atomically publish the merged index and update only that context's baseline
  after rename succeeds.

This preserves explicit deletion without allowing another process's unrelated
entry to be dropped. Tests use distinct `Store` objects so accidental reliance
on a receiver mutex cannot pass.

Safe local-index entries own conservative reference markers at
`refs\<hash-prefix>\<object-hash>\<namespace>.<epoch>`. When merged entries
change, assign a new random 128-bit epoch and, under the lease:

1. atomically write `reference-journal.json` with old/new epochs and unique
   object hashes;
2. create all new-epoch markers;
3. atomically replace the local index;
4. remove old-epoch markers;
5. remove the journal.

On every leased operation, recover a leftover journal first. If the active
index has the new epoch, finish old-marker removal; otherwise remove new
markers and retain the old index/markers. Marker updates are therefore
crash-recoverable and never leave a live index without references.

`LocalNamespace` and `CustomNamespace` hash canonical JSON containing a
required mode discriminator, normalized lowercase GOOS, and normalized absolute
user home; custom mode also contains the Spec fingerprint. On Windows,
normalization cleans separators and case-folds the volume/path. A managed
namespace is `local-` plus 64 lowercase hex characters; a custom namespace is
`custom-` plus 64 lowercase hex characters. Reject all other namespaces.
Normal sync, CLI status, and the full-provider desktop preview deliberately use
the same `managed` mode and therefore share one index only when GOOS and user
home also match.

Map a local namespace exactly to
`indexes\local\<namespace>.json`. Map a Git context to
`indexes\git\<sha256(canonical JSON of repositoryID, objectFormat,
namespace)>.json`; validate the three unhashed fields stored inside before
trusting it. Quarantine preserves the relative index kind plus timestamp, so
two contexts cannot collide.

Every successful local/Git load or save refreshes the index header's
`LastUsed`; throttle timestamp-only atomic rewrites to at most once per 24
hours. Existing indexes without a usable timestamp fall back to their validated
file modification time. Three-way entry deltas ignore `LastUsed`, and a
timestamp-only rewrite preserves the local epoch/reference markers.

`CleanupUnreferencedContext` uses the same lease and never recursively deletes
the cache root. Defaults are 500 inspected entries and 100 ms when a budget
field is zero. Check both limits and `ctx` before every index entry and object
path.

Persist a constant-size `cleanup-state.json` containing phase
(`prune-namespaces`, `prune-orphan-refs`, or `sweep`), a lexicographic
`namespaceFileCursor`, current evicting namespace/epoch and entry cursor, and
current orphan namespace/reference-path cursor plus object shard/path cursor.
Enumerate `indexes\local` and `indexes\git` in
lexicographic pages strictly after `namespaceFileCursor`, updating it after
every inspected file and clearing it only when both directories finish.
Cleanup uses private no-touch/no-baseline index readers, so inspection never
refreshes `LastUsed` or changes a caller's three-way baseline.

The prune phase removes local and Git index files whose header/file
`LastUsed` is older than `before`. For a local namespace, atomically rename the
active index to `evicting\<namespace>.<epoch>.json` first, then remove only that
epoch's reference markers in bounded entry pages before deleting the evicting
file. A concurrent later use creates a new active index/epoch and cannot lose
its markers. Git indexes have no CAS references and can be deleted directly.
This is cache eviction only; a later sync/preview safely rebuilds it.

The orphan-reference phase enumerates `orphaned` tombstones and reference
markers in bounded pages. Parse marker names as `{namespace}.{epoch}`. For the
orphan namespace, retain only a marker whose epoch equals the current valid
active index epoch; delete all older/unknown epochs. Remove the orphan
tombstone only after the complete reference tree has been examined. A rebuilt
same namespace is therefore protected while markers orphaned by unreadable JSON
are eventually reclaimed.

After namespace and orphan pruning complete, sweep validated 64-character object names directly
under `objects`. Retain an object when `ReadDir(1)` finds any reference marker
or when its touched modification time is newer than `before`; otherwise delete
it. Write the next compact cursor atomically before returning
`Incomplete=true`, and remove state only after a complete sweep. The phase
itself is paged by the same hard budget, so repeated custom-resource
fingerprints cannot grow one cycle's work without bound.

- [ ] **Step 4: Run package tests**

```powershell
go test ./internal/resourcecache -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```powershell
git add internal\resourcecache
git commit -m "feat: store scanned portable content safely" -m "Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
```

## Task 3: Add NUL-safe Git index and worktree primitives

**Files:**
- Modify: `internal/gitclient/gitclient.go`
- Modify: `internal/gitclient/gitclient_test.go`

- [ ] **Step 1: Write real-Git behavior tests**

Using `setupBareRemote`, create tracked, staged, unstaged, untracked, renamed,
and symlink paths. Assert:

```go
func TestIndexEntriesAndDirtyPaths(t *testing.T) {
	work := cloneGitFixture(t)
	client := &Client{Dir: work}
	writeAndCommit(t, work, "clean.txt", "clean")
	writeAndCommit(t, work, "unstaged.txt", "before")
	writeAndCommit(t, work, "staged.txt", "before")
	if err := os.WriteFile(filepath.Join(work, "unstaged.txt"), []byte("after"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(work, "staged.txt"), []byte("after"), 0o644); err != nil {
		t.Fatal(err)
	}
	mustGit(t, work, "add", "staged.txt")
	if err := os.WriteFile(filepath.Join(work, "untracked.txt"), []byte("new"), 0o644); err != nil {
		t.Fatal(err)
	}

	entries, err := client.IndexEntries()
	if err != nil {
		t.Fatal(err)
	}
	assertStageZeroBlob(t, entries, "clean.txt")
	dirty, err := client.DirtyPaths()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"unstaged.txt", "staged.txt", "untracked.txt"} {
		if _, ok := dirty[name]; !ok {
			t.Fatalf("%s not dirty: %#v", name, dirty)
		}
	}
}
```

Add paths containing spaces and Unicode, a rename whose old and new names are
both marked dirty, an unmerged fixture if supported, `ObjectFormat()` returning
`sha1` or `sha256`, and `ReadBlob` returning exact bytes.

Run:

```powershell
go test ./internal/gitclient -run 'TestIndexEntries|TestDirtyPaths|TestReadBlob|TestObjectFormat' -count=1
```

Expected: FAIL.

- [ ] **Step 2: Add typed Git APIs**

Define:

```go
type IndexEntry struct {
	Mode  string
	OID   string
	Stage int
	Path  string
}

func (c *Client) IndexEntries() ([]IndexEntry, error)
func (c *Client) DirtyPaths() (map[string]struct{}, error)
func (c *Client) ReadBlob(oid string) ([]byte, error)
func (c *Client) ObjectFormat() (string, error)
func (c *Client) RepositoryIdentity() (string, error)
```

Commands:

```text
git ls-files --stage -z
git status --porcelain=v1 -z --untracked-files=all
git cat-file blob {oid}
git rev-parse --show-object-format
```

Parse byte slices split on NUL; never parse paths by whitespace. Validate modes,
OID length against the object format, stage as `0..3`, and forward-slash paths.
Porcelain rename/copy records consume the second NUL path and mark both old and
new paths dirty.

`RepositoryIdentity` hashes canonical JSON containing the absolute Git common
directory and `origin` remote URL. It must not include OAuth credentials or
environment values.

- [ ] **Step 3: Run Git client tests**

```powershell
go test ./internal/gitclient -count=1
```

Expected: PASS.

- [ ] **Step 4: Commit**

```powershell
git add internal\gitclient
git commit -m "feat: inspect git blobs without reopening files" -m "Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
```

## Task 4: Replace temporary staging with indexed local collection

**Files:**
- Modify: `internal/resourcecollect/collector.go`
- Modify: `internal/resourcecollect/collector_test.go`
- Modify: `internal/syncengine/resource_apply.go`
- Modify: `internal/syncengine/resource_apply_test.go`
- Modify: `internal/syncengine/engine.go`
- Modify: `internal/syncengine/install_test.go`

- [ ] **Step 1: Write warm-cache and correctness tests**

Add an instrumented projector and cache store. Cover:

```go
func TestCollectorReusesSafePortableObjectAfterHashingSource(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "settings.json")
	writeCollectorFile(t, source, []byte(`{"theme":"dark","local":"remove"}`))
	cache := resourcecache.New(t.TempDir())
	projector := &countingProjector{output: []byte(`{"theme":"dark"}`)}
	options := indexedOptions(t, cache, projector)

	first, err := New(options).CollectContext(context.Background(), []resource.Spec{structuredSpec(root)})
	if err != nil {
		t.Fatal(err)
	}
	first.Close()
	second, err := New(options).CollectContext(context.Background(), []resource.Spec{structuredSpec(root)})
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()

	if projector.Calls() != 1 {
		t.Fatalf("projection calls = %d", projector.Calls())
	}
	artifact := onlyArtifact(t, second)
	if artifact.ContentPath == "" || !strings.Contains(artifact.ContentPath, "cache") {
		t.Fatalf("artifact = %#v", artifact)
	}
}
```

Add tests proving:

- Same size, same modification time, changed bytes cause a second projection.
- Deleted paths disappear from the new index and snapshot.
- Spec, filter, scanner, transformer, GOOS, and user-home changes invalidate.
- Blocked and malformed bytes never create objects.
- Inventory map keys are processed deterministically.
- `Result.Close()` no longer removes persistent objects.
- Source mutation after collection does not alter artifact bytes.
- Cancellation while hashing/projecting returns `context.Canceled`, publishes no
  local index, and leaves the previous index intact.

Run:

```powershell
go test ./internal/resourcecollect ./internal/syncengine -run 'Collector|PushArtifact' -count=1
```

Expected: FAIL.

- [ ] **Step 2: Define cache-aware collector options and artifacts**

Change:

```go
type Options struct {
	Cache         *resourcecache.Store
	CacheNamespace string
	StageParent   string
	GOOS          string
	UserHome      string
	Projector     Projector
	Inventory     InventoryProvider
	Filter        resource.FilterPolicy
	ApprovedLinks ApprovedLinkStore
	Workers       int
}

type Artifact struct {
	RepoRel     string
	ResourceKey string
	Relative    string
	ContentPath string
	Targets     []string
	Hash        string
	Size        int64
	ModTime     int64
}
```

Replace `StageRoot` with a private ephemeral-cache root. When `Cache` is nil,
create a temporary cache under `StageParent` and remove it from
`Result.Close()`; this keeps existing callers and commits buildable while they
are migrated. Persistent production callers pass `Cache`, and their
`Result.Close()` is a no-op. Keep `StageParent` only for this compatibility
fallback; no production caller uses it after Task 8. The fallback derives an
`ephemeral` local namespace from GOOS and user home rather than bypassing
namespace validation.

`CacheNamespace` is required with a persistent cache. Obtain it only through
`resourcecache.LocalNamespace("managed", GOOS, UserHome)` or
`resourcecache.CustomNamespace("custom-preview", GOOS, UserHome,
FingerprintSpec(spec))`; callers must not build cache keys manually. This puts
mode, GOOS, and normalized user home in the namespace rather than relying only
on a policy hash.

Extend the collector's projector contract:

```go
type Projector interface {
	Project(transformer, rel, goos, home string, data []byte) ([]byte, error)
	Fingerprint(transformer string) (string, error)
}
```

Update the existing fake projectors in `collector_test.go` to return explicit
fingerprints. This prevents reuse when an implementation cannot identify its
projection policy.

- [ ] **Step 3: Implement indexed file processing**

Continue serial directory/link traversal and directory pruning. Build sorted
file jobs containing the resolved spec, physical path, relative path, file
metadata, and target list.

At collection start, call `LoadLocalContext(ctx, CacheNamespace)` and append its
compatibility/corruption warnings to the result before processing jobs. That
loaded index is both the lookup source and the baseline for final three-way
publication.

For each file job:

1. Apply existing include/exclude and pre-read size rules.
2. Read source bytes once.
3. Compute `sourceHash`.
4. Compute `policyHash` from Spec, FilterPolicy, scanner, transformer, GOOS, and
   normalized user home; use the full cache namespace plus repo-relative path
   as the local index key.
5. If a local entry matches both hashes:
   - Safe: call metadata-only `LookupObject` with `PortableSize`; on success
     return an artifact referencing that path without reading object bytes.
   - Blocked/skipped: reproduce its issue without an object read.
6. On a miss, project, size-check, and scan exactly as today.
7. Safe: `PutObjectContext` and record a safe entry.
8. Blocked/skipped: record hash plus issue metadata with no object hash.

Inventory jobs hash the adapter-produced bytes and use the same safe object
path. Sort inventory relative paths before processing.

Only after every job succeeds does the reducer call `SaveLocalContext` with the
complete new local index. Append nonfatal three-way-merge issues to the result.
A fatal/cancelled collection never calls save and leaves the prior index
untouched.

- [ ] **Step 4: Add bounded deterministic workers**

Default `Workers` to `min(runtime.NumCPU(), 4)`, with a minimum of 1. Workers
receive sorted jobs by index and return `{index, artifact, issue, cacheEntry}`.
A single reducer processes results in ascending index, producing stable
Snapshots, Artifacts, Blocked, and Skipped slices. Do not write the index from
workers. `CollectContext` checks cancellation while traversing, before each
read/hash/projection, while sending/receiving worker jobs, and before index
publication; workers select on `ctx.Done()` so cancellation cannot strand a
goroutine.

Tests run the same fixture with `Workers: 1` and `Workers: 4` and compare
snapshots, artifact metadata, and ordered issues using `reflect.DeepEqual`.

- [ ] **Step 5: Read only artifacts required by planned work**

Add a per-cycle lazy `artifactReader` in `engine.go` and a typed integrity error:

```go
type ArtifactIntegrityError struct {
	RepoRel string
	Hash    string
	Err     error
}

func (e *ArtifactIntegrityError) Error() string
func (e *ArtifactIntegrityError) Unwrap() error

type artifactReader struct {
	content map[string][]byte
}

func (r *artifactReader) Read(artifact resourcecollect.Artifact) ([]byte, error)
```

`Read` opens `ContentPath` only on first demand, verifies SHA-256 equals
`artifact.Hash`, returns `ArtifactIntegrityError` on missing/corrupt content,
and caches an immutable byte copy for the rest of the cycle.

Change `ResourceApplier.PushArtifact` to accept already verified `data []byte`
instead of reopening the path. Route structured merge and install-manifest
consumers through the same reader. Thus a warm no-change sync reads zero CAS
object bytes; only artifacts required by `PushToRemote`, `MergeBoth`, or install
reconciliation are opened, and each at most once.

Update every `Artifact` construction and remaining reference in `engine.go`,
`resource_apply_test.go`, and `install_test.go` from `StagePath` to
`ContentPath`. Add an install test whose manifest exists only at
`ContentPath`; it must reconcile successfully, proving this non-apply consumer
was migrated. Add a warm no-action test that makes every `ContentPath`
read fail through an injected `readFile` seam and proves comparison completes
with zero reader calls.

- [ ] **Step 6: Run collector and apply tests**

```powershell
go test ./internal/resourcecollect ./internal/syncengine -run 'Collector|Artifact|InstallManifest|InstallPlan' -count=1
```

Expected: PASS.

- [ ] **Step 7: Commit**

```powershell
git add internal\resourcecollect internal\syncengine\resource_apply.go internal\syncengine\resource_apply_test.go internal\syncengine\engine.go internal\syncengine\install_test.go
git commit -m "perf: reuse scanned portable content" -m "Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
```

## Task 5: Combine remote snapshot and validation with Git blob reuse

**Files:**
- Create: `internal/syncengine/repository_scan.go`
- Create: `internal/syncengine/repository_scan_test.go`
- Modify: `internal/syncengine/resources.go`
- Modify: `internal/syncengine/resources_test.go`

- [ ] **Step 1: Write blob-reuse and dirty-bypass tests**

Use a real Git fixture plus a reader seam:

```go
type repositoryContentReader interface {
	IndexEntries() ([]gitclient.IndexEntry, error)
	DirtyPaths() (map[string]struct{}, error)
	ReadBlob(string) ([]byte, error)
	ObjectFormat() (string, error)
	RepositoryIdentity() (string, error)
}

func TestRepositoryScannerReusesCleanBlobVerdict(t *testing.T) {
	fixture := newRepositoryScanFixture(t)
	first, err := fixture.scanner.ScanContext(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.cache.SaveGitContext(context.Background(), first.NextIndex); err != nil {
		t.Fatal(err)
	}
	fixture.reader.FailBlobReads = true
	second, err := fixture.scanner.ScanContext(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(second.ValidOwned, first.ValidOwned) {
		t.Fatalf("valid snapshot changed: %#v %#v", first.ValidOwned, second.ValidOwned)
	}
}
```

Add tests:

- Changed clean blob is read once, hashed, and validated once.
- Staged, unstaged, unmerged, deleted, renamed, and untracked paths bypass reuse.
- Dirty and clean tracked symlinks remain blocked, including a clean symlink
  whose blob ID has a cached safe verdict from a regular-file mode.
- Secret, malformed install manifest, projection, and unknown-owner issues match
  current `ValidateRemoteResources` behavior.
- Conflict bundles and other internal metadata are revalidated every scan even
  when their individual blob IDs are unchanged.
- Corrupt Git cache produces a warning and full safe validation.

Run:

```powershell
go test ./internal/syncengine -run RepositoryScanner -count=1
```

Expected: FAIL.

- [ ] **Step 2: Define the combined scanner result**

Create:

```go
type RepositoryScan struct {
	All        state.Snapshot
	Owned      state.Snapshot
	Untouched  state.Snapshot
	ValidOwned state.Snapshot
	Blocked    []resource.Issue
	NextIndex  resourcecache.GitIndex
}

type RepositoryScanner struct {
	Git      repositoryContentReader
	RepoDir  string
	Specs    map[string]resource.Spec
	Codecs   *portableconfig.Registry
	GOOS     string
	UserHome string
	Cache    *resourcecache.Store
	CacheNamespace string
}

func (s *RepositoryScanner) ScanContext(ctx context.Context) (RepositoryScan, error)
```

`CacheNamespace` is the managed environment namespace from Task 2 and is
required. The remote policy hash includes the Spec, scanner, transformer,
object format, GOOS, normalized user home, and cache schema. `GitEntry.Mode` is
validated separately and must match the current index mode before any verdict
is reused. `ScanContext` begins with `LoadGitContext(ctx, repositoryID,
objectFormat, CacheNamespace)` so a later engine `SaveGitContext` performs a
three-way merge against the exact baseline used by this scan.

- [ ] **Step 3: Implement clean blob reuse and dirty reads**

Build the path set from stage-0 index entries under `agents/` plus untracked
dirty paths. Stage 1–3 entries are blocked as `remote-unmerged`.

For a clean, tracked, non-internal path:

- Accept only regular blob modes `100644` and `100755`. Treat `120000`
  symlinks, `160000` gitlinks, and unknown modes as blocked before cache lookup;
  never read or validate their blob bytes as ordinary files.
- Reuse only when path, blob ID, mode, policy hash, repository identity, object
  format, cache namespace, and cache schema match.
- Otherwise `ReadBlob` once, compute SHA-256/size, classify ownership, and run
  the existing validation on those bytes.

For dirty paths:

- Use `os.Lstat` and `os.ReadFile` from the worktree.
- A missing dirty path is a deletion and is omitted from the snapshot.
- Inspect the `Lstat` mode before any `ReadFile`; symlinks are represented in
  `All` consistently with existing behavior but blocked from `ValidOwned`
  without following or opening their targets.
- Do not persist a reusable verdict for dirty content.

Internal paths under `agents/_portable/config/conflicts/` are grouped by
conflict ID and validated as a complete dependency set every pass. Other
internal metadata is likewise validated every pass.

Sort paths and issues before building the result. Check `ctx` before every
worktree/blob read and validation unit. Save `NextIndex` only in the engine
after the complete scan succeeds.

- [ ] **Step 4: Preserve exported compatibility wrappers**

Keep `SnapshotRepo`, `SplitRemoteSnapshot`, and `ValidateRemoteResources` for
existing isolated callers/tests. Extract byte-oriented validation helpers from
`resources.go`, and have both the wrappers and `RepositoryScanner` call them.
Do not make wrappers create or trust a cache.

- [ ] **Step 5: Run all remote-resource tests**

```powershell
go test ./internal/syncengine -run 'RepositoryScanner|SnapshotRepo|ValidateRemote|SplitRemote' -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit**

```powershell
git add internal\syncengine\repository_scan.go internal\syncengine\repository_scan_test.go internal\syncengine\resources.go internal\syncengine\resources_test.go
git commit -m "perf: reuse validated git blobs" -m "Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
```

## Task 6: Skip unchanged base capture reads

**Files:**
- Modify: `internal/state/base.go`
- Modify: `internal/state/base_test.go`

- [ ] **Step 1: Write read-counter tests**

Add a package-private read seam and test:

```go
func TestCaptureRepoPreservingSkipsExistingMatchingObjects(t *testing.T) {
	root := t.TempDir()
	repo := t.TempDir()
	store := NewBaseStore(root)
	data := []byte("same")
	writeBaseFixture(t, repo, "agents/demo/file.txt", data)
	if err := store.CaptureRepo(repo, Snapshot{
		"agents/demo/file.txt": metaFor(data),
	}); err != nil {
		t.Fatal(err)
	}

	reads := 0
	store.readFile = func(name string) ([]byte, error) {
		reads++
		return os.ReadFile(name)
	}
	if err := store.CaptureRepo(repo, Snapshot{
		"agents/demo/file.txt": metaFor(data),
	}); err != nil {
		t.Fatal(err)
	}
	if reads != 0 {
		t.Fatalf("repository reads = %d", reads)
	}
}
```

Also test missing/corrupt object forces a repository read, changed hash updates
the object, preserved paths remain untouched, and deleted paths leave the
index. Add `TestNewBaseStoreSharesCanonicalRootLock` using two separately
constructed stores for equivalent cleaned roots, plus a concurrent disjoint
`Put` test proving both entries survive.

Run:

```powershell
go test ./internal/state -run CaptureRepoPreserving -count=1
```

Expected: FAIL because capture always reads every file.

- [ ] **Step 2: Reuse matching base objects**

Replace the receiver-owned mutex with a process-wide keyed mutex registry.
`NewBaseStore` canonicalizes the absolute base root and assigns the same
`*sync.Mutex` to every instance for that root. This application already
serializes Git ownership at the process/single-instance level; the shared root
lock closes the in-process multi-store hole without inventing a second
repository lock protocol.

Under that shared store lock, load the index before repository reads. For each
snapshot path, skip reading when:

- index hash equals `snapshot[repoRel].Hash`,
- hash syntax is valid, and
- the object exists and passes the existing SHA-256 integrity check.

Read changed/missing/corrupt paths through an injected `readFile` function while
retaining the shared lock, then publish one updated index. This preserves
serialization across separate `BaseStore` objects and avoids an index overwrite race. Keep
`CaptureRepoPreserving`'s all-or-nothing index publication and preserve
semantics.

- [ ] **Step 3: Run state tests**

```powershell
go test ./internal/state -count=1
```

Expected: PASS.

- [ ] **Step 4: Commit**

```powershell
git add internal\state\base.go internal\state\base_test.go
git commit -m "perf: skip unchanged base capture reads" -m "Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
```

## Task 7: Integrate indexed scans into the synchronization engine

**Files:**
- Modify: `internal/syncengine/engine.go`
- Modify: `internal/syncengine/engine_test.go`
- Modify: `internal/syncengine/install_test.go`
- Modify: `internal/syncengine/portable_e2e_test.go`
- Modify: `internal/syncengine/push_retry_test.go`
- Modify: `internal/cli/sync.go`
- Modify: `internal/cli/sync_test.go`

- [ ] **Step 1: Write a two-cycle integration test**

Instrument local projection, blob reads, and cache files:

```go
func TestSecondNoChangeSyncReusesIndexedWork(t *testing.T) {
	fixture := indexedEngineFixture(t)
	first, err := fixture.Engine.SyncOnce()
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Actions) == 0 {
		t.Fatal("first sync performed no fixture action")
	}
	fixture.ResetCounters()
	second, err := fixture.Engine.SyncOnce()
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Actions) != 0 {
		t.Fatalf("second actions = %#v", second.Actions)
	}
	if fixture.ProjectionCalls() != 0 {
		t.Fatalf("second projection calls = %d", fixture.ProjectionCalls())
	}
	if fixture.CleanBlobReads() != 0 {
		t.Fatalf("second clean blob reads = %d", fixture.CleanBlobReads())
	}
	if fixture.BaseCaptureReads() != 0 {
		t.Fatalf("second base reads = %d", fixture.BaseCaptureReads())
	}
}
```

Add a test where a local deletion and a remote dirty file between cycles are
both detected, proving cache reuse does not hide change. Add two integrity
tests:

- same-size corruption of a warm object on a no-action cycle performs zero CAS
  reads and does not affect comparison;
- when that artifact is later required for push/merge, verification happens
  before any repository/local mutation, the object is quarantined, one local
  recollection rebuilds it, and the cycle succeeds. A second integrity failure
  is fatal rather than looping.

Run:

```powershell
go test ./internal/syncengine -run 'TestSecondNoChange|TestIndexedSyncDetects|TestCorruptCachedArtifact' -count=1
```

Expected: FAIL.

- [ ] **Step 2: Add engine cache fields**

Extend:

```go
type Engine struct {
	Git                *gitclient.Client
	RepoDir            string
	Home               string
	UserHome           string
	GOOS               string
	StatePath          string
	Resources          map[string]resource.Spec
	Codecs             *portableconfig.Registry
	Base               *state.BaseStore
	Conflicts          *conflict.Store
	Inventory          resourcecollect.InventoryProvider
	InstallManager     *installplan.Manager
	ApprovedLinks      resourcecollect.ApprovedLinkStore
	TextMerger         portablemerge.TextMerger
	PushRetries        int
	Now                func() time.Time
	OnProgress         func(Progress)
	Cache              *resourcecache.Store
	CacheNamespace     string
	CacheRetentionDays int
}
```

Require a non-nil cache in `Engine.validate`; do not silently create one.
Update every engine fixture in `engine_test.go`, `install_test.go`,
`portable_e2e_test.go`, and `push_retry_test.go` to use
`resourcecache.New(t.TempDir())` and a namespace returned by
`LocalNamespace("managed", fixture.GOOS, fixture.UserHome)`. Construct the local
collector with that exact cache and namespace.

Replace the `SnapshotRepo` / split / validate sequence with one
`RepositoryScanner.ScanContext(context.Background())`. Append cache warnings to `result.Issues` without
converting a safe full rebuild into a fatal error.

- [ ] **Step 3: Publish indexes only after safe phases**

The local collector publishes its index only after complete local collection.
Save the Git `NextIndex` with `SaveGitContext` after remote validation
completes. Append concurrent-update warnings to `result.Issues`; a cache index
write failure is a visible `cache-index-write-failed` issue but does not undo a
safe full scan. After apply/push, perform one final combined repository scan
and save its index with the final state.

Before the first resource mutation, build the sorted required-artifact set from
`PushToRemote`/`MergeBoth` actions plus install-manifest artifacts and call the
per-cycle `artifactReader` for that set. If it returns
`ArtifactIntegrityError`, call
`Cache.QuarantineObjectContext(context.Background(), err.Hash)`, discard/close
the collected result, and rerun local collection + comparison once against the
already validated remote scan. Preflight the rebuilt required set before
continuing. Do not repeat pull and do not mutate repository/local targets until
preflight succeeds. On a second integrity error, return it. Pass the preloaded
reader bytes to push, merge, and install consumers so they do not reopen CAS.

The final scan supplies `currentOwned` for `state.Save` and base capture, so the
engine no longer calls `SnapshotRepo` separately.

Call `CleanupUnreferencedContext` after state and base persistence with
`CleanupBudget{MaxEntries: 500, MaxDuration: 100 * time.Millisecond}`. It
prunes expired namespace indexes/orphan markers and then sweeps unreferenced
objects over bounded resumable passes. The cutoff is
`Now().AddDate(0, 0, -CacheRetentionDays)` with a default of 30 days. An
incomplete pass is normal and not an attention issue. A cleanup error becomes a
visible cache issue and `NeedsAttention`; it does not roll back a successfully
synchronized repository.

- [ ] **Step 4: Wire one cache from CLI**

In `runSyncWithUserHome`:

```go
cache := resourcecache.New(home)
namespace, err := resourcecache.LocalNamespace("managed", goos, userHome)
if err != nil {
	return syncengine.Result{}, err
}
eng := &syncengine.Engine{
	Git:                client,
	RepoDir:            repoDir,
	Home:               home,
	UserHome:           userHome,
	GOOS:               goos,
	StatePath:          StatePath(home),
	Resources:          resources,
	Codecs:             codecs,
	Base:               baseStore,
	Conflicts:          conflictStore,
	Inventory:          inventory,
	InstallManager:     installManager,
	PushRetries:        5,
	Now:                time.Now,
	Cache:              cache,
	CacheRetentionDays: cfg.TrashGraceDays,
	CacheNamespace:     namespace,
}
```

Add `CacheNamespace string` to `Engine` and require it in `validate`. Use the
same store and namespace for every sync invocation so warmed indexes persist
between daemon cycles and application restarts without crossing GOOS or
user-home boundaries.

- [ ] **Step 5: Run engine and CLI sync tests**

```powershell
go test ./internal/syncengine ./internal/cli -run 'Sync|Indexed|NoChange' -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit**

```powershell
git add internal\syncengine\engine.go internal\syncengine\engine_test.go internal\cli\sync.go internal\cli\sync_test.go
git commit -m "perf: run synchronization from persistent indexes" -m "Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
```

## Task 8: Share indexed collection with status and desktop preview

**Files:**
- Modify: `internal/cli/status.go`
- Modify: `internal/cli/status_test.go`
- Modify: `internal/desktop/resources.go`
- Modify: `internal/desktop/resource_service_test.go`

- [ ] **Step 1: Write warm status and preview tests**

For status, run twice with a counting projector and Git reader and assert the
second call reuses safe local objects and clean blob verdicts. For desktop
preview, call `ResourcePreview(context.Background())` twice and assert the second call does not
project unchanged files or write new cache objects.

```go
func TestResourcePreviewReusesIndexedCollection(t *testing.T) {
	service, counters := indexedPreviewService(t)
	if _, err := service.ResourcePreview(context.Background()); err != nil {
		t.Fatal(err)
	}
	counters.Reset()
	if _, err := service.ResourcePreview(context.Background()); err != nil {
		t.Fatal(err)
	}
	if counters.ProjectionCalls() != 0 || counters.ObjectWrites() != 0 {
		t.Fatalf("warm preview counters = %#v", counters)
	}
}
```

Run:

```powershell
go test ./internal/cli ./internal/desktop -run 'Status.*Indexed|ResourcePreviewReuses' -count=1
```

Expected: FAIL.

- [ ] **Step 2: Reuse the cache in CLI status**

Replace `StageParent` collection with:

```go
cache := resourcecache.New(home)
namespace, err := resourcecache.LocalNamespace("managed", goos, userHome)
if err != nil {
	return Status{}, err
}
collector := resourcecollect.New(resourcecollect.Options{
	Cache:          cache,
	CacheNamespace: namespace,
	GOOS:           goos,
	UserHome:       userHome,
	Projector:      codecs,
	Inventory:      inventory,
})
collected, err := collector.CollectContext(context.Background(), specList)
```

Use `RepositoryScanner` with the same namespace and
`ScanContext(context.Background())` instead of separate
snapshot/split/validate calls.
`RunStatus` remains a full action calculation for CLI users, but its warmed
resource work is indexed.

- [ ] **Step 3: Reuse the cache in desktop preview**

The normal provider preview calls
`LocalNamespace("managed", goos, userHome)`, allowing preview, status, and sync
to share safe objects and entries only for the same normalized environment. A
one-custom-resource preview calls
`CustomNamespace("custom-preview", goos, userHome, FingerprintSpec(spec))` and
therefore cannot remove normal index entries.

Remove temporary preview stage creation and cleanup. Preserve the
single-flight/persisted-summary behavior from the prerequisite plan, and pass
its generation context into `CollectContext` so an invalidated preview cannot
publish an index after cancellation.

- [ ] **Step 4: Run affected tests**

```powershell
go test ./internal/cli ./internal/desktop -count=1
```

Expected: PASS.

- [ ] **Step 5: Commit**

```powershell
git add internal\cli\status.go internal\cli\status_test.go internal\desktop\resources.go internal\desktop\resource_service_test.go
git commit -m "perf: share resource indexes across workflows" -m "Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
```

## Task 9: Record and log ordered phase timings

**Files:**
- Create: `internal/syncengine/timing.go`
- Create: `internal/syncengine/timing_test.go`
- Modify: `internal/syncengine/engine.go`
- Modify: `internal/syncengine/engine_test.go`
- Modify: `internal/daemon/daemon.go`
- Modify: `internal/daemon/daemon_test.go`
- Modify: `internal/desktop/models.go`
- Modify: `internal/desktop/service.go`

- [ ] **Step 1: Write success and failure timing tests**

Define stable phase constants and test:

```go
func TestPhaseRecorderPublishesFailedPhase(t *testing.T) {
	now := time.Date(2026, 8, 18, 0, 0, 0, 0, time.UTC)
	recorder := newPhaseRecorder(func() time.Time {
		current := now
		now = now.Add(25 * time.Millisecond)
		return current
	})
	var observed []PhaseTiming
	recorder.onTiming = func(timing PhaseTiming) {
		observed = append(observed, timing)
	}

	finish := recorder.start(PhasePull)
	finish(errors.New("network down"))

	if len(observed) != 1 ||
		observed[0].Name != PhasePull ||
		observed[0].Milliseconds != 25 ||
		!observed[0].Failed {
		t.Fatalf("timings = %#v", observed)
	}
}
```

Add an engine test asserting successful order:

```text
conflict-recovery
pull
local-collection
remote-index-validation
conflict-resolution
comparison
resource-application
install-reconciliation
commit-push
state-cache-persistence
```

Add a daemon test proving a failed pull timing is logged even when sync returns
an error.

Run:

```powershell
go test ./internal/syncengine ./internal/daemon -run Timing -count=1
```

Expected: FAIL.

- [ ] **Step 2: Define timing models**

Create:

```go
type PhaseName string

const (
	PhaseConflictRecovery    PhaseName = "conflict-recovery"
	PhasePull               PhaseName = "pull"
	PhaseConflictResolution PhaseName = "conflict-resolution"
	PhaseLocalCollection    PhaseName = "local-collection"
	PhaseRemoteValidation   PhaseName = "remote-index-validation"
	PhaseComparison         PhaseName = "comparison"
	PhaseResourceApply      PhaseName = "resource-application"
	PhaseInstallReconcile   PhaseName = "install-reconciliation"
	PhaseCommitPush         PhaseName = "commit-push"
	PhaseStatePersistence   PhaseName = "state-cache-persistence"
)

type PhaseTiming struct {
	Name         PhaseName `json:"name"`
	Milliseconds int64     `json:"milliseconds"`
	Failed       bool      `json:"failed"`
}
```

Add `Timings []PhaseTiming` to `syncengine.Result`,
`daemon.CycleResult`, and the desktop snapshot. Define:

```go
type PhaseTiming struct {
	Name         string `json:"name"`
	Milliseconds int64  `json:"milliseconds"`
	Failed       bool   `json:"failed"`
}
```

Add `Timings []PhaseTiming` to `desktop.Snapshot` and map the last cycle's
timings in `Service.Snapshot`.

- [ ] **Step 3: Time every engine phase**

Add to `Engine`:

```go
OnTiming func(PhaseTiming)
```

The recorder appends to the named result and invokes `OnTiming` as each phase
ends. Use a deferred finish closure inside each small phase function so every
return path records `Failed=true` when its phase error is non-nil.

Wrap pre-pull transaction recovery from the prerequisite plan in
`PhaseConflictRecovery`, and wrap the post-mirror `ApplyPendingBatch` call in
`PhaseConflictResolution`, including its queued-only remote re-scan. Therefore
the conflict-resolution timing follows the first remote-index-validation phase
and precedes comparison. Record both conflict phases even when there is no
journal or pending batch. Add a failure test proving a terminal/fatal batch
error emits `PhaseConflictResolution` with `Failed=true` before `SyncOnce`
returns.

Do not change current progress stages or percentages. Push retries may append a
second set of phase timings; preserve attempt order rather than combining
durations.

- [ ] **Step 4: Pass timing observations through CLI and daemon**

Add:

```go
func RunSyncWithObservers(
	home, goos string,
	onProgress func(syncengine.Progress),
	onTiming func(syncengine.PhaseTiming),
) (syncengine.Result, error)
```

Keep `RunSyncWithProgress` as a compatibility wrapper.

Change the daemon's injectable sync function to accept both observers. During
`syncJob`, append timing callbacks to the cycle result and log:

```text
phase pull: 4123ms failed=false
```

On success, deduplicate callback timings against `res.Timings` by using the
callback list as the authoritative cycle list.

- [ ] **Step 5: Run timing and desktop tests**

```powershell
go test ./internal/syncengine ./internal/daemon ./internal/desktop -run 'Timing|Cycle|Snapshot' -count=1
```

Expected: PASS.

- [ ] **Step 6: Commit**

```powershell
git add internal\syncengine internal\daemon internal\cli\sync.go internal\desktop\models.go internal\desktop\service.go
git commit -m "feat: report synchronization phase timings" -m "Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
```

## Task 10: Validate safety and full regression coverage

**Files:**
- Modify only when a test exposes a defect directly caused by Tasks 1–9.

- [ ] **Step 1: Run focused safety packages**

```powershell
go test ./internal/secret ./internal/portableconfig ./internal/resource ./internal/resourcecache ./internal/resourcecollect ./internal/gitclient ./internal/state ./internal/syncengine -count=1
```

Expected: PASS.

- [ ] **Step 2: Run portable end-to-end scenarios**

```powershell
go test ./internal/syncengine -run 'TestPortable|TestFresh|TestTwo|TestDelete|TestConflict' -count=1 -v
```

Expected: legacy repositories, two-computer concurrent edits, deletion
propagation, fresh-profile restore, trusted installer argv, shared resources,
and conflict preservation all PASS.

- [ ] **Step 3: Run frontend before embedded Go tests**

```powershell
npm --prefix frontend test
npm --prefix frontend run build
```

Expected: PASS.

- [ ] **Step 4: Run full Go tests and vet sequentially**

```powershell
go test ./... -count=1
go vet ./...
```

Expected: PASS. Do not overlap this step with the frontend build.

- [ ] **Step 5: Commit direct regression fixes if any**

If a direct defect was corrected:

```powershell
git add internal\resourcecache internal\resourcecollect internal\gitclient internal\state internal\syncengine
git commit -m "fix: preserve indexed sync correctness" -m "Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
```

If no fixes were needed, leave the worktree clean.

## Task 11: Enforce the real-profile performance target

**Files:**
- Create: `internal/cli/performance_test.go`
- Modify implementation files only if phase timing identifies a bottleneck.

- [ ] **Step 1: Add an opt-in real-profile test**

Create:

```go
func TestNoChangeSyncPerformanceRealProfile(t *testing.T) {
	if os.Getenv("ACSYNC_REAL_PROFILE") != "1" {
		t.Skip("set ACSYNC_REAL_PROFILE=1 to measure the live profile")
	}
	userHome, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	home := filepath.Join(userHome, ".acsync")

	if _, err := RunSync(home, runtime.GOOS); err != nil {
		t.Fatalf("warm-up sync: %v", err)
	}
	started := time.Now()
	result, err := RunSync(home, runtime.GOOS)
	elapsed := time.Since(started)
	if err != nil {
		t.Fatal(err)
	}
	for _, timing := range result.Timings {
		t.Logf("%s: %dms failed=%v", timing.Name, timing.Milliseconds, timing.Failed)
	}
	t.Logf("no-change sync: %s", elapsed)
	if elapsed >= 30*time.Second {
		t.Fatalf("no-change sync = %s, want < 30s", elapsed)
	}
}
```

This test operates on the configured live repository and may fetch/push. It is
never enabled in normal CI.

- [ ] **Step 2: Run the warm real-profile measurement**

Ensure no other AgentConfigSync process is synchronizing the same profile, then:

```powershell
$env:ACSYNC_REAL_PROFILE = '1'
go test ./internal/cli -run TestNoChangeSyncPerformanceRealProfile -count=1 -v -timeout 10m
```

Expected: PASS with total no-change duration below 30 seconds and every phase
printed.

- [ ] **Step 3: Optimize only the measured slow phase if necessary**

If the test fails, use the emitted phase timings:

- `pull`: inspect network/Git fetch only; do not weaken fetch correctness.
- `conflict-recovery` / `conflict-resolution`: inspect journals, batch apply,
  and secret scanning without skipping transaction safety.
- `local-collection`: inspect file count, hashing throughput, worker occupancy,
  projection calls, and unexpected cache misses.
- `remote-index-validation`: inspect dirty-path count and unexpected blob reads.
- `resource-application`: inspect unexpected reconcile actions.
- `install-reconciliation`: inspect inventory subprocess duration.
- `state-cache-persistence`: inspect final blob reads and object cleanup.

Add a failing operation-count regression test for the identified cause before
changing code. Rerun only the affected package, then rerun the real-profile
test. Do not raise the 30-second threshold or skip safety checks.

- [ ] **Step 4: Commit the performance acceptance test and any targeted fix**

```powershell
git add internal\cli\performance_test.go
git add internal\resourcecache internal\resourcecollect internal\gitclient internal\state internal\syncengine
git commit -m "perf: meet no-change synchronization target" -m "Co-authored-by: Copilot <223556219+Copilot@users.noreply.github.com>"
```

## Task 12: Build, install, and smoke-test the final Windows package

**Files:**
- Modify: `scripts/smoke/windows.ps1`
- Modify implementation files only if installed validation exposes a directly related defect.
- Output uses `bin\AgentConfigSync-Windows-x64-2026-08-18-$sha.exe`, where
  `$sha` is assigned from `git rev-parse --short HEAD`.

- [ ] **Step 1: Perform final sequential validation**

```powershell
npm --prefix frontend test
npm --prefix frontend run build
go test ./... -count=1
go vet ./...
```

Expected: PASS in this exact order.

- [ ] **Step 2: Build the user-scope NSIS installer**

```powershell
$env:PATH = 'C:\Users\qinqiangxu\.copilot\session-state\be3cd41a-d061-4f74-9a7c-9024015f2234\files\nsis-3.12\nsis-3.12;' + $env:PATH
wails3 package GOOS=windows ARCH=amd64 INSTALL_SCOPE=user
$sha = (git rev-parse --short HEAD).Trim()
Copy-Item bin\AgentConfigSync-amd64-installer.exe "bin\AgentConfigSync-Windows-x64-2026-08-18-$sha.exe"
Get-FileHash "bin\AgentConfigSync-Windows-x64-2026-08-18-$sha.exe" -Algorithm SHA256
```

Expected: the timestamped installer exists and a SHA-256 is printed. The known
nonfatal Windows `uname`/`tail` warnings do not invalidate a successful Wails
package.

- [ ] **Step 3: Make profile preservation an executable smoke assertion**

Extend `scripts\smoke\windows.ps1` with mandatory `ProfileSource` and
`ProfileTarget` parameters. Resolve both paths, require the source to contain
`.acsync\smoke-profile-sentinel.txt`, copy the source profile to the target
before launch, and set `USERPROFILE`/`HOME` to the target. Record the sentinel
SHA-256 before install/launch.

After the installed process is stopped and the uninstaller completes—but
before deleting the isolated target—assert that the target sentinel still
exists and has the same hash. A missing/changed sentinel fails the script.
Only then remove the exact target directory. Never remove `ProfileSource`.

- [ ] **Step 4: Run isolated installer smoke**

Stop only a process whose executable path exactly equals the installed
AgentConfigSync path; never stop by process name. Then run:

```powershell
$sha = (git rev-parse --short HEAD).Trim()
$installer = "bin\AgentConfigSync-Windows-x64-2026-08-18-$sha.exe"
$profileSource = Join-Path $env:TEMP "AgentConfigSync-Smoke-Source-$PID"
$profileTarget = Join-Path $env:TEMP "AgentConfigSync-Smoke-Target-$PID"
$sentinel = Join-Path $profileSource '.acsync\smoke-profile-sentinel.txt'
New-Item -ItemType Directory -Path (Split-Path $sentinel) -Force | Out-Null
Set-Content -LiteralPath $sentinel -Value "preserve-$PID" -NoNewline
try {
    powershell -NoProfile -ExecutionPolicy Bypass -File scripts\smoke\windows.ps1 `
        -Installer $installer `
        -ProfileSource $profileSource `
        -ProfileTarget $profileTarget
} finally {
    if (Test-Path -LiteralPath $profileSource) {
        Remove-Item -LiteralPath $profileSource -Recurse -Force
    }
}
```

Expected: isolated install, first launch, second-instance handoff, profile
sentinel preservation, and uninstall checks PASS.

- [ ] **Step 5: Install the final package for the current user**

Run the timestamped installer normally so the user can complete UI prompts.
Launch from the Start Menu, confirm the tray icon appears, and confirm a second
launch activates the existing main window instead of starting another daemon.

- [ ] **Step 6: Verify adaptive window and fast control plane**

On a sufficiently large primary work area:

- Main client area opens centered at 1200 by 850 logical pixels.
- Smaller work areas retain 48-pixel margins where possible and scroll.
- Pause/Resume/Sync/Approve/Retry/Apply return control in under one second.
- Settings opens without waiting for a resource scan.

- [ ] **Step 7: Verify installed warmed synchronization**

Allow one installed cycle to warm the cache. Trigger one no-change cycle and
inspect `~\.acsync\logs\daemon.log`:

- Total duration is below 30 seconds.
- Every defined phase has a timing line.
- No repeated Plugin attempt occurs unless Retry was clicked.
- One submitted conflict batch produces one queued cycle.

- [ ] **Step 8: Record release evidence**

Report:

- Installer absolute path.
- SHA-256.
- Installed executable path.
- Installed process ID.
- Measured client dimensions.
- Maximum lightweight Snapshot duration.
- Warmed no-change sync duration and phase timings.
- Sequential frontend/test/vet results.

If any acceptance criterion fails, do not declare the task complete; return to
the failing phase's focused test and implementation task.
