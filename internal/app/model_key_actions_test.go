package app

import (
	"fmt"
	"testing"

	tea "charm.land/bubbletea/v2"

	"control/internal/guidedworkflows"
	"control/internal/types"
)

func TestReduceViewportNavigationKeysHandlesSectionAndSearchCommands(t *testing.T) {
	m := NewModel(nil)
	keys := []tea.KeyMsg{
		tea.KeyPressMsg{Text: "{"},
		tea.KeyPressMsg{Text: "}"},
		tea.KeyPressMsg{Text: "n"},
		tea.KeyPressMsg{Text: "N"},
	}
	for _, key := range keys {
		handled, cmd := m.reduceViewportNavigationKeys(key)
		if !handled {
			t.Fatalf("expected key %q to be handled", key.String())
		}
		if cmd != nil {
			t.Fatalf("expected no command for key %q", key.String())
		}
	}
}

func TestReduceViewportNavigationKeysRoutesTopBottomToDebugPanelWhenNavigable(t *testing.T) {
	m := NewModel(nil)
	m.resize(120, 40)
	panel := &fakeDebugPanelView{height: 6}
	m.debugPanel = panel
	m.debugPanelVisible = true
	m.appState.DebugStreamsEnabled = true

	handled, _ := m.reduceViewportNavigationKeys(tea.KeyPressMsg{Code: 'g', Text: "g"})
	if !handled {
		t.Fatalf("expected g to be handled")
	}
	if panel.gotoTop != 1 {
		t.Fatalf("expected debug panel goto top, got %d", panel.gotoTop)
	}
	handled, _ = m.reduceViewportNavigationKeys(tea.KeyPressMsg{Code: 'G', Text: "G"})
	if !handled {
		t.Fatalf("expected G to be handled")
	}
	if panel.gotoBottom != 1 {
		t.Fatalf("expected debug panel goto bottom, got %d", panel.gotoBottom)
	}
}

func TestReduceViewportNavigationKeysRoutesTopBottomToTranscriptWhenDebugHidden(t *testing.T) {
	m := NewModel(nil)
	m.resize(120, 30)
	blocks := make([]ChatBlock, 0, 120)
	for i := 0; i < 120; i++ {
		blocks = append(blocks, ChatBlock{Role: ChatRoleAgent, Text: fmt.Sprintf("line %d", i)})
	}
	m.applyBlocks(blocks)
	m.enableFollow(false)
	m.pauseFollow(false)
	m.viewport.GotoBottom()
	if m.viewport.YOffset() == 0 {
		t.Fatalf("expected non-zero initial offset")
	}

	handled, _ := m.reduceViewportNavigationKeys(tea.KeyPressMsg{Code: 'g', Text: "g"})
	if !handled {
		t.Fatalf("expected g to be handled")
	}
	if got := m.viewport.YOffset(); got != 0 {
		t.Fatalf("expected transcript goto top, got %d", got)
	}
	if m.follow {
		t.Fatalf("expected follow paused after top navigation")
	}
	handled, _ = m.reduceViewportNavigationKeys(tea.KeyPressMsg{Code: 'G', Text: "G"})
	if !handled {
		t.Fatalf("expected G to be handled")
	}
	if !m.follow {
		t.Fatalf("expected follow enabled after bottom navigation")
	}
}

func TestRenameHotkeyRoutesWorkspaceSelection(t *testing.T) {
	m := NewModel(nil)
	m.workspaces = []*types.Workspace{{ID: "ws1", Name: "Workspace", RepoPath: "/tmp/ws1"}}
	m.worktrees = map[string][]*types.Worktree{}
	m.sidebar.Apply(m.workspaces, m.worktrees, nil, nil, nil, "", "", false)

	handled, cmd := m.reduceComposeAndWorkspaceEntryKeys(keyRune('m'))
	if !handled {
		t.Fatalf("expected rename hotkey to be handled")
	}
	if cmd != nil {
		t.Fatalf("expected no async command for entering rename")
	}
	if m.mode != uiModeEditWorkspace {
		t.Fatalf("expected workspace rename mode, got %v", m.mode)
	}
	if m.renameWorkspaceID != "ws1" {
		t.Fatalf("expected workspace id ws1, got %q", m.renameWorkspaceID)
	}
}

func TestRenameHotkeyRoutesWorktreeSelection(t *testing.T) {
	m := NewModel(nil)
	m.workspaces = []*types.Workspace{{ID: "ws1", Name: "Workspace", RepoPath: "/tmp/ws1"}}
	m.worktrees = map[string][]*types.Worktree{
		"ws1": {
			{ID: "wt1", WorkspaceID: "ws1", Name: "Worktree", Path: "/tmp/ws1/wt1"},
		},
	}
	m.sidebar.Apply(m.workspaces, m.worktrees, nil, nil, nil, "", "", false)
	selectSidebarItemKind(t, &m, sidebarWorktree)

	handled, cmd := m.reduceComposeAndWorkspaceEntryKeys(keyRune('m'))
	if !handled {
		t.Fatalf("expected rename hotkey to be handled")
	}
	if cmd != nil {
		t.Fatalf("expected no async command for entering rename")
	}
	if m.mode != uiModeRenameWorktree {
		t.Fatalf("expected worktree rename mode, got %v", m.mode)
	}
	if m.renameWorktreeID != "wt1" || m.renameWorktreeWorkspaceID != "ws1" {
		t.Fatalf("expected worktree target ws1/wt1, got %q/%q", m.renameWorktreeWorkspaceID, m.renameWorktreeID)
	}
}

func TestRenameHotkeyRoutesSessionSelection(t *testing.T) {
	m := NewModel(nil)
	m.workspaces = []*types.Workspace{{ID: "ws1", Name: "Workspace", RepoPath: "/tmp/ws1"}}
	m.worktrees = map[string][]*types.Worktree{}
	m.sessions = []*types.Session{{ID: "s1", Title: "Session", Status: types.SessionStatusExited}}
	m.sessionMeta = map[string]*types.SessionMeta{
		"s1": {SessionID: "s1", WorkspaceID: "ws1", Title: "Session"},
	}
	m.sidebar.Apply(m.workspaces, m.worktrees, m.sessions, nil, m.sessionMeta, "", "", false)
	selectSidebarItemKind(t, &m, sidebarSession)

	handled, cmd := m.reduceComposeAndWorkspaceEntryKeys(keyRune('m'))
	if !handled {
		t.Fatalf("expected rename hotkey to be handled")
	}
	if cmd != nil {
		t.Fatalf("expected no async command for entering rename")
	}
	if m.mode != uiModeRenameSession {
		t.Fatalf("expected session rename mode, got %v", m.mode)
	}
	if m.renameSessionID != "s1" {
		t.Fatalf("expected session id s1, got %q", m.renameSessionID)
	}
}

