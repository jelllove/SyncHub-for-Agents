package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/qinqingxu/acsync/internal/config"
)

// Cloner clones a git URL into a directory.
type Cloner interface {
	Clone(url, dir string) error
}

// RunInit scaffolds the acsync home, clones the data repo, detects agents, and
// writes a default config.
func RunInit(home, repoURL string, cloner Cloner) error {
	for _, dir := range []string{home, ProvidersDir(home), LogsDir(home)} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}

	repo := RepoDir(home)
	empty, err := isEmptyOrMissing(repo)
	if err != nil {
		return err
	}
	if empty {
		if err := os.RemoveAll(repo); err != nil {
			return err
		}
		if err := cloner.Clone(repoURL, repo); err != nil {
			return fmt.Errorf("clone %s: %w", repoURL, err)
		}
	}

	// Store agent files byte-for-byte: never let git rewrite line endings.
	if err := ensureGitAttributes(repo); err != nil {
		return err
	}

	providers, err := LoadProviders(home)
	if err != nil {
		return err
	}
	names := make([]string, 0, len(providers))
	for _, p := range providers {
		names = append(names, p.Name)
	}

	cfg := config.Default(names)
	cfg.RepoURL = repoURL
	return config.Save(ConfigPath(home), cfg)
}

func isEmptyOrMissing(dir string) (bool, error) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	return len(entries) == 0, nil
}

// ensureGitAttributes writes a `.gitattributes` into the data repo (if the repo
// directory exists) so git treats every stored file as binary and never
// rewrites CRLF/LF. acsync copies agent files byte-for-byte, so git must not
// touch their bytes. Idempotent: a no-op if the file already exists.
func ensureGitAttributes(repo string) error {
	if _, err := os.Stat(repo); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	path := filepath.Join(repo, ".gitattributes")
	if _, err := os.Stat(path); err == nil {
		return nil
	}
	return os.WriteFile(path, []byte("* -text\n"), 0o644)
}
