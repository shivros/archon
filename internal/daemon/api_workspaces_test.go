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

	createBody, _ := json.Marshal(types.Workspace{RepoPath: repoDir})
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

	createBody, _ := json.Marshal(types.Workspace{RepoPath: repoDir})
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
	renameBody, _ := json.Marshal(types.Worktree{Name: "Renamed Worktree"})
	renameReq, _ := http.NewRequest(http.MethodPatch, server.URL+"/v1/workspaces/"+created.ID+"/worktrees/"+wt.ID, bytes.NewReader(renameBody))
	renameReq.Header.Set("Authorization", "Bearer token")
	renameReq.Header.Set("Content-Type", "application/json")
	renameResp, err := http.DefaultClient.Do(renameReq)
	if err != nil {
		t.Fatalf("rename worktree: %v", err)
	}
	defer renameResp.Body.Close()
	if renameResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(renameResp.Body)
		t.Fatalf("expected 200, got %d: %s", renameResp.StatusCode, string(body))
	}
	var renamed types.Worktree
	if err := json.NewDecoder(renameResp.Body).Decode(&renamed); err != nil {
		t.Fatalf("decode renamed wt: %v", err)
	}
	if renamed.Name != "Renamed Worktree" {
		t.Fatalf("expected renamed worktree, got %q", renamed.Name)
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

	createBody, _ := json.Marshal(types.Workspace{RepoPath: repoDir})
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
		Provider: "custom",
		Cmd:      os.Args[0],
		Args:     helperArgs("stdout=api", "stderr=err", "sleep_ms=20", "exit=0"),
		Env:      []string{"GO_WANT_HELPER_PROCESS=1"},
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

func TestWorkspaceSessionSubpathRoundTrip(t *testing.T) {
	stores := newTestStores(t)
	api := &API{Version: "test", Manager: nil, Stores: stores}
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/workspaces", api.Workspaces)
	mux.HandleFunc("/v1/workspaces/", api.WorkspaceByID)
	server := httptest.NewServer(TokenAuthMiddleware("token", mux))
	defer server.Close()

	repoDir := filepath.Join(t.TempDir(), "repo")
	sessionDir := filepath.Join(repoDir, "packages", "pennies")
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatalf("mkdir session dir: %v", err)
	}

	createBody, _ := json.Marshal(types.Workspace{
		RepoPath:       repoDir,
		SessionSubpath: "packages/pennies/",
	})
	createReq, _ := http.NewRequest(http.MethodPost, server.URL+"/v1/workspaces", bytes.NewReader(createBody))
	createReq.Header.Set("Authorization", "Bearer token")
	createReq.Header.Set("Content-Type", "application/json")
	createResp, err := http.DefaultClient.Do(createReq)
	if err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	defer createResp.Body.Close()
	if createResp.StatusCode != http.StatusCreated {
		data, _ := io.ReadAll(createResp.Body)
		t.Fatalf("expected 201, got %d: %s", createResp.StatusCode, string(data))
	}
	var created types.Workspace
	if err := json.NewDecoder(createResp.Body).Decode(&created); err != nil {
		t.Fatalf("decode workspace: %v", err)
	}
	if created.SessionSubpath != filepath.Join("packages", "pennies") {
		t.Fatalf("expected normalized session subpath, got %q", created.SessionSubpath)
	}
}

