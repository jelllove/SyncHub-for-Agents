package sharedlibrary

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode/utf8"

	"github.com/qinqingxu/synchub-for-agents/internal/resource"
	"github.com/qinqingxu/synchub-for-agents/internal/secret"
)

type file struct {
	Data []byte      `json:"data"`
	Mode fs.FileMode `json:"mode"`
}

type tree map[string]file

var credentialAssignment = regexp.MustCompile(`(?im)^\s*(?:export\s+)?["']?(?:api[_-]?key|access[_-]?token|password|secret|refresh[_-]?token)["']?\s*[:=]\s*\S+`)

func validateContent(rel string, data []byte) error {
	if len(data) > 1<<20 || !utf8.Valid(data) || strings.ContainsRune(string(data), 0) {
		return fmt.Errorf("library supports UTF-8 source files up to 1 MiB: %s", rel)
	}
	if allowed, reason := resource.DefaultFilterPolicy().Check(rel, int64(len(data)), resource.StrategySourceTree); !allowed {
		return fmt.Errorf("library resource %s blocked: %s", rel, reason)
	}
	blocked, err := secret.NewScanner([]string{"**/.env", "**/.env.*", "**/*credentials*", "**/auth.json", "**/*.pem", "**/id_rsa", "**/id_ed25519"},
		[]string{"apiKey", "token", "secret", "password", "oauth", "refresh_token"}).Scan(rel, data)
	if err != nil {
		return err
	}
	if blocked || credentialAssignment.Match(data) {
		return fmt.Errorf("library resource contains excluded or secret-looking content: %s", rel)
	}
	return nil
}

func digest(value any) string {
	data, _ := json.Marshal(value)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// Go exposes some Windows reparse points as ModeIrregular rather than symlinks.
// Neither is allowed at or below the caller's user-home boundary. Ancestors
// above that boundary may be OS aliases, such as macOS /var -> /private/var.
func noLinks(userHome, name string) error {
	home, err := filepath.Abs(userHome)
	if err != nil {
		return err
	}
	homeInfo, err := noLinkInfo(home)
	if err != nil {
		return err
	}
	absolute, err := filepath.Abs(name)
	if err != nil {
		return err
	}
	for p := absolute; ; p = filepath.Dir(p) {
		info, err := noLinkInfo(p)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(home, p)
		// Collection uses physical paths; SameFile also handles Windows short
		// names without resolving and accidentally accepting resource aliases.
		if (err == nil && rel == ".") || (homeInfo != nil && info != nil && os.SameFile(homeInfo, info)) {
			return nil
		}
		if filepath.Dir(p) == p {
			return fmt.Errorf("library path is outside user home: %s", name)
		}
	}
}

func noLinkInfo(name string) (fs.FileInfo, error) {
	info, err := os.Lstat(name)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if info.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0 {
		return nil, fmt.Errorf("library path is a symbolic link: %s; replace the alias with a reviewed real directory first", name)
	}
	return info, nil
}

func readTree(userHome, root string, directory bool) (tree, error) {
	if err := noLinks(userHome, root); err != nil {
		return nil, err
	}
	result := tree{}
	info, err := os.Lstat(root)
	if errors.Is(err, os.ErrNotExist) {
		return result, nil
	}
	if err != nil {
		return nil, err
	}
	if info.IsDir() != directory {
		return nil, fmt.Errorf("unexpected library path type: %s", root)
	}
	var total int64
	read := func(name, rel string, info fs.FileInfo) error {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("library resource is not a regular file: %s", name)
		}
		if len(result) >= 2000 || total+info.Size() > 20<<20 {
			return fmt.Errorf("library resource exceeds 2000 files or 20 MiB")
		}
		if info.Size() > 1<<20 {
			return fmt.Errorf("library source file exceeds 1 MiB: %s", name)
		}
		if allowed, reason := resource.DefaultFilterPolicy().Check(rel, info.Size(), resource.StrategySourceTree); !allowed {
			return fmt.Errorf("library resource %s blocked: %s", name, reason)
		}
		data, err := os.ReadFile(name)
		if err != nil {
			return err
		}
		if err := validateContent(rel, data); err != nil {
			return err
		}
		result[rel] = file{Data: data, Mode: info.Mode().Perm()}
		total += int64(len(data))
		return nil
	}
	if !directory {
		if err := read(root, "AGENTS.md", info); err != nil {
			return nil, err
		}
		return result, nil
	}
	err = filepath.Walk(root, func(name string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("library resource contains symbolic link: %s", name)
		}
		rel, err := filepath.Rel(root, name)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if info.IsDir() {
			if name != root {
				if allowed, reason := resource.DefaultFilterPolicy().CheckDirectory(rel); !allowed {
					return fmt.Errorf("library directory %s blocked: %s", name, reason)
				}
			}
			return nil
		}
		return read(name, rel, info)
	})
	return result, err
}

func atomicWrite(userHome, name string, data []byte, mode fs.FileMode) (err error) {
	if err := noLinks(userHome, name); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(name), 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(name), ".library-*")
	if err != nil {
		return err
	}
	temp := f.Name()
	defer func() {
		if removeErr := os.Remove(temp); removeErr != nil && !errors.Is(removeErr, os.ErrNotExist) {
			err = errors.Join(err, removeErr)
		}
	}()
	if _, err = f.Write(data); err == nil {
		err = f.Chmod(mode.Perm())
	}
	if err == nil {
		err = f.Sync()
	}
	err = errors.Join(err, f.Close())
	if err != nil {
		return err
	}
	return os.Rename(temp, name)
}

func filePath(root, rel string, directory bool) string {
	if !directory {
		return root
	}
	return filepath.Join(root, filepath.FromSlash(rel))
}

func writeTree(userHome, root string, directory bool, before, after tree) error {
	for rel, value := range after {
		if digest(before[rel]) == digest(value) {
			continue
		}
		if err := atomicWrite(userHome, filePath(root, rel, directory), value.Data, value.Mode); err != nil {
			return err
		}
	}
	for rel := range before {
		if _, exists := after[rel]; exists {
			continue
		}
		name := filePath(root, rel, directory)
		if err := noLinks(userHome, name); err != nil {
			return err
		}
		if err := os.Remove(name); err != nil {
			return err
		}
	}
	return nil
}
