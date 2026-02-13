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
	"sync/atomic"
	"testing"
	"time"

	"control/internal/types"
)

type openCodeRuntimeModelResolverFunc func(ctx context.Context, runtimeOptions *types.SessionRuntimeOptions) map[string]string

func (f openCodeRuntimeModelResolverFunc) Resolve(ctx context.Context, runtimeOptions *types.SessionRuntimeOptions) map[string]string {
	return f(ctx, runtimeOptions)
}

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
	const directory = "/tmp/opencode-worktree"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth := r.Header.Get("Authorization")
		want := "Basic " + base64.StdEncoding.EncodeToString([]byte(username+":"+token))
		if auth != want {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/session":
			if got := strings.TrimSpace(r.URL.Query().Get("directory")); got != directory {
				http.Error(w, "missing directory", http.StatusBadRequest)
				return
			}
			writeJSON(w, http.StatusCreated, map[string]any{"id": "sess_1"})
			return
		case r.Method == http.MethodPost && r.URL.Path == "/session/sess_1/prompt":
			if got := strings.TrimSpace(r.URL.Query().Get("directory")); got != directory {
				http.Error(w, "missing directory", http.StatusBadRequest)
				return
			}
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
			if got := strings.TrimSpace(r.URL.Query().Get("directory")); got != directory {
				http.Error(w, "missing directory", http.StatusBadRequest)
				return
			}
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

	sessionID, err := client.CreateSession(context.Background(), "demo", directory)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if sessionID != "sess_1" {
		t.Fatalf("unexpected session id: %q", sessionID)
	}

	reply, err := client.Prompt(context.Background(), "sess_1", "hello", &types.SessionRuntimeOptions{
		Model: "anthropic/claude-sonnet-4-20250514",
	}, directory)
	if err != nil {
		t.Fatalf("Prompt: %v", err)
	}
	if reply != "hello from server" {
		t.Fatalf("unexpected prompt reply: %q", reply)
	}

	if err := client.AbortSession(context.Background(), "sess_1", directory); err != nil {
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
		replyPath     string
		replyBody     map[string]any
		replyRawQuery string
	)
	const directory = "/tmp/opencode-worktree"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/permission":
			if got := strings.TrimSpace(r.URL.Query().Get("sessionID")); got != "session-1" {
				http.Error(w, "missing sessionID", http.StatusBadRequest)
				return
			}
			if got := strings.TrimSpace(r.URL.Query().Get("directory")); got != directory {
				http.Error(w, "missing directory", http.StatusBadRequest)
				return
			}
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
			replyRawQuery = r.URL.RawQuery
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
	permissions, err := client.ListPermissions(context.Background(), "session-1", directory)
	if err != nil {
		t.Fatalf("ListPermissions: %v", err)
	}
	if len(permissions) != 1 || permissions[0].PermissionID != "perm-1" {
		t.Fatalf("unexpected permissions: %#v", permissions)
	}
	if err := client.ReplyPermission(context.Background(), "", "perm-1", "approve", directory); err != nil {
		t.Fatalf("ReplyPermission: %v", err)
	}
	if replyPath != "/permission/perm-1/reply" {
		t.Fatalf("unexpected reply path: %q", replyPath)
	}
	if got := strings.TrimSpace(replyRawQuery); got != "directory=%2Ftmp%2Fopencode-worktree" {
		t.Fatalf("unexpected reply query: %q", got)
	}
	if replyBody["decision"] != "approve" {
		t.Fatalf("unexpected reply body: %#v", replyBody)
	}
}

