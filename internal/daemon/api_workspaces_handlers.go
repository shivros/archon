package daemon

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"control/internal/types"
)

func (a *API) Workspaces(w http.ResponseWriter, r *http.Request) {
	service := NewWorkspaceSyncService(a.Stores, a.Syncer)

	switch r.Method {
	case http.MethodGet:
		workspaces, err := service.List(r.Context())
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"workspaces": workspaces})
		return
	case http.MethodPost:
		var req types.Workspace
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		ws, err := service.Create(r.Context(), &req)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, ws)
		return
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

func (a *API) WorkspaceByID(w http.ResponseWriter, r *http.Request) {
	service := NewWorkspaceSyncService(a.Stores, a.Syncer)
	path := strings.TrimPrefix(r.URL.Path, "/v1/workspaces/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	id := parts[0]

	if len(parts) == 1 {
		switch r.Method {
		case http.MethodPatch:
			var req types.Workspace
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
				return
			}
			ws, err := service.Update(r.Context(), id, &req)
			if err != nil {
				writeServiceError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, ws)
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
			return
		}
	}

	if parts[1] == "worktrees" {
		a.Worktrees(w, r, id, parts)
		return
	}
	if parts[1] == "sessions" {
		a.startSessionForWorkspace(w, r, id, "")
		return
	}

	writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
}

func (a *API) Worktrees(w http.ResponseWriter, r *http.Request, workspaceID string, parts []string) {
	service := NewWorkspaceSyncService(a.Stores, a.Syncer)

	if len(parts) == 2 {
		switch r.Method {
		case http.MethodGet:
			worktrees, err := service.ListWorktrees(r.Context(), workspaceID)
			if err != nil {
				writeServiceError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"worktrees": worktrees})
			return
		case http.MethodPost:
			var req types.Worktree
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
				return
			}
			wt, err := service.AddWorktree(r.Context(), workspaceID, &req)
			if err != nil {
				writeServiceError(w, err)
				return
			}
			writeJSON(w, http.StatusCreated, wt)
			return
		default:
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
	}

	if len(parts) == 3 {
		switch parts[2] {
		case "available":
			if r.Method != http.MethodGet {
				writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
				return
			}
			worktrees, err := service.ListAvailableWorktrees(r.Context(), workspaceID)
			if err != nil {
				writeServiceError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, map[string]any{"worktrees": worktrees})
			return
		case "create":
			if r.Method != http.MethodPost {
				writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
				return
			}
			var req CreateWorktreeRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
				return
			}
			wt, err := service.CreateWorktree(r.Context(), workspaceID, &req)
			if err != nil {
				writeServiceError(w, err)
				return
			}
			writeJSON(w, http.StatusCreated, wt)
			return
		default:
			worktreeID := parts[2]
			switch r.Method {
			case http.MethodPatch:
				var req types.Worktree
				if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
					writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
					return
				}
				wt, err := service.UpdateWorktree(r.Context(), workspaceID, worktreeID, &req)
				if err != nil {
					writeServiceError(w, err)
					return
				}
				writeJSON(w, http.StatusOK, wt)
				return
			case http.MethodDelete:
				if err := service.DeleteWorktree(r.Context(), workspaceID, worktreeID); err != nil {
					writeServiceError(w, err)
					return
				}
				writeJSON(w, http.StatusOK, map[string]any{"ok": true})
				return
			default:
				writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
				return
			}
		}
	}
	if len(parts) == 4 && parts[3] == "sessions" {
		a.startSessionForWorkspace(w, r, workspaceID, parts[2])
		return
	}

	writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
}

func (a *API) startSessionForWorkspace(w http.ResponseWriter, r *http.Request, workspaceID, worktreeID string) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	service := a.newSessionService()
	var req StartSessionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
		return
	}
	provider := strings.TrimSpace(req.Provider)
	if provider == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "provider is required"})
		return
	}
	req.Provider = provider
	req.WorkspaceID = workspaceID
	req.WorktreeID = worktreeID
	session, err := service.Start(r.Context(), req)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, session)
}
