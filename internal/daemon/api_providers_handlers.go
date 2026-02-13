package daemon

import (
	"context"
	"net/http"
	"strings"
	"time"

	"control/internal/providers"
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
	if strings.EqualFold(provider, "opencode") || strings.EqualFold(provider, "kilocode") {
		if dynamic := a.loadOpenCodeDynamicOptionCatalog(ctx, provider); dynamic != nil {
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

func (a *API) loadOpenCodeDynamicOptionCatalog(ctx context.Context, provider string) *types.ProviderOptionCatalog {
	coreCfg := loadCoreConfigOrDefault()
	cfg := resolveOpenCodeClientConfig(provider, coreCfg)
	client, err := newOpenCodeClient(cfg)
	if err != nil {
		return nil
	}
	callCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	catalog, err := loadOpenCodeModelCatalog(callCtx, provider, client)
	if err != nil || catalog == nil {
		return nil
	}
	name := providers.Normalize(provider)
	out := &types.ProviderOptionCatalog{
		Provider: name,
		Models:   append([]string{}, catalog.Models...),
		Defaults: types.SessionRuntimeOptions{
			Model:   strings.TrimSpace(catalog.DefaultModel),
			Version: 1,
		},
	}
	if out.Defaults.Model == "" && len(out.Models) > 0 {
		out.Defaults.Model = out.Models[0]
	}
	return out
}

func loadOpenCodeModelCatalog(ctx context.Context, provider string, client *openCodeClient) (*openCodeModelCatalog, error) {
	if client == nil {
		return nil, nil
	}
	catalog, err := client.ListModels(ctx)
	if err == nil {
		return catalog, nil
	}
	if !isOpenCodeUnreachable(err) {
		return nil, err
	}
	startedBaseURL, startErr := maybeAutoStartOpenCodeServer(provider, client.baseURL, client.token, nil)
	if startErr != nil {
		return nil, startErr
	}
	if switchedClient, switchErr := cloneOpenCodeClientWithBaseURL(client, startedBaseURL); switchErr == nil {
		client = switchedClient
	}
	retryDelay := 250 * time.Millisecond
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(retryDelay):
		}
		catalog, retryErr := client.ListModels(ctx)
		if retryErr == nil {
			return catalog, nil
		}
		if !isOpenCodeUnreachable(retryErr) {
			return nil, retryErr
		}
		if retryDelay < 2*time.Second {
			retryDelay *= 2
		}
	}
}
