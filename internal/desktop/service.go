package desktop

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/qinqingxu/synchub-for-agents/internal/cli"
	"github.com/qinqingxu/synchub-for-agents/internal/config"
	"github.com/qinqingxu/synchub-for-agents/internal/daemon"
	"github.com/qinqingxu/synchub-for-agents/internal/installplan"
	"github.com/qinqingxu/synchub-for-agents/internal/pathresolver"
	"github.com/qinqingxu/synchub-for-agents/internal/provider"
	"github.com/qinqingxu/synchub-for-agents/internal/repository"
	"github.com/qinqingxu/synchub-for-agents/internal/scheduler"
	"github.com/qinqingxu/synchub-for-agents/internal/syncengine"
)

type stateObserver struct {
	callback    func(scheduler.State)
	unsubscribe func()
}

// ErrNotConfigured indicates that onboarding must finish before sync controls
// can be used.
var ErrNotConfigured = errors.New("SyncHub is not configured")

const (
	minSyncIntervalMinutes  = 1
	maxSyncIntervalMinutes  = 24 * 60
	minArchiveRetentionDays = 1
	maxArchiveRetentionDays = 365
)

// Service is the UI-independent desktop application facade.
type Service struct {
	home            string
	goos            string
	operations      sync.Mutex
	runMu           sync.Mutex
	runCancel       context.CancelFunc
	runDone         chan struct{}
	resetCredential func(int64) error

	mu              sync.RWMutex
	daemon          *daemon.Daemon
	start           chan *daemon.Daemon
	last            daemon.CycleResult
	progress        Progress
	successfulCycle daemon.CycleResult
	noticeReview    noticeReview
	stateRevision   uint64

	nextObserverID         uint64
	stateObservers         map[uint64]*stateObserver
	nextProgressObserverID uint64
	progressObservers      map[uint64]func(Progress)
	previewCollector       func(context.Context, config.Config, []provider.Provider) (ResourcePreview, error)
	previewCoordinator     *previewCoordinator
	previewMu              sync.Mutex
	previewGeneration      uint64
}

// New creates a desktop service. A missing config is a valid first-run state.
func New(home, goos string) (*Service, error) {
	if goos == "" {
		goos = runtime.GOOS
	}
	service := &Service{
		home:              home,
		goos:              goos,
		start:             make(chan *daemon.Daemon, 1),
		stateObservers:    make(map[uint64]*stateObserver),
		progressObservers: make(map[uint64]func(Progress)),
	}
	service.previewCollector = service.collectPreview
	service.previewCoordinator = newPreviewCoordinator(service.refreshPreview)
	last, err := newSummaryStore(home).loadCycle()
	if err != nil {
		return nil, fmt.Errorf("load desktop cycle summary: %w", err)
	}
	service.last = last
	store := newSummaryStore(home)
	service.successfulCycle, err = store.loadSuccessfulCycle(last)
	if err != nil {
		return nil, fmt.Errorf("load successful desktop cycle: %w", err)
	}
	service.noticeReview, err = store.loadNoticeReview()
	if err != nil {
		return nil, fmt.Errorf("load sync notice acknowledgement: %w", err)
	}
	service.progress = cycleProgress(last)
	if _, err := os.Stat(cli.ConfigPath(home)); err != nil {
		if os.IsNotExist(err) {
			return service, nil
		}
		return nil, err
	}
	if err := service.StartConfigured(); err != nil {
		return nil, err
	}
	return service, nil
}

// StartConfigured creates the daemon after onboarding has persisted config.
// Calling it more than once is a no-op.
func (s *Service) StartConfigured() error {
	s.operations.Lock()
	defer s.operations.Unlock()
	return s.startConfigured()
}

