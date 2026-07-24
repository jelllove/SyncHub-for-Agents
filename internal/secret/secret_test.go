package secret

import "testing"

func newTestScanner() *Scanner {
	return NewScanner(
		[]string{"**/.credentials.json", "**/*token*", "**/shell-snapshots/**"},
		[]string{"apiKey", "token", "secret", "password", "oauth", "refresh_token"},
	)
}

func TestIsExcludedByGlob(t *testing.T) {
	s := newTestScanner()
	cases := map[string]bool{
		"projects/a/.credentials.json":     true,
		"config/access_token.txt":          true,
		"shell-snapshots/snap-1.sh":        true,
		"config/settings.json":             false,
		"sessions/abc/uuid.jsonl":          false,
	}
	for rel, want := range cases {
		if got := s.IsExcluded(rel); got != want {
			t.Errorf("IsExcluded(%q) = %v, want %v", rel, got, want)
		}
	}
}

func TestHasSecretContent(t *testing.T) {
	s := newTestScanner()
	secretJSON := []byte(`{"model":"x","apiKey":"sk-123"}`)
	if !s.HasSecretContent(secretJSON) {
		t.Error("expected secret content to be detected")
	}
	cleanJSON := []byte(`{"model":"x","theme":"dark"}`)
	if s.HasSecretContent(cleanJSON) {
		t.Error("clean JSON should not be flagged")
	}
	notJSON := []byte("apiKey is a word in prose")
	if s.HasSecretContent(notJSON) {
		t.Error("non-JSON content must not be treated as secret JSON")
	}
}

func TestShouldBlock(t *testing.T) {
	s := newTestScanner()
	if !s.ShouldBlock("projects/.credentials.json", []byte("{}")) {
		t.Error("excluded path must be blocked regardless of content")
	}
	if !s.ShouldBlock("config/settings.json", []byte(`{"token":"abc"}`)) {
		t.Error("secret content must be blocked")
	}
	if s.ShouldBlock("config/settings.json", []byte(`{"theme":"dark"}`)) {
		t.Error("clean file must not be blocked")
	}
}