func TestOpenCodeClientListModelsSupportsMappedModelShape(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/config/providers" {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"providers": []map[string]any{
				{
					"id": "openai",
					"models": map[string]any{
						"gpt-5": map[string]any{
							"id": "gpt-5",
						},
						"gpt-5-codex": map[string]any{
							"name": "GPT-5 Codex",
						},
					},
				},
			},
			"default": map[string]any{
				"openai": "gpt-5",
			},
		})
	}))
	defer server.Close()

	client, err := newOpenCodeClient(openCodeClientConfig{BaseURL: server.URL})
	if err != nil {
		t.Fatalf("newOpenCodeClient: %v", err)
	}
	catalog, err := client.ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if catalog == nil {
		t.Fatalf("expected model catalog")
	}
	if len(catalog.Models) != 2 {
		t.Fatalf("expected two mapped models, got %#v", catalog.Models)
	}
	if catalog.Models[0] != "openai/gpt-5" {
		t.Fatalf("expected default model first, got %#v", catalog.Models)
	}
	if catalog.DefaultModel != "openai/gpt-5" {
		t.Fatalf("unexpected default model: %q", catalog.DefaultModel)
	}
}

func TestOpenCodeClientListModelsPrefixesProviderForSlashModelIDs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/config/providers" {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"providers": []map[string]any{
				{
					"id": "openrouter",
					"models": []map[string]any{
						{"id": "google/gemini-2.5-flash"},
					},
				},
			},
			"default": map[string]any{
				"openrouter": "google/gemini-2.5-flash",
			},
		})
	}))
	defer server.Close()

	client, err := newOpenCodeClient(openCodeClientConfig{BaseURL: server.URL})
	if err != nil {
		t.Fatalf("newOpenCodeClient: %v", err)
	}
	catalog, err := client.ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if catalog == nil {
		t.Fatalf("expected model catalog")
	}
	if len(catalog.Models) != 1 {
		t.Fatalf("expected one model, got %#v", catalog.Models)
	}
	if got := strings.TrimSpace(catalog.Models[0]); got != "openrouter/google/gemini-2.5-flash" {
		t.Fatalf("unexpected normalized model id: %q", got)
	}
	if got := strings.TrimSpace(catalog.DefaultModel); got != "openrouter/google/gemini-2.5-flash" {
		t.Fatalf("unexpected default model: %q", got)
	}
}

func TestOpenCodeClientCreateSessionFallsBackWhenCreateReturnsEOF(t *testing.T) {
	const directory = "/tmp/opencode-worktree"
	const title = "archon test title"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/session":
			if got := strings.TrimSpace(r.URL.Query().Get("directory")); got != directory {
				http.Error(w, "missing directory", http.StatusBadRequest)
				return
			}
			// Compatibility case: some server builds may return 200 with an empty body.
			w.WriteHeader(http.StatusOK)
			return
		case r.Method == http.MethodGet && r.URL.Path == "/session":
			if got := strings.TrimSpace(r.URL.Query().Get("directory")); got != directory {
				http.Error(w, "missing directory", http.StatusBadRequest)
				return
			}
			if got := strings.TrimSpace(r.URL.Query().Get("search")); got != title {
				http.Error(w, "missing search", http.StatusBadRequest)
				return
			}
			writeJSON(w, http.StatusOK, []map[string]any{
				{
					"id":    "sess_fallback_1",
					"title": title,
				},
			})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer server.Close()

	client, err := newOpenCodeClient(openCodeClientConfig{BaseURL: server.URL})
	if err != nil {
		t.Fatalf("newOpenCodeClient: %v", err)
	}
	sessionID, err := client.CreateSession(context.Background(), title, directory)
	if err != nil {
		t.Fatalf("CreateSession fallback: %v", err)
	}
	if sessionID != "sess_fallback_1" {
		t.Fatalf("unexpected fallback session id: %q", sessionID)
	}
}