func (s *Service) startConfigured() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.daemon != nil {
		return nil
	}
	cfg, err := config.Load(cli.ConfigPath(s.home))
	if err != nil {
		return err
	}
	d, err := daemon.New(s.home, s.goos)
	if err != nil {
		return err
	}
	d.AutoTriggerOnRun = !(cfg.FirstSync.Strategy == config.FirstSyncStrategyChoose && !cfg.FirstSync.Completed)
	d.OnCycle = s.recordCycle
	d.OnProgress = s.recordProgress
	s.daemon = d
	for _, observer := range s.stateObservers {
		observer.unsubscribe = d.Scheduler.Subscribe(observer.callback)
	}
	s.start <- d
	return nil
}

func (s *Service) recordProgress(update syncengine.Progress) {
	progress := Progress{
		Stage:            string(update.Stage),
		Label:            update.Label,
		Percentage:       update.Percentage,
		CompletedActions: update.CompletedActions,
		TotalActions:     update.TotalActions,
		BlockedFiles:     update.BlockedFiles,
		Pushed:           update.Pushed,
		Restored:         update.Restored,
		Reinstalled:      update.Reinstalled,
		Skipped:          update.Skipped,
		Conflicts:        update.Conflicts,
		PendingInstalls:  update.PendingInstalls,
		NeedsAttention:   update.NeedsAttention,
	}
	s.mu.Lock()
	s.stateRevision++
	progress.Revision = s.stateRevision
	if progress.Stage == "complete" {
		progress.NeedsAttention = cycleNeedsAttention(s.last, s.noticeReview)
		if !progress.NeedsAttention {
			progress.Label = "Synchronization complete"
		}
	}
	s.progress = progress
	observers := make([]func(Progress), 0, len(s.progressObservers))
	for _, observer := range s.progressObservers {
		observers = append(observers, observer)
	}
	s.mu.Unlock()
	for _, observer := range observers {
		observer(progress)
	}
}

func (s *Service) SubscribeProgress(callback func(Progress)) func() {
	s.mu.Lock()
	id := s.nextProgressObserverID
	s.nextProgressObserverID++
	s.progressObservers[id] = callback
	s.mu.Unlock()
	return func() {
		s.mu.Lock()
		delete(s.progressObservers, id)
		s.mu.Unlock()
	}
}

func (s *Service) recordCycle(result daemon.CycleResult) {
	s.mu.Lock()
	defer s.mu.Unlock()
	store := newSummaryStore(s.home)
	if result.Error == "" {
		if err := store.save("successful-cycle.json", result); err != nil {
			result.Error = appendCycleError(result.Error, fmt.Errorf("save successful desktop cycle: %w", err))
			result.NeedsAttention = true
		} else {
			s.successfulCycle = result
		}
	}
	if err := store.saveCycle(result); err != nil {
		result.Error = appendCycleError(
			result.Error,
			fmt.Errorf("save desktop cycle summary: %w", err),
		)
		result.NeedsAttention = true
	}
	s.last = result
	if result.Error == "" {
		s.progress = cycleProgress(result)
	}
}

func appendCycleError(existing string, err error) string {
	if err == nil {
		return existing
	}
	if existing == "" {
		return err.Error()
	}
	return errors.Join(errors.New(existing), err).Error()
}

// Daemon returns the configured daemon, or nil before onboarding.
func (s *Service) Daemon() *daemon.Daemon {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.daemon
}

// SubscribeState observes scheduler state even when registered before
// first-run configuration creates the daemon.
func (s *Service) SubscribeState(callback func(scheduler.State)) func() {
	s.mu.Lock()
	id := s.nextObserverID
	s.nextObserverID++
	observer := &stateObserver{callback: callback}
	if s.daemon != nil {
		observer.unsubscribe = s.daemon.Scheduler.Subscribe(callback)
	}
	s.stateObservers[id] = observer
	s.mu.Unlock()

	var once sync.Once
	return func() {
		once.Do(func() {
			s.mu.Lock()
			observer := s.stateObservers[id]
			delete(s.stateObservers, id)
			s.mu.Unlock()
			if observer != nil && observer.unsubscribe != nil {
				observer.unsubscribe()
			}
		})
	}
}

