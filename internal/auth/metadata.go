package auth

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type Metadata struct {
	Active Account `json:"active"`
}

func SaveMetadata(path string, metadata Metadata) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(metadata, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func LoadMetadata(path string) (Metadata, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Metadata{}, err
	}
	var metadata Metadata
	if err := json.Unmarshal(data, &metadata); err != nil {
		return Metadata{}, err
	}
	if metadata.Active.ID == 0 {
		return Metadata{}, ErrNotFound
	}
	return metadata, nil
}
