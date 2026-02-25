package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"control/internal/types"
)

type debugServiceStub struct {
	snapshot []types.DebugEvent
	readErr  error
	subCh    <-chan types.DebugEvent
	cancel   func()
	subErr   error
}

func (s *debugServiceStub) ReadDebug(context.Context, string, int) ([]types.DebugEvent, bool, error) {
	if s.readErr != nil {
		return nil, false, s.readErr
	}
	return append([]types.DebugEvent(nil), s.snapshot...), false, nil
}

func (s *debugServiceStub) SubscribeDebug(context.Context, string) (<-chan types.DebugEvent, func(), error) {
	if s.subErr != nil {
		return nil, nil, s.subErr
	}
	cancel := s.cancel
	if cancel == nil {
		cancel = func() {}
	}
	return s.subCh, cancel, nil
}

type noFlushResponseWriter struct {
	header http.Header
	body   strings.Builder
	code   int
}

func (w *noFlushResponseWriter) Header() http.Header {
	if w.header == nil {
		w.header = make(http.Header)
	}
	return w.header
}

func (w *noFlushResponseWriter) Write(b []byte) (int, error) {
	return w.body.Write(b)
}

func (w *noFlushResponseWriter) WriteHeader(statusCode int) {
	w.code = statusCode
}

func TestStreamDebugWithServiceNilService(t *testing.T) {
	api := &API{}
	req := httptest.NewRequest(http.MethodGet, "/v1/sessions/s1/debug?follow=1", nil)
	rec := httptest.NewRecorder()
	api.streamDebugWithService(rec, req, "s1", nil)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}

func TestStreamDebugWithServiceStreamsSnapshotAndFollow(t *testing.T) {
	api := &API{}
	ch := make(chan types.DebugEvent, 1)
	ch <- types.DebugEvent{Type: "debug", SessionID: "s1", Chunk: "live"}
	close(ch)
	service := &debugServiceStub{
		snapshot: []types.DebugEvent{{Type: "debug", SessionID: "s1", Chunk: "snap"}},
		subCh:    ch,
	}
	req := httptest.NewRequest(http.MethodGet, "/v1/sessions/s1/debug?follow=1", nil)
	rec := httptest.NewRecorder()
	api.streamDebugWithService(rec, req, "s1", service)

	payload := rec.Body.String()
	if !strings.Contains(payload, "snap") || !strings.Contains(payload, "live") {
		t.Fatalf("expected snapshot + live payload, got %q", payload)
	}
}

func TestStreamDebugWithServiceSubscribeError(t *testing.T) {
	api := &API{}
	service := &debugServiceStub{
		readErr: errors.New("read failed"),
		subErr:  errors.New("subscribe failed"),
	}
	req := httptest.NewRequest(http.MethodGet, "/v1/sessions/s1/debug?follow=1", nil)
	rec := httptest.NewRecorder()
	api.streamDebugWithService(rec, req, "s1", service)
	if rec.Code == http.StatusOK {
		t.Fatalf("expected non-200 on subscribe failure")
	}
}

func TestStreamDebugWithServiceRequiresFlusher(t *testing.T) {
	api := &API{}
	ch := make(chan types.DebugEvent)
	close(ch)
	service := &debugServiceStub{subCh: ch}
	req := httptest.NewRequest(http.MethodGet, "/v1/sessions/s1/debug?follow=1", nil)
	w := &noFlushResponseWriter{}
	api.streamDebugWithService(w, req, "s1", service)
	if w.code != http.StatusInternalServerError {
		t.Fatalf("expected 500 for non-flusher writer, got %d", w.code)
	}
}

func TestStreamDebugWithServiceWritesJSONEvents(t *testing.T) {
	api := &API{}
	ch := make(chan types.DebugEvent, 1)
	ch <- types.DebugEvent{Type: "debug", SessionID: "s1", Stream: "stdout", Chunk: "live"}
	close(ch)
	service := &debugServiceStub{subCh: ch}
	req := httptest.NewRequest(http.MethodGet, "/v1/sessions/s1/debug?follow=1", nil)
	rec := httptest.NewRecorder()
	api.streamDebugWithService(rec, req, "s1", service)

	lines := strings.Split(rec.Body.String(), "\n")
	foundJSON := false
	for _, line := range lines {
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var event types.DebugEvent
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event); err == nil {
			foundJSON = true
			break
		}
	}
	if !foundJSON {
		t.Fatalf("expected at least one valid SSE data JSON event, body=%q", rec.Body.String())
	}
}
