// Package gitclient wraps the git CLI. It is the only package that shells
// out to git.
package gitclient

import (
	"bytes"
	"fmt"
	"os/exec"
	"strings"
)

// Client operates on a git working directory at Dir.
type Client struct {
	Dir string
}

func (c *Client) run(args ...string) (string, error) {
	cmd := exec.Command("git", args...)
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

// Clone clones url into dir. Dir on the client should match dir.
func (c *Client) Clone(url, dir string) error {
	_, err := c.run("clone", url, dir)
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

// HasChanges reports whether the working tree has any staged or unstaged
// changes (including untracked files).
func (c *Client) HasChanges() (bool, error) {
	out, err := c.run("status", "--porcelain")
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(out) != "", nil
}
