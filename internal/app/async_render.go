package app

import (
	"sync"
)

type viewportRenderSignature struct {
	width          int
	contentVersion int
	selectionIndex int
	timestampMode  ChatTimestampMode
	relativeBucket int64
}

func (s viewportRenderSignature) Equal(other viewportRenderSignature) bool {
	return s.width == other.width &&
		s.contentVersion == other.contentVersion &&
		s.selectionIndex == other.selectionIndex &&
		s.timestampMode == other.timestampMode &&
		s.relativeBucket == other.relativeBucket
}

type asyncViewportRenderJob struct {
	generation int
	signature  viewportRenderSignature
	request    RenderRequest
}

type asyncViewportRenderResult struct {
	generation int
	signature  viewportRenderSignature
	result     RenderResult
}

type asyncViewportRenderer struct {
	mu        sync.Mutex
	inFlight  bool
	pending   *asyncViewportRenderJob
	completed *asyncViewportRenderResult
}

func newAsyncViewportRenderer() *asyncViewportRenderer {
	return &asyncViewportRenderer{}
}

func (r *asyncViewportRenderer) Schedule(job asyncViewportRenderJob, pipeline RenderPipeline) {
	if r == nil || pipeline == nil {
		return
	}
	job.request = cloneRenderRequest(job.request)
	r.mu.Lock()
	if r.inFlight {
		copy := job
		r.pending = &copy
		r.mu.Unlock()
		return
	}
	r.inFlight = true
	r.mu.Unlock()
	r.startJob(job, pipeline)
}

func (r *asyncViewportRenderer) startJob(job asyncViewportRenderJob, pipeline RenderPipeline) {
	if r == nil || pipeline == nil {
		return
	}
	go func() {
		result := pipeline.Render(job.request)
		next := r.finishJob(asyncViewportRenderResult{
			generation: job.generation,
			signature:  job.signature,
			result:     result,
		})
		if next != nil {
			r.startJob(*next, pipeline)
		}
	}()
}

func (r *asyncViewportRenderer) finishJob(result asyncViewportRenderResult) *asyncViewportRenderJob {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	copy := result
	r.completed = &copy
	if r.pending != nil {
		next := *r.pending
		r.pending = nil
		return &next
	}
	r.inFlight = false
	return nil
}

func (r *asyncViewportRenderer) TakeCompleted() (asyncViewportRenderResult, bool) {
	if r == nil {
		return asyncViewportRenderResult{}, false
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.completed == nil {
		return asyncViewportRenderResult{}, false
	}
	result := *r.completed
	r.completed = nil
	return result, true
}

func cloneRenderRequest(req RenderRequest) RenderRequest {
	out := req
	if req.Blocks != nil {
		out.Blocks = append([]ChatBlock(nil), req.Blocks...)
	}
	out.BlockMetaByID = cloneChatBlockMetaByID(req.BlockMetaByID)
	return out
}

func WithAsyncViewportRendering(enabled bool) ModelOption {
	return func(m *Model) {
		if m == nil {
			return
		}
		m.asyncViewportRendering = enabled
		if enabled {
			if m.asyncViewportRenderer == nil {
				m.asyncViewportRenderer = newAsyncViewportRenderer()
			}
			return
		}
		m.asyncViewportRenderer = nil
	}
}
