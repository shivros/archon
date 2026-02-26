package app

import (
	"testing"
	"time"

	"control/internal/guidedworkflows"
	"control/internal/types"
)

func TestVisibleInputPanelLayoutReturnsFalseWithoutLayout(t *testing.T) {
	if _, ok := visibleInputPanelLayout(InputPanelLayout{}, false); ok {
		t.Fatalf("expected missing layout to be hidden")
	}
}

func TestVisibleInputPanelLayoutReturnsFalseForEmptyRenderedView(t *testing.T) {
	layout := InputPanelLayout{
		line:       "",
		inputLines: 3,
		footerRows: 1,
	}
	if _, ok := visibleInputPanelLayout(layout, true); ok {
		t.Fatalf("expected empty rendered input panel to be hidden")
	}
}

func TestVisibleInputPanelLayoutKeepsNonEmptyRenderedView(t *testing.T) {
	layout := InputPanelLayout{
		line:       "input",
		inputLines: 3,
		footerRows: 1,
	}
	visible, ok := visibleInputPanelLayout(layout, true)
	if !ok {
		t.Fatalf("expected non-empty rendered input panel to remain visible")
	}
	if got, want := visible.LineCount(), 4; got != want {
		t.Fatalf("expected visible layout line count %d, got %d", want, got)
	}
}

func TestModeInputPanelGuidedWorkflowSetupHiddenOutsideGuidedWorkflowMode(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	enterGuidedWorkflowForTest(&m, guidedWorkflowLaunchContext{workspaceID: "ws1"})
	advanceGuidedWorkflowToComposerForTest(t, &m)

	m.mode = uiModeNormal
	if _, ok := m.modeInputPanel(); ok {
		t.Fatalf("did not expect guided workflow setup input panel outside guided workflow mode")
	}
	if got := m.modeInputLineCount(); got != 0 {
		t.Fatalf("expected no reserved input lines outside guided workflow mode, got %d", got)
	}
}

func TestViewportHeightReturnsToBaselineAfterComposeExit(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	m.resize(120, 40)

	baseline := m.viewport.Height()
	m.enterCompose("s1")
	duringCompose := m.viewport.Height()
	if duringCompose >= baseline {
		t.Fatalf("expected compose viewport height %d to be less than baseline %d", duringCompose, baseline)
	}

	m.exitCompose("")
	if got := m.viewport.Height(); got != baseline {
		t.Fatalf("expected viewport height to return to baseline %d after compose exit, got %d", baseline, got)
	}
}

func TestViewportHeightStableAcrossRepeatedComposeToggle(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	m.resize(120, 40)

	baseline := m.viewport.Height()
	lastComposeHeight := -1
	for i := 0; i < 5; i++ {
		m.enterCompose("s1")
		currentComposeHeight := m.viewport.Height()
		if currentComposeHeight >= baseline {
			t.Fatalf("cycle %d: expected compose viewport height %d to be less than baseline %d", i, currentComposeHeight, baseline)
		}
		if lastComposeHeight >= 0 && currentComposeHeight != lastComposeHeight {
			t.Fatalf("cycle %d: expected compose viewport height %d to match prior cycle %d", i, currentComposeHeight, lastComposeHeight)
		}
		lastComposeHeight = currentComposeHeight

		m.exitCompose("")
		if got := m.viewport.Height(); got != baseline {
			t.Fatalf("cycle %d: expected viewport height to return to baseline %d after compose exit, got %d", i, baseline, got)
		}
	}
}

func TestModeInputPanelGuidedWorkflowResumeVisibleInGuidedWorkflowMode(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	enterGuidedWorkflowForTest(&m, guidedWorkflowLaunchContext{workspaceID: "ws1"})
	if m.guidedWorkflow == nil {
		t.Fatalf("expected guided workflow controller")
	}
	m.guidedWorkflow.SetRun(&guidedworkflows.WorkflowRun{
		ID:     "gwf-1",
		Status: guidedworkflows.WorkflowRunStatusFailed,
	})

	panel, ok := m.modeInputPanel()
	if !ok {
		t.Fatalf("expected guided workflow resume input panel")
	}
	if panel.Input != m.guidedWorkflowResumeInput {
		t.Fatalf("expected resume input panel")
	}
}

