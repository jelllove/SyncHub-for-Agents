package desktop

import (
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/qinqingxu/acsync/internal/daemon"
)

func TestSummaryStoreRoundTripsOwnerOnlyState(t *testing.T) {
	root := t.TempDir()
	store := newSummaryStore(root)
	cycle := daemon.CycleResult{
		Actions:    3,
		Blocked:    2,
		FinishedAt: time.Date(2026, 8, 18, 10, 11, 12, 0, time.UTC),
	}
	preview := ResourcePreview{
		GeneratedAt: time.Date(2026, 8, 18, 10, 10, 0, 0, time.UTC),
		Files:       7,
	}

	if err := store.saveCycle(cycle); err != nil {
		t.Fatal(err)
	}
	if err := store.savePreview(preview); err != nil {
		t.Fatal(err)
	}
	gotCycle, err := store.loadCycle()
	if err != nil || !reflect.DeepEqual(gotCycle, cycle) {
		t.Fatalf("loadCycle() = %#v, %v", gotCycle, err)
	}
	gotPreview, err := store.loadPreview()
	if err != nil || !reflect.DeepEqual(gotPreview, preview) {
		t.Fatalf("loadPreview() = %#v, %v", gotPreview, err)
	}
	for _, name := range []string{"cycle.json", "preview.json"} {
		info, err := os.Stat(filepath.Join(root, "desktop", name))
		if err != nil {
			t.Fatalf("stat %s: %v", name, err)
		}
		if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
			t.Fatalf("%s mode = %v, want owner-only", name, info.Mode().Perm())
		}
	}
}

func TestSummaryStoreAtomicallyReplacesExistingPreview(t *testing.T) {
	store := newSummaryStore(t.TempDir())
	if err := store.savePreview(ResourcePreview{Files: 1}); err != nil {
		t.Fatal(err)
	}
	if err := store.savePreview(ResourcePreview{Files: 2}); err != nil {
		t.Fatal(err)
	}

	preview, err := store.loadPreview()
	if err != nil {
		t.Fatal(err)
	}
	if preview.Files != 2 {
		t.Fatalf("loadPreview().Files = %d, want 2", preview.Files)
	}
}

func TestSummaryStoreRejectsMalformedJSON(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "desktop", "cycle.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := newSummaryStore(root).loadCycle(); err == nil {
		t.Fatal("loadCycle() error = nil")
	}
}

func TestSummaryStoreMissingFilesReturnZeroValues(t *testing.T) {
	store := newSummaryStore(t.TempDir())

	cycle, err := store.loadCycle()
	if err != nil || !reflect.DeepEqual(cycle, daemon.CycleResult{}) {
		t.Fatalf("loadCycle() = %#v, %v", cycle, err)
	}
	preview, err := store.loadPreview()
	if err != nil || !reflect.DeepEqual(preview, ResourcePreview{}) {
		t.Fatalf("loadPreview() = %#v, %v", preview, err)
	}
}

func TestSummaryStoreClearPreviewIsIdempotent(t *testing.T) {
	store := newSummaryStore(t.TempDir())
	if err := store.savePreview(ResourcePreview{Files: 1}); err != nil {
		t.Fatal(err)
	}
	if err := store.clearPreview(); err != nil {
		t.Fatal(err)
	}
	if err := store.clearPreview(); err != nil {
		t.Fatal(err)
	}
	preview, err := store.loadPreview()
	if err != nil || !reflect.DeepEqual(preview, ResourcePreview{}) {
		t.Fatalf("loadPreview() = %#v, %v", preview, err)
	}
}

func TestRecordCyclePersistsForNextService(t *testing.T) {
	home := configuredHome(t)
	first, err := New(home, runtime.GOOS)
	if err != nil {
		t.Fatal(err)
	}
	cycle := daemon.CycleResult{
		Actions:    4,
		Pushed:     true,
		FinishedAt: time.Date(2026, 8, 18, 11, 12, 13, 0, time.UTC),
	}
	first.recordCycle(cycle)
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}

	second, err := New(home, runtime.GOOS)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	second.mu.RLock()
	got := second.last
	second.mu.RUnlock()
	if !reflect.DeepEqual(got, cycle) {
		t.Fatalf("last cycle = %#v, want %#v", got, cycle)
	}
}

func TestNewRejectsMalformedCycleSummary(t *testing.T) {
	home := configuredHome(t)
	path := filepath.Join(home, "desktop", "cycle.json")
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}

	service, err := New(home, runtime.GOOS)
	if service != nil {
		defer service.Close()
	}
	if err == nil {
		t.Fatal("New() error = nil")
	}
}

func TestRecordCycleSurfacesSummaryPersistenceFailure(t *testing.T) {
	home := configuredHome(t)
	service, err := New(home, runtime.GOOS)
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	if err := os.WriteFile(filepath.Join(home, "desktop"), []byte("blocked"), 0o600); err != nil {
		t.Fatal(err)
	}

	service.recordCycle(daemon.CycleResult{})

	service.mu.RLock()
	got := service.last
	service.mu.RUnlock()
	if !got.NeedsAttention {
		t.Fatal("summary persistence failure did not require attention")
	}
	if !strings.Contains(got.Error, "save desktop cycle summary") {
		t.Fatalf("cycle error = %q", got.Error)
	}
}
