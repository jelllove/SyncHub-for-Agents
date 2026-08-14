package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/qinqingxu/acsync/internal/config"
	"github.com/qinqingxu/acsync/internal/installplan"
	"github.com/qinqingxu/acsync/internal/portableconfig"
	"github.com/qinqingxu/acsync/internal/resource"
	"github.com/qinqingxu/acsync/internal/resourcecollect"
	"github.com/qinqingxu/acsync/internal/state"
	"github.com/qinqingxu/acsync/internal/syncengine"
)

type Status struct {
	RepoURL        string
	EnabledAgents  []string
	LastSync       time.Time
	PendingActions int
}

func RunStatus(home, goos string) (Status, error) {
	userHome, err := os.UserHomeDir()
	if err != nil {
		return Status{}, err
	}
	return runStatusWithUserHome(home, goos, userHome)
}

func runStatusWithUserHome(home, goos, userHome string) (result Status, retErr error) {
	cfg, err := config.Load(ConfigPath(home))
	if err != nil {
		return Status{}, err
	}
	providers, err := LoadProviders(home)
	if err != nil {
		return Status{}, err
	}
	resources, err := BuildResourceSpecs(cfg, providers, goos, userHome)
	if err != nil {
		return Status{}, err
	}
	codecs := portableconfig.BuiltinRegistry()
	inventory := installplan.NewBuiltinInventory(installplan.CommandRunner{})
	stageParent := filepath.Join(RepoDir(home), ".git", "acsync-stage")
	if err := os.MkdirAll(stageParent, 0o700); err != nil {
		return Status{}, fmt.Errorf("create status stage parent: %w", err)
	}
	specList := make([]resource.Spec, 0, len(resources))
	for _, spec := range resources {
		specList = append(specList, spec)
	}
	collected, err := resourcecollect.New(resourcecollect.Options{
		StageParent: stageParent,
		GOOS:        goos,
		UserHome:    userHome,
		Projector:   codecs,
		Inventory:   inventory,
	}).Collect(specList)
	if err != nil {
		return Status{}, err
	}
	defer func() {
		retErr = errors.Join(retErr, collected.Close())
	}()

	remote, err := syncengine.SnapshotRepo(RepoDir(home))
	if err != nil {
		return Status{}, err
	}
	remoteOwned, _, ownershipBlocked := syncengine.SplitRemoteSnapshot(remote, resources)
	validRemote, validationBlocked := syncengine.ValidateRemoteResources(
		RepoDir(home),
		remoteOwned,
		resources,
		codecs,
		goos,
		userHome,
	)
	base, err := state.Load(StatePath(home))
	if err != nil {
		return Status{}, err
	}
	blocked := append(append(append(
		[]resource.Issue{},
		collected.Blocked...),
		ownershipBlocked...),
		validationBlocked...)
	actions, _ := syncengine.PrepareResourceActions(
		base,
		collected.Snapshot,
		remote,
		validRemote,
		resources,
		collected.Skipped,
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
