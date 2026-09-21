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
	Permissions     map[string]string      `yaml:"permissions"`
	Timeout         int                    `yaml:"timeout-minutes"`
	ContinueOnError bool                   `yaml:"continue-on-error"`
	With            map[string]interface{} `yaml:"with"`
	Steps           []step                 `yaml:"steps"`
}

type step struct {
	Name            string            `yaml:"name"`
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
		"test": {
			"node scripts/dev.mjs setup",
			"go test -race ./...",
			"go vet ./...",
			"npm --prefix frontend run lint",
			"npm --prefix frontend run typecheck",
			"npm --prefix frontend run test:lint",
			"npm --prefix frontend test",
		},
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
	if ci.Jobs["package"].If != "${{ !inputs.maintenance }}" {
		t.Fatal("maintenance must not launch the packaging matrix")
	}
	if ci.Jobs["test"].If != "" {
		t.Fatal("maintenance must run the Linux race and frontend checks")
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

func TestEvidenceConfigurationFilesAreCommitted(t *testing.T) {
	for _, relative := range []string{
		".env.example",
		".github/labels.yml",
		".github/copilot-instructions.md",
		".github/workflows/copilot-setup-steps.yml",
		".agents/skills/synchub-validation/SKILL.md",
		"CODEOWNERS",
		".github/ISSUE_TEMPLATE/config.yml",
		"docs/specs/validation-receipt.v1.schema.json",
		"docs/specs/repair-proof.v1.schema.json",
		"docs/specs/README.md",
		"docs/specs/agentic-validation.v1.md",
		"docs/adr/0001-validation-evidence.md",
		"docs/operations/agentic-observability.md",
		"docs/reports/agentic-validation-reports.md",
		"docs/dashboards/agentic-readiness-dashboard.json",
		"docs/runbooks/ci-failure-response.md",
		"Makefile",
		"package.json",
		".vscode/mcp.json",
		"tools/mcp/validation-server.mjs",
		"tools/mcp/synchub-mcp-server.mjs",
		"mcp/synchub-validation/server.mjs",
		".mcp.json",
		".github/workflows/auto-revert.yml",
		".github/workflows/pr-validation.yml",
		".pre-commit-config.yaml",
	} {
		if _, err := os.Stat(filepath.Join("..", "..", relative)); err != nil {
			t.Fatalf("%s must exist: %v", relative, err)
		}
	}
}

func TestCodeownersCoverBothMaintainerAccounts(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "CODEOWNERS"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, required := range []string{"* ", "@qinqingxu", "@jelllove"} {
		if !strings.Contains(text, required) {
			t.Fatalf("CODEOWNERS must include %q", required)
		}
	}
}

func TestRootValidationEntrypointsAreDiscoverable(t *testing.T) {
	for _, filename := range []string{"package.json", "Makefile"} {
		data, err := os.ReadFile(filepath.Join("..", "..", filename))
		if err != nil {
			t.Fatal(err)
		}
		text := string(data)
		for _, required := range []string{
			"node scripts/dev.mjs setup",
			"node scripts/dev.mjs check",
			"node scripts/dev.mjs verify",
			"node scripts/dev.mjs docs",
			"node scripts/dev.mjs repair:verify",
			"node scripts/dev.mjs rollback:verify",
		} {
			if !strings.Contains(text, required) {
				t.Fatalf("%s must expose repository validation command %q", filename, required)
			}
		}
	}
}

func TestRepositoryValidationTasksAreDiscoverable(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "Taskfile.yml"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, required := range []string{
		"repo:setup:",
		"node scripts/dev.mjs setup",
		"repo:check:",
		"node scripts/dev.mjs check",
		"repo:verify:",
		"node scripts/dev.mjs verify",
		"repo:docs:",
		"node scripts/dev.mjs docs",
		"repo:repair:verify:",
		"node scripts/dev.mjs repair:verify",
		"repo:rollback:verify:",
		"node scripts/dev.mjs rollback:verify",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("Taskfile.yml must expose repository validation command %q", required)
		}
	}
}

func TestCopilotInstructionsAreCommitted(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", ".github", "copilot-instructions.md"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, required := range []string{
		"node scripts/dev.mjs setup",
		"node scripts/dev.mjs check",
		"node scripts/dev.mjs verify",
		"node scripts/dev.mjs docs",
		"Do not run application sync",
		"Do not create releases or tags",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("Copilot instructions must contain %q", required)
		}
	}
}

