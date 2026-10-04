// Package repository validates and prepares remote synchronization repositories.
package repository

import (
	"errors"
	"net/url"
	"regexp"
	"strings"
)

type Protocol string

const (
	HTTPS Protocol = "https"
	SSH   Protocol = "ssh"
)

type GitHubURL struct {
	Protocol   Protocol
	SSHHost    string
	Owner      string
	Repository string
	CloneURL   string
}

var (
	scpPattern = regexp.MustCompile(
		`^git@([A-Za-z0-9](?:[A-Za-z0-9._-]*[A-Za-z0-9])?):([^/]+)/([^/]+?)(?:\.git)?$`,
	)
	sshHostPattern = regexp.MustCompile(
		`^[A-Za-z0-9](?:[A-Za-z0-9._-]*[A-Za-z0-9])?$`,
	)
	segmentPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)
)

func ParseGitHubURL(raw string) (GitHubURL, error) {
	if raw == "" || strings.TrimSpace(raw) != raw {
		return GitHubURL{}, errors.New("invalid GitHub repository URL")
	}
	if match := scpPattern.FindStringSubmatch(raw); match != nil {
		if !validSegments(match[2], match[3]) {
			return GitHubURL{}, errors.New("invalid repository path")
		}
		return GitHubURL{
			Protocol:   SSH,
			SSHHost:    match[1],
			Owner:      match[2],
			Repository: match[3],
			CloneURL:   "git@" + match[1] + ":" + match[2] + "/" + match[3] + ".git",
		}, nil
	}

	parsed, err := url.Parse(raw)
	if err != nil || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.RawPath != "" {
		return GitHubURL{}, errors.New("invalid GitHub repository URL")
	}
	parts := strings.Split(strings.Trim(strings.TrimSuffix(parsed.Path, ".git"), "/"), "/")
	if len(parts) != 2 || !validSegments(parts[0], parts[1]) {
		return GitHubURL{}, errors.New("repository URL must contain owner and repository")
	}

	switch parsed.Scheme {
	case "https":
		if !strings.EqualFold(parsed.Hostname(), "github.com") || parsed.Port() != "" {
			return GitHubURL{}, errors.New("repository host must be github.com")
		}
		if parsed.User != nil {
			return GitHubURL{}, errors.New("credentials must not be embedded in repository URL")
		}
		return GitHubURL{
			Protocol:   HTTPS,
			Owner:      parts[0],
			Repository: parts[1],
			CloneURL:   "https://github.com/" + parts[0] + "/" + parts[1] + ".git",
		}, nil
	case "ssh":
		sshHost := parsed.Hostname()
		if parsed.Port() != "" || !sshHostPattern.MatchString(sshHost) {
			return GitHubURL{}, errors.New("invalid SSH host or alias")
		}
		if parsed.User == nil || parsed.User.Username() != "git" {
			return GitHubURL{}, errors.New("SSH GitHub URL must use git user")
		}
		if _, hasPassword := parsed.User.Password(); hasPassword {
			return GitHubURL{}, errors.New("credentials must not be embedded in repository URL")
		}
		return GitHubURL{
			Protocol:   SSH,
			SSHHost:    sshHost,
			Owner:      parts[0],
			Repository: parts[1],
			CloneURL:   "ssh://git@" + sshHost + "/" + parts[0] + "/" + parts[1] + ".git",
		}, nil
	default:
		return GitHubURL{}, errors.New("supported protocols are HTTPS and SSH")
	}
}

func validSegments(owner, repository string) bool {
	return validSegment(owner) && validSegment(repository)
}

func validSegment(value string) bool {
	return value != "." && value != ".." && segmentPattern.MatchString(value)
}
