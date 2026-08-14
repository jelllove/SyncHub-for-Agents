// Package cli implements acsync commands.
package cli

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/qinqingxu/acsync/internal/config"
	"github.com/qinqingxu/acsync/internal/pathresolver"
	"github.com/qinqingxu/acsync/internal/provider"
	"github.com/qinqingxu/acsync/internal/resource"
	"github.com/qinqingxu/acsync/internal/secret"
	"github.com/qinqingxu/acsync/internal/syncengine"
)

// Home returns the acsync home directory (~/.acsync).
func Home() (string, error) {
	h, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(h, ".acsync"), nil
}

// ConfigPath returns the config file path for a given home.
func ConfigPath(home string) string { return filepath.Join(home, "config.yaml") }

// RepoDir returns the local repo clone directory.
func RepoDir(home string) string { return filepath.Join(home, "repo") }

// StatePath returns the snapshot state file path.
func StatePath(home string) string { return filepath.Join(home, "state.json") }

// ProvidersDir returns the user provider directory.
func ProvidersDir(home string) string { return filepath.Join(home, "providers") }

// LogsDir returns the log directory.
func LogsDir(home string) string { return filepath.Join(home, "logs") }

// LoadProviders returns builtin providers merged with user YAML in
// ProvidersDir(home). User definitions with a duplicate name override builtins.
func LoadProviders(home string) ([]provider.Provider, error) {
	builtins, err := provider.Builtins()
	if err != nil {
		return nil, err
	}
	byName := map[string]provider.Provider{}
	order := []string{}
	for _, p := range builtins {
		if _, ok := byName[p.Name]; !ok {
			order = append(order, p.Name)
		}
		byName[p.Name] = p
	}

	entries, err := os.ReadDir(ProvidersDir(home))
	if err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		p, err := provider.LoadFile(filepath.Join(ProvidersDir(home), e.Name()))
		if err != nil {
			return nil, err
		}
		if _, ok := byName[p.Name]; !ok {
			order = append(order, p.Name)
		}
		byName[p.Name] = p
	}

	out := make([]provider.Provider, 0, len(order))
	for _, name := range order {
		out = append(out, byName[name])
	}
	return out, nil
}

// BuildSpecs resolves enabled providers into AgentSpecs for the given OS/home.
func BuildSpecs(cfg config.Config, providers []provider.Provider, goos, userHome string) (map[string]syncengine.AgentSpec, error) {
	specs := map[string]syncengine.AgentSpec{}
	for _, p := range providers {
		if !cfg.Agents[p.Name] {
			continue
		}
		raw, ok := p.Config.Paths[goos]
		if !ok || raw == "" {
			continue
		}
		root, err := pathresolver.ResolveFor(raw, goos, userHome)
		if err != nil {
			return nil, err
		}
		root = normalizeResolvedPath(goos, root)
		specs[p.Name] = syncengine.AgentSpec{
			Name:     p.Name,
			Root:     root,
			Include:  p.Config.Include,
			Sessions: p.Config.Sessions,
			Scanner:  secret.NewScanner(p.Config.Exclude, p.Secrets.KeyPatterns),
		}
	}
	return specs, nil
}

