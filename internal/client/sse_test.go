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
