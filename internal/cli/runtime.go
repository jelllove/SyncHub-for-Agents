// Package cli implements acsync commands.
package cli

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/qinqingxu/acsync/internal/config"
	"github.com/qinqingxu/acsync/internal/pathresolver"
	"github.com/qinqingxu/acsync/internal/provider"
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
		if goos != "windows" {
			root = filepath.ToSlash(root)
		}
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
