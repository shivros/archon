package app

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestModeInputPanelComposeUsesGuidedWorkflowFrame(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	m.enterCompose("s1")

	panel, ok := m.modeInputPanel()
	if !ok {
		t.Fatalf("expected compose mode input panel")
	}
	layout := BuildInputPanelLayout(panel)
	wantInputLines := m.chatInput.Height() + guidedWorkflowPromptFrameStyle.GetVerticalFrameSize()
	if got := layout.InputLineCount(); got != wantInputLines {
		t.Fatalf("expected framed compose input lines %d, got %d", wantInputLines, got)
	}
	footerRow, ok := layout.FooterStartRow()
	if !ok {
		t.Fatalf("expected compose footer row")
	}
	if footerRow != wantInputLines {
		t.Fatalf("expected compose footer row %d, got %d", wantInputLines, footerRow)
	}
}

func TestModeInputPanelAddNoteUsesGuidedWorkflowFrame(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	m.mode = uiModeAddNote

	panel, ok := m.modeInputPanel()
	if !ok {
		t.Fatalf("expected add-note mode input panel")
	}
	layout := BuildInputPanelLayout(panel)
	wantInputLines := m.noteInput.Height() + guidedWorkflowPromptFrameStyle.GetVerticalFrameSize()
	if got := layout.InputLineCount(); got != wantInputLines {
		t.Fatalf("expected framed add-note input lines %d, got %d", wantInputLines, got)
	}
	if _, ok := layout.FooterStartRow(); ok {
		t.Fatalf("did not expect add-note footer row")
	}
}

func TestModeInputPanelGuidedSetupUsesSharedFramePath(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	enterGuidedWorkflowForTest(&m, guidedWorkflowLaunchContext{workspaceID: "ws1"})

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	m = asModel(t, updated)
	if m.guidedWorkflow == nil || m.guidedWorkflow.Stage() != guidedWorkflowStageSetup {
		t.Fatalf("expected guided workflow setup stage")
	}

	panel, ok := m.modeInputPanel()
	if !ok {
		t.Fatalf("expected guided setup input panel")
	}
	layout := BuildInputPanelLayout(panel)
	wantInputLines := m.guidedWorkflowPromptInput.Height() + guidedWorkflowPromptFrameStyle.GetVerticalFrameSize()
	if got := layout.InputLineCount(); got != wantInputLines {
		t.Fatalf("expected framed guided setup input lines %d, got %d", wantInputLines, got)
	}
	if _, ok := layout.FooterStartRow(); ok {
		t.Fatalf("did not expect guided setup footer row")
	}
}
