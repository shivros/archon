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

func TestNotesEndpointsCRUD(t *testing.T) {
	stores := newNotesTestStores(t)
	server := newNotesTestServer(stores)
	defer server.Close()

	workspaceID := seedWorkspace(t, stores)

	createBody, _ := json.Marshal(types.Note{
		Kind:        types.NoteKindNote,
		Scope:       types.NoteScopeWorkspace,
		WorkspaceID: workspaceID,
		Body:        "Remember this decision.",
		Status:      types.NoteStatusIdea,
	})
	createReq, _ := http.NewRequest(http.MethodPost, server.URL+"/v1/notes", bytes.NewReader(createBody))
	createReq.Header.Set("Authorization", "Bearer token")
	createReq.Header.Set("Content-Type", "application/json")
	createResp, err := http.DefaultClient.Do(createReq)
	if err != nil {
		t.Fatalf("create note: %v", err)
	}
	defer createResp.Body.Close()
	if createResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", createResp.StatusCode)
	}
	var created types.Note
	if err := json.NewDecoder(createResp.Body).Decode(&created); err != nil {
		t.Fatalf("decode create: %v", err)
	}
	if created.ID == "" {
		t.Fatalf("expected note id")
	}

	listReq, _ := http.NewRequest(http.MethodGet, server.URL+"/v1/notes?scope=workspace&workspace_id="+workspaceID, nil)
	listReq.Header.Set("Authorization", "Bearer token")
	listResp, err := http.DefaultClient.Do(listReq)
	if err != nil {
		t.Fatalf("list notes: %v", err)
	}
	defer listResp.Body.Close()
	if listResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", listResp.StatusCode)
	}
	var listPayload struct {
		Notes []*types.Note `json:"notes"`
	}
	if err := json.NewDecoder(listResp.Body).Decode(&listPayload); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(listPayload.Notes) != 1 {
		t.Fatalf("expected 1 note, got %d", len(listPayload.Notes))
	}

	updateBody, _ := json.Marshal(types.Note{Status: types.NoteStatusTodo, Body: "Updated"})
	updateReq, _ := http.NewRequest(http.MethodPatch, server.URL+"/v1/notes/"+created.ID, bytes.NewReader(updateBody))
	updateReq.Header.Set("Authorization", "Bearer token")
	updateReq.Header.Set("Content-Type", "application/json")
	updateResp, err := http.DefaultClient.Do(updateReq)
	if err != nil {
		t.Fatalf("update note: %v", err)
	}
	defer updateResp.Body.Close()
	if updateResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", updateResp.StatusCode)
	}
	var updated types.Note
	if err := json.NewDecoder(updateResp.Body).Decode(&updated); err != nil {
		t.Fatalf("decode update: %v", err)
	}
	if updated.Status != types.NoteStatusTodo {
		t.Fatalf("expected status todo, got %s", updated.Status)
	}

	deleteReq, _ := http.NewRequest(http.MethodDelete, server.URL+"/v1/notes/"+created.ID, nil)
	deleteReq.Header.Set("Authorization", "Bearer token")
	deleteResp, err := http.DefaultClient.Do(deleteReq)
	if err != nil {
		t.Fatalf("delete note: %v", err)
	}
	defer deleteResp.Body.Close()
	if deleteResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", deleteResp.StatusCode)
	}
}

