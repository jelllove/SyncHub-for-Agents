package installplan

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

type Store struct {
	root string
	mu   sync.Mutex
}

func NewStore(root string) *Store {
	return &Store{root: root}
}

func (s *Store) SavePending(plan Plan) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.writeJSON("pending.json", plan)
}

func (s *Store) Pending() (*Plan, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var plan Plan
	exists, err := s.readJSON("pending.json", &plan)
	if err != nil || !exists {
		return nil, err
	}
	return &plan, nil
}

func (s *Store) Approve(operation Operation) error {
	if operation.Adapter == "" || operation.Source == "" {
		return fmt.Errorf("install approval identity is incomplete")
	}
	if err := rejectShellSyntax(operation.Source); err != nil {
		return fmt.Errorf("install approval source: %w", err)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	approvals, err := s.loadSet("approvals.json")
	if err != nil {
		return err
	}
	approvals[approvalKey(operation)] = true
	return s.writeJSON("approvals.json", approvals)
}

func (s *Store) IsApproved(operation Operation) bool {
	approved, err := s.approvalStatus(operation)
	return err == nil && approved
}

func (s *Store) approvalStatus(operation Operation) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	approvals, err := s.loadSet("approvals.json")
	if err != nil {
		return false, err
	}
	return approvals[approvalKey(operation)], nil
}

func (s *Store) ClearPending(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var plan Plan
	exists, err := s.readJSON("pending.json", &plan)
	if err != nil || !exists || plan.ID != id {
		return err
	}
	err = os.Remove(filepath.Join(s.root, "pending.json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}

func (s *Store) MarkApplied(operation Operation) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	applied, err := s.loadSet("applied.json")
	if err != nil {
		return err
	}
	applied[appliedKey(operation)] = true
	return s.writeJSON("applied.json", applied)
}

func (s *Store) IsApplied(operation Operation) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	applied, err := s.loadSet("applied.json")
	if err != nil {
		return false, err
	}
	return applied[appliedKey(operation)], nil
}

func (s *Store) loadSet(name string) (map[string]bool, error) {
	values := map[string]bool{}
	_, err := s.readJSON(name, &values)
	return values, err
}

func (s *Store) readJSON(name string, destination any) (bool, error) {
	data, err := os.ReadFile(filepath.Join(s.root, name))
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if err := json.Unmarshal(data, destination); err != nil {
		return false, fmt.Errorf("parse install store %s: %w", name, err)
	}
	return true, nil
}

func (s *Store) writeJSON(name string, value any) error {
	if err := os.MkdirAll(s.root, 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	temporary, err := os.CreateTemp(s.root, name+".tmp-*")
	if err != nil {
		return err
	}
	temporaryName := temporary.Name()
	defer os.Remove(temporaryName)
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
	return os.Rename(temporaryName, filepath.Join(s.root, name))
}

func approvalKey(operation Operation) string {
	sum := sha256.Sum256([]byte(operation.Adapter + "\x00" + operation.Source))
	return hex.EncodeToString(sum[:])
}

func appliedKey(operation Operation) string {
	sum := sha256.Sum256([]byte(operationIdentity(operation)))
	return hex.EncodeToString(sum[:])
}
