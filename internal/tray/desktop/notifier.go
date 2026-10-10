package desktop

import (
	"fmt"
	"log"
	"sync"
	"sync/atomic"

	"github.com/qinqingxu/synchub-for-agents/internal/scheduler"
	"github.com/qinqingxu/synchub-for-agents/internal/tray"
	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/events"
)

const (
	TipWindowName     = "synchub-tray-tip"
	windowsNoActivate = 0x08000000
	windowsToolWindow = 0x00000080
	windowsTopmost    = 0x00000008
)

type Notifier struct {
	app         *application.App
	tray        *application.SystemTray
	window      *application.WebviewWindow
	feedback    *tray.Feedback
	closed      atomic.Bool
	closeOnce   sync.Once
	unsubscribe []func()
	lastIcon    scheduler.State
	lastTooltip string
}

func desktopIcon(state scheduler.State) []byte {
	return tray.PNGIcon(state)
}

func tipWindowOptions() application.WebviewWindowOptions {
	return application.WebviewWindowOptions{
		Name: TipWindowName, Title: "SyncHub activity", URL: "/?view=tray-tip",
		Width: 360, Height: 168, Hidden: true, AlwaysOnTop: true,
		Frameless: true, DisableResize: true,
		BackgroundColour: application.NewRGB(251, 252, 250),
		Windows: application.WindowsWindow{
			HiddenOnTaskbar: true,
			ExStyle:         windowsNoActivate | windowsToolWindow | windowsTopmost,
		},
	}
}

func NewNotifier(app *application.App, systemTray *application.SystemTray, open func()) *Notifier {
	notifier := &Notifier{app: app, tray: systemTray}
	notifier.feedback = tray.NewFeedback(notifier.render, func(err error) { log.Printf("tray activity feedback: %v", err) })
	if popupSupported() {
		notifier.window = app.Window.NewWithOptions(tipWindowOptions())
		notifier.window.RegisterHook(events.Common.WindowClosing, func(event *application.WindowEvent) {
			if notifier.closed.Load() {
				return
			}
			notifier.feedback.Dismiss()
			event.Cancel()
		})
		notifier.unsubscribe = append(notifier.unsubscribe,
			app.Event.On("tray-tip:ready", func(event *application.CustomEvent) {
				if event.Sender == TipWindowName {
					notifier.feedback.Ready()
				}
			}),
			app.Event.On("tray-tip:dismiss", func(event *application.CustomEvent) {
				if event.Sender == TipWindowName {
					notifier.feedback.Dismiss()
				}
			}),
			app.Event.On("tray-tip:open", func(event *application.CustomEvent) {
				if event.Sender == TipWindowName && !notifier.closed.Load() {
					notifier.feedback.Dismiss()
					open()
				}
			}),
		)
	}
	app.OnShutdown(notifier.Close)
	notifier.feedback.State(scheduler.StateIdle)
	return notifier
}

func (n *Notifier) State(state scheduler.State) {
	n.feedback.State(state)
}

func (n *Notifier) Progress(stage string) {
	if err := n.feedback.Progress(stage); err != nil {
		log.Printf("tray activity progress: %v", err)
	}
}

func (n *Notifier) Finish(failed, needsAttention bool) {
	n.feedback.Finish(failed, needsAttention)
}

func (n *Notifier) Reviewed(needsAttention bool) {
	n.feedback.Reviewed(needsAttention)
}

func (n *Notifier) Close() {
	n.closeOnce.Do(func() {
		n.closed.Store(true)
		n.feedback.Close()
		for _, unsubscribe := range n.unsubscribe {
			unsubscribe()
		}
	})
}

func (n *Notifier) render(update tray.FeedbackUpdate) error {
	if n.closed.Load() {
		return nil
	}
	if n.lastIcon != update.IconState {
		n.tray.SetIcon(desktopIcon(update.IconState))
		n.lastIcon = update.IconState
	}
	if n.lastTooltip != update.Tooltip {
		n.tray.SetTooltip(update.Tooltip)
		n.lastTooltip = update.Tooltip
	}
	if n.window == nil {
		return nil
	}
	n.app.Event.Emit("tray-tip:status", update.Tip)
	if !update.Show {
		n.window.Hide()
		return nil
	}
	if err := n.tray.PositionWindow(n.window, 12); err != nil {
		return fmt.Errorf("position activity tip beside the tray: %w", err)
	}
	if !n.closed.Load() {
		n.window.Show()
	}
	return nil
}
