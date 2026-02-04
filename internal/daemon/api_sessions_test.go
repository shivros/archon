package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"control/internal/store"
	"control/internal/types"
)

type sessionsResponse struct {
	Sessions []*types.Session `json:"sessions"`
}

type itemsResponse struct {
	Items []map[string]any `json:"items"`
}

func TestAPISessionEndpoints(t *testing.T) {
	manager := newTestManager(t)
	server := newTestServer(t, manager)
	defer server.Close()

	startReq := StartSessionRequest{
		Provider: "custom",
		Cmd:      os.Args[0],
		Args:     helperArgs("stdout=api", "stderr=err", "sleep_ms=50", "exit=0"),
		Env:      []string{"GO_WANT_HELPER_PROCESS=1"},
		Title:    "api-test",
		Tags:     []string{"api"},
	}

	session := startSession(t, server, startReq)
	if session.ID == "" {
		t.Fatalf("expected session id")
	}

	list := listSessions(t, server)
	if len(list.Sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(list.Sessions))
	}

	got := getSession(t, server, session.ID)
	if got.ID != session.ID {
		t.Fatalf("expected session id %s, got %s", session.ID, got.ID)
	}

	waitForStatus(t, manager, session.ID, types.SessionStatusExited, 2*time.Second)

	tail := tailSession(t, server, session.ID)
	if len(tail.Items) == 0 {
		t.Fatalf("expected tail items")
	}

	history := historySession(t, server, session.ID)
	if len(history.Items) == 0 {
		t.Fatalf("expected history items")
	}

	killSession(t, server, session.ID)
}

func TestAPISessionSendUnsupported(t *testing.T) {
	manager := newTestManager(t)
	server := newTestServer(t, manager)
	defer server.Close()

	startReq := StartSessionRequest{
		Provider: "custom",
		Cmd:      os.Args[0],
		Args:     helperArgs("stdout=api", "stderr=err", "sleep_ms=50", "exit=0"),
		Env:      []string{"GO_WANT_HELPER_PROCESS=1"},
		Title:    "api-test",
	}

	session := startSession(t, server, startReq)
	if session.ID == "" {
		t.Fatalf("expected session id")
	}

	code := sendMessageStatus(t, server, session.ID, "hello")
	if code != http.StatusBadRequest {
		t.Fatalf("expected 400 for unsupported provider, got %d", code)
	}
}

func TestAPISessionExitHidesFromList(t *testing.T) {
	store := storeSessionsIndex(t)
	now := time.Now().UTC()
	_, err := store.UpsertRecord(context.Background(), &types.SessionRecord{
		Session: &types.Session{
			ID:        "sess-exit",
			Provider:  "codex",
			Cmd:       "codex app-server",
			Status:    types.SessionStatusInactive,
			CreatedAt: now,
		},
		Source: "codex",
	})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}

	server := newTestServerWithStores(t, nil, &Stores{Sessions: store})
	defer server.Close()

	list := listSessions(t, server)
	if len(list.Sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(list.Sessions))
	}

	markExited(t, server, "sess-exit")

	got := getSession(t, server, "sess-exit")
	if got.Status != types.SessionStatusExited {
		t.Fatalf("expected exited status, got %s", got.Status)
	}

	list = listSessions(t, server)
	if len(list.Sessions) != 0 {
		t.Fatalf("expected 0 sessions after exit, got %d", len(list.Sessions))
	}
}

func TestAPISessionListOrder(t *testing.T) {
	store := storeSessionsIndex(t)
	older := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	newer := older.Add(2 * time.Hour)
	_, err := store.UpsertRecord(context.Background(), &types.SessionRecord{
		Session: &types.Session{
			ID:        "sess-old",
			Provider:  "codex",
			Cmd:       "codex app-server",
			Status:    types.SessionStatusInactive,
			CreatedAt: older,
		},
		Source: "codex",
	})
	if err != nil {
		t.Fatalf("upsert old: %v", err)
	}
	_, err = store.UpsertRecord(context.Background(), &types.SessionRecord{
		Session: &types.Session{
			ID:        "sess-new",
			Provider:  "codex",
			Cmd:       "codex app-server",
			Status:    types.SessionStatusInactive,
			CreatedAt: newer,
		},
		Source: "codex",
	})
	if err != nil {
		t.Fatalf("upsert new: %v", err)
	}

	server := newTestServerWithStores(t, nil, &Stores{Sessions: store})
	defer server.Close()

	list := listSessions(t, server)
	if len(list.Sessions) != 2 {
		t.Fatalf("expected 2 sessions, got %d", len(list.Sessions))
	}
	if list.Sessions[0].ID != "sess-new" {
		t.Fatalf("expected newest session first, got %s", list.Sessions[0].ID)
	}
}

