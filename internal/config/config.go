// Package config persists user settings for synchub.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/qinqingxu/synchub-for-agents/internal/resource"
	"gopkg.in/yaml.v3"
)

const currentVersion = 3

type CustomResource struct {
	ID       string            `yaml:"id"`
	Category resource.Category `yaml:"category"`
	Paths    map[string]string `yaml:"paths"`
	Targets  map[string]string `yaml:"targets"`
	Include  []string          `yaml:"include,omitempty"`
	Exclude  []string          `yaml:"exclude,omitempty"`
	Strategy resource.Strategy `yaml:"strategy"`
}

// Config is the user-editable settings model.
type Config struct {
	Version             int                        `yaml:"version,omitempty"`
	RepoURL             string                     `yaml:"repo_url"`
	SyncIntervalMinutes int                        `yaml:"sync_interval_minutes"`
	TrashGraceDays      int                        `yaml:"trash_grace_days"`
	Agents              map[string]bool            `yaml:"agents"`
	Categories          map[string]map[string]bool `yaml:"categories,omitempty"`
	CustomResources     []CustomResource           `yaml:"custom_resources,omitempty"`
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

func (c Config) CategoryEnabled(provider string, category resource.Category) bool {
	if !c.Agents[provider] {
		return false
	}
	values, exists := c.Categories[provider]
	if !exists {
		return true
	}
	enabled, exists := values[string(category)]
	return !exists || enabled
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
	if c.Version < currentVersion {
		if _, configured := c.Agents["common"]; !configured {
			c.Agents["common"] = true
		}
		c.Version = currentVersion
	}
	return c, nil
}

// Save writes the config to path (creating parent dirs).
func Save(path string, c Config) error {
	if err := validateCustomResources(c.CustomResources); err != nil {
		return err
	}
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

func validateCustomResources(resources []CustomResource) error {
	seen := make(map[string]struct{}, len(resources))
	for _, item := range resources {
		if item.Strategy == resource.StrategyInstallManifest {
			return fmt.Errorf("custom resource %q: strategy %q is not supported", item.ID, item.Strategy)
		}

		declaration := resource.Declaration{
			ID:       item.ID,
			Category: item.Category,
			Paths:    item.Paths,
			Include:  item.Include,
			Exclude:  item.Exclude,
			Strategy: item.Strategy,
		}.Normalized()
		if item.Strategy == resource.StrategyStructuredMerge {
			declaration.Transformer = "generic-safe"
		}
		if err := declaration.Validate("custom"); err != nil {
			return err
		}
		if _, exists := seen[declaration.ID]; exists {
			return fmt.Errorf("duplicate custom resource id %q", declaration.ID)
		}
		seen[declaration.ID] = struct{}{}
		if err := validatePathMap(item.ID, "paths", item.Paths); err != nil {
			return err
		}
		if err := validatePathMap(item.ID, "targets", item.Targets); err != nil {
			return err
		}
	}
	return nil
}

func ValidateCustomResources(resources []CustomResource) error {
	return validateCustomResources(resources)
}

func validatePathMap(id, field string, values map[string]string) error {
	if len(values) == 0 {
		return fmt.Errorf("custom resource %q: missing %s", id, field)
	}
	for key, value := range values {
		if strings.TrimSpace(key) == "" {
			return fmt.Errorf("custom resource %q: %s has empty platform key", id, field)
		}
		if strings.TrimSpace(value) == "" {
			return fmt.Errorf("custom resource %q: %s[%s] is empty", id, field, key)
		}
	}
	return nil
}
