package main

import (
	"errors"
	"reflect"
	"testing"

	"github.com/qinqingxu/synchub-for-agents/internal/cli"
	"github.com/qinqingxu/synchub-for-agents/internal/desktop"
	"github.com/qinqingxu/synchub-for-agents/internal/instance"
)

func TestDefaultDesktopHomeUsesSharedCLIHome(t *testing.T) {
	userHome := t.TempDir()
	t.Setenv("HOME", userHome)
	t.Setenv("USERPROFILE", userHome)
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
	home := t.TempDir()

	err := runDesktop(false, desktopBootstrap{
		newGUI: func() *guiApplication {
			calls = append(calls, "single-instance")
			return &guiApplication{}
		},
		home: func() (string, error) {
			calls = append(calls, "home")
			return home, nil
		},
		newService: func(string, string) (*desktop.Service, error) {
			other, err := instance.Acquire(home)
			if !errors.Is(err, instance.ErrBusy) {
				if other != nil {
					other.Close()
				}
				t.Fatalf("Desktop initialized backend without profile ownership: %v", err)
			}
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
	lock, err := instance.Acquire(home)
	if err != nil {
		t.Fatalf("startup failure leaked ownership: %v", err)
	}
	defer lock.Close()
}

func TestDesktopDoesNotInitializeServiceWhenProfileIsOwned(t *testing.T) {
	home := t.TempDir()
	lock, err := instance.Acquire(home)
	if err != nil {
		t.Fatal(err)
	}
	defer lock.Close()
	err = runDesktop(false, desktopBootstrap{
		newGUI: func() *guiApplication { return &guiApplication{} },
		home:   func() (string, error) { return home, nil },
		newService: func(string, string) (*desktop.Service, error) {
			t.Fatal("backend initialized while CLI owns profile")
			return nil, nil
		},
	})
	if !errors.Is(err, instance.ErrBusy) {
		t.Fatalf("error = %v, want busy", err)
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
