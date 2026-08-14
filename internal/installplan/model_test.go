package installplan

import (
	"bytes"
	"encoding/json"
	"testing"
)

func TestDeclarationSerializationNeverContainsExecutionFields(t *testing.T) {
	data, err := json.Marshal(Declaration{
		ID:      "wiqd@wiqd",
		Adapter: "copilot-plugin",
		Source:  "wiqd@wiqd",
		Version: "0.6.0",
		Enabled: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range [][]byte{[]byte("executable"), []byte("args"), []byte("workingDir")} {
		if bytes.Contains(data, forbidden) {
			t.Fatalf("declaration contains execution field %q: %s", forbidden, data)
		}
	}
}

func TestNewPlanIDChangesWithOperationIdentity(t *testing.T) {
	declarations := []Declaration{{
		ID: "wiqd@wiqd", Adapter: "copilot-plugin", Source: "wiqd@wiqd", Enabled: true,
	}}
	operation := Operation{
		ID: "copilot:wiqd@wiqd", Adapter: "copilot-plugin",
		Source: "wiqd@wiqd", Kind: "install",
		Executable: "copilot", Args: []string{"plugin", "install", "wiqd@wiqd"},
	}

	first, err := NewPlan(declarations, []Operation{operation})
	if err != nil {
		t.Fatal(err)
	}
	operation.Source = "wiqd@other"
	operation.Args[2] = operation.Source
	second, err := NewPlan(declarations, []Operation{operation})
	if err != nil {
		t.Fatal(err)
	}
	if first.ID == second.ID {
		t.Fatal("plan ID did not change with operation source")
	}
}

func TestParseDeclarationsRejectsExecutionFields(t *testing.T) {
	_, err := ParseDeclarations([]byte(`[{
		"id":"wiqd@wiqd",
		"adapter":"copilot-plugin",
		"source":"wiqd@wiqd",
		"enabled":true,
		"executable":"powershell"
	}]`))
	if err == nil {
		t.Fatal("repository declaration accepted executable field")
	}
}

func TestParseDeclarationsRejectsExecutionSettings(t *testing.T) {
	_, err := ParseDeclarations([]byte(`[{
		"id":"wiqd@wiqd",
		"adapter":"copilot-plugin",
		"source":"wiqd@wiqd",
		"enabled":true,
		"settings":{"executable":"powershell"}
	}]`))
	if err == nil {
		t.Fatal("repository declaration accepted executable setting")
	}
}
