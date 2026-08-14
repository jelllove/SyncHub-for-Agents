package resource

import (
	"fmt"
	"path"
	"strings"
)

const portableProviderName = "_portable"

type RepoRef struct {
	Provider   string
	Category   Category
	ResourceID string
	Relative   string
	Portable   bool
	Common     bool
}

func (s Spec) RepoPath(relative string) (string, error) {
	if err := validateRepoRelative(relative); err != nil {
		return "", err
	}
	if err := ValidateIdentifier("provider name", s.Provider); err != nil {
		return "", err
	}

	switch s.Layout {
	case LayoutLegacy:
		var section string
		switch s.Category {
		case CategoryConfig:
			section = "config"
		case CategorySessions:
			section = "sessions"
		default:
			return "", fmt.Errorf("resource %q: legacy layout does not support category %q", s.Key, s.Category)
		}
		return path.Join("agents", s.Provider, section, relative), nil
	case LayoutPortable:
		if s.SharedAs != "" || s.Provider == "common" {
			resourceID := s.SharedAs
			if resourceID == "" {
				resourceID = s.ID
			}
			if err := ValidateIdentifier("shared_as", resourceID); err != nil {
				return "", err
			}
			return path.Join("agents", portableProviderName, "config", "common", string(s.Category), resourceID, relative), nil
		}
		if err := ValidateIdentifier("resource id", s.ID); err != nil {
			return "", err
		}
		return path.Join("agents", portableProviderName, "config", "providers", s.Provider, string(s.Category), s.ID, relative), nil
	default:
		return "", fmt.Errorf("resource %q: unsupported layout %q", s.Key, s.Layout)
	}
}

func ParseRepoPath(repoPath string) (RepoRef, error) {
	if err := validateRepoRelative(repoPath); err != nil {
		return RepoRef{}, err
	}

	parts := strings.Split(repoPath, "/")
	if len(parts) < 4 || parts[0] != "agents" {
		return RepoRef{}, fmt.Errorf("invalid repo path %q", repoPath)
	}
	if parts[1] != portableProviderName {
		category, err := parseLegacyCategory(parts[2])
		if err != nil {
			return RepoRef{}, err
		}
		return RepoRef{
			Provider:   parts[1],
			Category:   category,
			ResourceID: legacyResourceID(category),
			Relative:   strings.Join(parts[3:], "/"),
		}, nil
	}

	if len(parts) < 7 || parts[2] != "config" {
		return RepoRef{}, fmt.Errorf("invalid portable repo path %q", repoPath)
	}

	switch parts[3] {
	case "common":
		category, err := parseCategory(parts[4])
		if err != nil {
			return RepoRef{}, err
		}
		return RepoRef{
			Provider:   "common",
			Category:   category,
			ResourceID: parts[5],
			Relative:   strings.Join(parts[6:], "/"),
			Portable:   true,
			Common:     true,
		}, nil
	case "providers":
		if len(parts) < 8 {
			return RepoRef{}, fmt.Errorf("invalid portable repo path %q", repoPath)
		}
		category, err := parseCategory(parts[5])
		if err != nil {
			return RepoRef{}, err
		}
		return RepoRef{
			Provider:   parts[4],
			Category:   category,
			ResourceID: parts[6],
			Relative:   strings.Join(parts[7:], "/"),
			Portable:   true,
		}, nil
	default:
		return RepoRef{}, fmt.Errorf("invalid portable repo path %q", repoPath)
	}
}

func legacyResourceID(category Category) string {
	switch category {
	case CategoryConfig:
		return "legacy-config"
	case CategorySessions:
		return "legacy-sessions"
	default:
		return ""
	}
}

func parseLegacyCategory(value string) (Category, error) {
	category, err := parseCategory(value)
	if err != nil {
		return "", err
	}
	if category != CategoryConfig && category != CategorySessions {
		return "", fmt.Errorf("invalid legacy category %q", value)
	}
	return category, nil
}

func parseCategory(value string) (Category, error) {
	category := Category(value)
	switch category {
	case CategorySessions, CategoryConfig, CategoryInstructions, CategorySkills, CategoryPlugins:
		return category, nil
	default:
		return "", fmt.Errorf("invalid category %q", value)
	}
}

func validateRepoRelative(value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("path is empty")
	}
	if strings.Contains(value, `\`) {
		return fmt.Errorf("path %q must use forward slashes", value)
	}
	if strings.HasPrefix(value, "/") || isWindowsAbsolute(value) {
		return fmt.Errorf("path %q must be relative", value)
	}
	if cleaned := path.Clean(value); cleaned != value {
		return fmt.Errorf("path %q is malformed", value)
	}
	for _, part := range strings.Split(value, "/") {
		if err := validateRepoSegment(part); err != nil {
			return fmt.Errorf("path %q is malformed: %w", value, err)
		}
	}
	return nil
}

func validateRepoSegment(value string) error {
	if value == "" || value == "." || value == ".." {
		return fmt.Errorf("invalid path segment %q", value)
	}
	if strings.Contains(value, `\`) {
		return fmt.Errorf("invalid path segment %q", value)
	}
	return nil
}

func isWindowsAbsolute(value string) bool {
	if len(value) < 3 || value[1] != ':' {
		return false
	}
	drive := value[0]
	if (drive < 'A' || drive > 'Z') && (drive < 'a' || drive > 'z') {
		return false
	}
	return value[2] == '/'
}
