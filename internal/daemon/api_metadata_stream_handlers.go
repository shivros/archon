package daemon

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"control/internal/logging"
)

func (a *API) MetadataStreamEndpoint(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if !isFollowRequest(r) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "follow=1 is required"})
		return
	}
	a.streamMetadataEvents(w, r, a.MetadataEvents)
}

func (a *API) streamMetadataEvents(w http.ResponseWriter, r *http.Request, service MetadataEventStreamService) {
	if service == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "metadata stream unavailable"})
		return
	}
	afterRevision := strings.TrimSpace(r.URL.Query().Get("after_revision"))
	if afterRevision == "" {
		afterRevision = strings.TrimSpace(r.Header.Get("Last-Event-ID"))
	}
	reqID := logging.NewRequestID()
	if a.Logger != nil && a.Logger.Enabled(logging.Debug) {
		a.Logger.Debug("metadata_stream_open",
			logging.F("req_id", reqID),
			logging.F("after_revision", afterRevision),
		)
	}

	ch, cancel, err := service.Subscribe(afterRevision)
	if err != nil {
		if err == errInvalidAfterRevision {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid after_revision"})
			return
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
			a.Logger.Debug("metadata_stream_close",
				logging.F("req_id", reqID),
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
			if strings.TrimSpace(event.Revision) != "" {
				_, _ = w.Write([]byte("id: "))
				_, _ = w.Write([]byte(strings.TrimSpace(event.Revision)))
				_, _ = w.Write([]byte("\n"))
			}
			_, _ = w.Write([]byte("data: "))
			_, _ = w.Write(data)
			_, _ = w.Write([]byte("\n\n"))
			flusher.Flush()
			count++
		}
	}
}
