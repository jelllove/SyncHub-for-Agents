package main

import (
	"testing"

	"github.com/qinqingxu/acsync/internal/cli"
)

func TestDefaultDesktopHomeUsesSharedCLIHome(t *testing.T) {
	want, err := cli.Home()
	if err != nil {
		t.Fatal(err)
	}
	got, err := defaultDesktopHome()
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("defaultDesktopHome() = %q, want %q", got, want)
	}
}
