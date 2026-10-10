package repocheck

import (
	"strings"
	"testing"
)

func TestStandaloneCLIMatrixIsRequiredByCIAndRelease(t *testing.T) {
	for _, name := range []string{"ci.yml", "release.yml"} {
		w := loadWorkflow(t, name)
		if w.Jobs["cli"].Uses != "./.github/workflows/cli.yml" {
			t.Errorf("%s must use the standalone CLI matrix", name)
		}
	}
	text := packagingFile(t, ".github/workflows/release.yml")
	for _, required := range []string{"needs: [windows, macos, linux, cli]", "needs.cli.result == 'success'", "SHA256SUMS-cli-*.txt"} {
		if !strings.Contains(text, required) {
			t.Errorf("release CLI gate missing %q", required)
		}
	}
	text = packagingFile(t, ".github/workflows/cli.yml")
	for _, required := range []string{
		"windows-2025", "macos-15-intel", "macos-15", "ubuntu-24.04",
		"goos: windows", "goos: darwin", "goos: linux", "arch: amd64", "arch: arm64",
		"CGO_ENABLED:", "go test", "./internal/instance", "./cmd/synchub", "go run ./tools/clipack",
		"release-cli-", "if-no-files-found: error",
	} {
		if !strings.Contains(text, required) {
			t.Errorf("CLI native matrix missing %q", required)
		}
	}
}
