package daemon

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"control/internal/logging"
	"control/internal/types"
)

const sseHeartbeatInterval = 15 * time.Second

func writeSSEHeartbeat(w http.ResponseWriter, flusher http.Flusher) bool {
	if _, err := w.Write([]byte(":\n\n")); err != nil {
		return false
	}
	flusher.Flush()
	return true
}

type debugStreamService interface {
	ReadDebug(ctx context.Context, id string, lines int) ([]types.DebugEvent, bool, error)
	SubscribeDebug(ctx context.Context, id string) (<-chan types.DebugEvent, func(), error)
}

type tailStreamService interface {
	Subscribe(ctx context.Context, id, stream string) (<-chan types.LogEvent, func(), error)
}

func (a *API) streamTail(w http.ResponseWriter, r *http.Request, id string) {
	stream := r.URL.Query().Get("stream")
	a.streamTailWithService(w, r, id, stream, a.newSessionService())
}

func (a *API) streamTailWithService(w http.ResponseWriter, r *http.Request, id, stream string, service tailStreamService) {
	if service == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "streaming unsupported"})
		return
	}
	reqID := logging.NewRequestID()
	if a.Logger != nil && a.Logger.Enabled(logging.Debug) {
		a.Logger.Debug("tail_stream_open",
			logging.F("req_id", reqID),
			logging.F("session_id", id),
			logging.F("stream", stream),
		)
	}
	ch, cancel, err := service.Subscribe(r.Context(), id, stream)
	if err != nil {
		if a.Logger != nil {
			a.Logger.Warn("tail_stream_subscribe_error",
				logging.F("req_id", reqID),
				logging.F("session_id", id),
				logging.F("stream", stream),
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

	ctx := r.Context()
	heartbeat := time.NewTicker(sseHeartbeatInterval)
	defer heartbeat.Stop()
	reason := "unknown"
	count := 0
	defer func() {
		if a.Logger != nil && a.Logger.Enabled(logging.Debug) {
			a.Logger.Debug("tail_stream_close",
				logging.F("req_id", reqID),
				logging.F("session_id", id),
				logging.F("stream", stream),
				logging.F("count", count),
				logging.F("reason", reason),
			)
		}
	}()
	for {
		select {
		case <-ctx.Done():
			reason = "ctx_done"
			return
		case <-heartbeat.C:
			if !writeSSEHeartbeat(w, flusher) {
				reason = "heartbeat_write_error"
				return
			}
		case event, ok := <-ch:
			if !ok {
				reason = "channel_closed"
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
			count++
		}
	}
}

func (a *API) streamDebug(w http.ResponseWriter, r *http.Request, id string) {
	a.streamDebugWithService(w, r, id, a.newSessionService())
}

func (a *API) streamDebugWithService(w http.ResponseWriter, r *http.Request, id string, service debugStreamService) {
	if service == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "streaming unsupported"})
		return
	}
	lines := parseLines(r.URL.Query().Get("lines"))
	snapshot, _, snapErr := service.ReadDebug(r.Context(), id, lines)
	ch, cancel, err := service.SubscribeDebug(r.Context(), id)
	if err != nil && (snapErr != nil || len(snapshot) == 0) {
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

	if len(snapshot) > 0 {
		for _, event := range snapshot {
			data, err := json.Marshal(event)
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