func TestRenameHotkeyRoutesWorkflowSelection(t *testing.T) {
	m := NewModel(nil)
	m.workspaces = []*types.Workspace{{ID: "ws1", Name: "Workspace", RepoPath: "/tmp/ws1"}}
	m.worktrees = map[string][]*types.Worktree{}
	workflows := []*guidedworkflows.WorkflowRun{
		{ID: "gwf-1", WorkspaceID: "ws1", TemplateName: "SOLID", Status: guidedworkflows.WorkflowRunStatusRunning},
	}
	m.sidebar.Apply(m.workspaces, m.worktrees, nil, workflows, map[string]*types.SessionMeta{}, "", "", false)
	selectSidebarItemKind(t, &m, sidebarWorkflow)

	handled, cmd := m.reduceComposeAndWorkspaceEntryKeys(keyRune('m'))
	if !handled {
		t.Fatalf("expected rename hotkey to be handled")
	}
	if cmd != nil {
		t.Fatalf("expected no async command for entering rename")
	}
	if m.mode != uiModeRenameWorkflow {
		t.Fatalf("expected workflow rename mode, got %v", m.mode)
	}
	if m.renameWorkflowRunID != "gwf-1" {
		t.Fatalf("expected workflow run id gwf-1, got %q", m.renameWorkflowRunID)
	}
}

func TestDeleteHotkeyRoutesWorkspaceSelection(t *testing.T) {
	m := NewModel(nil)
	m.workspaces = []*types.Workspace{{ID: "ws1", Name: "Workspace", RepoPath: "/tmp/ws1"}}
	m.worktrees = map[string][]*types.Worktree{}
	m.sidebar.Apply(m.workspaces, m.worktrees, nil, nil, nil, "", "", false)

	handled, cmd := m.reduceSessionLifecycleKeys(keyRune('d'))
	if !handled {
		t.Fatalf("expected delete hotkey to be handled")
	}
	if cmd != nil {
		t.Fatalf("expected no async command for delete confirmation")
	}
	action, ok := m.pendingSelectionAction.(deleteWorkspaceSelectionAction)
	if !ok {
		t.Fatalf("expected workspace delete selection action, got %T", m.pendingSelectionAction)
	}
	if action.workspaceID != "ws1" {
		t.Fatalf("expected workspace id ws1, got %q", action.workspaceID)
	}
}

func TestDeleteHotkeyRoutesWorktreeSelection(t *testing.T) {
	m := NewModel(nil)
	m.workspaces = []*types.Workspace{{ID: "ws1", Name: "Workspace", RepoPath: "/tmp/ws1"}}
	m.worktrees = map[string][]*types.Worktree{
		"ws1": {
			{ID: "wt1", WorkspaceID: "ws1", Name: "Worktree", Path: "/tmp/ws1/wt1"},
		},
	}
	m.sidebar.Apply(m.workspaces, m.worktrees, nil, nil, nil, "", "", false)
	selectSidebarItemKind(t, &m, sidebarWorktree)

	handled, cmd := m.reduceSessionLifecycleKeys(keyRune('d'))
	if !handled {
		t.Fatalf("expected delete hotkey to be handled")
	}
	if cmd != nil {
		t.Fatalf("expected no async command for delete confirmation")
	}
	action, ok := m.pendingSelectionAction.(deleteWorktreeSelectionAction)
	if !ok {
		t.Fatalf("expected worktree delete selection action, got %T", m.pendingSelectionAction)
	}
	if action.workspaceID != "ws1" || action.worktreeID != "wt1" {
		t.Fatalf("expected worktree target ws1/wt1, got %q/%q", action.workspaceID, action.worktreeID)
	}
}

func TestDeleteHotkeyRoutesSessionSelection(t *testing.T) {
	m := NewModel(nil)
	m.workspaces = []*types.Workspace{{ID: "ws1", Name: "Workspace", RepoPath: "/tmp/ws1"}}
	m.worktrees = map[string][]*types.Worktree{}
	m.sessions = []*types.Session{{ID: "s1", Title: "Session", Status: types.SessionStatusExited}}
	m.sessionMeta = map[string]*types.SessionMeta{
		"s1": {SessionID: "s1", WorkspaceID: "ws1", Title: "Session"},
	}
	m.sidebar.Apply(m.workspaces, m.worktrees, m.sessions, nil, m.sessionMeta, "", "", false)
	selectSidebarItemKind(t, &m, sidebarSession)

	handled, cmd := m.reduceSessionLifecycleKeys(keyRune('d'))
	if !handled {
		t.Fatalf("expected delete hotkey to be handled")
	}
	if cmd != nil {
		t.Fatalf("expected no async command for dismiss confirmation")
	}
	action, ok := m.pendingSelectionAction.(dismissSessionSelectionAction)
	if !ok {
		t.Fatalf("expected session dismiss selection action, got %T", m.pendingSelectionAction)
	}
	if action.sessionID != "s1" {
		t.Fatalf("expected session target s1, got %q", action.sessionID)
	}
}

func TestDeleteHotkeyRoutesWorkflowSelection(t *testing.T) {
	m := NewModel(nil)
	m.workspaces = []*types.Workspace{{ID: "ws1", Name: "Workspace", RepoPath: "/tmp/ws1"}}
	m.worktrees = map[string][]*types.Worktree{}
	workflows := []*guidedworkflows.WorkflowRun{
		{ID: "gwf-1", WorkspaceID: "ws1", TemplateName: "SOLID", Status: guidedworkflows.WorkflowRunStatusRunning},
	}
	m.sidebar.Apply(m.workspaces, m.worktrees, nil, workflows, map[string]*types.SessionMeta{}, "", "", false)
	selectSidebarItemKind(t, &m, sidebarWorkflow)

	handled, cmd := m.reduceSessionLifecycleKeys(keyRune('d'))
	if !handled {
		t.Fatalf("expected delete hotkey to be handled")
	}
	if cmd != nil {
		t.Fatalf("expected no async command for dismiss confirmation")
	}
	action, ok := m.pendingSelectionAction.(dismissWorkflowSelectionAction)
	if !ok {
		t.Fatalf("expected workflow dismiss selection action, got %T", m.pendingSelectionAction)
	}
	if action.runID != "gwf-1" {
		t.Fatalf("expected workflow run id gwf-1, got %q", action.runID)
	}
}

func TestDeleteHotkeyBuildsBatchActionForMixedMultiSelect(t *testing.T) {
	m := NewModel(nil)
	m.workspaces = []*types.Workspace{{ID: "ws1", Name: "Workspace", RepoPath: "/tmp/ws1"}}
	m.worktrees = map[string][]*types.Worktree{}
	m.sessions = []*types.Session{{ID: "s1", Title: "Session", Status: types.SessionStatusExited}}
	m.sessionMeta = map[string]*types.SessionMeta{
		"s1": {SessionID: "s1", WorkspaceID: "ws1", Title: "Session"},
	}
	workflows := []*guidedworkflows.WorkflowRun{
		{ID: "gwf-1", WorkspaceID: "ws1", TemplateName: "SOLID", Status: guidedworkflows.WorkflowRunStatusRunning},
	}
	m.sidebar.Apply(m.workspaces, m.worktrees, m.sessions, workflows, m.sessionMeta, "", "", false)
	selectSidebarItemKind(t, &m, sidebarSession)
	_, _ = m.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	selectSidebarItemKind(t, &m, sidebarWorkflow)
	_, _ = m.Update(tea.KeyPressMsg{Code: tea.KeySpace})

	handled, cmd := m.reduceSessionLifecycleKeys(keyRune('d'))
	if !handled {
		t.Fatalf("expected delete hotkey to be handled")
	}
	if cmd != nil {
		t.Fatalf("expected no async command for confirmation")
	}
	action, ok := m.pendingSelectionAction.(selectionBatchAction)
	if !ok {
		t.Fatalf("expected selectionBatchAction, got %T", m.pendingSelectionAction)
	}
	if len(action.operations) != 2 {
		t.Fatalf("expected two operations for mixed selection, got %d", len(action.operations))
	}
}

