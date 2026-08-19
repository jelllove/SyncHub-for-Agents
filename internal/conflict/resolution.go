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

type transactionEntry struct {
	Path   string `json:"path"`
	Exists bool   `json:"exists"`
	Data   []byte `json:"data,omitempty"`
}

type transactionJournal struct {
	BatchID string             `json:"batchId"`
	Entries []transactionEntry `json:"entries"`
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

// ApplyPendingBatch applies the queued batch as one recoverable transaction.
func (s *Store) ApplyPendingBatch() error {
	s.locks.transaction.Lock()
	defer s.locks.transaction.Unlock()
	s.locks.metadata.Lock()
	defer s.locks.metadata.Unlock()
	batch, err := s.readBatchUnlocked("pending.json")
	if err != nil || batch == nil {
		return err
	}
	if batch.Status != "queued" {
		return fmt.Errorf("resolution batch %q has invalid status %q", batch.ID, batch.Status)
	}
	view, err := s.listVisibleUnlocked()
	if err != nil {
		return fmt.Errorf("snapshot conflicts for batch %s: %w", batch.ID, err)
	}
	if err := writeJSONAtomic(s.metadataPath("applying-view.json"), view); err != nil {
		return fmt.Errorf("publish applying conflict view: %w", err)
	}
	batch.Status = "applying"
	if err := writeJSONAtomic(s.metadataPath("pending.json"), batch); err != nil {
		return fmt.Errorf("mark resolution batch applying: %w", err)
	}
	journal := transactionJournal{BatchID: batch.ID}
	fail := func(cause error) error {
		rollbackErr := rollbackEntries(journal.Entries)
		batch.Status = "failed"
		batch.Error = errors.Join(cause, rollbackErr).Error()
		writeErr := writeJSONAtomic(s.metadataPath("failed.json"), batch)
		clearErr := clearResolutionMetadata(
			s.metadataPath("pending.json"),
			s.metadataPath("applying-view.json"),
			s.metadataPath("transaction.json"),
		)
		return errors.Join(cause, rollbackErr, writeErr, clearErr)
	}
	for _, selection := range batch.Selections {
		record, err := s.readRecord(selection.ID)
		if err != nil {
			return fail(fmt.Errorf("load conflict %s: %w", selection.ID, err))
		}
		current, err := s.revisionUnlocked(selection.ID)
		if err != nil || current != selection.Revision {
			if err == nil {
				err = fmt.Errorf("stale revision")
			}
			return fail(fmt.Errorf("conflict %s revision changed: %w", selection.ID, err))
		}
		var data []byte
		if selection.Choice == ChoiceMerged {
			data = selection.Content
		} else {
			data, err = os.ReadFile(filepath.Join(s.localRoot, selection.ID, string(selection.Choice)))
			if err != nil {
				return fail(fmt.Errorf("read conflict %s %s variant: %w", selection.ID, selection.Choice, err))
			}
		}
		if s.scanner != nil {
			if err := s.scanner(record, string(selection.Choice), data); err != nil {
				return fail(fmt.Errorf("scan conflict %s %s: %w", selection.ID, selection.Choice, err))
			}
		}
		canonical := filepath.Join(s.repoDir, filepath.FromSlash(record.RepoRel))
		entry := transactionEntry{Path: canonical}
		original, readErr := os.ReadFile(canonical)
		if readErr == nil {
			entry.Exists, entry.Data = true, original
		} else if !errors.Is(readErr, os.ErrNotExist) {
			return fail(fmt.Errorf("read canonical %s: %w", record.RepoRel, readErr))
		}
		journal.Entries = append(journal.Entries, entry)
		if err := writeJSONAtomic(s.metadataPath("transaction.json"), journal); err != nil {
			return fail(fmt.Errorf("journal canonical %s: %w", record.RepoRel, err))
		}
		if err := writeAtomic(canonical, data, 0o600); err != nil {
			return fail(fmt.Errorf("write resolved conflict %s: %w", selection.ID, err))
		}
	}
	for _, selection := range batch.Selections {
		if err := os.RemoveAll(filepath.Join(s.localRoot, selection.ID)); err != nil {
			return fail(fmt.Errorf("remove local conflict %s: %w", selection.ID, err))
		}
		if err := os.RemoveAll(filepath.Join(s.repoConflictRoot(), selection.ID)); err != nil {
			return fail(fmt.Errorf("remove repository conflict %s: %w", selection.ID, err))
		}
	}
	if err := os.Remove(s.metadataPath("pending.json")); err != nil {
		return fail(fmt.Errorf("clear pending resolution batch: %w", err))
	}
	if err := clearResolutionMetadata(
		s.metadataPath("failed.json"),
		s.metadataPath("applying-view.json"),
		s.metadataPath("transaction.json"),
	); err != nil {
		return err
	}
	return nil
}

// RecoverTransactions rolls back an interrupted applying batch.
func (s *Store) RecoverTransactions() error {
	s.locks.transaction.Lock()
	defer s.locks.transaction.Unlock()
	s.locks.metadata.Lock()
	defer s.locks.metadata.Unlock()
	batch, err := s.readBatchUnlocked("pending.json")
	if err != nil || batch == nil || batch.Status != "applying" {
		return err
	}
	data, err := os.ReadFile(s.metadataPath("transaction.json"))
	if err != nil {
		batch.Status = "failed"
		batch.Error = fmt.Errorf("read resolution transaction journal: %w", err).Error()
		writeErr := writeJSONAtomic(s.metadataPath("failed.json"), batch)
		clearErr := clearResolutionMetadata(
			s.metadataPath("pending.json"),
			s.metadataPath("applying-view.json"),
			s.metadataPath("transaction.json"),
		)
		return errors.Join(fmt.Errorf("read resolution transaction journal: %w", err), writeErr, clearErr)
	}
	var journal transactionJournal
	if err := json.Unmarshal(data, &journal); err != nil {
		return fmt.Errorf("parse resolution transaction journal: %w", err)
	}
	if err := rollbackEntries(journal.Entries); err != nil {
		return fmt.Errorf("rollback resolution batch %s: %w", batch.ID, err)
	}
	batch.Status = "failed"
	batch.Error = "recovered interrupted resolution transaction"
	if err := writeJSONAtomic(s.metadataPath("failed.json"), batch); err != nil {
		return err
	}
	return clearResolutionMetadata(
		s.metadataPath("pending.json"),
		s.metadataPath("applying-view.json"),
		s.metadataPath("transaction.json"),
	)
}

func rollbackEntries(entries []transactionEntry) error {
	var joined error
	for i := len(entries) - 1; i >= 0; i-- {
		entry := entries[i]
		var err error
		if entry.Exists {
			err = writeAtomic(entry.Path, entry.Data, 0o600)
		} else {
			err = os.Remove(entry.Path)
			if errors.Is(err, os.ErrNotExist) {
				err = nil
			}
		}
		joined = errors.Join(joined, err)
	}
	return joined
}

func clearResolutionMetadata(paths ...string) error {
	var joined error
	for _, metadataPath := range paths {
		err := os.Remove(metadataPath)
		if errors.Is(err, os.ErrNotExist) {
			err = nil
		}
		joined = errors.Join(joined, err)
	}
	return joined
}
