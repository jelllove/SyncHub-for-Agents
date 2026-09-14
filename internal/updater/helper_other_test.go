//go:build !windows

package updater

import "testing"

func TestHelperOtherPlatformsAreUnsupported(t *testing.T) {
	o := helperTestOptions(t)
	handled, err := RunHelper(o.args())
	if !handled || err == nil {
		t.Fatalf("non-Windows helper = %v, %v", handled, err)
	}
	if err := LaunchInstaller(pendingUpdate{}, o.executable, true, o.resultPath); err == nil {
		t.Fatal("non-Windows installation was not rejected")
	}
}
