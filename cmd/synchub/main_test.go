package main

import "testing"

func TestRootCommandUsesSynchub(t *testing.T) {
	root := newRootCommand()
	if root.Use != "synchub" {
		t.Fatalf("Use = %q, want synchub", root.Use)
	}
	if root.Short == "" {
		t.Fatal("Short description must not be empty")
	}
}