func TestInterruptHotkeyBuildsBatchActionForSessionsAndWorkflows(t *testing.T) {
	m := NewModel(nil)
	m.workspaces = []*types.Workspace{{ID: "ws1", Name: "Workspace", RepoPath: "/tmp/ws1"}}
	m.worktrees = map[string][]*types.Worktree{}
	m.sessions = []*types.Session{{ID: "s1", Title: "Session", Status: types.SessionStatusRunning}}
	m.sessionMeta = map[string]*types.SessionMeta{
		"s1": {SessionID: "s1", WorkspaceID: "ws1", Title: "Session"},
	}
	workflows := []*guidedworkflows.WorkflowRun{
		{ID: "gwf-1", WorkspaceID: "ws1", TemplateName: "SOLID", Status: guidedworkflows.WorkflowRunStatusRunning},
	}
	m.sidebar.Apply(m.workspaces, m.worktrees, m.sessions, workflows, m.sessionMeta, "", "", false)
	selectSidebarItemKind(t, &m, sidebarSession)
	_, _ = m.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	selectSidebarItemKind(t, &m, sidebarWorkflow)
	_, _ = m.Update(tea.KeyPressMsg{Code: tea.KeySpace})

	handled, cmd := m.reduceSessionLifecycleKeys(keyRune('i'))
	if !handled {
		t.Fatalf("expected interrupt hotkey to be handled")
	}
	if cmd != nil {
		t.Fatalf("expected no async command for confirmation")
	}
	action, ok := m.pendingSelectionAction.(selectionBatchAction)
	if !ok {
		t.Fatalf("expected selectionBatchAction, got %T", m.pendingSelectionAction)
	}
	if len(action.operations) != 2 {
		t.Fatalf("expected two operations for interrupt/stop, got %d", len(action.operations))
	}
}

func TestKillHotkeySkipsNonKillableSelections(t *testing.T) {
	m := NewModel(nil)
	m.workspaces = []*types.Workspace{{ID: "ws1", Name: "Workspace", RepoPath: "/tmp/ws1"}}
	m.worktrees = map[string][]*types.Worktree{}
	m.sessions = []*types.Session{
		{ID: "s1", Title: "Running", Status: types.SessionStatusRunning},
		{ID: "s2", Title: "Exited", Status: types.SessionStatusExited},
	}
	m.sessionMeta = map[string]*types.SessionMeta{
		"s1": {SessionID: "s1", WorkspaceID: "ws1", Title: "Running"},
		"s2": {SessionID: "s2", WorkspaceID: "ws1", Title: "Exited"},
	}
	m.sidebar.Apply(m.workspaces, m.worktrees, m.sessions, nil, m.sessionMeta, "", "", false)

	if !m.sidebar.SelectBySessionID("s1") {
		t.Fatalf("expected to select session s1")
	}
	_, _ = m.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	if !m.sidebar.SelectBySessionID("s2") {
		t.Fatalf("expected to select session s2")
	}
	_, _ = m.Update(tea.KeyPressMsg{Code: tea.KeySpace})

	handled, cmd := m.reduceSessionLifecycleKeys(keyRune('x'))
	if !handled {
		t.Fatalf("expected kill hotkey to be handled")
	}
	if cmd != nil {
		t.Fatalf("expected no async command for confirmation")
	}
	action, ok := m.pendingSelectionAction.(selectionBatchAction)
	if !ok {
		t.Fatalf("expected selectionBatchAction, got %T", m.pendingSelectionAction)
	}
	if len(action.operations) != 1 {
		t.Fatalf("expected one kill operation, got %d", len(action.operations))
	}
	if action.skippedCount != 1 {
		t.Fatalf("expected one skipped non-killable item, got %d", action.skippedCount)
	}
}

func TestInterruptHotkeyConfirmExecutesBatchAndClearsSelection(t *testing.T) {
	executor := &recordingSelectionOperationExecutor{}
	m := NewModel(nil, WithSelectionOperationExecutor(executor))
	m.workspaces = []*types.Workspace{{ID: "ws1", Name: "Workspace", RepoPath: "/tmp/ws1"}}
	m.worktrees = map[string][]*types.Worktree{}
	m.sessions = []*types.Session{{ID: "s1", Title: "Session", Status: types.SessionStatusRunning}}
	m.sessionMeta = map[string]*types.SessionMeta{
		"s1": {SessionID: "s1", WorkspaceID: "ws1", Title: "Session"},
	}
	workflows := []*guidedworkflows.WorkflowRun{
		{ID: "gwf-1", WorkspaceID: "ws1", TemplateName: "SOLID", Status: guidedworkflows.WorkflowRunStatusRunning},
	}
	m.sidebar.Apply(m.workspaces, m.worktrees, m.sessions, workflows, m.sessionMeta, "", "", false)
	selectSidebarItemKind(t, &m, sidebarSession)
	_, _ = m.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	selectSidebarItemKind(t, &m, sidebarWorkflow)
	_, _ = m.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	if !m.sidebar.HasSelectedKeys() {
		t.Fatalf("expected multi-selection before interrupt confirm")
	}

	handled, cmd := m.reduceSessionLifecycleKeys(keyRune('i'))
	if !handled {
		t.Fatalf("expected interrupt hotkey to be handled")
	}
	if cmd != nil {
		t.Fatalf("expected confirmation flow (no immediate async command)")
	}
	if m.confirm == nil || !m.confirm.IsOpen() {
		t.Fatalf("expected confirm modal to open")
	}

	confirmCmd := m.handleConfirmChoice(confirmChoiceConfirm)
	if confirmCmd == nil {
		t.Fatalf("expected command from confirmed interrupt/stop batch action")
	}
	if !executor.called {
		t.Fatalf("expected injected executor to run on confirm")
	}
	if len(executor.plan.Operations) != 2 {
		t.Fatalf("expected 2 operations, got %d", len(executor.plan.Operations))
	}
	if m.sidebar.HasSelectedKeys() {
		t.Fatalf("expected multi-selection to clear after confirm")
	}
}

func TestSelectionBatchCancelClearsSidebarSelectionSet(t *testing.T) {
	m := NewModel(nil)
	m.workspaces = []*types.Workspace{{ID: "ws1", Name: "Workspace", RepoPath: "/tmp/ws1"}}
	m.worktrees = map[string][]*types.Worktree{}
	m.sessions = []*types.Session{
		{ID: "s1", Title: "Session One", Status: types.SessionStatusRunning},
		{ID: "s2", Title: "Session Two", Status: types.SessionStatusRunning},
	}
	m.sessionMeta = map[string]*types.SessionMeta{
		"s1": {SessionID: "s1", WorkspaceID: "ws1", Title: "Session One"},
		"s2": {SessionID: "s2", WorkspaceID: "ws1", Title: "Session Two"},
	}
	m.sidebar.Apply(m.workspaces, m.worktrees, m.sessions, nil, m.sessionMeta, "", "", false)
	if !m.sidebar.SelectBySessionID("s1") {
		t.Fatalf("expected to select s1")
	}
	_, _ = m.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	if !m.sidebar.SelectBySessionID("s2") {
		t.Fatalf("expected to select s2")
	}
	_, _ = m.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	if !m.sidebar.HasSelectedKeys() {
		t.Fatalf("expected multi-selection before cancel")
	}

	handled, cmd := m.reduceSessionLifecycleKeys(keyRune('d'))
	if !handled {
		t.Fatalf("expected delete hotkey to be handled")
	}
	if cmd != nil {
		t.Fatalf("expected confirmation flow (no immediate async command)")
	}
	cancelCmd := m.handleConfirmChoice(confirmChoiceCancel)
	if cancelCmd != nil {
		t.Fatalf("expected no command on cancel")
	}
	if m.sidebar.HasSelectedKeys() {
		t.Fatalf("expected multi-selection to clear after cancel")
	}
}

