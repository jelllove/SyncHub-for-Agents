package desktop

import (
	"context"
	"errors"
	"runtime"
	"testing"
	"time"

	"github.com/qinqingxu/acsync/internal/config"
	"github.com/qinqingxu/acsync/internal/provider"
)

func TestWailsResourceMethodsDelegateToCore(t *testing.T) {
	core := configuredResourceService(t)
	defer core.Close()
	service := NewWailsService(nil, core, nil, nil)

	if _, err := service.ResourcePreview(context.Background()); err != nil {
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
	if err := service.QueueConflictBatch([]ConflictSelection{{
		ID: "missing", Revision: "stale", Choice: "local",
	}}); err == nil {
		t.Fatal("invalid conflict batch was accepted")
	}
	if err := service.RetryConflictBatch("missing"); err == nil {
		t.Fatal("missing failed conflict batch was accepted")
	}
	if _, err := service.PreviewCustomResource(context.Background(), CustomResourceInput{
		ID: "notes", Category: "instructions",
		Paths:   map[string]string{runtime.GOOS: t.TempDir()},
		Targets: map[string]string{runtime.GOOS: t.TempDir()},
		Include: []string{"**"}, Strategy: "text-tree",
	}); err != nil {
		t.Fatal(err)
	}
}

func TestWailsResourcePreviewForwardsCancellation(t *testing.T) {
	core := configuredResourceService(t)
	defer core.Close()
	started := make(chan struct{})
	collectionCanceled := make(chan struct{})
	core.previewCoordinator = newPreviewCoordinator(func(
		ctx context.Context,
	) (ResourcePreview, error) {
		close(started)
		<-ctx.Done()
		close(collectionCanceled)
		return ResourcePreview{}, ctx.Err()
	})
	service := NewWailsService(nil, core, nil, nil)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := service.ResourcePreview(ctx)
		done <- err
	}()

	<-started
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("ResourcePreview() error = %v, want context canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Wails resource preview did not return after cancellation")
	}
	select {
	case <-collectionCanceled:
	case <-time.After(time.Second):
		t.Fatal("Wails resource preview did not cancel collection")
	}
}

func TestWailsCustomPreviewForwardsContext(t *testing.T) {
	core := configuredResourceService(t)
	defer core.Close()
	type contextKey string
	const key contextKey = "preview"
	received := make(chan context.Context, 1)
	core.previewCollector = func(
		ctx context.Context,
		_ config.Config,
		_ []provider.Provider,
	) (ResourcePreview, error) {
		received <- ctx
		return ResourcePreview{}, nil
	}
	service := NewWailsService(nil, core, nil, nil)
	ctx := context.WithValue(context.Background(), key, "forwarded")

	_, err := service.PreviewCustomResource(ctx, CustomResourceInput{
		ID:       "notes",
		Category: "instructions",
		Paths:    map[string]string{runtime.GOOS: t.TempDir()},
		Targets:  map[string]string{runtime.GOOS: t.TempDir()},
		Include:  []string{"**"},
		Strategy: "text-tree",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := (<-received).Value(key); got != "forwarded" {
		t.Fatalf("forwarded context value = %v", got)
	}
}
