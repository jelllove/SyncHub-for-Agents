package cli

import (
	"fmt"
	"time"

	"github.com/qinqingxu/acsync/internal/config"
	"github.com/qinqingxu/acsync/internal/gitclient"
	"github.com/qinqingxu/acsync/internal/syncengine"
)

// RunCleanup purges expired trash from the repo and commits/pushes if anything
// was removed. It returns the purged repo-relative paths.
func RunCleanup(home string, now time.Time) ([]string, error) {
	cfg, err := config.Load(ConfigPath(home))
	if err != nil {
		return nil, err
	}
	grace := cfg.TrashGraceDays
	if grace <= 0 {
		grace = 30
	}

	repo := RepoDir(home)
	purged, err := syncengine.CleanupTrash(repo, now, grace)
	if err != nil {
		return nil, err
	}
	if len(purged) == 0 {
		return nil, nil
	}

	client := &gitclient.Client{Dir: repo}
	if err := client.AddAll(); err != nil {
		return purged, err
	}
	changed, err := client.HasChanges()
	if err != nil {
		return purged, err
	}
	if !changed {
		return purged, nil
	}
	if err := client.Commit(fmt.Sprintf("chore: purge %d expired trash entries", len(purged))); err != nil {
		return purged, err
	}
	if err := pushWithRebase(client, 3); err != nil {
		return purged, err
	}
	return purged, nil
}

// pushWithRebase pushes, and on failure pull-rebases and retries up to attempts.
func pushWithRebase(client *gitclient.Client, attempts int) error {
	var err error
	for i := 0; i < attempts; i++ {
		if err = client.Push(); err == nil {
			return nil
		}
		if rerr := client.PullRebase(); rerr != nil {
			return rerr
		}
	}
	return err
}
