package daemon

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"control/internal/types"
)

type metadataStreamServiceStub struct {
	after  string
	ch     <-chan types.MetadataEvent
	cancel func()
	subErr error
}

func (s *metadataStreamServiceStub) Subscribe(afterRevision string) (<-chan types.MetadataEvent, func(), error) {
	if s == nil {
		return nil, nil, errors.New("nil metadata stream service")
	}
	s.after = afterRevision
	if s.subErr != nil {
		return nil, nil, s.subErr
	}
	cancel := s.cancel
	if cancel == nil {
		cancel = func() {}
	}
	return s.ch, cancel, nil
}

func TestMetadataStreamEndpointStreamsEvents(t *testing.T) {
	api := &API{}
	ch := make(chan types.MetadataEvent, 1)
	ch <- types.MetadataEvent{
		Version:  types.MetadataEventSchemaVersionV1,
		Type:     types.MetadataEventTypeSessionUpdated,
		Revision: "3",
		Session: &types.MetadataEntityUpdated{
			ID:    "s1",
			Title: "Renamed",
		},
	}
	close(ch)
	service := &metadataStreamServiceStub{ch: ch}
	req := httptest.NewRequest(http.MethodGet, "/v1/metadata/stream?follow=1&after_revision=2", nil)
	rec := httptest.NewRecorder()
	api.streamMetadataEvents(rec, req, service)
	if got := strings.TrimSpace(service.after); got != "2" {
		t.Fatalf("expected after_revision 2, got %q", got)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "id: 3") || !strings.Contains(body, "session.updated") {
		t.Fatalf("expected SSE id + payload, got %q", body)
	}
}

func TestMetadataStreamEndpointUsesLastEventIDFallback(t *testing.T) {
	api := &API{}
	ch := make(chan types.MetadataEvent)
	close(ch)
	service := &metadataStreamServiceStub{ch: ch}
	req := httptest.NewRequest(http.MethodGet, "/v1/metadata/stream?follow=1", nil)
	req.Header.Set("Last-Event-ID", "7")
	rec := httptest.NewRecorder()
	api.streamMetadataEvents(rec, req, service)
	if got := strings.TrimSpace(service.after); got != "7" {
		t.Fatalf("expected Last-Event-ID fallback, got %q", got)
	}
}

func TestMetadataStreamEndpointInvalidAfterRevision(t *testing.T) {
	api := &API{}
	service := &metadataStreamServiceStub{subErr: errInvalidAfterRevision}
	req := httptest.NewRequest(http.MethodGet, "/v1/metadata/stream?follow=1&after_revision=bad", nil)
	rec := httptest.NewRecorder()
	api.streamMetadataEvents(rec, req, service)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestMetadataStreamEndpointRequiresFollow(t *testing.T) {
	api := &API{}
	req := httptest.NewRequest(http.MethodGet, "/v1/metadata/stream", nil)
	rec := httptest.NewRecorder()
	api.MetadataStreamEndpoint(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestMetadataStreamEndpointRejectsNonGetMethod(t *testing.T) {
	api := &API{}
	req := httptest.NewRequest(http.MethodPost, "/v1/metadata/stream?follow=1", nil)
	rec := httptest.NewRecorder()
	api.MetadataStreamEndpoint(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
}

func TestMetadataStreamEndpointNilService(t *testing.T) {
	api := &API{}
	req := httptest.NewRequest(http.MethodGet, "/v1/metadata/stream?follow=1", nil)
	rec := httptest.NewRecorder()
	api.streamMetadataEvents(rec, req, nil)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", rec.Code)
	}
}

func TestMetadataStreamEndpointSubscribeError(t *testing.T) {
	api := &API{}
	service := &metadataStreamServiceStub{subErr: errors.New("subscribe failed")}
	req := httptest.NewRequest(http.MethodGet, "/v1/metadata/stream?follow=1", nil)
	rec := httptest.NewRecorder()
	api.streamMetadataEvents(rec, req, service)
	if rec.Code == http.StatusOK {
		t.Fatalf("expected non-200 on subscribe failure")
	}
}

func TestMetadataStreamEndpointRequiresFlusher(t *testing.T) {
	api := &API{}
	ch := make(chan types.MetadataEvent)
	close(ch)
	service := &metadataStreamServiceStub{ch: ch}
	req := httptest.NewRequest(http.MethodGet, "/v1/metadata/stream?follow=1", nil)
	w := &noFlushResponseWriter{}
	api.streamMetadataEvents(w, req, service)
	if w.code != http.StatusInternalServerError {
		t.Fatalf("expected 500 for non-flusher writer, got %d", w.code)
	}
}

func TestMetadataStreamEndpointSkipsMalformedEventPayloadAndContinues(t *testing.T) {
	api := &API{}
	ch := make(chan types.MetadataEvent, 2)
	ch <- types.MetadataEvent{
		Type:     types.MetadataEventTypeSessionUpdated,
		Revision: "8",
		Session: &types.MetadataEntityUpdated{
			ID:    "s1",
			Title: "bad",
			Changed: map[string]any{
				"bad": make(chan int),
			},
		},
	}
	ch <- types.MetadataEvent{
		Type:     types.MetadataEventTypeSessionUpdated,
		Revision: "9",
		Session: &types.MetadataEntityUpdated{
			ID:    "s1",
			Title: "good",
		},
	}
	close(ch)
	service := &metadataStreamServiceStub{ch: ch}
	req := httptest.NewRequest(http.MethodGet, "/v1/metadata/stream?follow=1", nil)
	rec := httptest.NewRecorder()
	api.streamMetadataEvents(rec, req, service)

	body := rec.Body.String()
	if strings.Contains(body, "id: 8") {
		t.Fatalf("expected malformed event to be skipped, got body %q", body)
	}
	if !strings.Contains(body, "id: 9") || !strings.Contains(body, `"title":"good"`) {
		t.Fatalf("expected stream to continue after malformed event, got body %q", body)
	}
}

var _ MetadataEventStreamService = (*metadataStreamServiceStub)(nil)