func TestViewportHeightReturnsToBaselineAfterGuidedWorkflowExit(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	m.resize(120, 40)
	baseline := m.viewport.Height()

	enterGuidedWorkflowForTest(&m, guidedWorkflowLaunchContext{workspaceID: "ws1"})
	advanceGuidedWorkflowToComposerForTest(t, &m)
	duringSetup := m.viewport.Height()
	if duringSetup >= baseline {
		t.Fatalf("expected guided workflow setup viewport height %d to be less than baseline %d", duringSetup, baseline)
	}

	m.exitGuidedWorkflow("")
	if got := m.viewport.Height(); got != baseline {
		t.Fatalf("expected viewport height to return to baseline %d after guided workflow exit, got %d", baseline, got)
	}
}

func TestViewportHeightStableAcrossRepeatedGuidedWorkflowToggle(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	m.resize(120, 40)
	baseline := m.viewport.Height()

	lastGuidedHeight := -1
	for i := 0; i < 3; i++ {
		enterGuidedWorkflowForTest(&m, guidedWorkflowLaunchContext{workspaceID: "ws1"})
		advanceGuidedWorkflowToComposerForTest(t, &m)
		current := m.viewport.Height()
		if current >= baseline {
			t.Fatalf("cycle %d: expected guided workflow viewport height %d to be less than baseline %d", i, current, baseline)
		}
		if lastGuidedHeight >= 0 && current != lastGuidedHeight {
			t.Fatalf("cycle %d: expected guided workflow viewport height %d to match prior cycle %d", i, current, lastGuidedHeight)
		}
		lastGuidedHeight = current
		m.exitGuidedWorkflow("")
		if got := m.viewport.Height(); got != baseline {
			t.Fatalf("cycle %d: expected viewport height to return to baseline %d after guided workflow exit, got %d", i, baseline, got)
		}
	}
}

func TestViewportHeightReturnsToBaselineAfterRecentsReplyExit(t *testing.T) {
	m := NewModel(nil)
	m.resize(120, 40)
	baseline := m.viewport.Height()

	now := time.Now().UTC()
	m.showRecents = true
	m.appState.ActiveWorkspaceGroupIDs = []string{"ungrouped"}
	m.workspaces = []*types.Workspace{{ID: "ws1", Name: "Workspace"}}
	m.sessions = []*types.Session{
		{ID: "s1", Provider: "codex", Status: types.SessionStatusRunning, CreatedAt: now},
	}
	m.sessionMeta = map[string]*types.SessionMeta{
		"s1": {SessionID: "s1", WorkspaceID: "ws1", LastTurnID: "turn-1"},
	}
	m.recents.StartRun("s1", "turn-0", now.Add(-time.Minute))

	m.enterRecentsView(&sidebarItem{kind: sidebarRecentsAll})
	m.recentsSelectedSessionID = "s1"
	if !m.startRecentsReply() {
		t.Fatalf("expected to start recents reply")
	}
	duringReply := m.viewport.Height()
	if duringReply >= baseline {
		t.Fatalf("expected recents reply viewport height %d to be less than baseline %d", duringReply, baseline)
	}

	m.exitRecentsView()
	if got := m.viewport.Height(); got != baseline {
		t.Fatalf("expected viewport height to return to baseline %d after recents exit, got %d", baseline, got)
	}
}

func TestViewportHeightReturnsToBaselineAfterRecentsReplyExitSmallViewport(t *testing.T) {
	m := NewModel(nil)
	m.resize(90, 14)
	baseline := m.viewport.Height()

	now := time.Now().UTC()
	m.showRecents = true
	m.appState.ActiveWorkspaceGroupIDs = []string{"ungrouped"}
	m.workspaces = []*types.Workspace{{ID: "ws1", Name: "Workspace"}}
	m.sessions = []*types.Session{
		{ID: "s1", Provider: "codex", Status: types.SessionStatusRunning, CreatedAt: now},
	}
	m.sessionMeta = map[string]*types.SessionMeta{
		"s1": {SessionID: "s1", WorkspaceID: "ws1", LastTurnID: "turn-1"},
	}
	m.recents.StartRun("s1", "turn-0", now.Add(-time.Minute))

	m.enterRecentsView(&sidebarItem{kind: sidebarRecentsAll})
	m.recentsSelectedSessionID = "s1"
	if !m.startRecentsReply() {
		t.Fatalf("expected to start recents reply")
	}
	if m.viewport.Height() >= baseline {
		t.Fatalf("expected recents reply to reduce viewport height in tight viewport")
	}

	m.exitRecentsView()
	if got := m.viewport.Height(); got != baseline {
		t.Fatalf("expected viewport height to return to baseline %d after recents exit in tight viewport, got %d", baseline, got)
	}
}
