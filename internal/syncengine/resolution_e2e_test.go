package syncengine

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/qinqingxu/synchub-for-agents/internal/conflict"
)

func queueAllConflicts(t *testing.T, engine *Engine, choice conflict.Choice) {
	t.Helper()
	store := conflict.NewStore(
		filepath.Join(engine.Home, "conflicts"),
		engine.RepoDir,
		ConflictScanner(engine.Resources),
	)
	visible, _, err := store.VisibleConflicts()
	if err != nil {
		t.Fatal(err)
	}
	if len(visible) == 0 {
		t.Fatal("no visible conflicts to resolve")
	}
	selections := make([]conflict.ResolutionSelection, 0, len(visible))
	for _, item := range visible {
		selections = append(selections, conflict.ResolutionSelection{
			ID:       item.Record.ID,
			Revision: item.Revision,
			Choice:   choice,
		})
		if choice == conflict.ChoiceMerged {
			selections[len(selections)-1].Content = []byte(`{"value":"merged"}`)
		}
	}

	if _, err := store.QueueBatch(selections); err != nil {
		t.Fatal(err)
	}
}

func TestSyncOnceHoldsExcludedResolutionBatch(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	for _, choice := range []conflict.Choice{conflict.ChoiceRemote, conflict.ChoiceMerged} {
		for _, unavailable := range []bool{false, true} {
			name := string(choice) + "/secret-detected"
			if unavailable {
				name = string(choice) + "/root-unavailable"
			}
			t.Run(name, func(t *testing.T) {
				bare := newBareRemote(t)
				repoA, repoB := filepath.Join(t.TempDir(), "repoA"), filepath.Join(t.TempDir(), "repoB")
				rootA, rootB := t.TempDir(), t.TempDir()
				engineA := engineFor(cloneWorkspace(t, bare, repoA), repoA, filepath.Join(t.TempDir(), "state.json"), rootA)
				engineB := engineFor(cloneWorkspace(t, bare, repoB), repoB, filepath.Join(t.TempDir(), "state.json"), rootB)
				for _, engine := range []*Engine{engineA, engineB} {
					spec := engine.Resources["demo/legacy-config"]
					spec.Include = []string{"settings.json", "other.json"}
					spec.KeyPatterns = []string{"token"}
					engine.Resources[spec.Key] = spec
				}
				for _, filename := range []string{"settings.json", "other.json"} {
					writeFile(t, filepath.Join(rootA, filename), `{"value":"base"}`)
				}
				if _, err := engineA.SyncOnce(); err != nil {
					t.Fatal(err)
				}
				if _, err := engineB.SyncOnce(); err != nil {
					t.Fatal(err)
				}
				for _, filename := range []string{"settings.json", "other.json"} {
					writeFile(t, filepath.Join(rootA, filename), `{"value":"machine-a"}`)
					writeFile(t, filepath.Join(rootB, filename), `{"value":"machine-b"}`)
				}
				if _, err := engineA.SyncOnce(); err != nil {
					t.Fatal(err)
				}
				if result, err := engineB.SyncOnce(); err != nil || result.Conflicts != 2 {
					t.Fatalf("create conflicts = %#v, %v", result, err)
				}
				queueAllConflicts(t, engineB, choice)

				protected := `{"value":"machine-b","accessToken":"synthetic"}`
				if unavailable {
					if err := os.Rename(rootB, rootB+"-parked"); err != nil {
						t.Fatal(err)
					}
				} else {
					writeFile(t, filepath.Join(rootB, "settings.json"), protected)
				}
				result, err := engineB.SyncOnce()
				if err != nil {
					t.Fatal(err)
				}
				if unavailable {
					if _, err := os.Stat(rootB); !os.IsNotExist(err) {
						t.Fatalf("unavailable root was recreated: %v", err)
					}
					assertFileContent(t, filepath.Join(rootB+"-parked", "settings.json"), `{"value":"machine-b"}`)
				} else {
					assertFileContent(t, filepath.Join(rootB, "settings.json"), protected)
					assertFileContent(t, filepath.Join(rootB, "other.json"), `{"value":"machine-b"}`)
				}
				for _, filename := range []string{"settings.json", "other.json"} {
					assertFileContent(t, filepath.Join(repoB, "agents", "demo", "config", filename), `{"value":"machine-a"}`)
				}
				if result.Conflicts != 2 || !result.NeedsAttention {
					t.Fatalf("held resolution lost conflicts or attention: %#v", result)
				}
				issues, code := result.BlockedIssues, "secret-detected"
				if unavailable {
					issues, code = result.SkippedIssues, "root-unavailable"
				}
				found := false
				for _, issue := range issues {
					found = found || issue.Code == code
				}
				if !found {
					t.Fatalf("missing collection issue %q: %#v", code, issues)
				}
				store := conflict.NewStore(filepath.Join(engineB.Home, "conflicts"), repoB, ConflictScanner(engineB.Resources))
				pending, err := store.PendingBatch()
				if err != nil || pending == nil || pending.Status != "queued" {
					t.Fatalf("resolution batch was not retained: %#v, %v", pending, err)
				}
				if unavailable {
					if err := os.Rename(rootB+"-parked", rootB); err != nil {
						t.Fatal(err)
					}
				} else {
					writeFile(t, filepath.Join(rootB, "settings.json"), `{"value":"machine-b"}`)
				}
				result, err = engineB.SyncOnce()
				if err != nil || result.Conflicts != 0 || result.NeedsAttention {
					t.Fatalf("safe resolution did not converge: %#v, %v", result, err)
				}
				want := `{"value":"machine-a"}`
				if choice == conflict.ChoiceMerged {
					want = `{"value":"merged"}`
				}
				for _, filename := range []string{"settings.json", "other.json"} {
					assertFileContent(t, filepath.Join(rootB, filename), want)
				}
				assertNoConflicts(t, engineB)
			})
		}
	}
}
func assertNoConflicts(t *testing.T, engine *Engine) {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(engine.Home, "conflicts"))
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("conflicts remain after resolution: %#v", entries)
	}
}

