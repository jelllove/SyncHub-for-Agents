package desktop

import (
	"context"
	"testing"

	"github.com/qinqingxu/synchub-for-agents/internal/updater"
)

func TestUpdateBindingsDelegateAndRejectUninitializedService(t *testing.T) {
	service := NewWailsService(nil, nil, nil, nil)
	if _, err := service.UpdateStatus(); err == nil {
		t.Fatal("missing updater accepted")
	}
	if _, err := service.CheckForUpdates(context.Background()); err == nil {
		t.Fatal("missing updater accepted")
	}
	if _, err := service.SetAutomaticUpdates(false); err == nil {
		t.Fatal("missing updater accepted")
	}
	if err := service.RestartToUpdate(); err == nil {
		t.Fatal("missing updater accepted")
	}
	manager, err := updater.New(t.TempDir(), "dev", "windows", "amd64", nil)
	if err != nil {
		t.Fatal(err)
	}
	core, err := New(t.TempDir(), "windows")
	if err != nil {
		t.Fatal(err)
	}
	service = WithUpdates(NewWailsService(nil, core, nil, nil), manager, func() { t.Fatal("must not quit without a verified update") })
	status, err := service.UpdateStatus()
	if err != nil || status.CurrentVersion != "dev" {
		t.Fatalf("status = %#v, %v", status, err)
	}
	status, err = service.SetAutomaticUpdates(false)
	if err != nil || status.Automatic {
		t.Fatalf("settings = %#v, %v", status, err)
	}
	if err := service.RestartToUpdate(); err == nil {
		t.Fatal("restart without download accepted")
	}
}
