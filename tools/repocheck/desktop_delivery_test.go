package repocheck

import (
	"strings"
	"testing"
)

func TestReleaseVerifiesRequestedDesktopFeaturesInActualExecutable(t *testing.T) {
	workflow := loadWorkflow(t, "release.yml")
	var commands string
	for _, step := range workflow.Jobs["windows"].Steps {
		commands += step.Run + "\n"
	}
	for _, required := range []string{
		"./internal/startup", "./internal/tray", "./internal/tray/desktop",
		"src/TrayTip.test.tsx", "ReadAllBytes", "bin\\SyncHub.exe",
		"Settings categories", "settings-tabs", "tray-tip:status",
	} {
		if !strings.Contains(commands, required) {
			t.Errorf("Windows release lacks delivered-feature check %q", required)
		}
	}
}
