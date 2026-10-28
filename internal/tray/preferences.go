package tray

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

const TipPreferencesEvent = "tray-tip:preferences-changed"

type TipPreferences struct {
	operation   sync.Mutex
	mu          sync.Mutex
	path        string
	enabled     bool
	nextID      uint64
	subscribers map[uint64]func(bool)
}

func NewTipPreferences(path string) (*TipPreferences, error) {
	if path == "" {
		return nil, errors.New("tray tip preferences path is not configured")
	}
	prefs := &TipPreferences{path: path, enabled: true, subscribers: make(map[uint64]func(bool))}
	if err := validatePreferenceFile(path); err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return prefs, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load tray tip preferences: %w", err)
	}
	var stored struct {
		Enabled *bool `json:"enabled"`
	}
	if err := json.Unmarshal(data, &stored); err != nil {
		return nil, fmt.Errorf("decode tray tip preferences: %w", err)
	}
	if stored.Enabled == nil {
		return nil, errors.New("tray tip preferences are missing the enabled setting")
	}
	prefs.enabled = *stored.Enabled
	return prefs, nil
}

func (prefs *TipPreferences) Enabled() bool {
	prefs.mu.Lock()
	defer prefs.mu.Unlock()
	return prefs.enabled
}

func (prefs *TipPreferences) Subscribe(callback func(bool)) func() {
	prefs.mu.Lock()
	id := prefs.nextID
	prefs.nextID++
	prefs.subscribers[id] = callback
	prefs.mu.Unlock()
	return func() {
		prefs.mu.Lock()
		delete(prefs.subscribers, id)
		prefs.mu.Unlock()
	}
}

func (prefs *TipPreferences) SetEnabled(enabled bool) error {
	prefs.operation.Lock()
	defer prefs.operation.Unlock()
	if prefs.Enabled() == enabled {
		return nil
	}
	if err := saveTipPreference(prefs.path, enabled); err != nil {
		return err
	}
	prefs.mu.Lock()
	prefs.enabled = enabled
	callbacks := make([]func(bool), 0, len(prefs.subscribers))
	for _, callback := range prefs.subscribers {
		callbacks = append(callbacks, callback)
	}
	prefs.mu.Unlock()
	for _, callback := range callbacks {
		callback(enabled)
	}
	return nil
}

func validatePreferenceFile(path string) error {
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect tray tip preferences: %w", err)
	}
	if !info.Mode().IsRegular() {
		return errors.New("tray tip preferences must be a regular file")
	}
	return nil
}

func saveTipPreference(path string, enabled bool) (retErr error) {
	if err := validatePreferenceFile(path); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create tray tip preferences directory: %w", err)
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".tray-settings-*")
	if err != nil {
		return fmt.Errorf("create tray tip preferences file: %w", err)
	}
	defer func() {
		if err := os.Remove(file.Name()); err != nil && !errors.Is(err, os.ErrNotExist) {
			retErr = errors.Join(retErr, fmt.Errorf("clean tray tip preferences staging: %w", err))
		}
	}()
	data := []byte(fmt.Sprintf("{\"enabled\":%t}\n", enabled))
	writeErr := file.Chmod(0o600)
	if writeErr == nil {
		_, writeErr = file.Write(data)
	}
	if writeErr == nil {
		writeErr = file.Sync()
	}
	if err := errors.Join(writeErr, file.Close()); err != nil {
		return fmt.Errorf("write tray tip preferences: %w", err)
	}
	if err := os.Rename(file.Name(), path); err != nil {
		return fmt.Errorf("save tray tip preferences: %w", err)
	}
	return nil
}