func BuildResourceSpecs(cfg config.Config, providers []provider.Provider, goos, userHome string) (map[string]resource.Spec, error) {
	specs := map[string]resource.Spec{}
	for _, p := range providers {
		if err := resource.ValidateIdentifier("provider name", p.Name); err != nil {
			return nil, err
		}
		declarations, err := p.Declarations()
		if err != nil {
			return nil, err
		}
		for _, declaration := range declarations {
			if err := resource.ValidateIdentifier("resource id", declaration.ID); err != nil {
				return nil, err
			}
			if err := resource.ValidateOptionalIdentifier("shared_as", declaration.SharedAs); err != nil {
				return nil, err
			}
			if !cfg.CategoryEnabled(p.Name, declaration.Category) {
				continue
			}
			raw, ok := declaration.Paths[goos]
			if !ok || strings.TrimSpace(raw) == "" {
				continue
			}
			root, err := pathresolver.ResolveFor(raw, goos, userHome)
			if err != nil {
				return nil, err
			}
			root = normalizeResolvedPath(goos, root)

			spec := resource.Spec{
				Key:         p.Name + "/" + declaration.ID,
				Provider:    p.Name,
				ID:          declaration.ID,
				Category:    declaration.Category,
				Strategy:    declaration.Strategy,
				Layout:      declaration.Layout,
				Root:        root,
				Targets:     []string{root},
				Include:     append([]string(nil), declaration.Include...),
				Exclude:     append([]string(nil), declaration.Exclude...),
				Transformer: declaration.Transformer,
				Installer:   declaration.Installer,
				SharedAs:    declaration.SharedAs,
				KeyPatterns: append([]string(nil), p.Secrets.KeyPatterns...),
			}

			if declaration.SharedAs == "" {
				specs[spec.Key] = spec
				continue
			}

			key := "common/" + declaration.SharedAs
			if existing, ok := specs[key]; ok {
				if existing.Strategy != declaration.Strategy {
					return nil, conflictError(key, "strategy", string(existing.Strategy), string(declaration.Strategy))
				}
				if existing.Transformer != declaration.Transformer {
					return nil, conflictError(key, "transformer", existing.Transformer, declaration.Transformer)
				}
				existing.Targets = appendUnique(existing.Targets, root)
				existing.KeyPatterns = appendUnique(existing.KeyPatterns, p.Secrets.KeyPatterns...)
				specs[key] = existing
				continue
			}

			spec.Key = key
			spec.Provider = "common"
			spec.Targets = []string{root}
			specs[key] = spec
		}
	}

	for _, custom := range cfg.CustomResources {
		if err := resource.ValidateIdentifier("resource id", custom.ID); err != nil {
			return nil, err
		}
		raw, ok := custom.Paths[goos]
		if !ok || strings.TrimSpace(raw) == "" {
			continue
		}
		root, err := pathresolver.ResolveFor(raw, goos, userHome)
		if err != nil {
			return nil, err
		}
		spec := resource.Spec{
			Key:      "custom/" + custom.ID,
			Provider: "custom",
			ID:       custom.ID,
			Category: custom.Category,
			Strategy: custom.Strategy,
			Layout:   resource.LayoutPortable,
			Root:     normalizeResolvedPath(goos, root),
			Include:  append([]string(nil), custom.Include...),
			Exclude:  append([]string(nil), custom.Exclude...),
		}
		if custom.Strategy == resource.StrategyStructuredMerge {
			spec.Transformer = "generic-safe"
		}
		if rawTarget, ok := custom.Targets[goos]; ok && strings.TrimSpace(rawTarget) != "" {
			target, err := pathresolver.ResolveFor(rawTarget, goos, userHome)
			if err != nil {
				return nil, err
			}
			spec.Targets = []string{normalizeResolvedPath(goos, target)}
		}
		specs[spec.Key] = spec
	}

	return specs, nil
}

func normalizeResolvedPath(goos, value string) string {
	if goos == "windows" {
		return value
	}
	return filepath.ToSlash(value)
}

func appendUnique(values []string, additions ...string) []string {
	seen := make(map[string]struct{}, len(values))
	for _, value := range values {
		seen[value] = struct{}{}
	}
	for _, value := range additions {
		if _, exists := seen[value]; exists {
			continue
		}
		values = append(values, value)
		seen[value] = struct{}{}
	}
	return values
}

func conflictError(key, field, left, right string) error {
	return &resourceConflictError{key: key, field: field, left: left, right: right}
}

type resourceConflictError struct {
	key   string
	field string
	left  string
	right string
}

func (e *resourceConflictError) Error() string {
	return "shared resource " + e.key + ": conflicting " + e.field + " (" + e.left + " != " + e.right + ")"
}
