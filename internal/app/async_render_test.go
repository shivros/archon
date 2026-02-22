package app

import (
	"strings"
	"sync"
	"testing"
	"time"
)

type blockingRenderPipeline struct {
	mu      sync.Mutex
	nextID  int
	waiters map[int]chan struct{}
	started chan int
}

func newBlockingRenderPipeline() *blockingRenderPipeline {
	return &blockingRenderPipeline{
		waiters: map[int]chan struct{}{},
		started: make(chan int, 16),
	}
}

func (p *blockingRenderPipeline) Render(req RenderRequest) RenderResult {
	if len(req.Blocks) == 0 {
		return newRenderResult(strings.TrimSpace(req.RawContent), nil)
	}
	p.mu.Lock()
	p.nextID++
	id := p.nextID
	wait := make(chan struct{})
	p.waiters[id] = wait
	p.mu.Unlock()

	p.started <- id
	<-wait

	text := strings.TrimSpace(req.Blocks[0].Text)
	return newRenderResult(text, nil)
}

func (p *blockingRenderPipeline) waitForStart(t *testing.T) int {
	t.Helper()
	select {
	case id := <-p.started:
		return id
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for render start")
		return 0
	}
}

func (p *blockingRenderPipeline) release(id int) {
	p.mu.Lock()
	wait := p.waiters[id]
	delete(p.waiters, id)
	p.mu.Unlock()
	if wait != nil {
		close(wait)
	}
}

func waitForCompletedRender(t *testing.T, renderer *asyncViewportRenderer) asyncViewportRenderResult {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if result, ok := renderer.TakeCompleted(); ok {
			return result
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for async render completion")
	return asyncViewportRenderResult{}
}

func TestAsyncViewportRendererCoalescesPendingJobs(t *testing.T) {
	renderer := newAsyncViewportRenderer()
	pipeline := newBlockingRenderPipeline()

	renderer.Schedule(asyncViewportRenderJob{
		generation: 1,
		signature:  viewportRenderSignature{contentVersion: 1},
		request:    RenderRequest{Blocks: []ChatBlock{{Role: ChatRoleAgent, Text: "first"}}},
	}, pipeline)
	firstID := pipeline.waitForStart(t)

	renderer.Schedule(asyncViewportRenderJob{
		generation: 2,
		signature:  viewportRenderSignature{contentVersion: 2},
		request:    RenderRequest{Blocks: []ChatBlock{{Role: ChatRoleAgent, Text: "second"}}},
	}, pipeline)
	renderer.Schedule(asyncViewportRenderJob{
		generation: 3,
		signature:  viewportRenderSignature{contentVersion: 3},
		request:    RenderRequest{Blocks: []ChatBlock{{Role: ChatRoleAgent, Text: "third"}}},
	}, pipeline)

	pipeline.release(firstID)
	firstDone := waitForCompletedRender(t, renderer)
	if firstDone.generation != 1 {
		t.Fatalf("expected first completion generation 1, got %d", firstDone.generation)
	}

	secondID := pipeline.waitForStart(t)
	pipeline.release(secondID)
	secondDone := waitForCompletedRender(t, renderer)
	if secondDone.generation != 3 {
		t.Fatalf("expected coalesced completion generation 3, got %d", secondDone.generation)
	}
}

func TestModelConsumeCompletedViewportRenderDropsStaleGeneration(t *testing.T) {
	pipeline := newBlockingRenderPipeline()
	m := NewModel(nil, WithAsyncViewportRendering(true), WithRenderPipeline(pipeline))
	m.resize(120, 40)

	m.applyBlocks([]ChatBlock{{Role: ChatRoleAgent, Text: "first render"}})
	firstID := pipeline.waitForStart(t)

	m.applyBlocks([]ChatBlock{{Role: ChatRoleAgent, Text: "second render"}})

	pipeline.release(firstID)
	secondID := pipeline.waitForStart(t)
	m.consumeCompletedViewportRender()
	if strings.Contains(strings.ToLower(m.renderedText), "first render") {
		t.Fatalf("expected stale render generation to be ignored")
	}

	pipeline.release(secondID)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		m.consumeCompletedViewportRender()
		if strings.Contains(strings.ToLower(m.renderedText), "second render") {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	if !strings.Contains(strings.ToLower(m.renderedText), "second render") {
		t.Fatalf("expected latest render generation to apply, got %q", m.renderedText)
	}
}
