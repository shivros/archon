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
