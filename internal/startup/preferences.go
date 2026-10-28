package startup

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

func (manager *Manager) InitializeDefault() error {
	if manager.goos() != "windows" || manager.SkipDefault {
		return nil
	}
	manager.preferencesMu.Lock()
	defer manager.preferencesMu.Unlock()
	if manager.PreferencesPath == "" {
		return errors.New("Windows startup preferences path is not configured")
	}
	data, err := os.ReadFile(manager.PreferencesPath)
	if err == nil {
		var preference struct {
			Enabled *bool `json:"enabled"`
		}
		if err := json.Unmarshal(data, &preference); err != nil {
			return fmt.Errorf("load startup preferences: %w", err)
		}
		if preference.Enabled == nil {
			return errors.New("startup preferences are missing the enabled setting")
		}
		return nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("load startup preferences: %w", err)
	}
	return manager.setEnabled(true)
}

func (manager *Manager) SetEnabled(enabled bool) error {
	manager.preferencesMu.Lock()
	defer manager.preferencesMu.Unlock()
	if manager.goos() != "windows" {
		if enabled {
			return manager.Enable()
		}
		return manager.Disable()
	}
	if manager.PreferencesPath == "" {
		return errors.New("Windows startup preferences path is not configured")
	}
	return manager.setEnabled(enabled)
}

func (manager *Manager) setEnabled(enabled bool) error {
	previous, err := manager.IsEnabled()
	if err != nil {
		return fmt.Errorf("read startup registration: %w", err)
	}
	if previous != enabled {
		if err := manager.applyEnabled(enabled); err != nil {
			return fmt.Errorf("change startup registration: %w", err)
		}
	}
	if err := manager.savePreference(enabled); err != nil {
		if previous != enabled {
			if rollbackErr := manager.applyEnabled(previous); rollbackErr != nil {
				return errors.Join(err, fmt.Errorf("restore startup registration: %w", rollbackErr))
			}
		}
		return err
	}
	return nil
}

func (manager *Manager) applyEnabled(enabled bool) error {
	if enabled {
		return manager.Enable()
	}
	return manager.Disable()
}

func (manager *Manager) savePreference(enabled bool) error {
	data, err := json.Marshal(struct {
		Enabled bool `json:"enabled"`
	}{enabled})
	if err != nil {
		return fmt.Errorf("encode startup preferences: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(manager.PreferencesPath), 0o700); err != nil {
		return fmt.Errorf("create startup preferences directory: %w", err)
	}
	if err := writeFile(manager.PreferencesPath, data, 0o600); err != nil {
		return fmt.Errorf("save startup preferences: %w", err)
	}
	return nil
}
