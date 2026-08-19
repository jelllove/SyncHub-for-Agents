// Package sshprobe discovers SSH identities and verifies unattended repository
// access without prompting the desktop user in a terminal.
package sshprobe

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/qinqingxu/synchub-for-agents/internal/processattr"
)

type Runner interface {
	Run(ctx context.Context, name string, args ...string) (stdout, stderr string, err error)
}

type SystemRunner struct{}

func (SystemRunner) Run(ctx context.Context, name string, args ...string) (string, string, error) {
	command := exec.CommandContext(ctx, name, args...)
	processattr.HideWindow(command)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	err := command.Run()
	return stdout.String(), stderr.String(), err
}

type Status struct {
	SSHAvailable     bool     `json:"sshAvailable"`
	IdentityFiles    []string `json:"identityFiles"`
	AgentHasKeys     bool     `json:"agentHasKeys"`
	RepositoryAccess bool     `json:"repositoryAccess"`
	Message          string   `json:"message"`
}

func Check(ctx context.Context, runner Runner, repositoryURL string) (Status, error) {
	if runner == nil {
		return Status{}, errors.New("SSH runner is required")
	}
	if repositoryURL == "" {
		return Status{}, errors.New("repository URL is required")
	}
	status := Status{}
	if _, stderr, err := runner.Run(ctx, "ssh", "-V"); err != nil {
		status.Message = commandMessage("OpenSSH is not available", stderr)
		return status, nil
	}
	status.SSHAvailable = true

	config, stderr, err := runner.Run(ctx, "ssh", "-G", "github.com")
	if err != nil {
		status.Message = commandMessage("Could not read SSH configuration", stderr)
		return status, nil
	}
	status.IdentityFiles = parseIdentityFiles(config)
	if keys, _, err := runner.Run(ctx, "ssh-add", "-L"); err == nil && strings.TrimSpace(keys) != "" {
		status.AgentHasKeys = true
	}

	const sshCommand = "ssh -o BatchMode=yes -o StrictHostKeyChecking=yes -o ConnectTimeout=10"
	_, stderr, err = runner.Run(
		ctx,
		"git",
		"-c", "credential.interactive=false",
		"-c", "core.sshCommand="+sshCommand,
		"ls-remote", repositoryURL, "HEAD",
	)
	if err != nil {
		status.Message = commandMessage("GitHub repository access failed", stderr)
		return status, nil
	}
	status.RepositoryAccess = true
	status.Message = "SSH access is ready"
	return status, nil
}

func GenerateKey(ctx context.Context, runner Runner, path, comment string) (string, error) {
	if runner == nil {
		return "", errors.New("SSH runner is required")
	}
	if path == "" || comment == "" {
		return "", errors.New("key path and comment are required")
	}
	if _, err := os.Stat(path); err == nil {
		return "", errors.New("private key already exists")
	} else if !os.IsNotExist(err) {
		return "", err
	}
	if _, err := os.Stat(path + ".pub"); err == nil {
		return "", errors.New("public key already exists")
	} else if !os.IsNotExist(err) {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return "", err
	}
	_, stderr, err := runner.Run(
		ctx,
		"ssh-keygen",
		"-q", "-t", "ed25519", "-a", "64",
		"-C", comment,
		"-f", path,
		"-N", "",
	)
	if err != nil {
		return "", errors.New(commandMessage("SSH key generation failed", stderr))
	}
	publicKey, err := os.ReadFile(path + ".pub")
	if err != nil {
		return "", err
	}
	result := strings.TrimSpace(string(publicKey))
	if !strings.HasPrefix(result, "ssh-") {
		return "", errors.New("ssh-keygen returned an invalid public key")
	}
	return result, nil
}

func parseIdentityFiles(config string) []string {
	var result []string
	for _, line := range strings.Split(config, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && strings.EqualFold(fields[0], "identityfile") {
			result = append(result, fields[1])
		}
	}
	return result
}

func commandMessage(prefix, stderr string) string {
	message := strings.TrimSpace(stderr)
	if message == "" {
		return prefix
	}
	return prefix + ": " + message
}
