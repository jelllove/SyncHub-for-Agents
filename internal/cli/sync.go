package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/qinqingxu/synchub-for-agents/internal/config"
	"github.com/qinqingxu/synchub-for-agents/internal/conflict"
	"github.com/qinqingxu/synchub-for-agents/internal/installplan"
	"github.com/qinqingxu/synchub-for-agents/internal/portableconfig"
	"github.com/qinqingxu/synchub-for-agents/internal/state"
	"github.com/qinqingxu/synchub-for-agents/internal/syncengine"
)

// RunSync loads settings from home and performs one sync pass for goos.
func RunSync(home, goos string) (syncengine.Result, error) {
	return RunSyncWithProgress(home, goos, nil)
}

func RunSyncWithProgress(
	home, goos string,
	onProgress func(syncengine.Progress),
) (syncengine.Result, error) {
	userHome, err := os.UserHomeDir()
	if err != nil {
		return syncengine.Result{}, err
	}
	return runSyncWithUserHome(home, goos, userHome, onProgress)
}

func runSyncWithUserHome(
	home, goos, userHome string,
	onProgress ...func(syncengine.Progress),
) (syncengine.Result, error) {
	cfg, err := config.Load(ConfigPath(home))
	if err != nil {
		return syncengine.Result{}, err
	}
	providers, err := LoadProviders(home)
	if err != nil {
		return syncengine.Result{}, err
	}
	resources, err := BuildResourceSpecs(cfg, providers, goos, userHome)
	if err != nil {
		return syncengine.Result{}, err
	}
	repoDir, err := ResolveRepoDir(home, cfg, goos, userHome)
	if err != nil {
		return syncengine.Result{}, fmt.Errorf("resolve repository directory: %w", err)
	}
	if !cfg.FirstSync.Completed && strings.TrimSpace(cfg.FirstSync.Strategy) == config.FirstSyncStrategyChoose {
		return syncengine.Result{}, fmt.Errorf("first sync strategy is required before synchronizing")
	}

	client, err := NewGitClient(home, cfg.RepoURL, repoDir)
	if err != nil {
		return syncengine.Result{}, err
	}
	codecs := portableconfig.BuiltinRegistry()
	baseStore := state.NewBaseStore(home)
	conflictStore := conflict.NewStore(
		filepath.Join(home, "conflicts"),
		repoDir,
		syncengine.ConflictScanner(resources),
	)
	runner := installplan.CommandRunner{}
	inventory := installplan.NewBuiltinInventory(runner)
	installManager := installplan.NewManager(home, inventory, runner)
	eng := &syncengine.Engine{
		Git:            client,
		RepoDir:        repoDir,
		Home:           home,
		UserHome:       userHome,
		GOOS:           goos,
		StatePath:      StatePath(home),
		Resources:      resources,
		Codecs:         codecs,
		Base:           baseStore,
		Conflicts:      conflictStore,
		Inventory:      inventory,
		InstallManager: installManager,
		PushRetries:    5,
		Now:            time.Now,
		FirstSyncMode:  cfg.FirstSync.Strategy,
		FirstSyncRun:   !cfg.FirstSync.Completed,
	}
	if len(onProgress) > 0 {
		eng.OnProgress = onProgress[0]
	}
	result, err := eng.SyncOnce()
	if err != nil {
		return syncengine.Result{}, err
	}
	if !cfg.FirstSync.Completed {
		cfg.FirstSync.Completed = true
		if err := config.Save(ConfigPath(home), cfg); err != nil {
			return syncengine.Result{}, fmt.Errorf("mark first sync completed: %w", err)
		}
	}
	return result, nil
}
