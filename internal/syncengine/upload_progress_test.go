package syncengine

import (
	"path/filepath"
	"testing"
)

func TestNoChangeCycleDoesNotReportPushing(t *testing.T) {
	bare := newBareRemote(t)
	repo, root := filepath.Join(t.TempDir(), "repo"), t.TempDir()
	writeFile(t, filepath.Join(root, "settings.json"), `{"theme":"dark"}`)
	engine := engineFor(cloneWorkspace(t, bare, repo), repo, filepath.Join(t.TempDir(), "state.json"), root)
	if _, err := engine.SyncOnce(); err != nil {
		t.Fatal(err)
	}
	var progress []Progress
	engine.OnProgress = func(update Progress) { progress = append(progress, update) }
	result, err := engine.SyncOnce()
	if err != nil {
		t.Fatal(err)
	}
	if result.Pushed {
		t.Fatal("synthetic unchanged cycle unexpectedly pushed")
	}
	for _, update := range progress {
		if update.Stage == StageUploading {
			t.Fatal("a cycle with no Git push must not display Pushing")
		}
	}
}
