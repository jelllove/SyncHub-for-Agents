package desktop

import (
	"context"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/qinqingxu/synchub-for-agents/internal/syncengine"
)

func TestDesktopRevisionOrdersSnapshotAcknowledgementAndProgress(t *testing.T) {
	service, err := New(configuredHome(t), runtime.GOOS)
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	service.recordCycle(noticeCycle())
	initial := noticeSnapshot(t, service)
	wails := NewWailsService(nil, service, nil, nil)
	acknowledged, err := wails.AcknowledgeSyncNotices(initial.SyncNotices.Fingerprint)
	if err != nil {
		t.Fatal(err)
	}
	var progress Progress
	unsubscribe := service.SubscribeProgress(func(update Progress) { progress = update })
	defer unsubscribe()
	service.recordProgress(syncengine.Progress{Stage: syncengine.StageApplying, Percentage: 60})
	newer := noticeSnapshot(t, service)
	if initial.Revision == 0 || acknowledged.Revision <= initial.Revision ||
		progress.Revision <= acknowledged.Revision || newer.Revision <= progress.Revision {
		t.Fatalf("unordered revisions: initial=%d ack=%d progress=%d newer=%d",
			initial.Revision, acknowledged.Revision, progress.Revision, newer.Revision)
	}
	if newer.Progress.Revision != progress.Revision {
		t.Fatal("snapshot did not retain the sequenced progress update")
	}
}

func TestDesktopRevisionDoesNotRestartWhenLocalSetupIsReset(t *testing.T) {
	service := resetFixture(t)
	before := noticeSnapshot(t, service)
	if err := service.ResetLocalSetup(context.Background(), "RESET", filepath.Join(service.home, "repo")); err != nil {
		t.Fatal(err)
	}
	after := noticeSnapshot(t, service)
	if after.Revision <= before.Revision || after.Configured {
		t.Fatalf("reset revision regressed: before=%d after=%d", before.Revision, after.Revision)
	}
}
