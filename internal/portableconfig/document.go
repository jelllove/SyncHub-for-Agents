package portableconfig

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	toml "github.com/pelletier/go-toml/v2"
	"gopkg.in/yaml.v3"
)

type Format string

const (
	JSON Format = "json"
	YAML Format = "yaml"
	TOML Format = "toml"
)

func Parse(rel string, data []byte) (Format, map[string]any, error) {
	format, err := formatForPath(rel)
	if err != nil {
		return "", nil, err
	}
	var raw any
	switch format {
	case JSON:
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.UseNumber()
		if err := decoder.Decode(&raw); err != nil {
			return "", nil, fmt.Errorf("parse %s as JSON: %w", rel, err)
		}
		if err := requireEnd(decoder.Decode); err != nil {
			return "", nil, fmt.Errorf("parse %s as JSON: %w", rel, err)
		}
	case YAML:
		decoder := yaml.NewDecoder(bytes.NewReader(data))
		if err := decoder.Decode(&raw); err != nil {
			return "", nil, fmt.Errorf("parse %s as YAML: %w", rel, err)
		}
		if err := requireEnd(decoder.Decode); err != nil {
			return "", nil, fmt.Errorf("parse %s as YAML: %w", rel, err)
		}
	case TOML:
		value := map[string]any{}
		if err := toml.Unmarshal(data, &value); err != nil {
			return "", nil, fmt.Errorf("parse %s as TOML: %w", rel, err)
		}
		raw = value
	}
	normalized, err := normalizeValue(raw)
	if err != nil {
		return "", nil, fmt.Errorf("parse %s: %w", rel, err)
	}
	document, ok := normalized.(map[string]any)
	if !ok {
		return "", nil, fmt.Errorf("parse %s: document root must be a mapping", rel)
	}
	return format, document, nil
}

func Marshal(format Format, value map[string]any) ([]byte, error) {
	var buffer bytes.Buffer
	switch format {
	case JSON:
		encoder := json.NewEncoder(&buffer)
		encoder.SetIndent("", "  ")
		if err := encoder.Encode(value); err != nil {
			return nil, fmt.Errorf("marshal JSON: %w", err)
		}
	case YAML:
		encoder := yaml.NewEncoder(&buffer)
		encoder.SetIndent(2)
		if err := encoder.Encode(value); err != nil {
			return nil, fmt.Errorf("marshal YAML: %w", err)
		}
		if err := encoder.Close(); err != nil {
			return nil, fmt.Errorf("close YAML encoder: %w", err)
		}
	case TOML:
		encoder := toml.NewEncoder(&buffer)
		if err := encoder.Encode(value); err != nil {
			return nil, fmt.Errorf("marshal TOML: %w", err)
		}
	default:
		return nil, fmt.Errorf("marshal: unsupported format %q", format)
	}
	return buffer.Bytes(), nil
}

func formatForPath(rel string) (Format, error) {
	switch strings.ToLower(filepath.Ext(rel)) {
	case ".json":
		return JSON, nil
	case ".yaml", ".yml":
		return YAML, nil
	case ".toml":
		return TOML, nil
	default:
		return "", fmt.Errorf("unsupported structured document %q", rel)
	}
}

func requireEnd(decode func(any) error) error {
	var extra any
	err := decode(&extra)
	if err == io.EOF {
		return nil
	}
	if err == nil {
		return fmt.Errorf("multiple documents are not supported")
	}
	return err
}

func normalizeValue(value any) (any, error) {
	switch item := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(item))
		for key, child := range item {
			normalized, err := normalizeValue(child)
			if err != nil {
				return nil, err
			}
			out[key] = normalized
		}
		return out, nil
	case map[any]any:
		out := make(map[string]any, len(item))
		for key, child := range item {
			text, ok := key.(string)
			if !ok {
				return nil, fmt.Errorf("mapping key %v is not a string", key)
			}
			normalized, err := normalizeValue(child)
			if err != nil {
				return nil, err
			}
			out[text] = normalized
		}
		return out, nil
	case []any:
		out := make([]any, len(item))
		for index, child := range item {
			normalized, err := normalizeValue(child)
			if err != nil {
				return nil, err
			}
			out[index] = normalized
		}
		return out, nil
	default:
		return value, nil
	}
}
