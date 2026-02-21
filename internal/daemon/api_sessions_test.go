package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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

type fakeSessionSyncer struct {
	calls        int
	workspaceIDs []string
	err          error
}

func (f *fakeSessionSyncer) SyncAll(context.Context) error {
	f.calls++
	return f.err
}

func (f *fakeSessionSyncer) SyncWorkspace(_ context.Context, workspaceID string) error {
	f.calls++
	f.workspaceIDs = append(f.workspaceIDs, workspaceID)
	return nil
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

func TestAPISessionExitRemainsVisibleInDefaultList(t *testing.T) {
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
	if len(list.Sessions) != 1 {
		t.Fatalf("expected exited session to remain visible, got %d", len(list.Sessions))
	}
}

func TestAPISessionExitAllowsNoProcessProvider(t *testing.T) {
	store := storeSessionsIndex(t)
	now := time.Now().UTC()
	_, err := store.UpsertRecord(context.Background(), &types.SessionRecord{
		Session: &types.Session{
			ID:        "sess-claude",
			Provider:  "claude",
			Cmd:       "claude",
			Status:    types.SessionStatusRunning,
			CreatedAt: now,
		},
		Source: "claude",
	})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}

	server := newTestServerWithStores(t, nil, &Stores{Sessions: store})
	defer server.Close()

	markExited(t, server, "sess-claude")

	got := getSession(t, server, "sess-claude")
	if got.Status != types.SessionStatusExited {
		t.Fatalf("expected exited status, got %s", got.Status)
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

func TestAPISessionsRefreshTriggersSync(t *testing.T) {
	store := storeSessionsIndex(t)
	now := time.Now().UTC()
	_, err := store.UpsertRecord(context.Background(), &types.SessionRecord{
		Session: &types.Session{
			ID:        "sess-sync",
			Provider:  "codex",
			Cmd:       "codex app-server",
			Status:    types.SessionStatusInactive,
			CreatedAt: now,
		},
		Source: sessionSourceCodex,
	})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}

	syncer := &fakeSessionSyncer{}
	api := &API{Version: "test", Stores: &Stores{Sessions: store}, Syncer: syncer}
	server := newTestServerWithAPI(t, api)
	defer server.Close()

	list := listSessionsPath(t, server, "/v1/sessions?refresh=1")
	if len(list.Sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(list.Sessions))
	}
	if syncer.calls != 1 {
		t.Fatalf("expected one sync call, got %d", syncer.calls)
	}
}

func TestAPISessionsRefreshTriggersWorkspaceSync(t *testing.T) {
	store := storeSessionsIndex(t)
	now := time.Now().UTC()
	_, err := store.UpsertRecord(context.Background(), &types.SessionRecord{
		Session: &types.Session{
			ID:        "sess-sync-ws",
			Provider:  "codex",
			Cmd:       "codex app-server",
			Status:    types.SessionStatusInactive,
			CreatedAt: now,
		},
		Source: sessionSourceCodex,
	})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}

	syncer := &fakeSessionSyncer{}
	api := &API{Version: "test", Stores: &Stores{Sessions: store}, Syncer: syncer}
	server := newTestServerWithAPI(t, api)
	defer server.Close()

	list := listSessionsPath(t, server, "/v1/sessions?refresh=1&workspace_id=ws-1")
	if len(list.Sessions) != 1 {
		t.Fatalf("expected 1 session, got %d", len(list.Sessions))
	}
	if syncer.calls != 1 {
		t.Fatalf("expected one sync call, got %d", syncer.calls)
	}
	if len(syncer.workspaceIDs) != 1 || syncer.workspaceIDs[0] != "ws-1" {
		t.Fatalf("expected workspace sync ws-1, got %+v", syncer.workspaceIDs)
	}
}

