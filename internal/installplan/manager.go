package installplan

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/qinqingxu/synchub-for-agents/internal/resource"
)

const skillDependencyManifest = "agents/_portable/config/install/skill-dependencies.json"

type ReconcileResult struct {
	Executed int
	Pending  int
	Errors   map[string]string
}

type Manager struct {
	Home     string
	Registry *InventoryRegistry
	Runner   Runner
	Store    *Store
	Now      func() time.Time
}

func NewManager(home string, registry *InventoryRegistry, runner Runner) *Manager {
	return &Manager{
		Home:     home,
		Registry: registry,
		Runner:   runner,
		Store:    NewStore(filepath.Join(home, "install")),
		Now:      time.Now,
	}
}

func (m *Manager) Reconcile(
	ctx context.Context,
	repoDir string,
	specs map[string]resource.Spec,
	currentManifests map[string][]byte,
) (ReconcileResult, error) {
	if m.Registry == nil {
		return ReconcileResult{}, fmt.Errorf("install manager requires an inventory registry")
	}
	if m.Runner == nil {
		return ReconcileResult{}, fmt.Errorf("install manager requires a runner")
	}
	if m.Store == nil {
		return ReconcileResult{}, fmt.Errorf("install manager requires a store")
	}

	var declarations []Declaration
	var operations []Operation
	installSpecs := make(map[string]resource.Spec)
	for _, key := range sortedSpecKeys(specs) {
		spec := specs[key]
		if spec.Strategy != resource.StrategyInstallManifest {
			continue
		}
		adapter, ok := m.Registry.Adapter(spec.Installer)
		if !ok {
			return ReconcileResult{}, fmt.Errorf("installer adapter %q is unavailable", spec.Installer)
		}
		desired, err := readDeclarations(repoDir, spec)
		if err != nil {
			return ReconcileResult{}, err
		}
		current, err := parseDeclarations(
			currentManifests[spec.Key],
			"current "+spec.Key,
		)
		if err != nil {
			return ReconcileResult{}, err
		}
		if err := validateManifestAdapter(spec.Installer, desired); err != nil {
			return ReconcileResult{}, err
		}
		if err := validateManifestAdapter(spec.Installer, current); err != nil {
			return ReconcileResult{}, err
		}
		planned, err := adapter.Operations(desired, current)
		if err != nil {
			return ReconcileResult{}, err
		}
		declarations = append(declarations, desired...)
		operations = append(operations, planned...)
		installSpecs[spec.Installer] = spec
	}

	skillSpec, hasSkills := sharedSkillSpec(specs)
	if hasSkills {
		skillDeclarations, err := m.updateSkillManifest(repoDir)
		if err != nil {
			return ReconcileResult{}, err
		}
		skillOperations, err := OperationsForSkillDependencies(skillSpec.Root, skillDeclarations)
		if err != nil {
			return ReconcileResult{}, err
		}
		declarations = append(declarations, skillDeclarations...)
		operations = append(operations, skillOperations...)
	}

	plan, err := NewPlan(declarations, operations)
	if err != nil {
		return ReconcileResult{}, err
	}
	if len(plan.Operations) == 0 {
		pending, err := m.Store.Pending()
		if err != nil {
			return ReconcileResult{}, err
		}
		if pending != nil {
			if err := m.Store.ClearPending(pending.ID); err != nil {
				return ReconcileResult{}, err
			}
		}
		return ReconcileResult{}, nil
	}
	plan.Approved = true
	for _, operation := range plan.Operations {
		approved, err := m.Store.approvalStatus(operation)
		if err != nil {
			return ReconcileResult{}, err
		}
		if !approved {
			plan.Approved = false
			break
		}
	}
	if err := m.Store.SavePending(plan); err != nil {
		return ReconcileResult{}, err
	}

	executor := &Executor{
		Runner: m.Runner,
		Store:  m.Store,
		BeforeRun: func(operation Operation) error {
			if operation.Kind != "uninstall" {
				return nil
			}
			spec, ok := installSpecs[operation.Adapter]
			if !ok {
				return nil
			}
			return m.backupPlugin(spec, operation)
		},
	}
	executed, err := executor.Execute(ctx, plan)
	if err != nil {
		return ReconcileResult{}, err
	}
	return ReconcileResult{
		Executed: len(executed.Executed),
		Pending:  len(executed.Pending),
		Errors:   executed.Errors,
	}, nil
}

