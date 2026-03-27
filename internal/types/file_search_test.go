package types

import (
	"encoding/json"
	"testing"
)

func TestFileSearchStartRequestJSONShape(t *testing.T) {
	payload, err := json.Marshal(FileSearchStartRequest{
		Scope: FileSearchScope{
			Provider:    "codex",
			SessionID:   "sess-1",
			WorkspaceID: "ws-1",
			WorktreeID:  "wt-1",
			Cwd:         "/repo",
		},
		Query: "main",
		Limit: 7,
	})
	if err != nil {
		t.Fatalf("marshal start request: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("unmarshal start request: %v", err)
	}
	scope, _ := decoded["scope"].(map[string]any)
	if scope["provider"] != "codex" || scope["session_id"] != "sess-1" {
		t.Fatalf("unexpected scope payload: %#v", scope)
	}
	if decoded["query"] != "main" {
		t.Fatalf("unexpected query payload: %#v", decoded)
	}
	if decoded["limit"] != float64(7) {
		t.Fatalf("unexpected limit payload: %#v", decoded)
	}
}

func TestFileSearchUpdateRequestJSONShape(t *testing.T) {
	query := "main.go"
	limit := 11
	payload, err := json.Marshal(FileSearchUpdateRequest{
		Query: &query,
		Limit: &limit,
	})
	if err != nil {
		t.Fatalf("marshal update request: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(payload, &decoded); err != nil {
		t.Fatalf("unmarshal update request: %v", err)
	}
	if _, ok := decoded["scope"]; ok {
		t.Fatalf("expected scope to be omitted when unset")
	}
	if decoded["query"] != "main.go" {
		t.Fatalf("unexpected query payload: %#v", decoded)
	}
	if decoded["limit"] != float64(11) {
		t.Fatalf("unexpected limit payload: %#v", decoded)
	}
}
