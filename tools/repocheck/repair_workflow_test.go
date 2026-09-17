package repocheck

import (
	"strings"
	"testing"
)

func TestRepairVerificationRunsInReadOnlyBoundedCI(t *testing.T) {
	w := loadWorkflow(t, "repair-verification.yml")
	if trigger, ok := w.On["pull_request"]; !ok || trigger != nil {
		t.Fatal("repair verification must run for every PR")
	}
	if len(w.Permissions) != 1 || w.Permissions["contents"] != "read" {
		t.Fatal("repair verification must not have repository write permissions")
	}
	j, ok := w.Jobs["repair"]
	if !ok || j.Timeout != 45 || j.If != "" || j.ContinueOnError {
		t.Fatal("repair verification must have a bounded, fail-closed job")
	}
	run, upload := false, false
	for _, s := range j.Steps {
		if s.Run == "node scripts/dev.mjs repair:verify" {
			if s.ID != "verify" || s.If != "" || s.ContinueOnError {
				t.Fatal("the native repair verifier must not be skipped or ignored")
			}
			run = true
		}
		if strings.HasPrefix(s.Uses, "actions/upload-artifact@") {
			if s.If != "always() && steps.verify.outputs.report_directory != ''" ||
				s.With["path"] != "${{ steps.verify.outputs.report_directory }}" ||
				s.With["retention-days"] != "14" || s.With["if-no-files-found"] != "error" {
				t.Fatal("repair proof artifacts must be guarded, retained, and fail closed")
			}
			upload = true
		}
	}
	if !run || !upload {
		t.Fatal("repair verification must execute and publish its actual evidence")
	}
}