func newTestServer(t *testing.T, manager *SessionManager) *httptest.Server {
	return newTestServerWithStores(t, manager, nil)
}

func newTestServerWithStores(t *testing.T, manager *SessionManager, stores *Stores) *httptest.Server {
	t.Helper()
	api := &API{Version: "test", Manager: manager, Stores: stores}
	mux := http.NewServeMux()
	mux.HandleFunc("/health", api.Health)
	mux.HandleFunc("/v1/sessions", api.Sessions)
	mux.HandleFunc("/v1/sessions/", api.SessionByID)
	return httptest.NewServer(TokenAuthMiddleware("token", mux))
}

func startSession(t *testing.T, server *httptest.Server, req StartSessionRequest) *types.Session {
	t.Helper()
	body, _ := json.Marshal(req)
	httpReq, _ := http.NewRequest(http.MethodPost, server.URL+"/v1/sessions", bytes.NewReader(body))
	httpReq.Header.Set("Authorization", "Bearer token")
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		t.Fatalf("start session: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("unexpected status: %d", resp.StatusCode)
	}

	var session types.Session
	if err := json.NewDecoder(resp.Body).Decode(&session); err != nil {
		t.Fatalf("decode session: %v", err)
	}
	return &session
}

func listSessions(t *testing.T, server *httptest.Server) sessionsResponse {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, server.URL+"/v1/sessions", nil)
	req.Header.Set("Authorization", "Bearer token")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: %d", resp.StatusCode)
	}

	var payload sessionsResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode sessions: %v", err)
	}
	return payload
}

func getSession(t *testing.T, server *httptest.Server, id string) *types.Session {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, server.URL+"/v1/sessions/"+id, nil)
	req.Header.Set("Authorization", "Bearer token")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: %d", resp.StatusCode)
	}

	var session types.Session
	if err := json.NewDecoder(resp.Body).Decode(&session); err != nil {
		t.Fatalf("decode session: %v", err)
	}
	return &session
}

type tailResponse struct {
	Items []map[string]any `json:"items"`
}

func tailSession(t *testing.T, server *httptest.Server, id string) tailResponse {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, server.URL+"/v1/sessions/"+id+"/tail?lines=50", nil)
	req.Header.Set("Authorization", "Bearer token")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("tail session: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: %d", resp.StatusCode)
	}

	var payload tailResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode tail: %v", err)
	}
	return payload
}

func historySession(t *testing.T, server *httptest.Server, id string) itemsResponse {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, server.URL+"/v1/sessions/"+id+"/history?lines=50", nil)
	req.Header.Set("Authorization", "Bearer token")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("history session: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: %d", resp.StatusCode)
	}

	var payload itemsResponse
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode history: %v", err)
	}
	return payload
}

func sendMessageStatus(t *testing.T, server *httptest.Server, id, text string) int {
	t.Helper()
	body := bytes.NewBufferString(`{"text":"` + text + `"}`)
	req, _ := http.NewRequest(http.MethodPost, server.URL+"/v1/sessions/"+id+"/send", body)
	req.Header.Set("Authorization", "Bearer token")
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("send session: %v", err)
	}
	defer resp.Body.Close()
	return resp.StatusCode
}

func killSession(t *testing.T, server *httptest.Server, id string) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, server.URL+"/v1/sessions/"+id+"/kill", nil)
	req.Header.Set("Authorization", "Bearer token")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("kill session: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: %d", resp.StatusCode)
	}
}

func markExited(t *testing.T, server *httptest.Server, id string) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, server.URL+"/v1/sessions/"+id+"/exit", nil)
	req.Header.Set("Authorization", "Bearer token")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("exit session: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: %d", resp.StatusCode)
	}
}

func storeSessionsIndex(t *testing.T) *store.FileSessionIndexStore {
	t.Helper()
	path := filepath.Join(t.TempDir(), "sessions.json")
	return store.NewFileSessionIndexStore(path)
}
