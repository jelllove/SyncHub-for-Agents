package installplan

import (
	"bufio"
	"context"
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/qinqingxu/acsync/internal/resource"
)

var copilotPluginLine = regexp.MustCompile(
	`^\s*(?:[•*-]\s*)?([A-Za-z0-9._-]+@[A-Za-z0-9._-]+)\s+\(v?([^)]+)\)\s*$`,
)

type CopilotAdapter struct {
	runner Runner
}

func NewCopilotAdapter(runner Runner) *CopilotAdapter {
	return &CopilotAdapter{runner: runner}
}

func (a *CopilotAdapter) Discover(ctx context.Context, _ resource.Spec) ([]Declaration, error) {
	if a.runner == nil {
		return nil, fmt.Errorf("Copilot adapter requires a runner")
	}
	result, err := a.runner.Run(ctx, "copilot", []string{"plugin", "list"}, "")
	if err != nil {
		return nil, commandError("copilot plugin list", result, err)
	}
	var declarations []Declaration
	scanner := bufio.NewScanner(strings.NewReader(string(result.Stdout)))
	for scanner.Scan() {
		match := copilotPluginLine.FindStringSubmatch(scanner.Text())
		if match == nil {
			continue
		}
		declaration := Declaration{
			ID:      match[1],
			Adapter: "copilot-plugin",
			Source:  match[1],
			Version: match[2],
			Enabled: true,
		}
		if err := validateDeclaration(declaration); err != nil {
			return nil, err
		}
		declarations = append(declarations, declaration)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("parse copilot plugin list: %w", err)
	}
	sort.Slice(declarations, func(i, j int) bool {
		return declarationIdentity(declarations[i]) < declarationIdentity(declarations[j])
	})
	return declarations, nil
}

func (a *CopilotAdapter) Operations(desired, current []Declaration) ([]Operation, error) {
	return pluginOperations("copilot-plugin", "copilot", desired, current)
}
