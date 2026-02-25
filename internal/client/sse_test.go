package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

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
