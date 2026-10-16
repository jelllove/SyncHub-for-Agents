package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestLibraryCommandPreviewAndApproval(t *testing.T) {
	user := t.TempDir()
	t.Setenv("HOME", user)
	t.Setenv("USERPROFILE", user)
	source := filepath.Join(user, ".agents", "library", "skills", "review")
	if err := os.MkdirAll(source, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "SKILL.md"), []byte("Review tests."), 0o600); err != nil {
		t.Fatal(err)
	}
	args := []string{"--home", filepath.Join(user, "profile"), "--json", "library", "apply", "--kind", "skills", "--name", "review", "--harness", "claude"}
	run := func(args []string) ([]byte, error) {
		root := newRootCommand()
		var out bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&out)
		root.SetArgs(args)
		err := root.Execute()
		return out.Bytes(), err
	}
	output, err := run(args)
	if err != nil {
		t.Fatalf("%s %v", output, err)
	}
	target := filepath.Join(user, ".claude", "skills", "review", "SKILL.md")
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatal("preview wrote files")
	}
	var response struct {
		Data struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(output, &response); err != nil {
		t.Fatal(err)
	}
	if response.Data.ID == "" {
		t.Fatal("missing approval ID")
	}
	if out, err := run(append(args, "--approve", response.Data.ID)); err != nil {
		t.Fatalf("%s %v", out, err)
	}
	if _, err := os.Stat(target); err != nil {
		t.Fatal(err)
	}
}
