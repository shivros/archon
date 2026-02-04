package daemon

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"control/internal/store"
	"control/internal/types"
)

func TestWorkspaceEndpoints(t *testing.T) {
	stores := newTestStores(t)
	api := &API{Version: "test", Manager: nil, Stores: stores}
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/workspaces", api.Workspaces)
	mux.HandleFunc("/v1/workspaces/", api.WorkspaceByID)
	server := httptest.NewServer(TokenAuthMiddleware("token", mux))
	defer server.Close()

	repoDir := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}

	createBody, _ := json.Marshal(types.Workspace{RepoPath: repoDir, Provider: "codex"})
	createReq, _ := http.NewRequest(http.MethodPost, server.URL+"/v1/workspaces", bytes.NewReader(createBody))
	createReq.Header.Set("Authorization", "Bearer token")
	createReq.Header.Set("Content-Type", "application/json")

	createResp, err := http.DefaultClient.Do(createReq)
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	defer createResp.Body.Close()
	if createResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", createResp.StatusCode)
	}
	var created types.Workspace
	if err := json.NewDecoder(createResp.Body).Decode(&created); err != nil {
		t.Fatalf("decode: %v", err)
	}

	listReq, _ := http.NewRequest(http.MethodGet, server.URL+"/v1/workspaces", nil)
	listReq.Header.Set("Authorization", "Bearer token")
	listResp, err := http.DefaultClient.Do(listReq)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	defer listResp.Body.Close()
	if listResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(listResp.Body)
		t.Fatalf("expected 200, got %d: %s", listResp.StatusCode, string(body))
	}
	var listPayload struct {
		Workspaces []*types.Workspace `json:"workspaces"`
	}
	if err := json.NewDecoder(listResp.Body).Decode(&listPayload); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(listPayload.Workspaces) != 1 {
		t.Fatalf("expected 1 workspace")
	}

	updateBody, _ := json.Marshal(types.Workspace{Name: "Renamed"})
	updateReq, _ := http.NewRequest(http.MethodPatch, server.URL+"/v1/workspaces/"+created.ID, bytes.NewReader(updateBody))
	updateReq.Header.Set("Authorization", "Bearer token")
	updateReq.Header.Set("Content-Type", "application/json")
	updateResp, err := http.DefaultClient.Do(updateReq)
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	defer updateResp.Body.Close()
	if updateResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", updateResp.StatusCode)
	}

	deleteReq, _ := http.NewRequest(http.MethodDelete, server.URL+"/v1/workspaces/"+created.ID, nil)
	deleteReq.Header.Set("Authorization", "Bearer token")
	deleteResp, err := http.DefaultClient.Do(deleteReq)
	if err != nil {
		t.Fatalf("delete: %v", err)
	}
	defer deleteResp.Body.Close()
	if deleteResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", deleteResp.StatusCode)
	}
}

