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

type eventsStreamServiceStub struct {
	ch     <-chan types.CodexEvent
	cancel func()
	err    error
}

func (s *eventsStreamServiceStub) SubscribeEvents(context.Context, string) (<-chan types.CodexEvent, func(), error) {
	if s == nil {
		return nil, nil, errors.New("nil events stream service")
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

type itemsStreamServiceStub struct {
	snapshot []map[string]any
	readErr  error
	ch       <-chan map[string]any
	cancel   func()
	subErr   error
}

func (s *itemsStreamServiceStub) readSessionItems(string, int) ([]map[string]any, bool, error) {
	if s == nil || s.readErr != nil {
		return nil, false, s.readErr
	}
	out := make([]map[string]any, 0, len(s.snapshot))
	for _, item := range s.snapshot {
		cloned := map[string]any{}
		for key, value := range item {
			cloned[key] = value
		}
		out = append(out, cloned)
	}
	return out, false, nil
}

func (s *itemsStreamServiceStub) SubscribeItems(context.Context, string) (<-chan map[string]any, func(), error) {
	if s == nil {
		return nil, nil, errors.New("nil items stream service")
	}
	if s.subErr != nil {
		return nil, nil, s.subErr
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

func TestStreamEventsWithServiceWritesJSONEvents(t *testing.T) {
	api := &API{}
	ch := make(chan types.CodexEvent, 1)
	ch <- types.CodexEvent{Method: "turn/completed"}
	close(ch)
	service := &eventsStreamServiceStub{ch: ch}
	req := httptest.NewRequest(http.MethodGet, "/v1/sessions/s1/events?follow=1", nil)
	rec := httptest.NewRecorder()
	api.streamEventsWithService(rec, req, "s1", service)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	found := false
	for _, line := range strings.Split(rec.Body.String(), "\n") {
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var event types.CodexEvent
		if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &event); err == nil && strings.TrimSpace(event.Method) != "" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected at least one events payload, got %q", rec.Body.String())
	}
}

func TestStreamEventsWithServiceSubscribeError(t *testing.T) {
	api := &API{}
	service := &eventsStreamServiceStub{err: errors.New("subscribe failed")}
	req := httptest.NewRequest(http.MethodGet, "/v1/sessions/s1/events?follow=1", nil)
	rec := httptest.NewRecorder()
	api.streamEventsWithService(rec, req, "s1", service)
	if rec.Code == http.StatusOK {
		t.Fatalf("expected non-200 status for subscribe error")
	}
}

func TestStreamItemsWithServiceStreamsSnapshotAndFollow(t *testing.T) {
	api := &API{}
	ch := make(chan map[string]any, 1)
	ch <- map[string]any{"type": "assistant", "text": "live"}
	close(ch)
	service := &itemsStreamServiceStub{
		snapshot: []map[string]any{{"type": "assistant", "text": "snap"}},
		ch:       ch,
	}
	req := httptest.NewRequest(http.MethodGet, "/v1/sessions/s1/items?follow=1&lines=10", nil)
	rec := httptest.NewRecorder()
	api.streamItemsWithService(rec, req, "s1", service)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "snap") || !strings.Contains(body, "live") {
		t.Fatalf("expected snapshot + live payloads, got %q", body)
	}
}

func TestStreamItemsWithServiceSnapshotOnlyOnSubscribeError(t *testing.T) {
	api := &API{}
	service := &itemsStreamServiceStub{
		snapshot: []map[string]any{{"type": "assistant", "text": "snap"}},
		subErr:   errors.New("subscribe failed"),
	}
	req := httptest.NewRequest(http.MethodGet, "/v1/sessions/s1/items?follow=1&lines=10", nil)
	rec := httptest.NewRecorder()
	api.streamItemsWithService(rec, req, "s1", service)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected snapshot fallback status 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "snap") {
		t.Fatalf("expected snapshot payload in fallback path, got %q", rec.Body.String())
	}
}

func TestStreamItemsWithServiceSubscribeErrorWithoutSnapshot(t *testing.T) {
	api := &API{}
	service := &itemsStreamServiceStub{
		readErr: errors.New("read failed"),
		subErr:  errors.New("subscribe failed"),
	}
	req := httptest.NewRequest(http.MethodGet, "/v1/sessions/s1/items?follow=1&lines=10", nil)
	rec := httptest.NewRecorder()
	api.streamItemsWithService(rec, req, "s1", service)
	if rec.Code == http.StatusOK {
		t.Fatalf("expected non-200 status when no snapshot and subscribe fails")
	}
}
