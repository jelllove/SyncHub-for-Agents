package desktop

import (
	"path/filepath"
	"testing"

	"github.com/qinqingxu/synchub-for-agents/internal/tray"
)

func TestTrayTipServicePersistsPreferenceAndRejectsMissingBackend(t *testing.T) {
	service := NewWailsService(nil, nil, nil, nil)
	if _, err := service.TrayTipsEnabled(); err == nil {
		t.Fatal("missing tray preference manager returned success")
	}
	if err := service.SetTrayTipsEnabled(false); err == nil {
		t.Fatal("missing manager accepted preference update")
	}
	path := filepath.Join(t.TempDir(), "tray-settings.json")
	prefs, err := tray.NewTipPreferences(path)
	if err != nil {
		t.Fatal(err)
	}
	service = WithTrayTips(service, prefs)
	enabled, err := service.TrayTipsEnabled()
	if err != nil || !enabled {
		t.Fatalf("initial status = %t, %v", enabled, err)
	}
	if err := service.SetTrayTipsEnabled(false); err != nil {
		t.Fatal(err)
	}
	reloaded, err := tray.NewTipPreferences(path)
	if err != nil || reloaded.Enabled() {
		t.Fatalf("service preference did not persist: %v", err)
	}
	if err := service.SetTrayTipsEnabled(true); err != nil {
		t.Fatal(err)
	}
	if enabled, err := service.TrayTipsEnabled(); err != nil || !enabled {
		t.Fatalf("re-enabled status = %t, %v", enabled, err)
	}
}