func TestCopilotSetupStepsAreBoundedAndDeterministic(t *testing.T) {
	workflow := loadWorkflow(t, "copilot-setup-steps.yml")
	job := workflow.Jobs["copilot-setup-steps"]
	if len(workflow.Jobs) != 1 || job.Timeout <= 0 || job.Timeout > 59 || job.ContinueOnError {
		t.Fatal("Copilot setup steps must have one bounded fail-closed job")
	}
	if job.Permissions["contents"] != "read" || len(job.Permissions) != 1 {
		t.Fatal("Copilot setup steps must use read-only contents permission")
	}
	joined := ""
	for _, s := range job.Steps {
		if s.ContinueOnError {
			t.Fatal("Copilot setup steps must not ignore failures")
		}
		joined += "\n" + s.Uses + "\n" + s.Run + "\n"
		if strings.HasPrefix(s.Uses, "actions/setup-go@") && s.With["go-version-file"] != "go.mod" {
			t.Fatal("Copilot setup must read Go version from go.mod")
		}
		if strings.HasPrefix(s.Uses, "actions/setup-node@") && s.With["node-version-file"] != ".node-version" {
			t.Fatal("Copilot setup must read Node version from .node-version")
		}
	}
	for _, required := range []string{
		"actions/checkout@11bd71901bbe5b1630ceea73d27597364c9af683",
		"actions/setup-go@d35c59abb061a4a6fb18e82ac0862c26744d6ab5",
		"actions/setup-node@49933ea5288caeca8642d1e84afbd3f7d6820020",
		"libgtk-4-dev libwebkitgtk-6.0-dev",
		"node scripts/dev.mjs setup",
	} {
		if !strings.Contains(joined, required) {
			t.Fatalf("Copilot setup steps must contain %q", required)
		}
	}
}

func TestDocumentationDriftGateIsDedicatedAndFailClosed(t *testing.T) {
	ci := loadWorkflow(t, "ci.yml")
	job := ci.Jobs["documentation-drift"]
	if job.Name != "Documentation drift" || job.If != "" || job.Timeout <= 0 || job.ContinueOnError {
		t.Fatal("Documentation drift must be a dedicated unconditional fail-closed job")
	}
	if ci.Permissions["contents"] != "read" || len(ci.Permissions) != 1 {
		t.Fatal("Documentation drift must inherit read-only workflow permissions")
	}
	joined := ""
	for _, s := range job.Steps {
		if s.ContinueOnError || s.If != "" {
			t.Fatal("Documentation drift steps must not be skipped or ignored")
		}
		joined += "\n" + s.Uses + "\n" + s.Run + "\n"
	}
	for _, required := range []string{
		"actions/checkout@11bd71901bbe5b1630ceea73d27597364c9af683",
		"actions/setup-go@d35c59abb061a4a6fb18e82ac0862c26744d6ab5",
		"actions/setup-node@49933ea5288caeca8642d1e84afbd3f7d6820020",
		"node scripts/dev.mjs setup",
		"node scripts/dev.mjs docs",
	} {
		if !strings.Contains(joined, required) {
			t.Fatalf("Documentation drift job must contain %q", required)
		}
	}
}