func TestKillHotkeyConfirmExecutesActionableSubsetAndClearsSelection(t *testing.T) {
	executor := &recordingSelectionOperationExecutor{}
	m := NewModel(nil, WithSelectionOperationExecutor(executor))
	m.workspaces = []*types.Workspace{{ID: "ws1", Name: "Workspace", RepoPath: "/tmp/ws1"}}
	m.worktrees = map[string][]*types.Worktree{}
	m.sessions = []*types.Session{
		{ID: "s1", Title: "Running", Status: types.SessionStatusRunning},
		{ID: "s2", Title: "Exited", Status: types.SessionStatusExited},
	}
	m.sessionMeta = map[string]*types.SessionMeta{
		"s1": {SessionID: "s1", WorkspaceID: "ws1", Title: "Running"},
		"s2": {SessionID: "s2", WorkspaceID: "ws1", Title: "Exited"},
	}
	m.sidebar.Apply(m.workspaces, m.worktrees, m.sessions, nil, m.sessionMeta, "", "", false)
	if !m.sidebar.SelectBySessionID("s1") {
		t.Fatalf("expected to select s1")
	}
	_, _ = m.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	if !m.sidebar.SelectBySessionID("s2") {
		t.Fatalf("expected to select s2")
	}
	_, _ = m.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	if !m.sidebar.HasSelectedKeys() {
		t.Fatalf("expected multi-selection before kill confirm")
	}

	handled, cmd := m.reduceSessionLifecycleKeys(keyRune('x'))
	if !handled {
		t.Fatalf("expected kill hotkey to be handled")
	}
	if cmd != nil {
		t.Fatalf("expected confirmation flow (no immediate async command)")
	}
	confirmCmd := m.handleConfirmChoice(confirmChoiceConfirm)
	if confirmCmd == nil {
		t.Fatalf("expected command from confirmed kill batch action")
	}
	if !executor.called {
		t.Fatalf("expected injected executor to run on confirm")
	}
	if len(executor.plan.Operations) != 1 {
		t.Fatalf("expected one kill operation for actionable subset, got %d", len(executor.plan.Operations))
	}
	if executor.plan.Operations[0].kind != selectionOperationKillSession {
		t.Fatalf("expected kill operation kind, got %v", executor.plan.Operations[0].kind)
	}
	if m.sidebar.HasSelectedKeys() {
		t.Fatalf("expected multi-selection to clear after confirm")
	}
}

func TestInterruptAndKillHotkeysRequireActionableSelection(t *testing.T) {
	m := NewModel(nil)
	m.workspaces = []*types.Workspace{{ID: "ws1", Name: "Workspace", RepoPath: "/tmp/ws1"}}
	m.worktrees = map[string][]*types.Worktree{}
	m.sessions = []*types.Session{{ID: "s1", Title: "Exited", Status: types.SessionStatusExited}}
	m.sessionMeta = map[string]*types.SessionMeta{
		"s1": {SessionID: "s1", WorkspaceID: "ws1", Title: "Exited"},
	}
	workflows := []*guidedworkflows.WorkflowRun{
		{ID: "gwf-1", WorkspaceID: "ws1", TemplateName: "SOLID", Status: guidedworkflows.WorkflowRunStatusStopped},
	}
	m.sidebar.Apply(m.workspaces, m.worktrees, m.sessions, workflows, m.sessionMeta, "", "", false)
	selectSidebarItemKind(t, &m, sidebarSession)
	_, _ = m.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	selectSidebarItemKind(t, &m, sidebarWorkflow)
	_, _ = m.Update(tea.KeyPressMsg{Code: tea.KeySpace})

	handled, cmd := m.reduceSessionLifecycleKeys(keyRune('i'))
	if !handled {
		t.Fatalf("expected interrupt hotkey to be handled")
	}
	if cmd != nil {
		t.Fatalf("expected no command when interrupt selection is non-actionable")
	}
	if m.status != "selection has no interruptible or stoppable items" {
		t.Fatalf("unexpected interrupt validation status %q", m.status)
	}

	handled, cmd = m.reduceSessionLifecycleKeys(keyRune('x'))
	if !handled {
		t.Fatalf("expected kill hotkey to be handled")
	}
	if cmd != nil {
		t.Fatalf("expected no command when kill selection is non-actionable")
	}
	if m.status != "selection has no killable items" {
		t.Fatalf("unexpected kill validation status %q", m.status)
	}
}

func TestRenameHotkeyRequiresSelection(t *testing.T) {
	m := NewModel(nil)

	handled, cmd := m.reduceComposeAndWorkspaceEntryKeys(keyRune('m'))
	if !handled {
		t.Fatalf("expected rename hotkey to be handled")
	}
	if cmd != nil {
		t.Fatalf("expected no command when selection is missing")
	}
	if m.status != "select an item to rename" {
		t.Fatalf("unexpected status %q", m.status)
	}
}

func TestDeleteHotkeyRequiresSelection(t *testing.T) {
	m := NewModel(nil)

	handled, cmd := m.reduceSessionLifecycleKeys(keyRune('d'))
	if !handled {
		t.Fatalf("expected delete hotkey to be handled")
	}
	if cmd != nil {
		t.Fatalf("expected no command when selection is missing")
	}
	if m.status != "select an item to dismiss or delete" {
		t.Fatalf("unexpected status %q", m.status)
	}
}

func TestMenuKeyUsesCtrlMCanonicalBinding(t *testing.T) {
	m := NewModel(nil)
	handled, _ := m.reduceMenuAndAppKeys(keyRune('m'))
	if handled {
		t.Fatalf("expected plain m to not trigger menu with default bindings")
	}

	m.applyKeybindings(NewKeybindings(map[string]string{
		KeyCommandMenu: "m",
	}))
	handled, _ = m.reduceMenuAndAppKeys(keyRune('m'))
	if !handled {
		t.Fatalf("expected menu override key to be handled")
	}
	if m.menu == nil || !m.menu.IsActive() {
		t.Fatalf("expected menu to open")
	}
}

func TestEscOpensSettingsMenuInNormalMode(t *testing.T) {
	m := NewModel(nil)
	handled, cmd := m.reduceMenuAndAppKeys(tea.KeyPressMsg{Code: tea.KeyEsc})
	if !handled {
		t.Fatalf("expected esc to be handled")
	}
	if cmd != nil {
		t.Fatalf("expected no command on esc")
	}
	if m.settingsMenu == nil || !m.settingsMenu.IsOpen() {
		t.Fatalf("expected settings menu to open")
	}
}

func TestReduceGlobalKeyTogglesContextPanelWhenAllowed(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	if m.sidebar == nil || !m.sidebar.SelectBySessionID("s1") {
		t.Fatalf("expected selected session")
	}
	m.resize(180, 40)

	handled, cmd := m.reduceGlobalKey(tea.KeyPressMsg{Code: 'l', Mod: tea.ModCtrl}, globalKeyOptions{
		AllowToggleContext: true,
	})
	if !handled {
		t.Fatalf("expected ctrl+l to be handled when context toggle is allowed")
	}
	if cmd == nil {
		t.Fatalf("expected context toggle to return persistence command")
	}
	if !m.appState.ContextPanelHidden {
		t.Fatalf("expected context panel hidden after toggle")
	}
}

