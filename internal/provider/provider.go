// Package provider models declarative agent adapters and loads builtin and
// user-supplied definitions.
package provider

import (
	"bytes"
	"embed"
	"fmt"
	"os"
	"path"
	"strings"

	"github.com/qinqingxu/acsync/internal/resource"
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
	SchemaVersion int                    `yaml:"schema_version,omitempty"`
	Name          string                 `yaml:"name"`
	Config        ConfigSpec             `yaml:"config,omitempty"`
	Resources     []resource.Declaration `yaml:"resources,omitempty"`
	Secrets       SecretSpec             `yaml:"secrets"`
}

// Parse decodes a single provider definition from YAML bytes.
func Parse(data []byte) (Provider, error) {
	var p Provider
	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)
	if err := decoder.Decode(&p); err != nil {
		return Provider{}, fmt.Errorf("provider: parse: %w", err)
	}
	if strings.TrimSpace(p.Name) == "" {
		return Provider{}, fmt.Errorf("provider: missing name")
	}
	if p.Name == "_portable" {
		return Provider{}, fmt.Errorf("provider: name %q is reserved", p.Name)
	}
	if len(p.Resources) > 0 && p.hasLegacyConfig() {
		return Provider{}, fmt.Errorf("provider: mixed legacy config and typed resources for %q", p.Name)
	}
	if _, err := p.Declarations(); err != nil {
		return Provider{}, err
	}
	return p, nil
}

func (p Provider) hasLegacyConfig() bool {
	return len(p.Config.Paths) > 0 ||
		len(p.Config.Include) > 0 ||
		len(p.Config.Sessions) > 0 ||
		len(p.Config.Exclude) > 0
}

func (p Provider) Declarations() ([]resource.Declaration, error) {
	if p.Name == "_portable" {
		return nil, fmt.Errorf("provider name %q is reserved", p.Name)
	}
	if len(p.Resources) > 0 {
		out := make([]resource.Declaration, len(p.Resources))
		for index, declaration := range p.Resources {
			declaration = declaration.Normalized()
			if err := declaration.Validate(p.Name); err != nil {
				return nil, err
			}
			out[index] = declaration
		}
		return out, nil
	}

	var out []resource.Declaration
	if len(p.Config.Include) > 0 {
		out = append(out, resource.Declaration{
			ID:       "legacy-config",
			Category: resource.CategoryConfig,
			Paths:    p.Config.Paths,
			Include:  p.Config.Include,
			Exclude:  p.Config.Exclude,
			Strategy: resource.StrategyFileTree,
			Layout:   resource.LayoutLegacy,
		})
	}
	if len(p.Config.Sessions) > 0 {
		out = append(out, resource.Declaration{
			ID:       "legacy-sessions",
			Category: resource.CategorySessions,
			Paths:    p.Config.Paths,
			Include:  p.Config.Sessions,
			Exclude:  p.Config.Exclude,
			Strategy: resource.StrategyFileTree,
			Layout:   resource.LayoutLegacy,
		})
	}
	for index := range out {
		out[index] = out[index].Normalized()
		if err := out[index].Validate(p.Name); err != nil {
			return nil, err
		}
	}
	return out, nil
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
