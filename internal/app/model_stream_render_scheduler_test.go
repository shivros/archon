package app

import (
	"testing"
	"time"
)

type recordingRenderScheduler struct {
	requestReturn      bool
	shouldRenderReturn bool
	requestCalls       int
	shouldRenderCalls  int
	markRenderedCalls  int
}

func (r *recordingRenderScheduler) Request(time.Time) bool {
	r.requestCalls++
	return r.requestReturn
}

func (r *recordingRenderScheduler) ShouldRender(time.Time) bool {
	r.shouldRenderCalls++
	return r.shouldRenderReturn
}

func (r *recordingRenderScheduler) MarkRendered(time.Time) {
	r.markRenderedCalls++
}

func TestRenderViewportDoesNotMarkStreamScheduler(t *testing.T) {
	scheduler := &recordingRenderScheduler{}
	m := NewModel(nil, WithRenderScheduler(scheduler))

	m.renderViewport()

	if scheduler.markRenderedCalls != 0 {
		t.Fatalf("expected renderViewport to avoid stream scheduler mark, got %d", scheduler.markRenderedCalls)
	}
}

func TestRequestStreamRenderMarksWhenRequestAllowed(t *testing.T) {
	scheduler := &recordingRenderScheduler{requestReturn: true}
	m := NewModel(nil, WithRenderScheduler(scheduler))

	m.requestStreamRender(time.Now())

	if scheduler.requestCalls != 1 {
		t.Fatalf("expected one request call, got %d", scheduler.requestCalls)
	}
	if scheduler.markRenderedCalls != 1 {
		t.Fatalf("expected one mark call, got %d", scheduler.markRenderedCalls)
	}
}

func TestHandleTickMarksWhenDeferredStreamRenderAllowed(t *testing.T) {
	scheduler := &recordingRenderScheduler{shouldRenderReturn: true}
	m := NewModel(nil, WithRenderScheduler(scheduler))

	m.handleTick(tickMsg(time.Now()))

	if scheduler.shouldRenderCalls == 0 {
		t.Fatalf("expected ShouldRender to be checked")
	}
	if scheduler.markRenderedCalls != 1 {
		t.Fatalf("expected deferred tick render to mark once, got %d", scheduler.markRenderedCalls)
	}
}
