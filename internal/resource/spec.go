package resource

import "strings"

type Spec struct {
	Key             string
	Provider        string
	ID              string
	Category        Category
	Strategy        Strategy
	Layout          Layout
	Root            string
	Targets         []string
	Include         []string
	Exclude         []string
	Transformer     string
	Installer       string
	SharedAs        string
	KeyPatterns     []string
	Reserved        []string
	CopyOnly        bool
	OptionalRoot    bool
	ValidateRoot    func(string) error         `json:"-"`
	ValidateContent func(string, []byte) error `json:"-"`
}

// Owns excludes files whose lifecycle belongs to the shared library.
func (s Spec) Owns(relative string) bool {
	relative = strings.ToLower(strings.ReplaceAll(relative, `\`, "/"))
	for _, prefix := range s.Reserved {
		prefix = strings.ToLower(strings.ReplaceAll(prefix, `\`, "/"))
		if prefix == "" || relative == prefix || strings.HasPrefix(relative, prefix+"/") {
			return false
		}
	}
	return true
}
