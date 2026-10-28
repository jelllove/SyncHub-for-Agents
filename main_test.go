package main

import (
	"errors"
	"reflect"
	"testing"

	"github.com/qinqingxu/synchub-for-agents/internal/cli"
	"github.com/qinqingxu/synchub-for-agents/internal/desktop"
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

func TestDesktopBootstrapCreatesSingleInstanceBeforeService(t *testing.T) {
	var calls []string
	serviceErr := errors.New("stop after service factory")

	err := runDesktop(false, desktopBootstrap{
		newGUI: func() *guiApplication {
			calls = append(calls, "single-instance")
			return &guiApplication{}
		},
		home: func() (string, error) {
			calls = append(calls, "home")
			return "test-home", nil
		},
		newService: func(string, string) (*desktop.Service, error) {
			calls = append(calls, "service")
			return nil, serviceErr
		},
	})
	if !errors.Is(err, serviceErr) {
		t.Fatalf("runDesktop() error = %v, want %v", err, serviceErr)
	}
	if want := []string{"single-instance", "home", "service"}; !reflect.DeepEqual(calls, want) {
		t.Fatalf("startup order = %v, want %v", calls, want)
	}
}

func TestActivationQueuePreservesRequestUntilWindowIsReady(t *testing.T) {
	var queue activationQueue
	shows := 0

	queue.Request()
	queue.Ready(func() {
		shows++
	})
	if shows != 1 {
		t.Fatalf("shows after Ready() = %d, want 1", shows)
	}

	queue.Request()
	if shows != 2 {
		t.Fatalf("shows after second Request() = %d, want 2", shows)
	}
}
