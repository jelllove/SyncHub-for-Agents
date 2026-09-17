package archcheck

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestNativeTrayToolkitsStayInSeparateExecutables(t *testing.T) {
	for _, tc := range []struct {
		target    string
		forbidden string
	}{
		{".", "fyne.io/systray"},
		{"./cmd/synchub", "github.com/wailsapp/wails/v3/pkg/application"},
	} {
		t.Run(tc.target, func(t *testing.T) {
			command := exec.Command("go", "list", "-deps", tc.target)
			command.Dir = filepath.Join("..", "..")
			output, err := command.CombinedOutput()
			if err != nil {
				t.Fatalf("list dependencies: %v\n%s", err, output)
			}
			for _, dependency := range strings.Fields(string(output)) {
				if dependency == tc.forbidden {
					t.Fatalf("%s must not link %s; native tray toolkits have conflicting platform symbols", tc.target, dependency)
				}
			}
		})
	}
}
