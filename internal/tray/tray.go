package tray

import (
	"context"
	"os/exec"

	"fyne.io/systray"

	"github.com/qinqingxu/acsync/internal/cli"
	"github.com/qinqingxu/acsync/internal/config"
	"github.com/qinqingxu/acsync/internal/daemon"
	"github.com/qinqingxu/acsync/internal/scheduler"
	"github.com/qinqingxu/acsync/internal/settings"
)

// App binds a daemon to a system-tray UI.
type App struct {
	Home string
	GOOS string

	d                *daemon.Daemon
	cancel           context.CancelFunc
	settingsURL      string
	shutdownSettings func(context.Context) error
}

type agentItem struct {
	name string
	item *systray.MenuItem
}

// Run builds a daemon and runs the tray until the user quits.
func Run(home, goos string) error {
	d, err := daemon.New(home, goos)
	if err != nil {
		return err
	}
	app := &App{Home: home, GOOS: goos, d: d}
	systray.Run(app.onReady, app.onExit)
	return nil
}

func (a *App) onReady() {
	systray.SetTitle("acsync")
	systray.SetTooltip("AgentConfigSync")
	systray.SetIcon(Icon(scheduler.StateIdle, a.GOOS))

	// Repaint the icon (and keep logging) on every state change.
	a.d.Scheduler.Subscribe(func(s scheduler.State) {
		a.d.Logger.Printf("state: %s", s)
		systray.SetIcon(Icon(s, a.GOOS))
	})

	if addr, shutdown, err := settings.Serve(a.Home); err == nil {
		a.settingsURL = "http://" + addr + "/"
		a.shutdownSettings = shutdown
	} else {
		a.d.Logger.Printf("settings server error: %v", err)
	}

	mSync := systray.AddMenuItem("Sync now", "Trigger a sync immediately")
	mPause := systray.AddMenuItem("Pause", "Pause syncing")
	mSettings := systray.AddMenuItem("Settings…", "Open the settings page")
	mLogs := systray.AddMenuItem("Open logs", "Open the log folder")
	systray.AddSeparator()
	agents := a.addAgentItems()
	systray.AddSeparator()
	mQuit := systray.AddMenuItem("Quit", "Stop acsync")

	ctx, cancel := context.WithCancel(context.Background())
	a.cancel = cancel
	go a.d.Run(ctx)
	go a.handleMenu(mSync, mPause, mSettings, mLogs, mQuit, agents)
}

func (a *App) addAgentItems() []agentItem {
	cfg, err := config.Load(cli.ConfigPath(a.Home))
	if err != nil {
		a.d.Logger.Printf("load config for menu: %v", err)
		return nil
	}
	providers, err := cli.LoadProviders(a.Home)
	if err != nil {
		a.d.Logger.Printf("load providers for menu: %v", err)
		return nil
	}
	var items []agentItem
	for _, p := range providers {
		it := systray.AddMenuItemCheckbox(p.Name, "Sync "+p.Name, cfg.Agents[p.Name])
		items = append(items, agentItem{name: p.Name, item: it})
	}
	return items
}

func (a *App) handleMenu(mSync, mPause, mSettings, mLogs, mQuit *systray.MenuItem, agents []agentItem) {
	for _, ai := range agents {
		ai := ai
		go func() {
			for range ai.item.ClickedCh {
				on := !ai.item.Checked()
				if on {
					ai.item.Check()
				} else {
					ai.item.Uncheck()
				}
				a.toggleAgent(ai.name, on)
			}
		}()
	}
	for {
		select {
		case <-mSync.ClickedCh:
			a.d.Scheduler.Trigger()
		case <-mPause.ClickedCh:
			if a.d.Scheduler.State() == scheduler.StatePaused {
				a.d.Scheduler.Resume()
			} else {
				a.d.Scheduler.Pause()
			}
			mPause.SetTitle(pauseTitle(a.d.Scheduler.State() == scheduler.StatePaused))
		case <-mSettings.ClickedCh:
			a.openTarget(a.settingsURL)
		case <-mLogs.ClickedCh:
			a.openTarget(cli.LogsDir(a.Home))
		case <-mQuit.ClickedCh:
			systray.Quit()
			return
		}
	}
}

func (a *App) toggleAgent(name string, on bool) {
	cfg, err := config.Load(cli.ConfigPath(a.Home))
	if err != nil {
		a.d.Logger.Printf("toggle load error: %v", err)
		return
	}
	if cfg.Agents == nil {
		cfg.Agents = map[string]bool{}
	}
	cfg.Agents[name] = on
	if err := config.Save(cli.ConfigPath(a.Home), cfg); err != nil {
		a.d.Logger.Printf("toggle save error: %v", err)
	}
}

func (a *App) openTarget(target string) {
	if target == "" {
		return
	}
	name, args := openCommand(a.GOOS, target)
	if err := exec.Command(name, args...).Start(); err != nil {
		a.d.Logger.Printf("open %q error: %v", target, err)
	}
}

func (a *App) onExit() {
	if a.cancel != nil {
		a.cancel()
	}
	if a.shutdownSettings != nil {
		_ = a.shutdownSettings(context.Background())
	}
}
