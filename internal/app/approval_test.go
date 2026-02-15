package app

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"control/internal/types"
)

func TestPreserveApprovalPositionsReinsertsIntoOriginalGap(t *testing.T) {
	previous := []ChatBlock{
		{Role: ChatRoleUser, Text: "user one"},
		{Role: ChatRoleAgent, Text: "agent one"},
		{Role: ChatRoleApproval, ID: approvalBlockID(1), RequestID: 1, SessionID: "s1", Text: "Approval required: one"},
		{Role: ChatRoleApproval, ID: approvalBlockID(2), RequestID: 2, SessionID: "s1", Text: "Approval required: two"},
		{Role: ChatRoleAgent, Text: "agent two"},
		{Role: ChatRoleUser, Text: "user two"},
	}
	next := []ChatBlock{
		{Role: ChatRoleUser, Text: "user one"},
		{Role: ChatRoleAgent, Text: "agent one"},
		{Role: ChatRoleAgent, Text: "agent two"},
		{Role: ChatRoleUser, Text: "user two"},
		{Role: ChatRoleApproval, ID: approvalBlockID(1), RequestID: 1, SessionID: "s1", Text: "Approval required: one"},
		{Role: ChatRoleApproval, ID: approvalBlockID(2), RequestID: 2, SessionID: "s1", Text: "Approval required: two"},
	}

	got := preserveApprovalPositions(previous, next)
	want := []ChatRole{
		ChatRoleUser,
		ChatRoleAgent,
		ChatRoleApproval,
		ChatRoleApproval,
		ChatRoleAgent,
		ChatRoleUser,
	}
	if len(got) != len(want) {
		t.Fatalf("expected %d blocks, got %#v", len(want), got)
	}
	for i, role := range want {
		if got[i].Role != role {
			t.Fatalf("unexpected role at %d: got %s want %s (blocks=%#v)", i, got[i].Role, role, got)
		}
	}
	if got[2].RequestID != 1 || got[3].RequestID != 2 {
		t.Fatalf("unexpected approval order: %#v", got)
	}
}

func TestApprovalFromRecordBuildsRichCommandContext(t *testing.T) {
	params, err := json.Marshal(map[string]any{
		"permission_id": "perm-42",
		"session_id":    "remote-session",
		"parsedCmd":     "go test ./...",
		"metadata": map[string]any{
			"reason": "Run full suite before release",
			"cwd":    "/repo/worktree",
		},
	})
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}
	record := &types.Approval{
		SessionID: "s1",
		RequestID: 42,
		Method:    "item/commandExecution/requestApproval",
		Params:    params,
		CreatedAt: time.Date(2026, 2, 14, 12, 0, 0, 0, time.UTC),
	}

	req := approvalFromRecord(record)
	if req == nil {
		t.Fatalf("expected approval request")
	}
	if req.Summary != "command" {
		t.Fatalf("unexpected summary: %q", req.Summary)
	}
	if req.Detail != "go test ./..." {
		t.Fatalf("unexpected detail: %q", req.Detail)
	}
	if len(req.Context) == 0 {
		t.Fatalf("expected context lines, got %#v", req)
	}
	got := strings.Join(req.Context, "\n")
	for _, want := range []string{
		"Directory: /repo/worktree",
		"Reason: Run full suite before release",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing context line %q in %q", want, got)
		}
	}
	for _, blocked := range []string{"perm-42", "remote-session"} {
		if strings.Contains(got, blocked) {
			t.Fatalf("expected sensitive identifier %q to be hidden in %q", blocked, got)
		}
	}
}

func TestApprovalRequestToBlockRendersContextLines(t *testing.T) {
	req := &ApprovalRequest{
		RequestID: 9,
		SessionID: "s1",
		Summary:   "user input",
		Detail:    "Select deployment environment",
		Context: []string{
			"Question 2: Confirm region",
			"Options: staging | production",
		},
	}

	block := approvalRequestToBlock(req)
	if block.Role != ChatRoleApproval {
		t.Fatalf("unexpected block role: %s", block.Role)
	}
	for _, want := range []string{
		"Approval required: user input",
		"Select deployment environment",
		"Question 2: Confirm region",
		"Options: staging | production",
	} {
		if !strings.Contains(block.Text, want) {
			t.Fatalf("expected %q in block text:\n%s", want, block.Text)
		}
	}
}

func TestApprovalFromRecordBuildsExternalDirectoryRequest(t *testing.T) {
	params, err := json.Marshal(map[string]any{
		"permission_id": "per_hidden",
		"session_id":    "ses_hidden",
		"raw": map[string]any{
			"permission": "external_directory",
			"patterns": []any{
				"/home/shiv/go/pkg/mod/charm.land/bubbletea/v2@v2.0.0-rc.2/*",
				"/tmp/shared/*",
			},
		},
	})
	if err != nil {
		t.Fatalf("marshal params: %v", err)
	}
	record := &types.Approval{
		SessionID: "s1",
		RequestID: 111,
		Method:    "tool/requestUserInput",
		Params:    params,
		CreatedAt: time.Date(2026, 2, 14, 12, 0, 0, 0, time.UTC),
	}

	req := approvalFromRecord(record)
	if req == nil {
		t.Fatalf("expected approval request")
	}
	if req.Summary != "external directory access" {
		t.Fatalf("unexpected summary: %q", req.Summary)
	}
	if req.Detail != "/home/shiv/go/pkg/mod/charm.land/bubbletea/v2@v2.0.0-rc.2/*" {
		t.Fatalf("unexpected detail: %q", req.Detail)
	}
	contextText := strings.Join(req.Context, "\n")
	if !strings.Contains(contextText, "Target: /tmp/shared/*") {
		t.Fatalf("expected additional target in context, got %q", contextText)
	}
	if strings.Contains(contextText, "per_hidden") || strings.Contains(contextText, "ses_hidden") {
		t.Fatalf("expected hidden ids to stay out of display context: %q", contextText)
	}
}
