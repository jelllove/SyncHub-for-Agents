package sharedlibrary

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

func SkillNames(userHome string) ([]string, error) {
	return skillNames(filepath.Join(Root(userHome), "skills"))
}

func skillNames(root string) ([]string, error) {
	if err := noLinks(root); err != nil {
		return nil, err
	}
	items, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return []string{}, nil
	}
	if err != nil {
		return nil, err
	}
	var names []string
	for _, item := range items {
		if item.IsDir() || item.Type()&(os.ModeSymlink|os.ModeIrregular) != 0 {
			if err := validateName(item.Name()); err != nil {
				return nil, fmt.Errorf("unsupported skill directory %q: %w", item.Name(), err)
			}
			names = append(names, item.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}

func (m Manager) Snapshot() (Snapshot, error) {
	state, err := m.load()
	result := Snapshot{Root: Root(m.UserHome), Entries: []Entry{}}
	if err != nil {
		return result, err
	}
	roots := []struct{ harness, path string }{
		{"library", filepath.Join(Root(m.UserHome), "skills")},
		{"claude", filepath.Join(m.UserHome, ".claude", "skills")},
		{"common", filepath.Join(m.UserHome, ".agents", "skills")},
		{"copilot", filepath.Join(m.UserHome, ".copilot", "skills")},
	}
	for _, root := range roots {
		names, err := skillNames(root.path)
		if err != nil {
			result.Entries = append(result.Entries, Entry{Kind: "skills", Harness: root.harness,
				Path: root.path, Status: "blocked: " + err.Error()})
			continue
		}
		for _, name := range names {
			result.Entries = append(result.Entries, Entry{Kind: "skills", Name: name, Harness: root.harness,
				Path: filepath.Join(root.path, name), Status: "available"})
		}
	}
	for _, item := range []Entry{
		{Kind: "instructions", Name: "global", Harness: "library", Path: filepath.Join(Root(m.UserHome), "instructions", "AGENTS.md"), Status: "available"},
		{Kind: "instructions", Name: "global", Harness: "claude", Path: filepath.Join(m.UserHome, ".claude", "CLAUDE.md"), Status: "available"},
	} {
		if _, err := os.Lstat(item.Path); err == nil {
			result.Entries = append(result.Entries, item)
		} else if !errors.Is(err, os.ErrNotExist) {
			return result, err
		}
	}
	for _, rec := range state.Records {
		r := rec.Request
		_, target, err := m.paths(r)
		if err != nil {
			return result, err
		}
		current, err := readTree(target, r.Kind == "skills")
		status := "managed"
		if err != nil || digest(current) != digest(rec.Applied) {
			status = "modified"
		}
		if rec.Pending {
			status = "recovery-required"
		}
		found := false
		for i := range result.Entries {
			if result.Entries[i].Path == target {
				result.Entries[i].Status = status
				found = true
			}
		}
		if !found {
			result.Entries = append(result.Entries, Entry{Kind: r.Kind, Name: r.Name, Harness: r.Harness, Path: target, Status: status})
		}
	}
	sort.Slice(result.Entries, func(i, j int) bool {
		a, b := result.Entries[i], result.Entries[j]
		return a.Kind+a.Name+a.Harness < b.Kind+b.Name+b.Harness
	})
	return result, nil
}
