package installplan

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

type dependencyCommand struct {
	manager    string
	executable string
	args       []string
}

var dependencyCommands = map[string]dependencyCommand{
	"package-lock.json": {
		manager: "npm", executable: "npm", args: []string{"ci"},
	},
	"pnpm-lock.yaml": {
		manager: "pnpm", executable: "pnpm", args: []string{"install", "--frozen-lockfile"},
	},
	"yarn.lock": {
		manager: "yarn", executable: "yarn", args: []string{"install", "--immutable"},
	},
	"uv.lock": {
		manager: "uv", executable: "uv", args: []string{"sync", "--frozen"},
	},
	"requirements.txt": {
		manager: "pip", executable: "python", args: []string{"-m", "pip", "install", "-r", "requirements.txt"},
	},
	"go.mod": {
		manager: "go", executable: "go", args: []string{"mod", "download"},
	},
}

var generatedDependencyDirectories = map[string]struct{}{
	"node_modules": {},
	".vscode-test": {},
	".venv":        {},
	"venv":         {},
	"__pycache__":  {},
	".pnpm-store":  {},
	".yarn":        {},
	"vendor":       {},
}

func DiscoverSkillDependencies(root string) ([]Declaration, error) {
	root = filepath.Clean(root)
	var declarations []Declaration
	err := filepath.WalkDir(root, func(filename string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if filename != root {
				if _, generated := generatedDependencyDirectories[strings.ToLower(entry.Name())]; generated {
					return filepath.SkipDir
				}
			}
			return nil
		}
		command, recognized := dependencyCommands[strings.ToLower(entry.Name())]
		if !recognized {
			return nil
		}
		data, err := os.ReadFile(filename)
		if err != nil {
			return err
		}
		directory, err := filepath.Rel(root, filepath.Dir(filename))
		if err != nil {
			return err
		}
		source := filepath.ToSlash(directory)
		if source == "" {
			source = "."
		}
		sum := sha256.Sum256(data)
		version := hex.EncodeToString(sum[:])
		declarations = append(declarations, Declaration{
			ID:      "skill:" + source + ":" + command.manager,
			Adapter: "skill-dependencies",
			Source:  source,
			Version: version,
			Enabled: true,
			Settings: map[string]string{
				"manager":  command.manager,
				"manifest": entry.Name(),
			},
		})
		return nil
	})
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("discover Skill dependencies: %w", err)
	}
	sort.Slice(declarations, func(i, j int) bool {
		return declarationIdentity(declarations[i]) < declarationIdentity(declarations[j])
	})
	return declarations, nil
}

func OperationsForSkillDependencies(root string, declarations []Declaration) ([]Operation, error) {
	root = filepath.Clean(root)
	operations := make([]Operation, 0, len(declarations))
	for _, declaration := range declarations {
		if declaration.Adapter != "skill-dependencies" {
			continue
		}
		if err := validateDeclaration(declaration); err != nil {
			return nil, err
		}
		manifest := strings.ToLower(declaration.Settings["manifest"])
		command, recognized := dependencyCommands[manifest]
		if !recognized || command.manager != declaration.Settings["manager"] {
			return nil, fmt.Errorf("Skill dependency %q uses unsupported manifest metadata", declaration.ID)
		}
		workingDir, err := safeWorkingDirectory(root, declaration.Source)
		if err != nil {
			return nil, err
		}
		operations = append(operations, Operation{
			ID:         "skill:" + command.manager + ":" + declaration.Source + "@" + declaration.Version,
			Adapter:    declaration.Adapter,
			Source:     declaration.Source,
			Kind:       "dependencies",
			Executable: command.executable,
			Args:       append([]string(nil), command.args...),
			WorkingDir: workingDir,
		})
	}
	sort.Slice(operations, func(i, j int) bool {
		return operationIdentity(operations[i]) < operationIdentity(operations[j])
	})
	return operations, nil
}

func safeWorkingDirectory(root, source string) (string, error) {
	if source == "." {
		return root, nil
	}
	if filepath.IsAbs(source) {
		return "", fmt.Errorf("Skill dependency source %q must be relative", source)
	}
	joined := filepath.Join(root, filepath.FromSlash(source))
	relative, err := filepath.Rel(root, joined)
	if err != nil ||
		relative == ".." ||
		strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("Skill dependency source %q escapes its root", source)
	}
	return joined, nil
}
