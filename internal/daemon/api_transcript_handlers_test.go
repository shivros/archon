package daemon

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"control/internal/daemon/transcriptdomain"
)

type transcriptSnapshotServiceStub struct {
	snapshot transcriptdomain.TranscriptSnapshot
	err      error
}

func (s *transcriptSnapshotServiceStub) GetTranscriptSnapshot(context.Context, string, int) (transcriptdomain.TranscriptSnapshot, error) {
	if s.err != nil {
		return transcriptdomain.TranscriptSnapshot{}, s.err
	}
	return s.snapshot, nil
}

type transcriptStreamServiceStub struct {
	ch     <-chan transcriptdomain.TranscriptEvent
	cancel func()
	err    error
}

func (s *transcriptStreamServiceStub) SubscribeTranscript(context.Context, string, transcriptdomain.RevisionToken) (<-chan transcriptdomain.TranscriptEvent, func(), error) {
	if s.err != nil {
		return nil, nil, s.err
	}
	cancel := s.cancel
	if cancel == nil {
		cancel = func() {}
	}
	return s.ch, cancel, nil
}

func TestParseAfterRevision(t *testing.T) {
	token, err := parseAfterRevision(" 12 ")
	if err != nil {
		t.Fatalf("parseAfterRevision: %v", err)
	}
	if token.String() != "12" {
		t.Fatalf("expected token 12, got %q", token.String())
	}
	if token, err = parseAfterRevision(" "); err != nil {
		t.Fatalf("parseAfterRevision empty: %v", err)
	} else if !token.IsZero() {
		t.Fatalf("expected zero token for empty revision")
	}
	if _, err := parseAfterRevision("bad revision"); err == nil {
		t.Fatalf("expected parse error")
	}
}

func TestTranscriptSnapshotWithService(t *testing.T) {
	api := &API{}
	snapshot := transcriptdomain.TranscriptSnapshot{
		SessionID: "s1",
		Provider:  "codex",
		Revision:  transcriptdomain.MustParseRevisionToken("5"),
		Blocks: []transcriptdomain.Block{{
			Kind: "assistant",
			Text: "hello",
		}},
		Turn: transcriptdomain.TurnState{State: transcriptdomain.TurnStateIdle},
	}
	service := &transcriptSnapshotServiceStub{snapshot: snapshot}
	req := httptest.NewRequest(http.MethodGet, "/v1/sessions/s1/transcript", nil)
	rec := httptest.NewRecorder()

	api.transcriptSnapshotWithService(rec, req, "s1", service)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	var got transcriptdomain.TranscriptSnapshot
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode snapshot: %v", err)
	}
	if got.SessionID != "s1" || got.Revision.String() != "5" {
		t.Fatalf("unexpected snapshot: %#v", got)
	}
}

func TestTranscriptSnapshotWithServiceNilService(t *testing.T) {
	api := &API{}
	req := httptest.NewRequest(http.MethodGet, "/v1/sessions/s1/transcript", nil)
	rec := httptest.NewRecorder()
	api.transcriptSnapshotWithService(rec, req, "s1", nil)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", rec.Code)
	}
}

func TestTranscriptSnapshotWrapperUsesSessionService(t *testing.T) {
	api := &API{}
	req := httptest.NewRequest(http.MethodGet, "/v1/sessions/s1/transcript", nil)
	rec := httptest.NewRecorder()
	api.transcriptSnapshot(rec, req, "s1")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected not-found from default session service for unknown session, got %d", rec.Code)
	}
}

func TestTranscriptStreamWithServiceWritesEvents(t *testing.T) {
	api := &API{}
	ch := make(chan transcriptdomain.TranscriptEvent, 1)
	ch <- transcriptdomain.TranscriptEvent{
		Kind:      transcriptdomain.TranscriptEventDelta,
		SessionID: "s1",
		Provider:  "codex",
		Revision:  transcriptdomain.MustParseRevisionToken("2"),
		Delta:     []transcriptdomain.Block{{Kind: "assistant", Text: "hi"}},
	}
	close(ch)
	service := &transcriptStreamServiceStub{ch: ch}
	req := httptest.NewRequest(http.MethodGet, "/v1/sessions/s1/transcript/stream?follow=1&after_revision=1", nil)
	rec := httptest.NewRecorder()

	api.streamTranscriptWithService(rec, req, "s1", service)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "transcript.delta") {
		t.Fatalf("expected transcript event payload, got %q", rec.Body.String())
	}
}

func TestTranscriptStreamWithServiceNilService(t *testing.T) {
	api := &API{}
	req := httptest.NewRequest(http.MethodGet, "/v1/sessions/s1/transcript/stream?follow=1", nil)
	rec := httptest.NewRecorder()
	api.streamTranscriptWithService(rec, req, "s1", nil)
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", rec.Code)
	}
}

func TestTranscriptStreamWithServiceInvalidAfterRevision(t *testing.T) {
	api := &API{}
	req := httptest.NewRequest(http.MethodGet, "/v1/sessions/s1/transcript/stream?follow=1&after_revision=invalid%20token", nil)
	rec := httptest.NewRecorder()
	api.streamTranscriptWithService(rec, req, "s1", &transcriptStreamServiceStub{err: errors.New("should not be called")})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}
}

func TestTranscriptStreamWithServiceRequiresFlusher(t *testing.T) {
	api := &API{}
	ch := make(chan transcriptdomain.TranscriptEvent)
	close(ch)
	req := httptest.NewRequest(http.MethodGet, "/v1/sessions/s1/transcript/stream?follow=1", nil)
	w := &noFlushResponseWriter{}
	api.streamTranscriptWithService(w, req, "s1", &transcriptStreamServiceStub{ch: ch})
	if w.code != http.StatusInternalServerError {
		t.Fatalf("expected 500 for non-flusher writer, got %d", w.code)
	}
}

func TestTranscriptStreamWrapperUsesSessionService(t *testing.T) {
	api := &API{}
	req := httptest.NewRequest(http.MethodGet, "/v1/sessions/s1/transcript/stream?follow=1", nil)
	rec := httptest.NewRecorder()
	api.streamTranscript(rec, req, "s1")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected not-found from default session service for unknown session, got %d", rec.Code)
	}
}

func TestSessionByIDTranscriptStreamRequiresFollow(t *testing.T) {
	api := &API{}
	req := httptest.NewRequest(http.MethodGet, "/v1/sessions/s1/transcript/stream", nil)
	rec := httptest.NewRecorder()
	api.SessionByID(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "follow=1 is required") {
		t.Fatalf("expected follow required error, got %q", rec.Body.String())
	}
}
