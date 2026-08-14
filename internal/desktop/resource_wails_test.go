package desktop

import (
	"runtime"
	"testing"
)

func TestWailsResourceMethodsDelegateToCore(t *testing.T) {
	core := configuredResourceService(t)
	defer core.Close()
	service := NewWailsService(nil, core, nil, nil)

	if _, err := service.ResourcePreview(); err != nil {
		t.Fatal(err)
	}
	if err := service.ApproveInstallPlan("other"); err == nil {
		t.Fatal("wrong plan ID was accepted")
	}
	if err := service.ResolveConflict(ConflictResolution{
		ID: "missing", Choice: "remote",
	}); err == nil {
		t.Fatal("missing conflict ID was accepted")
	}
	if _, err := service.PreviewCustomResource(CustomResourceInput{
		ID: "notes", Category: "instructions",
		Paths:   map[string]string{runtime.GOOS: t.TempDir()},
		Targets: map[string]string{runtime.GOOS: t.TempDir()},
		Include: []string{"**"}, Strategy: "text-tree",
	}); err != nil {
		t.Fatal(err)
	}
}
