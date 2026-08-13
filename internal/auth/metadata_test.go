package auth

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMetadataRoundTripContainsNoToken(t *testing.T) {
	path := filepath.Join(t.TempDir(), "auth.json")
	want := Metadata{Active: Account{ID: 42, Login: "alice", Scopes: []string{"repo"}}}
	if err := SaveMetadata(path, want); err != nil {
		t.Fatal(err)
	}
	got, err := LoadMetadata(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Active.ID != want.Active.ID || got.Active.Login != want.Active.Login {
		t.Fatalf("LoadMetadata() = %#v", got)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	serialized := strings.ToLower(string(data))
	for _, forbidden := range []string{"gho_secret", `"token"`} {
		if strings.Contains(serialized, forbidden) {
			t.Fatalf("metadata contains secret marker %q: %s", forbidden, data)
		}
	}
}
