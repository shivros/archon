package daemon

import (
	"testing"

	"control/internal/types"
)

func TestClaudeAccessToPermissionMode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		access types.AccessLevel
		want   string
	}{
		{name: "read_only", access: types.AccessReadOnly, want: "plan"},
		{name: "on_request", access: types.AccessOnRequest, want: "default"},
		{name: "full_access", access: types.AccessFull, want: "bypassPermissions"},
		{name: "empty", access: "", want: ""},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := claudeAccessToPermissionMode(tt.access)
			if got != tt.want {
				t.Fatalf("expected %q, got %q", tt.want, got)
			}
		})
	}
}

func TestExtractClaudeSendRequestRuntimeOptions(t *testing.T) {
	t.Parallel()

	payload := buildClaudeUserPayloadWithRuntime("hello", &types.SessionRuntimeOptions{
		Model:  "sonnet",
		Access: types.AccessFull,
	})
	text, options, err := extractClaudeSendRequest(payload)
	if err != nil {
		t.Fatalf("extractClaudeSendRequest: %v", err)
	}
	if text != "hello" {
		t.Fatalf("expected text hello, got %q", text)
	}
	if options == nil {
		t.Fatalf("expected runtime options to be present")
	}
	if options.Model != "sonnet" {
		t.Fatalf("expected model sonnet, got %q", options.Model)
	}
	if options.Access != types.AccessFull {
		t.Fatalf("expected access full_access, got %q", options.Access)
	}
}
