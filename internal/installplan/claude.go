package installplan

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/qinqingxu/synchub-for-agents/internal/resource"
)

type ClaudeAdapter struct {
	runner Runner
}

func NewClaudeAdapter(runner Runner) *ClaudeAdapter {
	return &ClaudeAdapter{runner: runner}
}

func (a *ClaudeAdapter) Discover(ctx context.Context, _ resource.Spec) ([]Declaration, error) {
	if a.runner == nil {
		return nil, fmt.Errorf("Claude adapter requires a runner")
	}
	listResult, err := a.runner.Run(ctx, "claude", []string{"plugin", "list", "--json"}, "")
	if err != nil {
		return nil, commandError("claude plugin list", listResult, err)
	}
	marketplaceResult, err := a.runner.Run(
		ctx,
		"claude",
		[]string{"plugin", "marketplace", "list", "--json"},
		"",
	)
	if err != nil {
		return nil, commandError("claude plugin marketplace list", marketplaceResult, err)
	}
	marketplaces, err := parseClaudeMarketplaces(marketplaceResult.Stdout)
	if err != nil {
		return nil, err
	}
	var plugins []struct {
		PluginID        string `json:"pluginId"`
		Name            string `json:"name"`
		MarketplaceName string `json:"marketplaceName"`
		Version         string `json:"version"`
		Enabled         bool   `json:"enabled"`
	}
	if err := json.Unmarshal(listResult.Stdout, &plugins); err != nil {
		return nil, fmt.Errorf("parse claude plugin list: %w", err)
	}
	declarations := make([]Declaration, 0, len(plugins))
	for _, plugin := range plugins {
		source := strings.TrimSpace(plugin.PluginID)
		if source == "" && plugin.Name != "" && plugin.MarketplaceName != "" {
			source = plugin.Name + "@" + plugin.MarketplaceName
		}
		declaration := Declaration{
			ID:      source,
			Adapter: "claude-plugin",
			Source:  source,
			Version: plugin.Version,
			Enabled: plugin.Enabled,
		}
		if marketplaceSource := marketplaces[plugin.MarketplaceName]; marketplaceSource != "" {
			declaration.Settings = map[string]string{"marketplace": marketplaceSource}
		}
		if err := validateDeclaration(declaration); err != nil {
			return nil, err
		}
		declarations = append(declarations, declaration)
	}
	sort.Slice(declarations, func(i, j int) bool {
		return declarationIdentity(declarations[i]) < declarationIdentity(declarations[j])
	})
	return declarations, nil
}

func (a *ClaudeAdapter) Operations(desired, current []Declaration) ([]Operation, error) {
	return pluginOperations("claude-plugin", "claude", desired, current)
}

func parseClaudeMarketplaces(data []byte) (map[string]string, error) {
	var entries []struct {
		Name   string          `json:"name"`
		Source json.RawMessage `json:"source"`
	}
	if err := json.Unmarshal(data, &entries); err != nil {
		return nil, fmt.Errorf("parse claude marketplace list: %w", err)
	}
	result := make(map[string]string, len(entries)+1)
	result["claude-plugins-official"] = "github:anthropics/claude-plugins-official"
	for _, entry := range entries {
		var source string
		if err := json.Unmarshal(entry.Source, &source); err == nil {
			result[entry.Name] = source
			continue
		}
		var structured struct {
			Source string `json:"source"`
			Repo   string `json:"repo"`
		}
		if err := json.Unmarshal(entry.Source, &structured); err != nil {
			return nil, fmt.Errorf("parse Claude marketplace %q source: %w", entry.Name, err)
		}
		switch {
		case structured.Source == "github" && structured.Repo != "":
			result[entry.Name] = "github:" + structured.Repo
		case structured.Source != "":
			result[entry.Name] = structured.Source
		case structured.Repo != "":
			result[entry.Name] = "github:" + structured.Repo
		}
	}
	return result, nil
}

func pluginOperations(adapter, executable string, desired, current []Declaration) ([]Operation, error) {
	currentBySource := make(map[string]Declaration, len(current))
	desiredBySource := make(map[string]Declaration, len(desired))
	for _, declaration := range current {
		if err := validateDeclaration(declaration); err != nil {
			return nil, err
		}
		currentBySource[declaration.Source] = declaration
	}
	var operations []Operation
	for _, declaration := range desired {
		if err := validateDeclaration(declaration); err != nil {
			return nil, err
		}
		desiredBySource[declaration.Source] = declaration
		if !declaration.Enabled {
			continue
		}
		installed, exists := currentBySource[declaration.Source]
		kind := ""
		switch {
		case !exists:
			kind = "install"
		case declaration.Version != "" &&
			installed.Version != "" &&
			declaration.Version != installed.Version:
			kind = "update"
		}
		if kind != "" {
			operations = append(operations, pluginOperation(
				adapter,
				executable,
				declaration,
				kind,
			))
		}
	}
	for _, declaration := range current {
		if _, exists := desiredBySource[declaration.Source]; exists {
			continue
		}
		operations = append(operations, pluginOperation(
			adapter,
			executable,
			declaration,
			"uninstall",
		))
	}
	sort.Slice(operations, func(i, j int) bool {
		return operationIdentity(operations[i]) < operationIdentity(operations[j])
	})
	return operations, nil
}

func pluginOperation(adapter, executable string, declaration Declaration, kind string) Operation {
	args := []string{"plugin", kind, declaration.Source}
	if adapter == "claude-plugin" {
		args = append(args, "--scope", "user")
		if kind == "uninstall" {
			args = append(args, "--keep-data")
		}
	}
	return Operation{
		ID:         adapter + ":" + kind + ":" + declaration.Source + "@" + declaration.Version,
		Adapter:    adapter,
		Source:     declaration.Source,
		Kind:       kind,
		Executable: executable,
		Args:       args,
	}
}

func commandError(name string, result RunResult, err error) error {
	if stderr := strings.TrimSpace(string(result.Stderr)); stderr != "" {
		return fmt.Errorf("%s: %w: %s", name, err, stderr)
	}
	return fmt.Errorf("%s: %w", name, err)
}