func TestWorkspaceSessionSubpathPatchSemantics(t *testing.T) {
	stores := newTestStores(t)
	api := &API{Version: "test", Manager: nil, Stores: stores}
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/workspaces", api.Workspaces)
	mux.HandleFunc("/v1/workspaces/", api.WorkspaceByID)
	server := httptest.NewServer(TokenAuthMiddleware("token", mux))
	defer server.Close()

	repoDir := filepath.Join(t.TempDir(), "repo")
	sessionDir := filepath.Join(repoDir, "packages", "pennies")
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatalf("mkdir session dir: %v", err)
	}

	createBody, _ := json.Marshal(types.Workspace{
		RepoPath:       repoDir,
		SessionSubpath: "packages/pennies",
	})
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
		t.Fatalf("decode workspace: %v", err)
	}

	renamePatchBody, _ := json.Marshal(map[string]any{"name": "Renamed"})
	renamePatchReq, _ := http.NewRequest(http.MethodPatch, server.URL+"/v1/workspaces/"+created.ID, bytes.NewReader(renamePatchBody))
	renamePatchReq.Header.Set("Authorization", "Bearer token")
	renamePatchReq.Header.Set("Content-Type", "application/json")
	renamePatchResp, err := http.DefaultClient.Do(renamePatchReq)
	if err != nil {
		t.Fatalf("patch workspace name: %v", err)
	}
	defer renamePatchResp.Body.Close()
	if renamePatchResp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(renamePatchResp.Body)
		t.Fatalf("expected 200, got %d: %s", renamePatchResp.StatusCode, string(data))
	}
	var renamed types.Workspace
	if err := json.NewDecoder(renamePatchResp.Body).Decode(&renamed); err != nil {
		t.Fatalf("decode renamed workspace: %v", err)
	}
	if renamed.SessionSubpath != filepath.Join("packages", "pennies") {
		t.Fatalf("expected session_subpath unchanged, got %q", renamed.SessionSubpath)
	}

	newSubpath := filepath.Join("packages", "ledger")
	if err := os.MkdirAll(filepath.Join(repoDir, newSubpath), 0o755); err != nil {
		t.Fatalf("mkdir new subpath dir: %v", err)
	}
	setPatchBody, _ := json.Marshal(map[string]any{"session_subpath": newSubpath})
	setPatchReq, _ := http.NewRequest(http.MethodPatch, server.URL+"/v1/workspaces/"+created.ID, bytes.NewReader(setPatchBody))
	setPatchReq.Header.Set("Authorization", "Bearer token")
	setPatchReq.Header.Set("Content-Type", "application/json")
	setPatchResp, err := http.DefaultClient.Do(setPatchReq)
	if err != nil {
		t.Fatalf("patch workspace set subpath: %v", err)
	}
	defer setPatchResp.Body.Close()
	if setPatchResp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(setPatchResp.Body)
		t.Fatalf("expected 200, got %d: %s", setPatchResp.StatusCode, string(data))
	}
	var updated types.Workspace
	if err := json.NewDecoder(setPatchResp.Body).Decode(&updated); err != nil {
		t.Fatalf("decode updated workspace: %v", err)
	}
	if updated.SessionSubpath != newSubpath {
		t.Fatalf("expected session_subpath %q, got %q", newSubpath, updated.SessionSubpath)
	}

	clearPatchBody, _ := json.Marshal(map[string]any{"session_subpath": ""})
	clearPatchReq, _ := http.NewRequest(http.MethodPatch, server.URL+"/v1/workspaces/"+created.ID, bytes.NewReader(clearPatchBody))
	clearPatchReq.Header.Set("Authorization", "Bearer token")
	clearPatchReq.Header.Set("Content-Type", "application/json")
	clearPatchResp, err := http.DefaultClient.Do(clearPatchReq)
	if err != nil {
		t.Fatalf("patch workspace clear subpath: %v", err)
	}
	defer clearPatchResp.Body.Close()
	if clearPatchResp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(clearPatchResp.Body)
		t.Fatalf("expected 200, got %d: %s", clearPatchResp.StatusCode, string(data))
	}
	var cleared types.Workspace
	if err := json.NewDecoder(clearPatchResp.Body).Decode(&cleared); err != nil {
		t.Fatalf("decode cleared workspace: %v", err)
	}
	if cleared.SessionSubpath != "" {
		t.Fatalf("expected session_subpath cleared, got %q", cleared.SessionSubpath)
	}

	invalidPatchBody, _ := json.Marshal(map[string]any{"session_subpath": "../outside"})
	invalidPatchReq, _ := http.NewRequest(http.MethodPatch, server.URL+"/v1/workspaces/"+created.ID, bytes.NewReader(invalidPatchBody))
	invalidPatchReq.Header.Set("Authorization", "Bearer token")
	invalidPatchReq.Header.Set("Content-Type", "application/json")
	invalidPatchResp, err := http.DefaultClient.Do(invalidPatchReq)
	if err != nil {
		t.Fatalf("patch workspace invalid subpath: %v", err)
	}
	defer invalidPatchResp.Body.Close()
	if invalidPatchResp.StatusCode != http.StatusBadRequest {
		data, _ := io.ReadAll(invalidPatchResp.Body)
		t.Fatalf("expected 400, got %d: %s", invalidPatchResp.StatusCode, string(data))
	}
}

