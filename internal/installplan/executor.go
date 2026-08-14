package installplan

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

type RunResult struct {
	Stdout []byte
	Stderr []byte
}

type Runner interface {
	Run(ctx context.Context, executable string, args []string, workingDir string) (RunResult, error)
}

type CommandRunner struct{}

func (CommandRunner) Run(
	ctx context.Context,
	executable string,
	args []string,
	workingDir string,
) (RunResult, error) {
	command := exec.CommandContext(ctx, executable, args...)
	command.Dir = workingDir
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	return RunResult{Stdout: stdout.Bytes(), Stderr: stderr.Bytes()}, err
}

type ExecutionResult struct {
	Executed []Operation
	Pending  []Operation
	Errors   map[string]string
}

type Executor struct {
	Runner    Runner
	Store     *Store
	BeforeRun func(Operation) error
}

func (e *Executor) Execute(ctx context.Context, plan Plan) (ExecutionResult, error) {
	if e.Runner == nil {
		return ExecutionResult{}, fmt.Errorf("install executor requires a runner")
	}
	if e.Store == nil {
		return ExecutionResult{}, fmt.Errorf("install executor requires a store")
	}
	for _, operation := range plan.Operations {
		if err := ValidateOperation(operation); err != nil {
			return ExecutionResult{}, err
		}
	}
	result := ExecutionResult{Errors: map[string]string{}}
	for _, operation := range plan.Operations {
		applied, err := e.Store.IsApplied(operation)
		if err != nil {
			return ExecutionResult{}, err
		}
		if applied {
			continue
		}
		approved, err := e.Store.approvalStatus(operation)
		if err != nil {
			return ExecutionResult{}, err
		}
		if !approved {
			result.Pending = append(result.Pending, operation)
			continue
		}
		if e.BeforeRun != nil {
			if err := e.BeforeRun(operation); err != nil {
				return ExecutionResult{}, err
			}
		}
		runResult, runErr := e.Runner.Run(
			ctx,
			operation.Executable,
			append([]string(nil), operation.Args...),
			operation.WorkingDir,
		)
		if runErr != nil {
			result.Pending = append(result.Pending, operation)
			message := runErr.Error()
			if stderr := strings.TrimSpace(string(runResult.Stderr)); stderr != "" {
				message += ": " + stderr
			}
			result.Errors[operation.ID] = message
			continue
		}
		if err := e.Store.MarkApplied(operation); err != nil {
			return ExecutionResult{}, err
		}
		result.Executed = append(result.Executed, operation)
	}

	if len(result.Pending) == 0 {
		if plan.ID != "" {
			if err := e.Store.ClearPending(plan.ID); err != nil {
				return ExecutionResult{}, err
			}
		}
		return result, nil
	}
	pendingPlan := plan
	pendingPlan.Operations = append([]Operation(nil), result.Pending...)
	pendingPlan.Approved = false
	pendingPlan.Errors = result.Errors
	if err := e.Store.SavePending(pendingPlan); err != nil {
		return ExecutionResult{}, err
	}
	return result, nil
}

func ValidateOperation(operation Operation) error {
	if strings.TrimSpace(operation.ID) == "" ||
		strings.TrimSpace(operation.Adapter) == "" ||
		strings.TrimSpace(operation.Source) == "" {
		return fmt.Errorf("install operation identity is incomplete")
	}
	if err := rejectShellSyntax(operation.Source); err != nil {
		return fmt.Errorf("install operation %q source: %w", operation.ID, err)
	}
	if err := rejectShellSyntax(operation.Executable); err != nil {
		return fmt.Errorf("install operation %q executable: %w", operation.ID, err)
	}
	for _, arg := range operation.Args {
		if err := rejectShellSyntax(arg); err != nil {
			return fmt.Errorf("install operation %q argv: %w", operation.ID, err)
		}
	}
	switch operation.Adapter {
	case "claude-plugin":
		return validateClaudeOperation(operation)
	case "copilot-plugin":
		return validateCopilotOperation(operation)
	case "skill-dependencies":
		return validateDependencyOperation(operation)
	default:
		return fmt.Errorf("install operation %q uses unsupported adapter %q", operation.ID, operation.Adapter)
	}
}

func validateClaudeOperation(operation Operation) error {
	if operation.Executable != "claude" ||
		len(operation.Args) < 5 ||
		operation.Args[0] != "plugin" ||
		operation.Args[1] != operation.Kind ||
		operation.Args[2] != operation.Source ||
		operation.Args[3] != "--scope" ||
		operation.Args[4] != "user" {
		return fmt.Errorf("install operation %q is not a trusted Claude command", operation.ID)
	}
	switch operation.Kind {
	case "install", "update":
		if len(operation.Args) != 5 {
			return fmt.Errorf("install operation %q has unexpected Claude argv", operation.ID)
		}
	case "uninstall":
		if len(operation.Args) != 6 || operation.Args[5] != "--keep-data" {
			return fmt.Errorf("install operation %q has unexpected Claude uninstall argv", operation.ID)
		}
	default:
		return fmt.Errorf("install operation %q has unsupported Claude action", operation.ID)
	}
	return nil
}

func validateCopilotOperation(operation Operation) error {
	if operation.Executable != "copilot" ||
		len(operation.Args) != 3 ||
		operation.Args[0] != "plugin" ||
		operation.Args[1] != operation.Kind ||
		operation.Args[2] != operation.Source {
		return fmt.Errorf("install operation %q is not a trusted Copilot command", operation.ID)
	}
	switch operation.Kind {
	case "install", "update", "uninstall":
		return nil
	default:
		return fmt.Errorf("install operation %q has unsupported Copilot action", operation.ID)
	}
}

func validateDependencyOperation(operation Operation) error {
	if operation.Kind != "dependencies" || operation.WorkingDir == "" {
		return fmt.Errorf("install operation %q is not a trusted dependency command", operation.ID)
	}
	for _, command := range dependencyCommands {
		if operation.Executable == command.executable &&
			equalStrings(operation.Args, command.args) {
			return nil
		}
	}
	return fmt.Errorf("install operation %q is not a trusted dependency command", operation.ID)
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
