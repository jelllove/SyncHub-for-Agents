//go:build legacytray

package main

import (
	"bytes"
	"errors"
	"path/filepath"
	"testing"

	"github.com/qinqingxu/synchub-for-agents/internal/instance"
	"github.com/spf13/cobra"
)

func TestLegacyTrayUsesSelectedProfileWithoutLaunchingGUI(t *testing.T) {
	userHome := t.TempDir()
	t.Setenv("HOME", userHome)
	t.Setenv("USERPROFILE", userHome)
	selected := filepath.Join(userHome, "isolated-profile")
	options := &commandOptions{}
	root := &cobra.Command{Use: "synchub"}
	root.PersistentFlags().StringVar(&options.home, "home", "", "profile")
	root.PersistentFlags().BoolVar(&options.json, "json", false, "JSON")
	called := false
	root.AddCommand(legacyTrayCommand(options, func(home, _ string) error {
		called = true
		if home != selected {
			t.Fatalf("tray used %s instead of selected profile", home)
		}
		lock, err := instance.Acquire(home)
		if !errors.Is(err, instance.ErrBusy) {
			if lock != nil {
				lock.Close()
			}
			t.Fatalf("legacy tray does not own selected profile: %v", err)
		}
		return nil
	}))
	root.SetOut(&bytes.Buffer{})
	root.SetArgs([]string{"--home", selected, "tray"})
	if err := root.Execute(); err != nil {
		t.Fatal(err)
	}
	if !called {
		t.Fatal("tray runner not called")
	}
}
