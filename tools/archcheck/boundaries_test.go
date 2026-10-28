package archcheck

import (
	"errors"
	"fmt"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"testing/fstest"
)

const modulePrefix = "github.com/qinqingxu/synchub-for-agents/"

var foundationPackages = []string{
	"internal/config",
	"internal/pathresolver",
	"internal/resource",
	"internal/secret",
	"internal/state",
}

var syncDomainPackages = []string{
	"internal/conflict",
	"internal/gitclient",
	"internal/installplan",
	"internal/portableconfig",
	"internal/portablemerge",
	"internal/provider",
	"internal/repository",
	"internal/resourcecollect",
	"internal/syncengine",
}

var compositionPackages = []string{
	"internal/cli",
	"internal/daemon",
	"internal/desktop",
	"internal/settings",
	"internal/tray",
}

func inPackageTree(pkg string, roots []string) bool {
	for _, root := range roots {
		if pkg == root || strings.HasPrefix(pkg, root+"/") {
			return true
		}
	}
	return false
}

func violatedBoundary(source, imported string) string {
	if !strings.HasPrefix(imported, modulePrefix) {
		return ""
	}
	target := strings.TrimPrefix(imported, modulePrefix)
	foundation := inPackageTree(source, foundationPackages)
	if foundation && inPackageTree(target, syncDomainPackages) {
		return "foundations-below-sync-domain"
	}
	if (foundation || inPackageTree(source, syncDomainPackages)) &&
		inPackageTree(target, compositionPackages) {
		return "core-independent-of-composition"
	}
	return ""
}

func checkBoundaries(tree fs.FS) (int, error) {
	files := 0
	var failures []error
	positions := token.NewFileSet()
	walkErr := fs.WalkDir(tree, "internal", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return fmt.Errorf("%s: walk source tree: %w", name, err)
		}
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			return nil
		}
		files++
		data, err := fs.ReadFile(tree, name)
		if err != nil {
			return fmt.Errorf("%s: read source: %w", name, err)
		}
		// Parse imports without selecting a GOOS or evaluating build constraints.
		file, err := parser.ParseFile(positions, name, data, parser.ImportsOnly)
		if err != nil {
			return fmt.Errorf("%s: parse imports: %w", name, err)
		}
		for _, spec := range file.Imports {
			imported, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				return fmt.Errorf("%s: decode import path: %w", name, err)
			}
			source := path.Dir(name)
			if boundary := violatedBoundary(source, imported); boundary != "" {
				failures = append(failures, fmt.Errorf(
					"%s: boundary %q: %s must not import %s",
					positions.Position(spec.Pos()), boundary, source, imported,
				))
			}
		}
		return nil
	})
	if walkErr != nil {
		failures = append(failures, walkErr)
	} else if files == 0 {
		failures = append(failures, errors.New("internal: no production Go files found"))
	}
	return files, errors.Join(failures...)
}

func TestInternalDependencyBoundaries(t *testing.T) {
	files, err := checkBoundaries(os.DirFS(filepath.Join("..", "..")))
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("checked imports in %d production Go files, including build-tagged files", files)
}

