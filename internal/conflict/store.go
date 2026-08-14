package conflict

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Record struct {
	ID          string    `json:"id"`
	ResourceKey string    `json:"resourceKey"`
	RepoRel     string    `json:"repoRel"`
	CreatedAt   time.Time `json:"createdAt"`
}

type Scanner func(record Record, variant string, data []byte) error

type Store struct {
	localRoot string
	repoDir   string
	scanner   Scanner
}

func NewStore(localRoot, repoDir string, scanner Scanner) *Store {
	return &Store{localRoot: localRoot, repoDir: repoDir, scanner: scanner}
}

func (s *Store) Create(record Record, base, local, remote []byte) error {
	if record.CreatedAt.IsZero() {
		record.CreatedAt = time.Now().UTC()
	}
	if err := validateRecord(record); err != nil {
		return err
	}
	variants := []struct {
		name string
		data []byte
	}{
		{"base", base},
		{"local", local},
		{"remote", remote},
	}
	if s.scanner != nil {
		for _, variant := range variants {
			if err := s.scanner(record, variant.name, variant.data); err != nil {
				return fmt.Errorf("scan conflict %s %s: %w", record.ID, variant.name, err)
			}
		}
	}

	localFinal := filepath.Join(s.localRoot, record.ID)
	repoFinal := filepath.Join(s.repoConflictRoot(), record.ID)
	for _, final := range []string{localFinal, repoFinal} {
		if _, err := os.Stat(final); err == nil {
			return fmt.Errorf("conflict %q already exists", record.ID)
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	localTemp, err := s.writeTempBundle(s.localRoot, record, variants)
	if err != nil {
		return err
	}
	defer os.RemoveAll(localTemp)
	repoTemp, err := s.writeTempBundle(s.repoConflictRoot(), record, variants)
	if err != nil {
		return err
	}
	defer os.RemoveAll(repoTemp)

	if err := os.Rename(localTemp, localFinal); err != nil {
		return fmt.Errorf("publish local conflict bundle: %w", err)
	}
	if err := os.Rename(repoTemp, repoFinal); err != nil {
		removeErr := os.RemoveAll(localFinal)
		return errors.Join(fmt.Errorf("publish repository conflict bundle: %w", err), removeErr)
	}
	return nil
}

func (s *Store) List() ([]Record, error) {
	entries, err := os.ReadDir(s.localRoot)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	records := make([]Record, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".tmp-") {
			continue
		}
		record, err := s.readRecord(entry.Name())
		if err != nil {
			return nil, err
		}
		records = append(records, record)
	}
	sort.Slice(records, func(left, right int) bool {
		if records[left].CreatedAt.Equal(records[right].CreatedAt) {
			return records[left].ID < records[right].ID
		}
		return records[left].CreatedAt.Before(records[right].CreatedAt)
	})
	return records, nil
}

func (s *Store) MirrorFromRepo(preserveLocal map[string]struct{}) error {
	repoRoot := s.repoConflictRoot()
	entries, err := os.ReadDir(repoRoot)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	remoteIDs := map[string]struct{}{}
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".tmp-") {
			continue
		}
		id := entry.Name()
		if _, preserve := preserveLocal[id]; preserve {
			continue
		}
		record, variants, err := s.readBundle(repoRoot, id)
		if err != nil {
			return err
		}
		remoteIDs[id] = struct{}{}
		localFinal := filepath.Join(s.localRoot, id)
		if info, err := os.Lstat(localFinal); err == nil {
			if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
				return fmt.Errorf("local conflict %q is not a regular directory", id)
			}
			localRecord, localVariants, err := s.readBundle(s.localRoot, id)
			if err != nil {
				return err
			}
			if localRecord != record || !sameVariants(localVariants, variants) {
				return fmt.Errorf("local and repository conflict %q do not match", id)
			}
			continue
		} else if !errors.Is(err, os.ErrNotExist) {
			return err
		}
		localTemp, err := s.writeTempBundle(s.localRoot, record, variants)
		if err != nil {
			return err
		}
		if err := os.Rename(localTemp, localFinal); err != nil {
			_ = os.RemoveAll(localTemp)
			return fmt.Errorf("publish mirrored local conflict bundle: %w", err)
		}
	}

	localEntries, err := os.ReadDir(s.localRoot)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range localEntries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".tmp-") {
			continue
		}
		id := entry.Name()
		if _, preserve := preserveLocal[id]; preserve {
			continue
		}
		if _, exists := remoteIDs[id]; exists {
			continue
		}
		if err := os.RemoveAll(filepath.Join(s.localRoot, id)); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) readBundle(parent, id string) (
	Record,
	[]struct {
		name string
		data []byte
	},
	error,
) {
	if err := validateSegment("conflict id", id); err != nil {
		return Record{}, nil, err
	}
	data, err := os.ReadFile(filepath.Join(parent, id, "record.json"))
	if err != nil {
		return Record{}, nil, err
	}
	var record Record
	if err := json.Unmarshal(data, &record); err != nil {
		return Record{}, nil, fmt.Errorf("parse conflict %q: %w", id, err)
	}
	if err := validateRecord(record); err != nil {
		return Record{}, nil, err
	}
	if record.ID != id {
		return Record{}, nil, fmt.Errorf("conflict directory %q contains record %q", id, record.ID)
	}
	variants := make([]struct {
		name string
		data []byte
	}, 0, 3)
	for _, variant := range []string{"base", "local", "remote"} {
		data, err := os.ReadFile(filepath.Join(parent, id, variant))
		if err != nil {
			return Record{}, nil, err
		}
		if s.scanner != nil {
			if err := s.scanner(record, variant, data); err != nil {
				return Record{}, nil, fmt.Errorf("scan conflict %s %s: %w", id, variant, err)
			}
		}
		variants = append(variants, struct {
			name string
			data []byte
		}{name: variant, data: data})
	}
	return record, variants, nil
}

