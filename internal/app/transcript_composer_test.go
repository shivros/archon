package app

import "testing"

func TestDefaultTranscriptComposerAppendOptimisticUser(t *testing.T) {
	composer := NewDefaultTranscriptComposer()
	base := []ChatBlock{{Role: ChatRoleAgent, Text: "hello"}}

	blocks, idx := composer.AppendOptimisticUser(base, "hi")
	if idx != 1 {
		t.Fatalf("expected header index 1, got %d", idx)
	}
	if len(blocks) != 2 {
		t.Fatalf("expected two blocks, got %#v", blocks)
	}
	if blocks[1].Role != ChatRoleUser || blocks[1].Status != ChatStatusSending {
		t.Fatalf("expected sending user block, got %#v", blocks[1])
	}
}

func TestDefaultTranscriptComposerMarkUserStatus(t *testing.T) {
	composer := NewDefaultTranscriptComposer()
	base := []ChatBlock{{Role: ChatRoleUser, Text: "hi", Status: ChatStatusSending}}

	blocks, changed := composer.MarkUserStatus(base, 0, ChatStatusNone)
	if !changed {
		t.Fatalf("expected status update")
	}
	if blocks[0].Status != ChatStatusNone {
		t.Fatalf("expected cleared status, got %#v", blocks[0])
	}
}

func TestDefaultTranscriptComposerMarkUserStatusSending(t *testing.T) {
	composer := NewDefaultTranscriptComposer()
	base := []ChatBlock{{Role: ChatRoleUser, Text: "hi", Status: ChatStatusNone}}

	blocks, changed := composer.MarkUserStatus(base, 0, ChatStatusSending)
	if !changed {
		t.Fatalf("expected status update")
	}
	if blocks[0].Status != ChatStatusSending {
		t.Fatalf("expected sending status, got %#v", blocks[0])
	}
}

func TestDefaultTranscriptComposerMergeApprovalsPreservesRelativePositions(t *testing.T) {
	composer := NewDefaultTranscriptComposer()
	base := []ChatBlock{
		{Role: ChatRoleUser, Text: "prompt"},
		{Role: ChatRoleAgent, Text: "reply"},
	}
	previous := []ChatBlock{
		{Role: ChatRoleUser, Text: "prompt"},
		{Role: ChatRoleApproval, RequestID: 3, Text: "approval required"},
		{Role: ChatRoleAgent, Text: "reply"},
	}
	requests := []*ApprovalRequest{
		{RequestID: 3, SessionID: "s1", Summary: "command", Detail: "go test ./..."},
	}

	merged := composer.MergeApprovals(base, requests, nil, previous)
	if len(merged) != 3 {
		t.Fatalf("expected approval to be preserved in relative order, got %#v", merged)
	}
	if merged[1].Role != ChatRoleApproval || merged[1].RequestID != 3 {
		t.Fatalf("expected preserved approval in middle position, got %#v", merged)
	}
}
