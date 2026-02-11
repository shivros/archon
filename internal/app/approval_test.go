package app

import "testing"

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