func sameVariants(
	left, right []struct {
		name string
		data []byte
	},
) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index].name != right[index].name ||
			!bytes.Equal(left[index].data, right[index].data) {
			return false
		}
	}
	return true
}

func (s *Store) Resolve(id string, merged []byte) error {
	if err := validateSegment("conflict id", id); err != nil {
		return err
	}

	record, err := s.readRecord(id)
	if err != nil {
		return err
	}
	if s.scanner != nil {
		if err := s.scanner(record, "merged", merged); err != nil {
			return fmt.Errorf("scan conflict %s merged: %w", record.ID, err)
		}
	}
	canonical := filepath.Join(s.repoDir, filepath.FromSlash(record.RepoRel))
	if err := writeAtomic(canonical, merged, 0o600); err != nil {
		return fmt.Errorf("write resolved conflict %s: %w", id, err)
	}
	localErr := os.RemoveAll(filepath.Join(s.localRoot, id))
	repoErr := os.RemoveAll(filepath.Join(s.repoConflictRoot(), id))
	return errors.Join(localErr, repoErr)
}

func (s *Store) ResolveVariant(id, variant string) error {
	if variant != "local" && variant != "remote" {
		return fmt.Errorf("conflict choice %q is not supported", variant)
	}
	if err := validateSegment("conflict id", id); err != nil {
		return err
	}
	if _, err := s.readRecord(id); err != nil {
		return err
	}
	data, err := os.ReadFile(filepath.Join(s.localRoot, id, variant))
	if err != nil {
		return fmt.Errorf("read conflict %s %s variant: %w", id, variant, err)
	}
	return s.Resolve(id, data)
}

func (s *Store) readRecord(id string) (Record, error) {
	if err := validateSegment("conflict id", id); err != nil {
		return Record{}, err
	}
	data, err := os.ReadFile(filepath.Join(s.localRoot, id, "record.json"))
	if errors.Is(err, os.ErrNotExist) {
		return Record{}, fmt.Errorf("conflict %q does not exist", id)
	}
	if err != nil {
		return Record{}, err
	}
	var record Record
	if err := json.Unmarshal(data, &record); err != nil {
		return Record{}, fmt.Errorf("parse conflict %q: %w", id, err)
	}
	if err := validateRecord(record); err != nil {
		return Record{}, err
	}
	if record.ID != id {
		return Record{}, fmt.Errorf("conflict directory %q contains record %q", id, record.ID)
	}
	return record, nil
}

func (s *Store) writeTempBundle(
	parent string,
	record Record,
	variants []struct {
		name string
		data []byte
	},
) (string, error) {
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return "", err
	}
	temp, err := os.MkdirTemp(parent, ".tmp-"+record.ID+"-*")
	if err != nil {
		return "", err
	}
	success := false
	defer func() {
		if !success {
			_ = os.RemoveAll(temp)
		}
	}()
	recordData, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return "", err
	}
	recordData = append(recordData, '\n')
	if err := os.WriteFile(filepath.Join(temp, "record.json"), recordData, 0o600); err != nil {
		return "", err
	}
	for _, variant := range variants {
		if err := os.WriteFile(filepath.Join(temp, variant.name), variant.data, 0o600); err != nil {
			return "", err
		}
	}
	success = true
	return temp, nil
}

func (s *Store) repoConflictRoot() string {
	return filepath.Join(s.repoDir, "agents", "_portable", "config", "conflicts")
}

func validateRecord(record Record) error {
	if err := validateSegment("conflict id", record.ID); err != nil {
		return err
	}
	if strings.TrimSpace(record.ResourceKey) == "" {
		return fmt.Errorf("conflict %q: missing resource key", record.ID)
	}
	if err := validateRepoPath(record.RepoRel); err != nil {
		return err
	}
	if !strings.HasPrefix(record.RepoRel, "agents/") ||
		strings.HasPrefix(record.RepoRel, "agents/_portable/config/conflicts/") {
		return fmt.Errorf("conflict %q: invalid canonical repository path %q", record.ID, record.RepoRel)
	}
	return nil
}

func validateSegment(kind, value string) error {
	if strings.TrimSpace(value) == "" ||
		value == "." ||
		value == ".." ||
		strings.ContainsAny(value, `/\`) {
		return fmt.Errorf("invalid %s %q", kind, value)
	}
	return nil
}

func validateRepoPath(value string) error {
	if strings.TrimSpace(value) == "" ||
		strings.Contains(value, `\`) ||
		path.IsAbs(value) ||
		path.Clean(value) != value {
		return fmt.Errorf("invalid repository path %q", value)
	}
	for _, segment := range strings.Split(value, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return fmt.Errorf("invalid repository path %q", value)
		}
	}
	return nil
}

func writeAtomic(filename string, data []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(filename), 0o700); err != nil {
		return err
	}
	temp, err := os.CreateTemp(filepath.Dir(filename), "."+filepath.Base(filename)+".tmp-*")
	if err != nil {
		return err
	}
	tempName := temp.Name()
	defer os.Remove(tempName)
	if err := temp.Chmod(mode); err != nil {
		temp.Close()
		return err
	}
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Sync(); err != nil {
		temp.Close()
		return err
	}
	if err := temp.Close(); err != nil {
		return err
	}
	return os.Rename(tempName, filename)
}
