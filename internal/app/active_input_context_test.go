package app

import "testing"

func TestActiveInputContextRecentsReplyRequiresActiveSession(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	m.mode = uiModeRecents
	m.recentsReplySessionID = ""

	if _, ok := m.activeInputContext(); ok {
		t.Fatalf("did not expect recents input context without an active reply target")
	}

	m.recentsReplySessionID = "s1"
	ctx, ok := m.activeInputContext()
	if !ok {
		t.Fatalf("expected recents input context with active reply target")
	}
	if ctx.input != m.recentsReplyInput {
		t.Fatalf("expected recents reply input context")
	}
	if ctx.footer == nil || ctx.footer.InputFooter() != "enter send • esc cancel" {
		t.Fatalf("expected recents reply footer in input context")
	}
	if ctx.frame != nil {
		t.Fatalf("did not expect recents reply frame")
	}
}

func TestActiveInputContextComposeUsesGuidedWorkflowFrame(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	m.enterCompose("s1")

	ctx, ok := m.activeInputContext()
	if !ok {
		t.Fatalf("expected compose input context")
	}
	if ctx.input != m.chatInput {
		t.Fatalf("expected compose chat input context")
	}
	if ctx.frame == nil {
		t.Fatalf("expected compose input frame")
	}
	panel := ctx.panel()
	layout := BuildInputPanelLayout(panel)
	wantLines := m.chatInput.Height() + guidedWorkflowPromptFrameStyle.GetVerticalFrameSize()
	if got := layout.InputLineCount(); got != wantLines {
		t.Fatalf("expected framed compose input lines %d, got %d", wantLines, got)
	}
}

func TestActiveInputContextApprovalResponseHasFooterWithoutFrame(t *testing.T) {
	m := NewModel(nil)
	m.mode = uiModeApprovalResponse

	ctx, ok := m.activeInputContext()
	if !ok {
		t.Fatalf("expected approval response input context")
	}
	if ctx.input != m.approvalInput {
		t.Fatalf("expected approval response input")
	}
	if ctx.footer == nil || ctx.footer.InputFooter() != "enter submit  esc cancel" {
		t.Fatalf("expected approval response footer text")
	}
	if ctx.frame != nil {
		t.Fatalf("did not expect approval response frame")
	}
}

func TestActiveInputContextSearchHasNoFooterAndNoFrame(t *testing.T) {
	m := NewModel(nil)
	m.mode = uiModeSearch

	ctx, ok := m.activeInputContext()
	if !ok {
		t.Fatalf("expected search input context")
	}
	if ctx.input != m.searchInput {
		t.Fatalf("expected search input")
	}
	if ctx.footer != nil {
		t.Fatalf("did not expect search footer")
	}
	if ctx.frame != nil {
		t.Fatalf("did not expect search frame")
	}
}

func TestActiveInputContextApprovalResponseNilInputReturnsNoContext(t *testing.T) {
	m := NewModel(nil)
	m.mode = uiModeApprovalResponse
	m.approvalInput = nil

	if _, ok := m.activeInputContext(); ok {
		t.Fatalf("did not expect approval response context when input is nil")
	}
}

func TestActiveInputContextAddNoteNilInputReturnsNoContext(t *testing.T) {
	m := NewModel(nil)
	m.mode = uiModeAddNote
	m.noteInput = nil

	if _, ok := m.activeInputContext(); ok {
		t.Fatalf("did not expect add-note context when input is nil")
	}
}

func TestActiveInputContextSearchNilInputReturnsNoContext(t *testing.T) {
	m := NewModel(nil)
	m.mode = uiModeSearch
	m.searchInput = nil

	if _, ok := m.activeInputContext(); ok {
		t.Fatalf("did not expect search context when input is nil")
	}
}

func TestActiveInputContextNilModelReturnsNoContext(t *testing.T) {
	var m *Model
	if _, ok := m.activeInputContext(); ok {
		t.Fatalf("did not expect input context for nil model")
	}
}

func TestActiveInputContextNormalModeHasNoContext(t *testing.T) {
	m := NewModel(nil)
	m.mode = uiModeNormal
	if _, ok := m.activeInputContext(); ok {
		t.Fatalf("did not expect active input context in normal mode")
	}
}
