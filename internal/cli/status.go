package cli

import (
	"os"
	"time"

	"github.com/qinqingxu/acsync/internal/config"
	"github.com/qinqingxu/acsync/internal/state"
	"github.com/qinqingxu/acsync/internal/syncengine"
)

// Status is a snapshot of the current sync situation.
type Status struct {
	RepoURL        string
	EnabledAgents  []string
	LastSync       time.Time
	PendingActions int
}

// RunStatus computes status for home without contacting the remote.
func RunStatus(home, goos string) (Status, error) {
	userHome, err := os.UserHomeDir()
	if err != nil {
		return Status{}, err
	}
	return runStatusWithUserHome(home, goos, userHome)
}

func runStatusWithUserHome(home, goos, userHome string) (Status, error) {
	cfg, err := config.Load(ConfigPath(home))
	if err != nil {
		return Status{}, err
	}
	providers, err := LoadProviders(home)
	if err != nil {
		return Status{}, err
	}
	specs, err := BuildSpecs(cfg, providers, goos, userHome)
	if err != nil {
		return Status{}, err
	}

	specList := make([]syncengine.AgentSpec, 0, len(specs))
	for _, s := range specs {
		specList = append(specList, s)
	}
	collected, err := syncengine.Collect(specList)
	if err != nil {
		return Status{}, err
	}

	remote, err := syncengine.SnapshotRepo(RepoDir(home))
	if err != nil {
		return Status{}, err
	}
	remoteBlocked, err := syncengine.ScanRemoteBlocked(RepoDir(home), specs, remote)
	if err != nil {
		return Status{}, err
	}
	base, err := state.Load(StatePath(home))
	if err != nil {
		return Status{}, err
	}

	blocked := append(append([]string{}, collected.Blocked...), remoteBlocked...)
	actions := syncengine.ReconcileWithBlocked(
		syncengine.FilterSnapshotForSpecs(base, specs),
		collected.Snapshot,
		syncengine.FilterSnapshotForSpecs(remote, specs),
		blocked,
	)

	var last time.Time
	if info, err := os.Stat(StatePath(home)); err == nil {
		last = info.ModTime()
	}

	return Status{
		RepoURL:        cfg.RepoURL,
		EnabledAgents:  cfg.EnabledAgents(),
		LastSync:       last,
		PendingActions: len(actions),
	}, nil
}
