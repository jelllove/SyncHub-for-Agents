package installplan

import (
	"context"
	"reflect"
	"testing"

	"github.com/qinqingxu/acsync/internal/resource"
)

var copilotListFixture = []byte(`
Installed plugins:
  • wiqd@wiqd (v0.6.0)
`)

func TestCopilotDiscoveryPreservesMarketplaceSource(t *testing.T) {
	runner := &fakeRunner{responses: map[string]RunResult{
		"copilot plugin list": {Stdout: copilotListFixture},
	}}
	adapter := NewCopilotAdapter(runner)
	declarations, err := adapter.Discover(context.Background(), resource.Spec{
		Installer: "copilot-plugin",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(declarations) != 1 ||
		declarations[0].Source != "wiqd@wiqd" ||
		declarations[0].Version != "0.6.0" {
		t.Fatalf("declarations = %#v", declarations)
	}
}

func TestCopilotOperationsUseFixedArgv(t *testing.T) {
	adapter := NewCopilotAdapter(&fakeRunner{})
	operations, err := adapter.Operations([]Declaration{{
		ID: "wiqd@wiqd", Adapter: "copilot-plugin",
		Source: "wiqd@wiqd", Version: "0.6.0", Enabled: true,
	}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(operations) != 1 ||
		operations[0].Executable != "copilot" ||
		!reflect.DeepEqual(operations[0].Args, []string{
			"plugin", "install", "wiqd@wiqd",
		}) {
		t.Fatalf("operations = %#v", operations)
	}
}
