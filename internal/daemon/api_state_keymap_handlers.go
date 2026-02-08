package daemon

import (
	"encoding/json"
	"net/http"

	"control/internal/types"
)

func (a *API) AppState(w http.ResponseWriter, r *http.Request) {
	service := NewAppStateService(a.Stores)
	switch r.Method {
	case http.MethodGet:
		state, err := service.Get(r.Context())
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, state)
		return
	case http.MethodPatch:
		var req types.AppState
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		if err := service.Update(r.Context(), &req); err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, req)
		return
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}

func (a *API) Keymap(w http.ResponseWriter, r *http.Request) {
	service := NewKeymapService(a.Stores)
	switch r.Method {
	case http.MethodGet:
		keymap, err := service.Get(r.Context())
		if err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, keymap)
		return
	case http.MethodPatch:
		var req types.Keymap
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}
		if err := service.Update(r.Context(), &req); err != nil {
			writeServiceError(w, err)
			return
		}
		writeJSON(w, http.StatusOK, req)
		return
	default:
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
	}
}
