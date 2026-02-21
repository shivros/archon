package app

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"control/internal/client"
)

func TestClientAPIListSessionsWithMetaQueryUsesOptionsPathWhenNotRefreshing(t *testing.T) {
	t.Parallel()

	var seenQuery url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/sessions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		seenQuery = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"sessions":[{"id":"s1"}],"session_meta":[]}`))
	}))
	defer server.Close()

	raw := client.NewWithBaseURL(server.URL, "token")
	api := NewClientAPI(raw)

	sessions, meta, err := api.ListSessionsWithMetaQuery(context.Background(), SessionListQuery{
		Refresh:              false,
		WorkspaceID:          "ws-ignored",
		IncludeDismissed:     true,
		IncludeWorkflowOwned: true,
	})
	if err != nil {
		t.Fatalf("ListSessionsWithMetaQuery error: %v", err)
	}
	if len(sessions) != 1 || sessions[0].ID != "s1" {
		t.Fatalf("unexpected sessions: %#v", sessions)
	}
	if len(meta) != 0 {
		t.Fatalf("unexpected meta: %#v", meta)
	}
	if seenQuery.Get("include_dismissed") != "1" {
		t.Fatalf("expected include_dismissed=1, got %q", seenQuery.Get("include_dismissed"))
	}
	if seenQuery.Get("include_workflow_owned") != "1" {
		t.Fatalf("expected include_workflow_owned=1, got %q", seenQuery.Get("include_workflow_owned"))
	}
	if seenQuery.Get("refresh") != "" {
		t.Fatalf("did not expect refresh query for non-refresh path, got %q", seenQuery.Get("refresh"))
	}
	if seenQuery.Get("workspace_id") != "" {
		t.Fatalf("did not expect workspace_id query for non-refresh path, got %q", seenQuery.Get("workspace_id"))
	}
}

func TestClientAPIListSessionsWithMetaQueryUsesRefreshPathWhenRefreshing(t *testing.T) {
	t.Parallel()

	var seenQuery url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/sessions" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		seenQuery = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"sessions":[],"session_meta":[]}`))
	}))
	defer server.Close()

	raw := client.NewWithBaseURL(server.URL, "token")
	api := NewClientAPI(raw)

	_, _, err := api.ListSessionsWithMetaQuery(context.Background(), SessionListQuery{
		Refresh:              true,
		WorkspaceID:          "ws-1",
		IncludeDismissed:     true,
		IncludeWorkflowOwned: true,
	})
	if err != nil {
		t.Fatalf("ListSessionsWithMetaQuery error: %v", err)
	}
	if seenQuery.Get("refresh") != "1" {
		t.Fatalf("expected refresh=1, got %q", seenQuery.Get("refresh"))
	}
	if seenQuery.Get("workspace_id") != "ws-1" {
		t.Fatalf("expected workspace_id=ws-1, got %q", seenQuery.Get("workspace_id"))
	}
	if seenQuery.Get("include_dismissed") != "1" {
		t.Fatalf("expected include_dismissed=1, got %q", seenQuery.Get("include_dismissed"))
	}
	if seenQuery.Get("include_workflow_owned") != "1" {
		t.Fatalf("expected include_workflow_owned=1, got %q", seenQuery.Get("include_workflow_owned"))
	}
}

func TestClientAPIListSessionsWithMetaQueryPropagatesError(t *testing.T) {
	t.Parallel()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"boom"}`, http.StatusInternalServerError)
	}))
	defer server.Close()

	raw := client.NewWithBaseURL(server.URL, "token")
	api := NewClientAPI(raw)

	_, _, err := api.ListSessionsWithMetaQuery(context.Background(), SessionListQuery{
		IncludeDismissed: true,
	})
	if err == nil {
		t.Fatalf("expected error from ListSessionsWithMetaQuery")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "boom") {
		t.Fatalf("expected boom in error, got %v", err)
	}
}
