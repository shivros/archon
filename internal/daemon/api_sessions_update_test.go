package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"control/internal/store"
	"control/internal/types"
)

func TestSessionTitleUpdate(t *testing.T) {
	manager := newTestManager(t)
	metaStore := store.NewFileSessionMetaStore(filepath.Join(t.TempDir(), "sessions_meta.json"))
	manager.SetMetaStore(metaStore)
	api := &API{Version: "test", Manager: manager, Stores: &Stores{SessionMeta: metaStore}}

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/sessions", api.Sessions)
	mux.HandleFunc("/v1/sessions/", api.SessionByID)
	server := httptest.NewServer(TokenAuthMiddleware("token", mux))
	defer server.Close()

	startReq := StartSessionRequest{
		Provider: "custom",
		Cmd:      os.Args[0],
		Args:     helperArgs("stdout=hello", "exit=0"),
		Env:      []string{"GO_WANT_HELPER_PROCESS=1"},
		Title:    "",
	}
	body, _ := json.Marshal(startReq)
	httpReq, _ := http.NewRequest(http.MethodPost, server.URL+"/v1/sessions", bytes.NewReader(body))
	httpReq.Header.Set("Authorization", "Bearer token")
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(httpReq)
	if err != nil {
		t.Fatalf("start session: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
	var session types.Session
	if err := json.NewDecoder(resp.Body).Decode(&session); err != nil {
		t.Fatalf("decode session: %v", err)
	}

	waitForStatus(t, manager, session.ID, types.SessionStatusExited, 2*time.Second)

	updateReq := UpdateSessionRequest{Title: "Custom Title"}
	updateBody, _ := json.Marshal(updateReq)
	patchReq, _ := http.NewRequest(http.MethodPatch, server.URL+"/v1/sessions/"+session.ID, bytes.NewReader(updateBody))
	patchReq.Header.Set("Authorization", "Bearer token")
	patchReq.Header.Set("Content-Type", "application/json")
	patchResp, err := http.DefaultClient.Do(patchReq)
	if err != nil {
		t.Fatalf("patch session: %v", err)
	}
	defer patchResp.Body.Close()
	if patchResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", patchResp.StatusCode)
	}

	meta, ok, err := metaStore.Get(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("get meta: %v", err)
	}
	if !ok {
		t.Fatalf("expected meta to exist")
	}
	if meta.Title != "Custom Title" {
		t.Fatalf("expected updated title")
	}

	runtimeUpdateReq := UpdateSessionRequest{
		RuntimeOptions: &types.SessionRuntimeOptions{
			Model:     "gpt-5.2-codex",
			Reasoning: types.ReasoningHigh,
			Access:    types.AccessOnRequest,
		},
	}
	runtimeBody, _ := json.Marshal(runtimeUpdateReq)
	runtimeReq, _ := http.NewRequest(http.MethodPatch, server.URL+"/v1/sessions/"+session.ID, bytes.NewReader(runtimeBody))
	runtimeReq.Header.Set("Authorization", "Bearer token")
	runtimeReq.Header.Set("Content-Type", "application/json")
	runtimeResp, err := http.DefaultClient.Do(runtimeReq)
	if err != nil {
		t.Fatalf("patch runtime options: %v", err)
	}
	defer runtimeResp.Body.Close()
	if runtimeResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", runtimeResp.StatusCode)
	}

	meta, ok, err = metaStore.Get(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("get meta runtime options: %v", err)
	}
	if !ok || meta == nil || meta.RuntimeOptions == nil {
		t.Fatalf("expected runtime options to persist")
	}
	if meta.RuntimeOptions.Model != "gpt-5.2-codex" {
		t.Fatalf("expected runtime model to persist, got %q", meta.RuntimeOptions.Model)
	}
	if meta.RuntimeOptions.Reasoning != types.ReasoningHigh {
		t.Fatalf("expected runtime reasoning to persist, got %q", meta.RuntimeOptions.Reasoning)
	}
	if meta.RuntimeOptions.Access != types.AccessOnRequest {
		t.Fatalf("expected runtime access to persist, got %q", meta.RuntimeOptions.Access)
	}

	enabled := false
	notificationUpdateReq := UpdateSessionRequest{
		NotificationOverrides: &types.NotificationSettingsPatch{
			Enabled:  &enabled,
			Methods:  []types.NotificationMethod{types.NotificationMethodBell},
			Triggers: []types.NotificationTrigger{types.NotificationTriggerSessionFailed},
		},
	}
	notificationBody, _ := json.Marshal(notificationUpdateReq)
	notificationReq, _ := http.NewRequest(http.MethodPatch, server.URL+"/v1/sessions/"+session.ID, bytes.NewReader(notificationBody))
	notificationReq.Header.Set("Authorization", "Bearer token")
	notificationReq.Header.Set("Content-Type", "application/json")
	notificationResp, err := http.DefaultClient.Do(notificationReq)
	if err != nil {
		t.Fatalf("patch notification overrides: %v", err)
	}
	defer notificationResp.Body.Close()
	if notificationResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", notificationResp.StatusCode)
	}

	meta, ok, err = metaStore.Get(context.Background(), session.ID)
	if err != nil {
		t.Fatalf("get meta notification overrides: %v", err)
	}
	if !ok || meta == nil || meta.NotificationOverrides == nil {
		t.Fatalf("expected notification overrides to persist")
	}
	if meta.NotificationOverrides.Enabled == nil || *meta.NotificationOverrides.Enabled {
		t.Fatalf("expected notification enabled=false to persist")
	}
	if len(meta.NotificationOverrides.Methods) != 1 || meta.NotificationOverrides.Methods[0] != types.NotificationMethodBell {
		t.Fatalf("expected notification methods to persist, got %#v", meta.NotificationOverrides.Methods)
	}
	if len(meta.NotificationOverrides.Triggers) != 1 || meta.NotificationOverrides.Triggers[0] != types.NotificationTriggerSessionFailed {
		t.Fatalf("expected notification triggers to persist, got %#v", meta.NotificationOverrides.Triggers)
	}
}
