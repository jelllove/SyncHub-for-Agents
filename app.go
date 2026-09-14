package main

import (
	"context"
	"log"
	"os"
	"os/exec"
	"runtime"
	"sync"
	"sync/atomic"

	"github.com/qinqingxu/synchub-for-agents/internal/appversion"
	"github.com/qinqingxu/synchub-for-agents/internal/desktop"
	"github.com/qinqingxu/synchub-for-agents/internal/onboarding"
	"github.com/qinqingxu/synchub-for-agents/internal/scheduler"
	"github.com/qinqingxu/synchub-for-agents/internal/startup"
	"github.com/qinqingxu/synchub-for-agents/internal/tray"
	"github.com/qinqingxu/synchub-for-agents/internal/updater"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

type guiApplication struct {
	app        *application.App
	window     *application.WebviewWindow
	tray       *application.SystemTray
	service    *desktop.Service
	startup    *startup.Manager
	activation activationQueue
	isQuitting atomic.Bool
	updates    *updater.Manager
}

type activationQueue struct {
	mu      sync.Mutex
	pending bool
	show    func()
}

func (q *activationQueue) Request() {
	q.mu.Lock()
	show := q.show
	if show == nil {
		q.pending = true
	}
	q.mu.Unlock()
	if show != nil {
		show()
	}
}

func (q *activationQueue) Ready(show func()) {
	q.mu.Lock()
	q.show = show
	pending := q.pending
	q.pending = false
	q.mu.Unlock()
	if pending {
		show()
	}
}

func newGUIApplication() *guiApplication {
	gui := &guiApplication{}
	gui.app = application.New(application.Options{
		Name:        "SyncHub for Agents",
		Description: "Synchronize AI agent configs and sessions across computers",
		Assets: application.AssetOptions{
			Handler: application.BundledAssetFileServer(frontendAssets),
		},
		Mac: application.MacOptions{
			ApplicationShouldTerminateAfterLastWindowClosed: false,
		},
		Windows: application.WindowsOptions{
			DisableQuitOnLastWindowClosed: true,
		},
		Linux: application.LinuxOptions{
			DisableQuitOnLastWindowClosed: true,
			ProgramName:                   "synchub",
		},
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID: "com.qinqingxu.synchub",
			OnSecondInstanceLaunch: func(application.SecondInstanceData) {
				gui.show()
			},
		},
	})
	return gui
}

func (gui *guiApplication) configure(
	core *desktop.Service,
	onboardingService *onboarding.Service,
	hidden bool,
	home string,
) error {
	gui.service = core
	gui.startup = &startup.Manager{
		Backend:    gui.app.Autostart,
		Identifier: "io.github.qinqingxu.synchub",
		Arguments:  []string{"--hidden"},
		GOOS:       runtime.GOOS,
	}
	if err := gui.migrateLegacyStartup(); err != nil {
		return err
	}
	updates, err := updater.New(home, appversion.Version, runtime.GOOS, runtime.GOARCH, func(status updater.Status) {
		gui.app.Event.Emit(desktop.UpdateEvent, status)
	})
	if err != nil {
		return err
	}
	gui.updates = updates
	gui.app.RegisterService(application.NewService(desktop.WithUpdates(
		desktop.NewWailsService(gui.app, core, onboardingService, gui.startup),
		updates,
		func() {
			gui.isQuitting.Store(true)
			gui.app.Quit()
		},
	)))
	gui.window = gui.app.Window.NewWithOptions(initialMainWindowOptions())
	gui.app.Event.OnApplicationEvent(events.Common.ApplicationStarted, func(*application.ApplicationEvent) {
		screen := gui.app.Screen.GetPrimary()
		if screen == nil || screen.WorkArea.Width <= 0 || screen.WorkArea.Height <= 0 {
			log.Printf("primary screen work area unavailable; using default window size")
			screen = nil
		}
		applyMainWindowLayout(gui.window, screen, hidden)
		gui.activation.Ready(gui.showWindow)
	})
	gui.window.RegisterHook(events.Common.WindowClosing, func(event *application.WindowEvent) {
		if shouldCancelWindowClose(gui.isQuitting.Load()) {
			gui.window.Hide()
			event.Cancel()
		}
	})
	gui.app.Event.OnApplicationEvent(events.Mac.ApplicationShouldHandleReopen, func(*application.ApplicationEvent) {
		gui.show()
	})
	gui.configureTray()
	return nil
}

func (g *guiApplication) migrateLegacyStartup() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	removed, err := startup.RemoveLegacy(runtime.GOOS, home, func(name string, args ...string) error {
		return exec.Command(name, args...).Run()
	})
	if err != nil {
		return err
	}
	if len(removed) == 0 {
		return nil
	}
	log.Printf("migrated legacy startup entry: %v", removed)
	return g.startup.Enable()
}

func (g *guiApplication) configureTray() {
	g.tray = g.app.SystemTray.New()
	g.tray.SetIcon(tray.Icon(scheduler.StateIdle, runtime.GOOS))
	g.tray.SetTooltip("SyncHub for Agents")

	menu := g.app.NewMenu()
	menu.Add("Open SyncHub for Agents").OnClick(func(*application.Context) {
		g.show()
	})
	menu.AddSeparator()
	menu.Add("Sync Now").OnClick(func(*application.Context) {
		if err := g.service.Trigger(); err != nil && err != desktop.ErrNotConfigured {
			log.Printf("trigger sync: %v", err)
		}
	})
	pauseItem := menu.Add("Pause").OnClick(func(*application.Context) {
		daemon := g.service.Daemon()
		if daemon == nil {
			return
		}
		if daemon.Scheduler.State() == scheduler.StatePaused {
			daemon.Scheduler.Resume()
			return
		}
		daemon.Scheduler.Pause()
	})
	menu.AddSeparator()
	menu.Add("Quit").OnClick(func(*application.Context) {
		g.isQuitting.Store(true)
		g.app.Quit()
	})
	g.tray.SetMenu(menu)
	g.tray.OnClick(g.show)

	g.service.SubscribeState(func(state scheduler.State) {
		g.tray.SetIcon(tray.Icon(state, runtime.GOOS))
		if state == scheduler.StatePaused {
			pauseItem.SetLabel("Resume")
		} else {
			pauseItem.SetLabel("Pause")
		}
		menu.Update()
	})
}

func (g *guiApplication) show() {
	g.activation.Request()
}

func (g *guiApplication) showWindow() {
	g.window.Show()
	g.window.Restore()
	g.window.Focus()
}

func (g *guiApplication) run() error {
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		g.updates.Run(ctx)
	}()
	err := g.app.Run()
	cancel()
	<-done
	if err != nil {
		return err
	}
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	return g.updates.ApplyOnExit(executable)
}

func shouldCancelWindowClose(isQuitting bool) bool {
	return !isQuitting
}
