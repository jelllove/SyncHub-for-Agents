package updater

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const CheckInterval = 6 * time.Hour

type Status struct {
	CurrentVersion string `json:"currentVersion"`
	LatestVersion  string `json:"latestVersion"`
	Phase          string `json:"phase"`
	Automatic      bool   `json:"automatic"`
	Supported      bool   `json:"supported"`
	ReleaseURL     string `json:"releaseURL"`
	Error          string `json:"error"`
	LastChecked    string `json:"lastChecked"`
}

type preferences struct {
	Automatic bool `json:"automatic"`
}

type Manager struct {
	mu        sync.Mutex
	operation sync.Mutex
	settings  sync.Mutex
	status    Status
	pending   *pendingUpdate
	restart   bool
	home      string
	client    *releaseClient
	notify    func(Status)
	wake      chan struct{}
	interval  time.Duration
	launch    func(pendingUpdate, string, bool, string) error
}

func New(home, currentVersion, goos, arch string, notify func(Status)) (*Manager, error) {
	prefs := preferences{Automatic: true}
	if err := loadJSON(filepath.Join(home, "update-settings.json"), &prefs); err != nil {
		return nil, fmt.Errorf("load update preferences: %w", err)
	}
	manager := &Manager{
		home:     home,
		client:   newReleaseClient(),
		notify:   notify,
		wake:     make(chan struct{}, 1),
		interval: CheckInterval,
		launch:   LaunchInstaller,
		status: Status{
			CurrentVersion: currentVersion,
			Phase:          "idle",
			Automatic:      prefs.Automatic,
			Supported:      goos == "windows" && arch == "amd64",
			ReleaseURL:     RepositoryURL + "/releases/latest",
		},
	}
	var result struct {
		Error string `json:"error"`
	}
	if err := loadJSON(manager.resultPath(), &result); err != nil {
		return nil, fmt.Errorf("load previous update result: %w", err)
	}
	if result.Error != "" {
		manager.status.Phase = "error"
		manager.status.Error = "Previous update failed: " + result.Error
	}
	if err := CleanupStaleHelpers(filepath.Join(home, "updates")); err != nil {
		log.Printf("clean up old update helpers: %v", err)
	}
	return manager, nil
}

func (m *Manager) resultPath() string {
	return filepath.Join(m.home, "update-result.json")
}

func (m *Manager) Status() Status {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.status
}

func (m *Manager) change(update func(*Status)) Status {
	m.mu.Lock()
	update(&m.status)
	status := m.status
	m.mu.Unlock()
	if m.notify != nil {
		m.notify(status)
	}
	return status
}

func (m *Manager) fail(err error) (Status, error) {
	status := m.change(func(s *Status) {
		s.Phase = "error"
		s.Error = err.Error()
	})
	return status, err
}

func (m *Manager) SetAutomatic(enabled bool) (Status, error) {
	m.settings.Lock()
	defer m.settings.Unlock()
	if err := saveJSON(filepath.Join(m.home, "update-settings.json"), preferences{Automatic: enabled}); err != nil {
		return m.Status(), fmt.Errorf("save update preferences: %w", err)
	}
	status := m.change(func(s *Status) { s.Automatic = enabled })
	if enabled {
		select {
		case m.wake <- struct{}{}:
		default:
		}
	}
	return status, nil
}

