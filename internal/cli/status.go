package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/qinqingxu/synchub-for-agents/internal/config"
	"github.com/qinqingxu/synchub-for-agents/internal/installplan"
	"github.com/qinqingxu/synchub-for-agents/internal/portableconfig"
	"github.com/qinqingxu/synchub-for-agents/internal/resource"
	"github.com/qinqingxu/synchub-for-agents/internal/resourcecollect"
	"github.com/qinqingxu/synchub-for-agents/internal/state"
	"github.com/qinqingxu/synchub-for-agents/internal/syncengine"
)

type Status struct {
	RepoURL        string    `json:"repositoryURL"`
	EnabledAgents  []string  `json:"enabledAgents"`
	LastSync       time.Time `json:"lastSync"`
	PendingActions int       `json:"pendingActions"`
}

type Inspection struct {
	Status
	Actions        []syncengine.Action
	Blocked        []string
	Skipped        int
	FirstSync      config.FirstSyncPolicy
	RequiresChoice bool
}

func Inspect(home, goos string) (Inspection, error) {
	userHome, err := os.UserHomeDir()
	if err != nil {
		return Inspection{}, err
	}
	return inspectWithUserHome(home, goos, userHome)
}

func RunStatus(home, goos string) (Status, error) {
	userHome, err := os.UserHomeDir()
	if err != nil {
		return Status{}, err
	}
	return runStatusWithUserHome(home, goos, userHome)
}

func runStatusWithUserHome(home, goos, userHome string) (Status, error) {
	result, err := inspectWithUserHome(home, goos, userHome)
	return result.Status, err
}

func inspectWithUserHome(home, goos, userHome string) (result Inspection, retErr error) {
	cfg, err := config.Load(ConfigPath(home))
	if err != nil {
		return Inspection{}, err
	}
	providers, err := LoadProviders(home)
	if err != nil {
		return Inspection{}, err
	}
	resources, err := BuildResourceSpecs(cfg, providers, goos, userHome)
	if err != nil {
		return Inspection{}, err
	}
	repoDir, err := ResolveRepoDir(home, cfg, goos, userHome)
	if err != nil {
		return Inspection{}, fmt.Errorf("resolve repository directory: %w", err)
	}
	codecs := portableconfig.BuiltinRegistry()
	inventory := installplan.NewBuiltinInventory(installplan.CommandRunner{})
	stageParent := filepath.Join(repoDir, ".git", "synchub-stage")
	if err := os.MkdirAll(stageParent, 0o700); err != nil {
		return Inspection{}, fmt.Errorf("create status stage parent: %w", err)
	}
	specList := make([]resource.Spec, 0, len(resources))
	for _, spec := range resources {
		specList = append(specList, spec)
	}
	sort.Slice(specList, func(i, j int) bool { return specList[i].Key < specList[j].Key })
	collected, err := resourcecollect.New(resourcecollect.Options{
		StageParent: stageParent,
		GOOS:        goos,
		UserHome:    userHome,
		Projector:   codecs,
		Inventory:   inventory,
	}).Collect(specList)
	if err != nil {
		return Inspection{}, err
	}
	defer func() {
		retErr = errors.Join(retErr, collected.Close())
	}()

	remote, err := syncengine.SnapshotRepo(repoDir)
	if err != nil {
		return Inspection{}, err
	}
	remoteOwned, _, ownershipBlocked := syncengine.SplitRemoteSnapshot(remote, resources)
	validRemote, validationBlocked := syncengine.ValidateRemoteResources(
		repoDir,
		remoteOwned,
		resources,
		codecs,
		goos,
		userHome,
	)
	base, err := state.Load(StatePath(home))
	if err != nil {
		return Inspection{}, err
	}
	blocked := append(append(append(
		[]resource.Issue{},
		collected.Blocked...),
		ownershipBlocked...),
		validationBlocked...)
	firstSyncMode := ""
	if !cfg.FirstSync.Completed {
		firstSyncMode = cfg.FirstSync.Strategy
	}
	actions, blockedPaths := syncengine.PreviewResourceActions(
		base,
		collected.Snapshot,
		remote,
		validRemote,
		resources,
		collected.Skipped,
		blocked,
		firstSyncMode,
	)

	var last time.Time
	if info, err := os.Stat(StatePath(home)); err == nil {
		last = info.ModTime()
	} else if !os.IsNotExist(err) {
		return Inspection{}, fmt.Errorf("inspect last synchronization state: %w", err)
	}
	enabledAgents := cfg.EnabledAgents()
	if enabledAgents == nil {
		enabledAgents = []string{}
	}
	return Inspection{
		Status: Status{
			RepoURL: cfg.RepoURL, EnabledAgents: enabledAgents,
			LastSync: last, PendingActions: len(actions),
		},
		Actions: actions, Blocked: blockedPaths, Skipped: len(collected.Skipped),
		FirstSync:      cfg.FirstSync,
		RequiresChoice: !cfg.FirstSync.Completed && cfg.FirstSync.Strategy == config.FirstSyncStrategyChoose,
	}, nil
}
