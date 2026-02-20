package app

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

type stubInputFramePolicy struct {
	frames map[InputFrameTarget]InputPanelFrame
}

func (p stubInputFramePolicy) FrameForTarget(target InputFrameTarget) InputPanelFrame {
	if p.frames == nil {
		return nil
	}
	return p.frames[target]
}

func TestDefaultInputFramePolicyTargets(t *testing.T) {
	policy := NewDefaultInputFramePolicy()

	tests := []struct {
		target InputFrameTarget
		want   bool
	}{
		{target: InputFrameTargetCompose, want: true},
		{target: InputFrameTargetAddNote, want: true},
		{target: InputFrameTargetGuidedWorkflowSetup, want: true},
		{target: InputFrameTargetApprovalResponse, want: false},
		{target: InputFrameTargetRecentsReply, want: false},
		{target: InputFrameTargetSearch, want: false},
	}

	for _, tt := range tests {
		got := policy.FrameForTarget(tt.target) != nil
		if got != tt.want {
			t.Fatalf("FrameForTarget(%q) frame presence=%v, want %v", tt.target, got, tt.want)
		}
	}
}

func TestWithInputFramePolicyInjectsComposeFrame(t *testing.T) {
	custom := testInputPanelFrame{inset: 5}
	m := NewModel(nil, WithInputFramePolicy(stubInputFramePolicy{
		frames: map[InputFrameTarget]InputPanelFrame{
			InputFrameTargetCompose: custom,
		},
	}))
	m.enterCompose("s1")

	panel, ok := m.activeInputPanel()
	if !ok {
		t.Fatalf("expected compose input panel")
	}
	layout := BuildInputPanelLayout(panel)
	if got, want := layout.InputLineCount(), m.chatInput.Height()+5; got != want {
		t.Fatalf("expected injected frame inset in compose input lines: got %d want %d", got, want)
	}

	m.mode = uiModeAddNote
	panel, ok = m.activeInputPanel()
	if !ok {
		t.Fatalf("expected add-note panel")
	}
	layout = BuildInputPanelLayout(panel)
	if got, want := layout.InputLineCount(), m.noteInput.Height(); got != want {
		t.Fatalf("expected no add-note frame from injected policy: got %d want %d", got, want)
	}
}

func TestWithInputFramePolicyNilRestoresDefaultPolicy(t *testing.T) {
	m := NewModel(nil, WithInputFramePolicy(stubInputFramePolicy{}), WithInputFramePolicy(nil))
	m.enterCompose("s1")

	panel, ok := m.activeInputPanel()
	if !ok {
		t.Fatalf("expected compose panel")
	}
	layout := BuildInputPanelLayout(panel)
	want := m.chatInput.Height() + guidedWorkflowPromptFrameStyle.GetVerticalFrameSize()
	if got := layout.InputLineCount(); got != want {
		t.Fatalf("expected default frame inset after nil policy reset: got %d want %d", got, want)
	}
}

func TestWithInputFramePolicyInjectsAddNoteFrame(t *testing.T) {
	custom := testInputPanelFrame{inset: 6}
	m := NewModel(nil, WithInputFramePolicy(stubInputFramePolicy{
		frames: map[InputFrameTarget]InputPanelFrame{
			InputFrameTargetAddNote: custom,
		},
	}))
	m.mode = uiModeAddNote

	panel, ok := m.activeInputPanel()
	if !ok {
		t.Fatalf("expected add-note panel")
	}
	layout := BuildInputPanelLayout(panel)
	if got, want := layout.InputLineCount(), m.noteInput.Height()+6; got != want {
		t.Fatalf("expected injected add-note frame inset: got %d want %d", got, want)
	}
}

func TestWithInputFramePolicyInjectsGuidedSetupFrame(t *testing.T) {
	custom := testInputPanelFrame{inset: 7}
	m := NewModel(nil, WithInputFramePolicy(stubInputFramePolicy{
		frames: map[InputFrameTarget]InputPanelFrame{
			InputFrameTargetGuidedWorkflowSetup: custom,
		},
	}))
	enterGuidedWorkflowForTest(&m, guidedWorkflowLaunchContext{workspaceID: "ws1"})

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEnter})
	next := asModel(t, updated)
	panel, ok := next.modeInputPanel()
	if !ok {
		t.Fatalf("expected guided setup panel")
	}
	layout := BuildInputPanelLayout(panel)
	if got, want := layout.InputLineCount(), next.guidedWorkflowPromptInput.Height()+7; got != want {
		t.Fatalf("expected injected guided setup frame inset: got %d want %d", got, want)
	}
}

func TestInputFramePolicyOrDefaultFallsBackWhenPolicyNil(t *testing.T) {
	m := NewModel(nil)
	m.inputFramePolicy = nil

	policy := m.inputFramePolicyOrDefault()
	if policy == nil {
		t.Fatalf("expected default policy fallback")
	}
	if frame := m.inputFrame(InputFrameTargetCompose); frame == nil {
		t.Fatalf("expected compose frame from default fallback policy")
	}
}
