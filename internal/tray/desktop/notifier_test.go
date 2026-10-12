package desktop

import "testing"

func TestTipWindowIsHiddenNonActivatingAndAbsentFromTaskbar(t *testing.T) {
	options := tipWindowOptions()
	if !options.Hidden || !options.AlwaysOnTop || !options.Frameless || !options.DisableResize ||
		!options.Windows.HiddenOnTaskbar || options.Windows.ExStyle&windowsNoActivate == 0 ||
		options.Windows.ExStyle&windowsToolWindow == 0 || options.Windows.ExStyle&windowsTopmost == 0 {
		t.Fatalf("activity tips must not steal focus or add a taskbar icon: %#v", options)
	}
	if options.Name != TipWindowName || options.URL != "/?view=tray-tip" || options.Width != 300 || options.Height != 132 {
		t.Fatalf("unexpected popup size/route: %#v", options)
	}
	if options.AllowSimpleEventEmit {
		t.Fatal("the popup must not enable unscoped raw event commands")
	}
}
