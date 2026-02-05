package daemon

import (
	"net/http"
	"time"

	"control/internal/logging"
)

type responseRecorder struct {
	http.ResponseWriter
	status int
	bytes  int
}

func (r *responseRecorder) Flush() {
	if flusher, ok := r.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (r *responseRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func (r *responseRecorder) Write(p []byte) (int, error) {
	if r.status == 0 {
		r.status = http.StatusOK
	}
	n, err := r.ResponseWriter.Write(p)
	r.bytes += n
	return n, err
}

func LoggingMiddleware(logger logging.Logger, next http.Handler) http.Handler {
	if logger == nil {
		logger = logging.Nop()
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reqID := r.Header.Get("X-Request-Id")
		if reqID == "" {
			reqID = logging.NewRequestID()
		}
		w.Header().Set("X-Request-Id", reqID)
		start := time.Now()
		rec := &responseRecorder{ResponseWriter: w}
		next.ServeHTTP(rec, r)
		latency := time.Since(start)
		logger.Info("http_request",
			logging.F("request_id", reqID),
			logging.F("method", r.Method),
			logging.F("path", r.URL.Path),
			logging.F("status", rec.status),
			logging.F("bytes", rec.bytes),
			logging.F("latency_ms", latency.Milliseconds()),
		)
	})
}