func TestOpenCodeClientCreateSessionFallbackWithoutTitleSelectsRecentSession(t *testing.T) {
	const directory = "/tmp/opencode-worktree"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/session":
			if got := strings.TrimSpace(r.URL.Query().Get("directory")); got != directory {
				http.Error(w, "missing directory", http.StatusBadRequest)
				return
			}
			w.WriteHeader(http.StatusOK)
			return
		case r.Method == http.MethodGet && r.URL.Path == "/session":
			if got := strings.TrimSpace(r.URL.Query().Get("directory")); got != directory {
				http.Error(w, "missing directory", http.StatusBadRequest)
				return
			}
			if got := strings.TrimSpace(r.URL.Query().Get("search")); got != "" {
				http.Error(w, "unexpected search", http.StatusBadRequest)
				return
			}
			nowMs := time.Now().UnixMilli()
			writeJSON(w, http.StatusOK, []map[string]any{
				{
					"id": "sess_old",
					"time": map[string]any{
						"created": nowMs - 10*60*1000,
					},
				},
				{
					"id": "sess_new",
					"time": map[string]any{
						"created": nowMs,
					},
				},
			})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer server.Close()

	client, err := newOpenCodeClient(openCodeClientConfig{BaseURL: server.URL})
	if err != nil {
		t.Fatalf("newOpenCodeClient: %v", err)
	}
	sessionID, err := client.CreateSession(context.Background(), "", directory)
	if err != nil {
		t.Fatalf("CreateSession fallback (empty title): %v", err)
	}
	if sessionID != "sess_new" {
		t.Fatalf("unexpected fallback session id: %q", sessionID)
	}
}

func TestOpenCodeClientCreateSessionFallbackWithoutTitleDoesNotReuseStaleSession(t *testing.T) {
	const directory = "/tmp/opencode-worktree"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/session":
			if got := strings.TrimSpace(r.URL.Query().Get("directory")); got != directory {
				http.Error(w, "missing directory", http.StatusBadRequest)
				return
			}
			w.WriteHeader(http.StatusOK)
			return
		case r.Method == http.MethodGet && r.URL.Path == "/session":
			if got := strings.TrimSpace(r.URL.Query().Get("directory")); got != directory {
				http.Error(w, "missing directory", http.StatusBadRequest)
				return
			}
			oldMs := time.Now().Add(-10 * time.Minute).UnixMilli()
			writeJSON(w, http.StatusOK, []map[string]any{
				{
					"id": "sess_old_only",
					"time": map[string]any{
						"created": oldMs,
					},
				},
			})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer server.Close()

	client, err := newOpenCodeClient(openCodeClientConfig{BaseURL: server.URL})
	if err != nil {
		t.Fatalf("newOpenCodeClient: %v", err)
	}
	if _, err := client.CreateSession(context.Background(), "", directory); err == nil {
		t.Fatalf("expected create fallback error for stale-only sessions")
	}
}

func TestOpenCodeClientPromptAllowsEOFResponse(t *testing.T) {
	const directory = "/tmp/opencode-worktree"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/session/sess_1/message":
			if got := strings.TrimSpace(r.URL.Query().Get("directory")); got != directory {
				http.Error(w, "missing directory", http.StatusBadRequest)
				return
			}
			// Compatibility case: successful response with empty body.
			w.WriteHeader(http.StatusOK)
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer server.Close()

	client, err := newOpenCodeClient(openCodeClientConfig{BaseURL: server.URL})
	if err != nil {
		t.Fatalf("newOpenCodeClient: %v", err)
	}
	reply, err := client.Prompt(context.Background(), "sess_1", "hello", nil, directory)
	if err != nil {
		t.Fatalf("Prompt EOF response: %v", err)
	}
	if strings.TrimSpace(reply) != "" {
		t.Fatalf("expected empty reply, got %q", reply)
	}
}

