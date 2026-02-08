package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"control/internal/store"
	"control/internal/types"
)

func TestWorkspaceGroupCRUD(t *testing.T) {
	workspaces := store.NewFileWorkspaceStore(filepath.Join(t.TempDir(), "workspaces.json"))
	api := &API{Version: "test", Stores: &Stores{Groups: workspaces}}
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/workspace-groups", api.WorkspaceGroups)
	mux.HandleFunc("/v1/workspace-groups/", api.WorkspaceGroupByID)
	server := httptest.NewServer(TokenAuthMiddleware("token", mux))
	t.Cleanup(server.Close)

	createBody, _ := json.Marshal(types.WorkspaceGroup{Name: "Work"})
	req, _ := http.NewRequest(http.MethodPost, server.URL+"/v1/workspace-groups", bytes.NewReader(createBody))
	req.Header.Set("Authorization", "Bearer token")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("create group: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
	var created types.WorkspaceGroup
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		t.Fatalf("decode group: %v", err)
	}

	listReq, _ := http.NewRequest(http.MethodGet, server.URL+"/v1/workspace-groups", nil)
	listReq.Header.Set("Authorization", "Bearer token")
	listResp, err := http.DefaultClient.Do(listReq)
	if err != nil {
		t.Fatalf("list groups: %v", err)
	}
	defer listResp.Body.Close()
	if listResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", listResp.StatusCode)
	}
	var list struct {
		Groups []*types.WorkspaceGroup `json:"groups"`
	}
	if err := json.NewDecoder(listResp.Body).Decode(&list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(list.Groups) != 1 {
		t.Fatalf("expected 1 group")
	}

	updateBody, _ := json.Marshal(types.WorkspaceGroup{Name: "Personal"})
	updateReq, _ := http.NewRequest(http.MethodPatch, server.URL+"/v1/workspace-groups/"+created.ID, bytes.NewReader(updateBody))
	updateReq.Header.Set("Authorization", "Bearer token")
	updateResp, err := http.DefaultClient.Do(updateReq)
	if err != nil {
		t.Fatalf("update group: %v", err)
	}
	defer updateResp.Body.Close()
	if updateResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", updateResp.StatusCode)
	}

	deleteReq, _ := http.NewRequest(http.MethodDelete, server.URL+"/v1/workspace-groups/"+created.ID, nil)
	deleteReq.Header.Set("Authorization", "Bearer token")
	deleteResp, err := http.DefaultClient.Do(deleteReq)
	if err != nil {
		t.Fatalf("delete group: %v", err)
	}
	defer deleteResp.Body.Close()
	if deleteResp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", deleteResp.StatusCode)
	}

	// Ensure deletion
	ctx := context.Background()
	if _, ok, err := workspaces.GetGroup(ctx, created.ID); err != nil || ok {
		t.Fatalf("expected group deleted")
	}
}