func TestEvidenceArtifactsRemainDocumentedAndPublished(t *testing.T) {
	ciData, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", "ci.yml"))
	if err != nil {
		t.Fatal(err)
	}
	repairData, err := os.ReadFile(filepath.Join("..", "..", ".github", "workflows", "repair-verification.yml"))
	if err != nil {
		t.Fatal(err)
	}
	guide, err := os.ReadFile(filepath.Join("..", "..", "docs", "operations", "agentic-observability.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"repository-validation", "maintenance-proposal"} {
		if !strings.Contains(string(ciData), required) || !strings.Contains(string(guide), required) {
			t.Fatalf("%s must be published by CI and documented", required)
		}
	}
	if !strings.Contains(string(repairData), "repair-verification") || !strings.Contains(string(guide), "repair-verification") {
		t.Fatal("repair proof must be published and documented")
	}
}

func TestStaticAnalysisAndPreCommitContracts(t *testing.T) {
	preCommit, err := os.ReadFile(filepath.Join("..", "..", ".pre-commit-config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"repo: local", "node scripts/dev.mjs check", "node scripts/dev.mjs docs", "pass_filenames: false"} {
		if !strings.Contains(string(preCommit), required) {
			t.Fatalf(".pre-commit-config.yaml must contain %q", required)
		}
	}
	codeql := loadWorkflow(t, "codeql.yml")
	if _, ok := codeql.On["pull_request"]; !ok {
		t.Fatal("CodeQL must run on pull requests")
	}
	if codeql.Permissions["contents"] != "read" || codeql.Permissions["security-events"] != "write" || len(codeql.Permissions) != 2 {
		t.Fatal("CodeQL must use only read contents and write security-events permissions")
	}
	analyze := codeql.Jobs["analyze"]
	if analyze.Timeout <= 0 || analyze.ContinueOnError {
		t.Fatal("CodeQL analysis must be time-bounded and fail closed")
	}
	joined := ""
	languages := false
	for _, s := range analyze.Steps {
		if s.ContinueOnError {
			t.Fatal("CodeQL steps must not ignore failures")
		}
		joined += "\n" + s.Uses + "\n" + s.Run + "\n"
		if s.With["languages"] == "javascript-typescript" {
			languages = true
		}
	}
	for _, required := range []string{
		"github/codeql-action/init@b96794f015dfd88f77b49b1c93e0fa7110f94c63",
		"github/codeql-action/analyze@b96794f015dfd88f77b49b1c93e0fa7110f94c63",
	} {
		if !strings.Contains(joined, required) {
			t.Fatalf("CodeQL workflow must contain %q", required)
		}
	}
	if !languages {
		t.Fatal("CodeQL workflow must analyze JavaScript/TypeScript")
	}
}

func TestCopilotAgentReviewWorkflowIsReadOnlyAndFailClosed(t *testing.T) {
	workflow := loadWorkflow(t, "copilot-agent-review.yml")
	if _, ok := workflow.On["pull_request"]; !ok {
		t.Fatal("Copilot agent review must run on pull requests")
	}
	if _, ok := workflow.On["workflow_dispatch"]; !ok {
		t.Fatal("Copilot agent review must support manual dispatch")
	}
	if workflow.Permissions["contents"] != "read" ||
		workflow.Permissions["pull-requests"] != "read" ||
		workflow.Permissions["checks"] != "read" ||
		workflow.Permissions["copilot-requests"] != "write" ||
		len(workflow.Permissions) != 4 {
		t.Fatal("Copilot agent review must use only read permissions plus copilot-requests write")
	}
	review := workflow.Jobs["review"]
	if review.Name != "Copilot agent review" || review.Timeout <= 0 || review.ContinueOnError {
		t.Fatal("Copilot agent review job must be named for the required status, time-bounded, and fail closed")
	}
	joined := ""
	foundCopilot, foundPrompt, foundArtifact := false, false, false
	for _, s := range review.Steps {
		if s.ContinueOnError {
			t.Fatal("Copilot agent review steps must not hide failures")
		}
		joined += "\n" + s.Run + "\n" + s.Uses + "\n"
		if strings.Contains(s.Run, "npm install --global @github/copilot@1.0.84") {
			foundCopilot = true
		}
		if strings.Contains(s.Run, "You are reviewing SyncHub for Agents") &&
			strings.Contains(s.Run, "Do not modify files") {
			foundPrompt = true
		}
		if strings.HasPrefix(s.Uses, "actions/upload-artifact@") &&
			s.With["name"] == "copilot-agent-review" &&
			s.With["if-no-files-found"] == "error" {
			foundArtifact = true
		}
	}
	for _, forbidden := range []string{"contents: write", "pull-requests: write", "gh issue create", "gh pr create", "git push"} {
		if strings.Contains(joined, forbidden) {
			t.Fatalf("Copilot agent review must not mutate repository state with %q", forbidden)
		}
	}
	if !foundCopilot || !foundPrompt || !foundArtifact {
		t.Fatal("Copilot agent review must run a pinned Copilot CLI prompt and publish its report artifact")
	}
}

func TestRecurringCopilotReviewWorkflowIsStaticallyDiscoverable(t *testing.T) {
	workflow := loadWorkflow(t, "recurring-copilot-review.yml")
	if _, ok := workflow.On["pull_request"]; !ok {
		t.Fatal("recurring Copilot review must run on pull requests")
	}
	if workflow.Permissions["contents"] != "read" || workflow.Permissions["copilot-requests"] != "write" || len(workflow.Permissions) != 2 {
		t.Fatal("recurring Copilot review must use read permissions plus copilot-requests write")
	}
	review := workflow.Jobs["review"]
	if review.Name != "Recurring Copilot review" || review.Timeout <= 0 || review.ContinueOnError {
		t.Fatal("recurring Copilot review must be named, time-bounded, and fail closed")
	}
	usesLocalAction := false
	for _, s := range review.Steps {
		if s.ContinueOnError {
			t.Fatal("recurring Copilot review must not hide step failures")
		}
		if s.Uses == "./.github/actions/recurring-copilot-review" {
			usesLocalAction = true
		}
	}
	if !usesLocalAction {
		t.Fatal("recurring Copilot review must call the local static review action")
	}
	action, err := os.ReadFile(filepath.Join("..", "..", ".github", "actions", "recurring-copilot-review", "action.yml"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(action)
	for _, required := range []string{
		"npm install --global @github/copilot@1.0.84",
		`copilot -p "Review pull request changed files and publish review output with scoped guards"`,
		"using: composite",
	} {
		if !strings.Contains(text, required) {
			t.Fatalf("recurring Copilot review action must contain %q", required)
		}
	}
}

func TestAgenticLearningLifecycleIsDocumented(t *testing.T) {
	for _, relative := range []string{
		"docs/operations/agentic-learning-lifecycle.md",
		"docs/reports/agentic-review-findings.md",
	} {
		if _, err := os.Stat(filepath.Join("..", "..", relative)); err != nil {
			t.Fatalf("%s must exist: %v", relative, err)
		}
	}
	lifecycle, err := os.ReadFile(filepath.Join("..", "..", "docs", "operations", "agentic-learning-lifecycle.md"))
	if err != nil {
		t.Fatal(err)
	}
	lifeText := string(lifecycle)
	for _, required := range []string{
		"candidate",
		"active",
		"retired",
		"rejected",
		"Recurring Copilot review",
		"regression validation",
		"agentic-review-findings.md",
	} {
		if !strings.Contains(lifeText, required) {
			t.Fatalf("agentic learning lifecycle must mention %q", required)
		}
	}
	findings, err := os.ReadFile(filepath.Join("..", "..", "docs", "reports", "agentic-review-findings.md"))
	if err != nil {
		t.Fatal(err)
	}
	findingsText := string(findings)
	for _, required := range []string{
		"PR #16",
		"Copilot review finding",
		"follow-up validation",
		"Recurring Copilot review",
	} {
		if !strings.Contains(findingsText, required) {
			t.Fatalf("agentic review findings index must mention %q", required)
		}
	}
}

func TestSelfHealingDiagnosticsWorkflowIsReadOnlyAndReviewOnly(t *testing.T) {
	workflow := loadWorkflow(t, "self-healing.yml")
	if _, ok := workflow.On["workflow_run"]; !ok {
		t.Fatal("self-healing diagnostics must run from workflow_run failure signals")
	}
	if _, ok := workflow.On["workflow_dispatch"]; !ok {
		t.Fatal("self-healing diagnostics must support manual dispatch")
	}
	if workflow.Permissions["contents"] != "read" || workflow.Permissions["actions"] != "read" || len(workflow.Permissions) != 2 {
		t.Fatal("self-healing diagnostics must remain read-only")
	}
	response := workflow.Jobs["response"]
	if response.If == "" || response.Timeout <= 0 || response.ContinueOnError {
		t.Fatal("self-healing diagnostics must be conditional, time-bounded, and fail closed")
	}
	joined := ""
	for _, s := range response.Steps {
		if s.ContinueOnError {
			t.Fatal("self-healing diagnostics must not hide step failures")
		}
		joined += "\n" + s.Name + "\n" + s.Run + "\n" + s.Uses + "\n"
		for _, value := range s.With {
			joined += value + "\n"
		}
	}
	for _, forbidden := range []string{"git push", "gh issue create", "gh pr create", "pull-requests: write", "contents: write"} {
		if strings.Contains(joined, forbidden) {
			t.Fatalf("self-healing diagnostics must not mutate repository state with %q", forbidden)
		}
	}
	if !strings.Contains(joined, "node scripts/dev.mjs propose") ||
		!strings.Contains(joined, "node scripts/dev.mjs rollback:verify") ||
		!strings.Contains(joined, "bounded repair and rollback handoff") ||
		!strings.Contains(joined, "ci-failure-response") ||
		!strings.Contains(joined, "rollback-verification") {
		t.Fatal("self-healing diagnostics must publish the existing bounded repair and rollback handoff")
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

func TestLinuxBuildDependenciesPrecedeWailsInstallation(t *testing.T) {
	ci := loadWorkflow(t, "ci.yml")
	dependencies, wails := -1, -1
	for index, s := range ci.Jobs["package"].Steps {
		if strings.Contains(s.Run, "libgtk-4-dev") && strings.Contains(s.Run, "libwebkitgtk-6.0-dev") {
			if s.If != "runner.os == 'Linux'" {
				t.Fatal("Linux native dependencies must be scoped to the Linux runner")
			}
			dependencies = index
		}
		if strings.Contains(s.Run, "go install github.com/wailsapp/wails/v3/cmd/wails3@") {
			wails = index
		}
	}
	if dependencies < 0 || wails < 0 || dependencies >= wails {
		t.Fatal("GTK/WebKit development libraries must be installed before compiling the Wails CLI")
	}
}

func TestAppImageVerificationUsesNormalizedGeneratorPaths(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "build", "linux", "Taskfile.yml"))
	if err != nil {
		t.Fatal(err)
	}
	var taskfile struct {
		Tasks map[string]struct {
			Vars map[string]yaml.Node `yaml:"vars"`
			Cmds []yaml.Node          `yaml:"cmds"`
		} `yaml:"tasks"`
	}
	if err := yaml.Unmarshal(data, &taskfile); err != nil {
		t.Fatal(err)
	}
	task := taskfile.Tasks["create:appimage"]
	if task.Vars["APPIMAGE_NAME"].Value != "{{.APP_NAME | lower}}" {
		t.Fatal("AppImage paths must match the Wails generator's normalized name")
	}
	var commands []string
	for _, command := range task.Cmds {
		if command.Kind == yaml.ScalarNode {
			commands = append(commands, command.Value)
		}
	}

	script := strings.Join(commands, "\n")
	for _, required := range []string{
		"cmp --silent",
		"{{.APPIMAGE_NAME}}-x86_64.AppDir/AppRun",
		`mv -- "{{.OUTPUT_DIR}}/{{.APPIMAGE_NAME}}-x86_64.AppImage" "{{.OUTPUT_DIR}}/{{.APP_NAME}}-x86_64.AppImage"`,
	} {
		if !strings.Contains(script, required) {
			t.Fatalf("AppImage verification/publication is missing %q", required)
		}
	}
}

func TestLinuxSmokeConsumesPackageListingBeforeSearching(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "scripts", "smoke", "linux.sh"))
	if err != nil {
		t.Fatal(err)
	}
	script := string(data)
	if strings.Contains(script, `dpkg-deb --contents "$deb" |`) {
		t.Fatal("grep -q can close the pipe early and make dpkg-deb fail under pipefail")
	}
	if !strings.Contains(script, `dpkg-deb --contents "$deb" > "$work_dir/deb-contents.txt"`) ||
		!strings.Contains(script, `grep -q 'usr/bin/SyncHub' "$work_dir/deb-contents.txt"`) {
		t.Fatal("the complete package listing must be captured and checked")
	}
}

func TestAutoRevertWorkflowOpensReviewOnlyRevertPR(t *testing.T) {
	workflow := loadWorkflow(t, "auto-revert.yml")
	if _, ok := workflow.On["workflow_run"]; !ok {
		t.Fatal("auto-revert must run from workflow_run failure signals")
	}
	if workflow.Permissions["contents"] != "write" || workflow.Permissions["pull-requests"] != "write" {
		t.Fatal("auto-revert must be able to open a revert pull request")
	}
	job := workflow.Jobs["auto-revert"]
	if job.If == "" || job.Timeout <= 0 || job.ContinueOnError {
		t.Fatal("auto-revert must be conditional, time-bounded, and fail closed")
	}
	joined := ""
	for _, s := range job.Steps {
		if s.ContinueOnError {
			t.Fatal("auto-revert must not hide step failures")
		}
		joined += "\n" + s.Name + "\n" + s.Run + "\n" + s.Uses + "\n"
	}
	for _, required := range []string{"auto-revert", "gh pr create", "revert"} {
		if !strings.Contains(strings.ToLower(joined), required) {
			t.Fatalf("auto-revert must contain %q", required)
		}
	}
	for _, forbidden := range []string{"gh pr merge", "git push --force", "git reset --hard"} {
		if strings.Contains(joined, forbidden) {
			t.Fatalf("auto-revert must not merge or force-reset with %q", forbidden)
		}
	}
}

func TestPosixPrValidationWorkflowExposesGoAndNpmTests(t *testing.T) {
	workflow := loadWorkflow(t, "pr-validation.yml")
	if _, ok := workflow.On["pull_request"]; !ok {
		t.Fatal("PR validation must run on pull requests")
	}
	if workflow.Permissions["contents"] != "read" || len(workflow.Permissions) != 1 {
		t.Fatal("PR validation must remain read-only")
	}
	if len(workflow.Jobs) != 1 {
		t.Fatal("PR validation must be a single POSIX job")
	}
	job := workflow.Jobs["tests"]
	if job.If != "" || job.Timeout <= 0 || job.ContinueOnError {
		t.Fatal("PR validation must be unconditional, time-bounded, and fail closed")
	}
	joined := ""
	for _, s := range job.Steps {
		if s.If != "" || s.ContinueOnError {
			t.Fatal("PR validation steps must not be skipped or ignored")
		}
		joined += "\n" + s.Run + "\n"
	}
	for _, required := range []string{"go test ./...", "npm test"} {
		if !strings.Contains(joined, required) {
			t.Fatalf("PR validation must run %q", required)
		}
	}
}

func TestShippedMcpServerLivesUnderMcpDirectory(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", ".mcp.json"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, required := range []string{"synchub-validation", "mcp/synchub-validation/server.mjs"} {
		if !strings.Contains(text, required) {
			t.Fatalf(".mcp.json must mention %q", required)
		}
	}
	server, err := os.ReadFile(filepath.Join("..", "..", "mcp", "synchub-validation", "server.mjs"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(server), `import "../../tools/mcp/validation-server.mjs"`) {
		t.Fatal("shipped MCP server must delegate to tools/mcp/validation-server.mjs")
	}
	impl, err := os.ReadFile(filepath.Join("..", "..", "tools", "mcp", "validation-server.mjs"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(impl)
	for _, required := range []string{"tools/list", "repository_validation_commands", "readOnlyHint"} {
		if !strings.Contains(body, required) {
			t.Fatalf("MCP implementation must contain %q", required)
		}
	}
}
