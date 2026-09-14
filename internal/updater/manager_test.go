package updater

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func testManager(t *testing.T, current, goos, arch string) *Manager {
	t.Helper()
	m, err := New(t.TempDir(), current, goos, arch, nil)
	if err != nil {
		t.Fatal(err)
	}
	m.client = fixtureClient(t, nil, "installer", sumLine("installer"))
	m.launch = func(pendingUpdate, string, bool, string) error {
		t.Fatal("test must not launch a real installer")
		return nil
	}
	return m
}

func TestManagerVersionAndPlatformDecisions(t *testing.T) {
	for _, test := range []struct{ current, goos, arch, phase string }{
		{"0.2.3", "windows", "amd64", "ready"},
		{"0.3.0", "windows", "amd64", "upToDate"},
		{"0.4.0", "windows", "amd64", "upToDate"},
		{"0.2.3", "darwin", "arm64", "available"},
		{"0.2.3", "linux", "amd64", "available"},
		{"0.2.3", "windows", "arm64", "available"},
	} {
		t.Run(test.current+test.goos+test.arch, func(t *testing.T) {
			m := testManager(t, test.current, test.goos, test.arch)
			status, err := m.Check(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if status.Phase != test.phase || status.LatestVersion != "v0.3.0" ||
				status.ReleaseURL != RepositoryURL+"/releases/tag/v0.3.0" || status.LastChecked == "" {
				t.Fatalf("status = %#v", status)
			}
			if (m.pending != nil) != (test.phase == "ready") {
				t.Fatal("incorrect staged installer")
			}
		})
	}
}

func TestPreferencesPersistWithoutChangingSyncConfiguration(t *testing.T) {
	m := testManager(t, "0.2.3", "windows", "amd64")
	if !m.Status().Automatic {
		t.Fatal("automatic updates should default on")
	}
	for _, enabled := range []bool{false, true, false} {
		if _, err := m.SetAutomatic(enabled); err != nil {
			t.Fatal(err)
		}
		reloaded, err := New(m.home, "0.2.3", "windows", "amd64", nil)
		if err != nil {
			t.Fatal(err)
		}
		if reloaded.Status().Automatic != enabled {
			t.Fatal("update preference not persisted")
		}
	}
	if _, err := os.Stat(filepath.Join(m.home, "config.yaml")); !os.IsNotExist(err) {
		t.Fatal("updater should not change sync configuration")
	}
	if err := os.WriteFile(filepath.Join(m.home, "update-settings.json"), []byte("invalid JSON"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := New(m.home, "0.2.3", "windows", "amd64", nil); err == nil {
		t.Fatal("invalid preferences were silently ignored")
	}
}

func TestStagedUpdateOnlyLaunchesOnExit(t *testing.T) {
	m := testManager(t, "0.2.3", "windows", "amd64")
	launches := 0
	restarted := false
	m.launch = func(pending pendingUpdate, executable string, restart bool, result string) error {
		launches++
		restarted = restart
		if pending.Version != "v0.3.0" || executable != "test.exe" || result != m.resultPath() {
			t.Fatal("incorrect launch parameters")
		}
		return verifyInstaller(pending.Path, pending.Checksum)
	}
	if _, err := m.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	if launches != 0 {
		t.Fatal("checking must not launch installer")
	}
	if _, err := m.SetAutomatic(false); err != nil {
		t.Fatal(err)
	}
	if err := m.ApplyOnExit("test.exe"); err != nil {
		t.Fatal(err)
	}
	if launches != 0 {
		t.Fatal("disabled updater installed on exit")
	}
	if m.pending != nil {
		t.Fatal("unused installer cache was retained after exit")
	}
	if _, err := m.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := m.RequestRestart(); err != nil {
		t.Fatal(err)
	}
	m.client.http.Transport = transportFunc(func(*http.Request) (*http.Response, error) {
		t.Error("checking after restart was requested must not invalidate the staged update")
		return nil, errors.New("unexpected request during restart")
	})
	if status, err := m.Check(context.Background()); err != nil || status.Phase != "ready" {
		t.Fatalf("restart readiness lost: %#v, %v", status, err)
	}
	if err := m.ApplyOnExit("test.exe"); err != nil {
		t.Fatal(err)
	}
	if launches != 1 || !restarted {
		t.Fatal("explicit restart did not install")
	}
}

func TestAutomaticExitAndFailedLaunchResult(t *testing.T) {
	m := testManager(t, "0.2.3", "windows", "amd64")
	if _, err := m.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	m.launch = func(_ pendingUpdate, _ string, restart bool, _ string) error {
		if restart {
			t.Error("normal Quit must not relaunch the app")
		}
		return errors.New("install directory is read-only")
	}
	if err := m.ApplyOnExit("test.exe"); err == nil {
		t.Fatal("launch error was hidden")
	}
	reloaded, err := New(m.home, "0.2.3", "windows", "amd64", nil)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.Status().Phase != "error" || !strings.Contains(reloaded.Status().Error, "read-only") {
		t.Fatalf("failure did not persist: %#v", reloaded.Status())
	}
}

func TestCacheReuseAndTamperRecovery(t *testing.T) {
	m := testManager(t, "0.2.3", "windows", "amd64")
	if err := m.RequestRestart(); err == nil {
		t.Fatal("restart allowed before download")
	}
	if _, err := m.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	first := m.pending.Path
	if _, err := m.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	if m.pending.Path != first {
		t.Fatal("redownloaded verified cache")
	}
	if err := os.WriteFile(first, []byte("tampered"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := m.RequestRestart(); err == nil {
		t.Fatal("tampered installer allowed restart")
	}
	if _, err := m.Check(context.Background()); err != nil {
		t.Fatal(err)
	}
	if m.pending.Path == first {
		t.Fatal("tampered cache was reused")
	}
	if _, err := os.Stat(first); !os.IsNotExist(err) {
		t.Fatal("stale cache not removed")
	}
}

func TestFailedDownloadsCannotInstallAndCanBeRetried(t *testing.T) {
	m := testManager(t, "0.2.3", "windows", "amd64")
	m.client = fixtureClient(t, nil, "tampered", sumLine("installer"))
	status, err := m.Check(context.Background())
	if err == nil || status.Phase != "error" || status.Error == "" || m.pending != nil {
		t.Fatalf("status = %#v, %v", status, err)
	}
	if err := m.ApplyOnExit("test.exe"); err != nil {
		t.Fatal(err)
	}
	m.client = fixtureClient(t, nil, "installer", sumLine("installer"))
	status, err = m.Check(context.Background())
	if err != nil || status.Phase != "ready" || status.Error != "" {
		t.Fatalf("retry = %#v, %v", status, err)
	}
}

func TestDisabledAndDevelopmentBuildsDoNotPoll(t *testing.T) {
	for _, current := range []string{"dev", "0.2.3"} {
		m := testManager(t, current, "windows", "amd64")
		if current != "dev" {
			if _, err := m.SetAutomatic(false); err != nil {
				t.Fatal(err)
			}
		}
		m.client.http.Transport = transportFunc(func(*http.Request) (*http.Response, error) {
			t.Error("disabled updater requested network")
			return nil, errors.New("unexpected request")
		})
		m.interval = time.Millisecond
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		m.Run(ctx)
		cancel()
	}
}

func TestPeriodicChecksAndCancellation(t *testing.T) {
	m := testManager(t, "0.3.0", "windows", "amd64")
	var checks atomic.Int32
	original := m.client.http.Transport
	m.client.http.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
		checks.Add(1)
		return original.RoundTrip(r)
	})
	m.interval = time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	m.Run(ctx)
	if checks.Load() < 2 {
		t.Fatal("no startup and periodic checks")
	}
}

func TestConcurrentChecksAndCancelledDownload(t *testing.T) {
	m := testManager(t, "0.2.3", "windows", "amd64")
	started := make(chan struct{})
	m.client.http.Transport = transportFunc(func(r *http.Request) (*http.Response, error) {
		close(started)
		<-r.Context().Done()
		return nil, r.Context().Err()
	})
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { _, err := m.Check(ctx); done <- err }()
	<-started
	if _, err := m.Check(context.Background()); err == nil {
		t.Fatal("overlapping check accepted")
	}

	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v", err)
	}
	if m.pending != nil {
		t.Fatal("cancelled download staged an installer")
	}
}

func TestPublishedReleaseDownload(t *testing.T) {
	expected := os.Getenv("SYNCHUB_TEST_RELEASE")
	if expected == "" {
		t.Skip("set SYNCHUB_TEST_RELEASE to verify a published release without installing it")
	}
	m, err := New(t.TempDir(), "0.0.0", "windows", "amd64", nil)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	status, err := m.Check(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if status.LatestVersion != expected || status.Phase != "ready" || m.pending == nil {
		t.Fatalf("published update was not staged: %#v", status)
	}
	if err := verifyInstaller(m.pending.Path, m.pending.Checksum); err != nil {
		t.Fatal(err)
	}
}