func TestAPISessionsRefreshSyncError(t *testing.T) {
	syncer := &fakeSessionSyncer{err: errors.New("sync failed")}
	api := &API{Version: "test", Syncer: syncer}
	server := newTestServerWithAPI(t, api)
	defer server.Close()

	req, _ := http.NewRequest(http.MethodGet, server.URL+"/v1/sessions?refresh=1", nil)
	req.Header.Set("Authorization", "Bearer token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("refresh request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusInternalServerError {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 500, got %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if syncer.calls != 1 {
		t.Fatalf("expected one sync call, got %d", syncer.calls)
	}
}

func TestAPIDismissAndUndismissSession(t *testing.T) {
	sessionStore := storeSessionsIndex(t)
	metaStore := store.NewFileSessionMetaStore(filepath.Join(t.TempDir(), "sessions_meta.json"))
	now := time.Now().UTC()
	_, err := sessionStore.UpsertRecord(context.Background(), &types.SessionRecord{
		Session: &types.Session{
			ID:        "sess-dismiss",
			Provider:  "codex",
			Cmd:       "codex app-server",
			Status:    types.SessionStatusInactive,
			CreatedAt: now,
		},
		Source: sessionSourceInternal,
	})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}

	server := newTestServerWithStores(t, nil, &Stores{Sessions: sessionStore, SessionMeta: metaStore})
	defer server.Close()

	dismissSession(t, server, "sess-dismiss")

	got := getSession(t, server, "sess-dismiss")
	if got.Status != types.SessionStatusInactive {
		t.Fatalf("expected status unchanged after dismiss, got %s", got.Status)
	}
	meta, ok, err := metaStore.Get(context.Background(), "sess-dismiss")
	if err != nil {
		t.Fatalf("get meta after dismiss: %v", err)
	}
	if !ok || meta == nil || meta.DismissedAt == nil {
		t.Fatalf("expected dismissed_at after dismiss")
	}

	list := listSessions(t, server)
	if len(list.Sessions) != 0 {
		t.Fatalf("expected dismissed session hidden from default list, got %d", len(list.Sessions))
	}
	list = listSessionsPath(t, server, "/v1/sessions?include_dismissed=1")
	if len(list.Sessions) != 1 || list.Sessions[0].ID != "sess-dismiss" {
		t.Fatalf("expected dismissed session in include_dismissed list, got %+v", list.Sessions)
	}

	undismissSession(t, server, "sess-dismiss")
	got = getSession(t, server, "sess-dismiss")
	if got.Status != types.SessionStatusInactive {
		t.Fatalf("expected inactive status after undismiss, got %s", got.Status)
	}
	meta, ok, err = metaStore.Get(context.Background(), "sess-dismiss")
	if err != nil {
		t.Fatalf("get meta after undismiss: %v", err)
	}
	if !ok || meta == nil || meta.DismissedAt != nil {
		t.Fatalf("expected dismissed_at cleared after undismiss")
	}
	list = listSessions(t, server)
	if len(list.Sessions) != 1 || list.Sessions[0].ID != "sess-dismiss" {
		t.Fatalf("expected undismissed session in default list, got %+v", list.Sessions)
	}
}

func TestAPISessionsIncludeWorkflowOwnedQuery(t *testing.T) {
	sessionStore := storeSessionsIndex(t)
	metaStore := store.NewFileSessionMetaStore(filepath.Join(t.TempDir(), "sessions_meta.json"))
	now := time.Now().UTC()
	_, err := sessionStore.UpsertRecord(context.Background(), &types.SessionRecord{
		Session: &types.Session{
			ID:        "sess-workflow-owned",
			Provider:  "codex",
			Cmd:       "codex app-server",
			Status:    types.SessionStatusInactive,
			CreatedAt: now,
		},
		Source: sessionSourceInternal,
	})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	if _, err := metaStore.Upsert(context.Background(), &types.SessionMeta{
		SessionID:     "sess-workflow-owned",
		WorkflowRunID: "gwf-1",
	}); err != nil {
		t.Fatalf("upsert meta: %v", err)
	}

	server := newTestServerWithStores(t, nil, &Stores{Sessions: sessionStore, SessionMeta: metaStore})
	defer server.Close()

	defaultList := listSessions(t, server)
	if len(defaultList.Sessions) != 0 {
		t.Fatalf("expected workflow-owned sessions excluded by default, got %+v", defaultList.Sessions)
	}

	includedList := listSessionsPath(t, server, "/v1/sessions?include_workflow_owned=1")
	if len(includedList.Sessions) != 1 || includedList.Sessions[0].ID != "sess-workflow-owned" {
		t.Fatalf("expected workflow-owned session when include_workflow_owned=1, got %+v", includedList.Sessions)
	}
}

func TestAPISessionsCombinedIncludeQuery(t *testing.T) {
	sessionStore := storeSessionsIndex(t)
	metaStore := store.NewFileSessionMetaStore(filepath.Join(t.TempDir(), "sessions_meta.json"))
	now := time.Now().UTC()
	_, err := sessionStore.UpsertRecord(context.Background(), &types.SessionRecord{
		Session: &types.Session{
			ID:        "sess-workflow-owned-dismissed",
			Provider:  "codex",
			Cmd:       "codex app-server",
			Status:    types.SessionStatusExited,
			CreatedAt: now,
		},
		Source: sessionSourceInternal,
	})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	dismissedAt := now.Add(-time.Minute)
	if _, err := metaStore.Upsert(context.Background(), &types.SessionMeta{
		SessionID:     "sess-workflow-owned-dismissed",
		WorkflowRunID: "gwf-1",
		DismissedAt:   &dismissedAt,
	}); err != nil {
		t.Fatalf("upsert meta: %v", err)
	}

	server := newTestServerWithStores(t, nil, &Stores{Sessions: sessionStore, SessionMeta: metaStore})
	defer server.Close()

	defaultList := listSessions(t, server)
	if len(defaultList.Sessions) != 0 {
		t.Fatalf("expected session hidden from default list, got %+v", defaultList.Sessions)
	}

	combinedList := listSessionsPath(t, server, "/v1/sessions?include_dismissed=1&include_workflow_owned=1")
	if len(combinedList.Sessions) != 1 || combinedList.Sessions[0].ID != "sess-workflow-owned-dismissed" {
		t.Fatalf("expected session when include_dismissed=1&include_workflow_owned=1, got %+v", combinedList.Sessions)
	}
}

func newTestServer(t *testing.T, manager *SessionManager) *httptest.Server {
	return newTestServerWithStores(t, manager, nil)
}

func newTestServerWithStores(t *testing.T, manager *SessionManager, stores *Stores) *httptest.Server {
	t.Helper()
	api := &API{Version: "test", Manager: manager, Stores: stores}
	return newTestServerWithAPI(t, api)
}

func newTestServerWithAPI(t *testing.T, api *API) *httptest.Server {
	t.Helper()
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
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("unexpected status: %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var session types.Session
	if err := json.NewDecoder(resp.Body).Decode(&session); err != nil {
		t.Fatalf("decode session: %v", err)
	}
	return &session
}

func listSessions(t *testing.T, server *httptest.Server) sessionsResponse {
	t.Helper()
	return listSessionsPath(t, server, "/v1/sessions")
}

func listSessionsPath(t *testing.T, server *httptest.Server, path string) sessionsResponse {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, server.URL+path, nil)
	req.Header.Set("Authorization", "Bearer token")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("list sessions: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("unexpected status: %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
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
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("unexpected status: %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
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
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("unexpected status: %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
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
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("unexpected status: %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
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

func dismissSession(t *testing.T, server *httptest.Server, id string) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, server.URL+"/v1/sessions/"+id+"/dismiss", nil)
	req.Header.Set("Authorization", "Bearer token")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("dismiss session: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("unexpected status: %d", resp.StatusCode)
	}
}

func undismissSession(t *testing.T, server *httptest.Server, id string) {
	t.Helper()
	req, _ := http.NewRequest(http.MethodPost, server.URL+"/v1/sessions/"+id+"/undismiss", nil)
	req.Header.Set("Authorization", "Bearer token")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("undismiss session: %v", err)
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
