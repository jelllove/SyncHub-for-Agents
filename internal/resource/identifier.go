package resource

import (
	"fmt"
	"strings"
)

func ValidateIdentifier(kind, value string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("invalid %s %q", kind, value)
	}
	if value == "." || value == ".." || strings.Contains(value, "/") || strings.Contains(value, `\`) {
		return fmt.Errorf("invalid %s %q", kind, value)
	}
	return nil
}

func ValidateOptionalIdentifier(kind, value string) error {
	if value == "" {
		return nil
	}
	return ValidateIdentifier(kind, value)
}
