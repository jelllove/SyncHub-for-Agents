package resource

import (
	"path"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

const DefaultMaxFileSize int64 = 50 << 20

var defaultGeneratedGlobs = []string{
	"**/.git/**",
	"**/.svn/**",
	"**/node_modules/**",
	"**/.venv/**",
	"**/venv/**",
	"**/__pycache__/**",
	"**/.vscode-test/**",
	"**/coverage/**",
	"**/.cache/**",
	"**/*.db",
	"**/*.db-*",
	"**/*.sqlite",
	"**/*.sqlite-*",
	"**/*.log",
	"**/*.lock",
	"**/*.tmp",
}

var defaultExecutableExtensions = []string{
	".exe",
	".dll",
	".so",
	".dylib",
	".node",
}

type FilterPolicy struct {
	MaxFileSize          int64
	GeneratedGlobs       []string
	ExecutableExtensions []string
}

func DefaultFilterPolicy() FilterPolicy {
	return FilterPolicy{
		MaxFileSize:          DefaultMaxFileSize,
		GeneratedGlobs:       append([]string(nil), defaultGeneratedGlobs...),
		ExecutableExtensions: append([]string(nil), defaultExecutableExtensions...),
	}
}

func (p FilterPolicy) Allows(rel string, size int64) bool {
	allowed, _ := p.Check(rel, size, StrategyFileTree)
	return allowed
}

func (p FilterPolicy) Check(rel string, size int64, strategy Strategy) (bool, string) {
	rel = strings.ReplaceAll(rel, `\`, "/")
	if err := validateRepoRelative(rel); err != nil {
		return false, "invalid-path"
	}
	if size < 0 {
		return false, "invalid-size"
	}
	if p.MaxFileSize > 0 && size >= p.MaxFileSize {
		return false, "file-too-large"
	}
	for _, pattern := range p.GeneratedGlobs {
		matched, err := doublestar.Match(strings.ReplaceAll(pattern, `\`, "/"), rel)
		if err != nil {
			return false, "invalid-filter"
		}
		if matched {
			return false, "generated-content"
		}
	}
	if strategy == StrategySourceTree {
		extension := path.Ext(rel)
		for _, blocked := range p.ExecutableExtensions {
			if strings.EqualFold(extension, blocked) {
				return false, "platform-binary"
			}
		}
	}
	return true, ""
}

func (p FilterPolicy) CheckDirectory(rel string) (bool, string) {
	rel = strings.ReplaceAll(rel, `\`, "/")
	if err := validateRepoRelative(rel); err != nil {
		return false, "invalid-path"
	}
	probe := strings.TrimSuffix(rel, "/") + "/.acsync-entry"
	for _, pattern := range p.GeneratedGlobs {
		normalized := strings.ReplaceAll(pattern, `\`, "/")
		matched, err := doublestar.Match(normalized, rel)
		if err != nil {
			return false, "invalid-filter"
		}
		if !matched {
			matched, err = doublestar.Match(normalized, probe)
			if err != nil {
				return false, "invalid-filter"
			}
		}
		if matched {
			return false, "generated-content"
		}
	}
	return true, ""
}
