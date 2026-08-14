package installplan

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/qinqingxu/acsync/internal/resource"
)

var claudeListFixture = []byte(`[
  {
    "pluginId":"code-review@claude-plugins-official",
    "name":"code-review",
    "marketplaceName":"claude-plugins-official",
    "version":"1.2.0",
    "enabled":true
  }
]`)

var claudeMarketplaceFixture = []byte(`[
  {
    "name":"claude-plugins-official",
    "source":{
      "source":"github",
      "repo":"anthropics/claude-plugins-official"
    }
  }
]`)

func TestClaudeDiscoveryMapsMarketplaceWithoutSerializingCommands(t *testing.T) {
	runner := &fakeRunner{responses: map[string]RunResult{
		"claude plugin list --json":             {Stdout: claudeListFixture},
		"claude plugin marketplace list --json": {Stdout: claudeMarketplaceFixture},
	}}
	adapter := NewClaudeAdapter(runner)
	declarations, err := adapter.Discover(context.Background(), resource.Spec{
		Installer: "claude-plugin",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(declarations) != 1 {
		t.Fatalf("declarations = %#v", declarations)
	}
	got := declarations[0]
	if got.Source != "code-review@claude-plugins-official" ||
		got.Settings["marketplace"] != "github:anthropics/claude-plugins-official" {
		t.Fatalf("declaration = %#v", got)
	}
}

func TestClaudeOperationsUseFixedArgv(t *testing.T) {
	adapter := NewClaudeAdapter(&fakeRunner{})
	operations, err := adapter.Operations([]Declaration{{
		ID: "code-review@claude-plugins-official", Adapter: "claude-plugin",
		Source: "code-review@claude-plugins-official", Version: "1.2.0", Enabled: true,
	}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(operations) != 1 ||
		operations[0].Executable != "claude" ||
		!reflect.DeepEqual(operations[0].Args, []string{
			"plugin", "install", "code-review@claude-plugins-official", "--scope", "user",
		}) {
		t.Fatalf("operations = %#v", operations)
	}
}

type invocation struct {
	executable string
	args       []string
	dir        string
}

type fakeRunner struct {
	responses map[string]RunResult
	errs      map[string]error
	calls     []invocation
}

func (f *fakeRunner) Run(_ context.Context, executable string, args []string, dir string) (RunResult, error) {
	f.calls = append(f.calls, invocation{
		executable: executable,
		args:       append([]string(nil), args...),
		dir:        dir,
	})
	key := executable
	for _, arg := range args {
		key += " " + arg
	}
	if err := f.errs[key]; err != nil {
		return RunResult{}, err
	}
	if result, ok := f.responses[key]; ok {
		return result, nil
	}
	return RunResult{}, nil
}

var errRunnerFailure = errors.New("runner failure")
