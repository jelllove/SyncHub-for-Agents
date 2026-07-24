// Package provider models declarative agent adapters and loads builtin and
// user-supplied definitions.
package provider

import (
	"embed"
	"fmt"
	"os"
	"path"

	"gopkg.in/yaml.v3"
)

//go:embed builtin/*.yaml
var builtinFS embed.FS

// ConfigSpec describes which files of an agent are synced.
type ConfigSpec struct {
	Paths    map[string]string `yaml:"paths"`
	Include  []string          `yaml:"include"`
	Sessions []string          `yaml:"sessions"`
	Exclude  []string          `yaml:"exclude"`
}

// SecretSpec lists JSON key substrings that mark a file as secret.
type SecretSpec struct {
	KeyPatterns []string `yaml:"key_patterns"`
}

// Provider is a declarative adapter for one agent.
type Provider struct {
	Name    string     `yaml:"name"`
	Config  ConfigSpec `yaml:"config"`
	Secrets SecretSpec `yaml:"secrets"`
}

// Parse decodes a single provider definition from YAML bytes.
func Parse(data []byte) (Provider, error) {
	var p Provider
	if err := yaml.Unmarshal(data, &p); err != nil {
		return Provider{}, fmt.Errorf("provider: parse: %w", err)
	}
	if p.Name == "" {
		return Provider{}, fmt.Errorf("provider: missing name")
	}
	return p, nil
}

// Load reads and parses a provider definition from a file path.
func LoadFile(filename string) (Provider, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return Provider{}, err
	}
	return Parse(data)
}

// Builtins returns all embedded provider definitions.
func Builtins() ([]Provider, error) {
	entries, err := builtinFS.ReadDir("builtin")
	if err != nil {
		return nil, err
	}
	var out []Provider
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		data, err := builtinFS.ReadFile(path.Join("builtin", e.Name()))
		if err != nil {
			return nil, err
		}
		p, err := Parse(data)
		if err != nil {
			return nil, fmt.Errorf("builtin %s: %w", e.Name(), err)
		}
		out = append(out, p)
	}
	return out, nil
}