func TestBoundaryPolicy(t *testing.T) {
	tests := []struct {
		name     string
		source   string
		imported string
		boundary string
	}{
		{"resource cannot compose CLI", "internal/resource", "internal/cli", "core-independent-of-composition"},
		{"config cannot compose daemon", "internal/config", "internal/daemon", "core-independent-of-composition"},
		{"secret cannot import desktop", "internal/secret", "internal/desktop", "core-independent-of-composition"},
		{"state cannot import tray", "internal/state", "internal/tray", "core-independent-of-composition"},
		{"paths cannot import settings", "internal/pathresolver", "internal/settings", "core-independent-of-composition"},
		{"sync cannot import CLI", "internal/syncengine", "internal/cli", "core-independent-of-composition"},
		{"projection cannot import desktop", "internal/portableconfig", "internal/desktop", "core-independent-of-composition"},
		{"collection cannot import daemon", "internal/resourcecollect", "internal/daemon", "core-independent-of-composition"},
		{"state stays below engine", "internal/state", "internal/syncengine", "foundations-below-sync-domain"},
		{"config stays below providers", "internal/config", "internal/provider", "foundations-below-sync-domain"},
		{"resource stays below collection", "internal/resource", "internal/resourcecollect", "foundations-below-sync-domain"},
		{"source subpackages inherit boundary", "internal/portablemerge/nested", "internal/cli", "core-independent-of-composition"},
		{"target subpackages inherit boundary", "internal/installplan", "internal/desktop/models", "core-independent-of-composition"},
		{"config uses resource enums", "internal/config", "internal/resource", ""},
		{"collection uses snapshots", "internal/resourcecollect", "internal/state", ""},
		{"collection uses secret scanning", "internal/resourcecollect", "internal/secret", ""},
		{"merging uses portable codecs", "internal/portablemerge", "internal/portableconfig", ""},
		{"engine uses approved installs", "internal/syncengine", "internal/installplan", ""},
		{"daemon wires CLI runners", "internal/daemon", "internal/cli", ""},
		{"desktop uses daemon", "internal/desktop", "internal/daemon", ""},
		{"similar target name is not a subtree", "internal/resource", "internal/client", ""},
		{"similar source name is not a subtree", "internal/resourceextra", "internal/cli", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			name := tt.source + "/fixture.go"
			imported := modulePrefix + tt.imported
			tree := fstest.MapFS{
				name: {Data: []byte(fmt.Sprintf("package fixture\nimport _ %q\n", imported))},
			}
			_, err := checkBoundaries(tree)
			if tt.boundary == "" {
				if err != nil {
					t.Fatalf("allowed import rejected: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("forbidden import was accepted")
			}
			for _, want := range []string{name + ":2:", tt.boundary, imported} {
				if !strings.Contains(err.Error(), want) {
					t.Errorf("error %q must identify %q", err, want)
				}
			}
		})
	}
}

func TestBuildTaggedImportsAndTestExclusion(t *testing.T) {
	tagged := "internal/syncengine/fixture_plan9.go"
	integration := "internal/syncengine/fixture_test.go"
	tree := fstest.MapFS{
		"internal/resource/model.go": {Data: []byte("package resource\n")},
		tagged: {Data: []byte("//go:build plan9 && archcheck_fixture\n\npackage syncengine\nimport (\n" +
			"desktop \"" + modulePrefix + "internal/desktop\"\n" +
			"_ `" + modulePrefix + "internal/daemon`\n)\n")},
		integration:                         {Data: []byte("package syncengine_test\nimport \"" + modulePrefix + "internal/cli\"\n")},
		"internal/resource/invalid_test.go": {Data: []byte("not Go syntax")},
		"internal/resource/notes.txt":       {Data: []byte("not Go syntax")},
	}
	files, err := checkBoundaries(tree)
	if err == nil {
		t.Fatal("build-tagged forbidden imports were accepted")
	}
	if files != 2 || strings.Count(err.Error(), "core-independent-of-composition") != 2 {
		t.Fatalf("want two production files and two violations, got %d files: %v", files, err)
	}
	for _, want := range []string{tagged, modulePrefix + "internal/desktop", modulePrefix + "internal/daemon"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q must identify %q", err, want)
		}
	}
	if strings.Contains(err.Error(), integration) {
		t.Fatalf("integration test imports must be excluded: %v", err)
	}
	delete(tree, tagged)
	if files, err := checkBoundaries(tree); files != 1 || err != nil {
		t.Fatalf("test and non-Go files must be skipped; got %d production files: %v", files, err)
	}
}

type unreadableFS struct {
	fs.FS
	name string
}

func (f unreadableFS) Open(name string) (fs.File, error) {
	if name == f.name {
		return nil, fs.ErrPermission
	}
	return f.FS.Open(name)
}

func TestSourceErrorsFailTheCheck(t *testing.T) {
	name := "internal/resource/model.go"
	valid := fstest.MapFS{name: {Data: []byte("package resource\n")}}
	tests := []struct {
		name    string
		tree    fs.FS
		message string
		cause   error
	}{
		{"missing tree", fstest.MapFS{}, "internal: walk source tree", fs.ErrNotExist},
		{"empty tree", fstest.MapFS{"internal": {Mode: fs.ModeDir}}, "no production Go files", nil},
		{"unreadable directory", unreadableFS{valid, "internal/resource"}, "internal/resource: walk source tree", fs.ErrPermission},
		{"unreadable file", unreadableFS{valid, name}, name + ": read source", fs.ErrPermission},
		{"malformed imports even outside guarded packages", fstest.MapFS{
			"internal/updater/broken.go": {Data: []byte("package updater\nimport )\n")},
		}, "internal/updater/broken.go: parse imports", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := checkBoundaries(tt.tree)
			if err == nil || !strings.Contains(err.Error(), tt.message) {
				t.Fatalf("want error containing %q, got %v", tt.message, err)
			}
			if tt.cause != nil && !errors.Is(err, tt.cause) {
				t.Errorf("want wrapped cause %v, got %v", tt.cause, err)
			}
		})
	}
}