func readDeclarations(repoDir string, spec resource.Spec) ([]Declaration, error) {
	repoRel, err := spec.RepoPath("manifest.json")
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filepath.Join(repoDir, filepath.FromSlash(repoRel)))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return parseDeclarations(data, repoRel)
}

func parseDeclarations(data []byte, label string) ([]Declaration, error) {
	if len(data) == 0 {
		return nil, nil
	}
	var declarations []Declaration
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&declarations); err != nil {
		return nil, fmt.Errorf("parse install declarations %s: %w", label, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			return nil, fmt.Errorf("parse install declarations %s: multiple JSON values", label)
		}
		return nil, fmt.Errorf("parse install declarations %s: %w", label, err)
	}
	for _, declaration := range declarations {
		if err := validateDeclaration(declaration); err != nil {
			return nil, fmt.Errorf("validate install declarations %s: %w", label, err)
		}
	}
	return declarations, nil
}

func ParseDeclarations(data []byte) ([]Declaration, error) {
	return parseDeclarations(data, "manifest")
}

func ValidateDeclarations(adapter string, data []byte) ([]Declaration, error) {
	declarations, err := ParseDeclarations(data)
	if err != nil {
		return nil, err
	}
	if err := validateManifestAdapter(adapter, declarations); err != nil {
		return nil, err
	}
	return declarations, nil
}

func validateManifestAdapter(adapter string, declarations []Declaration) error {
	for _, declaration := range declarations {
		if declaration.Adapter != adapter {
			return fmt.Errorf(
				"install declaration %q uses adapter %q, want %q",
				declaration.ID,
				declaration.Adapter,
				adapter,
			)
		}
	}
	return nil
}

func (m *Manager) updateSkillManifest(repoDir string) ([]Declaration, error) {
	sourceRoot := filepath.Join(
		repoDir,
		"agents",
		"_portable",
		"config",
		"common",
		"skills",
		"common-skills",
	)
	declarations, err := DiscoverSkillDependencies(sourceRoot)
	if err != nil {
		return nil, err
	}
	filename := filepath.Join(repoDir, filepath.FromSlash(skillDependencyManifest))
	if len(declarations) == 0 {
		if err := os.Remove(filename); err != nil && !errors.Is(err, os.ErrNotExist) {
			return nil, err
		}
		return nil, nil
	}
	data, err := json.MarshalIndent(declarations, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
		return nil, err
	}
	if err := writeInstallFile(filename, append(data, '\n')); err != nil {
		return nil, err
	}
	return declarations, nil
}

func (m *Manager) backupPlugin(spec resource.Spec, operation Operation) error {
	name := strings.SplitN(operation.Source, "@", 2)[0]
	if name == "" ||
		name == "." ||
		name == ".." ||
		strings.ContainsAny(name, `/\`) {
		return fmt.Errorf("plugin source %q has no safe payload name", operation.Source)
	}
	source := filepath.Join(spec.Root, name)
	info, err := os.Lstat(source)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("plugin payload %q is a symbolic link", source)
	}
	now := m.Now
	if now == nil {
		now = time.Now
	}
	destination := filepath.Join(
		m.Home,
		"local-trash",
		now().UTC().Format("20060102T150405.000000000Z"),
		"plugins",
		operation.Adapter,
		name,
	)
	return copyPayload(source, destination)
}

func copyPayload(source, destination string) error {
	return filepath.WalkDir(source, func(filename string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, filename)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, relative)
		if entry.Type()&os.ModeSymlink != 0 {
			return fmt.Errorf("plugin payload contains symbolic link %q", filename)
		}
		if entry.IsDir() {
			return os.MkdirAll(target, 0o700)
		}
		data, err := os.ReadFile(filename)
		if err != nil {
			return err
		}
		return writeInstallFile(target, data)
	})
}

func writeInstallFile(filename string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(filename), 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(filename), filepath.Base(filename)+".tmp-*")
	if err != nil {
		return err
	}
	name := temporary.Name()
	defer os.Remove(name)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	return os.Rename(name, filename)
}

func sharedSkillSpec(specs map[string]resource.Spec) (resource.Spec, bool) {
	if spec, ok := specs["common/common-skills"]; ok &&
		spec.Category == resource.CategorySkills {
		return spec, true
	}
	for _, key := range sortedSpecKeys(specs) {
		spec := specs[key]
		if spec.Category == resource.CategorySkills {
			return spec, true
		}
	}
	return resource.Spec{}, false
}

func sortedSpecKeys(specs map[string]resource.Spec) []string {
	keys := make([]string, 0, len(specs))
	for key := range specs {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
