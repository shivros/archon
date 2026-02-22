package daemon

import (
	"bytes"
	"encoding/json"
	"fmt"
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

	updateBody, _ := json.Marshal(map[string]any{"name": "Renamed"})
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

func TestWorkspaceAdditionalDirectoriesPatchSemantics(t *testing.T) {
	stores := newTestStores(t)
	api := &API{Version: "test", Manager: nil, Stores: stores}
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/workspaces", api.Workspaces)
	mux.HandleFunc("/v1/workspaces/", api.WorkspaceByID)
	server := httptest.NewServer(TokenAuthMiddleware("token", mux))
	defer server.Close()

	repoDir := filepath.Join(t.TempDir(), "repo")
	sessionSubpath := filepath.Join("packages", "pennies")
	sessionDir := filepath.Join(repoDir, sessionSubpath)
	backendDir := filepath.Join(repoDir, "packages", "backend")
	sharedDir := filepath.Join(repoDir, "packages", "shared")
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatalf("mkdir session dir: %v", err)
	}
	if err := os.MkdirAll(backendDir, 0o755); err != nil {
		t.Fatalf("mkdir backend dir: %v", err)
	}
	if err := os.MkdirAll(sharedDir, 0o755); err != nil {
		t.Fatalf("mkdir shared dir: %v", err)
	}

	createBody, _ := json.Marshal(types.Workspace{
		RepoPath:              repoDir,
		SessionSubpath:        sessionSubpath,
		AdditionalDirectories: []string{"../backend", "../shared"},
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
	if len(created.AdditionalDirectories) != 2 {
		t.Fatalf("expected additional directories to persist, got %#v", created.AdditionalDirectories)
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
	if len(renamed.AdditionalDirectories) != 2 {
		t.Fatalf("expected additional directories unchanged, got %#v", renamed.AdditionalDirectories)
	}

	setPatchBody, _ := json.Marshal(map[string]any{"additional_directories": []string{"../backend"}})
	setPatchReq, _ := http.NewRequest(http.MethodPatch, server.URL+"/v1/workspaces/"+created.ID, bytes.NewReader(setPatchBody))
	setPatchReq.Header.Set("Authorization", "Bearer token")
	setPatchReq.Header.Set("Content-Type", "application/json")
	setPatchResp, err := http.DefaultClient.Do(setPatchReq)
	if err != nil {
		t.Fatalf("patch workspace additional directories: %v", err)
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
	if len(updated.AdditionalDirectories) != 1 || updated.AdditionalDirectories[0] != filepath.Clean("../backend") {
		t.Fatalf("expected additional directories update, got %#v", updated.AdditionalDirectories)
	}

	clearPatchBody, _ := json.Marshal(map[string]any{"additional_directories": []string{}})
	clearPatchReq, _ := http.NewRequest(http.MethodPatch, server.URL+"/v1/workspaces/"+created.ID, bytes.NewReader(clearPatchBody))
	clearPatchReq.Header.Set("Authorization", "Bearer token")
	clearPatchReq.Header.Set("Content-Type", "application/json")
	clearPatchResp, err := http.DefaultClient.Do(clearPatchReq)
	if err != nil {
		t.Fatalf("patch workspace clear additional directories: %v", err)
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
	if len(cleared.AdditionalDirectories) != 0 {
		t.Fatalf("expected additional directories cleared, got %#v", cleared.AdditionalDirectories)
	}

	invalidPatchBody, _ := json.Marshal(map[string]any{"additional_directories": []string{"../missing"}})
	invalidPatchReq, _ := http.NewRequest(http.MethodPatch, server.URL+"/v1/workspaces/"+created.ID, bytes.NewReader(invalidPatchBody))
	invalidPatchReq.Header.Set("Authorization", "Bearer token")
	invalidPatchReq.Header.Set("Content-Type", "application/json")
	invalidPatchResp, err := http.DefaultClient.Do(invalidPatchReq)
	if err != nil {
		t.Fatalf("patch workspace invalid additional directories: %v", err)
	}
	defer invalidPatchResp.Body.Close()
	if invalidPatchResp.StatusCode != http.StatusBadRequest {
		data, _ := io.ReadAll(invalidPatchResp.Body)
		t.Fatalf("expected 400, got %d: %s", invalidPatchResp.StatusCode, string(data))
	}
}

func TestWorkspaceRepoPathPatchSemantics(t *testing.T) {
	stores := newTestStores(t)
	api := &API{Version: "test", Manager: nil, Stores: stores}
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/workspaces", api.Workspaces)
	mux.HandleFunc("/v1/workspaces/", api.WorkspaceByID)
	server := httptest.NewServer(TokenAuthMiddleware("token", mux))
	defer server.Close()

	repoOne := filepath.Join(t.TempDir(), "repo-one")
	repoTwo := filepath.Join(t.TempDir(), "repo-two")
	subpath := filepath.Join("packages", "pennies")
	if err := os.MkdirAll(filepath.Join(repoOne, subpath), 0o755); err != nil {
		t.Fatalf("mkdir repo one subpath: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(repoTwo, subpath), 0o755); err != nil {
		t.Fatalf("mkdir repo two subpath: %v", err)
	}

	createBody, _ := json.Marshal(types.Workspace{
		RepoPath:       repoOne,
		SessionSubpath: subpath,
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

	patchBody, _ := json.Marshal(map[string]any{"repo_path": repoTwo})
	patchReq, _ := http.NewRequest(http.MethodPatch, server.URL+"/v1/workspaces/"+created.ID, bytes.NewReader(patchBody))
	patchReq.Header.Set("Authorization", "Bearer token")
	patchReq.Header.Set("Content-Type", "application/json")
	patchResp, err := http.DefaultClient.Do(patchReq)
	if err != nil {
		t.Fatalf("patch workspace repo path: %v", err)
	}
	defer patchResp.Body.Close()
	if patchResp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(patchResp.Body)
		t.Fatalf("expected 200, got %d: %s", patchResp.StatusCode, string(data))
	}
	var updated types.Workspace
	if err := json.NewDecoder(patchResp.Body).Decode(&updated); err != nil {
		t.Fatalf("decode updated workspace: %v", err)
	}
	if updated.RepoPath != repoTwo {
		t.Fatalf("expected repo_path %q, got %q", repoTwo, updated.RepoPath)
	}
	if updated.SessionSubpath != subpath {
		t.Fatalf("expected session_subpath %q, got %q", subpath, updated.SessionSubpath)
	}
	if updated.Name != filepath.Base(repoOne) {
		t.Fatalf("expected name to remain %q, got %q", filepath.Base(repoOne), updated.Name)
	}

	clearNamePatchBody, _ := json.Marshal(map[string]any{"name": ""})
	clearNamePatchReq, _ := http.NewRequest(http.MethodPatch, server.URL+"/v1/workspaces/"+created.ID, bytes.NewReader(clearNamePatchBody))
	clearNamePatchReq.Header.Set("Authorization", "Bearer token")
	clearNamePatchReq.Header.Set("Content-Type", "application/json")
	clearNamePatchResp, err := http.DefaultClient.Do(clearNamePatchReq)
	if err != nil {
		t.Fatalf("patch workspace clear name: %v", err)
	}
	defer clearNamePatchResp.Body.Close()
	if clearNamePatchResp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(clearNamePatchResp.Body)
		t.Fatalf("expected 200, got %d: %s", clearNamePatchResp.StatusCode, string(data))
	}
	var renamed types.Workspace
	if err := json.NewDecoder(clearNamePatchResp.Body).Decode(&renamed); err != nil {
		t.Fatalf("decode renamed workspace: %v", err)
	}
	if renamed.Name != filepath.Base(repoTwo) {
		t.Fatalf("expected default name %q, got %q", filepath.Base(repoTwo), renamed.Name)
	}

	whitespaceNamePatchBody, _ := json.Marshal(map[string]any{"name": "   "})
	whitespaceNamePatchReq, _ := http.NewRequest(http.MethodPatch, server.URL+"/v1/workspaces/"+created.ID, bytes.NewReader(whitespaceNamePatchBody))
	whitespaceNamePatchReq.Header.Set("Authorization", "Bearer token")
	whitespaceNamePatchReq.Header.Set("Content-Type", "application/json")
	whitespaceNamePatchResp, err := http.DefaultClient.Do(whitespaceNamePatchReq)
	if err != nil {
		t.Fatalf("patch workspace whitespace name: %v", err)
	}
	defer whitespaceNamePatchResp.Body.Close()
	if whitespaceNamePatchResp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(whitespaceNamePatchResp.Body)
		t.Fatalf("expected 200, got %d: %s", whitespaceNamePatchResp.StatusCode, string(data))
	}
	var whitespaceRenamed types.Workspace
	if err := json.NewDecoder(whitespaceNamePatchResp.Body).Decode(&whitespaceRenamed); err != nil {
		t.Fatalf("decode whitespace-renamed workspace: %v", err)
	}
	if whitespaceRenamed.Name != filepath.Base(repoTwo) {
		t.Fatalf("expected default name %q, got %q", filepath.Base(repoTwo), whitespaceRenamed.Name)
	}

	emptyPathPatchBody, _ := json.Marshal(map[string]any{"repo_path": ""})
	emptyPathPatchReq, _ := http.NewRequest(http.MethodPatch, server.URL+"/v1/workspaces/"+created.ID, bytes.NewReader(emptyPathPatchBody))
	emptyPathPatchReq.Header.Set("Authorization", "Bearer token")
	emptyPathPatchReq.Header.Set("Content-Type", "application/json")
	emptyPathPatchResp, err := http.DefaultClient.Do(emptyPathPatchReq)
	if err != nil {
		t.Fatalf("patch workspace empty repo path: %v", err)
	}
	defer emptyPathPatchResp.Body.Close()
	if emptyPathPatchResp.StatusCode != http.StatusBadRequest {
		data, _ := io.ReadAll(emptyPathPatchResp.Body)
		t.Fatalf("expected 400, got %d: %s", emptyPathPatchResp.StatusCode, string(data))
	}

	invalidPath := filepath.Join(t.TempDir(), "missing-repo")
	invalidPatchBody, _ := json.Marshal(map[string]any{"repo_path": invalidPath})
	invalidPatchReq, _ := http.NewRequest(http.MethodPatch, server.URL+"/v1/workspaces/"+created.ID, bytes.NewReader(invalidPatchBody))
	invalidPatchReq.Header.Set("Authorization", "Bearer token")
	invalidPatchReq.Header.Set("Content-Type", "application/json")
	invalidPatchResp, err := http.DefaultClient.Do(invalidPatchReq)
	if err != nil {
		t.Fatalf("patch workspace invalid repo path: %v", err)
	}
	defer invalidPatchResp.Body.Close()
	if invalidPatchResp.StatusCode != http.StatusBadRequest {
		data, _ := io.ReadAll(invalidPatchResp.Body)
		t.Fatalf("expected 400, got %d: %s", invalidPatchResp.StatusCode, string(data))
	}
}

func TestWorkspaceGroupIDsPatchSemantics(t *testing.T) {
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

	createBody, _ := json.Marshal(types.Workspace{
		RepoPath: repoDir,
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

	setPatchBody, _ := json.Marshal(map[string]any{
		"group_ids": []string{"group-b", " group-a ", "group-a", "ungrouped", ""},
	})
	setPatchReq, _ := http.NewRequest(http.MethodPatch, server.URL+"/v1/workspaces/"+created.ID, bytes.NewReader(setPatchBody))
	setPatchReq.Header.Set("Authorization", "Bearer token")
	setPatchReq.Header.Set("Content-Type", "application/json")
	setPatchResp, err := http.DefaultClient.Do(setPatchReq)
	if err != nil {
		t.Fatalf("patch workspace groups: %v", err)
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
	if len(updated.GroupIDs) != 2 || updated.GroupIDs[0] != "group-a" || updated.GroupIDs[1] != "group-b" {
		t.Fatalf("expected normalized group ids, got %#v", updated.GroupIDs)
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
	if len(renamed.GroupIDs) != 2 || renamed.GroupIDs[0] != "group-a" || renamed.GroupIDs[1] != "group-b" {
		t.Fatalf("expected group ids unchanged, got %#v", renamed.GroupIDs)
	}

	clearPatchBody, _ := json.Marshal(map[string]any{"group_ids": []string{}})
	clearPatchReq, _ := http.NewRequest(http.MethodPatch, server.URL+"/v1/workspaces/"+created.ID, bytes.NewReader(clearPatchBody))
	clearPatchReq.Header.Set("Authorization", "Bearer token")
	clearPatchReq.Header.Set("Content-Type", "application/json")
	clearPatchResp, err := http.DefaultClient.Do(clearPatchReq)
	if err != nil {
		t.Fatalf("patch workspace clear groups: %v", err)
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
	if len(cleared.GroupIDs) != 0 {
		t.Fatalf("expected group ids cleared, got %#v", cleared.GroupIDs)
	}
}

func TestWorkspaceSessionsEndpointUsesAdditionalDirectoriesForGemini(t *testing.T) {
	stores := newTestStores(t)
	manager := newTestManager(t)
	api := &API{Version: "test", Manager: manager, Stores: stores}
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/workspaces", api.Workspaces)
	mux.HandleFunc("/v1/workspaces/", api.WorkspaceByID)
	mux.HandleFunc("/v1/sessions", api.Sessions)
	server := httptest.NewServer(TokenAuthMiddleware("token", mux))
	defer server.Close()

	homeDir := filepath.Join(t.TempDir(), "home")
	if err := os.MkdirAll(filepath.Join(homeDir, ".archon"), 0o700); err != nil {
		t.Fatalf("mkdir home config dir: %v", err)
	}
	wrapper := filepath.Join(t.TempDir(), "gemini-wrapper.sh")
	argsFile := filepath.Join(t.TempDir(), "gemini-args.txt")
	script := `#!/bin/sh
if [ -n "$ARCHON_EXEC_ARGS_FILE" ]; then
  printf '%s\n' "$@" > "$ARCHON_EXEC_ARGS_FILE"
fi
echo ok
`
	if err := os.WriteFile(wrapper, []byte(script), 0o755); err != nil {
		t.Fatalf("write wrapper: %v", err)
	}
	cfg := fmt.Sprintf("[providers.gemini]\ncommand = %q\n", wrapper)
	if err := os.WriteFile(filepath.Join(homeDir, ".archon", "config.toml"), []byte(cfg), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	t.Setenv("HOME", homeDir)

	repoDir := filepath.Join(t.TempDir(), "repo")
	sessionSubpath := filepath.Join("packages", "pennies")
	sessionDir := filepath.Join(repoDir, sessionSubpath)
	backendDir := filepath.Join(repoDir, "packages", "backend")
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatalf("mkdir session dir: %v", err)
	}
	if err := os.MkdirAll(backendDir, 0o755); err != nil {
		t.Fatalf("mkdir backend dir: %v", err)
	}

	createBody, _ := json.Marshal(types.Workspace{
		RepoPath:              repoDir,
		SessionSubpath:        sessionSubpath,
		AdditionalDirectories: []string{"../backend"},
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

	startReq := StartSessionRequest{
		Provider: "gemini",
		Text:     "hello",
		Env:      []string{"ARCHON_EXEC_ARGS_FILE=" + argsFile},
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

	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, err := os.Stat(argsFile); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for args file")
		}
		time.Sleep(20 * time.Millisecond)
	}
	got, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("read args file: %v", err)
	}
	args := string(got)
	if !strings.Contains(args, "--include-directories") {
		t.Fatalf("expected --include-directories in args, got %q", args)
	}
	if !strings.Contains(args, backendDir) {
		t.Fatalf("expected backend path in args, got %q", args)
	}
	if !strings.Contains(args, "hello") {
		t.Fatalf("expected prompt text in args, got %q", args)
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