func TestReduceGlobalKeyIgnoresContextToggleWhenDisallowed(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	handled, cmd := m.reduceGlobalKey(tea.KeyPressMsg{Code: 'l', Mod: tea.ModCtrl}, globalKeyOptions{})
	if handled {
		t.Fatalf("expected ctrl+l to be ignored when context toggle is disallowed")
	}
	if cmd != nil {
		t.Fatalf("expected no command when toggle is disallowed")
	}
}

func TestReduceGlobalKeyMenuSettingsSidebarNotesDebugMatrix(t *testing.T) {
	newSessionModel := func(t *testing.T) Model {
		t.Helper()
		m := newPhase0ModelWithSession("codex")
		if m.sidebar == nil || !m.sidebar.SelectBySessionID("s1") {
			t.Fatalf("expected selected session")
		}
		m.resize(180, 40)
		return m
	}

	{
		m := newSessionModel(t)
		handled, cmd := m.reduceGlobalKey(tea.KeyPressMsg{Code: 'm', Mod: tea.ModCtrl}, globalKeyOptions{AllowMenu: true})
		if !handled || cmd != nil {
			t.Fatalf("expected menu toggle handled without command")
		}
		if m.menu == nil || !m.menu.IsActive() {
			t.Fatalf("expected menu to be open")
		}
	}

	{
		m := newSessionModel(t)
		handled, cmd := m.reduceGlobalKey(tea.KeyPressMsg{Code: tea.KeyEsc}, globalKeyOptions{AllowSettings: true})
		if !handled || cmd != nil {
			t.Fatalf("expected settings open handled without command")
		}
		if m.settingsMenu == nil || !m.settingsMenu.IsOpen() {
			t.Fatalf("expected settings menu to be open")
		}
	}

	{
		m := newSessionModel(t)
		handled, cmd := m.reduceGlobalKey(tea.KeyPressMsg{Code: 'b', Mod: tea.ModCtrl}, globalKeyOptions{AllowToggleSidebar: true})
		if !handled || cmd == nil {
			t.Fatalf("expected sidebar toggle handled with persistence command")
		}
	}

	{
		m := newSessionModel(t)
		m.notesPanelOpen = false
		handled, cmd := m.reduceGlobalKey(tea.KeyPressMsg{Code: 'o', Mod: tea.ModCtrl}, globalKeyOptions{AllowToggleNotes: true})
		if !handled || cmd == nil {
			t.Fatalf("expected notes toggle handled with reflow/fetch command")
		}
		if !m.notesPanelOpen {
			t.Fatalf("expected notes panel open after toggle")
		}
	}

	{
		m := newSessionModel(t)
		m.appState.DebugStreamsEnabled = false
		handled, cmd := m.reduceGlobalKey(tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl}, globalKeyOptions{AllowToggleDebug: true})
		if !handled || cmd == nil {
			t.Fatalf("expected debug toggle handled with command batch")
		}
		if !m.appState.DebugStreamsEnabled {
			t.Fatalf("expected debug streams enabled after toggle")
		}
	}
}

func TestOpenSettingsKeyUsesRebindableCommand(t *testing.T) {
	m := NewModel(nil)
	m.applyKeybindings(NewKeybindings(map[string]string{
		KeyCommandOpenSettings: "s",
	}))
	handled, cmd := m.reduceMenuAndAppKeys(keyRune('s'))
	if !handled {
		t.Fatalf("expected rebound open-settings key to be handled")
	}
	if cmd != nil {
		t.Fatalf("expected no command on rebound open-settings key")
	}
	if m.settingsMenu == nil || !m.settingsMenu.IsOpen() {
		t.Fatalf("expected settings menu to open from rebound key")
	}

	m.settingsMenu.Close()
	handled, _ = m.reduceMenuAndAppKeys(tea.KeyPressMsg{Code: tea.KeyEsc})
	if handled {
		t.Fatalf("expected esc to stop opening settings when open-settings is rebound")
	}
	if m.settingsMenu.IsOpen() {
		t.Fatalf("did not expect settings menu to open from default esc after rebinding")
	}
}

func TestEscDoesNotQuitWhenOpenSettingsAndQuitAreBothRebound(t *testing.T) {
	m := NewModel(nil)
	m.applyKeybindings(NewKeybindings(map[string]string{
		KeyCommandOpenSettings: "ctrl+shift+s",
		KeyCommandQuit:         "ctrl+q",
	}))

	handled, cmd := m.reduceMenuAndAppKeys(tea.KeyPressMsg{Code: tea.KeyEsc})
	if handled {
		t.Fatalf("expected esc to be unhandled when open-settings is rebound off esc")
	}
	if cmd != nil {
		t.Fatalf("did not expect quit command from esc with rebound quit key")
	}
}

func TestUpdateEscDoesNotQuitViaSidebarDefaultsWhenOpenSettingsRebound(t *testing.T) {
	m := NewModel(nil)
	m.applyKeybindings(NewKeybindings(map[string]string{
		KeyCommandOpenSettings: "ctrl+shift+s",
		KeyCommandQuit:         "ctrl+q",
	}))

	updated, _ := m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	next, ok := updated.(*Model)
	if !ok {
		t.Fatalf("expected model update result")
	}
	if next.settingsMenu != nil && next.settingsMenu.IsOpen() {
		t.Fatalf("did not expect esc to open settings when open-settings is rebound")
	}
}

func TestEscDoesNotOpenSettingsMenuInComposeMode(t *testing.T) {
	m := NewModel(nil)
	m.mode = uiModeCompose
	handled, _ := m.reduceMenuAndAppKeys(tea.KeyPressMsg{Code: tea.KeyEsc})
	if !handled {
		t.Fatalf("expected esc to be handled")
	}
	if m.settingsMenu != nil && m.settingsMenu.IsOpen() {
		t.Fatalf("did not expect settings menu to open in compose mode")
	}
}

func TestEscClosesStatusHistoryBeforeSettingsMenu(t *testing.T) {
	m := NewModel(nil)
	m.resize(120, 40)
	m.setStatusMessage("ready")
	m.statusHistoryOverlay.Open()

	updated, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeyEsc})
	if cmd != nil {
		t.Fatalf("expected no command on esc")
	}
	next, ok := updated.(*Model)
	if !ok {
		t.Fatalf("expected model update result")
	}
	if next.statusHistoryOverlayOpen() {
		t.Fatalf("expected esc to close status history overlay")
	}
	if next.settingsMenu != nil && next.settingsMenu.IsOpen() {
		t.Fatalf("expected esc not to open settings menu while closing status history")
	}
}

func TestStatusHistoryReducerFallsThroughForUnhandledKeys(t *testing.T) {
	m := NewModel(nil)
	m.statusHistoryOverlay.Open()

	handled, cmd := m.reduceStatusHistoryKey(keyRune('q'))
	if handled {
		t.Fatalf("expected unhandled key to fall through while status history is open")
	}
	if cmd != nil {
		t.Fatalf("expected no command for unhandled status history key")
	}
}

