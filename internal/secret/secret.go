// Package secret decides which files must be kept out of the sync repo,
// either by path (exclude globs) or by detecting secret-looking JSON keys.
package secret

import (
	"encoding/json"
	"path"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

// Scanner evaluates files against exclude globs and secret key patterns.
type Scanner struct {
	excludeGlobs []string
	keyPatterns  []string
}

// NewScanner builds a Scanner. keyPatterns are matched case-insensitively as
// substrings of JSON object keys.
func NewScanner(excludeGlobs, keyPatterns []string) *Scanner {
	lowered := make([]string, len(keyPatterns))
	for i, p := range keyPatterns {
		lowered[i] = strings.ToLower(p)
	}
	return &Scanner{excludeGlobs: excludeGlobs, keyPatterns: lowered}
}

// IsExcluded reports whether the forward-slash relative path matches any
// exclude glob.
func (s *Scanner) IsExcluded(rel string) bool {
	rel = path.Clean(strings.ReplaceAll(rel, "\\", "/"))
	for _, g := range s.excludeGlobs {
		if ok, _ := doublestar.Match(g, rel); ok {
			return true
		}
	}
	return false
}

// HasSecretContent reports whether data parses as JSON and contains a key
// matching any secret pattern (searched recursively).
func (s *Scanner) HasSecretContent(data []byte) bool {
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		return false
	}
	return s.walk(v)
}

func (s *Scanner) walk(v any) bool {
	switch t := v.(type) {
	case map[string]any:
		for k, child := range t {
			if s.keyMatches(k) {
				return true
			}
			if s.walk(child) {
				return true
			}
		}
	case []any:
		for _, child := range t {
			if s.walk(child) {
				return true
			}
		}
	}
	return false
}

func (s *Scanner) keyMatches(key string) bool {
	lk := strings.ToLower(key)
	for _, p := range s.keyPatterns {
		if strings.Contains(lk, p) {
			return true
		}
	}
	return false
}

// ShouldBlock returns true if the file must not be uploaded.
func (s *Scanner) ShouldBlock(rel string, data []byte) bool {
	if s.IsExcluded(rel) {
		return true
	}
	return s.HasSecretContent(data)
}