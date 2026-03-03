package daemon

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"control/internal/daemon/transcriptdomain"
	"control/internal/logging"
)

type transcriptSnapshotService interface {
	GetTranscriptSnapshot(ctx context.Context, id string, lines int) (transcriptdomain.TranscriptSnapshot, error)
}

type transcriptStreamService interface {
	SubscribeTranscript(ctx context.Context, id string, after transcriptdomain.RevisionToken) (<-chan transcriptdomain.TranscriptEvent, func(), error)
}

func parseAfterRevision(raw string) (transcriptdomain.RevisionToken, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", nil
	}
	return transcriptdomain.ParseRevisionToken(trimmed)
}

func (a *API) transcriptSnapshot(w http.ResponseWriter, r *http.Request, id string) {
	a.transcriptSnapshotWithService(w, r, id, a.newSessionService())
}

func (a *API) transcriptSnapshotWithService(w http.ResponseWriter, r *http.Request, id string, service transcriptSnapshotService) {
	if service == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "transcript service unavailable"})
		return
	}
	lines := parseLines(r.URL.Query().Get("lines"))
	snapshot, err := service.GetTranscriptSnapshot(r.Context(), id, lines)
	if err != nil {
		writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, snapshot)
}

func (a *API) streamTranscript(w http.ResponseWriter, r *http.Request, id string) {
	a.streamTranscriptWithService(w, r, id, a.newSessionService())
}

func (a *API) streamTranscriptWithService(w http.ResponseWriter, r *http.Request, id string, service transcriptStreamService) {
	if service == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "transcript stream unavailable"})
		return
	}
	after, err := parseAfterRevision(r.URL.Query().Get("after_revision"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid after_revision"})
		return
	}
	reqID := logging.NewRequestID()
	if a.Logger != nil && a.Logger.Enabled(logging.Debug) {
		a.Logger.Debug("transcript_stream_open",
			logging.F("req_id", reqID),
			logging.F("session_id", id),
			logging.F("after_revision", after.String()),
		)
	}
	ch, cancel, err := service.SubscribeTranscript(r.Context(), id, after)
	if err != nil {
		if a.Logger != nil {
			a.Logger.Warn("transcript_stream_subscribe_error",
				logging.F("req_id", reqID),
				logging.F("session_id", id),
				logging.F("after_revision", after.String()),
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
			a.Logger.Debug("transcript_stream_close",
				logging.F("req_id", reqID),
				logging.F("session_id", id),
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
