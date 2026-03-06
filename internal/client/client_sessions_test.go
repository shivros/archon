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

func TestClientGetTranscriptSnapshot(t *testing.T) {
	var seenPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenPath = r.URL.RequestURI()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"session_id":"s1","provider":"codex","revision":"2","blocks":[{"kind":"assistant","text":"hello"}],"turn_state":{"state":"idle"},"capabilities":{"supports_events":true}}`))
	}))
	defer server.Close()

	c := &Client{
		baseURL: server.URL,
		token:   "token",
		http: &http.Client{
			Timeout: 2 * time.Second,
		},
	}

	snapshot, err := c.GetTranscriptSnapshot(context.Background(), "s1", 150)
	if err != nil {
		t.Fatalf("GetTranscriptSnapshot error: %v", err)
	}
	if seenPath != "/v1/sessions/s1/transcript?lines=150" {
		t.Fatalf("unexpected request path: %s", seenPath)
	}
	if snapshot.SessionID != "s1" || snapshot.Revision.String() != "2" {
		t.Fatalf("unexpected snapshot: %#v", snapshot)
	}
}

func TestClientGetTranscriptSnapshotReturnsAPIErrorOnNon2xx(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
	}))
	defer server.Close()

	c := &Client{
		baseURL: server.URL,
		token:   "token",
		http: &http.Client{
			Timeout: 2 * time.Second,
		},
	}
	if _, err := c.GetTranscriptSnapshot(context.Background(), "s1", 150); err == nil {
		t.Fatalf("expected non-2xx error")
	}
}

func TestClientCloudAuthEndpoints(t *testing.T) {
	var calls []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/cloud-auth/device":
			_, _ = w.Write([]byte(`{"device_code":"dev-1","user_code":"ABCD-EFGH","verification_uri":"https://archon.example/activate","expires_in":600,"interval":5}`))
		case "/v1/cloud-auth/poll":
			_, _ = w.Write([]byte(`{"status":"approved","auth":{"linked":true}}`))
		case "/v1/cloud-auth/status":
			_, _ = w.Write([]byte(`{"linked":true}`))
		case "/v1/cloud-auth/logout":
			_, _ = w.Write([]byte(`{"status":"revoked_and_unlinked","remote_revoked":true,"local_cleared":true}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	c := &Client{
		baseURL: server.URL,
		token:   "token",
		http: &http.Client{
			Timeout: 2 * time.Second,
		},
	}

	started, err := c.StartCloudLogin(context.Background())
	if err != nil || started.DeviceCode != "dev-1" {
		t.Fatalf("StartCloudLogin error=%v resp=%#v", err, started)
	}
	polled, err := c.PollCloudLogin(context.Background())
	if err != nil || polled.Status != "approved" {
		t.Fatalf("PollCloudLogin error=%v resp=%#v", err, polled)
	}
	status, err := c.CloudAuthStatus(context.Background())
	if err != nil || !status.Linked {
		t.Fatalf("CloudAuthStatus error=%v resp=%#v", err, status)
	}
	logoutResp, err := c.LogoutCloud(context.Background())
	if err != nil {
		t.Fatalf("LogoutCloud error: %v", err)
	}
	if logoutResp.Status == "" {
		t.Fatalf("expected logout response payload, got %#v", logoutResp)
	}
	if len(calls) != 4 {
		t.Fatalf("expected four cloud auth calls, got %#v", calls)
	}
}

func TestClientCloudAuthLogoutReturnsAPIErrorOnNon2xx(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"error":"unauthorized"}`))
	}))
	defer server.Close()

	c := &Client{
		baseURL: server.URL,
		token:   "token",
		http: &http.Client{
			Timeout: 2 * time.Second,
		},
	}
	if _, err := c.LogoutCloud(context.Background()); err == nil {
		t.Fatalf("expected non-2xx error")
	}
}
