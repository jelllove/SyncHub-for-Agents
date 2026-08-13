// Package config persists user settings for acsync.
package config

import (
	"os"
	"path/filepath"
	"sort"

	"gopkg.in/yaml.v3"
)

const currentVersion = 1

// Config is the user-editable settings model.
type Config struct {
	Version             int             `yaml:"version,omitempty"`
	RepoURL             string          `yaml:"repo_url"`
	SyncIntervalMinutes int             `yaml:"sync_interval_minutes"`
	TrashGraceDays      int             `yaml:"trash_grace_days"`
	Agents              map[string]bool `yaml:"agents"`
}

// Default returns a Config with all provided agent names enabled.
func Default(agentNames []string) Config {
	agents := make(map[string]bool, len(agentNames))
	for _, n := range agentNames {
		agents[n] = true
	}
	return Config{
		Version:             currentVersion,
		SyncIntervalMinutes: 10,
		TrashGraceDays:      30,
		Agents:              agents,
	}
}

// EnabledAgents returns the sorted names of enabled agents.
func (c Config) EnabledAgents() []string {
	var out []string
	for name, on := range c.Agents {
		if on {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

// Load reads a config from path. A missing file is an error (run init first).
func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	var c Config
	if err := yaml.Unmarshal(data, &c); err != nil {
		return Config{}, err
	}
	if c.Agents == nil {
		c.Agents = map[string]bool{}
	}
	if c.Version < 1 {
		if _, configured := c.Agents["vscode-copilot"]; !configured {
			c.Agents["vscode-copilot"] = true
		}
		c.Version = 1
	}
	return c, nil
}

// Save writes the config to path (creating parent dirs).
func Save(path string, c Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	c.Version = currentVersion
	data, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
