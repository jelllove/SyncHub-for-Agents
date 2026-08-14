package state

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

type BaseStore struct {
	root string
	mu   sync.Mutex
}

func NewBaseStore(root string) *BaseStore {
	return &BaseStore{root: filepath.Join(root, "base")}
}

func (s *BaseStore) Put(repoRel string, data []byte) error {
	if err := validateBaseRepoPath(repoRel); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	index, err := s.loadIndex()
	if err != nil {
		return err
	}
	hash, err := s.putObject(data)
	if err != nil {
		return err
	}
	index[repoRel] = hash
	return s.saveIndex(index)
}

func (s *BaseStore) Get(repoRel string) ([]byte, bool, error) {
	if err := validateBaseRepoPath(repoRel); err != nil {
		return nil, false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	index, err := s.loadIndex()
	if err != nil {
		return nil, false, err
	}
	hash, ok := index[repoRel]
	if !ok {
		return nil, false, nil
	}
	if err := validateObjectHash(hash); err != nil {
		return nil, false, err
	}
	data, err := os.ReadFile(filepath.Join(s.root, "objects", hash))
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, fmt.Errorf("base object %s for %q is missing", hash, repoRel)
	}
	if err != nil {
		return nil, false, err
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != hash {
		return nil, false, fmt.Errorf("base object %s for %q failed integrity check", hash, repoRel)
	}
	return data, true, nil
}

func (s *BaseStore) Delete(repoRel string) error {
	if err := validateBaseRepoPath(repoRel); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	index, err := s.loadIndex()
	if err != nil {
		return err
	}
	if _, ok := index[repoRel]; !ok {
		return nil
	}
	delete(index, repoRel)
	return s.saveIndex(index)
}

func (s *BaseStore) CaptureRepo(repoDir string, snapshot Snapshot) error {
	paths := make([]string, 0, len(snapshot))
	for repoRel := range snapshot {
		if err := validateBaseRepoPath(repoRel); err != nil {
			return err
		}
		paths = append(paths, repoRel)
	}
	sort.Strings(paths)
	content := make(map[string][]byte, len(paths))
	for _, repoRel := range paths {
		data, err := os.ReadFile(filepath.Join(repoDir, filepath.FromSlash(repoRel)))
		if err != nil {
			return fmt.Errorf("capture base %q: %w", repoRel, err)
		}
		content[repoRel] = data
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	index, err := s.loadIndex()
	if err != nil {
		return err
	}
	for _, repoRel := range paths {
		hash, err := s.putObject(content[repoRel])
		if err != nil {
			return err
		}
		index[repoRel] = hash
	}
	for repoRel := range index {
		if _, keep := snapshot[repoRel]; !keep {
			delete(index, repoRel)
		}
	}
	return s.saveIndex(index)
}

func (s *BaseStore) putObject(data []byte) (string, error) {
	sum := sha256.Sum256(data)
	hash := hex.EncodeToString(sum[:])
	objects := filepath.Join(s.root, "objects")
	if err := os.MkdirAll(objects, 0o700); err != nil {
		return "", err
	}
	target := filepath.Join(objects, hash)
	if _, err := os.Stat(target); err == nil {
		return hash, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	if err := writeFileAtomic(target, data, 0o600); err != nil {
		return "", err
	}
	return hash, nil
}

func (s *BaseStore) loadIndex() (map[string]string, error) {
	data, err := os.ReadFile(filepath.Join(s.root, "index.json"))
	if errors.Is(err, os.ErrNotExist) {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, err
	}
	index := map[string]string{}
	if err := json.Unmarshal(data, &index); err != nil {
		return nil, fmt.Errorf("load base index: %w", err)
	}
	for repoRel, hash := range index {
		if err := validateBaseRepoPath(repoRel); err != nil {
			return nil, fmt.Errorf("load base index: %w", err)
		}
		if err := validateObjectHash(hash); err != nil {
			return nil, fmt.Errorf("load base index: %w", err)
		}
	}
	return index, nil
}

func (s *BaseStore) saveIndex(index map[string]string) error {
	if err := os.MkdirAll(s.root, 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(index, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	return writeFileAtomic(filepath.Join(s.root, "index.json"), data, 0o600)
}

func writeFileAtomic(filename string, data []byte, mode os.FileMode) error {
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

func validateBaseRepoPath(repoRel string) error {
	if strings.TrimSpace(repoRel) == "" ||
		strings.Contains(repoRel, `\`) ||
		path.IsAbs(repoRel) ||
		isDrivePath(repoRel) ||
		path.Clean(repoRel) != repoRel {
		return fmt.Errorf("invalid repository path %q", repoRel)
	}
	for _, segment := range strings.Split(repoRel, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return fmt.Errorf("invalid repository path %q", repoRel)
		}
	}
	return nil
}

func isDrivePath(value string) bool {
	return len(value) >= 3 &&
		((value[0] >= 'A' && value[0] <= 'Z') || (value[0] >= 'a' && value[0] <= 'z')) &&
		value[1] == ':' &&
		value[2] == '/'
}

func validateObjectHash(hash string) error {
	if len(hash) != sha256.Size*2 {
		return fmt.Errorf("invalid base object hash %q", hash)
	}
	if _, err := hex.DecodeString(hash); err != nil {
		return fmt.Errorf("invalid base object hash %q", hash)
	}
	return nil
}
