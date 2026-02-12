package daemon

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"control/internal/types"
)

func TestNewOpenCodeClientValidation(t *testing.T) {
	if _, err := newOpenCodeClient(openCodeClientConfig{}); err == nil {
		t.Fatalf("expected missing base_url error")
	}
	if _, err := newOpenCodeClient(openCodeClientConfig{BaseURL: "not-a-url"}); err == nil {
		t.Fatalf("expected invalid base_url error")
	}
}

func TestOpenCodeClientCreatePromptAbortAndModels(t *testing.T) {
	const username = "archon"
	const token = "secret"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		want := "Basic " + base64.StdEncoding.EncodeToString([]byte(username+":"+token))
		if auth != want {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/session":
			writeJSON(w, http.StatusCreated, map[string]any{"id": "sess_1"})
			return
		case r.Method == http.MethodPost && r.URL.Path == "/session/sess_1/prompt":
			var payload map[string]any
			_ = json.NewDecoder(r.Body).Decode(&payload)
			model, _ := payload["model"].(map[string]any)
			if strings.TrimSpace(asString(model["providerID"])) != "anthropic" {
				http.Error(w, "missing providerID", http.StatusBadRequest)
				return
			}
			if strings.TrimSpace(asString(model["modelID"])) != "claude-sonnet-4-20250514" {
				http.Error(w, "missing modelID", http.StatusBadRequest)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{
				"parts": []map[string]any{
					{"type": "text", "text": "hello from server"},
				},
			})
			return
		case r.Method == http.MethodPost && r.URL.Path == "/session/sess_1/abort":
			writeJSON(w, http.StatusOK, map[string]any{"ok": true})
			return
		case r.Method == http.MethodGet && r.URL.Path == "/config/providers":
			writeJSON(w, http.StatusOK, map[string]any{
				"providers": []map[string]any{
					{
						"id": "anthropic",
						"models": []map[string]any{
							{"id": "claude-sonnet-4-20250514"},
							{"id": "claude-opus-4-20250514"},
						},
					},
					{
						"id": "openai",
						"models": []any{
							"gpt-5",
						},
					},
				},
				"default": map[string]any{
					"anthropic": "claude-sonnet-4-20250514",
				},
			})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer server.Close()

	client, err := newOpenCodeClient(openCodeClientConfig{
		BaseURL:  server.URL,
		Username: username,
		Token:    token,
		Timeout:  2 * time.Second,
	})
	if err != nil {
		t.Fatalf("newOpenCodeClient: %v", err)
	}

	sessionID, err := client.CreateSession(context.Background(), "demo")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if sessionID != "sess_1" {
		t.Fatalf("unexpected session id: %q", sessionID)
	}

	reply, err := client.Prompt(context.Background(), "sess_1", "hello", &types.SessionRuntimeOptions{
		Model: "anthropic/claude-sonnet-4-20250514",
	})
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	if reply != "hello from server" {
		t.Fatalf("unexpected prompt reply: %q", reply)
	}

	if err := client.AbortSession(context.Background(), "sess_1"); err != nil {
		t.Fatalf("AbortSession: %v", err)
	}

	catalog, err := client.ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if catalog == nil || len(catalog.Models) != 3 {
		t.Fatalf("unexpected model catalog: %#v", catalog)
	}
	if catalog.DefaultModel != "anthropic/claude-sonnet-4-20250514" {
		t.Fatalf("unexpected default model: %q", catalog.DefaultModel)
	}
}
