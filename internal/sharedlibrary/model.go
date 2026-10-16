// Package sharedlibrary manages explicitly approved projections of portable
// sources. Callers serialize operations with Lock, including synchronization.
package sharedlibrary

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/qinqingxu/synchub-for-agents/internal/instance"
)

type Manager struct {
	Home     string
	UserHome string
}

type Request struct {
	Action  string `json:"action"`
	Kind    string `json:"kind"`
	Name    string `json:"name"`
	Harness string `json:"harness"`
}

type Change struct {
	Path   string `json:"path"`
	Action string `json:"action"`
	Before string `json:"before"`
	After  string `json:"after"`
}

type Plan struct {
	ID      string   `json:"id"`
	Request Request  `json:"request"`
	Changes []Change `json:"changes"`
}

type Entry struct {
	Kind    string `json:"kind"`
	Name    string `json:"name"`
	Harness string `json:"harness"`
	Path    string `json:"path"`
	Status  string `json:"status"`
}

type Snapshot struct {
	Root    string  `json:"root"`
	Entries []Entry `json:"entries"`
}

func Root(userHome string) string {
	return filepath.Join(userHome, ".agents", "library")
}

func Lock(userHome string) (*instance.Lock, error) {
	root := filepath.Join(userHome, ".synchub", "library-owner")
	if err := noLinks(userHome, root); err != nil {
		return nil, err
	}
	return instance.Acquire(root)
}

var namePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,63}$`)

func validateName(name string) error {
	if !namePattern.MatchString(name) {
		return fmt.Errorf("library name must contain 1-64 lowercase letters, digits, '-' or '_'")
	}
	switch strings.ToUpper(name) {
	case "CON", "PRN", "AUX", "NUL", "COM1", "COM2", "COM3", "COM4", "COM5", "COM6", "COM7", "COM8", "COM9",
		"LPT1", "LPT2", "LPT3", "LPT4", "LPT5", "LPT6", "LPT7", "LPT8", "LPT9":
		return fmt.Errorf("library name %q is reserved", name)
	}
	return nil
}

func (m Manager) paths(r Request) (string, string, error) {
	if err := validateName(r.Name); err != nil {
		return "", "", err
	}
	switch r.Kind {
	case "skills":
		source := filepath.Join(Root(m.UserHome), "skills", r.Name)
		switch r.Harness {
		case "claude":
			return source, filepath.Join(m.UserHome, ".claude", "skills", r.Name), nil
		case "common":
			return source, filepath.Join(m.UserHome, ".agents", "skills", r.Name), nil
		case "copilot":
			if r.Action == "import" {
				return source, filepath.Join(m.UserHome, ".copilot", "skills", r.Name), nil
			}
		}
	case "instructions":
		if r.Name == "global" && r.Harness == "claude" {
			return filepath.Join(Root(m.UserHome), "instructions", "AGENTS.md"),
				filepath.Join(m.UserHome, ".claude", "CLAUDE.md"), nil
		}
	}
	return "", "", fmt.Errorf("unsupported library mapping %s/%s to %s", r.Kind, r.Name, r.Harness)
}

func key(r Request) string { return r.Kind + "/" + r.Name + "/" + r.Harness }
