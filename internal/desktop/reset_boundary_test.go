package desktop

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/qinqingxu/synchub-for-agents/internal/cli"
	"github.com/qinqingxu/synchub-for-agents/internal/config"
	"github.com/qinqingxu/synchub-for-agents/internal/onboarding"
)

func TestResetDefaultCloneWithStaleRepositorySettings(t *testing.T) {
	for _, scenario := range []string{"changed URL", "missing config", "missing origin", "SSH alias"} {
		t.Run(scenario, func(t *testing.T) {
			core := resetFixture(t)
			repo := cli.RepoDir(core.home)
			switch scenario {
			case "missing config":
				if err := os.Remove(cli.ConfigPath(core.home)); err != nil {
					t.Fatal(err)
				}
			case "missing origin":
				if err := exec.Command("git", "-C", repo, "remote", "remove", "origin").Run(); err != nil {
					t.Fatal(err)
				}
			default:
				cfg, err := config.Load(cli.ConfigPath(core.home))
				if err != nil {
					t.Fatal(err)
				}
				cfg.RepoURL = "git@github.com:example/changed.git"
				if scenario == "SSH alias" {
					cfg.RepoURL = "git@github-work:example/synthetic.git"
				}
				if err := config.Save(cli.ConfigPath(core.home), cfg); err != nil {
					t.Fatal(err)
				}
			}
			wizard := onboarding.New(onboarding.Dependencies{})
			service := NewWailsService(nil, core, wizard, nil)
			if err := service.ResetLocalSetup(context.Background(), "RESET", repo); err != nil {
				t.Fatalf("default clone reset blocked by stale setup: %v", err)
			}
			if _, err := os.Stat(repo); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("default clone remains: %v", err)
			}
			if wizard.State().Step != onboarding.Welcome {
				t.Fatal("reset did not return to Welcome")
			}
		})
	}
}

func TestResetIncompleteOnboardingClone(t *testing.T) {
	core := resetFixture(t)
	if err := os.Remove(cli.ConfigPath(core.home)); err != nil {
		t.Fatal(err)
	}
	wizard := onboarding.New(onboarding.Dependencies{})
	if err := wizard.SetRepository("git@github.com:example/synthetic.git"); err != nil {
		t.Fatal(err)
	}
	service := NewWailsService(nil, core, wizard, nil)
	if err := service.ResetLocalSetup(context.Background(), "RESET", cli.RepoDir(core.home)); err != nil {
		t.Fatal(err)
	}
	if wizard.State().Step != onboarding.Welcome || wizard.State().RepositoryURL != "" {
		t.Fatalf("wizard not reset: %#v", wizard.State())
	}
	if _, err := os.Stat(cli.RepoDir(core.home)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("incomplete clone remains: %v", err)
	}
}

func TestResetCustomCloneOnlyRemovesSelectedDirectory(t *testing.T) {
	service := resetFixture(t)
	parent := t.TempDir()
	repo := filepath.Join(parent, "clone")
	if err := os.Rename(cli.RepoDir(service.home), repo); err != nil {
		t.Fatal(err)
	}
	resetFile(t, filepath.Join(parent, "keep.txt"))
	cfg, err := config.Load(cli.ConfigPath(service.home))
	if err != nil {
		t.Fatal(err)
	}
	cfg.RepoDir = repo
	if err := config.Save(cli.ConfigPath(service.home), cfg); err != nil {
		t.Fatal(err)
	}
	if err := service.ResetLocalSetup(context.Background(), "RESET", repo); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(repo); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("custom clone remains: %v", err)
	}
	if _, err := os.Stat(filepath.Join(parent, "keep.txt")); err != nil {
		t.Fatal("reset removed neighbouring data")
	}
}

func TestResetRejectsFilesystemRoot(t *testing.T) {
	service := resetFixture(t)
	cfg, err := config.Load(cli.ConfigPath(service.home))
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.VolumeName(service.home) + string(filepath.Separator)
	cfg.RepoDir = root
	if err := config.Save(cli.ConfigPath(service.home), cfg); err != nil {
		t.Fatal(err)
	}
	if err := service.ResetLocalSetup(context.Background(), "RESET", root); err == nil {
		t.Fatal("filesystem root accepted")
	}
}

func TestResetRejectsSymlinkedClone(t *testing.T) {
	service := resetFixture(t)
	repo := cli.RepoDir(service.home)
	other := filepath.Join(t.TempDir(), "actual")
	if err := os.Rename(repo, other); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(other, repo); err != nil {
		t.Skipf("symlink unavailable on this host: %v", err)
	}
	if err := service.ResetLocalSetup(context.Background(), "RESET", repo); err == nil {
		t.Fatal("symlinked clone accepted")
	}
	if _, err := os.Stat(other); err != nil {
		t.Fatal("symlink target was removed")
	}
}