// Close releases daemon resources.
func (s *Service) Close() error {
	s.mu.RLock()
	d := s.daemon
	s.mu.RUnlock()
	if d == nil {
		return nil
	}
	return d.Close()
}

func (s *Service) nextStateRevision() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stateRevision++
	return s.stateRevision
}

// Snapshot reserves its revision before reading state. Snapshots and progress
// share one stream, so a slow capture cannot overtake a newer published update.
func (s *Service) Snapshot() (Snapshot, error) {
	revision := s.nextStateRevision()
	cfg, err := config.Load(cli.ConfigPath(s.home))
	if err != nil {
		if os.IsNotExist(err) {
			snapshot, err := s.unconfiguredSnapshot()
			snapshot.Revision = revision
			return snapshot, err
		}
		return Snapshot{}, err
	}
	repoPath, err := s.resolveRepoDir(cfg)
	if err != nil {
		return Snapshot{}, err
	}
	providers, err := cli.LoadProviders(s.home)
	if err != nil {
		return Snapshot{}, err
	}
	preview, err := newSummaryStore(s.home).loadPreview()
	if err != nil {
		return Snapshot{}, fmt.Errorf("load desktop preview summary: %w", err)
	}
	pending, err := installplan.NewStore(filepath.Join(s.home, "install")).Pending()
	if err != nil {
		return Snapshot{}, err
	}
	conflictRecords, resolution, err := conflictStore(s.home, nil).VisibleConflicts()
	if err != nil {
		return Snapshot{}, err
	}

	s.mu.RLock()
	d := s.daemon
	last := s.last
	progress := s.progress
	review := s.noticeReview
	successfulCycle := s.successfulCycle
	s.mu.RUnlock()
	if d == nil {
		return Snapshot{}, ErrNotConfigured
	}
	pendingActions := progress.TotalActions - progress.CompletedActions
	if pendingActions < 0 {
		pendingActions = 0
	}
	stateValue := d.Scheduler.State().String()
	attention := cycleNeedsAttention(last, review) || pending != nil || len(conflictRecords) > 0 ||
		(resolution != nil && resolution.Status != "completed")
	stateValue = terminalAttention(stateValue, attention)
	if progress.Stage == "complete" || progress.Stage == "" {
		progress.NeedsAttention = attention
		if attention {
			progress.Label = "Synchronization needs attention"
		} else if progress.Stage == "complete" {
			progress.Label = "Synchronization complete"
		}
	} else {
		progress.NeedsAttention = progress.NeedsAttention || attention
	}
	notices := cycleNotices(successfulCycle, review)
	agents, err := makeAgents(providers, cfg, s.goos, preview)
	if err != nil {
		return Snapshot{}, err
	}
	return Snapshot{
		Revision:           revision,
		Configured:         true,
		State:              stateValue,
		RepositoryURL:      cfg.RepoURL,
		Platform:           s.goos,
		IntervalMinutes:    cfg.SyncIntervalMinutes,
		TrashGraceDays:     cfg.TrashGraceDays,
		Agents:             agents,
		LastSync:           last.FinishedAt,
		NextSync:           d.Scheduler.NextRun(),
		PendingActions:     pendingActions,
		BlockedFiles:       last.Blocked,
		LastError:          last.Error,
		RepoPath:           repoPath,
		FirstSyncRequired:  cfg.FirstSync.Strategy == config.FirstSyncStrategyChoose && !cfg.FirstSync.Completed,
		SyncDiagnostic:     classifySyncDiagnostic(last.Error, repoPath),
		SyncNotices:        &notices,
		Progress:           progress,
		Preview:            preview,
		CustomResources:    desktopCustomResources(cfg.CustomResources),
		PendingInstallPlan: desktopInstallPlan(pending),
		Conflicts:          desktopConflicts(conflictRecords),
		ConflictResolution: desktopConflictResolution(resolution),
	}, nil
}

