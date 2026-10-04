package desktop

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"

	"github.com/qinqingxu/synchub-for-agents/internal/daemon"
	"github.com/qinqingxu/synchub-for-agents/internal/resource"
)

const noticeReviewVersion = 1

type noticeReview struct {
	Version     int    `json:"version"`
	Fingerprint string `json:"fingerprint"`
}

func (s *summaryStore) loadNoticeReview() (noticeReview, error) {
	var review noticeReview
	exists, err := s.loadExisting("notice-review.json", &review)
	if err != nil {
		return review, err
	}
	if !exists {
		return review, nil
	}
	digest, err := hex.DecodeString(review.Fingerprint)
	if review.Version != noticeReviewVersion || err != nil || len(digest) != sha256.Size {
		return noticeReview{}, errors.New("invalid or unsupported sync notice acknowledgement")
	}
	return review, nil
}

func (s *summaryStore) loadSuccessfulCycle(last daemon.CycleResult) (daemon.CycleResult, error) {
	var cycle daemon.CycleResult
	exists, err := s.loadExisting("successful-cycle.json", &cycle)
	if err != nil {
		return cycle, err
	}
	if !exists && last.Error == "" {
		cycle = last
	}
	return cycle, nil
}

// Collection exclusions can be reviewed without allowing the excluded files.
// Operational failures outside the skipped/blocked groups remain actionable.
func reviewableNotice(code string) bool {
	switch code {
	case "secret-detected", "remote-secret-detected", "generated-content",
		"platform-binary", "file-too-large", "invalid-path", "repo-path-invalid",
		"unapproved-link", "link-cycle", "remote-path-invalid",
		"remote-internal-invalid", "remote-resource-unknown", "remote-link-blocked",
		"remote-nonportable-fields", "remote-install-manifest-invalid",
		"root-unavailable", "link-unavailable", "projection-failed", "remote-projection-invalid":
		return true
	default:
		return false
	}
}

func cycleNotices(cycle daemon.CycleResult, review noticeReview) SyncNotices {
	notices := SyncNotices{
		Version: cycle.IssueDetailsVersion, FinishedAt: cycle.FinishedAt,
		Skipped: cycle.Skipped, Blocked: cycle.Blocked,
		DetailsAvailable: cycle.IssueDetailsVersion == noticeReviewVersion,
		Issues:           []SyncNoticeIssue{},
	}
	if !notices.DetailsAvailable {
		return notices
	}
	remaining := make(map[resource.Issue]int)
	for _, issue := range cycle.Issues {
		remaining[issue]++
	}
	appendIssues := func(issues []resource.Issue, kind string) {
		for _, issue := range issues {
			notices.Issues = append(notices.Issues, SyncNoticeIssue{
				ResourceKey: issue.ResourceKey, Path: issue.Path, Code: issue.Code,
				Message: issue.Message, Bytes: issue.Bytes, Kind: kind,
				Reviewable: kind != "error" && reviewableNotice(issue.Code),
			})
			remaining[issue]--
		}
	}
	appendIssues(cycle.SkippedIssues, "skipped")
	appendIssues(cycle.BlockedIssues, "blocked")
	for _, issue := range cycle.Issues {
		if remaining[issue] > 0 {
			appendIssues([]resource.Issue{issue}, "error")
		}
	}
	// Sort only the fingerprint input: the log retains every original record,
	// including duplicates. A later cycle's timestamp/order do not invalidate review.
	records := make([]string, 0, len(notices.Issues))
	for _, issue := range notices.Issues {
		// File growth does not create a new exclusion; retain sizes only in the log.
		issue.Bytes = 0
		data, _ := json.Marshal(issue) // Concrete fields cannot fail JSON encoding.
		records = append(records, string(data))
	}
	sort.Strings(records)
	data, _ := json.Marshal(struct {
		Version int
		Skipped int
		Blocked int
		Records []string
	}{notices.Version, notices.Skipped, notices.Blocked, records})
	digest := sha256.Sum256(data)
	notices.Fingerprint = hex.EncodeToString(digest[:])
	notices.Reviewed = review.Version == noticeReviewVersion && review.Fingerprint == notices.Fingerprint
	return notices
}

func cycleNeedsAttention(cycle daemon.CycleResult, review noticeReview) bool {
	if cycle.Error != "" || cycle.Conflicts > 0 || cycle.PendingInstalls > 0 {
		return true
	}
	notices := cycleNotices(cycle, review)
	if !notices.DetailsAvailable {
		return cycle.NeedsAttention || cycle.Blocked > 0 || cycle.Skipped > 0
	}
	for _, issue := range notices.Issues {
		if !issue.Reviewable || !notices.Reviewed {
			return true
		}
	}
	if len(notices.Issues) == 0 {
		return cycle.NeedsAttention || cycle.Blocked > 0 || cycle.Skipped > 0
	}
	return false
}

func cycleProgress(cycle daemon.CycleResult) Progress {
	if cycle.FinishedAt.IsZero() {
		return Progress{}
	}
	return Progress{
		Stage: "complete", Label: "Synchronization complete", Percentage: 100,
		CompletedActions: cycle.Actions, TotalActions: cycle.Actions,
		BlockedFiles: cycle.Blocked, Skipped: cycle.Skipped,
		Pushed: cycle.Pushed, Restored: cycle.Restored, Reinstalled: cycle.Reinstalled,
		Conflicts: cycle.Conflicts, PendingInstalls: cycle.PendingInstalls,
		NeedsAttention: cycle.NeedsAttention || cycle.Error != "",
	}
}

func (s *Service) AcknowledgeSyncNotices(fingerprint string) error {
	s.operations.Lock()
	defer s.operations.Unlock()
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.daemon == nil {
		return ErrNotConfigured
	}
	notices := cycleNotices(s.successfulCycle, s.noticeReview)
	if !notices.DetailsAvailable {
		return errors.New("historical cycle details are unavailable; cannot acknowledge unseen notices")
	}
	if fingerprint == "" || fingerprint != notices.Fingerprint {
		return errors.New("sync notice set changed; close and reopen the log before acknowledging")
	}
	reviewable := false
	for _, issue := range notices.Issues {
		reviewable = reviewable || issue.Reviewable
	}
	if !reviewable {
		return errors.New("this cycle has no safety or exclusion notices to acknowledge")
	}
	review := noticeReview{Version: noticeReviewVersion, Fingerprint: fingerprint}
	if err := newSummaryStore(s.home).save("notice-review.json", review); err != nil {
		return fmt.Errorf("save sync notice acknowledgement: %w", err)
	}
	s.noticeReview = review
	return nil
}

func terminalAttention(state string, attention bool) string {
	if attention && (state == "idle" || state == "done") {
		return "error"
	}
	return state
}