func TestWorktreeEndpoints(t *testing.T) {
	stores := newTestStores(t)
	api := &API{Version: "test", Manager: nil, Stores: stores}
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/workspaces", api.Workspaces)
	mux.HandleFunc("/v1/workspaces/", api.WorkspaceByID)
	server := httptest.NewServer(TokenAuthMiddleware("token", mux))
	defer server.Close()

	repoDir := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}

	createBody, _ := json.Marshal(types.Workspace{RepoPath: repoDir, Provider: "codex"})
	createReq, _ := http.NewRequest(http.MethodPost, server.URL+"/v1/workspaces", bytes.NewReader(createBody))
	createReq.Header.Set("Authorization", "Bearer token")
	createReq.Header.Set("Content-Type", "application/json")
	createResp, err := http.DefaultClient.Do(createReq)
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	defer createResp.Body.Close()
	var created types.Workspace
	if err := json.NewDecoder(createResp.Body).Decode(&created); err != nil {
		t.Fatalf("decode: %v", err)
	}

	worktreeDir := filepath.Join(repoDir, "wt")
	if err := os.MkdirAll(worktreeDir, 0o755); err != nil {
		t.Fatalf("mkdir wt: %v", err)
	}

	wtBody, _ := json.Marshal(types.Worktree{Path: worktreeDir})
	wtReq, _ := http.NewRequest(http.MethodPost, server.URL+"/v1/workspaces/"+created.ID+"/worktrees", bytes.NewReader(wtBody))
	wtReq.Header.Set("Authorization", "Bearer token")
	wtReq.Header.Set("Content-Type", "application/json")
	wtResp, err := http.DefaultClient.Do(wtReq)
	if err != nil {
		t.Fatalf("add worktree: %v", err)
	}
	defer wtResp.Body.Close()
	if wtResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", wtResp.StatusCode)
	}
	var wt types.Worktree
	if err := json.NewDecoder(wtResp.Body).Decode(&wt); err != nil {
		t.Fatalf("decode wt: %v", err)
	}

	listReq, _ := http.NewRequest(http.MethodGet, server.URL+"/v1/workspaces/"+created.ID+"/worktrees", nil)
	listReq.Header.Set("Authorization", "Bearer token")
	listResp, err := http.DefaultClient.Do(listReq)
	if err != nil {
		t.Fatalf("list worktrees: %v", err)
	}
	defer listResp.Body.Close()
	if listResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(listResp.Body)
		t.Fatalf("expected 200, got %d: %s", listResp.StatusCode, string(body))
	}

	deleteReq, _ := http.NewRequest(http.MethodDelete, server.URL+"/v1/workspaces/"+created.ID+"/worktrees/"+wt.ID, nil)
	deleteReq.Header.Set("Authorization", "Bearer token")
	deleteResp, err := http.DefaultClient.Do(deleteReq)
	if err != nil {
		t.Fatalf("delete worktree: %v", err)
	}
	defer deleteResp.Body.Close()
	if deleteResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", deleteResp.StatusCode)
	}
}

func TestWorkspaceSessionsEndpoint(t *testing.T) {
	stores := newTestStores(t)
	manager := newTestManager(t)
	api := &API{Version: "test", Manager: manager, Stores: stores}
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/workspaces", api.Workspaces)
	mux.HandleFunc("/v1/workspaces/", api.WorkspaceByID)
	mux.HandleFunc("/v1/sessions", api.Sessions)
	server := httptest.NewServer(TokenAuthMiddleware("token", mux))
	defer server.Close()

	repoDir := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}

	createBody, _ := json.Marshal(types.Workspace{RepoPath: repoDir, Provider: "custom"})
	createReq, _ := http.NewRequest(http.MethodPost, server.URL+"/v1/workspaces", bytes.NewReader(createBody))
	createReq.Header.Set("Authorization", "Bearer token")
	createReq.Header.Set("Content-Type", "application/json")
	createResp, err := http.DefaultClient.Do(createReq)
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	defer createResp.Body.Close()
	var created types.Workspace
	if err := json.NewDecoder(createResp.Body).Decode(&created); err != nil {
		t.Fatalf("decode: %v", err)
	}

	startReq := StartSessionRequest{
		Cmd:  os.Args[0],
		Args: helperArgs("stdout=api", "stderr=err", "sleep_ms=20", "exit=0"),
		Env:  []string{"GO_WANT_HELPER_PROCESS=1"},
	}
	body, _ := json.Marshal(startReq)
	req, _ := http.NewRequest(http.MethodPost, server.URL+"/v1/workspaces/"+created.ID+"/sessions", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer token")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("start session: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		data, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 201, got %d: %s", resp.StatusCode, string(data))
	}
	var session types.Session
	if err := json.NewDecoder(resp.Body).Decode(&session); err != nil {
		t.Fatalf("decode session: %v", err)
	}
	if session.ID == "" {
		t.Fatalf("expected session id")
	}
}

func newTestStores(t *testing.T) *Stores {
	t.Helper()
	base := t.TempDir()
	workspaces := store.NewFileWorkspaceStore(filepath.Join(base, "workspaces.json"))
	state := store.NewFileAppStateStore(filepath.Join(base, "state.json"))
	keymap := store.NewFileKeymapStore(filepath.Join(base, "keymap.json"))
	meta := store.NewFileSessionMetaStore(filepath.Join(base, "sessions_meta.json"))
	return &Stores{
		Workspaces:  workspaces,
		Worktrees:   workspaces,
		AppState:    state,
		Keymap:      keymap,
		SessionMeta: meta,
	}
}
