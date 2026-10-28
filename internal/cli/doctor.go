package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/qinqingxu/synchub-for-agents/internal/config"
	"github.com/qinqingxu/synchub-for-agents/internal/repository"
)

type DoctorCheck struct {
	Name    string `json:"name"`
	OK      bool   `json:"ok"`
	Message string `json:"message"`
}

func Doctor(home, goos string) []DoctorCheck {
	checks := make([]DoctorCheck, 0, 4)
	record := func(name string, err error, success string) {
		check := DoctorCheck{Name: name, OK: err == nil, Message: success}
		if err != nil {
			check.Message = err.Error()
		}
		checks = append(checks, check)
	}
	_, err := exec.LookPath("git")
	record("git", err, "Git is available on PATH")
	cfg, err := config.Load(ConfigPath(home))
	if err == nil {
		_, err = repository.ParseGitHubURL(cfg.RepoURL)
	}
	if err == nil && (cfg.SyncIntervalMinutes < 1 || cfg.TrashGraceDays < 1) {
		err = fmt.Errorf("sync interval and trash retention must be positive")
	}
	record("configuration", err, "Configuration is valid")
	if err != nil {
		return checks
	}
	providers, err := LoadProviders(home)
	userHome, homeErr := os.UserHomeDir()
	if err == nil {
		err = homeErr
	}
	if err == nil {
		_, err = BuildResourceSpecs(cfg, providers, goos, userHome)
	}
	record("resources", err, "Enabled resource declarations are valid")
	repo, err := ResolveRepoDir(home, cfg, goos, userHome)
	if err == nil {
		info, statErr := os.Stat(filepath.Join(repo, ".git"))
		err = statErr
		if err == nil && !info.IsDir() && !info.Mode().IsRegular() {
			err = fmt.Errorf("repository Git metadata is not a file or directory")
		}
	}
	record("repository", err, "Local repository Git metadata exists; no network authentication was tested")
	if !cfg.FirstSync.Completed && cfg.FirstSync.Strategy == config.FirstSyncStrategyChoose {
		record("firstSync", fmt.Errorf("select an initial synchronization strategy in Desktop before syncing"), "")
	}
	return checks
}
