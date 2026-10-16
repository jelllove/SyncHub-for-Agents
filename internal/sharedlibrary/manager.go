package sharedlibrary

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

type record struct {
	Request  Request `json:"request"`
	Original tree    `json:"original"`
	Applied  tree    `json:"applied"`
	Previous tree    `json:"previous,omitempty"`
	Pending  bool    `json:"pending"`
}

type ownership struct {
	Version int               `json:"version"`
	Records map[string]record `json:"records"`
}

func (m Manager) statePath() string {
	// Ownership is machine-local and shared across profiles using this user home.
	return filepath.Join(m.UserHome, ".synchub", "library-ownership.json")
}

func (m Manager) load() (ownership, error) {
	state := ownership{Version: 1, Records: map[string]record{}}
	if err := noLinks(m.statePath()); err != nil {
		return state, err
	}
	data, err := os.ReadFile(m.statePath())
	if errors.Is(err, os.ErrNotExist) {
		return state, nil
	}
	if err != nil {
		return state, err
	}
	if err := json.Unmarshal(data, &state); err != nil {
		return state, fmt.Errorf("read library ownership: %w", err)
	}
	if state.Version != 1 || state.Records == nil {
		return state, fmt.Errorf("unsupported library ownership format")
	}
	for id, rec := range state.Records {
		if _, _, err := m.paths(rec.Request); err != nil || id != key(rec.Request) {
			return state, fmt.Errorf("invalid library ownership record %q", id)
		}
		for _, files := range []tree{rec.Original, rec.Applied, rec.Previous} {
			for rel := range files {
				if !filepath.IsLocal(filepath.FromSlash(rel)) || filepath.ToSlash(filepath.Clean(filepath.FromSlash(rel))) != rel {
					return state, fmt.Errorf("invalid library backup path")
				}
			}
		}
	}
	return state, nil
}

func (m Manager) save(state ownership) error {
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return atomicWrite(m.statePath(), data, 0o600)
}

type prepared struct {
	Plan
	root      string
	directory bool
	before    tree
	after     tree
	state     ownership
}

func (m Manager) prepare(r Request) (prepared, error) {
	source, target, err := m.paths(r)
	if err != nil {
		return prepared{}, err
	}
	state, err := m.load()
	if err != nil {
		return prepared{}, err
	}
	p := prepared{Plan: Plan{Request: r, Changes: []Change{}}, root: target, directory: r.Kind == "skills", state: state}
	if r.Action == "import" {
		p.root = source
	}
	if r.Action != "detach" {
		p.before, err = readTree(p.root, p.directory)
		if err != nil {
			return p, err
		}
	}
	rec, owned := state.Records[key(r)]
	switch r.Action {
	case "import":
		p.after, err = readTree(target, p.directory)
	case "apply":
		if r.Kind == "skills" && r.Harness == "common" {
			duplicate := filepath.Join(m.UserHome, ".copilot", "skills", r.Name)
			if _, err := os.Lstat(duplicate); err == nil {
				return p, fmt.Errorf("duplicate native skill discovery: %s already exists; import and review it, then move or rename the private copy before applying to common", duplicate)
			} else if !errors.Is(err, os.ErrNotExist) {
				return p, err
			}
		}
		if owned && (rec.Pending || digest(p.before) != digest(rec.Applied)) {
			return p, fmt.Errorf("target was modified or an operation is incomplete; import changes, detach, or restore before applying")
		}
		p.after, err = readTree(source, p.directory)
	case "restore":
		if !owned {
			return p, fmt.Errorf("target is not managed")
		}
		if !recoverable(p.before, rec) {
			return p, fmt.Errorf("target was modified; detach to preserve it instead of overwriting")
		}
		p.after = rec.Original
	case "detach":
		if !owned {
			return p, fmt.Errorf("target is not managed")
		}
		p.after = p.before
	default:
		return p, fmt.Errorf("unsupported library action %q", r.Action)
	}
	if err != nil {
		return p, err
	}
	if r.Action == "import" || r.Action == "apply" {
		if len(p.after) == 0 {
			return p, fmt.Errorf("source is missing or empty")
		}
		if p.directory {
			if _, ok := p.after["SKILL.md"]; !ok {
				return p, fmt.Errorf("skill must contain SKILL.md")
			}
		}
	}
	names := map[string]bool{}
	for rel := range p.before {
		names[rel] = true
	}
	for rel := range p.after {
		names[rel] = true
	}
	for rel := range names {
		before, had := p.before[rel]
		after, has := p.after[rel]
		if had && has && digest(before) == digest(after) {
			continue
		}
		action := "replace"
		if !had {
			action = "create"
		} else if !has {
			action = "delete"
		}
		p.Changes = append(p.Changes, Change{Path: filePath(p.root, rel, p.directory), Action: action,
			Before: string(before.Data), After: string(after.Data)})
	}
	sort.Slice(p.Changes, func(i, j int) bool { return p.Changes[i].Path < p.Changes[j].Path })
	p.ID = digest(struct {
		Request Request
		Root    string
		Before  tree
		After   tree
		State   ownership
	}{r, p.root, p.before, p.after, state})
	return p, nil
}

