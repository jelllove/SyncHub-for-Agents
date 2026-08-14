package syncengine

import (
	"os/exec"
	"path/filepath"
	"sync"
	"testing"
)

func TestSyncOnceReconcilesAgainAfterRejectedPush(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	bare := newBareRemote(t)
	repo := filepath.Join(t.TempDir(), "repo")
	root := t.TempDir()
	client := cloneWorkspace(t, bare, repo)
	engine := engineFor(client, repo, filepath.Join(t.TempDir(), "state.json"), root)
	writeFile(t, filepath.Join(root, "settings.json"), `{"value":"base"}`)
	if _, err := engine.SyncOnce(); err != nil {
		t.Fatal(err)
	}

	racerRepo := filepath.Join(t.TempDir(), "racer")
	cloneWorkspace(t, bare, racerRepo)
	var raceOnce sync.Once
	client.Command = func(name string, args ...string) *exec.Cmd {
		if len(args) > 0 && args[0] == "push" {
			raceOnce.Do(func() {
				writeFile(
					t,
					filepath.Join(racerRepo, "agents", "demo", "config", "settings.json"),
					`{"value":"remote-race"}`,
				)
				git(t, racerRepo, "add", "-A")
				git(t, racerRepo, "commit", "-m", "race")
				git(t, racerRepo, "push")
			})
		}
		return exec.Command(name, args...)
	}
	writeFile(t, filepath.Join(root, "settings.json"), `{"value":"local-race"}`)

	result, err := engine.SyncOnce()
	if err != nil {
		t.Fatal(err)
	}
	if result.Conflicts != 1 || !result.Pushed {
		t.Fatalf("result = %#v", result)
	}
	assertFileContent(t, filepath.Join(root, "settings.json"), `{"value":"local-race"}`)
	assertFileContent(
		t,
		filepath.Join(repo, "agents", "demo", "config", "settings.json"),
		`{"value":"remote-race"}`,
	)
}
