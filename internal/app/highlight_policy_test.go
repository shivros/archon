package app

import "testing"

func TestDefaultHighlightContextPolicyAllowsExpectedContexts(t *testing.T) {
	policy := NewDefaultHighlightContextPolicy()

	if !policy.AllowsContext(uiModeNormal, highlightContextChatTranscript) {
		t.Fatalf("expected chat transcript allowed in normal mode")
	}
	if policy.AllowsContext(uiModeNormal, highlightContextMainNotes) {
		t.Fatalf("expected main notes disallowed in normal mode")
	}
	if !policy.AllowsContext(uiModeNotes, highlightContextMainNotes) {
		t.Fatalf("expected main notes allowed in notes mode")
	}
	if policy.AllowsContext(uiModeNotes, highlightContextChatTranscript) {
		t.Fatalf("expected chat transcript disallowed in notes mode")
	}
	if !policy.AllowsContext(uiModeCompose, highlightContextSidebar) {
		t.Fatalf("expected sidebar always allowed")
	}
	if !policy.AllowsContext(uiModeCompose, highlightContextSideNotesPanel) {
		t.Fatalf("expected notes side panel always allowed")
	}
}
