package sharedlibrary

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/qinqingxu/synchub-for-agents/internal/resource"
	"github.com/qinqingxu/synchub-for-agents/internal/state"
)

// ProtectSpecs reserves generated outputs even after the canonical source is
// removed. A managed source deletion requires explicit restore/detach, not an
// automatic destructive cascade through the legacy alias synchronizer.
func ProtectSpecs(specs map[string]resource.Spec, userHome string) error {
	configureSources(specs, userHome)
	names, err := SkillNames(userHome)
	if err != nil {
		return err
	}
	m := Manager{UserHome: userHome}
	state, err := m.load()
	if err != nil {
		return err
	}
	instructions := false
	if _, err := os.Lstat(filepath.Join(Root(userHome), "instructions", "AGENTS.md")); err == nil {
		instructions = true
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	for _, rec := range state.Records {
		if rec.Request.Kind == "skills" {
			names = append(names, rec.Request.Name)
		} else {
			instructions = true
		}
	}
	return protect(specs, userHome, names, instructions)
}

// ProtectRemoteSpecs runs before collection as well as restore planning. A new
// computer must not restore historical aliases while first receiving sources.
func ProtectRemoteSpecs(specs map[string]resource.Spec, remote state.Snapshot, userHome string) error {
	configureSources(specs, userHome)
	var names []string
	seen := map[string]bool{}
	instructions := false
	for rel := range remote {
		ref, err := resource.ParseRepoPath(rel)
		if err != nil || !ref.Common {
			continue
		}
		if ref.ResourceID == "library-skills" && ref.Category == resource.CategorySkills {
			name, _, found := strings.Cut(ref.Relative, "/")
			if !found {
				continue
			}
			if err := validateName(name); err != nil {
				return err
			}
			if !seen[name] {
				names = append(names, name)
				seen[name] = true
			}
		}
		if ref.ResourceID == "library-instructions" && ref.Category == resource.CategoryInstructions && ref.Relative == "AGENTS.md" {
			instructions = true
		}
	}
	return protect(specs, userHome, names, instructions)
}

func configureSources(specs map[string]resource.Spec, userHome string) {
	for _, key := range []string{"common/library-skills", "common/library-instructions"} {
		spec, exists := specs[key]
		if !exists {
			continue
		}
		spec.OptionalRoot = true
		spec.CopyOnly = true
		spec.ValidateRoot = func(root string) error { return noLinks(userHome, root) }
		spec.ValidateContent = validateContent
		specs[key] = spec
	}
}

func protect(specs map[string]resource.Spec, userHome string, names []string, instructions bool) error {
	if len(names) == 0 && !instructions {
		return nil
	}
	m := Manager{UserHome: userHome}
	for key, spec := range specs {
		if spec.Provider == "common" && strings.HasPrefix(spec.ID, "library-") {
			spec.CopyOnly = true
			specs[key] = spec
			continue
		}
		if spec.SharedAs == "common-skills" {
			spec.CopyOnly = true
		}
		// Reserve paths by their actual location, including custom/legacy
		// resources rooted above a managed output, not only built-in IDs.
		roots := append([]string{spec.Root}, spec.Targets...)
		for _, root := range roots {
			reserve(&spec, root, Root(userHome))
			for _, name := range names {
				for _, harness := range []string{"claude", "common", "copilot"} {
					r := Request{Action: "import", Kind: "skills", Name: name, Harness: harness}
					_, target, err := m.paths(r)
					if err != nil {
						return err
					}
					reserve(&spec, root, target)
				}
			}
			if instructions {
				reserve(&spec, root, filepath.Join(userHome, ".claude", "CLAUDE.md"))
			}
		}
		specs[key] = spec
	}
	return nil
}

func reserve(spec *resource.Spec, root, target string) {
	rel, err := filepath.Rel(root, target)
	if err != nil {
		return
	}
	if rel == "." {
		spec.Reserved = append(spec.Reserved, "")
		spec.Exclude = append(spec.Exclude, "**")
		return
	}
	if !filepath.IsLocal(rel) {
		// A custom root inside an output is wholly owned by the library.
		if inverse, err := filepath.Rel(target, root); err == nil && filepath.IsLocal(inverse) {
			spec.Reserved = append(spec.Reserved, "")
			spec.Exclude = append(spec.Exclude, "**")
		}
		return
	}
	rel = filepath.ToSlash(rel)
	spec.Reserved = append(spec.Reserved, rel)
	spec.Exclude = append(spec.Exclude, rel, rel+"/**")
}
