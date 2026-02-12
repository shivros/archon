package daemon

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"control/internal/types"
)

func TestAPIProviderOptionsEndpoint(t *testing.T) {
	api := &API{Version: "test"}
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/providers/", api.ProviderByName)
	server := httptest.NewServer(TokenAuthMiddleware("token", mux))
	defer server.Close()

	req, _ := http.NewRequest(http.MethodGet, server.URL+"/v1/providers/codex/options", nil)
	req.Header.Set("Authorization", "Bearer token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("get provider options: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var payload struct {
		Options *types.ProviderOptionCatalog `json:"options"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Options == nil {
		t.Fatalf("expected options payload")
	}
	if payload.Options.Provider != "codex" {
		t.Fatalf("expected codex provider, got %q", payload.Options.Provider)
	}
	if len(payload.Options.Models) == 0 {
		t.Fatalf("expected non-empty model catalog")
	}
	if len(payload.Options.AccessLevels) == 0 {
		t.Fatalf("expected non-empty access catalog")
	}
}

func TestAPIProviderOptionsEndpointClaude(t *testing.T) {
	api := &API{Version: "test"}
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/providers/", api.ProviderByName)
	server := httptest.NewServer(TokenAuthMiddleware("token", mux))
	defer server.Close()

	req, _ := http.NewRequest(http.MethodGet, server.URL+"/v1/providers/claude/options", nil)
	req.Header.Set("Authorization", "Bearer token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("get provider options: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var payload struct {
		Options *types.ProviderOptionCatalog `json:"options"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Options == nil {
		t.Fatalf("expected options payload")
	}
	if payload.Options.Provider != "claude" {
		t.Fatalf("expected claude provider, got %q", payload.Options.Provider)
	}
	if len(payload.Options.Models) == 0 {
		t.Fatalf("expected non-empty model catalog")
	}
	if len(payload.Options.AccessLevels) == 0 {
		t.Fatalf("expected non-empty access catalog")
	}
}

func TestAPIProviderOptionsEndpointOpenCodeDynamic(t *testing.T) {
	modelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/config/providers" {
			http.NotFound(w, r)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"providers": []map[string]any{
				{
					"id": "anthropic",
					"models": []map[string]any{
						{"id": "claude-sonnet-4-20250514"},
						{"id": "claude-opus-4-20250514"},
					},
				},
			},
			"default": map[string]any{
				"anthropic": "claude-sonnet-4-20250514",
			},
		})
	}))
	defer modelServer.Close()

	t.Setenv("OPENCODE_BASE_URL", modelServer.URL)
	t.Setenv("HOME", t.TempDir())
	api := &API{Version: "test"}
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/providers/", api.ProviderByName)
	server := httptest.NewServer(TokenAuthMiddleware("token", mux))
	defer server.Close()

	req, _ := http.NewRequest(http.MethodGet, server.URL+"/v1/providers/opencode/options", nil)
	req.Header.Set("Authorization", "Bearer token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("get provider options: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var payload struct {
		Options *types.ProviderOptionCatalog `json:"options"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if payload.Options == nil {
		t.Fatalf("expected options payload")
	}
	if payload.Options.Provider != "opencode" {
		t.Fatalf("expected opencode provider, got %q", payload.Options.Provider)
	}
	if len(payload.Options.Models) != 2 {
		t.Fatalf("expected two dynamic models, got %#v", payload.Options.Models)
	}
	if payload.Options.Defaults.Model != "anthropic/claude-sonnet-4-20250514" {
		t.Fatalf("unexpected dynamic default model: %q", payload.Options.Defaults.Model)
	}
}
