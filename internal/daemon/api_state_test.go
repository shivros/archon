package daemon

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"control/internal/store"
	"control/internal/types"
)

func TestStateEndpoint(t *testing.T) {
	base := t.TempDir()
	state := store.NewFileAppStateStore(filepath.Join(base, "state.json"))
	stores := &Stores{AppState: state}
	api := &API{Version: "test", Stores: stores}

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/state", api.AppState)
	server := httptest.NewServer(TokenAuthMiddleware("token", mux))
	defer server.Close()

	statePayload := types.AppState{ActiveWorkspaceID: "ws", ActiveWorktreeID: "wt", SidebarCollapsed: true}
	body, _ := json.Marshal(statePayload)
	req, _ := http.NewRequest(http.MethodPatch, server.URL+"/v1/state", bytes.NewReader(body))
	req.Header.Set("Authorization", "Bearer token")
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("patch state: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	getReq, _ := http.NewRequest(http.MethodGet, server.URL+"/v1/state", nil)
	getReq.Header.Set("Authorization", "Bearer token")
	getResp, err := http.DefaultClient.Do(getReq)
	if err != nil {
		t.Fatalf("get state: %v", err)
	}
	defer getResp.Body.Close()
	if getResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", getResp.StatusCode)
	}
}
