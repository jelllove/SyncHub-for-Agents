package portableconfig

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/qinqingxu/acsync/internal/secret"
)

var genericSecretPatterns = []string{
	"apiKey",
	"token",
	"secret",
	"password",
	"oauth",
	"refresh_token",
}

type Registry struct {
	policies map[string]Policy
}

func NewRegistry() *Registry {
	return &Registry{policies: map[string]Policy{}}
}

func (r *Registry) Register(transformer string, policy Policy) {
	r.policies[transformer] = Policy{
		Portable:     append([]string(nil), policy.Portable...),
		Sensitive:    append([]string(nil), policy.Sensitive...),
		MachineLocal: append([]string(nil), policy.MachineLocal...),
		PathFields:   append([]string(nil), policy.PathFields...),
	}
}

func (r *Registry) Project(transformer, rel, goos, home string, data []byte) ([]byte, error) {
	if transformer == "" {
		return bytes.Clone(data), nil
	}
	format, document, err := Parse(rel, data)
	if err != nil {
		return nil, err
	}
	if transformer == "generic-safe" {
		if err := scanDocument(document); err != nil {
			return nil, err
		}
		return Marshal(format, document)
	}
	policy, ok := r.policies[transformer]
	if !ok {
		return nil, fmt.Errorf("portable config: unknown transformer %q", transformer)
	}
	projected := projectDocument(policy, document, goos, home)
	if err := scanDocument(projected); err != nil {
		return nil, err
	}
	return Marshal(format, projected)
}

func (r *Registry) Restore(
	transformer, rel, goos, home string,
	local, base, remote []byte,
) ([]byte, error) {
	if transformer == "" {
		return bytes.Clone(remote), nil
	}
	format, remoteDocument, err := Parse(rel, remote)
	if err != nil {
		return nil, err
	}
	if err := scanDocument(remoteDocument); err != nil {
		return nil, err
	}
	if transformer == "generic-safe" {
		if len(bytes.TrimSpace(local)) > 0 {
			_, localDocument, err := Parse(rel, local)
			if err != nil {
				return nil, err
			}
			if err := scanDocument(localDocument); err != nil {
				return nil, err
			}
		}
		return Marshal(format, remoteDocument)
	}
	policy, ok := r.policies[transformer]
	if !ok {
		return nil, fmt.Errorf("portable config: unknown transformer %q", transformer)
	}
	localDocument := map[string]any{}
	if len(bytes.TrimSpace(local)) > 0 {
		_, localDocument, err = Parse(rel, local)
		if err != nil {
			return nil, err
		}
	}
	baseDocument := map[string]any{}
	if len(bytes.TrimSpace(base)) > 0 {
		_, baseDocument, err = Parse(rel, base)
		if err != nil {
			return nil, err
		}
	}
	restored, err := restoreDocument(policy, localDocument, baseDocument, remoteDocument, goos, home)
	if err != nil {
		return nil, err
	}
	return Marshal(format, restored)
}

func scanDocument(document map[string]any) error {
	data, err := json.Marshal(document)
	if err != nil {
		return fmt.Errorf("scan portable config: %w", err)
	}
	scanner := secret.NewScanner(nil, genericSecretPatterns)
	blocked, err := scanner.Scan("projection.json", data)
	if err != nil {
		return fmt.Errorf("scan portable config: %w", err)
	}
	if blocked {
		return fmt.Errorf("scan portable config: secret-looking field detected")
	}
	return nil
}
