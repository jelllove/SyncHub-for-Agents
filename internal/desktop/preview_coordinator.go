package desktop

import (
	"context"
	"sync"
)

type previewResult struct {
	preview ResourcePreview
	err     error
}

type previewCall struct {
	done      chan struct{}
	cancel    context.CancelFunc
	waiters   int
	completed bool
	result    previewResult
}

type previewCoordinator struct {
	mu       sync.Mutex
	collect  func(context.Context) (ResourcePreview, error)
	inflight *previewCall
}

func newPreviewCoordinator(
	collect func(context.Context) (ResourcePreview, error),
) *previewCoordinator {
	return &previewCoordinator{collect: collect}
}

func (c *previewCoordinator) refresh(ctx context.Context) (ResourcePreview, error) {
	if err := ctx.Err(); err != nil {
		return ResourcePreview{}, err
	}

	c.mu.Lock()
	call := c.inflight
	if call == nil {
		collectionCtx, cancel := context.WithCancel(context.Background())
		call = &previewCall{
			done:    make(chan struct{}),
			cancel:  cancel,
			waiters: 1,
		}
		c.inflight = call
		go c.run(call, collectionCtx)
	} else {
		call.waiters++
	}
	c.mu.Unlock()

	select {
	case <-call.done:
		c.release(call)
		return call.result.preview, call.result.err
	case <-ctx.Done():
		c.release(call)
		return ResourcePreview{}, ctx.Err()
	}
}

func (c *previewCoordinator) run(call *previewCall, ctx context.Context) {
	preview, err := c.collect(ctx)

	c.mu.Lock()
	call.result = previewResult{preview: preview, err: err}
	call.completed = true
	if c.inflight == call {
		c.inflight = nil
	}
	close(call.done)
	c.mu.Unlock()
	call.cancel()
}

func (c *previewCoordinator) release(call *previewCall) {
	c.mu.Lock()
	call.waiters--
	if call.waiters == 0 && !call.completed {
		if c.inflight == call {
			c.inflight = nil
		}
		call.cancel()
	}
	c.mu.Unlock()
}
