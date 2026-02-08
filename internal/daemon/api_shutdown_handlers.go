package daemon

import (
	"context"
	"net/http"
	"time"
)

func (a *API) ShutdownDaemon(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	if a.Shutdown == nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "shutdown not available"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = a.Shutdown(ctx)
	}()
}
