package daemon

import (
	"net/http"
	"strings"
)

func (a *API) CodexThreadDiagnostics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	cwd := strings.TrimSpace(r.URL.Query().Get("cwd"))
	threadID := strings.TrimSpace(r.URL.Query().Get("id"))
	workspaceID := strings.TrimSpace(r.URL.Query().Get("workspace_id"))
	if cwd == "" || threadID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "cwd and id are required"})
		return
	}
	workspacePath := ""
	if workspaceID != "" && a.Stores != nil && a.Stores.Workspaces != nil {
		if ws, ok, err := a.Stores.Workspaces.Get(r.Context(), workspaceID); err == nil && ok && ws != nil {
			workspacePath = ws.RepoPath
		}
	}
	codexHome := resolveCodexHome(cwd, workspacePath)
	client, err := startCodexAppServer(r.Context(), cwd, codexHome, a.Logger)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	defer client.Close()

	thread, err := client.ReadThread(r.Context(), threadID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	cwds := detectStringsByKey(thread, "cwd")
	writeJSON(w, http.StatusOK, map[string]any{
		"thread": thread,
		"cwds":   cwds,
	})
}
