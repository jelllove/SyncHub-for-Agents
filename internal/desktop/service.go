package desktop

import (
	"errors"
	"os"
	"runtime"
	"sort"
	"sync"
	"time"

	"github.com/qinqingxu/acsync/internal/cli"
	"github.com/qinqingxu/acsync/internal/config"
	"github.com/qinqingxu/acsync/internal/daemon"
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
	s.mu.Lock()
	s.last = result
	s.mu.Unlock()
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
	return Snapshot{
		Configured:      true,
		State:           d.Scheduler.State().String(),
		RepositoryURL:   cfg.RepoURL,
		IntervalMinutes: cfg.SyncIntervalMinutes,
		TrashGraceDays:  cfg.TrashGraceDays,
		Agents:          makeAgents(providers, cfg.Agents),
		LastSync:        status.LastSync,
		NextSync:        next,
		PendingActions:  status.PendingActions,
		BlockedFiles:    last.Blocked,
		LastError:       last.Error,
		Progress:        progress,
	}, nil
}

func (s *Service) unconfiguredSnapshot() (Snapshot, error) {
	providers, err := cli.LoadProviders(s.home)
	if err != nil {
		return Snapshot{}, err
	}
	enabled := make(map[string]bool, len(providers))
	for _, provider := range providers {
		enabled[provider.Name] = true
	}
	return Snapshot{
		Configured:      false,
		State:           "idle",
		IntervalMinutes: 10,
		TrashGraceDays:  30,
		Agents:          makeAgents(providers, enabled),
	}, nil
}

func makeAgents(providers []provider.Provider, enabled map[string]bool) []Agent {
	agents := make([]Agent, 0, len(providers))
	for _, provider := range providers {
		agents = append(agents, Agent{
			Name:    provider.Name,
			Enabled: enabled[provider.Name],
			Exclude: provider.Config.Exclude,
		})
	}
	sort.Slice(agents, func(i, j int) bool { return agents[i].Name < agents[j].Name })
	return agents
}

// SaveSettings persists edits and applies the interval to the running daemon.
func (s *Service) SaveSettings(input SettingsInput) error {
	if input.IntervalMinutes < 1 {
		return errors.New("sync interval must be at least one minute")
	}
	if input.TrashGraceDays < 0 {
		return errors.New("trash grace days cannot be negative")
	}
	cfg := config.Config{
		RepoURL:             input.RepositoryURL,
		SyncIntervalMinutes: input.IntervalMinutes,
		TrashGraceDays:      input.TrashGraceDays,
		Agents:              input.Agents,
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
