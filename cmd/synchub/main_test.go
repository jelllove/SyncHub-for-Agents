package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/qinqingxu/synchub-for-agents/internal/instance"
)

func TestRootCommandUsesSynchub(t *testing.T) {
	root := newRootCommand()
	if root.Use != "synchub" {
		t.Fatalf("Use = %q, want synchub", root.Use)
	}
	if root.Short == "" {
		t.Fatal("Short description must not be empty")
	}
}

func TestHeadlessInspectionCommandsHaveJSONFlag(t *testing.T) {
	for _, name := range []string{"status", "plan", "doctor", "sync"} {
		root := newRootCommand()
		cmd, _, err := root.Find([]string{name})
		if err != nil || cmd == root || cmd.Flags().Lookup("json") == nil && cmd.InheritedFlags().Lookup("json") == nil {
			t.Errorf("%s missing command or --json", name)
		}
	}
}

func TestCommandsRejectUnexpectedArguments(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	root := newRootCommand()
	root.SetArgs([]string{"status", "unexpected"})
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	if err := root.Execute(); err == nil {
		t.Fatal("unexpected argument accepted")
	}
}

func TestVersionDoesNotTouchProfile(t *testing.T) {
	home := filepath.Join(t.TempDir(), "must-not-exist")
	root := newRootCommand()
	root.SetArgs([]string{"--home", home, "--version"})
	var out bytes.Buffer
	root.SetOut(&out)
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if out.Len() == 0 {
		t.Fatal("empty version")
	}
	if _, err := os.Stat(home); !os.IsNotExist(err) {
		t.Fatalf("version touched profile: %v", err)
	}
}

func TestBusyCLIProducesJSONAndDoesNotInitialize(t *testing.T) {
	home := t.TempDir()
	lock, err := instance.Acquire(home)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	root := newRootCommand()
	root.SetArgs([]string{"--home", home, "--json", "init", "--repo", "git@github.com:example/synthetic.git", "--first-sync", "merge-cloud-local"})
	var out, stderr bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&stderr)
	if err := root.Execute(); err == nil {
		t.Fatal("initialization bypassed profile lock")
	}
	var payload struct {
		SchemaVersion int  `json:"schemaVersion"`
		OK            bool `json:"ok"`
		Error         struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
		t.Fatalf("invalid JSON %q: %v", out.String(), err)
	}
	if payload.SchemaVersion != 1 || payload.OK || payload.Error.Code != "busy" {
		t.Fatalf("busy payload = %+v", payload)
	}
	if _, err := os.Stat(filepath.Join(home, "config.yaml")); !os.IsNotExist(err) {
		t.Fatalf("busy command changed config: %v", err)
	}
}
