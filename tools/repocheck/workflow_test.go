package repocheck

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

type workflow struct {
	On          map[string]interface{} `yaml:"on"`
	Permissions map[string]string      `yaml:"permissions"`
	Jobs        map[string]job         `yaml:"jobs"`
}

type job struct {
	Name            string                 `yaml:"name"`
	If              string                 `yaml:"if"`
	Uses            string                 `yaml:"uses"`
	Timeout         int                    `yaml:"timeout-minutes"`
	ContinueOnError bool                   `yaml:"continue-on-error"`
	With            map[string]interface{} `yaml:"with"`
	Steps           []step                 `yaml:"steps"`
}

type step struct {
	ID              string            `yaml:"id"`
	Run             string            `yaml:"run"`
	Uses            string            `yaml:"uses"`
	If              string            `yaml:"if"`
	ContinueOnError bool              `yaml:"continue-on-error"`
	With            map[string]string `yaml:"with"`
}

func loadWorkflow(t *testing.T, name string) workflow {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", name))
	if err != nil {
		t.Fatal(err)
	}
	var result workflow
	if err := yaml.Unmarshal(data, &result); err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return result
}

func TestRepositoryGatesAreUnconditionalForPullRequests(t *testing.T) {
	ci := loadWorkflow(t, "ci.yml")
	trigger, exists := ci.On["pull_request"]
	if !exists || trigger != nil {
		t.Fatal("PR checks must run without branch/path filters")
	}
	if ci.Permissions["contents"] != "read" || len(ci.Permissions) != 1 {
		t.Fatal("repository checks must have read-only permissions")
	}
	for id, expected := range map[string][]string{
		"repository": {"node scripts/dev.mjs setup", "node scripts/dev.mjs verify"},
		"security":   {"node scripts/dev.mjs setup", "npm --prefix frontend audit --audit-level=high", "go run golang.org/x/vuln/cmd/govulncheck@v1.1.4 ./..."},
	} {
		current, ok := ci.Jobs[id]
		if !ok || current.If != "" || current.ContinueOnError || current.Timeout <= 0 {
			t.Fatalf("%s must be unconditional, fail-closed, and time-bounded", id)
		}
		next := 0
		for _, s := range current.Steps {
			artifactUpload := strings.HasPrefix(s.Uses, "actions/upload-artifact@") &&
				(s.If == "always() && steps.validation.outputs.report_directory != ''" ||
					s.If == "always() && steps.proposal.outputs.report_directory != ''")
			proposal := s.Run == "node scripts/dev.mjs propose" &&
				s.If == "failure() && steps.validation.outcome == 'failure'"
			if (s.If != "" && !artifactUpload && !proposal) || s.ContinueOnError {
				t.Fatalf("%s must not skip or swallow a check", id)
			}
			if next < len(expected) && s.Run == expected[next] {
				next++
			}
		}
		if next != len(expected) {
			t.Fatalf("%s must run the shared setup/check sequence: %v", id, expected)
		}
	}
}

func TestMaintenanceReusesChecksWithoutPackagingOrWritePermissions(t *testing.T) {
	maintenance := loadWorkflow(t, "maintenance.yml")
	if _, ok := maintenance.On["schedule"]; !ok {
		t.Fatal("maintenance must have a concrete schedule")
	}
	if _, ok := maintenance.On["workflow_dispatch"]; !ok {
		t.Fatal("maintenance must support a manual audit")
	}
	if maintenance.Permissions["contents"] != "read" || len(maintenance.Permissions) != 1 {
		t.Fatal("maintenance must not have write permissions")
	}
	audit := maintenance.Jobs["audit"]
	if len(maintenance.Jobs) != 1 || audit.Uses != "./.github/workflows/ci.yml" || audit.With["maintenance"] != true {
		t.Fatal("maintenance must reuse CI in maintenance mode")
	}
	ci := loadWorkflow(t, "ci.yml")
	if _, ok := ci.On["workflow_call"]; !ok {
		t.Fatal("CI must remain reusable")
	}
	for _, id := range []string{"test", "package"} {
		if ci.Jobs[id].If != "${{ !inputs.maintenance }}" {
			t.Fatalf("maintenance must not launch the %s matrix", id)
		}
	}
}

func TestWorkflowsUsePinnedNodeVersion(t *testing.T) {
	for _, name := range []string{"ci.yml", "release.yml"} {
		w := loadWorkflow(t, name)
		for id, j := range w.Jobs {
			for _, s := range j.Steps {
				if strings.HasPrefix(s.Uses, "actions/setup-node@") &&
					(s.With["node-version-file"] != ".node-version" || s.With["node-version"] != "") {
					t.Fatalf("%s/%s must use .node-version", name, id)
				}
			}
		}
	}
}

func TestValidationReportsRemainAvailableAfterFailures(t *testing.T) {
	ci := loadWorkflow(t, "ci.yml")
	foundRunner, foundUpload := false, false
	for _, s := range ci.Jobs["repository"].Steps {
		if s.Run == "node scripts/dev.mjs verify" {
			if s.ID != "validation" || s.If != "" || s.ContinueOnError {
				t.Fatal("validation must fail the job rather than being skipped or ignored")
			}
			foundRunner = true
		}
		if strings.HasPrefix(s.Uses, "actions/upload-artifact@") && s.With["name"] == "repository-validation" {
			if s.If != "always() && steps.validation.outputs.report_directory != ''" ||
				s.With["path"] != "${{ steps.validation.outputs.report_directory }}" ||
				s.With["retention-days"] != "14" || s.With["if-no-files-found"] != "error" {
				t.Fatal("validation must upload only the safely created report directory, including on failure")
			}
			foundUpload = true
		}
	}
	if !foundRunner || !foundUpload {
		t.Fatal("CI must execute validation and publish its real report artifacts")
	}
}

func TestMaintenanceProposalIsBoundedToValidationFailures(t *testing.T) {
	ci := loadWorkflow(t, "ci.yml")
	foundProposal, foundUpload := false, false
	for _, s := range ci.Jobs["repository"].Steps {
		if s.Run == "node scripts/dev.mjs propose" {
			if s.ID != "proposal" || s.If != "failure() && steps.validation.outcome == 'failure'" || s.ContinueOnError {
				t.Fatal("the proposal must run only after failed validation and must not mask errors")
			}
			foundProposal = true
		}
		if strings.HasPrefix(s.Uses, "actions/upload-artifact@") && s.With["name"] == "maintenance-proposal" {
			if s.If != "always() && steps.proposal.outputs.report_directory != ''" ||
				s.With["path"] != "${{ steps.proposal.outputs.report_directory }}" ||
				s.With["retention-days"] != "14" || s.With["if-no-files-found"] != "error" {
				t.Fatal("proposal artifacts must use the guarded output directory with bounded retention")
			}
			foundUpload = true
		}
	}
	if !foundProposal || !foundUpload {
		t.Fatal("CI must offer a review-only repair proposal and retain its evidence")
	}
}
