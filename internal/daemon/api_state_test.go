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

func TestStateAndKeymapEndpoints(t *testing.T) {
	base := t.TempDir()
	state := store.NewFileAppStateStore(filepath.Join(base, "state.json"))
	keymap := store.NewFileKeymapStore(filepath.Join(base, "keymap.json"))
	stores := &Stores{AppState: state, Keymap: keymap}
	api := &API{Version: "test", Stores: stores}

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/state", api.AppState)
	mux.HandleFunc("/v1/keymap", api.Keymap)
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

	keymapPayload := types.Keymap{Bindings: map[string]string{types.KeyActionToggleSidebar: "ctrl+b"}}
	keyBody, _ := json.Marshal(keymapPayload)
	keyReq, _ := http.NewRequest(http.MethodPatch, server.URL+"/v1/keymap", bytes.NewReader(keyBody))
	keyReq.Header.Set("Authorization", "Bearer token")
	keyReq.Header.Set("Content-Type", "application/json")
	keyResp, err := http.DefaultClient.Do(keyReq)
	if err != nil {
		t.Fatalf("patch keymap: %v", err)
	}
	defer keyResp.Body.Close()
	if keyResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", keyResp.StatusCode)
	}
}
