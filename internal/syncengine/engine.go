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

	if err := e.Git.PullRebase(); err != nil {
		return Result{}, fmt.Errorf("pull: %w", err)
	}

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

	base, err := state.Load(e.StatePath)
	if err != nil {
		return Result{}, fmt.Errorf("load state: %w", err)
	}

	actions := Reconcile(base, collected.Snapshot, remote)

	ap := &Applier{
		RepoDir: e.RepoDir,
		Specs:   e.Specs,
		Sources: collected.Sources,
		Now:     now(),
	}
	if err := ap.Apply(actions); err != nil {
		return Result{}, fmt.Errorf("apply: %w", err)
	}

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

	return Result{Actions: actions, Blocked: collected.Blocked, Pushed: pushed}, nil
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
