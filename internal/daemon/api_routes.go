package daemon

import "net/http"

func (a *API) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/health", a.Health)
	mux.HandleFunc("/v1/sessions", a.Sessions)
	mux.HandleFunc("/v1/sessions/", a.SessionByID)
	mux.HandleFunc("/v1/workspaces", a.Workspaces)
	mux.HandleFunc("/v1/workspaces/", a.WorkspaceByID)
	mux.HandleFunc("/v1/workspace-groups", a.WorkspaceGroups)
	mux.HandleFunc("/v1/workspace-groups/", a.WorkspaceGroupByID)
	mux.HandleFunc("/v1/notes", a.Notes)
	mux.HandleFunc("/v1/notes/", a.NoteByID)
	mux.HandleFunc("/v1/state", a.AppState)
	mux.HandleFunc("/v1/keymap", a.Keymap)
	mux.HandleFunc("/v1/diagnostics/codex/thread", a.CodexThreadDiagnostics)
	mux.HandleFunc("/v1/shutdown", a.ShutdownDaemon)
}
