package desktop

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"
)

func TestPreviewCoordinatorCoalescesConcurrentCalls(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	coordinator := newPreviewCoordinator(func(ctx context.Context) (ResourcePreview, error) {
		if calls.Add(1) == 1 {
			close(started)
		}
		select {
		case <-release:
			return ResourcePreview{Files: 8}, nil
		case <-ctx.Done():
			return ResourcePreview{}, ctx.Err()
		}
	})

	results := make(chan previewResult, 2)
	go func() {
		preview, err := coordinator.refresh(context.Background())
		results <- previewResult{preview: preview, err: err}
	}()
	<-started
	go func() {
		preview, err := coordinator.refresh(context.Background())
		results <- previewResult{preview: preview, err: err}
	}()
	waitForPreviewWaiters(t, coordinator, 2)
	close(release)

	for range 2 {
		result := <-results
		if result.err != nil {
			t.Fatal(result.err)
		}
		if result.preview.Files != 8 {
			t.Fatalf("preview files = %d, want 8", result.preview.Files)
		}
	}
	if calls.Load() != 1 {
		t.Fatalf("collector calls = %d, want 1", calls.Load())
	}
}

func TestPreviewCoordinatorKeepsCollectionForRemainingWaiter(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	collectionCanceled := make(chan struct{}, 1)
	coordinator := newPreviewCoordinator(func(ctx context.Context) (ResourcePreview, error) {
		close(started)
		select {
		case <-release:
			return ResourcePreview{Files: 9}, nil
		case <-ctx.Done():
			collectionCanceled <- struct{}{}
			return ResourcePreview{}, ctx.Err()
		}
	})

	firstCtx, cancelFirst := context.WithCancel(context.Background())
	firstResult := make(chan previewResult, 1)
	go func() {
		preview, err := coordinator.refresh(firstCtx)
		firstResult <- previewResult{preview: preview, err: err}
	}()
	<-started
	secondResult := make(chan previewResult, 1)
	go func() {
		preview, err := coordinator.refresh(context.Background())
		secondResult <- previewResult{preview: preview, err: err}
	}()
	waitForPreviewWaiters(t, coordinator, 2)

	cancelFirst()
	if result := <-firstResult; !errors.Is(result.err, context.Canceled) {
		t.Fatalf("first caller error = %v, want context canceled", result.err)
	}
	select {
	case <-collectionCanceled:
		t.Fatal("collection was canceled while a waiter remained")
	case <-time.After(20 * time.Millisecond):
	}

	close(release)
	result := <-secondResult
	if result.err != nil || result.preview.Files != 9 {
		t.Fatalf("second result = %#v, %v", result.preview, result.err)
	}
}

func TestPreviewCoordinatorCancelsCollectionWithoutWaiters(t *testing.T) {
	started := make(chan struct{})
	collectionCanceled := make(chan struct{})
	coordinator := newPreviewCoordinator(func(ctx context.Context) (ResourcePreview, error) {
		close(started)
		<-ctx.Done()
		close(collectionCanceled)
		return ResourcePreview{}, ctx.Err()
	})

	firstCtx, cancelFirst := context.WithCancel(context.Background())
	secondCtx, cancelSecond := context.WithCancel(context.Background())
	results := make(chan error, 2)
	go func() {
		_, err := coordinator.refresh(firstCtx)
		results <- err
	}()
	<-started
	go func() {
		_, err := coordinator.refresh(secondCtx)
		results <- err
	}()
	waitForPreviewWaiters(t, coordinator, 2)

	cancelFirst()
	cancelSecond()
	for range 2 {
		if err := <-results; !errors.Is(err, context.Canceled) {
			t.Fatalf("caller error = %v, want context canceled", err)
		}
	}
	select {
	case <-collectionCanceled:
	case <-time.After(time.Second):
		t.Fatal("collector context was not canceled")
	}
}

func waitForPreviewWaiters(t *testing.T, coordinator *previewCoordinator, want int) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		coordinator.mu.Lock()
		matched := coordinator.inflight != nil && coordinator.inflight.waiters == want
		coordinator.mu.Unlock()
		if matched {
			return
		}
		time.Sleep(2 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for %d preview waiters", want)
}
