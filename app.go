package main

import (
	"log"
	"runtime"

	"github.com/qinqingxu/acsync/internal/desktop"
	"github.com/qinqingxu/acsync/internal/scheduler"
	"github.com/qinqingxu/acsync/internal/tray"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

type guiApplication struct {
	app     *application.App
	window  *application.WebviewWindow
	tray    *application.SystemTray
	service *desktop.Service
}

func newGUIApplication(core *desktop.Service) *guiApplication {
	gui := &guiApplication{service: core}
	gui.app = application.New(application.Options{
		Name:        "AgentConfigSync",
		Description: "Synchronize AI agent settings and sessions across computers",
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
			ProgramName:                   "agentconfigsync",
		},
		SingleInstance: &application.SingleInstanceOptions{
			UniqueID: "com.qinqingxu.agentconfigsync",
			OnSecondInstanceLaunch: func(application.SecondInstanceData) {
				gui.show()
			},
		},
	})

	gui.app.RegisterService(application.NewService(desktop.NewWailsService(gui.app, core)))
	gui.window = gui.app.Window.NewWithOptions(application.WebviewWindowOptions{
		Name:      "main",
		Title:     "AgentConfigSync",
		URL:       "/",
		Width:     1080,
		Height:    720,
		MinWidth:  820,
		MinHeight: 560,
	})
	gui.window.RegisterHook(events.Common.WindowClosing, func(event *application.WindowEvent) {
		gui.window.Hide()
		event.Cancel()
	})
	gui.app.Event.OnApplicationEvent(events.Mac.ApplicationShouldHandleReopen, func(*application.ApplicationEvent) {
		gui.show()
	})
	gui.configureTray()
	return gui
}

func (g *guiApplication) configureTray() {
	g.tray = g.app.SystemTray.New()
	g.tray.SetIcon(tray.Icon(scheduler.StateIdle, runtime.GOOS))
	g.tray.SetTooltip("AgentConfigSync")

	menu := g.app.NewMenu()
	menu.Add("Open AgentConfigSync").OnClick(func(*application.Context) {
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
	if g.window == nil {
		return
	}
	g.window.Show()
	g.window.Restore()
	g.window.Focus()
}

func (g *guiApplication) run() error {
	return g.app.Run()
}
