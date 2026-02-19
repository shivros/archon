package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestClientListSessionsWithMetaOptionsIncludesWorkflowOwned(t *testing.T) {
	var seenPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenPath = r.URL.RequestURI()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"sessions":[],"session_meta":[]}`))
	}))
	defer server.Close()

	c := &Client{
		baseURL: server.URL,
		token:   "token",
		http: &http.Client{
			Timeout: 2 * time.Second,
		},
	}

	_, _, err := c.ListSessionsWithMetaOptions(context.Background(), true, true)
	if err != nil {
		t.Fatalf("ListSessionsWithMetaOptions error: %v", err)
	}
	if seenPath != "/v1/sessions?include_dismissed=1&include_workflow_owned=1" && seenPath != "/v1/sessions?include_workflow_owned=1&include_dismissed=1" {
		t.Fatalf("unexpected request path: %s", seenPath)
	}
}

func TestClientListSessionsWithMetaRefreshWithOptionsIncludesWorkflowOwned(t *testing.T) {
	var seenPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenPath = r.URL.RequestURI()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"sessions":[],"session_meta":[]}`))
	}))
	defer server.Close()

	c := &Client{
		baseURL: server.URL,
		token:   "token",
		http: &http.Client{
			Timeout: 2 * time.Second,
		},
	}

	_, _, err := c.ListSessionsWithMetaRefreshWithOptions(context.Background(), "ws-1", false, true)
	if err != nil {
		t.Fatalf("ListSessionsWithMetaRefreshWithOptions error: %v", err)
	}
	if seenPath != "/v1/sessions?include_workflow_owned=1&refresh=1&workspace_id=ws-1" &&
		seenPath != "/v1/sessions?refresh=1&workspace_id=ws-1&include_workflow_owned=1" &&
		seenPath != "/v1/sessions?refresh=1&include_workflow_owned=1&workspace_id=ws-1" {
		t.Fatalf("unexpected request path: %s", seenPath)
	}
}
