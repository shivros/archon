package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"control/internal/daemon/transcriptdomain"
	"control/internal/types"
)

func TestTailStreamParsesEvents(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)

		event := types.LogEvent{
			Type:   "log",
			Stream: "stdout",
			Chunk:  "hello",
			TS:     time.Now().UTC().Format(time.RFC3339Nano),
		}
		data, _ := json.Marshal(event)
		_, _ = w.Write(append([]byte("data: "), data...))
		_, _ = w.Write([]byte("\n\n"))
		if flusher != nil {
			flusher.Flush()
		}
	}))
	defer server.Close()

	client := &Client{
		baseURL: server.URL,
		token:   "token",
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	ch, stop, err := client.TailStream(ctx, "abc", "stdout")
	if err != nil {
		t.Fatalf("TailStream: %v", err)
	}
	defer stop()

	select {
	case event := <-ch:
		if event.Stream != "stdout" || event.Chunk != "hello" {
			t.Fatalf("unexpected event: %+v", event)
		}
	case <-time.After(1 * time.Second):
		t.Fatalf("timeout waiting for event")
	}
}

func TestDebugStreamParsesEvents(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		event := types.DebugEvent{
			Type:      "debug",
			SessionID: "s1",
			Provider:  "codex",
			Stream:    "stdout",
			Chunk:     "hello-debug",
		}
		data, _ := json.Marshal(event)
		_, _ = w.Write(append([]byte("data: "), data...))
		_, _ = w.Write([]byte("\n\n"))
		if flusher != nil {
			flusher.Flush()
		}
	}))
	defer server.Close()

	client := &Client{
		baseURL: server.URL,
		token:   "token",
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	ch, stop, err := client.DebugStream(ctx, "s1")
	if err != nil {
		t.Fatalf("DebugStream: %v", err)
	}
	defer stop()
	select {
	case event := <-ch:
		if event.SessionID != "s1" || event.Chunk != "hello-debug" {
			t.Fatalf("unexpected debug event: %+v", event)
		}
	case <-time.After(1 * time.Second):
		t.Fatalf("timeout waiting for debug event")
	}
}

func TestDebugStreamReturnsAPIErrorOnNon2xx(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
	}))
	defer server.Close()

	client := &Client{
		baseURL: server.URL,
		token:   "token",
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, _, err := client.DebugStream(ctx, "s1"); err == nil {
		t.Fatalf("expected non-2xx error")
	}
}

func TestTranscriptStreamParsesEvents(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.URL.Query().Get("after_revision") != "2" {
			t.Fatalf("expected after_revision query parameter")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		event := transcriptdomain.TranscriptEvent{
			Kind:      transcriptdomain.TranscriptEventDelta,
			SessionID: "s1",
			Provider:  "codex",
			Revision:  transcriptdomain.MustParseRevisionToken("3"),
			Delta:     []transcriptdomain.Block{{Kind: "assistant", Text: "ok"}},
		}
		data, _ := json.Marshal(event)
		_, _ = w.Write(append([]byte("data: "), data...))
		_, _ = w.Write([]byte("\n\n"))
		if flusher != nil {
			flusher.Flush()
		}
	}))
	defer server.Close()

	c := &Client{baseURL: server.URL, token: "token"}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	ch, stop, err := c.TranscriptStream(ctx, "s1", "2")
	if err != nil {
		t.Fatalf("TranscriptStream: %v", err)
	}
	defer stop()
	select {
	case event := <-ch:
		if event.Kind != transcriptdomain.TranscriptEventDelta || event.Revision.String() != "3" {
			t.Fatalf("unexpected transcript event: %#v", event)
		}
	case <-time.After(time.Second):
		t.Fatalf("timeout waiting for transcript stream payload")
	}
}

func TestTranscriptStreamReturnsAPIErrorOnNon2xx(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
	}))
	defer server.Close()

	c := &Client{baseURL: server.URL, token: "token"}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, _, err := c.TranscriptStream(ctx, "s1", ""); err == nil {
		t.Fatalf("expected non-2xx error")
	}
}

