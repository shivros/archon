package daemon

import (
	"encoding/json"
	"net/http"

	"control/internal/logging"
)

func (a *API) streamTail(w http.ResponseWriter, r *http.Request, id string) {
	stream := r.URL.Query().Get("stream")
	service := a.newSessionService()
	ch, cancel, err := service.Subscribe(r.Context(), id, stream)
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
	for {
		select {
		case <-ctx.Done():
			return
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

func (a *API) streamEvents(w http.ResponseWriter, r *http.Request, id string) {
	service := a.newSessionService()
	reqID := logging.NewRequestID()
	if a.Logger != nil && a.Logger.Enabled(logging.Debug) {
		a.Logger.Debug("events_stream_open",
			logging.F("req_id", reqID),
			logging.F("session_id", id),
		)
	}
	ch, cancel, err := service.SubscribeEvents(r.Context(), id)
	if err != nil {
		if a.Logger != nil {
			a.Logger.Warn("events_stream_subscribe_error",
				logging.F("req_id", reqID),
				logging.F("session_id", id),
				logging.F("error", err),
			)
		}
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
	_, _ = w.Write([]byte(":\n\n"))
	flusher.Flush()

	ctx := r.Context()
	var count int
	firstMethod := ""
	reason := "unknown"
	defer func() {
		if a.Logger != nil && a.Logger.Enabled(logging.Debug) {
			a.Logger.Debug("events_stream_close",
				logging.F("req_id", reqID),
				logging.F("session_id", id),
				logging.F("count", count),
				logging.F("first_method", firstMethod),
				logging.F("reason", reason),
			)
		}
	}()
	for {
		select {
		case <-ctx.Done():
			reason = "ctx_done"
			return
		case event, ok := <-ch:
			if !ok {
				reason = "channel_closed"
				return
			}
			if count == 0 {
				firstMethod = event.Method
			}
			count++
			data, err := json.Marshal(event)
			if err != nil {
				continue
			}
			_, _ = w.Write([]byte("data: "))
			_, _ = w.Write(data)
			_, _ = w.Write([]byte("\n\n"))
			flusher.Flush()
			if event.Method == "turn/completed" {
				reason = "turn_completed"
				return
			}
		}
	}
}

func (a *API) streamItems(w http.ResponseWriter, r *http.Request, id string) {
	service := a.newSessionService()
	reqID := logging.NewRequestID()
	lines := parseLines(r.URL.Query().Get("lines"))
	snapshot, _, snapErr := service.readSessionItems(id, lines)
	ch, cancel, err := service.SubscribeItems(r.Context(), id)
	if err != nil && (snapErr != nil || len(snapshot) == 0) {
		if a.Logger != nil {
			a.Logger.Warn("items_stream_subscribe_error",
				logging.F("req_id", reqID),
				logging.F("session_id", id),
				logging.F("error", err),
			)
		}
		writeServiceError(w, err)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "streaming unsupported"})
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	if a.Logger != nil && a.Logger.Enabled(logging.Debug) {
		a.Logger.Debug("items_stream_open",
			logging.F("req_id", reqID),
			logging.F("session_id", id),
			logging.F("snapshot_items", len(snapshot)),
			logging.F("lines", lines),
			logging.F("subscribe_error", err != nil),
		)
	}

	if len(snapshot) > 0 {
		for _, item := range snapshot {
			data, err := json.Marshal(item)
			if err != nil {
				continue
			}
			_, _ = w.Write([]byte("data: "))
			_, _ = w.Write(data)
			_, _ = w.Write([]byte("\n\n"))
		}
		flusher.Flush()
	}
	if err != nil {
		return
	}
	defer cancel()

	ctx := r.Context()
	var count int
	firstType := ""
	reason := "unknown"
	defer func() {
		if a.Logger != nil && a.Logger.Enabled(logging.Debug) {
			a.Logger.Debug("items_stream_close",
				logging.F("req_id", reqID),
				logging.F("session_id", id),
				logging.F("count", count),
				logging.F("first_type", firstType),
				logging.F("reason", reason),
			)
		}
	}()
	for {
		select {
		case <-ctx.Done():
			reason = "ctx_done"
			return
		case item, ok := <-ch:
			if !ok {
				reason = "channel_closed"
				return
			}
			if count == 0 {
				if typ, _ := item["type"].(string); typ != "" {
					firstType = typ
				}
			}
			count++
			data, err := json.Marshal(item)
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