func TestOpenCodeClientPromptRecoversAssistantAfterEOF(t *testing.T) {
	const directory = "/tmp/opencode-worktree"
	var listCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/session/sess_1/message":
			if got := strings.TrimSpace(r.URL.Query().Get("directory")); got != directory {
				http.Error(w, "missing directory", http.StatusBadRequest)
				return
			}
			// Compatibility case: successful response with empty body.
			w.WriteHeader(http.StatusOK)
			return
		case r.Method == http.MethodGet && r.URL.Path == "/session/sess_1/message":
			if got := strings.TrimSpace(r.URL.Query().Get("directory")); got != directory {
				http.Error(w, "missing directory", http.StatusBadRequest)
				return
			}
			call := listCalls.Add(1)
			if call <= 1 {
				writeJSON(w, http.StatusOK, []map[string]any{
					{
						"info": map[string]any{
							"id":        "assistant-old",
							"role":      "assistant",
							"createdAt": "2026-02-12T01:00:00Z",
						},
						"parts": []map[string]any{
							{"type": "text", "text": "old reply"},
						},
					},
				})
				return
			}
			writeJSON(w, http.StatusOK, []map[string]any{
				{
					"info": map[string]any{
						"id":        "assistant-old",
						"role":      "assistant",
						"createdAt": "2026-02-12T01:00:00Z",
					},
					"parts": []map[string]any{
						{"type": "text", "text": "old reply"},
					},
				},
				{
					"info": map[string]any{
						"id":        "assistant-new",
						"role":      "assistant",
						"createdAt": "2026-02-12T01:00:05Z",
					},
					"parts": []map[string]any{
						{"type": "text", "text": "fresh reply"},
					},
				},
			})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer server.Close()

	client, err := newOpenCodeClient(openCodeClientConfig{BaseURL: server.URL})
	if err != nil {
		t.Fatalf("newOpenCodeClient: %v", err)
	}
	reply, err := client.Prompt(context.Background(), "sess_1", "hello", nil, directory)
	if err != nil {
		t.Fatalf("Prompt EOF recovery: %v", err)
	}
	if strings.TrimSpace(reply) != "fresh reply" {
		t.Fatalf("expected recovered assistant text, got %q", reply)
	}
}