func TestTranscriptStreamOmitsAfterRevisionWhenEmpty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("after_revision"); got != "" {
			t.Fatalf("expected empty after_revision query, got %q", got)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		event := transcriptdomain.TranscriptEvent{
			Kind:      transcriptdomain.TranscriptEventStreamStatus,
			SessionID: "s1",
			Provider:  "codex",
			Revision:  transcriptdomain.MustParseRevisionToken("1"),
		}
		data, _ := json.Marshal(event)
		_, _ = w.Write(append([]byte("data: "), data...))
		_, _ = w.Write([]byte("\n\n"))
		if flusher != nil {
			flusher.Flush()
		}
	}))
	defer server.Close()

	c := &Client{baseURL: server.URL, token: "token"}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	ch, stop, err := c.TranscriptStream(ctx, "s1", "")
	if err != nil {
		t.Fatalf("TranscriptStream: %v", err)
	}
	defer stop()
	select {
	case <-ch:
	case <-time.After(time.Second):
		t.Fatalf("timeout waiting for transcript stream payload")
	}
}

func TestTranscriptStreamEscapesAfterRevisionQuery(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("after_revision"); got != "rev A/B" {
			t.Fatalf("expected decoded after_revision query, got %q", got)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		event := transcriptdomain.TranscriptEvent{
			Kind:      transcriptdomain.TranscriptEventStreamStatus,
			SessionID: "s1",
			Provider:  "codex",
			Revision:  transcriptdomain.MustParseRevisionToken("1"),
		}
		data, _ := json.Marshal(event)
		_, _ = w.Write(append([]byte("data: "), data...))
		_, _ = w.Write([]byte("\n\n"))
		if flusher != nil {
			flusher.Flush()
		}
	}))
	defer server.Close()

	c := &Client{baseURL: server.URL, token: "token"}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	ch, stop, err := c.TranscriptStream(ctx, "s1", "rev A/B")
	if err != nil {
		t.Fatalf("TranscriptStream: %v", err)
	}
	defer stop()
	select {
	case <-ch:
	case <-time.After(time.Second):
		t.Fatalf("timeout waiting for transcript stream payload")
	}
}

func TestTranscriptStreamSkipsMalformedPayload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		_, _ = w.Write([]byte("data: {not-json}\n\n"))
		event := transcriptdomain.TranscriptEvent{
			Kind:      transcriptdomain.TranscriptEventDelta,
			SessionID: "s1",
			Provider:  "codex",
			Revision:  transcriptdomain.MustParseRevisionToken("3"),
			Delta:     []transcriptdomain.Block{{Kind: "assistant", Text: "ok"}},
		}
		data, _ := json.Marshal(event)
		_, _ = w.Write(append([]byte("data: "), data...))
		_, _ = w.Write([]byte("\n\n"))
		if flusher != nil {
			flusher.Flush()
		}
	}))
	defer server.Close()

	c := &Client{baseURL: server.URL, token: "token"}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	ch, stop, err := c.TranscriptStream(ctx, "s1", "")
	if err != nil {
		t.Fatalf("TranscriptStream: %v", err)
	}
	defer stop()
	select {
	case event := <-ch:
		if event.Revision.String() != "3" {
			t.Fatalf("expected valid event after malformed payload, got %#v", event)
		}
	case <-time.After(time.Second):
		t.Fatalf("timeout waiting for transcript stream payload")
	}
}

func TestMetadataStreamParsesEventsAndIDFallback(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if r.URL.Query().Get("after_revision") != "7" {
			t.Fatalf("expected after_revision query parameter")
		}
		if r.Header.Get("Last-Event-ID") != "7" {
			t.Fatalf("expected Last-Event-ID header")
		}
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		_, _ = w.Write([]byte("id: 8\n"))
		_, _ = w.Write([]byte(`data: {"version":"v1","type":"session.updated","session":{"id":"s1","title":"Renamed"}}` + "\n\n"))
		if flusher != nil {
			flusher.Flush()
		}
	}))
	defer server.Close()

	c := &Client{baseURL: server.URL, token: "token"}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	ch, stop, err := c.MetadataStream(ctx, "7")
	if err != nil {
		t.Fatalf("MetadataStream: %v", err)
	}
	defer stop()
	select {
	case event := <-ch:
		if event.Type != "session.updated" {
			t.Fatalf("unexpected event type: %q", event.Type)
		}
		if event.Revision != "8" {
			t.Fatalf("expected revision from SSE id fallback, got %q", event.Revision)
		}
		if event.Session == nil || event.Session.ID != "s1" || event.Session.Title != "Renamed" {
			t.Fatalf("unexpected session payload: %#v", event.Session)
		}
	case <-time.After(time.Second):
		t.Fatalf("timeout waiting for metadata stream payload")
	}
}

