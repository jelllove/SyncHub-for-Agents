package desktop

import (
	"context"
	"path/filepath"
	"runtime"
	"sync"
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

func TestRunPublishesStateTransitionsFromLightweightSnapshots(t *testing.T) {
	home := configuredHome(t)
	writeDesktopPreview(t, home, ResourcePreview{Files: 12})
	service, err := New(home, runtime.GOOS)
	if err != nil {
		t.Fatal(err)
	}
	release := make(chan struct{})
	var releaseOnce sync.Once
	releaseJob := func() {
		releaseOnce.Do(func() { close(release) })
	}
	service.Daemon().Scheduler.Job = func() error {
		<-release
		return nil
	}

	updates := make(chan Snapshot, 4)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- service.Run(ctx, func(snapshot Snapshot) {
			updates <- snapshot
		})
	}()
	t.Cleanup(func() {
		cancel()
		select {
		case err := <-done:
			if err != nil {
				t.Errorf("Run cleanup error: %v", err)
			}
		case <-time.After(time.Second):
			t.Error("service did not stop during cleanup")
		}
	})
	t.Cleanup(releaseJob)

	var updating Snapshot
	select {
	case updating = <-updates:
		if updating.State != "updating" {
			t.Fatalf("published state = %q, want updating", updating.State)
		}
		if updating.Preview.Files != 12 {
			t.Fatalf("published preview files = %d, want 12", updating.Preview.Files)
		}
	case <-time.After(time.Second):
		t.Fatal("no updating snapshot published")
	}
	releaseJob()

	select {
	case doneSnapshot := <-updates:
		if doneSnapshot.State != "done" {
			t.Fatalf("published state = %q, want done", doneSnapshot.State)
		}
		if doneSnapshot.Preview.Files != 12 {
			t.Fatalf("published preview files = %d, want 12", doneSnapshot.Preview.Files)
		}
		if !doneSnapshot.NextSync.After(updating.NextSync) {
			t.Fatalf(
				"done NextSync = %v, want after updating NextSync %v",
				doneSnapshot.NextSync,
				updating.NextSync,
			)
		}
	case <-time.After(time.Second):
		t.Fatal("no done snapshot published")
	}
}

func TestRunWaitsForConfigurationThenStarts(t *testing.T) {
	service, err := New(filepath.Join(t.TempDir(), ".synchub"), runtime.GOOS)
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
