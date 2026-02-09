package daemon

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"control/internal/store"
	"control/internal/types"
)

func (a *API) Notes(w http.ResponseWriter, r *http.Request) {
	service := NewNoteService(a.Stores)
	switch r.Method {
	case http.MethodGet:
		filter := store.NoteFilter{
			Scope:       types.NoteScope(strings.TrimSpace(r.URL.Query().Get("scope"))),
			WorkspaceID: strings.TrimSpace(r.URL.Query().Get("workspace_id")),
			WorktreeID:  strings.TrimSpace(r.URL.Query().Get("worktree_id")),
			SessionID:   strings.TrimSpace(r.URL.Query().Get("session_id")),
		}
		notes, err := service.List(r.Context(), filter)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"notes": notes})
		return
	case http.MethodPost:
		var req types.Note
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		note, err := service.Create(r.Context(), &req)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, note)
		return
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

func (a *API) NoteByID(w http.ResponseWriter, r *http.Request) {
	service := NewNoteService(a.Stores)
	path := strings.TrimPrefix(r.URL.Path, "/v1/notes/")
	id := strings.TrimSpace(strings.Trim(path, "/"))
	if id == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}

	switch r.Method {
	case http.MethodPatch:
		var req types.Note
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		note, err := service.Update(r.Context(), id, &req)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, note)
		return
	case http.MethodDelete:
		if err := service.Delete(r.Context(), id); err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

func (a *API) SessionPins(w http.ResponseWriter, r *http.Request, sessionID string) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	service := NewNoteService(a.Stores)
	var req PinSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
		return
	}
	note, err := service.PinSession(r.Context(), sessionID, &req)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, note)
}
