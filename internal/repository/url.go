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
	Owner      string
	Repository string
	CloneURL   string
}

var (
	scpPattern     = regexp.MustCompile(`^git@github\.com:([^/]+)/([^/]+?)(?:\.git)?$`)
	segmentPattern = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)
)

func ParseGitHubURL(raw string) (GitHubURL, error) {
	if raw == "" || strings.TrimSpace(raw) != raw {
		return GitHubURL{}, errors.New("invalid GitHub repository URL")
	}
	if match := scpPattern.FindStringSubmatch(raw); match != nil {
		if !validSegments(match[1], match[2]) {
			return GitHubURL{}, errors.New("invalid repository path")
		}
		return GitHubURL{
			Protocol:   SSH,
			Owner:      match[1],
			Repository: match[2],
			CloneURL:   "git@github.com:" + match[1] + "/" + match[2] + ".git",
		}, nil
	}

	parsed, err := url.Parse(raw)
	if err != nil || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.RawPath != "" {
		return GitHubURL{}, errors.New("invalid GitHub repository URL")
	}
	if !strings.EqualFold(parsed.Hostname(), "github.com") || parsed.Port() != "" {
		return GitHubURL{}, errors.New("repository host must be github.com")
	}
	parts := strings.Split(strings.Trim(strings.TrimSuffix(parsed.Path, ".git"), "/"), "/")
	if len(parts) != 2 || !validSegments(parts[0], parts[1]) {
		return GitHubURL{}, errors.New("repository URL must contain owner and repository")
	}

	switch parsed.Scheme {
	case "https":
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
		if parsed.User == nil || parsed.User.Username() != "git" {
			return GitHubURL{}, errors.New("SSH GitHub URL must use git user")
		}
		if _, hasPassword := parsed.User.Password(); hasPassword {
			return GitHubURL{}, errors.New("credentials must not be embedded in repository URL")
		}
		return GitHubURL{
			Protocol:   SSH,
			Owner:      parts[0],
			Repository: parts[1],
			CloneURL:   "ssh://git@github.com/" + parts[0] + "/" + parts[1] + ".git",
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
