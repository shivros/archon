package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSendMessageUsesExtendedTimeout(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/sessions/session-1/send" {
			http.NotFound(w, r)
			return
		}
		// Simulate a provider-backed send that takes longer than the default
		// API client timeout.
		time.Sleep(120 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	c := &Client{
		baseURL: server.URL,
		token:   "token",
		http: &http.Client{
			Timeout: 20 * time.Millisecond,
		},
	}

	resp, err := c.SendMessage(context.Background(), "session-1", SendSessionRequest{
		Text: "hello",
	})
	if err != nil {
		t.Fatalf("SendMessage should not use default 20ms timeout: %v", err)
	}
	if resp == nil || !resp.OK {
		t.Fatalf("unexpected send response: %#v", resp)
	}
}
