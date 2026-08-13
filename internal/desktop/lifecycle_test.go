package desktop

import (
	"context"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func TestRunPublishesSnapshotsAndStops(t *testing.T) {
	service, err := New(configuredHome(t), runtime.GOOS)
	if err != nil {
		t.Fatal(err)
	}
	updates := make(chan Snapshot, 8)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- service.Run(ctx, func(snapshot Snapshot) {
			updates <- snapshot
		})
	}()

	select {
	case snapshot := <-updates:
		if !snapshot.Configured {
			t.Fatalf("published snapshot = %#v", snapshot)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("no snapshot published")
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("service did not stop")
	}
}

func TestRunWaitsForConfigurationThenStarts(t *testing.T) {
	service, err := New(filepath.Join(t.TempDir(), ".acsync"), runtime.GOOS)
	if err != nil {
		t.Fatal(err)
	}
	updates := make(chan Snapshot, 8)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- service.Run(ctx, func(snapshot Snapshot) {
			updates <- snapshot
		})
	}()

	select {
	case snapshot := <-updates:
		t.Fatalf("unconfigured service published before setup: %#v", snapshot)
	case <-time.After(20 * time.Millisecond):
	}

	if err := service.SaveSettings(SettingsInput{
		RepositoryURL:   "git@github.com:owner/repo.git",
		IntervalMinutes: 10,
		TrashGraceDays:  30,
		Agents:          map[string]bool{},
	}); err != nil {
		t.Fatal(err)
	}
	select {
	case snapshot := <-updates:
		if !snapshot.Configured {
			t.Fatalf("published snapshot = %#v", snapshot)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("service did not start after configuration")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("service did not stop")
	}
}