func recoverable(current tree, rec record) bool {
	if !rec.Pending {
		return digest(current) == digest(rec.Applied)
	}
	for rel, value := range current {
		if digest(value) != digest(rec.Applied[rel]) && digest(value) != digest(rec.Previous[rel]) {
			return false
		}
	}
	for rel := range rec.Applied {
		if _, ok := current[rel]; !ok {
			if _, existed := rec.Previous[rel]; existed {
				return false
			}
		}
	}
	return true
}

func (m Manager) Preview(r Request) (Plan, error) {
	p, err := m.prepare(r)
	return p.Plan, err
}

func (m Manager) Apply(r Request, id string) error {
	p, err := m.prepare(r)
	if err != nil {
		return err
	}
	if id == "" || id != p.ID {
		return fmt.Errorf("library preview is stale; preview again before approving")
	}
	if r.Action == "import" {
		backup, err := json.Marshal(struct {
			Request Request `json:"request"`
			Before  tree    `json:"before"`
		}{r, p.before})
		if err != nil {
			return err
		}
		if err := atomicWrite(filepath.Join(m.UserHome, ".synchub", "library-import-backups", p.ID+".json"), backup, 0o600); err != nil {
			return fmt.Errorf("save canonical import backup: %w", err)
		}
		// Imports do not take ownership of the source Harness. Roll back partial
		// canonical writes if a write fails; never change the import source.
		if err := writeTree(p.root, p.directory, p.before, p.after); err != nil {
			current, readErr := readTree(p.root, p.directory)
			if readErr != nil {
				return errors.Join(err, readErr)
			}
			return errors.Join(err, writeTree(p.root, p.directory, current, p.before))
		}
		return nil
	}
	if r.Action == "detach" {
		delete(p.state.Records, key(r))
		return m.save(p.state)
	}
	rec, exists := p.state.Records[key(r)]
	if !exists {
		rec = record{Request: r, Original: p.before}
	}
	rec.Previous, rec.Applied, rec.Pending = p.before, p.after, true
	p.state.Records[key(r)] = rec
	if err := m.save(p.state); err != nil {
		return err
	}
	if err := writeTree(p.root, p.directory, p.before, p.after); err != nil {
		return fmt.Errorf("library operation incomplete; preview restore to recover: %w", err)
	}
	rec.Pending, rec.Previous = false, nil
	p.state.Records[key(r)] = rec
	if r.Action == "restore" {
		delete(p.state.Records, key(r))
	}
	if err := m.save(p.state); err != nil {
		return fmt.Errorf("library files written but ownership update failed; preview restore to recover: %w", err)
	}
	return nil
}

// Refresh only updates previously approved targets. New targets always require
// a reviewed plan, even when sources have arrived from another computer.
func (m Manager) Refresh(skills, instructions bool) error {
	state, err := m.load()
	if err != nil {
		return err
	}
	var failures []error
	for _, rec := range state.Records {
		r := rec.Request
		if (r.Kind == "skills" && !skills) || (r.Kind == "instructions" && !instructions) {
			continue
		}
		r.Action = "apply"
		p, err := m.Preview(r)
		if err == nil {
			err = m.Apply(r, p.ID)
		}
		if err != nil {
			failures = append(failures, fmt.Errorf("%s: %w", key(r), err))
		}
	}
	return errors.Join(failures...)
}
