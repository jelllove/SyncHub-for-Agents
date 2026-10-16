package desktop

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/qinqingxu/synchub-for-agents/internal/sharedlibrary"
)

func TestLibraryWailsFacadeUsesPreviewApproval(t *testing.T) {
	user := t.TempDir()
	t.Setenv("HOME", user)
	t.Setenv("USERPROFILE", user)
	root := filepath.Join(user, ".claude")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "CLAUDE.md"), []byte("Read the tests."), 0o600); err != nil {
		t.Fatal(err)
	}
	core, err := New(filepath.Join(user, "profile"), "")
	if err != nil {
		t.Fatal(err)
	}
	defer core.Close()
	service := NewWailsService(nil, core, nil, nil)
	r := sharedlibrary.Request{Action: "import", Kind: "instructions", Name: "global", Harness: "claude"}
	preview, err := service.PreviewLibrary(r)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.ApplyLibrary(r, "stale"); err == nil {
		t.Fatal("accepted stale preview")
	}
	if err := service.ApplyLibrary(r, preview.ID); err != nil {
		t.Fatal(err)
	}
	snapshot, err := service.LibrarySnapshot()
	if err != nil || len(snapshot.Entries) < 2 {
		t.Fatalf("%+v %v", snapshot, err)
	}
}
