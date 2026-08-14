package installplan

import (
	"context"
	"testing"
)

func TestExecutorRunsOnlyApprovedOperations(t *testing.T) {
	store := NewStore(t.TempDir())
	approved := Operation{
		ID: "copilot:wiqd@wiqd", Adapter: "copilot-plugin",
		Source: "wiqd@wiqd", Kind: "install", Executable: "copilot",
		Args: []string{"plugin", "install", "wiqd@wiqd"},
	}
	pending := Operation{
		ID: "claude:review", Adapter: "claude-plugin",
		Source: "review@official", Kind: "install", Executable: "claude",
		Args: []string{"plugin", "install", "review@official", "--scope", "user"},
	}
	if err := store.Approve(approved); err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{}
	result, err := (&Executor{Runner: runner, Store: store}).Execute(
		context.Background(),
		Plan{ID: "plan-1", Operations: []Operation{approved, pending}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Executed) != 1 || result.Executed[0].ID != approved.ID ||
		len(result.Pending) != 1 || result.Pending[0].ID != pending.ID {
		t.Fatalf("execution result = %#v", result)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("runner calls = %#v", runner.calls)
	}
}

func TestExecutorRejectsShellMetacharactersWithoutRunning(t *testing.T) {
	store := NewStore(t.TempDir())
	operation := Operation{
		ID: "bad", Adapter: "copilot-plugin", Source: "wiqd@wiqd",
		Kind: "install", Executable: "copilot",
		Args: []string{"plugin", "install", "wiqd@wiqd; calc.exe"},
	}
	if err := store.Approve(operation); err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{}
	_, err := (&Executor{Runner: runner, Store: store}).Execute(
		context.Background(),
		Plan{ID: "plan-1", Operations: []Operation{operation}},
	)
	if err == nil {
		t.Fatal("malicious argv was accepted")
	}
	if len(runner.calls) != 0 {
		t.Fatalf("runner was called: %#v", runner.calls)
	}
}

func TestExecutorKeepsFailedCommandsPendingWithActionableError(t *testing.T) {
	store := NewStore(t.TempDir())
	operation := Operation{
		ID: "copilot:wiqd@wiqd", Adapter: "copilot-plugin",
		Source: "wiqd@wiqd", Kind: "install", Executable: "copilot",
		Args: []string{"plugin", "install", "wiqd@wiqd"},
	}
	if err := store.Approve(operation); err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{
		errs: map[string]error{"copilot plugin install wiqd@wiqd": errRunnerFailure},
	}
	result, err := (&Executor{Runner: runner, Store: store}).Execute(
		context.Background(),
		Plan{ID: "plan-1", Operations: []Operation{operation}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Pending) != 1 ||
		result.Errors[operation.ID] == "" {
		t.Fatalf("failed execution result = %#v", result)
	}
	pending, err := store.Pending()
	if err != nil {
		t.Fatal(err)
	}
	if pending == nil || pending.Errors[operation.ID] == "" {
		t.Fatalf("pending failure = %#v", pending)
	}
}
