// Package secret decides which files must be kept out of the sync repo,
// either by path (exclude globs) or by detecting secret-looking JSON keys.
package secret

import (
	"bytes"
	"encoding/json"
	"fmt"
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

func (s *Scanner) hasSecretJSONL(data []byte) (bool, error) {
	for _, line := range bytes.Split(data, []byte{'\n'}) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		var v any
		if err := json.Unmarshal(line, &v); err != nil {
			return false, fmt.Errorf("invalid JSONL record: %w", err)
		}
		if s.walk(v) {
			return true, nil
		}
	}
	return false, nil
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
	if strings.ContainsAny(key, `/\`) {
		return false
	}
	lk := strings.ToLower(key)
	for _, p := range s.keyPatterns {
		if p == "token" {
			normalized := strings.NewReplacer("_", "", "-", "", " ", "").Replace(lk)
			if strings.Contains(normalized, "token") && !isTokenMetric(normalized) {
				return true
			}
			continue
		}
		if strings.Contains(lk, p) {
			return true
		}
	}
	return false
}

func isTokenMetric(key string) bool {
	switch key {
	case "cachedtokens",
		"cachereadtokens",
		"cachewritetokens",
		"compactiontokensused",
		"completiontokens",
		"conversationtokens",
		"currenttokens",
		"inputtokens",
		"maxinputtokens",
		"maxoutputtokens",
		"outputtokens",
		"precompactiontokens",
		"prompttokens",
		"prompttokensdetails",
		"reasoningtokens",
		"responsetokenlimit",
		"systemtokens",
		"tokencount",
		"tokendetails",
		"tokentype",
		"tokenusage",
		"tooldefinitionstokens",
		"totaltokens":
		return true
	}
	return false
}

// Scan reports whether a file is blocked. Malformed JSONL returns an error so
// callers retry later rather than treating an active partial record as a secret.
func (s *Scanner) Scan(rel string, data []byte) (bool, error) {
	if s.IsExcluded(rel) {
		return true, nil
	}
	if s.HasSecretContent(data) {
		return true, nil
	}
	if strings.EqualFold(path.Ext(rel), ".jsonl") {
		return s.hasSecretJSONL(data)
	}
	return false, nil
}

// ShouldBlock returns true if the file must not be uploaded. Scanner errors fail closed.
func (s *Scanner) ShouldBlock(rel string, data []byte) bool {
	blocked, err := s.Scan(rel, data)
	return blocked || err != nil
}
