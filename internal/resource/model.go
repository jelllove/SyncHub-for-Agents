package resource

import (
	"fmt"
	"strings"
)

type Category string

const (
	CategorySessions     Category = "sessions"
	CategoryConfig       Category = "config"
	CategoryInstructions Category = "instructions"
	CategorySkills       Category = "skills"
	CategoryPlugins      Category = "plugins"
)

type Strategy string

const (
	StrategyFileTree        Strategy = "file-tree"
	StrategyTextTree        Strategy = "text-tree"
	StrategyStructuredMerge Strategy = "structured-merge"
	StrategySourceTree      Strategy = "source-tree"
	StrategyInstallManifest Strategy = "install-manifest"
)

type Layout string

const (
	LayoutLegacy   Layout = "legacy"
	LayoutPortable Layout = "portable"
)

type Declaration struct {
	ID          string            `yaml:"id"`
	Category    Category          `yaml:"category"`
	Paths       map[string]string `yaml:"paths"`
	Include     []string          `yaml:"include,omitempty"`
	Exclude     []string          `yaml:"exclude,omitempty"`
	Strategy    Strategy          `yaml:"strategy"`
	Layout      Layout            `yaml:"layout,omitempty"`
	Transformer string            `yaml:"transformer,omitempty"`
	Installer   string            `yaml:"installer,omitempty"`
	SharedAs    string            `yaml:"shared_as,omitempty"`
}

type Issue struct {
	ResourceKey string `json:"resourceKey"`
	Path        string `json:"path"`
	Code        string `json:"code"`
	Message     string `json:"message"`
	Bytes       int64  `json:"bytes,omitempty"`
}

var knownTransformers = map[string]struct{}{
	"claude-settings":   {},
	"copilot-settings":  {},
	"gemini-settings":   {},
	"vscode-settings":   {},
	"vscode-mcp":        {},
	"cursor-settings":   {},
	"common-skill-lock": {},
	"generic-safe":      {},
}

var knownInstallers = map[string]struct{}{
	"claude-plugin":      {},
	"copilot-plugin":     {},
	"skill-dependencies": {},
}

func (d Declaration) Normalized() Declaration {
	if d.Layout == "" {
		d.Layout = LayoutPortable
	}
	return d
}

func (d Declaration) Validate(provider string) error {
	if provider == "_portable" {
		return fmt.Errorf("provider name %q is reserved", provider)
	}
	if err := ValidateIdentifier("provider name", provider); err != nil {
		return err
	}
	if strings.TrimSpace(d.ID) == "" {
		return fmt.Errorf("provider %s resource: missing id", provider)
	}
	if err := ValidateIdentifier("resource id", d.ID); err != nil {
		return err
	}
	if err := ValidateOptionalIdentifier("shared_as", d.SharedAs); err != nil {
		return err
	}
	switch d.Category {
	case CategorySessions, CategoryConfig, CategoryInstructions, CategorySkills, CategoryPlugins:
	default:
		return fmt.Errorf("provider %s resource %s: unsupported category %q", provider, d.ID, d.Category)
	}
	switch d.Strategy {
	case StrategyFileTree, StrategyTextTree, StrategyStructuredMerge, StrategySourceTree, StrategyInstallManifest:
	default:
		return fmt.Errorf("provider %s resource %s: unsupported strategy %q", provider, d.ID, d.Strategy)
	}
	switch d.Layout {
	case LayoutLegacy, LayoutPortable:
	default:
		return fmt.Errorf("provider %s resource %s: unsupported layout %q", provider, d.ID, d.Layout)
	}
	if d.Strategy == StrategyStructuredMerge && strings.TrimSpace(d.Transformer) == "" {
		return fmt.Errorf("provider %s resource %s: missing transformer", provider, d.ID)
	}
	if d.Transformer != "" {
		if _, ok := knownTransformers[d.Transformer]; !ok {
			return fmt.Errorf("provider %s resource %s: unknown transformer %q", provider, d.ID, d.Transformer)
		}
	}
	if d.Strategy == StrategyInstallManifest && strings.TrimSpace(d.Installer) == "" {
		return fmt.Errorf("provider %s resource %s: missing installer", provider, d.ID)
	}
	if d.Installer != "" {
		if _, ok := knownInstallers[d.Installer]; !ok {
			return fmt.Errorf("provider %s resource %s: unknown installer %q", provider, d.ID, d.Installer)
		}
	}
	return nil
}
