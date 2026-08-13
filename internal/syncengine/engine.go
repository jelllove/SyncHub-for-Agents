package syncengine

import (
	"fmt"
	"time"

	"github.com/qinqingxu/acsync/internal/gitclient"
	"github.com/qinqingxu/acsync/internal/state"
)

// Engine orchestrates a single sync pass over a cloned repo workspace.
type Engine struct {
	Git         *gitclient.Client
	RepoDir     string
	StatePath   string
	Specs       map[string]AgentSpec
	PushRetries int
	Now         func() time.Time
	OnProgress  func(Progress)
}

// Result summarizes what a sync pass did.
type Result struct {
	Actions []Action
	Blocked []string
	Pushed  bool
}

// SyncOnce performs one full pull → reconcile → apply → push cycle.
func (e *Engine) SyncOnce() (Result, error) {
	now := time.Now
	if e.Now != nil {
		now = e.Now
	}

	e.progress(Progress{Stage: StagePulling, Label: "Pulling remote changes", Percentage: 10})
	if err := e.Git.PullRebase(); err != nil {
		return Result{}, fmt.Errorf("pull: %w", err)
	}

	e.progress(Progress{Stage: StageScanning, Label: "Scanning local files", Percentage: 30})
	remote, err := SnapshotRepo(e.RepoDir)
	if err != nil {
		return Result{}, fmt.Errorf("snapshot remote: %w", err)
	}

	specs := make([]AgentSpec, 0, len(e.Specs))
	for _, s := range e.Specs {
		specs = append(specs, s)
	}
	collected, err := Collect(specs)
	if err != nil {
		return Result{}, fmt.Errorf("collect: %w", err)
	}

	e.progress(Progress{
		Stage:        StageComparing,
		Label:        "Comparing changes",
		Percentage:   50,
		BlockedFiles: len(collected.Blocked),
	})
	base, err := state.Load(e.StatePath)
	if err != nil {
		return Result{}, fmt.Errorf("load state: %w", err)
	}

	actions := Reconcile(base, collected.Snapshot, remote)
	e.progress(Progress{
		Stage:        StageApplying,
		Label:        "Applying changes",
		Percentage:   65,
		TotalActions: len(actions),
		BlockedFiles: len(collected.Blocked),
	})

	ap := &Applier{
		RepoDir: e.RepoDir,
		Specs:   e.Specs,
		Sources: collected.Sources,
		Now:     now(),
	}
	if err := ap.Apply(actions); err != nil {
		return Result{}, fmt.Errorf("apply: %w", err)
	}

	e.progress(Progress{
		Stage:            StageUploading,
		Label:            "Uploading changes",
		Percentage:       85,
		CompletedActions: len(actions),
		TotalActions:     len(actions),
		BlockedFiles:     len(collected.Blocked),
	})
	pushed := false
	if err := e.Git.AddAll(); err != nil {
		return Result{}, fmt.Errorf("git add: %w", err)
	}
	changed, err := e.Git.HasChanges()
	if err != nil {
		return Result{}, err
	}
	if changed {
		msg := fmt.Sprintf("acsync sync %s", now().UTC().Format(time.RFC3339))
		if err := e.Git.Commit(msg); err != nil {
			return Result{}, fmt.Errorf("commit: %w", err)
		}
		if err := e.pushWithRetry(); err != nil {
			return Result{}, fmt.Errorf("push: %w", err)
		}
		pushed = true
	}

	newBase, err := SnapshotRepo(e.RepoDir)
	if err != nil {
		return Result{}, fmt.Errorf("snapshot new base: %w", err)
	}
	if err := state.Save(e.StatePath, newBase); err != nil {
		return Result{}, fmt.Errorf("save state: %w", err)
	}

	e.progress(Progress{
		Stage:            StageComplete,
		Label:            "Synchronization complete",
		Percentage:       100,
		CompletedActions: len(actions),
		TotalActions:     len(actions),
		BlockedFiles:     len(collected.Blocked),
		Pushed:           pushed,
	})
	return Result{Actions: actions, Blocked: collected.Blocked, Pushed: pushed}, nil
}

func (e *Engine) progress(update Progress) {
	if e.OnProgress != nil {
		e.OnProgress(update)
	}
}

func (e *Engine) pushWithRetry() error {
	retries := e.PushRetries
	if retries < 1 {
		retries = 1
	}
	var err error
	for i := 0; i < retries; i++ {
		if err = e.Git.Push(); err == nil {
			return nil
		}
		if perr := e.Git.PullRebase(); perr != nil {
			return perr
		}
	}
	return err
}
