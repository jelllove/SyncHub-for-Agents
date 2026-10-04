package desktop

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/qinqingxu/synchub-for-agents/internal/auth"
	"github.com/qinqingxu/synchub-for-agents/internal/cli"
	"github.com/qinqingxu/synchub-for-agents/internal/config"
	"github.com/qinqingxu/synchub-for-agents/internal/scheduler"
)

func resetFixture(t *testing.T) *Service {
	t.Helper()
	home := filepath.Join(t.TempDir(), ".synchub")
	repo := cli.RepoDir(home)
	if err := os.MkdirAll(repo, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"init", repo},
		{"-C", repo, "remote", "add", "origin", "git@github.com:example/synthetic.git"},
	} {
		if out, err := exec.Command("git", args...).CombinedOutput(); err != nil {
			t.Fatalf("git: %v: %s", err, out)
		}
	}
	cfg := config.Default(nil)
	cfg.RepoURL = "git@github.com:example/synthetic.git"
	cfg.FirstSync = config.FirstSyncPolicy{Strategy: config.FirstSyncStrategyChoose}
	if err := config.Save(cli.ConfigPath(home), cfg); err != nil {
		t.Fatal(err)
	}
	service, err := New(home, runtime.GOOS)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = service.Close() })
	return service
}

func resetFile(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("synthetic"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestResetRemovesSetupAndPreservesUnrelatedData(t *testing.T) {
	service := resetFixture(t)
	for _, path := range []string{
		"state.json", "base/index.json", "conflicts/sample/local",
		"conflict-resolution/pending.json", "install/pending.json",
		"desktop/preview.json",
	} {
		resetFile(t, filepath.Join(service.home, filepath.FromSlash(path)))
	}
	for _, path := range []string{"providers/notes.txt", "logs/keep.log", "updates/settings.json"} {
		resetFile(t, filepath.Join(service.home, filepath.FromSlash(path)))
	}
	source := filepath.Join(filepath.Dir(service.home), "agent", "settings.json")
	resetFile(t, source)
	if err := auth.SaveMetadata(cli.AuthMetadataPath(service.home), auth.Metadata{
		Active: auth.Account{ID: 123, Login: "synthetic"},
	}); err != nil {
		t.Fatal(err)
	}
	deleted := int64(0)
	service.resetCredential = func(id int64) error { deleted = id; return nil }
	preview, err := service.ResetPreview()
	if err != nil {
		t.Fatal(err)
	}
	if err := service.ResetLocalSetup(context.Background(), "RESET", preview.RepoPath); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"config.yaml", "repo", "state.json", "base", "conflicts",
		"conflict-resolution", "install", "desktop", "auth.json",
	} {
		if _, err := os.Stat(filepath.Join(service.home, name)); !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("%s remains: %v", name, err)
		}
	}
	for _, path := range []string{
		source, filepath.Join(service.home, "providers", "notes.txt"),
		filepath.Join(service.home, "logs", "keep.log"),
		filepath.Join(service.home, "updates", "settings.json"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("preserved path %s: %v", path, err)
		}
	}
	if deleted != 123 || service.Daemon() != nil {
		t.Fatalf("credential ID = %d, daemon = %v", deleted, service.Daemon())
	}
}

func TestResetRejectsUnconfirmedStaleAndUnsafeClone(t *testing.T) {
	for _, test := range []string{"confirmation", "stale path", "unexpected file", "mismatched origin"} {
		t.Run(test, func(t *testing.T) {
			service := resetFixture(t)
			token, repo := "RESET", cli.RepoDir(service.home)
			switch test {
			case "confirmation":
				token = ""
			case "stale path":
				repo = t.TempDir()
			case "unexpected file":
				resetFile(t, filepath.Join(repo, "personal.txt"))
			case "mismatched origin":
				custom := filepath.Join(t.TempDir(), "clone")
				if err := os.Rename(repo, custom); err != nil {
					t.Fatal(err)
				}
				cfg, err := config.Load(cli.ConfigPath(service.home))
				if err != nil {
					t.Fatal(err)
				}
				cfg.RepoDir = custom
				if err := config.Save(cli.ConfigPath(service.home), cfg); err != nil {
					t.Fatal(err)
				}
				repo = custom
				if err := exec.Command("git", "-C", repo, "remote", "set-url", "origin", "other").Run(); err != nil {
					t.Fatal(err)
				}
			}
			if err := service.ResetLocalSetup(context.Background(), token, repo); err == nil {
				t.Fatal("unsafe reset succeeded")
			}
			if _, err := os.Stat(cli.ConfigPath(service.home)); err != nil {
				t.Fatal("configuration changed after rejected reset")
			}
			preservedRepo := repo
			if test == "stale path" {
				preservedRepo = cli.RepoDir(service.home)
			}
			if _, err := os.Stat(preservedRepo); err != nil {
				t.Fatal("clone changed after rejected reset")
			}
		})
	}
}

