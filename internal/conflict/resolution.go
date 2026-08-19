package conflict

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"sync"
	"time"
)

type Choice string

const (
	ChoiceLocal  Choice = "local"
	ChoiceRemote Choice = "remote"
	ChoiceMerged Choice = "merged"
)

type ResolutionSelection struct {
	ID       string `json:"id"`
	Revision string `json:"revision"`
	Choice   Choice `json:"choice"`
	Content  []byte `json:"content,omitempty"`
}

type ResolutionBatch struct {
	ID         string                `json:"id"`
	CreatedAt  time.Time             `json:"createdAt"`
	Status     string                `json:"status"`
	Error      string                `json:"error,omitempty"`
	Selections []ResolutionSelection `json:"selections"`
}

type VisibleConflict struct {
	Record   Record `json:"record"`
	Revision string `json:"revision"`
}

var ErrResolutionActive = errors.New("a conflict resolution batch is already active")

type resolutionLockSet struct {
	metadata    sync.Mutex
	transaction sync.RWMutex
}

var resolutionLocks sync.Map

func sharedResolutionLocks(root string) *resolutionLockSet {
	absolute, err := filepath.Abs(root)
	if err == nil {
		root = filepath.Clean(absolute)
	} else {
		root = filepath.Clean(root)
	}
	actual, _ := resolutionLocks.LoadOrStore(root, &resolutionLockSet{})
	return actual.(*resolutionLockSet)
}

func (s *Store) resolutionMetadataRoot() string {
	return filepath.Join(filepath.Dir(s.localRoot), "conflict-resolution")
}

func (s *Store) metadataPath(name string) string {
	return filepath.Join(s.resolutionMetadataRoot(), name)
}

type revisionPayload struct {
	Record     Record `json:"record"`
	BaseHash   string `json:"baseHash"`
	LocalHash  string `json:"localHash"`
	RemoteHash string `json:"remoteHash"`
}

func (s *Store) Revision(id string) (string, error) {
	s.locks.transaction.RLock()
	defer s.locks.transaction.RUnlock()
	return s.revisionUnlocked(id)
}

func (s *Store) revisionUnlocked(id string) (string, error) {
	record, variants, err := s.readBundle(s.localRoot, id)
	if err != nil {
		return "", err
	}
	payload := revisionPayload{Record: record}
	for _, variant := range variants {
		sum := sha256.Sum256(variant.data)
		switch variant.name {
		case "base":
			payload.BaseHash = hex.EncodeToString(sum[:])
		case "local":
			payload.LocalHash = hex.EncodeToString(sum[:])
		case "remote":
			payload.RemoteHash = hex.EncodeToString(sum[:])
		}
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:]), nil
}

func (s *Store) QueueBatch(selections []ResolutionSelection) (ResolutionBatch, error) {
	s.locks.transaction.RLock()
	defer s.locks.transaction.RUnlock()

	visible, err := s.listVisibleUnlocked()
	if err != nil {
		return ResolutionBatch{}, err
	}
	byID := make(map[string]VisibleConflict, len(visible))
	for _, item := range visible {
		byID[item.Record.ID] = item
	}
	if len(selections) != len(visible) {
		return ResolutionBatch{}, fmt.Errorf("all visible conflicts must be selected")
	}
	ordered := append([]ResolutionSelection(nil), selections...)
	seen := make(map[string]struct{}, len(ordered))
	for _, selection := range ordered {
		if err := validateSegment("conflict id", selection.ID); err != nil {
			return ResolutionBatch{}, err
		}
		if _, ok := seen[selection.ID]; ok {
			return ResolutionBatch{}, fmt.Errorf("duplicate conflict selection %q", selection.ID)
		}
		seen[selection.ID] = struct{}{}
		conflict, ok := byID[selection.ID]
		if !ok {
			return ResolutionBatch{}, fmt.Errorf("conflict %q is not visible", selection.ID)
		}
		if selection.Revision != conflict.Revision {
			return ResolutionBatch{}, fmt.Errorf("conflict %q has a stale revision", selection.ID)
		}
		switch selection.Choice {
		case ChoiceLocal, ChoiceRemote:
			if len(selection.Content) != 0 {
				return ResolutionBatch{}, fmt.Errorf("content is not allowed for choice %q", selection.Choice)
			}
		case ChoiceMerged:
			if selection.Content == nil {
				return ResolutionBatch{}, fmt.Errorf("merged content is required for conflict %q", selection.ID)
			}
			if s.scanner != nil {
				if err := s.scanner(conflict.Record, "merged", selection.Content); err != nil {
					return ResolutionBatch{}, fmt.Errorf("scan conflict %s merged: %w", selection.ID, err)
				}
			}
		default:
			return ResolutionBatch{}, fmt.Errorf("conflict choice %q is not supported", selection.Choice)
		}
	}
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].ID < ordered[j].ID })
	canonical, err := json.Marshal(ordered)
	if err != nil {
		return ResolutionBatch{}, err
	}
	sum := sha256.Sum256(canonical)
	batch := ResolutionBatch{ID: hex.EncodeToString(sum[:]), CreatedAt: time.Now().UTC(), Status: "queued", Selections: ordered}

	s.locks.metadata.Lock()
	defer s.locks.metadata.Unlock()
	if pending, err := s.readBatchUnlocked("pending.json"); err != nil {
		return ResolutionBatch{}, err
	} else if pending != nil {
		return ResolutionBatch{}, ErrResolutionActive
	}
	if err := writeJSONAtomic(s.metadataPath("pending.json"), batch); err != nil {
		return ResolutionBatch{}, err
	}
	_ = os.Remove(s.metadataPath("failed.json"))
	return batch, nil
}

