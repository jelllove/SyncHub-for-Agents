package cli

import (
	"errors"
	"net/url"
	"os"
	"path/filepath"

	"github.com/qinqingxu/synchub-for-agents/internal/auth"
	"github.com/qinqingxu/synchub-for-agents/internal/gitclient"
	"github.com/qinqingxu/synchub-for-agents/internal/repository"
)

func NewGitClient(home, repositoryURL, dir string) (*gitclient.Client, error) {
	client := &gitclient.Client{Dir: dir}
	parsed, err := repository.ParseGitHubURL(repositoryURL)
	if err != nil {
		if isLocalRepositoryURL(repositoryURL) {
			return client, nil
		}
		return nil, err
	}
	if parsed.Protocol != repository.HTTPS {
		return client, nil
	}
	if _, err := auth.LoadMetadata(AuthMetadataPath(home)); err != nil {
		if os.IsNotExist(err) || errors.Is(err, auth.ErrNotFound) {
			return client, nil
		}
		return nil, err
	}
	executable, err := os.Executable()
	if err != nil {
		return nil, err
	}
	client.AuthMode = gitclient.AuthOAuth
	client.Executable = executable
	return client, nil
}

func isLocalRepositoryURL(repositoryURL string) bool {
	if filepath.IsAbs(repositoryURL) {
		return true
	}
	parsed, err := url.Parse(repositoryURL)
	return err == nil && parsed.Scheme == "file"
}
