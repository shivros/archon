package daemon

import (
	"net/http"
	"os"
)

func (a *API) Health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":      true,
		"version": a.Version,
		"pid":     os.Getpid(),
	})
}