func (s *Store) PendingBatch() (*ResolutionBatch, error) {
	s.locks.metadata.Lock()
	defer s.locks.metadata.Unlock()
	return s.readBatchUnlocked("pending.json")
}

func (s *Store) FailedBatch() (*ResolutionBatch, error) {
	s.locks.metadata.Lock()
	defer s.locks.metadata.Unlock()
	return s.readBatchUnlocked("failed.json")
}

func (s *Store) ResolutionStatus() (*ResolutionBatch, error) {
	s.locks.metadata.Lock()
	defer s.locks.metadata.Unlock()
	if batch, err := s.readBatchUnlocked("pending.json"); err != nil || batch != nil {
		return batch, err
	}
	return s.readBatchUnlocked("failed.json")
}

func (s *Store) VisibleConflicts() ([]VisibleConflict, *ResolutionBatch, error) {
	for {
		s.locks.metadata.Lock()
		pending, err := s.readBatchUnlocked("pending.json")
		if err == nil && pending != nil && pending.Status == "applying" {
			view, viewErr := s.readViewUnlocked()
			s.locks.metadata.Unlock()
			return view, pendingOrFailed(pending, viewErr), viewErr
		}
		s.locks.metadata.Unlock()
		if !s.locks.transaction.TryRLock() {
			runtime.Gosched()
			continue
		}
		s.locks.metadata.Lock()
		pending, err = s.readBatchUnlocked("pending.json")
		if err != nil {
			s.locks.metadata.Unlock()
			s.locks.transaction.RUnlock()
			return nil, nil, err
		}
		if pending != nil && pending.Status == "applying" {
			view, viewErr := s.readViewUnlocked()
			s.locks.metadata.Unlock()
			s.locks.transaction.RUnlock()
			return view, pending, viewErr
		}
		visible, listErr := s.listVisibleUnlocked()
		status := pending
		if status == nil {
			status, listErr = s.readBatchUnlocked("failed.json")
		}
		s.locks.metadata.Unlock()
		s.locks.transaction.RUnlock()
		return visible, status, listErr
	}
}

func pendingOrFailed(batch *ResolutionBatch, err error) *ResolutionBatch {
	if err != nil {
		return batch
	}
	return batch
}

func (s *Store) listVisibleUnlocked() ([]VisibleConflict, error) {
	records, err := s.listUnlocked()
	if err != nil {
		return nil, err
	}
	result := make([]VisibleConflict, 0, len(records))
	for _, record := range records {
		revision, err := s.revisionUnlocked(record.ID)
		if err != nil {
			return nil, err
		}
		result = append(result, VisibleConflict{Record: record, Revision: revision})
	}
	return result, nil
}

func (s *Store) readBatchUnlocked(name string) (*ResolutionBatch, error) {
	data, err := os.ReadFile(s.metadataPath(name))
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var batch ResolutionBatch
	if err := json.Unmarshal(data, &batch); err != nil {
		return nil, fmt.Errorf("parse %s: %w", name, err)
	}
	return &batch, nil
}

func (s *Store) readViewUnlocked() ([]VisibleConflict, error) {
	data, err := os.ReadFile(s.metadataPath("applying-view.json"))
	if err != nil {
		return nil, err
	}
	var view []VisibleConflict
	if err := json.Unmarshal(data, &view); err != nil {
		return nil, fmt.Errorf("parse applying view: %w", err)
	}
	return view, nil
}

func writeJSONAtomic(filename string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return writeAtomic(filename, data, 0o600)
}
