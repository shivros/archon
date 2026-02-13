package daemon

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

func TestOpenCodeClientListPermissionsAndReply(t *testing.T) {
	var (
		replyPath string
		replyBody map[string]any
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/permission":
			writeJSON(w, http.StatusOK, []map[string]any{
				{
					"id":        "perm-1",
					"sessionID": "session-1",
					"status":    "pending",
					"type":      "command",
					"command":   "echo one",
					"createdAt": "2026-02-12T01:00:00Z",
				},
				{
					"id":        "perm-2",
					"sessionID": "session-2",
					"status":    "pending",
					"type":      "file",
					"reason":    "write file",
					"createdAt": "2026-02-12T01:01:00Z",
				},
			})
			return
		case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/permission/"):
			replyPath = r.URL.Path
			_ = json.NewDecoder(r.Body).Decode(&replyBody)
			writeJSON(w, http.StatusOK, map[string]any{"ok": true})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer server.Close()

	client, err := newOpenCodeClient(openCodeClientConfig{
		BaseURL: server.URL,
	})
	if err != nil {
		t.Fatalf("newOpenCodeClient: %v", err)
	}
	permissions, err := client.ListPermissions(context.Background(), "session-1")
	if err != nil {
		t.Fatalf("ListPermissions: %v", err)
	}
	if len(permissions) != 1 || permissions[0].PermissionID != "perm-1" {
		t.Fatalf("unexpected permissions: %#v", permissions)
	}
	if err := client.ReplyPermission(context.Background(), "", "perm-1", "approve"); err != nil {
		t.Fatalf("ReplyPermission: %v", err)
	}
	if replyPath != "/permission/perm-1/reply" {
		t.Fatalf("unexpected reply path: %q", replyPath)
	}
	if replyBody["decision"] != "approve" {
		t.Fatalf("unexpected reply body: %#v", replyBody)
	}
}

func TestOpenCodeClientReplyPermissionUsesSessionEndpoint(t *testing.T) {
	var (
		replyPath string
		replyBody map[string]any
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		replyPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&replyBody)
		writeJSON(w, http.StatusOK, true)
	}))
	defer server.Close()

	client, err := newOpenCodeClient(openCodeClientConfig{BaseURL: server.URL})
	if err != nil {
		t.Fatalf("newOpenCodeClient: %v", err)
	}
	if err := client.ReplyPermission(context.Background(), "session-1", "perm-1", "accept"); err != nil {
		t.Fatalf("ReplyPermission: %v", err)
	}
	if replyPath != "/session/session-1/permissions/perm-1" {
		t.Fatalf("unexpected reply path: %q", replyPath)
	}
	if got := strings.TrimSpace(asString(replyBody["response"])); got != "once" {
		t.Fatalf("unexpected response payload: %#v", replyBody)
	}
}

func TestOpenCodeClientSubscribeSessionEvents(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/event" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatalf("expected flusher")
		}
		send := func(payload map[string]any) {
			data, _ := json.Marshal(payload)
			_, _ = fmt.Fprintf(w, "data: %s\n\n", string(data))
			flusher.Flush()
		}

		send(map[string]any{
			"type": "session.status",
			"properties": map[string]any{
				"sessionID": "sess_1",
				"status": map[string]any{
					"type": "busy",
				},
			},
		})
		send(map[string]any{
			"type": "message.part.updated",
			"properties": map[string]any{
				"part": map[string]any{
					"type":      "text",
					"sessionID": "sess_1",
					"text":      "hello",
				},
				"delta": "hello",
			},
		})
		send(map[string]any{
			"type": "permission.updated",
			"properties": map[string]any{
				"id":        "perm-1",
				"sessionID": "sess_1",
				"type":      "command",
				"title":     "run command",
				"metadata": map[string]any{
					"command": "echo one",
				},
			},
		})
		send(map[string]any{
			"type": "permission.replied",
			"properties": map[string]any{
				"sessionID":    "sess_1",
				"permissionID": "perm-1",
				"response":     "once",
			},
		})
		send(map[string]any{
			"type": "session.idle",
			"properties": map[string]any{
				"sessionID": "sess_1",
			},
		})
		_, _ = io.WriteString(w, "\n")
		flusher.Flush()
	}))
	defer server.Close()

	client, err := newOpenCodeClient(openCodeClientConfig{BaseURL: server.URL})
	if err != nil {
		t.Fatalf("newOpenCodeClient: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	events, closeFn, err := client.SubscribeSessionEvents(ctx, "sess_1", "")
	if err != nil {
		t.Fatalf("SubscribeSessionEvents: %v", err)
	}
	defer closeFn()

	collected := make([]types.CodexEvent, 0, 8)
	timer := time.NewTimer(3 * time.Second)
	defer timer.Stop()
	for len(collected) < 5 {
		select {
		case event, ok := <-events:
			if !ok {
				t.Fatalf("event stream closed early")
			}
			collected = append(collected, event)
		case <-timer.C:
			t.Fatalf("timeout waiting for mapped events, got=%d", len(collected))
		}
	}

	methods := make([]string, 0, len(collected))
	for _, event := range collected {
		methods = append(methods, event.Method)
	}
	wantMethods := []string{
		"turn/started",
		"item/agentMessage/delta",
		"item/commandExecution/requestApproval",
		"permission/replied",
		"turn/completed",
	}
	for _, method := range wantMethods {
		found := false
		for _, got := range methods {
			if got == method {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected mapped method %q in %v", method, methods)
		}
	}

	approvalFound := false
	for _, event := range collected {
		if event.Method != "item/commandExecution/requestApproval" {
			continue
		}
		if event.ID == nil || *event.ID <= 0 {
			t.Fatalf("expected approval request id on mapped event: %#v", event)
		}
		payload := map[string]any{}
		if err := json.Unmarshal(event.Params, &payload); err != nil {
			t.Fatalf("decode approval payload: %v", err)
		}
		if strings.TrimSpace(asString(payload["permission_id"])) != "perm-1" {
			t.Fatalf("unexpected mapped permission payload: %#v", payload)
		}
		approvalFound = true
	}
	if !approvalFound {
		t.Fatalf("expected mapped approval event in stream")
	}
}

func TestOpenCodeClientSubscribeSessionEventsRejectsNonSSE(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer server.Close()

	client, err := newOpenCodeClient(openCodeClientConfig{BaseURL: server.URL})
	if err != nil {
		t.Fatalf("newOpenCodeClient: %v", err)
	}
	_, _, err = client.SubscribeSessionEvents(context.Background(), "sess_1", "")
	if err == nil {
		t.Fatalf("expected subscribe error for non-200 response")
	}
	var reqErr *openCodeRequestError
	if !errors.As(err, &reqErr) {
		t.Fatalf("expected *openCodeRequestError, got %T (%v)", err, err)
	}
	if reqErr.StatusCode != http.StatusNotFound {
		t.Fatalf("unexpected status code: %d", reqErr.StatusCode)
	}
	if !strings.Contains(strings.ToLower(reqErr.Error()), "not found") {
		t.Fatalf("unexpected subscribe error: %v", err)
	}
}
