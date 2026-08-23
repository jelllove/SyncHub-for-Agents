package main

import "testing"

func TestWindowClosingDoesNotCancelWhenAppIsQuitting(t *testing.T) {
	if !shouldCancelWindowClose(false) {
		t.Fatal("expected regular close to be canceled (hide to tray)")
	}
	if shouldCancelWindowClose(true) {
		t.Fatal("expected close to proceed while quitting")
	}
}
