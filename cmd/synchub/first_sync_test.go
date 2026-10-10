package main

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"testing"

	"github.com/qinqingxu/synchub-for-agents/internal/cli"
	"github.com/qinqingxu/synchub-for-agents/internal/config"
)

func TestPendingFirstSyncChoiceIsAttentionAcrossCommands(t *testing.T) {
	userHome := t.TempDir()
	t.Setenv("HOME", userHome)
	t.Setenv("USERPROFILE", userHome)
	home := filepath.Join(userHome, ".synchub")
	cfg := config.Default(nil)
	cfg.RepoURL = "git@github.com:example/synthetic.git"
	cfg.FirstSync = config.FirstSyncPolicy{Strategy: config.FirstSyncStrategyChoose}
	if err := config.Save(cli.ConfigPath(home), cfg); err != nil {
		t.Fatal(err)
	}
	for _, command := range []string{"plan", "sync", "daemon"} {
		root := newRootCommand()
		args := []string{"--home", home, command}
		if command != "daemon" {
			args = append(args, "--json")
		}
		root.SetArgs(args)
		var out, stderr bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&stderr)
		err := root.Execute()
		if err == nil || asCommandError(err).Code != 4 {
			t.Fatalf("%s error = %v, want attention exit 4", command, err)
		}
		if command != "daemon" {
			var payload envelope
			if err := json.Unmarshal(out.Bytes(), &payload); err != nil {
				t.Fatal(err)
			}
			if payload.OK || payload.Error.Code != "needs_attention" {
				t.Fatalf("%s payload = %s", command, out.String())
			}
		}
	}
}
