package desktop

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/qinqingxu/synchub-for-agents/internal/conflict"
	"github.com/qinqingxu/synchub-for-agents/internal/daemon"
	"github.com/qinqingxu/synchub-for-agents/internal/installplan"
	"github.com/qinqingxu/synchub-for-agents/internal/resource"
	"github.com/qinqingxu/synchub-for-agents/internal/scheduler"
	"github.com/qinqingxu/synchub-for-agents/internal/syncengine"
)

func noticeCycle() daemon.CycleResult {
	skipped := resource.Issue{ResourceKey: "demo/source", Path: "cache.tmp", Code: "generated-content", Message: "Excluded by policy", Bytes: 12}
	blocked := resource.Issue{ResourceKey: "demo/source", Path: "secret.json", Code: "secret-detected", Message: "Credential content"}
	return daemon.CycleResult{
		Skipped: 2, Blocked: 1, NeedsAttention: true, IssueDetailsVersion: 1,
		Issues:        []resource.Issue{blocked, skipped, skipped},
		SkippedIssues: []resource.Issue{skipped, skipped}, BlockedIssues: []resource.Issue{blocked},
		FinishedAt: time.Date(2026, 10, 3, 10, 0, 0, 0, time.UTC),
	}
}

func noticeSnapshot(t *testing.T, service *Service) Snapshot {
	t.Helper()
	snapshot, err := service.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func TestSyncNoticesPersistExactRecordsAndReviewAcrossRestart(t *testing.T) {
	home := configuredHome(t)
	service, err := New(home, runtime.GOOS)
	if err != nil {
		t.Fatal(err)
	}
	cycle := noticeCycle()
	service.recordCycle(cycle)
	writeDesktopPreview(t, home, ResourcePreview{Issues: []ResourceIssue{{Path: "not-from-cycle"}}})
	before := noticeSnapshot(t, service)
	if before.State != "error" || !before.Progress.NeedsAttention || before.Progress.Skipped != 2 {
		t.Fatalf("unreviewed snapshot = %#v", before)
	}
	if !before.SyncNotices.DetailsAvailable || len(before.SyncNotices.Issues) != 3 || before.SyncNotices.Reviewed {
		t.Fatalf("notices = %#v", before.SyncNotices)
	}
	if before.SyncNotices.Issues[0].Path != "cache.tmp" || before.SyncNotices.Issues[2].Code != "secret-detected" {
		t.Fatalf("exact grouped issues = %#v", before.SyncNotices.Issues)
	}
	wails := NewWailsService(nil, service, nil, nil)
	reviewed, err := wails.AcknowledgeSyncNotices(before.SyncNotices.Fingerprint)
	if err != nil {
		t.Fatal(err)
	}
	if !reviewed.SyncNotices.Reviewed || reviewed.State != "idle" || reviewed.Progress.NeedsAttention {
		t.Fatalf("reviewed snapshot = %#v", reviewed)
	}
	if !reflect.DeepEqual(before.SyncNotices.Issues, reviewed.SyncNotices.Issues) {
		t.Fatal("acknowledgement removed diagnostic records")
	}
	if err := service.Close(); err != nil {
		t.Fatal(err)
	}
	service, err = New(home, runtime.GOOS)
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	restored := noticeSnapshot(t, service)
	if !restored.SyncNotices.Reviewed || restored.Progress.NeedsAttention || restored.Progress.Skipped != 2 {
		t.Fatalf("restored snapshot = %#v", restored)
	}
	cycle.FinishedAt = cycle.FinishedAt.Add(time.Hour)
	cycle.Issues[0], cycle.Issues[1] = cycle.Issues[1], cycle.Issues[0]
	service.recordCycle(cycle)
	var event Progress
	unsubscribe := service.SubscribeProgress(func(progress Progress) { event = progress })
	defer unsubscribe()
	service.recordProgress(syncengine.Progress{Stage: syncengine.StageComplete, NeedsAttention: true, Skipped: 2, BlockedFiles: 1})
	if !noticeSnapshot(t, service).SyncNotices.Reviewed || event.NeedsAttention {
		t.Fatal("unchanged issues lost review in snapshot or completion event")
	}
}

func TestSyncNoticesRejectStaleReviewAndRequireReviewForChanges(t *testing.T) {
	service, err := New(configuredHome(t), runtime.GOOS)
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	cycle := noticeCycle()
	service.recordCycle(cycle)
	old := noticeSnapshot(t, service).SyncNotices.Fingerprint
	if err := service.AcknowledgeSyncNotices(old); err != nil {
		t.Fatal(err)
	}
	cycle.SkippedIssues[0].Message = "Policy changed"
	cycle.Issues[1] = cycle.SkippedIssues[0]
	service.recordCycle(cycle)
	got := noticeSnapshot(t, service)
	if got.SyncNotices.Reviewed || !got.Progress.NeedsAttention || got.State != "error" {
		t.Fatalf("changed notices = %#v", got)
	}
	if err := service.AcknowledgeSyncNotices(old); err == nil || !strings.Contains(err.Error(), "changed") {
		t.Fatalf("stale acknowledgement = %v", err)
	}
	if noticeSnapshot(t, service).SyncNotices.Reviewed {
		t.Fatal("stale confirmation reviewed a new issue set")
	}
}

func TestSyncNoticesCanReviewReportedCollectionExclusions(t *testing.T) {
	service, err := New(configuredHome(t), runtime.GOOS)
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	cycle := noticeCycle()
	missing := resource.Issue{ResourceKey: "cursor/settings", Path: "missing", Code: "root-unavailable", Message: "Source directory does not exist"}
	projection := resource.Issue{ResourceKey: "demo/settings", Path: "settings.json", Code: "projection-failed", Message: "Cannot parse portable configuration"}
	cycle.SkippedIssues = append(cycle.SkippedIssues, missing)
	cycle.BlockedIssues = append(cycle.BlockedIssues, projection)
	cycle.Issues = append(cycle.Issues, missing, projection)
	cycle.Skipped++
	cycle.Blocked++
	service.recordCycle(cycle)
	before := noticeSnapshot(t, service)
	for _, issue := range before.SyncNotices.Issues {
		if !issue.Reviewable {
			t.Fatalf("reported exclusion cannot be reviewed: %#v", issue)
		}
	}
	if err := service.AcknowledgeSyncNotices(before.SyncNotices.Fingerprint); err != nil {
		t.Fatal(err)
	}
	reviewed := noticeSnapshot(t, service)
	if reviewed.State == "error" || reviewed.Progress.NeedsAttention || !reviewed.SyncNotices.Reviewed {
		t.Fatalf("reviewed collection exclusions still require attention: %#v", reviewed)
	}
	if !reflect.DeepEqual(before.SyncNotices.Issues, reviewed.SyncNotices.Issues) {
		t.Fatal("reviewing exclusions removed failure details")
	}
}

func TestSyncNoticesSizeOnlyChangesDoNotInvalidateReview(t *testing.T) {
	service, err := New(configuredHome(t), runtime.GOOS)
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	cycle := noticeCycle()
	service.recordCycle(cycle)
	before := noticeSnapshot(t, service)
	if err := service.AcknowledgeSyncNotices(before.SyncNotices.Fingerprint); err != nil {
		t.Fatal(err)
	}
	for i := range cycle.Issues {
		cycle.Issues[i].Bytes += 100
	}
	for i := range cycle.SkippedIssues {
		cycle.SkippedIssues[i].Bytes += 100
	}
	for i := range cycle.BlockedIssues {
		cycle.BlockedIssues[i].Bytes += 100
	}
	cycle.FinishedAt = cycle.FinishedAt.Add(time.Hour)
	service.recordCycle(cycle)
	got := noticeSnapshot(t, service)
	if got.SyncNotices.Fingerprint != before.SyncNotices.Fingerprint || !got.SyncNotices.Reviewed || got.Progress.NeedsAttention {
		t.Fatalf("file growth invalidated acknowledgement: %#v", got)
	}
	if got.SyncNotices.Issues[0].Bytes != before.SyncNotices.Issues[0].Bytes+100 {
		t.Fatal("updated diagnostic size was not retained")
	}
}

func TestSyncNoticesRetainSuccessfulCycleAfterErrorAndCannotReviewError(t *testing.T) {
	service, err := New(configuredHome(t), runtime.GOOS)
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	service.recordCycle(noticeCycle())
	notices := noticeSnapshot(t, service).SyncNotices
	service.recordCycle(daemon.CycleResult{Error: "pull failed", FinishedAt: time.Now()})
	if err := service.AcknowledgeSyncNotices(notices.Fingerprint); err != nil {
		t.Fatal(err)
	}
	got := noticeSnapshot(t, service)
	if got.State != "error" || !got.Progress.NeedsAttention || got.LastError != "pull failed" {
		t.Fatalf("actual error hidden: %#v", got)
	}
	if !reflect.DeepEqual(got.SyncNotices.Issues, notices.Issues) {
		t.Fatal("failed cycle replaced successful-cycle notices")
	}
}

func TestSyncNoticesNeverAcknowledgeOperationalIssuesOrUnknownAttention(t *testing.T) {
	for _, gate := range []string{"restore-failed", "install-failed", "resolution-restore-failed", "scan-failed", "read-failed", "projection-failed", "remote-projection-invalid", "unknown-code", "conflicts", "pending-installs", "unknown-attention"} {
		t.Run(gate, func(t *testing.T) {
			service, err := New(configuredHome(t), runtime.GOOS)
			if err != nil {
				t.Fatal(err)
			}
			defer service.Close()
			cycle := noticeCycle()
			switch gate {
			case "conflicts":
				cycle.Conflicts = 1
			case "pending-installs":
				cycle.PendingInstalls = 1
			case "unknown-attention":
				cycle.Issues, cycle.SkippedIssues, cycle.BlockedIssues = nil, nil, nil
				cycle.Skipped, cycle.Blocked = 0, 0
			default:
				cycle.Issues = append(cycle.Issues, resource.Issue{Code: gate, Message: "failed"})
			}
			service.recordCycle(cycle)
			before := noticeSnapshot(t, service)
			if gate != "unknown-attention" {
				if err := service.AcknowledgeSyncNotices(before.SyncNotices.Fingerprint); err != nil {
					t.Fatal(err)
				}
			}
			got := noticeSnapshot(t, service)
			if got.State != "error" || !got.Progress.NeedsAttention {
				t.Fatalf("%s gate hidden: %#v", gate, got)
			}
		})
	}
}

func TestSyncNoticesLegacyDetailsAreUnavailableNotPreview(t *testing.T) {
	home := configuredHome(t)
	cycle := daemon.CycleResult{Skipped: 9, Blocked: 3, NeedsAttention: true, FinishedAt: time.Now()}
	if err := newSummaryStore(home).saveCycle(cycle); err != nil {
		t.Fatal(err)
	}
	writeDesktopPreview(t, home, ResourcePreview{Issues: []ResourceIssue{{Path: "invented"}}})
	service, err := New(home, runtime.GOOS)
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	got := noticeSnapshot(t, service).SyncNotices
	if got.DetailsAvailable || len(got.Issues) != 0 || got.Skipped != 9 || got.Blocked != 3 {
		t.Fatalf("legacy details = %#v", got)
	}
	if err := service.AcknowledgeSyncNotices(got.Fingerprint); err == nil {
		t.Fatal("legacy issues acknowledged without details")
	}
}

func TestSyncNoticeReviewPersistenceAndLoadFailuresAreExplicit(t *testing.T) {
	service, err := New(configuredHome(t), runtime.GOOS)
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	service.recordCycle(noticeCycle())
	path := filepath.Join(service.home, "desktop", "notice-review.json")
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := service.AcknowledgeSyncNotices(noticeSnapshot(t, service).SyncNotices.Fingerprint); err == nil {
		t.Fatal("persistence failure hidden")
	}
	if noticeSnapshot(t, service).SyncNotices.Reviewed {
		t.Fatal("failed write published reviewed state")
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	for _, data := range []string{"{", "{}", `{"version":99,"fingerprint":"unknown"}`} {
		if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
		restarted, err := New(service.home, runtime.GOOS)
		if restarted != nil {
			restarted.Close()
		}
		if err == nil {
			t.Fatal("invalid acknowledgement loaded silently")
		}
	}
}

func TestSyncNoticesLiveGatesRemainAfterReviewAndDuringProgress(t *testing.T) {
	for _, gate := range []string{"pending-plan", "failed-plan", "unresolved-conflict", "failed-resolution", "sync-error"} {
		t.Run(gate, func(t *testing.T) {
			service, err := New(configuredHome(t), runtime.GOOS)
			if err != nil {
				t.Fatal(err)
			}
			defer service.Close()
			service.recordCycle(noticeCycle())
			if err := service.AcknowledgeSyncNotices(noticeSnapshot(t, service).SyncNotices.Fingerprint); err != nil {
				t.Fatal(err)
			}
			switch gate {
			case "pending-plan", "failed-plan":
				operation := installplan.Operation{ID: "synthetic"}
				plan := installplan.Plan{ID: "synthetic-plan", Operations: []installplan.Operation{operation}}
				if gate == "failed-plan" {
					plan.Errors = map[string]string{operation.ID: "installer failed"}
				}
				if err := installplan.NewStore(filepath.Join(service.home, "install")).SavePending(plan); err != nil {
					t.Fatal(err)
				}
			case "unresolved-conflict":
				store := conflict.NewStore(filepath.Join(service.home, "conflicts"), filepath.Join(service.home, "repo"), nil)
				if err := store.Create(conflict.Record{
					ID: "synthetic", ResourceKey: "demo/source", RepoRel: "agents/demo/config/settings.json",
				}, []byte("base"), []byte("local"), []byte("remote")); err != nil {
					t.Fatal(err)
				}
			case "failed-resolution":
				path := filepath.Join(service.home, "conflict-resolution", "pending.json")
				if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, []byte(`{"id":"synthetic","status":"failed","error":"transaction failed","selections":[]}`), 0o600); err != nil {
					t.Fatal(err)
				}
			case "sync-error":
				service.recordCycle(daemon.CycleResult{Error: "synthetic pull failure", FinishedAt: time.Now()})
			}
			got := noticeSnapshot(t, service)
			if !got.SyncNotices.Reviewed || got.State != "error" || !got.Progress.NeedsAttention {
				t.Fatalf("live %s hidden after review: %#v", gate, got)
			}
			service.recordProgress(syncengine.Progress{Stage: syncengine.StageApplying, Percentage: 60})
			if !noticeSnapshot(t, service).Progress.NeedsAttention {
				t.Fatalf("active %s hidden during progress", gate)
			}
			service.Daemon().Scheduler.Pause()
			if got := noticeSnapshot(t, service); got.State != "paused" || !got.Progress.NeedsAttention {
				t.Fatalf("review hid pause or %s gate: %#v", gate, got)
			}
		})
	}
}

func TestSyncNoticeFingerprintChangesForIssueIdentity(t *testing.T) {
	base := cycleNotices(noticeCycle(), noticeReview{})
	for _, change := range []string{"resource", "path", "code", "message", "count", "duplicate", "version"} {
		t.Run(change, func(t *testing.T) {
			cycle := noticeCycle()
			switch change {
			case "resource":
				cycle.SkippedIssues[0].ResourceKey = "other/source"
			case "path":
				cycle.SkippedIssues[0].Path = "new.tmp"
			case "code":
				cycle.SkippedIssues[0].Code = "file-too-large"
			case "message":
				cycle.SkippedIssues[0].Message = "New reason"
			case "count":
				cycle.Blocked++
			case "duplicate":
				cycle.SkippedIssues = cycle.SkippedIssues[:1]
				cycle.Skipped--
				cycle.Issues = cycle.Issues[:2]
			case "version":
				cycle.IssueDetailsVersion++
			}
			if change != "count" && change != "duplicate" && change != "version" {
				cycle.Issues[1] = cycle.SkippedIssues[0]
			}
			if got := cycleNotices(cycle, noticeReview{}); got.Fingerprint == base.Fingerprint {
				t.Fatal("changed exact issue set retained fingerprint")
			}
		})
	}
}

func TestSyncNoticesLoadFailureDoesNotDiscardSuccessfulHistory(t *testing.T) {
	home := configuredHome(t)
	service, err := New(home, runtime.GOOS)
	if err != nil {
		t.Fatal(err)
	}
	service.recordCycle(noticeCycle())
	service.recordCycle(daemon.CycleResult{Error: "synthetic failure", FinishedAt: time.Now()})
	service.Close()
	restarted, err := New(home, runtime.GOOS)
	if err != nil {
		t.Fatal(err)
	}
	got := noticeSnapshot(t, restarted)
	restarted.Close()
	if len(got.SyncNotices.Issues) != 3 || got.LastError != "synthetic failure" {
		t.Fatalf("history lost after failed-cycle restart: %#v", got)
	}
	path := filepath.Join(home, "desktop", "successful-cycle.json")
	if err := os.WriteFile(path, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	restarted, err = New(home, runtime.GOOS)
	if restarted != nil {
		restarted.Close()
	}
	if err == nil || !strings.Contains(err.Error(), "successful desktop cycle") {
		t.Fatalf("successful-cycle load error hidden: %v", err)
	}
}

func TestResetRemovesSyncNoticeReview(t *testing.T) {
	service := resetFixture(t)
	service.recordCycle(noticeCycle())
	if err := service.AcknowledgeSyncNotices(noticeSnapshot(t, service).SyncNotices.Fingerprint); err != nil {
		t.Fatal(err)
	}

	if err := service.ResetLocalSetup(context.Background(), "RESET", filepath.Join(service.home, "repo")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(service.home, "desktop", "notice-review.json")); !os.IsNotExist(err) {
		t.Fatalf("review remains after reset: %v", err)
	}
	if service.noticeReview.Fingerprint != "" {
		t.Fatal("reset retained in-memory review")
	}
}

func TestSyncNoticesStateEventsPreserveUpdatingAndDoneAfterReview(t *testing.T) {
	service, err := New(configuredHome(t), runtime.GOOS)
	if err != nil {
		t.Fatal(err)
	}
	defer service.Close()
	service.recordCycle(noticeCycle())
	if err := service.AcknowledgeSyncNotices(noticeSnapshot(t, service).SyncNotices.Fingerprint); err != nil {
		t.Fatal(err)
	}
	events := make(chan Snapshot, 4)
	unsubscribe := service.SubscribeState(func(scheduler.State) {
		events <- noticeSnapshot(t, service)
	})
	defer unsubscribe()
	d := service.Daemon()
	d.Scheduler.Job = func() error {
		cycle := noticeCycle()
		cycle.FinishedAt = cycle.FinishedAt.Add(time.Hour)
		service.recordCycle(cycle)
		service.recordProgress(syncengine.Progress{Stage: syncengine.StageComplete, NeedsAttention: true})
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { d.Scheduler.Run(ctx); close(done) }()
	defer func() { cancel(); <-done }()
	d.Scheduler.Trigger()
	for _, state := range []string{"updating", "done"} {
		select {
		case event := <-events:
			if event.State != state || !event.SyncNotices.Reviewed || event.Progress.NeedsAttention {
				t.Fatalf("state event = %#v, want %s with reviewed notices", event, state)
			}
		case <-time.After(3 * time.Second):
			t.Fatalf("missing %s state event", state)
		}
	}
}