func TestResetRollsBackWhenCredentialDeletionFails(t *testing.T) {
	service := resetFixture(t)
	if err := auth.SaveMetadata(cli.AuthMetadataPath(service.home), auth.Metadata{
		Active: auth.Account{ID: 123},
	}); err != nil {
		t.Fatal(err)
	}
	service.resetCredential = func(int64) error { return errors.New("keyring unavailable") }
	err := service.ResetLocalSetup(context.Background(), "RESET", cli.RepoDir(service.home))
	if err == nil {
		t.Fatal("credential deletion failure was hidden")
	}
	for _, path := range []string{cli.ConfigPath(service.home), cli.RepoDir(service.home), cli.AuthMetadataPath(service.home)} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("rollback lost %s: %v", path, err)
		}
	}
	if _, err := service.Snapshot(); err != nil {
		t.Fatalf("restored setup has no working snapshot: %v", err)
	}
	if service.Daemon().AutoTriggerOnRun {
		t.Fatal("failed reset scheduled an unrequested startup sync")
	}
}

func TestFailedResetPreservesPausedDaemon(t *testing.T) {
	service := resetFixture(t)
	service.Daemon().Scheduler.Pause()
	if err := auth.SaveMetadata(cli.AuthMetadataPath(service.home), auth.Metadata{
		Active: auth.Account{ID: 123},
	}); err != nil {
		t.Fatal(err)
	}
	service.resetCredential = func(int64) error { return errors.New("keyring unavailable") }
	if err := service.ResetLocalSetup(context.Background(), "RESET", cli.RepoDir(service.home)); err == nil {
		t.Fatal("credential deletion failure was hidden")
	}
	d := service.Daemon()
	if d == nil || d.Scheduler.State() != scheduler.StatePaused {
		t.Fatal("failed reset did not preserve paused synchronization")
	}
	started := make(chan struct{})
	d.Scheduler.Job = func() error { close(started); return nil }
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- service.Run(ctx, func(Snapshot) {}) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Error("recovered runner did not stop")
		}
	})
	if err := service.Resume(); err != nil {
		t.Fatal(err)
	}
	if err := service.Trigger(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("recovered daemon could not synchronize")
	}
}

func TestResetStopsIdleRunnerAndAllowsReconfiguration(t *testing.T) {
	service := resetFixture(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- service.Run(ctx, func(Snapshot) {}) }()
	deadline := time.Now().Add(time.Second)
	for {
		service.runMu.Lock()
		running := service.runDone != nil
		service.runMu.Unlock()
		if running {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("initial daemon did not enter runner")
		}
		time.Sleep(time.Millisecond)
	}
	if err := service.ResetLocalSetup(ctx, "RESET", cli.RepoDir(service.home)); err != nil {
		t.Fatal(err)
	}
	cfg := config.Default(nil)
	cfg.RepoURL = "git@github.com:example/new.git"
	cfg.FirstSync = config.FirstSyncPolicy{Strategy: config.FirstSyncStrategyChoose}
	if err := config.Save(cli.ConfigPath(service.home), cfg); err != nil {
		t.Fatal(err)
	}
	if err := service.StartConfigured(); err != nil {
		t.Fatal(err)
	}
	jobStarted := make(chan struct{})
	service.Daemon().Scheduler.Job = func() error { close(jobStarted); return nil }
	if err := service.Trigger(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-jobStarted:
	case <-time.After(time.Second):
		t.Fatal("new daemon did not run after reconfiguration")
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("restartable runner did not stop")
	}
}

func TestResetRejectsProtectedProviderDirectory(t *testing.T) {
	service := resetFixture(t)
	target := filepath.Join(service.home, "providers")
	if err := os.Rename(cli.RepoDir(service.home), target); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(cli.ConfigPath(service.home))
	if err != nil {
		t.Fatal(err)
	}
	cfg.RepoDir = target
	if err := config.Save(cli.ConfigPath(service.home), cfg); err != nil {
		t.Fatal(err)
	}
	if err := service.ResetLocalSetup(context.Background(), "RESET", target); err == nil {
		t.Fatal("reset deleted protected providers directory")
	}
	if _, err := os.Stat(target); err != nil {
		t.Fatal(err)
	}
}

func TestResetRefusesRunningSyncWithoutRemovingData(t *testing.T) {
	service := resetFixture(t)
	started, release := make(chan struct{}), make(chan struct{})
	service.Daemon().Scheduler.Job = func() error { close(started); <-release; return nil }
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- service.Run(ctx, func(Snapshot) {}) }()
	t.Cleanup(func() {
		cancel()
		close(release)
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			t.Error("sync runner failed to stop")
		}
	})
	if err := service.Trigger(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("mocked job did not start")
	}
	if err := service.ResetLocalSetup(ctx, "RESET", cli.RepoDir(service.home)); err == nil {
		t.Fatal("active sync reset succeeded")
	}
	if _, err := os.Stat(cli.ConfigPath(service.home)); err != nil {
		t.Fatal("active sync reset removed configuration")
	}
}
