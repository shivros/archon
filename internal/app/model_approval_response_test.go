package app

import "testing"

func approvalResponseTestRequest() *ApprovalRequest {
	return &ApprovalRequest{
		RequestID: 7,
		SessionID: "s1",
		Method:    approvalMethodRequestUserInput,
		Summary:   "user input",
		Detail:    "provide context",
	}
}

func TestExitApprovalResponseReturnsToComposeAndRestoresChatFocus(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	m.enterCompose("s1")
	req := approvalResponseTestRequest()
	m.enterApprovalResponse("s1", req)

	m.exitApprovalResponse("back to compose")

	if m.mode != uiModeCompose {
		t.Fatalf("expected compose mode after approval exit, got %v", m.mode)
	}
	if m.input == nil || !m.input.IsChatFocused() {
		t.Fatalf("expected chat focus after returning to compose")
	}
	if m.chatInput == nil || !m.chatInput.Focused() {
		t.Fatalf("expected chat input to be focused after returning to compose")
	}
}

func TestExitApprovalResponseFallbackFromApprovalReturnModeToNormal(t *testing.T) {
	m := NewModel(nil)
	req := approvalResponseTestRequest()
	m.enterApprovalResponse("s1", req)
	m.approvalResponseReturnMode = uiModeApprovalResponse

	m.exitApprovalResponse("done")

	if m.mode != uiModeNormal {
		t.Fatalf("expected normal mode fallback, got %v", m.mode)
	}
}

func TestExitApprovalResponseSidebarPathBlursChatInput(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	m.enterCompose("s1")
	req := approvalResponseTestRequest()
	m.enterApprovalResponse("s1", req)
	m.approvalResponseReturnMode = uiModeNormal
	m.approvalResponseReturnFocus = focusSidebar

	m.exitApprovalResponse("closed")

	if m.mode != uiModeNormal {
		t.Fatalf("expected normal mode, got %v", m.mode)
	}
	if m.input == nil || !m.input.IsSidebarFocused() {
		t.Fatalf("expected sidebar focus after non-compose approval exit")
	}
	if m.chatInput != nil && m.chatInput.Focused() {
		t.Fatalf("expected chat input to be blurred after non-compose approval exit")
	}
}

func TestExitApprovalResponseNilModelNoPanic(t *testing.T) {
	var m *Model
	m.exitApprovalResponse("")
}
