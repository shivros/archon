package daemon

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestOpenCodeLiveSessionStartTurnAcceptsPromptPending(t *testing.T) {
	const providerSessionID = "remote-live-pending"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/session/"+providerSessionID+"/message" {
			http.NotFound(w, r)
			return
		}
		switch r.Method {
		case http.MethodGet:
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode([]map[string]any{})
		case http.MethodPost:
			if got := strings.TrimSpace(r.URL.Query().Get("directory")); got != "/tmp/live-pending" {
				http.Error(w, "missing directory", http.StatusBadRequest)
				return
			}
			// Exceed client timeout so prompt service returns errOpenCodePromptPending.
			time.Sleep(80 * time.Millisecond)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{})
		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}))
	defer server.Close()

	client, err := newOpenCodeClient(openCodeClientConfig{
		BaseURL:  server.URL,
		Username: "opencode",
		Timeout:  10 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("newOpenCodeClient: %v", err)
	}

	ls := &openCodeLiveSession{
		sessionID:    "s-live-pending",
		providerName: "opencode",
		providerID:   providerSessionID,
		directory:    "/tmp/live-pending",
		client:       client,
	}

	turnID, err := ls.StartTurn(context.Background(), []map[string]any{{"type": "text", "text": "hello"}}, nil)
	if err != nil {
		t.Fatalf("StartTurn: %v", err)
	}
	if strings.TrimSpace(turnID) == "" {
		t.Fatalf("expected non-empty turn id")
	}
	if got := strings.TrimSpace(ls.ActiveTurnID()); got != strings.TrimSpace(turnID) {
		t.Fatalf("expected active turn %q, got %q", turnID, got)
	}
}
