package conflict

import (
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

func resolutionFixture(t *testing.T, scanner Scanner) *Store {
	t.Helper()
	store := NewStore(t.TempDir(), t.TempDir(), scanner)
	record := Record{ID: "one", ResourceKey: "demo/settings", RepoRel: "agents/demo/settings.json"}
	if err := store.Create(record, []byte("base"), []byte("local"), []byte("remote")); err != nil {
		t.Fatal(err)
	}
	record.ID = "two"
	if err := store.Create(record, []byte("base2"), []byte("local2"), []byte("remote2")); err != nil {
		t.Fatal(err)
	}
	return store
}

func TestConflictRevisionIsDeterministicAndQueuePersists(t *testing.T) {
	store := resolutionFixture(t, nil)
	rev, err := store.Revision("one")
	if err != nil {
		t.Fatal(err)
	}
	if rev2, _ := store.Revision("one"); rev != rev2 {
		t.Fatalf("revisions differ: %q %q", rev, rev2)
	}
	batch, err := store.QueueBatch([]ResolutionSelection{
		{ID: "two", Revision: mustRevision(t, store, "two"), Choice: ChoiceRemote},
		{ID: "one", Revision: rev, Choice: ChoiceLocal},
	})
	if err != nil {
		t.Fatal(err)
	}
	if batch.ID == "" || batch.Status != "queued" {
		t.Fatalf("batch = %#v", batch)
	}
	pending, err := store.PendingBatch()
	if err != nil || pending == nil || pending.ID != batch.ID {
		t.Fatalf("pending = %#v, %v", pending, err)
	}
	if pending.Selections[0].ID != "one" {
		t.Fatalf("selections not sorted: %#v", pending.Selections)
	}
}

func TestQueueBatchRequiresCompleteValidFreshSelections(t *testing.T) {
	store := resolutionFixture(t, nil)
	rev := mustRevision(t, store, "one")
	tests := []struct {
		name string
		sel  []ResolutionSelection
	}{
		{"empty", nil},
		{"missing", []ResolutionSelection{{ID: "one", Revision: rev, Choice: ChoiceLocal}}},
		{"duplicate", []ResolutionSelection{{ID: "one", Revision: rev, Choice: ChoiceLocal}, {ID: "one", Revision: rev, Choice: ChoiceLocal}, {ID: "two", Revision: mustRevision(t, store, "two"), Choice: ChoiceLocal}}},
		{"unsupported", []ResolutionSelection{{ID: "one", Revision: rev, Choice: "bad"}, {ID: "two", Revision: mustRevision(t, store, "two"), Choice: ChoiceLocal}}},
		{"stale", []ResolutionSelection{{ID: "one", Revision: "stale", Choice: ChoiceLocal}, {ID: "two", Revision: mustRevision(t, store, "two"), Choice: ChoiceLocal}}},
		{"merged-missing", []ResolutionSelection{{ID: "one", Revision: rev, Choice: ChoiceMerged}, {ID: "two", Revision: mustRevision(t, store, "two"), Choice: ChoiceLocal}}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := store.QueueBatch(test.sel); err == nil {
				t.Fatal("QueueBatch() error = nil")
			}
			if pending, err := store.PendingBatch(); err != nil || pending != nil {
				t.Fatalf("pending = %#v, %v", pending, err)
			}
		})
	}
}

func TestQueueBatchScansMergedBeforePersisting(t *testing.T) {
	store := resolutionFixture(t, func(record Record, variant string, _ []byte) error {
		if record.ID == "one" && variant == "merged" {
			return errors.New("blocked")
		}
		return nil
	})
	_, err := store.QueueBatch([]ResolutionSelection{
		{ID: "one", Revision: mustRevision(t, store, "one"), Choice: ChoiceMerged, Content: []byte("merged")},
		{ID: "two", Revision: mustRevision(t, store, "two"), Choice: ChoiceLocal},
	})
	if err == nil {
		t.Fatal("QueueBatch() error = nil")
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(store.localRoot), "conflict-resolution", "pending.json")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("pending metadata exists: %v", err)
	}
}

func TestQueueBatchAllowsOnlyOneActiveBatchAcrossStores(t *testing.T) {
	localRoot, repoDir := t.TempDir(), t.TempDir()
	first := NewStore(localRoot, repoDir, nil)
	record := Record{ID: "one", ResourceKey: "demo/settings", RepoRel: "agents/demo/settings.json"}
	if err := first.Create(record, []byte("base"), []byte("local"), []byte("remote")); err != nil {
		t.Fatal(err)
	}
	second := NewStore(localRoot, repoDir, nil)
	selection := ResolutionSelection{ID: "one", Revision: mustRevision(t, first, "one"), Choice: ChoiceLocal}
	start := make(chan struct{})
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, store := range []*Store{first, second} {
		wg.Add(1)
		go func(store *Store) {
			defer wg.Done()
			<-start
			_, err := store.QueueBatch([]ResolutionSelection{selection})
			results <- err
		}(store)
	}
	close(start)
	wg.Wait()
	close(results)
	var successes, active int
	for err := range results {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ErrResolutionActive):
			active++
		}
	}
	if successes != 1 || active != 1 {
		t.Fatalf("queue results: successes=%d active=%d", successes, active)
	}
}

func mustRevision(t *testing.T, store *Store, id string) string {
	t.Helper()
	rev, err := store.Revision(id)
	if err != nil {
		t.Fatal(err)
	}
	return rev
}
