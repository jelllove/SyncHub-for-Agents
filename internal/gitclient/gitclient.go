// Package gitclient wraps the git CLI. It is the only package that shells
// out to git.
package gitclient

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/qinqingxu/acsync/internal/processattr"
)

type AuthMode string

const (
	AuthSystem AuthMode = "system"
	AuthOAuth  AuthMode = "oauth"
)

type CommandFactory func(name string, args ...string) *exec.Cmd

// Client operates on a git working directory at Dir.
type Client struct {
	Dir        string
	AuthMode   AuthMode
	Executable string
	Command    CommandFactory
}

func (c *Client) run(args ...string) (string, error) {
	cmd, err := c.command(args...)
	if err != nil {
		return "", err
	}
	if c.Dir != "" && (len(args) == 0 || args[0] != "clone") {
		cmd.Dir = c.Dir
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return stdout.String(), fmt.Errorf("git %s: %w: %s",
			strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return stdout.String(), nil
}

func (c *Client) command(args ...string) (*exec.Cmd, error) {
	gitArgs := args
	oauthNetwork := c.AuthMode == AuthOAuth && isNetworkOperation(args)
	if oauthNetwork {
		if !filepath.IsAbs(c.Executable) {
			return nil, errors.New("OAuth Git authentication requires an absolute executable path")
		}
		helper := "!" + strconv.Quote(filepath.ToSlash(c.Executable)) + " --git-credential"
		gitArgs = append([]string{
			"-c", "credential.helper=",
			"-c", "credential.https://github.com.helper=" + helper,
			"-c", "credential.interactive=false",
		}, args...)
	}

	factory := c.Command
	if factory == nil {
		factory = exec.Command
	}
	command := factory("git", gitArgs...)
	processattr.HideWindow(command)
	if oauthNetwork {
		command.Env = sanitizedGitEnvironment(os.Environ())
		command.Env = append(command.Env, "GIT_TERMINAL_PROMPT=0")
	}
	return command, nil
}

func isNetworkOperation(args []string) bool {
	if len(args) == 0 {
		return false
	}
	switch args[0] {
	case "clone", "fetch", "ls-remote", "pull", "push":
		return true
	default:
		return false
	}
}

func sanitizedGitEnvironment(environment []string) []string {
	result := make([]string, 0, len(environment))
	for _, value := range environment {
		name, _, _ := strings.Cut(value, "=")
		upper := strings.ToUpper(name)
		if strings.HasPrefix(upper, "GIT_TRACE") ||
			upper == "GIT_CURL_VERBOSE" ||
			upper == "GIT_TERMINAL_PROMPT" {
			continue
		}
		result = append(result, value)
	}
	return result
}

// Clone clones url into dir. Dir on the client should match dir.
func (c *Client) Clone(url, dir string) error {
	_, err := c.run("clone", "-c", "core.autocrlf=false", url, dir)
	return err
}

// PullRebase runs git pull --rebase from origin.
func (c *Client) PullRebase() error {
	_, err := c.run("pull", "--rebase")
	return err
}

// AddAll stages all changes including deletions.
func (c *Client) AddAll() error {
	_, err := c.run("add", "-A")
	return err
}

// Commit records staged changes. It is a no-op error if nothing is staged;
// callers should check HasChanges first.
func (c *Client) Commit(message string) error {
	_, err := c.run("commit", "-m", message)
	return err
}

// Push pushes the current branch to origin.
func (c *Client) Push() error {
	_, err := c.run("push")
	return err
}

func (c *Client) FetchOrigin() error {
	_, err := c.run("fetch", "origin")
	return err
}

func (c *Client) ResetKeepUpstream() error {
	_, err := c.run("reset", "--keep", "@{upstream}")
	return err
}

// LsRemote returns the refs advertised by a remote.
func (c *Client) LsRemote(remote string) (string, error) {
	return c.run("ls-remote", remote)
}

// HasHEAD reports whether the current repository has at least one commit.
func (c *Client) HasHEAD() (bool, error) {
	_, err := c.run("rev-parse", "--verify", "HEAD")
	if err == nil {
		return true, nil
	}
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		return false, nil
	}
	return false, err
}

// CheckoutBranch resets or creates a local branch at the current worktree.
func (c *Client) CheckoutBranch(branch string) error {
	_, err := c.run("checkout", "-B", branch)
	return err
}

// LocalConfig returns a repository-local Git configuration value.
func (c *Client) LocalConfig(key string) (string, bool, error) {
	value, err := c.run("config", "--local", "--get", key)
	if err == nil {
		return strings.TrimSpace(value), true, nil
	}
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		return "", false, nil
	}
	return "", false, err
}

// SetLocalConfig sets one repository-local Git configuration value.
func (c *Client) SetLocalConfig(key, value string) error {
	_, err := c.run("config", "--local", key, value)
	return err
}

// PushUpstream pushes a branch and records its origin upstream.
func (c *Client) PushUpstream(remote, branch string) error {
	_, err := c.run("push", "-u", remote, branch)
	return err
}

// RemoteURL returns the configured URL for a named remote.
func (c *Client) RemoteURL(remote string) (string, bool, error) {
	value, err := c.run("remote", "get-url", remote)
	if err == nil {
		return strings.TrimSpace(value), true, nil
	}
	var exitError *exec.ExitError
	if errors.As(err, &exitError) {
		return "", false, nil
	}
	return "", false, err
}

// CurrentBranch returns the checked-out local branch name.
func (c *Client) CurrentBranch() (string, error) {
	value, err := c.run("branch", "--show-current")
	return strings.TrimSpace(value), err
}

// HasChanges reports whether the working tree has any staged or unstaged
// changes (including untracked files).
func (c *Client) HasChanges() (bool, error) {
	out, err := c.run("status", "--porcelain")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) != "", nil
}

// AheadOfUpstream reports whether HEAD contains commits not present upstream.
func (c *Client) AheadOfUpstream() (bool, error) {
	out, err := c.run("rev-list", "--count", "@{upstream}..HEAD")
	if err != nil {
		return false, err
	}
	count, err := strconv.Atoi(strings.TrimSpace(out))
	if err != nil {
		return false, fmt.Errorf("parse ahead count: %w", err)
	}
	return count > 0, nil
}
