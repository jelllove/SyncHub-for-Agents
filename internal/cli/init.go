package cli

import (
	"os"
	"path/filepath"

	"github.com/qinqingxu/synchub-for-agents/internal/config"
	"github.com/qinqingxu/synchub-for-agents/internal/repository"
)

// RepositoryInitializer prepares a local sync repository from a remote.
type RepositoryInitializer interface {
	Initialize(remote, dir string) error
}

// RunInit scaffolds the synchub home, clones the data repo, detects agents, and
// writes a default config.
func RunInit(home, repoURL string, initializer RepositoryInitializer) error {
	parsed, err := repository.ParseGitHubURL(repoURL)
	if err != nil {
		return err
	}
	repoURL = parsed.CloneURL
	for _, dir := range []string{home, ProvidersDir(home), LogsDir(home)} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return err
		}
	}

	repo := RepoDir(home)
	if err := initializer.Initialize(repoURL, repo); err != nil {
		return err
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

// ensureGitAttributes writes a `.gitattributes` into the data repo (if the repo
// directory exists) so git treats every stored file as binary and never
// rewrites CRLF/LF. synchub copies agent files byte-for-byte, so git must not
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