func TestStatusHistoryReducerHandlesNavigationKeys(t *testing.T) {
	m := NewModel(nil)
	m.resize(120, 40)
	m.setStatusMessage("one")
	m.setStatusMessage("two")
	m.setStatusMessage("three")
	m.statusHistoryOverlay.Open()

	handled, cmd := m.reduceStatusHistoryKey(tea.KeyPressMsg{Code: tea.KeyDown})
	if !handled || cmd != nil {
		t.Fatalf("expected down key handled without command")
	}
	if got := m.statusHistoryOverlay.SelectedIndex(); got != 0 {
		t.Fatalf("expected selected index 0 after first down, got %d", got)
	}
	handled, _ = m.reduceStatusHistoryKey(tea.KeyPressMsg{Code: tea.KeyUp})
	if !handled {
		t.Fatalf("expected up key handled")
	}
	if got := m.statusHistoryOverlay.SelectedIndex(); got != 0 {
		t.Fatalf("expected up key to clamp selection at 0, got %d", got)
	}
	handled, _ = m.reduceStatusHistoryKey(tea.KeyPressMsg{Code: tea.KeyEnd})
	if !handled {
		t.Fatalf("expected end key handled")
	}
	if got := m.statusHistoryOverlay.SelectedIndex(); got != 2 {
		t.Fatalf("expected end key to move to last index, got %d", got)
	}
	handled, _ = m.reduceStatusHistoryKey(tea.KeyPressMsg{Code: tea.KeyHome})
	if !handled {
		t.Fatalf("expected home key handled")
	}
	if got := m.statusHistoryOverlay.SelectedIndex(); got != 0 {
		t.Fatalf("expected home key to move to first index, got %d", got)
	}
	handled, _ = m.reduceStatusHistoryKey(tea.KeyPressMsg{Code: tea.KeyPgDown})
	if !handled {
		t.Fatalf("expected pgdown key handled")
	}
	handled, _ = m.reduceStatusHistoryKey(tea.KeyPressMsg{Code: tea.KeyPgUp})
	if !handled {
		t.Fatalf("expected pgup key handled")
	}
}

func TestStatusHistoryReducerKeyboardCopyEmitsCommand(t *testing.T) {
	m := NewModel(nil)
	m.setStatusMessage("copy me")
	m.statusHistoryOverlay.Open()
	m.statusHistoryOverlay.Select(0, 1, 1)

	handled, cmd := m.reduceStatusHistoryKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !handled {
		t.Fatalf("expected enter key handled")
	}
	if cmd == nil {
		t.Fatalf("expected enter key to emit copy command")
	}
}

func TestNotesNewOverrideWorksFromSidebarSelection(t *testing.T) {
	m := NewModel(nil)
	m.workspaces = []*types.Workspace{{ID: "ws1", Name: "Workspace", RepoPath: "/tmp/ws1"}}
	m.worktrees = map[string][]*types.Worktree{}
	m.sessions = []*types.Session{{ID: "s1", Title: "Session", Status: types.SessionStatusExited}}
	m.sessionMeta = map[string]*types.SessionMeta{
		"s1": {SessionID: "s1", WorkspaceID: "ws1", Title: "Session"},
	}
	m.sidebar.Apply(m.workspaces, m.worktrees, m.sessions, nil, m.sessionMeta, "", "", false)
	selectSidebarItemKind(t, &m, sidebarSession)
	m.applyKeybindings(NewKeybindings(map[string]string{
		KeyCommandNotesNew:   "ctrl+n",
		KeyCommandNewSession: "ctrl+s",
	}))

	handled, cmd := m.reduceNormalModeKey(tea.KeyPressMsg{Code: 'n', Mod: tea.ModCtrl})
	if !handled {
		t.Fatalf("expected notes-new override to be handled")
	}
	_ = cmd
	if m.mode != uiModeAddNote {
		t.Fatalf("expected add note mode, got %v", m.mode)
	}
	if m.notesScope.Scope != types.NoteScopeSession || m.notesScope.SessionID != "s1" {
		t.Fatalf("unexpected note scope: %#v", m.notesScope)
	}
}

func TestSpaceTogglesSidebarMultiSelect(t *testing.T) {
	m := NewModel(nil)
	m.workspaces = []*types.Workspace{{ID: "ws1", Name: "Workspace", RepoPath: "/tmp/ws1"}}
	m.worktrees = map[string][]*types.Worktree{}
	m.sessions = []*types.Session{
		{ID: "s1", Title: "Session One", Status: types.SessionStatusExited},
		{ID: "s2", Title: "Session Two", Status: types.SessionStatusExited},
	}
	m.sessionMeta = map[string]*types.SessionMeta{
		"s1": {SessionID: "s1", WorkspaceID: "ws1", Title: "Session One"},
		"s2": {SessionID: "s2", WorkspaceID: "ws1", Title: "Session Two"},
	}
	m.sidebar.Apply(m.workspaces, m.worktrees, m.sessions, nil, m.sessionMeta, "", "", false)
	selectSidebarItemKind(t, &m, sidebarSession)

	_, cmd := m.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	if cmd != nil {
		t.Fatalf("expected no command for space key in normal mode")
	}
	selected := m.sidebar.SelectedItems()
	if len(selected) != 1 {
		t.Fatalf("expected one selected item after first space toggle, got %d", len(selected))
	}
	if got := m.selectedSessionID(); got != "s1" {
		t.Fatalf("expected focused session to remain unchanged, got %q", got)
	}

	_, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyDown})
	_, _ = m.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	selected = m.sidebar.SelectedItems()
	if len(selected) != 2 {
		t.Fatalf("expected two selected items after second toggle, got %d", len(selected))
	}
	_, _ = m.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	selected = m.sidebar.SelectedItems()
	if len(selected) != 1 {
		t.Fatalf("expected one selected item after toggling focused row off, got %d", len(selected))
	}
}