// OnboardingAgents returns provider settings without scanning resource files or
// querying repository status.
func (s *Service) OnboardingAgents() ([]Agent, error) {
	providers, err := cli.LoadProviders(s.home)
	if err != nil {
		return nil, err
	}
	cfg, err := config.Load(cli.ConfigPath(s.home))
	if err != nil {
		if !os.IsNotExist(err) {
			return nil, err
		}
		names := make([]string, 0, len(providers))
		for _, item := range providers {
			names = append(names, item.Name)
		}
		cfg = config.Default(names)
	}
	return makeAgents(providers, cfg, s.goos, ResourcePreview{})
}

func (s *Service) unconfiguredSnapshot() (Snapshot, error) {
	providers, err := cli.LoadProviders(s.home)
	if err != nil {
		return Snapshot{}, err
	}
	names := make([]string, 0, len(providers))
	for _, provider := range providers {
		names = append(names, provider.Name)
	}
	cfg := config.Default(names)
	agents, err := makeAgents(providers, cfg, s.goos, ResourcePreview{})
	if err != nil {
		return Snapshot{}, err
	}
	return Snapshot{
		Configured:      false,
		State:           "idle",
		RepoPath:        cli.RepoDir(s.home),
		IntervalMinutes: 10,
		TrashGraceDays:  30,
		Platform:        s.goos,
		Agents:          agents,
	}, nil
}

func makeAgents(
	providers []provider.Provider,
	cfg config.Config,
	goos string,
	preview ResourcePreview,
) ([]Agent, error) {
	previewItems := previewByIdentity(preview)
	agents := make([]Agent, 0, len(providers))
	for _, item := range providers {
		agent := Agent{Name: item.Name, Enabled: cfg.Agents[item.Name]}
		declarations, err := item.Declarations()
		if err != nil {
			return nil, err
		}
		excludes := map[string]struct{}{}
		for _, declaration := range declarations {
			for _, exclude := range declaration.Exclude {
				excludes[exclude] = struct{}{}
			}
			enabled := cfg.CategoryEnabled(item.Name, declaration.Category)
			resourceItem, exists := previewItems[resourceIdentity(
				item.Name,
				declaration.ID,
				string(declaration.Category),
			)]
			if exists {
				resourceItem.Enabled = enabled
				agent.Resources = append(agent.Resources, resourceItem)
				continue
			}
			source, supported := declaration.Paths[goos]
			resourceItem = ResourceCategory{
				Provider: item.Name, ID: declaration.ID,
				Category: string(declaration.Category),
				Enabled:  enabled, Supported: supported, Source: source,
				Target: source, Status: "disabled",
			}
			if !supported {
				resourceItem = unsupportedResource(
					item.Name,
					declaration.ID,
					declaration.Category,
					source,
					enabled,
				)
			}
			agent.Resources = append(agent.Resources, resourceItem)
		}
		for exclude := range excludes {
			agent.Exclude = append(agent.Exclude, exclude)
		}
		sort.Strings(agent.Exclude)
		agents = append(agents, agent)
	}
	sort.Slice(agents, func(i, j int) bool { return agents[i].Name < agents[j].Name })
	return agents, nil
}

