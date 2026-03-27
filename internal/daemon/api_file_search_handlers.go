package daemon

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"control/internal/types"
)

func (a *API) FileSearchesEndpoint(w http.ResponseWriter, r *http.Request) {
	service := a.fileSearchService()
	if service == nil {
		writeServiceError(w, unavailableError("file search service not available", nil))
		return
	}
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	var req types.FileSearchStartRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
		return
	}
	search, err := service.Start(r.Context(), req)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, search)
}

func (a *API) FileSearchByID(w http.ResponseWriter, r *http.Request) {
	service := a.fileSearchService()
	if service == nil {
		writeServiceError(w, unavailableError("file search service not available", nil))
		return
	}

	path := strings.TrimPrefix(r.URL.Path, "/v1/file-searches/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 0 || strings.TrimSpace(parts[0]) == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	id := strings.TrimSpace(parts[0])

	if len(parts) == 1 {
		switch r.Method {
		case http.MethodPatch:
			var req types.FileSearchUpdateRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
				return
			}
			search, err := service.Update(r.Context(), id, req)
			if err != nil {
				writeServiceError(w, err)
				return
			}
			writeJSON(w, http.StatusOK, search)
			return
		case http.MethodDelete:
			if err := service.Close(r.Context(), id); err != nil {
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

	if len(parts) == 2 && parts[1] == "events" {
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		if !isFollowRequest(r) {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "follow=1 is required"})
			return
		}
		a.streamFileSearchEvents(w, r, id, service)
		return
	}

	writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
}

func (a *API) streamFileSearchEvents(w http.ResponseWriter, r *http.Request, id string, service FileSearchService) {
	ch, cancel, err := service.Subscribe(r.Context(), id)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	defer cancel()

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "streaming unsupported"})
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	ctx := r.Context()
	heartbeat := time.NewTicker(sseHeartbeatInterval)
	defer heartbeat.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-heartbeat.C:
			if !writeSSEHeartbeat(w, flusher) {
				return
			}
		case event, ok := <-ch:
			if !ok {
				return
			}
			data, err := json.Marshal(event)
			if err != nil {
				continue
			}
			_, _ = w.Write([]byte("data: "))
			_, _ = w.Write(data)
			_, _ = w.Write([]byte("\n\n"))
			flusher.Flush()
		}
	}
}