func TestSessionPinsEndpointCreatesPin(t *testing.T) {
	stores := newNotesTestStores(t)
	server := newNotesTestServer(stores)
	defer server.Close()

	workspaceID := seedWorkspace(t, stores)
	seedSession(t, stores, "s-note-1", workspaceID)

	pinBody, _ := json.Marshal(PinSessionRequest{
		SourceBlockID: "assistant-1",
		SourceRole:    "assistant",
		SourceSnippet: "Use a workspace-scoped note for this.",
		Body:          "Useful direction",
	})
	pinReq, _ := http.NewRequest(http.MethodPost, server.URL+"/v1/sessions/s-note-1/pins", bytes.NewReader(pinBody))
	pinReq.Header.Set("Authorization", "Bearer token")
	pinReq.Header.Set("Content-Type", "application/json")
	pinResp, err := http.DefaultClient.Do(pinReq)
	if err != nil {
		t.Fatalf("create pin: %v", err)
	}
	defer pinResp.Body.Close()
	if pinResp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", pinResp.StatusCode)
	}
	var created types.Note
	if err := json.NewDecoder(pinResp.Body).Decode(&created); err != nil {
		t.Fatalf("decode pin: %v", err)
	}
	if created.Kind != types.NoteKindPin {
		t.Fatalf("expected pin kind, got %s", created.Kind)
	}
	if created.Scope != types.NoteScopeSession || created.SessionID != "s-note-1" {
		t.Fatalf("unexpected scope/session: scope=%s session=%s", created.Scope, created.SessionID)
	}
	if created.Source == nil || created.Source.Snippet == "" || created.Source.SessionID != "s-note-1" {
		t.Fatalf("expected source metadata to be populated")
	}

	listReq, _ := http.NewRequest(http.MethodGet, server.URL+"/v1/notes?session_id=s-note-1", nil)
	listReq.Header.Set("Authorization", "Bearer token")
	listResp, err := http.DefaultClient.Do(listReq)
	if err != nil {
		t.Fatalf("list notes: %v", err)
	}
	defer listResp.Body.Close()
	if listResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", listResp.StatusCode)
	}
	var listPayload struct {
		Notes []*types.Note `json:"notes"`
	}
	if err := json.NewDecoder(listResp.Body).Decode(&listPayload); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(listPayload.Notes) != 1 {
		t.Fatalf("expected 1 note, got %d", len(listPayload.Notes))
	}
}

func newNotesTestServer(stores *Stores) *httptest.Server {
	api := &API{Version: "test", Stores: stores}
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/notes", api.Notes)
	mux.HandleFunc("/v1/notes/", api.NoteByID)
	mux.HandleFunc("/v1/sessions/", api.SessionByID)
	return httptest.NewServer(TokenAuthMiddleware("token", mux))
}

func newNotesTestStores(t *testing.T) *Stores {
	t.Helper()
	base := t.TempDir()
	workspaces := store.NewFileWorkspaceStore(filepath.Join(base, "workspaces.json"))
	meta := store.NewFileSessionMetaStore(filepath.Join(base, "sessions_meta.json"))
	sessions := store.NewFileSessionIndexStore(filepath.Join(base, "sessions_index.json"))
	notes := store.NewFileNoteStore(filepath.Join(base, "notes.json"))
	return &Stores{
		Workspaces:  workspaces,
		Worktrees:   workspaces,
		Groups:      workspaces,
		SessionMeta: meta,
		Sessions:    sessions,
		Notes:       notes,
	}
}

func seedWorkspace(t *testing.T, stores *Stores) string {
	t.Helper()
	repoDir := filepath.Join(t.TempDir(), "repo")
	if err := os.MkdirAll(repoDir, 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	ws, err := stores.Workspaces.Add(context.Background(), &types.Workspace{RepoPath: repoDir, Name: "test"})
	if err != nil {
		t.Fatalf("seed workspace: %v", err)
	}
	return ws.ID
}

func seedSession(t *testing.T, stores *Stores, sessionID, workspaceID string) {
	t.Helper()
	now := time.Now().UTC()
	if _, err := stores.Sessions.UpsertRecord(context.Background(), &types.SessionRecord{
		Session: &types.Session{
			ID:        sessionID,
			Provider:  "codex",
			Cmd:       "codex app-server",
			Status:    types.SessionStatusRunning,
			CreatedAt: now,
		},
		Source: "codex",
	}); err != nil {
		t.Fatalf("seed session record: %v", err)
	}
	if _, err := stores.SessionMeta.Upsert(context.Background(), &types.SessionMeta{
		SessionID:   sessionID,
		WorkspaceID: workspaceID,
	}); err != nil {
		t.Fatalf("seed session meta: %v", err)
	}
}
