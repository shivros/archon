package daemon

import (
	"encoding/json"
	"net/http"
	"strings"

	"control/internal/types"
)

func (a *API) WorkspaceGroups(w http.ResponseWriter, r *http.Request) {
	service := NewWorkspaceGroupService(a.Stores)

	switch r.Method {
	case http.MethodGet:
		groups, err := service.List(r.Context())
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"groups": groups})
		return
	case http.MethodPost:
		var req types.WorkspaceGroup
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		group, err := service.Create(r.Context(), &req)
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusCreated, group)
		return
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

func (a *API) WorkspaceGroupByID(w http.ResponseWriter, r *http.Request) {
	service := NewWorkspaceGroupService(a.Stores)
	path := strings.TrimPrefix(r.URL.Path, "/v1/workspace-groups/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 0 || parts[0] == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	id := parts[0]

	if len(parts) == 1 {
		switch r.Method {
		case http.MethodPatch:
			var req types.WorkspaceGroup
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
				return
			}
			group, err := service.Update(r.Context(), id, &req)
			if err != nil {
				writeServiceError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, group)
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

	writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
}
