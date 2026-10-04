package desktop

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/qinqingxu/synchub-for-agents/internal/auth"
	"github.com/qinqingxu/synchub-for-agents/internal/cli"
	"github.com/qinqingxu/synchub-for-agents/internal/config"
	"github.com/qinqingxu/synchub-for-agents/internal/daemon"
	"github.com/qinqingxu/synchub-for-agents/internal/gitclient"
	"github.com/qinqingxu/synchub-for-agents/internal/scheduler"
)

type ResetPreview struct {
	RepoPath string `json:"repoPath"`
}

var resetArtifacts = []string{
	"config.yaml", "state.json", "base", "conflicts", "conflict-resolution",
	"install", "desktop", "auth.json", "aliases.json",
}

func (s *Service) ResetPreview() (ResetPreview, error) {
	s.operations.Lock()
	defer s.operations.Unlock()
	cfg, err := config.Load(cli.ConfigPath(s.home))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return ResetPreview{}, err
	}
	repo, err := s.resolveRepoDir(cfg)
	if err != nil {
		return ResetPreview{}, err
	}
	return ResetPreview{RepoPath: repo}, nil
}

func (s *Service) ResetLocalSetup(ctx context.Context, confirmation, confirmedRepo string) error {
	return s.resetLocalSetup(ctx, confirmation, confirmedRepo, "")
}

func (s *Service) resetLocalSetup(ctx context.Context, confirmation, confirmedRepo, onboardingRepo string) (resultErr error) {
	s.operations.Lock()
	defer s.operations.Unlock()
	if confirmation != "RESET" {
		return errors.New("type RESET to confirm removing local SyncHub setup")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	cfg, err := config.Load(cli.ConfigPath(s.home))
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	if errors.Is(err, os.ErrNotExist) {
		cfg.RepoURL = onboardingRepo
	}
	repo, err := s.resolveRepoDir(cfg)
	if err != nil {
		return err
	}
	if !samePath(repo, confirmedRepo) {
		return errors.New("local repository path changed; review reset again")
	}
	d := s.Daemon()
	wasPaused := d != nil && d.Scheduler.State() == scheduler.StatePaused
	if d != nil && !d.Scheduler.PauseIfIdle() {
		return errors.New("cannot reset while synchronization is running; wait for it to finish")
	}
	resumeOnError := true
	defer func() {
		if resumeOnError && d != nil && !wasPaused {
			d.Scheduler.Resume()
		}
	}()
	if err := s.validateResetClone(repo, cfg); err != nil {
		return err
	}
	metadata, err := auth.LoadMetadata(cli.AuthMetadataPath(s.home))
	if err != nil && !errors.Is(err, os.ErrNotExist) && !errors.Is(err, auth.ErrNotFound) {
		return fmt.Errorf("read reset credentials: %w", err)
	}
	for _, name := range resetArtifacts {
		if err := rejectResetLinks(filepath.Join(s.home, name)); err != nil {
			return err
		}
	}
	resumeOnError = false
	s.runMu.Lock()
	cancel, done := s.runCancel, s.runDone
	if cancel != nil {
		cancel()
	}
	s.runMu.Unlock()
	if done != nil {
		select {
		case <-done:
		case <-ctx.Done():
			return fmt.Errorf("wait for synchronization to stop: %w", ctx.Err())
		case <-time.After(5 * time.Second):
			return errors.New("timed out stopping synchronization; nothing was removed")
		}
	} else if d != nil {
		if err := d.Close(); err != nil {
			return err
		}
	}
	s.clearResetDaemon()
	restoreDaemon := true
	defer func() {
		if resultErr == nil || !restoreDaemon || d == nil {
			return
		}
		if err := s.startConfigured(); err != nil {
			resultErr = errors.Join(resultErr, fmt.Errorf("restore synchronization after failed reset: %w", err))
			return
		}
		recovered := s.Daemon()
		recovered.AutoTriggerOnRun = false
		if wasPaused {
			recovered.Scheduler.Pause()
		}
	}()
	s.previewMu.Lock()
	defer s.previewMu.Unlock()
	s.previewGeneration++
	if err := os.MkdirAll(s.home, 0o700); err != nil {
		return err
	}
	staging, err := os.MkdirTemp(s.home, ".reset-")
	if err != nil {
		return err
	}
	type move struct{ from, to string }
	var moved []move
	var repoStaging string
	rollback := func(cause error) error {
		restoreDaemon = true
		for i := len(moved) - 1; i >= 0; i-- {
			if err := os.Rename(moved[i].to, moved[i].from); err != nil {
				restoreDaemon = false
				cause = errors.Join(cause, fmt.Errorf("restore reset target %s from %s: %w", moved[i].from, moved[i].to, err))
			}
		}
		if removeErr := os.Remove(staging); removeErr != nil {
			cause = errors.Join(cause, fmt.Errorf("reset recovery data at %s: %w", staging, removeErr))
		}
		if repoStaging != "" {
			if removeErr := os.Remove(repoStaging); removeErr != nil {
				cause = errors.Join(cause, fmt.Errorf("reset clone recovery data at %s: %w", repoStaging, removeErr))
			}
		}
		return cause
	}
	targets := []string{repo}
	for _, name := range resetArtifacts {
		targets = append(targets, filepath.Join(s.home, name))
	}
	for index, target := range targets {
		if _, err := os.Lstat(target); errors.Is(err, os.ErrNotExist) {
			continue
		} else if err != nil {
			return rollback(err)
		}
		destination := filepath.Join(staging, fmt.Sprintf("%02d", index))
		if index == 0 {
			repoStaging, err = os.MkdirTemp(filepath.Dir(repo), ".synchub-reset-")
			if err != nil {
				return rollback(err)
			}
			destination = filepath.Join(repoStaging, "clone")
		}
		if err := os.Rename(target, destination); err != nil {
			return rollback(fmt.Errorf("stage reset target %s: %w", target, err))
		}
		moved = append(moved, move{target, destination})
		restoreDaemon = false
	}
	if metadata.Active.ID != 0 {
		deleteCredential := s.resetCredential
		if deleteCredential == nil {
			deleteCredential = auth.NewSystemStore().Forget
		}
		restoreDaemon = false
		if err := deleteCredential(metadata.Active.ID); err != nil && !errors.Is(err, auth.ErrNotFound) {
			return rollback(fmt.Errorf("remove SyncHub OAuth credential: %w", err))
		}
	}
	if repoStaging != "" {
		if err := os.RemoveAll(repoStaging); err != nil {
			return fmt.Errorf("clone cleanup failed; remaining clone at %s and setup at %s: %w", repoStaging, staging, err)
		}
	}
	if err := os.RemoveAll(staging); err != nil {
		return fmt.Errorf("reset cleanup failed; remaining data at %s: %w", staging, err)
	}
	s.mu.Lock()
	s.last = daemon.CycleResult{}
	s.progress = Progress{}
	s.successfulCycle = daemon.CycleResult{}
	s.noticeReview = noticeReview{}
	s.mu.Unlock()
	return nil
}

func (s *Service) clearResetDaemon() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.daemon = nil
	for _, observer := range s.stateObservers {
		if observer.unsubscribe != nil {
			observer.unsubscribe()
			observer.unsubscribe = nil
		}
	}
	select {
	case <-s.start:
	default:
	}
}