// SaveSettings persists edits and applies the interval to the running daemon.
func (s *Service) SaveSettings(input SettingsInput) error {
	s.operations.Lock()
	defer s.operations.Unlock()
	if err := validateTimingSettings(input.IntervalMinutes, input.TrashGraceDays); err != nil {
		return err
	}
	existing, err := config.Load(cli.ConfigPath(s.home))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	categories := input.Categories
	if categories == nil {
		categories = existing.Categories
	}
	custom := make([]config.CustomResource, 0, len(input.CustomResources))
	if input.CustomResources == nil {
		custom = existing.CustomResources
	} else {
		for _, item := range input.CustomResources {
			custom = append(custom, item.toConfig())
		}
	}
	repositoryDir := strings.TrimSpace(input.RepositoryDir)
	if repositoryDir == "" {
		repositoryDir = existing.RepoDir
	}
	if err := validateRepoPathMode(input.RepoPathMode); err != nil {
		return err
	}
	resolvedExistingRepo, err := s.resolveRepoDir(existing)
	if err != nil {
		return err
	}
	resolvedRequestedRepo, err := s.resolveRepoDir(config.Config{RepoDir: repositoryDir})
	if err != nil {
		return err
	}
	if !samePath(resolvedExistingRepo, resolvedRequestedRepo) {
		if err := s.applyRepoDirChange(
			input.RepositoryURL,
			resolvedExistingRepo,
			resolvedRequestedRepo,
			strings.TrimSpace(input.RepoPathMode),
		); err != nil {
			return err
		}
	}
	firstSync := existing.FirstSync
	if input.FirstSyncChoiceRequired {
		firstSync.Strategy = strings.TrimSpace(input.FirstSyncStrategy)
		if firstSync.Strategy == "" {
			firstSync.Strategy = config.FirstSyncStrategyChoose
		}
		firstSync.Completed = false
	} else if strategy := strings.TrimSpace(input.FirstSyncStrategy); strategy != "" {
		firstSync.Strategy = strategy
		firstSync.Completed = false
	}
	if err := config.ValidateCustomResources(custom); err != nil {
		return err
	}
	if err := validateFirstSyncStrategy(firstSync.Strategy); err != nil {
		return err
	}
	if firstSync.Strategy == config.FirstSyncStrategyChoose && !input.FirstSyncChoiceRequired {
		firstSync.Completed = false
	} else if firstSync.Strategy == "" && !firstSync.Completed {
		firstSync.Completed = true
	}
	persistedRepoDir := repositoryDir
	defaultRepoDir := cli.RepoDir(s.home)
	if samePath(resolvedRequestedRepo, defaultRepoDir) {
		persistedRepoDir = ""
	} else {
		if userHome, homeErr := os.UserHomeDir(); homeErr == nil {
			if tokenized, ok := pathresolver.TokenizeHome(resolvedRequestedRepo, s.goos, userHome); ok {
				persistedRepoDir = tokenized
			} else {
				persistedRepoDir = resolvedRequestedRepo
			}
		} else {
			persistedRepoDir = resolvedRequestedRepo
		}
	}
	cfg := config.Config{
		RepoURL:             input.RepositoryURL,
		RepoDir:             persistedRepoDir,
		SyncIntervalMinutes: input.IntervalMinutes,
		TrashGraceDays:      input.TrashGraceDays,
		Agents:              input.Agents,
		Categories:          categories,
		CustomResources:     custom,
		FirstSync:           firstSync,
	}
	if err := config.Save(cli.ConfigPath(s.home), cfg); err != nil {
		return err
	}
	if err := s.invalidatePreview(); err != nil {
		return err
	}
	if err := s.startConfigured(); err != nil {
		return err
	}
	s.Daemon().Scheduler.SetInterval(time.Duration(input.IntervalMinutes) * time.Minute)
	if cfg.FirstSync.Strategy == config.FirstSyncStrategyChoose && !cfg.FirstSync.Completed {
		return nil
	}
	return s.trigger()
}

func validateTimingSettings(intervalMinutes, retentionDays int) error {
	if intervalMinutes < minSyncIntervalMinutes || intervalMinutes > maxSyncIntervalMinutes {
		return fmt.Errorf(
			"sync interval must be between %d and %d minutes",
			minSyncIntervalMinutes,
			maxSyncIntervalMinutes,
		)
	}
	if retentionDays < minArchiveRetentionDays || retentionDays > maxArchiveRetentionDays {
		return fmt.Errorf(
			"archive retention must be between %d and %d days",
			minArchiveRetentionDays,
			maxArchiveRetentionDays,
		)
	}
	return nil
}

