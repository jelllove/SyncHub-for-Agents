package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/qinqingxu/synchub-for-agents/internal/config"
	"github.com/qinqingxu/synchub-for-agents/internal/repository"
)

// RepositoryInitializer prepares a local sync repository from a remote.
type RepositoryInitializer interface {
	Initialize(remote, dir string) error
}

type InitOptions struct {
	FirstSync string
	Agents    []string
}

func RunInitWithOptions(home, repoURL string, options InitOptions, initializer RepositoryInitializer) error {
	switch options.FirstSync {
	case config.FirstSyncStrategyUseCloud, config.FirstSyncStrategyMerge, config.FirstSyncStrategyUseLocal:
	default:
		return fmt.Errorf("init requires --first-sync use-cloud, merge-cloud-local or use-local")
	}
	if _, err := os.Lstat(ConfigPath(home)); err == nil {
		return fmt.Errorf("profile is already configured; initialization will not overwrite it")
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect existing configuration: %w", err)
	}
	if info, err := os.Lstat(RepoDir(home)); err == nil {
		if !info.IsDir() {
			return fmt.Errorf("existing repository path is not a directory")
		}
		entries, err := os.ReadDir(RepoDir(home))
		if err != nil {
			return fmt.Errorf("inspect existing repository: %w", err)
		}
		if len(entries) != 0 {
			return fmt.Errorf("existing repository is not empty; initialization will not overwrite it")
		}
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("inspect repository path: %w", err)
	}
	return runInit(home, repoURL, options, initializer)
}

// RunInit scaffolds the synchub home, clones the data repo, detects agents, and
// writes a default config.
func RunInit(home, repoURL string, initializer RepositoryInitializer) error {
	return runInit(home, repoURL, InitOptions{}, initializer)
}

func runInit(home, repoURL string, options InitOptions, initializer RepositoryInitializer) error {
	parsed, err := repository.ParseGitHubURL(repoURL)
	if err != nil {
		return err
	}
	repoURL = parsed.CloneURL
	providers, err := LoadProviders(home)
	if err != nil {
		return err
	}
	names := make([]string, 0, len(providers))
	known := make(map[string]bool, len(providers))
	for _, p := range providers {
		names = append(names, p.Name)
		known[p.Name] = true
	}
	for _, name := range options.Agents {
		if !known[name] {
			return fmt.Errorf("unknown agent %q", name)
		}
	}
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

	cfg := config.Default(names)
	cfg.RepoURL = repoURL
	if options.FirstSync != "" {
		cfg.FirstSync = config.FirstSyncPolicy{Strategy: options.FirstSync}
	}
	if len(options.Agents) != 0 {
		for name := range cfg.Agents {
			cfg.Agents[name] = false
		}
		for _, name := range options.Agents {
			cfg.Agents[name] = true
		}
	}
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
