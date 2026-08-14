package installplan

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

type Declaration struct {
	ID       string            `json:"id"`
	Adapter  string            `json:"adapter"`
	Source   string            `json:"source"`
	Version  string            `json:"version,omitempty"`
	Enabled  bool              `json:"enabled"`
	Settings map[string]string `json:"settings,omitempty"`
}

type Operation struct {
	ID         string   `json:"id"`
	Adapter    string   `json:"adapter"`
	Source     string   `json:"source"`
	Kind       string   `json:"kind"`
	Executable string   `json:"executable"`
	Args       []string `json:"args"`
	WorkingDir string   `json:"workingDir,omitempty"`
}

type Plan struct {
	ID         string            `json:"id"`
	Operations []Operation       `json:"operations"`
	Approved   bool              `json:"approved"`
	Errors     map[string]string `json:"errors,omitempty"`
}

func NewPlan(declarations []Declaration, operations []Operation) (Plan, error) {
	declarationCopy := append([]Declaration(nil), declarations...)
	operationCopy := append([]Operation(nil), operations...)
	sort.Slice(declarationCopy, func(i, j int) bool {
		return declarationIdentity(declarationCopy[i]) < declarationIdentity(declarationCopy[j])
	})
	sort.Slice(operationCopy, func(i, j int) bool {
		return operationIdentity(operationCopy[i]) < operationIdentity(operationCopy[j])
	})
	for _, declaration := range declarationCopy {
		if err := validateDeclaration(declaration); err != nil {
			return Plan{}, err
		}
	}
	for _, operation := range operationCopy {
		if err := ValidateOperation(operation); err != nil {
			return Plan{}, err
		}
	}
	canonical := struct {
		Declarations []Declaration `json:"declarations"`
		Operations   []Operation   `json:"operations"`
	}{
		Declarations: declarationCopy,
		Operations:   operationCopy,
	}
	data, err := json.Marshal(canonical)
	if err != nil {
		return Plan{}, fmt.Errorf("marshal canonical install plan: %w", err)
	}
	sum := sha256.Sum256(data)
	return Plan{
		ID:         hex.EncodeToString(sum[:]),
		Operations: operationCopy,
	}, nil
}

func validateDeclaration(declaration Declaration) error {
	if strings.TrimSpace(declaration.ID) == "" {
		return fmt.Errorf("install declaration is missing id")
	}
	if strings.TrimSpace(declaration.Adapter) == "" {
		return fmt.Errorf("install declaration %q is missing adapter", declaration.ID)
	}
	if strings.TrimSpace(declaration.Source) == "" {
		return fmt.Errorf("install declaration %q is missing source", declaration.ID)
	}
	if err := rejectShellSyntax(declaration.Source); err != nil {
		return fmt.Errorf("install declaration %q source: %w", declaration.ID, err)
	}
	allowedSettings := map[string]struct{}{}
	switch declaration.Adapter {
	case "claude-plugin":
		allowedSettings["marketplace"] = struct{}{}
	case "copilot-plugin":
	case "skill-dependencies":
		allowedSettings["manager"] = struct{}{}
		allowedSettings["manifest"] = struct{}{}
	default:
		return fmt.Errorf(
			"install declaration %q uses unsupported adapter %q",
			declaration.ID,
			declaration.Adapter,
		)
	}
	for key := range declaration.Settings {
		if _, allowed := allowedSettings[key]; !allowed {
			return fmt.Errorf(
				"install declaration %q uses unsupported setting %q",
				declaration.ID,
				key,
			)
		}
	}
	return nil
}

func declarationIdentity(declaration Declaration) string {
	return strings.Join([]string{
		declaration.Adapter,
		declaration.ID,
		declaration.Source,
		declaration.Version,
		fmt.Sprintf("%t", declaration.Enabled),
	}, "\x00")
}

func operationIdentity(operation Operation) string {
	values := []string{
		operation.Adapter,
		operation.ID,
		operation.Source,
		operation.Kind,
		operation.Executable,
		operation.WorkingDir,
	}
	values = append(values, operation.Args...)
	return strings.Join(values, "\x00")
}

func rejectShellSyntax(value string) error {
	if strings.ContainsAny(value, ";&|><`\r\n\x00") ||
		strings.Contains(value, "$(") {
		return fmt.Errorf("shell metacharacters are not allowed")
	}
	return nil
}
