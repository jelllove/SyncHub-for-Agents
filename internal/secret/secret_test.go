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
		"projects/a/.credentials.json": true,
		"config/access_token.txt":      true,
		"shell-snapshots/snap-1.sh":    true,
		"config/settings.json":         false,
		"sessions/abc/uuid.jsonl":      false,
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

func TestShouldBlockSecretInJSONLRecord(t *testing.T) {
	s := newTestScanner()
	data := []byte("{\"message\":\"safe\"}\n{\"oauthToken\":\"secret-value\"}\n")
	if !s.ShouldBlock("sessions/chat.jsonl", data) {
		t.Fatal("JSONL record containing a secret key must be blocked")
	}
	clean := []byte("{\"message\":\"safe\"}\n{\"theme\":\"dark\"}\n")
	if s.ShouldBlock("sessions/chat.jsonl", clean) {
		t.Fatal("clean JSONL records must not be blocked")
	}
}

func TestShouldBlockDoesNotParseJSONLinesInMarkdown(t *testing.T) {
	s := newTestScanner()
	data := []byte("Example configuration:\n{\"token\":\"placeholder\"}\n")
	if s.ShouldBlock("CLAUDE.md", data) {
		t.Fatal("JSON examples in non-JSONL text must not be blocked")
	}
}

func TestShouldBlockAllowsTokenUsageMetrics(t *testing.T) {
	s := newTestScanner()
	data := []byte(`{"inputTokens":10,"tokenDetails":{"tokenType":"input","tokenCount":10}}`)
	if s.ShouldBlock("sessions/events.jsonl", data) {
		t.Fatal("token usage metrics are not credentials")
	}
}

func TestShouldBlockStillDetectsCredentialTokens(t *testing.T) {
	s := newTestScanner()
	for _, key := range []string{
		"token", "accessToken", "oauthToken", "refresh_token", "tokenValue",
		"token_data", "currentAccessTokens", "cachedRefreshTokens",
	} {
		data := []byte(`{"` + key + `":"credential-value"}`)
		if !s.ShouldBlock("sessions/events.jsonl", data) {
			t.Errorf("credential key %q must be blocked", key)
		}
	}
}

func TestShouldBlockIgnoresPathKeysContainingToken(t *testing.T) {
	s := newTestScanner()
	data := []byte(`{"file:///c:/src/AccessToken.cs":{"line":12}}`)
	if s.ShouldBlock("sessions/chat.jsonl", data) {
		t.Fatal("a file URI containing token is not a credential key")
	}
}

func TestShouldBlockMalformedJSONL(t *testing.T) {
	s := newTestScanner()
	data := []byte("{\"message\":\"safe\"}\n{\"accessToken\":\"truncated")
	if !s.ShouldBlock("sessions/events.jsonl", data) {
		t.Fatal("a malformed nonblank JSONL record must fail closed")
	}
	if _, err := s.Scan("sessions/events.jsonl", data); err == nil {
		t.Fatal("malformed JSONL must request a retry instead of being treated as a secret")
	}
}
