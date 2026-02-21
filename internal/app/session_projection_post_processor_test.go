package app

import "testing"

func TestDefaultSessionProjectionPostProcessorHandlesNilModel(t *testing.T) {
	processor := NewDefaultSessionProjectionPostProcessor()
	processor.PostProcessSessionProjection(nil, SessionProjectionPostProcessInput{
		Source:    sessionProjectionSourceHistory,
		SessionID: "s1",
		Blocks:    []ChatBlock{{Role: ChatRoleUser, TurnID: "turn-1"}},
	})
}

func TestDefaultSessionProjectionPostProcessorAppliesWorkflowTurnFocus(t *testing.T) {
	processor := NewDefaultSessionProjectionPostProcessor()
	m := NewModel(nil)
	m.setPendingWorkflowTurnFocus("s1", "turn-1")
	processor.PostProcessSessionProjection(&m, SessionProjectionPostProcessInput{
		Source:    sessionProjectionSourceHistory,
		SessionID: "s1",
		Blocks:    []ChatBlock{{Role: ChatRoleUser, TurnID: "turn-1", Text: "request"}},
	})
	if m.pendingWorkflowTurnFocus != nil {
		t.Fatalf("expected default post processor to clear matching pending turn focus")
	}
}