// Check also stages a verified installer on supported platforms. It never
// launches it: installation happens only after the desktop has shut down.
func (m *Manager) Check(ctx context.Context) (Status, error) {
	if !m.operation.TryLock() {
		return m.Status(), errors.New("an update check is already in progress")
	}
	defer m.operation.Unlock()
	m.mu.Lock()
	restarting := m.restart
	m.mu.Unlock()
	if restarting {
		return m.Status(), nil
	}
	current, err := parseVersion(m.Status().CurrentVersion)
	if err != nil {
		return m.fail(errors.New("automatic updates are disabled for development builds"))
	}
	m.change(func(s *Status) {
		s.Phase = "checking"
		s.Error = ""
	})
	release, err := m.client.latest(ctx)
	if err != nil {
		return m.fail(err)
	}
	latest, err := parseVersion(release.Tag)
	if err != nil {
		return m.fail(err)
	}
	m.change(func(s *Status) {
		s.LatestVersion = release.Tag
		s.LastChecked = time.Now().UTC().Format(time.RFC3339)
		s.ReleaseURL = RepositoryURL + "/releases/tag/" + release.Tag
	})
	if !latest.newerThan(current) {
		return m.change(func(s *Status) { s.Phase = "upToDate" }), nil
	}
	if !m.Status().Supported {
		return m.change(func(s *Status) { s.Phase = "available" }), nil
	}
	m.mu.Lock()
	pending := m.pending
	m.mu.Unlock()
	if pending != nil && pending.Version == release.Tag {
		if err := verifyInstaller(pending.Path, pending.Checksum); err == nil {
			return m.change(func(s *Status) { s.Phase = "ready" }), nil
		}
		// A missing or modified cache is re-downloaded, never installed.
	}
	m.change(func(s *Status) { s.Phase = "downloading" })
	download, err := m.client.download(ctx, release, filepath.Join(m.home, "updates"))
	if err != nil {
		return m.fail(err)
	}
	m.mu.Lock()
	m.pending = download
	m.mu.Unlock()
	if pending != nil {
		if err := os.Remove(pending.Path); err != nil && !errors.Is(err, os.ErrNotExist) {
			log.Printf("remove superseded update installer: %v", err)
		}
	}
	return m.change(func(s *Status) { s.Phase = "ready" }), nil
}

func (m *Manager) Run(ctx context.Context) {
	if _, err := parseVersion(m.Status().CurrentVersion); err != nil {
		return
	}
	check := func() {
		if !m.Status().Automatic {
			return
		}
		if _, err := m.Check(ctx); err != nil && ctx.Err() == nil {
			log.Printf("check for software updates: %v", err)
		}
	}
	if m.Status().Phase != "error" {
		check()
	}
	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			check()
		case <-m.wake:
			check()
		}
	}
}

func (m *Manager) RequestRestart() error {
	if !m.operation.TryLock() {
		return errors.New("wait for the update check to finish before restarting")
	}
	defer m.operation.Unlock()
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.status.Supported || m.status.Phase != "ready" || m.pending == nil {
		return errors.New("no verified update is ready to install")
	}
	if err := verifyInstaller(m.pending.Path, m.pending.Checksum); err != nil {
		return err
	}
	m.restart = true
	return nil
}

// ApplyOnExit must run after all application services have stopped. The helper
// opens a handle to this process before returning, then waits for its exit.
func (m *Manager) ApplyOnExit(executable string) error {
	m.operation.Lock()
	defer m.operation.Unlock()
	m.mu.Lock()
	pending, restart, status := m.pending, m.restart, m.status
	m.mu.Unlock()
	if pending == nil {
		return nil
	}
	if status.Phase != "ready" || (!status.Automatic && !restart) {
		if err := os.Remove(pending.Path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove unused update installer: %w", err)
		}
		m.mu.Lock()
		m.pending = nil
		m.mu.Unlock()
		return nil
	}
	if err := m.launch(*pending, executable, restart, m.resultPath()); err != nil {
		_, _ = m.fail(err)
		if saveErr := saveJSON(m.resultPath(), struct {
			Error string `json:"error"`
		}{err.Error()}); saveErr != nil {
			return errors.Join(err, fmt.Errorf("save update failure: %w", saveErr))
		}
		return err
	}
	return nil
}

func loadJSON(path string, target any) error {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	return json.Unmarshal(data, target)
}

func saveJSON(path string, value any) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".update-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	defer file.Close()
	if _, err := file.Write(data); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}
