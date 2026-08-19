package portableconfig

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/qinqingxu/synchub-for-agents/internal/pathresolver"
)

type Policy struct {
	Portable     []string
	Sensitive    []string
	MachineLocal []string
	PathFields   []string
}

type field struct {
	segments []string
	path     string
	value    any
}

func projectDocument(policy Policy, document map[string]any, goos, home string) map[string]any {
	projected := map[string]any{}
	for _, item := range flatten(document) {
		if !matches(policy.Portable, item.path) ||
			matches(policy.Sensitive, item.path) ||
			matches(policy.MachineLocal, item.path) {
			continue
		}
		value := cloneValue(item.value)
		if matches(policy.PathFields, item.path) {
			var ok bool
			value, ok = tokenizePathValue(value, goos, home)
			if !ok {
				continue
			}
		}
		setAt(projected, item.segments, value)
	}
	return projected
}

func restoreDocument(
	policy Policy,
	local map[string]any,
	base map[string]any,
	remote map[string]any,
	goos string,
	home string,
) (map[string]any, error) {
	restored := cloneMap(local)
	remoteFields := indexFields(flatten(remote))
	for key, baseField := range indexFields(flatten(base)) {
		if _, exists := remoteFields[key]; exists {
			continue
		}
		if isPortable(policy, baseField.path) {
			deleteAt(restored, baseField.segments)
		}
	}
	for _, item := range flatten(remote) {
		if !isPortable(policy, item.path) {
			continue
		}
		if object, ok := item.value.(map[string]any); ok && len(object) == 0 {
			continue
		}
		value := cloneValue(item.value)
		if matches(policy.PathFields, item.path) {
			var ok bool
			var err error
			value, ok, err = expandPathValue(value, goos, home)
			if err != nil {
				return nil, fmt.Errorf("restore path field %s: %w", item.path, err)
			}
			if !ok {
				continue
			}
		}
		setAt(restored, item.segments, value)
	}
	return restored, nil
}

func isPortable(policy Policy, path string) bool {
	return matches(policy.Portable, path) &&
		!matches(policy.Sensitive, path) &&
		!matches(policy.MachineLocal, path)
}

func matches(patterns []string, keyPath string) bool {
	lowerPath := strings.ReplaceAll(strings.ToLower(keyPath), ".", "/")
	for _, pattern := range patterns {
		normalizedPattern := strings.ReplaceAll(strings.ToLower(pattern), ".", "/")
		matched, err := doublestar.Match(normalizedPattern, lowerPath)
		if err == nil && matched {
			return true
		}
	}
	return false
}

func flatten(document map[string]any) []field {
	var out []field
	for key, value := range document {
		out = append(out, flattenValue([]string{key}, value)...)
	}
	return out
}

func flattenValue(segments []string, value any) []field {
	object, ok := value.(map[string]any)
	if !ok || len(object) == 0 {
		return []field{{
			segments: append([]string(nil), segments...),
			path:     strings.Join(segments, "."),
			value:    value,
		}}
	}
	var out []field
	for key, child := range object {
		next := append(append([]string(nil), segments...), key)
		out = append(out, flattenValue(next, child)...)
	}
	return out
}

func indexFields(fields []field) map[string]field {
	out := make(map[string]field, len(fields))
	for _, item := range fields {
		encoded, _ := json.Marshal(item.segments)
		out[string(encoded)] = item
	}
	return out
}

func setAt(document map[string]any, segments []string, value any) {
	current := document
	for _, segment := range segments[:len(segments)-1] {
		next, ok := current[segment].(map[string]any)
		if !ok {
			next = map[string]any{}
			current[segment] = next
		}
		current = next
	}
	current[segments[len(segments)-1]] = cloneValue(value)
}

func deleteAt(document map[string]any, segments []string) bool {
	if len(segments) == 1 {
		delete(document, segments[0])
		return len(document) == 0
	}
	child, ok := document[segments[0]].(map[string]any)
	if !ok {
		return false
	}
	if deleteAt(child, segments[1:]) {
		delete(document, segments[0])
	}
	return len(document) == 0
}

func cloneMap(value map[string]any) map[string]any {
	return cloneValue(value).(map[string]any)
}

func cloneValue(value any) any {
	switch item := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(item))
		for key, child := range item {
			out[key] = cloneValue(child)
		}
		return out
	case []any:
		out := make([]any, len(item))
		for index, child := range item {
			out[index] = cloneValue(child)
		}
		return out
	default:
		return value
	}
}

func tokenizePathValue(value any, goos, home string) (any, bool) {
	switch item := value.(type) {
	case string:
		if tokenized, ok := pathresolver.TokenizeHome(item, goos, home); ok {
			return tokenized, true
		}
		if looksAbsolute(item, goos) {
			return nil, false
		}
		return item, true
	case []any:
		out := make([]any, len(item))
		for index, child := range item {
			transformed, ok := tokenizePathValue(child, goos, home)
			if !ok {
				return nil, false
			}
			out[index] = transformed
		}
		return out, true
	default:
		return value, true
	}
}

func expandPathValue(value any, goos, home string) (any, bool, error) {
	switch item := value.(type) {
	case string:
		expanded, err := pathresolver.ExpandHomeToken(item, goos, home)
		if err != nil {
			return nil, false, err
		}
		if expanded == item && looksAbsolute(item, goos) {
			return nil, false, nil
		}
		return expanded, true, nil
	case []any:
		out := make([]any, len(item))
		for index, child := range item {
			transformed, ok, err := expandPathValue(child, goos, home)
			if err != nil || !ok {
				return nil, ok, err
			}
			out[index] = transformed
		}
		return out, true, nil
	default:
		return value, true, nil
	}
}

func looksAbsolute(value, goos string) bool {
	normalized := strings.ReplaceAll(value, `\`, "/")
	if goos == "windows" {
		return strings.HasPrefix(normalized, "//") ||
			(len(normalized) >= 3 && normalized[1] == ':' && normalized[2] == '/')
	}
	return strings.HasPrefix(normalized, "/")
}
