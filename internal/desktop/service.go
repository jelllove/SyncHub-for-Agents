package desktop

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"sync"
	"time"

	"github.com/qinqingxu/acsync/internal/cli"
	"github.com/qinqingxu/acsync/internal/config"
	"github.com/qinqingxu/acsync/internal/daemon"
	"github.com/qinqingxu/acsync/internal/installplan"
	"github.com/qinqingxu/acsync/internal/provider"
	"github.com/qinqingxu/acsync/internal/scheduler"
	"github.com/qinqingxu/acsync/internal/syncengine"
)

type stateObserver struct {
	callback    func(scheduler.State)
	unsubscribe func()
}

// ErrNotConfigured indicates that onboarding must finish before sync controls
// can be used.
var ErrNotConfigured = errors.New("AgentConfigSync is not configured")

const (
	minSyncIntervalMinutes  = 1
	maxSyncIntervalMinutes  = 24 * 60
	minArchiveRetentionDays = 1
	maxArchiveRetentionDays = 365
)

// Service is the UI-independent desktop application facade.
type Service struct {
	home string
	goos string

	mu       sync.RWMutex
	daemon   *daemon.Daemon
	start    chan *daemon.Daemon
	last     daemon.CycleResult
	progress Progress

	nextObserverID         uint64
	stateObservers         map[uint64]*stateObserver
	nextProgressObserverID uint64
	progressObservers      map[uint64]func(Progress)
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
	last, err := newSummaryStore(home).loadCycle()
	if err != nil {
		return nil, fmt.Errorf("load desktop cycle summary: %w", err)
	}
	service.last = last
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
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.daemon != nil {
		return nil
	}
	d, err := daemon.New(s.home, s.goos)
	if err != nil {
		return err
	}
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
	if err := newSummaryStore(s.home).saveCycle(result); err != nil {
		result.Error = appendCycleError(
			result.Error,
			fmt.Errorf("save desktop cycle summary: %w", err),
		)
		result.NeedsAttention = true
	}
	s.mu.Lock()
	s.last = result
	s.mu.Unlock()
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

// Snapshot returns the current desktop-visible state.
func (s *Service) Snapshot() (Snapshot, error) {
	cfg, err := config.Load(cli.ConfigPath(s.home))
	if err != nil {
		if os.IsNotExist(err) {
			return s.unconfiguredSnapshot()
		}
		return Snapshot{}, err
	}
	providers, err := cli.LoadProviders(s.home)
	if err != nil {
		return Snapshot{}, err
	}
	status, err := cli.RunStatus(s.home, s.goos)
	if err != nil {
		return Snapshot{}, err
	}
	preview, err := s.preview(cfg, providers)
	if err != nil {
		return Snapshot{}, err
	}
	pending, err := installplan.NewStore(filepath.Join(s.home, "install")).Pending()
	if err != nil {
		return Snapshot{}, err
	}
	conflictRecords, err := conflictStore(s.home, nil).List()
	if err != nil {
		return Snapshot{}, err
	}

	s.mu.RLock()
	d := s.daemon
	last := s.last
	progress := s.progress
	s.mu.RUnlock()
	if d == nil {
		return Snapshot{}, ErrNotConfigured
	}
	next := time.Time{}
	if !last.FinishedAt.IsZero() {
		next = last.FinishedAt.Add(d.Scheduler.IntervalDuration())
	}
	stateValue := d.Scheduler.State().String()
	if last.NeedsAttention && (stateValue == "idle" || stateValue == "done") {
		stateValue = "error"
	}
	agents, err := makeAgents(providers, cfg, s.goos, preview)
	if err != nil {
		return Snapshot{}, err
	}
	return Snapshot{
		Configured:         true,
		State:              stateValue,
		RepositoryURL:      cfg.RepoURL,
		Platform:           s.goos,
		IntervalMinutes:    cfg.SyncIntervalMinutes,
		TrashGraceDays:     cfg.TrashGraceDays,
		Agents:             agents,
		LastSync:           status.LastSync,
		NextSync:           next,
		PendingActions:     status.PendingActions,
		BlockedFiles:       last.Blocked,
		LastError:          last.Error,
		Progress:           progress,
		Preview:            preview,
		CustomResources:    desktopCustomResources(cfg.CustomResources),
		PendingInstallPlan: desktopInstallPlan(pending),
		Conflicts:          desktopConflicts(conflictRecords),
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
	cfg := config.Config{
		RepoURL:             input.RepositoryURL,
		SyncIntervalMinutes: input.IntervalMinutes,
		TrashGraceDays:      input.TrashGraceDays,
		Agents:              input.Agents,
		Categories:          categories,
		CustomResources:     custom,
	}
	if err := config.Save(cli.ConfigPath(s.home), cfg); err != nil {
		return err
	}
	if err := s.StartConfigured(); err != nil {
		return err
	}
	s.Daemon().Scheduler.SetInterval(time.Duration(input.IntervalMinutes) * time.Minute)
	return nil
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
	d := s.Daemon()
	if d == nil {
		return ErrNotConfigured
	}
	d.Scheduler.Trigger()
	return nil
}

// Pause pauses scheduled and triggered syncs.
func (s *Service) Pause() error {
	d := s.Daemon()
	if d == nil {
		return ErrNotConfigured
	}
	d.Scheduler.Pause()
	return nil
}

// Resume resumes scheduled and triggered syncs.
func (s *Service) Resume() error {
	d := s.Daemon()
	if d == nil {
		return ErrNotConfigured
	}
	d.Scheduler.Resume()
	return nil
}