func TestWorkspaceSessionsEndpointUsesSessionSubpath(t *testing.T) {
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
	sessionSubpath := filepath.Join("packages", "pennies")
	sessionDir := filepath.Join(repoDir, sessionSubpath)
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatalf("mkdir session dir: %v", err)
	}

	createBody, _ := json.Marshal(types.Workspace{
		RepoPath:       repoDir,
		SessionSubpath: sessionSubpath,
	})
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
		t.Fatalf("decode workspace: %v", err)
	}

	startReq := StartSessionRequest{
		Provider: "custom",
		Cmd:      os.Args[0],
		Args:     helperArgs("stdout=api", "stderr=err", "sleep_ms=20", "exit=0"),
		Env:      []string{"GO_WANT_HELPER_PROCESS=1"},
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
	if session.Cwd != sessionDir {
		t.Fatalf("expected session cwd %q, got %q", sessionDir, session.Cwd)
	}
}

func TestWorktreeSessionsEndpointUsesSessionSubpath(t *testing.T) {
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
	sessionSubpath := filepath.Join("packages", "pennies")
	if err := os.MkdirAll(filepath.Join(repoDir, sessionSubpath), 0o755); err != nil {
		t.Fatalf("mkdir repo session dir: %v", err)
	}

	createBody, _ := json.Marshal(types.Workspace{
		RepoPath:       repoDir,
		SessionSubpath: sessionSubpath,
	})
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
		t.Fatalf("decode workspace: %v", err)
	}

	worktreeDir := filepath.Join(t.TempDir(), "repo-wt")
	worktreeSessionDir := filepath.Join(worktreeDir, sessionSubpath)
	if err := os.MkdirAll(worktreeSessionDir, 0o755); err != nil {
		t.Fatalf("mkdir worktree session dir: %v", err)
	}
	worktreeBody, _ := json.Marshal(types.Worktree{Path: worktreeDir})
	worktreeReq, _ := http.NewRequest(http.MethodPost, server.URL+"/v1/workspaces/"+created.ID+"/worktrees", bytes.NewReader(worktreeBody))
	worktreeReq.Header.Set("Authorization", "Bearer token")
	worktreeReq.Header.Set("Content-Type", "application/json")
	worktreeResp, err := http.DefaultClient.Do(worktreeReq)
	if err != nil {
		t.Fatalf("create worktree: %v", err)
	}
	defer worktreeResp.Body.Close()
	var worktree types.Worktree
	if err := json.NewDecoder(worktreeResp.Body).Decode(&worktree); err != nil {
		t.Fatalf("decode worktree: %v", err)
	}

	startReq := StartSessionRequest{
		Provider: "custom",
		Cmd:      os.Args[0],
		Args:     helperArgs("stdout=api", "stderr=err", "sleep_ms=20", "exit=0"),
		Env:      []string{"GO_WANT_HELPER_PROCESS=1"},
	}
	body, _ := json.Marshal(startReq)
	req, _ := http.NewRequest(http.MethodPost, server.URL+"/v1/workspaces/"+created.ID+"/worktrees/"+worktree.ID+"/sessions", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer token")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("start worktree session: %v", err)
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
	if session.Cwd != worktreeSessionDir {
		t.Fatalf("expected session cwd %q, got %q", worktreeSessionDir, session.Cwd)
	}
}

func newTestStores(t *testing.T) *Stores {
	t.Helper()
	base := t.TempDir()
	workspaces := store.NewFileWorkspaceStore(filepath.Join(base, "workspaces.json"))
	state := store.NewFileAppStateStore(filepath.Join(base, "state.json"))
	meta := store.NewFileSessionMetaStore(filepath.Join(base, "sessions_meta.json"))
	return &Stores{
		Workspaces:  workspaces,
		Worktrees:   workspaces,
		Groups:      workspaces,
		AppState:    state,
		SessionMeta: meta,
	}
}