func TestMetadataStreamOmitsAfterRevisionWhenEmpty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("after_revision"); got != "" {
			t.Fatalf("expected empty after_revision query, got %q", got)
		}
		if got := r.Header.Get("Last-Event-ID"); got != "" {
			t.Fatalf("expected empty Last-Event-ID header, got %q", got)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		_, _ = w.Write([]byte(`data: {"version":"v1","type":"workflow_run.updated","revision":"11","workflow_run":{"id":"gwf-1","title":"Workflow"}}` + "\n\n"))
		if flusher != nil {
			flusher.Flush()
		}
	}))
	defer server.Close()

	c := &Client{baseURL: server.URL, token: "token"}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	ch, stop, err := c.MetadataStream(ctx, "")
	if err != nil {
		t.Fatalf("MetadataStream: %v", err)
	}
	defer stop()
	select {
	case event := <-ch:
		if event.Revision != "11" {
			t.Fatalf("expected revision 11, got %q", event.Revision)
		}
	case <-time.After(time.Second):
		t.Fatalf("timeout waiting for metadata stream payload")
	}
}

func TestMetadataStreamReturnsAPIErrorOnNon2xx(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"metadata unavailable"}`))
	}))
	defer server.Close()

	c := &Client{baseURL: server.URL, token: "token"}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if _, _, err := c.MetadataStream(ctx, ""); err == nil {
		t.Fatalf("expected API error for non-2xx metadata stream response")
	}
}

func TestMetadataStreamSkipsMalformedPayloadAndContinues(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		_, _ = w.Write([]byte("data: {bad json}\n\n"))
		_, _ = w.Write([]byte(`data: {"version":"v1","type":"session.updated","revision":"12","session":{"id":"s1","title":"after"}}` + "\n\n"))
		if flusher != nil {
			flusher.Flush()
		}
	}))
	defer server.Close()

	c := &Client{baseURL: server.URL, token: "token"}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	ch, stop, err := c.MetadataStream(ctx, "")
	if err != nil {
		t.Fatalf("MetadataStream: %v", err)
	}
	defer stop()

	select {
	case event := <-ch:
		if event.Revision != "12" || event.Session == nil || event.Session.Title != "after" {
			t.Fatalf("expected valid event after malformed payload, got %#v", event)
		}
	case <-time.After(time.Second):
		t.Fatalf("timeout waiting for metadata stream payload")
	}
}

func TestMetadataStreamScannerErrorClosesChannel(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		// Larger than scanner max token (1 MiB) in client.MetadataStream.
		_, _ = w.Write([]byte("data: " + strings.Repeat("x", 1024*1024+8) + "\n\n"))
		if flusher != nil {
			flusher.Flush()
		}
	}))
	defer server.Close()

	c := &Client{baseURL: server.URL, token: "token"}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	ch, stop, err := c.MetadataStream(ctx, "")
	if err != nil {
		t.Fatalf("MetadataStream: %v", err)
	}
	defer stop()

	select {
	case _, ok := <-ch:
		if ok {
			t.Fatalf("expected channel to close on scanner error")
		}
	case <-time.After(time.Second):
		t.Fatalf("timeout waiting for metadata channel close")
	}
}

func TestMetadataStreamStopCancelsStream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		_, _ = w.Write([]byte(`data: {"version":"v1","type":"session.updated","revision":"1","session":{"id":"s1","title":"first"}}` + "\n\n"))
		if flusher != nil {
			flusher.Flush()
		}
		<-r.Context().Done()
	}))
	defer server.Close()

	c := &Client{baseURL: server.URL, token: "token"}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	ch, stop, err := c.MetadataStream(ctx, "")
	if err != nil {
		t.Fatalf("MetadataStream: %v", err)
	}

	select {
	case <-ch:
	case <-time.After(time.Second):
		t.Fatalf("timeout waiting for initial metadata event")
	}
	stop()
	select {
	case _, ok := <-ch:
		if ok {
			t.Fatalf("expected closed channel after stop")
		}
	case <-time.After(time.Second):
		t.Fatalf("timeout waiting for metadata stream shutdown")
	}
}
