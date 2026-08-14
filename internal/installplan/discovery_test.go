package installplan

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

func TestSkillDependencyDiscoveryUsesOnlyTrustedManifests(t *testing.T) {
	root := t.TempDir()
	fixtures := map[string]string{
		"npm/package-lock.json":        "{}",
		"pnpm/pnpm-lock.yaml":          "lockfileVersion: 9",
		"yarn/yarn.lock":               "# lock",
		"uv/uv.lock":                   "version = 1",
		"pip/requirements.txt":         "requests==2.0",
		"go/go.mod":                    "module example.com/demo",
		"npm/node_modules/evil/go.mod": "module evil",
		"npm/.vscode-test/go.mod":      "module evil",
	}
	for rel, content := range fixtures {
		filename := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filename, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	declarations, err := DiscoverSkillDependencies(root)
	if err != nil {
		t.Fatal(err)
	}
	operations, err := OperationsForSkillDependencies(root, declarations)
	if err != nil {
		t.Fatal(err)
	}
	var commands []string
	for _, operation := range operations {
		commands = append(commands, operation.Executable+" "+joinArgs(operation.Args))
		if operation.WorkingDir == "" {
			t.Fatalf("operation has no working directory: %#v", operation)
		}
	}
	sort.Strings(commands)
	want := []string{
		"go mod download",
		"npm ci",
		"pnpm install --frozen-lockfile",
		"python -m pip install -r requirements.txt",
		"uv sync --frozen",
		"yarn install --immutable",
	}
	if !reflect.DeepEqual(commands, want) {
		t.Fatalf("commands = %#v, want %#v", commands, want)
	}
	for _, declaration := range declarations {
		if declaration.Source == "npm/node_modules/evil" ||
			declaration.Source == "npm/.vscode-test" {
			t.Fatalf("generated directory became a declaration: %#v", declaration)
		}
	}
}

func joinArgs(args []string) string {
	var result string
	for index, arg := range args {
		if index > 0 {
			result += " "
		}
		result += arg
	}
	return result
}