func TestSyncOnceConvergesAfterBatchResolution(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	type setup func(t *testing.T, rootA, rootB string, engineA, engineB *Engine)
	withBase := func(t *testing.T, rootA, rootB string, engineA, engineB *Engine) {
		writeFile(t, filepath.Join(rootA, "settings.json"), `{"value":"base"}`)
		if _, err := engineA.SyncOnce(); err != nil {
			t.Fatal(err)
		}
		if _, err := engineB.SyncOnce(); err != nil {
			t.Fatal(err)
		}
		writeFile(t, filepath.Join(rootA, "settings.json"), `{"value":"machine-a"}`)
		if _, err := engineA.SyncOnce(); err != nil {
			t.Fatal(err)
		}
		writeFile(t, filepath.Join(rootB, "settings.json"), `{"value":"machine-b"}`)
	}
	withoutBase := func(t *testing.T, rootA, rootB string, engineA, _ *Engine) {
		writeFile(t, filepath.Join(rootA, "settings.json"), `{"value":"machine-a"}`)
		if _, err := engineA.SyncOnce(); err != nil {
			t.Fatal(err)
		}
		writeFile(t, filepath.Join(rootB, "settings.json"), `{"value":"machine-b"}`)
	}
	for _, tc := range []struct {
		name   string
		setup  setup
		choice conflict.Choice
		want   string
	}{
		{"existing base use local", withBase, conflict.ChoiceLocal, `{"value":"machine-b"}`},
		{"existing base use remote", withBase, conflict.ChoiceRemote, `{"value":"machine-a"}`},
		{"first sync use local", withoutBase, conflict.ChoiceLocal, `{"value":"machine-b"}`},
		{"first sync use remote", withoutBase, conflict.ChoiceRemote, `{"value":"machine-a"}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			bare := newBareRemote(t)
			repoA := filepath.Join(t.TempDir(), "repoA")
			repoB := filepath.Join(t.TempDir(), "repoB")
			rootA := t.TempDir()
			rootB := t.TempDir()
			engineA := engineFor(cloneWorkspace(t, bare, repoA), repoA, filepath.Join(t.TempDir(), "state.json"), rootA)
			engineB := engineFor(cloneWorkspace(t, bare, repoB), repoB, filepath.Join(t.TempDir(), "state.json"), rootB)
			tc.setup(t, rootA, rootB, engineA, engineB)

			result, err := engineB.SyncOnce()
			if err != nil {
				t.Fatal(err)
			}
			if result.Conflicts != 1 {
				t.Fatalf("expected one conflict, result = %#v", result)
			}

			queueAllConflicts(t, engineB, tc.choice)
			result, err = engineB.SyncOnce()
			if err != nil {
				t.Fatal(err)
			}
			if result.Conflicts != 0 || result.NeedsAttention {
				t.Fatalf("resolution did not converge, result = %#v", result)
			}
			assertNoConflicts(t, engineB)
			assertFileContent(t, filepath.Join(rootB, "settings.json"), tc.want)
			assertFileContent(t, filepath.Join(repoB, "agents", "demo", "config", "settings.json"), tc.want)

			result, err = engineB.SyncOnce()
			if err != nil {
				t.Fatal(err)
			}
			if result.Conflicts != 0 || len(result.Actions) != 0 {
				t.Fatalf("follow-up sync did not stay converged, result = %#v", result)
			}

			if _, err := engineA.SyncOnce(); err != nil {
				t.Fatal(err)
			}
			assertFileContent(t, filepath.Join(rootA, "settings.json"), tc.want)
		})
	}
}