func (s *Service) validateResetClone(repo string, cfg config.Config) error {
	absolute, err := filepath.Abs(repo)
	if err != nil {
		return err
	}
	userHome, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	if filepath.Dir(absolute) == absolute || samePath(absolute, userHome) ||
		resetContains(absolute, s.home) {
		return fmt.Errorf("refusing unsafe reset repository path %s", repo)
	}
	if resetContains(s.home, absolute) && !samePath(absolute, cli.RepoDir(s.home)) {
		return fmt.Errorf("only the default repo directory can be reset inside SyncHub home: %s", repo)
	}
	for _, name := range []string{"providers", "logs", "updates", "recovery"} {
		protected := filepath.Join(s.home, name)
		if resetContains(absolute, protected) || resetContains(protected, absolute) {
			return fmt.Errorf("repository overlaps retained SyncHub directory %s", protected)
		}
	}
	if err := rejectResetLinks(absolute); err != nil {
		return err
	}
	providers, err := cli.LoadProviders(s.home)
	if err != nil {
		return err
	}
	names := make([]string, 0, len(providers))
	for _, provider := range providers {
		names = append(names, provider.Name)
	}
	safetyConfig := config.Default(names)
	safetyConfig.CustomResources = cfg.CustomResources
	specs, err := cli.BuildResourceSpecs(safetyConfig, providers, s.goos, userHome)
	if err != nil {
		return err
	}
	for _, spec := range specs {
		for _, source := range append([]string{spec.Root}, spec.Targets...) {
			if source != "" && (resetContains(absolute, source) ||
				(!samePath(source, userHome) && resetContains(source, absolute))) {
				return fmt.Errorf("repository overlaps agent resource %s", source)
			}
		}
	}
	info, err := os.Lstat(absolute)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("reset repository is not a directory: %s", repo)
	}
	entries, err := os.ReadDir(absolute)
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		return nil
	}
	gitInfo, err := os.Lstat(filepath.Join(absolute, ".git"))
	if err != nil || !gitInfo.IsDir() || gitInfo.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("reset requires a standalone Git clone at %s", repo)
	}
	// The dedicated default clone must remain resettable after changing setup.
	if !samePath(absolute, cli.RepoDir(s.home)) {
		client := &gitclient.Client{Dir: absolute}
		origin, exists, err := client.RemoteURL("origin")
		if err != nil {
			return err
		}
		if !exists || cfg.RepoURL == "" || origin != cfg.RepoURL {
			return errors.New("custom local clone origin does not match current settings; refusing reset")
		}
	}
	allowed := map[string]bool{
		".git": true, ".gitattributes": true, "manifest.json": true,
		"agents": true, ".trash": true,
	}
	for _, entry := range entries {
		if !allowed[entry.Name()] {
			return fmt.Errorf("clone contains unexpected entry %s; move it elsewhere before resetting", entry.Name())
		}
	}
	return filepath.WalkDir(absolute, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing linked reset path %s", path)
		}
		return nil
	})
}

func resetContains(parent, child string) bool {
	parent, parentErr := filepath.Abs(parent)
	child, childErr := filepath.Abs(child)
	if parentErr != nil || childErr != nil {
		return true
	}
	rel, err := filepath.Rel(parent, child)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func rejectResetLinks(path string) error {
	for current := filepath.Clean(path); ; current = filepath.Dir(current) {
		info, err := os.Lstat(current)
		if err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
		if err == nil && info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing linked reset path %s", current)
		}
		if filepath.Dir(current) == current {
			return nil
		}
	}
}
