package desktop

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/qinqingxu/synchub-for-agents/internal/daemon"
)

type summaryStore struct {
	root string
}

func newSummaryStore(home string) *summaryStore {
	return &summaryStore{root: filepath.Join(home, "desktop")}
}

func (s *summaryStore) loadCycle() (daemon.CycleResult, error) {
	var result daemon.CycleResult
	if err := s.load("cycle.json", &result); err != nil {
		return daemon.CycleResult{}, err
	}
	return result, nil
}

func (s *summaryStore) saveCycle(result daemon.CycleResult) error {
	return s.save("cycle.json", result)
}

func (s *summaryStore) loadPreview() (ResourcePreview, error) {
	var preview ResourcePreview
	if err := s.load("preview.json", &preview); err != nil {
		return ResourcePreview{}, err
	}
	return preview, nil
}

func (s *summaryStore) savePreview(preview ResourcePreview) error {
	return s.save("preview.json", preview)
}

func (s *summaryStore) clearPreview() error {
	target := filepath.Join(s.root, "preview.json")
	if err := os.Remove(target); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("clear desktop summary %q: %w", target, err)
	}
	return nil
}

func (s *summaryStore) load(name string, destination any) error {
	target := filepath.Join(s.root, name)
	data, err := os.ReadFile(target)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("load desktop summary %q: %w", target, err)
	}
	if err := json.Unmarshal(data, destination); err != nil {
		return fmt.Errorf("parse desktop summary %q: %w", target, err)
	}
	return nil
}

func (s *summaryStore) save(name string, value any) error {
	if err := os.MkdirAll(s.root, 0o700); err != nil {
		return fmt.Errorf("create desktop summary directory %q: %w", s.root, err)
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal desktop summary %q: %w", name, err)
	}
	data = append(data, '\n')

	target := filepath.Join(s.root, name)
	temporary, err := os.CreateTemp(s.root, "."+name+".tmp-*")
	if err != nil {
		return fmt.Errorf("create desktop summary temporary file for %q: %w", target, err)
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)

	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return fmt.Errorf("set desktop summary permissions for %q: %w", target, err)
	}
	if _, err := temporary.Write(data); err != nil {
		temporary.Close()
		return fmt.Errorf("write desktop summary %q: %w", target, err)
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return fmt.Errorf("sync desktop summary %q: %w", target, err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close desktop summary %q: %w", target, err)
	}
	if err := os.Rename(temporaryName, target); err != nil {
		return fmt.Errorf("replace desktop summary %q: %w", target, err)
	}
	return nil
}