// Trigger requests an immediate sync.
func (s *Service) Trigger() error {
	s.operations.Lock()
	defer s.operations.Unlock()
	return s.trigger()
}

func (s *Service) trigger() error {
	d := s.Daemon()
	if d == nil {
		return ErrNotConfigured
	}
	d.Scheduler.Trigger()
	return nil
}

// Pause pauses scheduled and triggered syncs.
func (s *Service) Pause() error {
	s.operations.Lock()
	defer s.operations.Unlock()
	d := s.Daemon()
	if d == nil {
		return ErrNotConfigured
	}
	d.Scheduler.Pause()
	return nil
}

// Resume resumes scheduled and triggered syncs.
func (s *Service) Resume() error {
	s.operations.Lock()
	defer s.operations.Unlock()
	d := s.Daemon()
	if d == nil {
		return ErrNotConfigured
	}
	d.Scheduler.Resume()
	return nil
}

func (s *Service) resolveRepoDir(cfg config.Config) (string, error) {
	userHome, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	repoPath, err := cli.ResolveRepoDir(s.home, cfg, s.goos, userHome)
	if err != nil {
		return "", fmt.Errorf("resolve repository directory: %w", err)
	}
	return repoPath, nil
}

func (s *Service) applyRepoDirChange(repositoryURL, currentRepo, nextRepo, mode string) error {
	if mode == "" {
		mode = "reclone"
	}
	switch mode {
	case "migrate":
		if err := os.MkdirAll(filepath.Dir(nextRepo), 0o755); err != nil {
			return err
		}
		if _, err := os.Stat(nextRepo); err == nil {
			return fmt.Errorf("new repository directory already exists: %s", nextRepo)
		}
		if err := os.Rename(currentRepo, nextRepo); err != nil {
			return fmt.Errorf("migrate repository directory: %w", err)
		}
		return nil
	case "reclone":
		client, err := cli.NewGitClient(s.home, repositoryURL, nextRepo)
		if err != nil {
			return err
		}
		setup := repository.Setup{Client: client}
		return setup.Initialize(repositoryURL, nextRepo)
	default:
		return fmt.Errorf("unsupported repository directory mode %q", mode)
	}
}

func validateRepoPathMode(mode string) error {
	trimmed := strings.TrimSpace(mode)
	if trimmed == "" || trimmed == "reclone" || trimmed == "migrate" {
		return nil
	}
	return fmt.Errorf("repository directory mode %q is not supported", mode)
}

func validateFirstSyncStrategy(strategy string) error {
	switch strategy {
	case "",
		config.FirstSyncStrategyChoose,
		config.FirstSyncStrategyUseCloud,
		config.FirstSyncStrategyMerge,
		config.FirstSyncStrategyUseLocal:
		return nil
	default:
		return fmt.Errorf("first sync strategy %q is not supported", strategy)
	}
}

func classifySyncDiagnostic(lastError, repoPath string) *SyncDiagnostic {
	if strings.Contains(lastError, "cannot pull with rebase") &&
		strings.Contains(lastError, "Please commit or stash them") {
		return &SyncDiagnostic{
			Code:     "git-rebase-dirty-worktree",
			Summary:  "Local repository has unstaged changes, so pull --rebase is blocked.",
			RepoPath: repoPath,
			Steps: []SyncFixStep{
				{Title: "Commit local changes", Command: "git add -A && git commit -m \"wip: local changes\" && git pull --rebase"},
				{Title: "Stash then pull", Command: "git stash push -u -m \"temp before sync\" && git pull --rebase && git stash pop"},
				{Title: "Discard local changes", Command: "git restore . && git pull --rebase", Warning: "Destructive: discards local edits"},
			},
		}
	}
	return nil
}

func samePath(left, right string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(filepath.Clean(left), filepath.Clean(right))
	}
	return filepath.Clean(left) == filepath.Clean(right)
}
