package daemon

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"control/internal/types"
)

type tailStreamServiceStub struct {
	ch     <-chan types.LogEvent
	cancel func()
	err    error
}

func (s *tailStreamServiceStub) Subscribe(context.Context, string, string) (<-chan types.LogEvent, func(), error) {
	if s == nil {
		return nil, nil, errors.New("nil tail stream service")
	}
	if s.err != nil {
		return nil, nil, s.err
	}
	cancel := s.cancel
	if cancel == nil {
		cancel = func() {}
	}
	return s.ch, cancel, nil
}

func TestStreamTailWithServiceStreamsEvent(t *testing.T) {
	api := &API{}
	ch := make(chan types.LogEvent, 1)
	ch <- types.LogEvent{Type: "log", Stream: "stdout", Chunk: "hello"}
	close(ch)
	service := &tailStreamServiceStub{ch: ch}
	req := httptest.NewRequest(http.MethodGet, "/v1/sessions/s1/tail?follow=1&stream=stdout", nil)
	rec := httptest.NewRecorder()
	api.streamTailWithService(rec, req, "s1", "stdout", service)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "hello") {
		t.Fatalf("expected payload in SSE body, got %q", rec.Body.String())
	}
}

func TestStreamTailWithServiceSubscribeError(t *testing.T) {
	api := &API{}
	service := &tailStreamServiceStub{err: errors.New("subscribe failed")}
	req := httptest.NewRequest(http.MethodGet, "/v1/sessions/s1/tail?follow=1", nil)
	rec := httptest.NewRecorder()
	api.streamTailWithService(rec, req, "s1", "combined", service)
	if rec.Code == http.StatusOK {
		t.Fatalf("expected non-200 status for subscribe error")
	}
}

func TestStreamTailWithServiceRequiresFlusher(t *testing.T) {
	api := &API{}
	ch := make(chan types.LogEvent)
	close(ch)
	service := &tailStreamServiceStub{ch: ch}
	req := httptest.NewRequest(http.MethodGet, "/v1/sessions/s1/tail?follow=1", nil)
	w := &noFlushResponseWriter{}
	api.streamTailWithService(w, req, "s1", "combined", service)
	if w.code != http.StatusInternalServerError {
		t.Fatalf("expected 500 for non-flusher writer, got %d", w.code)
	}
}
