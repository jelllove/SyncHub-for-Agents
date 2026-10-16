//go:build windows && !server

package desktop

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
	"time"

	"github.com/qinqingxu/synchub-for-agents/internal/tray"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/w32"
	"golang.org/x/sys/windows"
)

func TestNativeTipReshowKeepsContentAndFocus(t *testing.T) {
	if os.Getenv("SYNCHUB_NATIVE_TRAY_TEST") != "1" {
		t.Skip("opt-in native WebView2 test; requires built frontend assets")
	}
	if os.Getenv("SYNCHUB_NATIVE_TRAY_CHILD") != "1" {
		profile := filepath.Join(t.TempDir(), "webview")
		t.Cleanup(func() {
			deadline := time.Now().Add(10 * time.Second)
			for {
				err := os.RemoveAll(profile)
				if err == nil {
					return
				}
				if (!errors.Is(err, os.ErrPermission) && !errors.Is(err, windows.ERROR_SHARING_VIOLATION)) || time.Now().After(deadline) {
					t.Errorf("clean up isolated WebView2 profile: %v", err)
					return
				}
				time.Sleep(100 * time.Millisecond)
			}
		})
		executable, err := os.Executable()
		if err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithTimeout(context.Background(), 110*time.Second)
		defer cancel()
		command := exec.CommandContext(ctx, executable, "-test.run=^TestNativeTipReshowKeepsContentAndFocus$", "-test.v")
		command.Env = append(os.Environ(), "SYNCHUB_NATIVE_TRAY_CHILD=1", "SYNCHUB_NATIVE_TRAY_PROFILE="+profile)
		output, err := command.CombinedOutput()
		t.Logf("%s", output)
		if err != nil {
			t.Fatalf("native tray subprocess: %v", err)
		}
		return
	}
	profile := os.Getenv("SYNCHUB_NATIVE_TRAY_PROFILE")
	if profile == "" {
		t.Fatal("native tray subprocess requires an isolated WebView2 profile")
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if hr := w32.CoInitializeEx(w32.COINIT_APARTMENTTHREADED); hr != 0 && hr != 1 {
		t.Fatalf("initialize test UI thread COM apartment: %#x", hr)
	}
	defer w32.CoUninitialize()
	dist, err := filepath.Abs(filepath.Join("..", "..", "..", "frontend", "dist"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dist, "index.html")); err != nil {
		t.Fatalf("build frontend assets before running the native test: %v", err)
	}
	app := application.New(application.Options{
		Name: "SyncHub isolated tray test",
		Assets: application.AssetOptions{
			Handler: application.BundledAssetFileServer(os.DirFS(dist)),
		},
		Windows: application.WindowsOptions{
			WebviewUserDataPath:           profile,
			DisableQuitOnLastWindowClosed: true,
		},
	})
	tip := app.Window.NewWithOptions(tipWindowOptions())
	sentinel := app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name: "isolated-focus-sentinel", Title: "SyncHub isolated focus sentinel",
		HTML:  "<!doctype html><html><body>Isolated focus sentinel</body></html>",
		Width: 240, Height: 120, Hidden: true,
	})
	ready := make(chan struct{})
	var once sync.Once
	app.Event.On("tray-tip:ready", func(event *application.CustomEvent) {
		if event.Sender == TipWindowName {
			once.Do(func() { close(ready) })
		}
	})
	type observation struct {
		Visibility string `json:"visibility"`
		Kind       string `json:"kind"`
		Title      string `json:"title"`
		Buttons    int    `json:"buttons"`
	}
	observations := make(chan observation, 1)
	app.Event.On("native-tip:probe", func(event *application.CustomEvent) {
		if event.Sender != TipWindowName {
			return
		}
		data, err := json.Marshal(event.Data)
		if err != nil {
			t.Error(err)
			return
		}
		var observed observation
		if err := json.Unmarshal(data, &observed); err != nil {
			t.Error(err)
			return
		}
		select {
		case observations <- observed:
		default:
		}
	})
	activeWindow := windows.NewLazySystemDLL("user32.dll").NewProc("GetActiveWindow")
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	result := make(chan error, 1)
	go func() {
		defer app.Quit()
		result <- func() error {
			select {
			case <-ready:
			case <-ctx.Done():
				return fmt.Errorf("tip frontend did not become ready: %w", ctx.Err())
			}
			for index, status := range []tray.Tip{
				{Kind: "scanning", Title: "Scanning", Message: "Finding portable resources and applying safety exclusions."},
				{Kind: "warning", Title: "Needs attention", Message: "Review conflicts, blocked files or pending installations in the app."},
				{Kind: "done", Title: "Sync complete", Message: "Your selected resources have been synchronized."},
			} {
				cycle := index + 1
				tip.Hide()
				sentinel.Show()
				var before uintptr
				application.InvokeSync(func() { before, _, _ = activeWindow.Call() })
				if before == 0 || before != uintptr(sentinel.NativeWindow()) {
					return fmt.Errorf("cycle %d: no active isolated focus sentinel", cycle)
				}
				status.Revision = uint64(cycle)
				app.Event.Emit("tray-tip:status", status)
				if err := showTipWindow(tip); err != nil {
					return err
				}
				var after uintptr
				application.InvokeSync(func() { after, _, _ = activeWindow.Call() })
				if after != before {
					return fmt.Errorf("cycle %d: tip stole focus: %x -> %x", cycle, before, after)
				}
				deadline := time.NewTimer(5 * time.Second)
				ticker := time.NewTicker(100 * time.Millisecond)
				var last observation
				visible := false
			waitForContent:
				for {
					select {
					case <-ticker.C:
						// A second runtime import replaces the built frontend's event dispatcher.
						tip.ExecJS(`fetch('/wails/runtime', {
							method: 'POST',
							headers: {'Content-Type': 'application/json', 'x-wails-client-id': 'native-tray-test', 'x-wails-window-name': 'synchub-tray-tip'},
							body: JSON.stringify({object: 3, method: 0, args: {name: 'native-tip:probe', data: {
								visibility: document.visibilityState,
								kind: document.querySelector('.tray-tip-card')?.dataset.kind ?? '',
								title: document.querySelector('.tray-tip-content h1')?.textContent ?? '',
								buttons: document.querySelectorAll('.tray-tip-card button').length
							}}})
						}).then(response => { if (!response.ok) throw new Error('Native tip probe failed: ' + response.status) })`)
					case last = <-observations:
						if last.Visibility == "visible" && last.Kind == status.Kind && last.Title != "" && last.Buttons == 3 {
							visible = true
							break waitForContent
						}
					case <-deadline.C:
						break waitForContent
					case <-ctx.Done():
						break waitForContent
					}
				}
				deadline.Stop()
				ticker.Stop()
				if !visible {
					return fmt.Errorf("cycle %d: native tip content not visible: %+v", cycle, last)
				}
				t.Logf("cycle %d: visible activity title %q, three controls, focus preserved", cycle, last.Title)
			}
			tip.Hide()
			return nil
		}()
	}()
	if err := app.Run(); err != nil {
		t.Fatal(err)
	}
	if err := <-result; err != nil {
		t.Fatal(err)
	}
}