func TestOpenCodeClientPromptRecoversAssistantAfterRequestTimeout(t *testing.T) {
	const directory = "/tmp/opencode-worktree"
	var promptStarted atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/session/sess_1/message":
			if got := strings.TrimSpace(r.URL.Query().Get("directory")); got != directory {
				http.Error(w, "missing directory", http.StatusBadRequest)
				return
			}
			promptStarted.Store(true)
			time.Sleep(200 * time.Millisecond)
			writeJSON(w, http.StatusOK, map[string]any{
				"parts": []map[string]any{
					{"type": "text", "text": "fresh reply"},
				},
			})
			return
		case r.Method == http.MethodGet && r.URL.Path == "/session/sess_1/message":
			if got := strings.TrimSpace(r.URL.Query().Get("directory")); got != directory {
				http.Error(w, "missing directory", http.StatusBadRequest)
				return
			}
			if !promptStarted.Load() {
				writeJSON(w, http.StatusOK, []map[string]any{
					{
						"info": map[string]any{
							"id":        "assistant-old",
							"role":      "assistant",
							"createdAt": "2026-02-12T01:00:00Z",
						},
						"parts": []map[string]any{
							{"type": "text", "text": "old reply"},
						},
					},
				})
				return
			}
			writeJSON(w, http.StatusOK, []map[string]any{
				{
					"info": map[string]any{
						"id":        "assistant-old",
						"role":      "assistant",
						"createdAt": "2026-02-12T01:00:00Z",
					},
					"parts": []map[string]any{
						{"type": "text", "text": "old reply"},
					},
				},
				{
					"info": map[string]any{
						"id":        "assistant-new",
						"role":      "assistant",
						"createdAt": "2026-02-12T01:00:05Z",
					},
					"parts": []map[string]any{
						{"type": "text", "text": "fresh reply"},
					},
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
		BaseURL: server.URL,
		Timeout: 50 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("newOpenCodeClient: %v", err)
	}
	reply, err := client.Prompt(context.Background(), "sess_1", "hello", nil, directory)
	if err != nil {
		t.Fatalf("Prompt timeout recovery: %v", err)
	}
	if strings.TrimSpace(reply) != "fresh reply" {
		t.Fatalf("expected recovered assistant text, got %q", reply)
	}
}

func TestOpenCodeClientPromptParsesMessageContentShape(t *testing.T) {
	const directory = "/tmp/opencode-worktree"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/session/sess_1/message":
			writeJSON(w, http.StatusOK, []map[string]any{})
			return
		case r.Method == http.MethodPost && r.URL.Path == "/session/sess_1/message":
			writeJSON(w, http.StatusOK, map[string]any{
				"message": map[string]any{
					"role": "assistant",
					"content": []map[string]any{
						{"type": "text", "text": "reply from message content"},
					},
				},
			})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer server.Close()

	client, err := newOpenCodeClient(openCodeClientConfig{BaseURL: server.URL})
	if err != nil {
		t.Fatalf("newOpenCodeClient: %v", err)
	}
	reply, err := client.Prompt(context.Background(), "sess_1", "hello", nil, directory)
	if err != nil {
		t.Fatalf("Prompt message-content shape: %v", err)
	}
	if strings.TrimSpace(reply) != "reply from message content" {
		t.Fatalf("unexpected prompt reply: %q", reply)
	}
}

func TestOpenCodeClientPromptResolvesLegacyModelValueToProvider(t *testing.T) {
	const directory = "/tmp/opencode-worktree"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/config/providers":
			writeJSON(w, http.StatusOK, map[string]any{
				"providers": []map[string]any{
					{
						"id": "openrouter",
						"models": []map[string]any{
							{"id": "google/gemini-2.5-flash"},
						},
					},
				},
			})
			return
		case r.Method == http.MethodGet && r.URL.Path == "/session/sess_1/message":
			writeJSON(w, http.StatusOK, []map[string]any{})
			return
		case r.Method == http.MethodPost && r.URL.Path == "/session/sess_1/message":
			var payload map[string]any
			_ = json.NewDecoder(r.Body).Decode(&payload)
			model, _ := payload["model"].(map[string]any)
			if got := strings.TrimSpace(asString(model["providerID"])); got != "openrouter" {
				http.Error(w, "unexpected providerID: "+got, http.StatusBadRequest)
				return
			}
			if got := strings.TrimSpace(asString(model["modelID"])); got != "google/gemini-2.5-flash" {
				http.Error(w, "unexpected modelID: "+got, http.StatusBadRequest)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{
				"parts": []map[string]any{
					{"type": "text", "text": "resolved"},
				},
			})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer server.Close()

	client, err := newOpenCodeClient(openCodeClientConfig{BaseURL: server.URL})
	if err != nil {
		t.Fatalf("newOpenCodeClient: %v", err)
	}
	reply, err := client.Prompt(context.Background(), "sess_1", "hello", &types.SessionRuntimeOptions{
		Model: "google/gemini-2.5-flash",
	}, directory)
	if err != nil {
		t.Fatalf("Prompt legacy model resolution: %v", err)
	}
	if strings.TrimSpace(reply) != "resolved" {
		t.Fatalf("unexpected prompt reply: %q", reply)
	}
}

func TestOpenCodeClientPromptFallsBackToRawModelWhenProviderCatalogUnavailable(t *testing.T) {
	const directory = "/tmp/opencode-worktree"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/config/providers":
			http.NotFound(w, r)
			return
		case r.Method == http.MethodGet && r.URL.Path == "/session/sess_1/message":
			writeJSON(w, http.StatusOK, []map[string]any{})
			return
		case r.Method == http.MethodPost && r.URL.Path == "/session/sess_1/message":
			var payload map[string]any
			_ = json.NewDecoder(r.Body).Decode(&payload)
			model, _ := payload["model"].(map[string]any)
			if got := strings.TrimSpace(asString(model["providerID"])); got != "" {
				http.Error(w, "unexpected providerID: "+got, http.StatusBadRequest)
				return
			}
			if got := strings.TrimSpace(asString(model["modelID"])); got != "google/gemini-2.5-flash" {
				http.Error(w, "unexpected modelID: "+got, http.StatusBadRequest)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{
				"parts": []map[string]any{
					{"type": "text", "text": "fallback-raw-model"},
				},
			})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer server.Close()

	client, err := newOpenCodeClient(openCodeClientConfig{BaseURL: server.URL})
	if err != nil {
		t.Fatalf("newOpenCodeClient: %v", err)
	}
	reply, err := client.Prompt(context.Background(), "sess_1", "hello", &types.SessionRuntimeOptions{
		Model: "google/gemini-2.5-flash",
	}, directory)
	if err != nil {
		t.Fatalf("Prompt raw model fallback: %v", err)
	}
	if strings.TrimSpace(reply) != "fallback-raw-model" {
		t.Fatalf("unexpected prompt reply: %q", reply)
	}
}

func TestOpenCodeClientPromptUsesInjectedRuntimeModelResolver(t *testing.T) {
	const directory = "/tmp/opencode-worktree"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/session/sess_1/message":
			writeJSON(w, http.StatusOK, []map[string]any{})
			return
		case r.Method == http.MethodPost && r.URL.Path == "/session/sess_1/message":
			var payload map[string]any
			_ = json.NewDecoder(r.Body).Decode(&payload)
			model, _ := payload["model"].(map[string]any)
			if got := strings.TrimSpace(asString(model["providerID"])); got != "custom-provider" {
				http.Error(w, "unexpected providerID: "+got, http.StatusBadRequest)
				return
			}
			if got := strings.TrimSpace(asString(model["modelID"])); got != "custom/model" {
				http.Error(w, "unexpected modelID: "+got, http.StatusBadRequest)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{
				"parts": []map[string]any{
					{"type": "text", "text": "resolver-injected"},
				},
			})
			return
		default:
			http.NotFound(w, r)
			return
		}
	}))
	defer server.Close()

	client, err := newOpenCodeClient(openCodeClientConfig{BaseURL: server.URL})
	if err != nil {
		t.Fatalf("newOpenCodeClient: %v", err)
	}
	client.modelResolver = openCodeRuntimeModelResolverFunc(func(_ context.Context, _ *types.SessionRuntimeOptions) map[string]string {
		return map[string]string{
			"providerID": "custom-provider",
			"modelID":    "custom/model",
		}
	})

	reply, err := client.Prompt(context.Background(), "sess_1", "hello", &types.SessionRuntimeOptions{
		Model: "ignored/by/resolver",
	}, directory)
	if err != nil {
		t.Fatalf("Prompt injected resolver: %v", err)
	}
	if strings.TrimSpace(reply) != "resolver-injected" {
		t.Fatalf("unexpected prompt reply: %q", reply)
	}
}

func TestOpenCodeClientReplyPermissionUsesSessionEndpoint(t *testing.T) {
	var (
		replyPath     string
		replyBody     map[string]any
		replyRawQuery string
	)
	const directory = "/tmp/opencode-worktree"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		replyPath = r.URL.Path
		replyRawQuery = r.URL.RawQuery
		_ = json.NewDecoder(r.Body).Decode(&replyBody)
		writeJSON(w, http.StatusOK, true)
	}))
	defer server.Close()

	client, err := newOpenCodeClient(openCodeClientConfig{BaseURL: server.URL})
	if err != nil {
		t.Fatalf("newOpenCodeClient: %v", err)
	}
	if err := client.ReplyPermission(context.Background(), "session-1", "perm-1", "accept", directory); err != nil {
		t.Fatalf("ReplyPermission: %v", err)
	}
	if replyPath != "/session/session-1/permissions/perm-1" {
		t.Fatalf("unexpected reply path: %q", replyPath)
	}
	if got := strings.TrimSpace(replyRawQuery); got != "directory=%2Ftmp%2Fopencode-worktree" {
		t.Fatalf("unexpected reply query: %q", got)
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
