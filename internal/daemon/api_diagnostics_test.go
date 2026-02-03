package daemon

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCodexThreadDiagnosticsRequiresParams(t *testing.T) {
	api := &API{Version: "test"}
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/diagnostics/codex/thread", api.CodexThreadDiagnostics)
	server := httptest.NewServer(TokenAuthMiddleware("token", mux))
	defer server.Close()

	req, _ := http.NewRequest(http.MethodGet, server.URL+"/v1/diagnostics/codex/thread", nil)
	req.Header.Set("Authorization", "Bearer token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("expected status 400, got %d", resp.StatusCode)
	}
}
