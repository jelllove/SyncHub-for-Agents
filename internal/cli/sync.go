package cli

import (
	"os"
	"time"

	"github.com/qinqingxu/acsync/internal/config"
	"github.com/qinqingxu/acsync/internal/syncengine"
)

// RunSync loads settings from home and performs one sync pass for goos.
func RunSync(home, goos string) (syncengine.Result, error) {
	userHome, err := os.UserHomeDir()
	if err != nil {
		return syncengine.Result{}, err
	}
	return runSyncWithUserHome(home, goos, userHome)
}

func runSyncWithUserHome(home, goos, userHome string) (syncengine.Result, error) {
	cfg, err := config.Load(ConfigPath(home))
	if err != nil {
		return syncengine.Result{}, err
	}
	providers, err := LoadProviders(home)
	if err != nil {
		return syncengine.Result{}, err
	}
	specs, err := BuildSpecs(cfg, providers, goos, userHome)
	if err != nil {
		return syncengine.Result{}, err
	}

	client, err := NewGitClient(home, cfg.RepoURL, RepoDir(home))
	if err != nil {
		return syncengine.Result{}, err
	}
	eng := &syncengine.Engine{
		Git:         client,
		RepoDir:     RepoDir(home),
		StatePath:   StatePath(home),
		Specs:       specs,
		PushRetries: 5,
		Now:         time.Now,
	}
	return eng.SyncOnce()
}
