package daemon

import (
	"context"
	"net/http"
	"strings"
	"time"

	"control/internal/types"
)

func (a *API) ProviderByName(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(r.URL.Path, "/v1/providers/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) < 2 || strings.TrimSpace(parts[0]) == "" || strings.TrimSpace(parts[1]) == "" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	if r.Method != http.MethodGet {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		return
	}
	provider := strings.TrimSpace(parts[0])
	section := strings.TrimSpace(parts[1])
	if section != "options" {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
		return
	}
	options := a.providerOptionsForRequest(r.Context(), provider, strings.TrimSpace(r.URL.Query().Get("cwd")), strings.TrimSpace(r.URL.Query().Get("workspace_id")))
	writeJSON(w, http.StatusOK, map[string]any{"options": options})
}

func (a *API) providerOptionsForRequest(ctx context.Context, provider, cwd, workspaceID string) *types.ProviderOptionCatalog {
	provider = strings.TrimSpace(provider)
	if strings.EqualFold(provider, "codex") {
		if dynamic := a.loadCodexDynamicOptionCatalog(ctx, cwd, workspaceID); dynamic != nil {
			return dynamic
		}
	}
	return providerOptionCatalog(provider)
}

func (a *API) loadCodexDynamicOptionCatalog(ctx context.Context, cwd, workspaceID string) *types.ProviderOptionCatalog {
	workspacePath := ""
	if workspaceID != "" && a.Stores != nil && a.Stores.Workspaces != nil {
		if ws, ok, err := a.Stores.Workspaces.Get(ctx, workspaceID); err == nil && ok && ws != nil {
			workspacePath = ws.RepoPath
		}
	}
	codexHome := resolveCodexHome(cwd, workspacePath)
	callCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	client, err := startCodexAppServer(callCtx, cwd, codexHome, a.Logger)
	if err != nil {
		return nil
	}
	defer client.Close()

	var all []codexModelSummary
	var cursor *string
	const maxPages = 10
	for page := 0; page < maxPages; page++ {
		result, err := client.ListModels(callCtx, cursor, 20)
		if err != nil {
			return nil
		}
		if result != nil && len(result.Data) > 0 {
			all = append(all, result.Data...)
		}
		if result == nil || result.NextCursor == nil || strings.TrimSpace(*result.NextCursor) == "" {
			break
		}
		cursor = result.NextCursor
	}
	if len(all) == 0 {
		return nil
	}
	return codexProviderOptionCatalogFromModels(all)
}