func TestEnterTogglesWorkspaceExpansion(t *testing.T) {
	m := NewModel(nil)
	m.appState.ActiveWorkspaceGroupIDs = []string{"ungrouped"}
	m.workspaces = []*types.Workspace{{ID: "ws1", Name: "Workspace", RepoPath: "/tmp/ws1"}}
	m.worktrees = map[string][]*types.Worktree{}
	m.sessions = []*types.Session{{ID: "s1", Title: "Session", Status: types.SessionStatusRunning}}
	m.sessionMeta = map[string]*types.SessionMeta{
		"s1": {SessionID: "s1", WorkspaceID: "ws1"},
	}
	m.applySidebarItems()
	selectSidebarItemKind(t, &m, sidebarWorkspace)

	handled, _ := m.reduceComposeAndWorkspaceEntryKeys(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !handled {
		t.Fatalf("expected enter key to be handled")
	}
	if len(m.sidebar.Items()) != 1 {
		t.Fatalf("expected workspace collapse to hide nested session, got %d rows", len(m.sidebar.Items()))
	}
	if expanded := m.appState.SidebarWorkspaceExpanded["ws1"]; expanded {
		t.Fatalf("expected persisted workspace expansion override ws1=false")
	}
}

func TestEnterOpensSelectedWorkflow(t *testing.T) {
	m := NewModel(nil)
	m.appState.ActiveWorkspaceGroupIDs = []string{"ungrouped"}
	m.workspaces = []*types.Workspace{{ID: "ws1", Name: "Workspace", RepoPath: "/tmp/ws1"}}
	m.worktrees = map[string][]*types.Worktree{}
	m.sessions = []*types.Session{{ID: "s1", Title: "Session", Status: types.SessionStatusRunning}}
	m.sessionMeta = map[string]*types.SessionMeta{
		"s1": {SessionID: "s1", WorkspaceID: "ws1", WorkflowRunID: "gwf-1"},
	}
	workflows := []*guidedworkflows.WorkflowRun{
		{ID: "gwf-1", WorkspaceID: "ws1", TemplateName: "SOLID", Status: guidedworkflows.WorkflowRunStatusRunning},
	}
	m.sidebar.Apply(m.workspaces, m.worktrees, m.sessions, workflows, m.sessionMeta, "", "", false)
	selectSidebarItemKind(t, &m, sidebarWorkflow)

	handled, _ := m.reduceComposeAndWorkspaceEntryKeys(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !handled {
		t.Fatalf("expected enter key to be handled for workflow")
	}
	if m.mode != uiModeGuidedWorkflow {
		t.Fatalf("expected guided workflow mode, got %v", m.mode)
	}
	if m.guidedWorkflow == nil || m.guidedWorkflow.RunID() != "gwf-1" {
		t.Fatalf("expected selected guided workflow run gwf-1 to be opened")
	}
}

func TestWorkflowOpenShortcutUsesLowercaseO(t *testing.T) {
	m := NewModel(nil)
	m.appState.ActiveWorkspaceGroupIDs = []string{"ungrouped"}
	m.workspaces = []*types.Workspace{{ID: "ws1", Name: "Workspace", RepoPath: "/tmp/ws1"}}
	m.worktrees = map[string][]*types.Worktree{}
	workflows := []*guidedworkflows.WorkflowRun{
		{ID: "gwf-1", WorkspaceID: "ws1", TemplateName: "SOLID", Status: guidedworkflows.WorkflowRunStatusRunning},
	}
	m.sidebar.Apply(m.workspaces, m.worktrees, nil, workflows, map[string]*types.SessionMeta{}, "", "", false)
	selectSidebarItemKind(t, &m, sidebarWorkflow)

	handled, cmd := m.reduceComposeAndWorkspaceEntryKeys(keyRune('o'))
	if !handled {
		t.Fatalf("expected workflow open shortcut to be handled")
	}
	if cmd == nil {
		t.Fatalf("expected workflow open shortcut to return snapshot command")
	}
	if m.mode != uiModeGuidedWorkflow {
		t.Fatalf("expected guided workflow mode, got %v", m.mode)
	}
	if m.guidedWorkflow == nil || m.guidedWorkflow.RunID() != "gwf-1" {
		t.Fatalf("expected selected guided workflow run gwf-1 to be opened")
	}
}

func TestWorkflowSelectionStaysSidebarInteractiveUntilExplicitOpen(t *testing.T) {
	m := NewModel(nil)
	m.appState.ActiveWorkspaceGroupIDs = []string{"ungrouped"}
	m.workspaces = []*types.Workspace{{ID: "ws1", Name: "Workspace", RepoPath: "/tmp/ws1"}}
	m.worktrees = map[string][]*types.Worktree{}
	workflows := []*guidedworkflows.WorkflowRun{
		{ID: "gwf-1", WorkspaceID: "ws1", TemplateName: "SOLID", Status: guidedworkflows.WorkflowRunStatusRunning},
	}
	m.sidebar.Apply(m.workspaces, m.worktrees, nil, workflows, map[string]*types.SessionMeta{}, "", "", false)
	selectSidebarItemKind(t, &m, sidebarWorkflow)

	_ = m.onSelectionChangedImmediate()
	if m.mode == uiModeGuidedWorkflow {
		t.Fatalf("expected guided workflow mode to remain closed after workflow selection")
	}

	handled, renameCmd := m.reduceComposeAndWorkspaceEntryKeys(keyRune('m'))
	if !handled {
		t.Fatalf("expected rename hotkey to be handled")
	}
	if renameCmd != nil {
		t.Fatalf("expected rename hotkey to avoid async command")
	}
	if m.mode != uiModeRenameWorkflow {
		t.Fatalf("expected rename workflow mode, got %v", m.mode)
	}
}

func TestStartGuidedWorkflowHotkeyFromWorkspaceSelection(t *testing.T) {
	m := NewModel(nil)
	m.workspaces = []*types.Workspace{{ID: "ws1", Name: "Workspace", RepoPath: "/tmp/ws1"}}
	m.worktrees = map[string][]*types.Worktree{}
	m.sidebar.Apply(m.workspaces, m.worktrees, nil, nil, nil, "", "", false)

	handled, cmd := m.reduceComposeAndWorkspaceEntryKeys(keyRune('w'))
	if !handled {
		t.Fatalf("expected start guided workflow hotkey to be handled")
	}
	if cmd == nil {
		t.Fatalf("expected workflow template fetch command")
	}
	if _, ok := cmd().(workflowTemplatesMsg); !ok {
		t.Fatalf("expected workflowTemplatesMsg, got %T", cmd())
	}
	if m.mode != uiModeGuidedWorkflow {
		t.Fatalf("expected guided workflow mode, got %v", m.mode)
	}
	if m.guidedWorkflow == nil {
		t.Fatalf("expected guided workflow controller")
	}
	if got := m.guidedWorkflow.context.workspaceID; got != "ws1" {
		t.Fatalf("expected workspace id ws1, got %q", got)
	}
}

func TestStartGuidedWorkflowHotkeyFromWorktreeSelection(t *testing.T) {
	m := NewModel(nil)
	m.workspaces = []*types.Workspace{{ID: "ws1", Name: "Workspace", RepoPath: "/tmp/ws1"}}
	m.worktrees = map[string][]*types.Worktree{
		"ws1": {
			{ID: "wt1", WorkspaceID: "ws1", Name: "feature/retry-cleanup", Path: "/tmp/ws1/wt1"},
		},
	}
	m.sidebar.Apply(m.workspaces, m.worktrees, nil, nil, nil, "", "", false)
	selectSidebarItemKind(t, &m, sidebarWorktree)

	handled, cmd := m.reduceComposeAndWorkspaceEntryKeys(keyRune('w'))
	if !handled {
		t.Fatalf("expected start guided workflow hotkey to be handled")
	}
	if cmd == nil {
		t.Fatalf("expected workflow template fetch command")
	}
	if _, ok := cmd().(workflowTemplatesMsg); !ok {
		t.Fatalf("expected workflowTemplatesMsg, got %T", cmd())
	}
	if m.mode != uiModeGuidedWorkflow {
		t.Fatalf("expected guided workflow mode, got %v", m.mode)
	}
	if m.guidedWorkflow == nil {
		t.Fatalf("expected guided workflow controller")
	}
	if got := m.guidedWorkflow.context.workspaceID; got != "ws1" {
		t.Fatalf("expected workspace id ws1, got %q", got)
	}
	if got := m.guidedWorkflow.context.worktreeID; got != "wt1" {
		t.Fatalf("expected worktree id wt1, got %q", got)
	}
}

func TestStartGuidedWorkflowHotkeyFromSessionSelection(t *testing.T) {
	m := NewModel(nil)
	m.workspaces = []*types.Workspace{{ID: "ws1", Name: "Workspace", RepoPath: "/tmp/ws1"}}
	m.worktrees = map[string][]*types.Worktree{
		"ws1": {
			{ID: "wt1", WorkspaceID: "ws1", Name: "feature/retry-cleanup", Path: "/tmp/ws1/wt1"},
		},
	}
	m.sessions = []*types.Session{{ID: "s1", Title: "Retry policy cleanup", Status: types.SessionStatusExited}}
	m.sessionMeta = map[string]*types.SessionMeta{
		"s1": {SessionID: "s1", WorkspaceID: "ws1", WorktreeID: "wt1", Title: "Retry policy cleanup"},
	}
	m.sidebar.Apply(m.workspaces, m.worktrees, m.sessions, nil, m.sessionMeta, "", "", false)
	selectSidebarItemKind(t, &m, sidebarSession)

	handled, cmd := m.reduceComposeAndWorkspaceEntryKeys(keyRune('w'))
	if !handled {
		t.Fatalf("expected start guided workflow hotkey to be handled")
	}
	if cmd == nil {
		t.Fatalf("expected workflow template fetch command")
	}
	if _, ok := cmd().(workflowTemplatesMsg); !ok {
		t.Fatalf("expected workflowTemplatesMsg, got %T", cmd())
	}
	if m.mode != uiModeGuidedWorkflow {
		t.Fatalf("expected guided workflow mode, got %v", m.mode)
	}
	if m.guidedWorkflow == nil {
		t.Fatalf("expected guided workflow controller")
	}
	if got := m.guidedWorkflow.context.workspaceID; got != "ws1" {
		t.Fatalf("expected workspace id ws1, got %q", got)
	}
	if got := m.guidedWorkflow.context.worktreeID; got != "wt1" {
		t.Fatalf("expected worktree id wt1, got %q", got)
	}
	if got := m.guidedWorkflow.context.sessionID; got != "s1" {
		t.Fatalf("expected session id s1, got %q", got)
	}
}

func TestStartGuidedWorkflowHotkeyOverrideWorks(t *testing.T) {
	m := NewModel(nil)
	m.workspaces = []*types.Workspace{{ID: "ws1", Name: "Workspace", RepoPath: "/tmp/ws1"}}
	m.worktrees = map[string][]*types.Worktree{}
	m.sidebar.Apply(m.workspaces, m.worktrees, nil, nil, nil, "", "", false)
	m.applyKeybindings(NewKeybindings(map[string]string{
		KeyCommandStartGuidedWorkflow: "ctrl+w",
	}))

	handled, cmd := m.reduceComposeAndWorkspaceEntryKeys(tea.KeyPressMsg{Code: 'w', Mod: tea.ModCtrl})
	if !handled {
		t.Fatalf("expected overridden start guided workflow hotkey to be handled")
	}
	if cmd == nil {
		t.Fatalf("expected workflow template fetch command")
	}
	if _, ok := cmd().(workflowTemplatesMsg); !ok {
		t.Fatalf("expected workflowTemplatesMsg, got %T", cmd())
	}
	if m.mode != uiModeGuidedWorkflow {
		t.Fatalf("expected guided workflow mode, got %v", m.mode)
	}
}

func TestStartGuidedWorkflowHotkeyRequiresSupportedSelection(t *testing.T) {
	m := NewModel(nil)

	handled, cmd := m.reduceComposeAndWorkspaceEntryKeys(keyRune('w'))
	if !handled {
		t.Fatalf("expected start guided workflow hotkey to be handled")
	}
	if cmd != nil {
		t.Fatalf("expected no command without a valid selection")
	}
	if m.status != "select a workspace, worktree, or session to start guided workflow" {
		t.Fatalf("unexpected status %q", m.status)
	}
}

func TestSidebarArrowLeftRightCollapseAndExpandSelection(t *testing.T) {
	m := NewModel(nil)
	m.appState.ActiveWorkspaceGroupIDs = []string{"ungrouped"}
	m.workspaces = []*types.Workspace{{ID: "ws1", Name: "Workspace", RepoPath: "/tmp/ws1"}}
	m.worktrees = map[string][]*types.Worktree{}
	m.sessions = []*types.Session{{ID: "s1", Title: "Session", Status: types.SessionStatusRunning}}
	m.sessionMeta = map[string]*types.SessionMeta{
		"s1": {SessionID: "s1", WorkspaceID: "ws1"},
	}
	m.applySidebarItems()
	selectSidebarItemKind(t, &m, sidebarWorkspace)

	handled, _ := m.reduceSidebarArrowKey(tea.KeyPressMsg{Code: tea.KeyLeft})
	if !handled {
		t.Fatalf("expected left arrow to be handled")
	}
	if len(m.sidebar.Items()) != 1 {
		t.Fatalf("expected collapsed workspace list length 1, got %d", len(m.sidebar.Items()))
	}

	handled, _ = m.reduceSidebarArrowKey(tea.KeyPressMsg{Code: tea.KeyRight})
	if !handled {
		t.Fatalf("expected right arrow to be handled")
	}
	if len(m.sidebar.Items()) != 2 {
		t.Fatalf("expected expanded workspace list length 2, got %d", len(m.sidebar.Items()))
	}
}

func TestSelectionHistoryHotkeysNavigateBackAndForward(t *testing.T) {
	m := NewModel(nil)
	m.resize(120, 40)
	m.appState.ActiveWorkspaceGroupIDs = []string{"ungrouped"}
	m.workspaces = []*types.Workspace{{ID: "ws1", Name: "Workspace", RepoPath: "/tmp/ws1"}}
	m.worktrees = map[string][]*types.Worktree{}
	m.sessions = []*types.Session{
		{ID: "s1", Status: types.SessionStatusRunning},
		{ID: "s2", Status: types.SessionStatusRunning},
	}
	m.sessionMeta = map[string]*types.SessionMeta{
		"s1": {SessionID: "s1", WorkspaceID: "ws1"},
		"s2": {SessionID: "s2", WorkspaceID: "ws1"},
	}
	m.applySidebarItems()

	if !m.sidebar.SelectBySessionID("s1") {
		t.Fatalf("expected to select s1")
	}
	_ = m.onSystemSelectionChangedImmediate()
	if !m.sidebar.SelectBySessionID("s2") {
		t.Fatalf("expected to select s2")
	}
	_ = m.onSelectionChangedImmediate()

	handled, _ := m.reduceSelectionKeys(tea.KeyPressMsg{Code: tea.KeyLeft, Mod: tea.ModAlt})
	if !handled {
		t.Fatalf("expected alt+left to be handled")
	}
	if got := m.selectedSessionID(); got != "s1" {
		t.Fatalf("expected history back hotkey to select s1, got %q", got)
	}

	handled, _ = m.reduceSelectionKeys(tea.KeyPressMsg{Code: tea.KeyRight, Mod: tea.ModAlt})
	if !handled {
		t.Fatalf("expected alt+right to be handled")
	}
	if got := m.selectedSessionID(); got != "s2" {
		t.Fatalf("expected history forward hotkey to select s2, got %q", got)
	}
}

func selectSidebarItemKind(t *testing.T, m *Model, kind sidebarItemKind) {
	t.Helper()
	if m == nil || m.sidebar == nil {
		t.Fatalf("missing sidebar")
	}
	for i, item := range m.sidebar.Items() {
		entry, ok := item.(*sidebarItem)
		if !ok || entry == nil {
			continue
		}
		if entry.kind == kind {
			m.sidebar.Select(i)
			return
		}
	}
	t.Fatalf("did not find sidebar item kind %v", kind)
}

func keyRune(r rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Text: string(r)}
}
