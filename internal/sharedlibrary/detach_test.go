package sharedlibrary

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetachPreservesSensitivePrivateEdits(t *testing.T) {
	m := fixture(t)
	put(t, filepath.Join(Root(m.UserHome), "skills", "review", "SKILL.md"), "Review\n")
	r := Request{Action: "apply", Kind: "skills", Name: "review", Harness: "claude"}
	execute(t, m, r)
	credential := filepath.Join(m.UserHome, ".claude", "skills", "review", ".env")
	put(t, credential, "PRIVATE_VALUE=local-only\n")
	r.Action = "detach"
	execute(t, m, r)
	data, err := os.ReadFile(credential)
	if err != nil || string(data) != "PRIVATE_VALUE=local-only\n" {
		t.Fatal("detach changed private files")
	}
}
