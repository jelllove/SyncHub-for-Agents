package cli

import (
	"os"
	"path/filepath"
	"time"

	"github.com/qinqingxu/acsync/internal/config"
	"github.com/qinqingxu/acsync/internal/conflict"
	"github.com/qinqingxu/acsync/internal/installplan"
	"github.com/qinqingxu/acsync/internal/portableconfig"
	"github.com/qinqingxu/acsync/internal/state"
	"github.com/qinqingxu/acsync/internal/syncengine"
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

	client, err := NewGitClient(home, cfg.RepoURL, RepoDir(home))
	if err != nil {
		return syncengine.Result{}, err
	}
	repoDir := RepoDir(home)
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
	}
	if len(onProgress) > 0 {
		eng.OnProgress = onProgress[0]
	}
	return eng.SyncOnce()
}
