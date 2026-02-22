package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"control/internal/types"
)

func TestClientUpdateWorkspaceSendsPatchFields(t *testing.T) {
	var (
		seenMethod string
		seenPath   string
		seenBody   map[string]any
	)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenMethod = r.Method
		seenPath = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&seenBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"ws1","name":"Renamed","repo_path":"/tmp/repo-2","session_subpath":"","additional_directories":[],"created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"}`))
	}))
	defer server.Close()

	c := &Client{
		baseURL: server.URL,
		token:   "token",
		http: &http.Client{
			Timeout: 2 * time.Second,
		},
	}

	name := "Renamed"
	repoPath := "/tmp/repo-2"
	sessionSubpath := ""
	additionalDirectories := []string{}
	groupIDs := []string{}
	workspace, err := c.UpdateWorkspace(context.Background(), "ws1", &types.WorkspacePatch{
		Name:                  &name,
		RepoPath:              &repoPath,
		SessionSubpath:        &sessionSubpath,
		AdditionalDirectories: &additionalDirectories,
		GroupIDs:              &groupIDs,
	})
	if err != nil {
		t.Fatalf("UpdateWorkspace error: %v", err)
	}
	if workspace == nil {
		t.Fatalf("expected updated workspace")
	}
	if seenMethod != http.MethodPatch {
		t.Fatalf("expected PATCH method, got %s", seenMethod)
	}
	if seenPath != "/v1/workspaces/ws1" {
		t.Fatalf("unexpected path: %s", seenPath)
	}
	if got, ok := seenBody["repo_path"].(string); !ok || got != repoPath {
		t.Fatalf("expected repo_path %q, got %#v", repoPath, seenBody["repo_path"])
	}
	if got, ok := seenBody["session_subpath"].(string); !ok || got != "" {
		t.Fatalf("expected session_subpath to be empty string, got %#v", seenBody["session_subpath"])
	}
	dirs, ok := seenBody["additional_directories"].([]any)
	if !ok {
		t.Fatalf("expected additional_directories array, got %#v", seenBody["additional_directories"])
	}
	if len(dirs) != 0 {
		t.Fatalf("expected additional_directories to be empty, got %#v", dirs)
	}
	groups, ok := seenBody["group_ids"].([]any)
	if !ok {
		t.Fatalf("expected group_ids array, got %#v", seenBody["group_ids"])
	}
	if len(groups) != 0 {
		t.Fatalf("expected group_ids to be empty, got %#v", groups)
	}
}

func TestClientUpdateWorkspaceRejectsNilPatch(t *testing.T) {
	c := &Client{}
	if _, err := c.UpdateWorkspace(context.Background(), "ws1", nil); err == nil {
		t.Fatalf("expected nil patch to fail")
	}
}

func TestClientUpdateWorkspaceRejectsBlankWorkspaceID(t *testing.T) {
	c := &Client{}
	patch := &types.WorkspacePatch{}
	if _, err := c.UpdateWorkspace(context.Background(), "   ", patch); err == nil {
		t.Fatalf("expected blank workspace id to fail")
	}
}
