package app

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"control/internal/guidedworkflows"
	"control/internal/types"

	tea "charm.land/bubbletea/v2"
	xansi "github.com/charmbracelet/x/ansi"
)

func TestMouseReducerLeftPressOutsideContextMenuCloses(t *testing.T) {
	m := NewModel(nil)
	m.resize(120, 40)
	if m.contextMenu == nil {
		t.Fatalf("expected context menu controller")
	}
	m.contextMenu.OpenSession("s1", "", "", "Session", 2, 2)

	handled := m.reduceContextMenuLeftPressMouse(tea.MouseClickMsg{Button: tea.MouseLeft, X: 119, Y: 39})
	if handled {
		t.Fatalf("expected outside click to remain unhandled")
	}
	if m.contextMenu.IsOpen() {
		t.Fatalf("expected context menu to close on outside click")
	}
}

func TestMouseReducerRightPressClosesContextMenu(t *testing.T) {
	m := NewModel(nil)
	m.resize(120, 40)
	if m.contextMenu == nil {
		t.Fatalf("expected context menu controller")
	}
	m.contextMenu.OpenSession("s1", "", "", "Session", 2, 2)
	layout := m.resolveMouseLayout()

	handled := m.reduceContextMenuRightPressMouse(tea.MouseClickMsg{Button: tea.MouseRight, X: layout.rightStart, Y: 0}, layout)
	if !handled {
		t.Fatalf("expected right click to be handled")
	}
	if m.contextMenu.IsOpen() {
		t.Fatalf("expected context menu to close")
	}
}

func TestMouseReducerRightReleaseDoesNotOpenContextMenu(t *testing.T) {
	m := NewModel(nil)
	m.resize(120, 40)
	m.workspaces = []*types.Workspace{{ID: "ws1", Name: "Workspace", RepoPath: "/tmp/ws1"}}
	m.worktrees = map[string][]*types.Worktree{}
	m.sidebar.Apply(m.workspaces, m.worktrees, nil, nil, nil, "", "", false)
	if m.contextMenu == nil {
		t.Fatalf("expected context menu controller")
	}

	row := -1
	for y := 0; y < 20; y++ {
		if m.sidebar.ItemAtRow(y) != nil {
			row = y
			break
		}
	}
	if row < 0 {
		t.Fatalf("expected at least one visible sidebar row")
	}

	handled := m.handleMouse(tea.MouseReleaseMsg{Button: tea.MouseRight, X: 1, Y: row})
	if handled {
		t.Fatalf("expected right-button release to be ignored")
	}
	if m.contextMenu.IsOpen() {
		t.Fatalf("expected context menu to remain closed on right-button release")
	}
}

func TestMouseReducerBackwardAndForwardButtonsNavigateSelectionHistory(t *testing.T) {
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

	if got := m.selectedSessionID(); got != "s2" {
		t.Fatalf("expected selected session s2 before back, got %q", got)
	}
	if handled := m.handleMouse(tea.MouseClickMsg{Button: tea.MouseBackward, X: 1, Y: 1}); !handled {
		t.Fatalf("expected back mouse click to be handled")
	}
	if got := m.selectedSessionID(); got != "s1" {
		t.Fatalf("expected back navigation to select s1, got %q", got)
	}

	if handled := m.handleMouse(tea.MouseClickMsg{Button: tea.MouseForward, X: 1, Y: 1}); !handled {
		t.Fatalf("expected forward mouse click to be handled")
	}
	if got := m.selectedSessionID(); got != "s2" {
		t.Fatalf("expected forward navigation to select s2, got %q", got)
	}
}

func TestMouseReducerButtonTenNavigatesSelectionHistoryBack(t *testing.T) {
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

	if handled := m.handleMouse(tea.MouseClickMsg{Button: tea.MouseButton10, X: 1, Y: 1}); !handled {
		t.Fatalf("expected mouse button10 click to be handled")
	}
	if got := m.selectedSessionID(); got != "s1" {
		t.Fatalf("expected button10 navigation to select s1, got %q", got)
	}

}

func TestMouseReducerBackwardButtonReleaseIsIgnored(t *testing.T) {
	m := NewModel(nil)
	m.resize(120, 40)
	if handled := m.handleMouse(tea.MouseReleaseMsg{Button: tea.MouseBackward, X: 1, Y: 1}); handled {
		t.Fatalf("expected back-button release to be ignored")
	}
}

func TestMouseReducerSystemSelectionSyncDoesNotCreateBackEntry(t *testing.T) {
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
	_ = m.onSystemSelectionChangedImmediate()

	if handled := m.handleMouse(tea.MouseClickMsg{Button: tea.MouseBackward, X: 1, Y: 1}); !handled {
		t.Fatalf("expected back mouse click to be handled")
	}
	if got := m.selectedSessionID(); got != "s2" {
		t.Fatalf("expected system-only selection changes to keep current session, got %q", got)
	}
}

func TestMouseReducerSidebarWorkspaceRowClickSelectsWithoutToggle(t *testing.T) {
	m := NewModel(nil)
	m.resize(120, 40)
	m.appState.ActiveWorkspaceGroupIDs = []string{"ungrouped"}
	m.workspaces = []*types.Workspace{{ID: "ws1", Name: "Workspace", RepoPath: "/tmp/ws1"}}
	m.worktrees = map[string][]*types.Worktree{}
	m.sessions = []*types.Session{{ID: "s1", Status: types.SessionStatusRunning}}
	m.sessionMeta = map[string]*types.SessionMeta{
		"s1": {SessionID: "s1", WorkspaceID: "ws1"},
	}
	m.applySidebarItems()
	layout := m.resolveMouseLayout()

	row := -1
	for y := 0; y < 20; y++ {
		entry := m.sidebar.ItemAtRow(y)
		if entry != nil && entry.kind == sidebarWorkspace {
			row = y
			break
		}
	}
	if row < 0 {
		t.Fatalf("expected visible workspace row")
	}
	handled := m.reduceSidebarSelectionLeftPressMouse(tea.MouseClickMsg{Button: tea.MouseLeft, X: 4, Y: row}, layout)
	if !handled {
		t.Fatalf("expected sidebar click to be handled")
	}
	if len(m.sidebar.Items()) != 2 {
		t.Fatalf("expected workspace row click to keep expansion, got %d rows", len(m.sidebar.Items()))
	}
	if m.appState.ActiveWorkspaceID != "ws1" {
		t.Fatalf("expected workspace row click to select ws1, got %q", m.appState.ActiveWorkspaceID)
	}
}

func TestMouseReducerSidebarWorkspaceCaretClickTogglesExpansion(t *testing.T) {
	m := NewModel(nil)
	m.resize(120, 40)
	m.appState.ActiveWorkspaceGroupIDs = []string{"ungrouped"}
	m.workspaces = []*types.Workspace{{ID: "ws1", Name: "Workspace", RepoPath: "/tmp/ws1"}}
	m.worktrees = map[string][]*types.Worktree{}
	m.sessions = []*types.Session{{ID: "s1", Status: types.SessionStatusRunning}}
	m.sessionMeta = map[string]*types.SessionMeta{
		"s1": {SessionID: "s1", WorkspaceID: "ws1"},
	}
	m.applySidebarItems()
	layout := m.resolveMouseLayout()

	row := -1
	for y := 0; y < 20; y++ {
		entry := m.sidebar.ItemAtRow(y)
		if entry != nil && entry.kind == sidebarWorkspace {
			row = y
			break
		}
	}
	if row < 0 {
		t.Fatalf("expected visible workspace row")
	}
	handled := m.reduceSidebarSelectionLeftPressMouse(tea.MouseClickMsg{Button: tea.MouseLeft, X: 1, Y: row}, layout)
	if !handled {
		t.Fatalf("expected sidebar click to be handled")
	}
	if len(m.sidebar.Items()) != 1 {
		t.Fatalf("expected workspace caret click to collapse nested session, got %d rows", len(m.sidebar.Items()))
	}
}

func TestMouseReducerSidebarWorktreeRowClickSelectsWithoutToggle(t *testing.T) {
	m := NewModel(nil)
	m.resize(120, 40)
	m.appState.ActiveWorkspaceGroupIDs = []string{"ungrouped"}
	m.workspaces = []*types.Workspace{{ID: "ws1", Name: "Workspace", RepoPath: "/tmp/ws1"}}
	m.worktrees = map[string][]*types.Worktree{
		"ws1": {&types.Worktree{ID: "wt1", WorkspaceID: "ws1", Name: "feature"}},
	}
	m.sessions = []*types.Session{{ID: "s1", Status: types.SessionStatusRunning}}
	m.sessionMeta = map[string]*types.SessionMeta{
		"s1": {SessionID: "s1", WorkspaceID: "ws1", WorktreeID: "wt1"},
	}
	m.applySidebarItems()
	layout := m.resolveMouseLayout()

	row := -1
	for y := 0; y < 20; y++ {
		entry := m.sidebar.ItemAtRow(y)
		if entry != nil && entry.kind == sidebarWorktree {
			row = y
			break
		}
	}
	if row < 0 {
		t.Fatalf("expected visible worktree row")
	}
	handled := m.reduceSidebarSelectionLeftPressMouse(tea.MouseClickMsg{Button: tea.MouseLeft, X: 6, Y: row}, layout)
	if !handled {
		t.Fatalf("expected sidebar click to be handled")
	}
	if len(m.sidebar.Items()) != 3 {
		t.Fatalf("expected worktree row click to keep expansion, got %d rows", len(m.sidebar.Items()))
	}
	if m.appState.ActiveWorkspaceID != "ws1" || m.appState.ActiveWorktreeID != "wt1" {
		t.Fatalf("expected worktree row click to select ws1/wt1, got %q/%q", m.appState.ActiveWorkspaceID, m.appState.ActiveWorktreeID)
	}
}

func TestMouseReducerSidebarWorktreeCaretClickTogglesExpansion(t *testing.T) {
	m := NewModel(nil)
	m.resize(120, 40)
	m.appState.ActiveWorkspaceGroupIDs = []string{"ungrouped"}
	m.workspaces = []*types.Workspace{{ID: "ws1", Name: "Workspace", RepoPath: "/tmp/ws1"}}
	m.worktrees = map[string][]*types.Worktree{
		"ws1": {&types.Worktree{ID: "wt1", WorkspaceID: "ws1", Name: "feature"}},
	}
	m.sessions = []*types.Session{{ID: "s1", Status: types.SessionStatusRunning}}
	m.sessionMeta = map[string]*types.SessionMeta{
		"s1": {SessionID: "s1", WorkspaceID: "ws1", WorktreeID: "wt1"},
	}
	m.applySidebarItems()
	layout := m.resolveMouseLayout()

	row := -1
	for y := 0; y < 20; y++ {
		entry := m.sidebar.ItemAtRow(y)
		if entry != nil && entry.kind == sidebarWorktree {
			row = y
			break
		}
	}
	if row < 0 {
		t.Fatalf("expected visible worktree row")
	}
	handled := m.reduceSidebarSelectionLeftPressMouse(tea.MouseClickMsg{Button: tea.MouseLeft, X: 3, Y: row}, layout)
	if !handled {
		t.Fatalf("expected sidebar click to be handled")
	}
	if len(m.sidebar.Items()) != 2 {
		t.Fatalf("expected worktree caret click to collapse nested session, got %d rows", len(m.sidebar.Items()))
	}
}

func TestMouseReducerSidebarWorkflowCaretClickTogglesExpansion(t *testing.T) {
	m := NewModel(nil)
	m.resize(120, 40)
	m.appState.ActiveWorkspaceGroupIDs = []string{"ungrouped"}
	m.workspaces = []*types.Workspace{{ID: "ws1", Name: "Workspace", RepoPath: "/tmp/ws1"}}
	m.worktrees = map[string][]*types.Worktree{}
	m.sessions = []*types.Session{{ID: "s1", Status: types.SessionStatusRunning}}
	m.sessionMeta = map[string]*types.SessionMeta{
		"s1": {SessionID: "s1", WorkspaceID: "ws1", WorkflowRunID: "gwf-1"},
	}
	m.applySidebarItems()
	layout := m.resolveMouseLayout()

	row := -1
	for y := 0; y < 20; y++ {
		entry := m.sidebar.ItemAtRow(y)
		if entry != nil && entry.kind == sidebarWorkflow {
			row = y
			break
		}
	}
	if row < 0 {
		t.Fatalf("expected visible workflow row")
	}
	handled := m.reduceSidebarSelectionLeftPressMouse(tea.MouseClickMsg{Button: tea.MouseLeft, X: 3, Y: row}, layout)
	if !handled {
		t.Fatalf("expected sidebar click to be handled")
	}
	if len(m.sidebar.Items()) != 2 {
		t.Fatalf("expected workflow caret click to collapse nested session, got %d rows", len(m.sidebar.Items()))
	}
}

func TestMouseReducerSidebarWorkflowRowClickKeepsSelectionPassive(t *testing.T) {
	m := NewModel(nil)
	m.resize(120, 40)
	m.appState.ActiveWorkspaceGroupIDs = []string{"ungrouped"}
	m.workspaces = []*types.Workspace{{ID: "ws1", Name: "Workspace", RepoPath: "/tmp/ws1"}}
	m.worktrees = map[string][]*types.Worktree{}
	m.sessions = []*types.Session{{ID: "s1", Status: types.SessionStatusRunning}}
	m.sessionMeta = map[string]*types.SessionMeta{
		"s1": {SessionID: "s1", WorkspaceID: "ws1", WorkflowRunID: "gwf-1"},
	}
	m.applySidebarItems()
	layout := m.resolveMouseLayout()

	row := -1
	for y := 0; y < 20; y++ {
		entry := m.sidebar.ItemAtRow(y)
		if entry != nil && entry.kind == sidebarWorkflow {
			row = y
			break
		}
	}
	if row < 0 {
		t.Fatalf("expected visible workflow row")
	}
	handled := m.reduceSidebarSelectionLeftPressMouse(tea.MouseClickMsg{Button: tea.MouseLeft, X: 6, Y: row}, layout)
	if !handled {
		t.Fatalf("expected sidebar click to be handled")
	}
	if m.mode == uiModeGuidedWorkflow {
		t.Fatalf("expected workflow row click to remain passive before command flush")
	}
	flushPendingMouseCmd(t, &m)
	if m.mode == uiModeGuidedWorkflow {
		t.Fatalf("expected workflow row click to remain passive after command flush")
	}
	if item := m.selectedItem(); item == nil || item.kind != sidebarWorkflow {
		t.Fatalf("expected workflow row to remain selected")
	}
}

func TestMouseReducerSidebarClickClearsMultiSelectionWhenTargetNotSelected(t *testing.T) {
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
	layout := m.resolveMouseLayout()

	if !m.sidebar.SelectBySessionID("s1") {
		t.Fatalf("expected to select s1")
	}
	_, _ = m.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	if !m.sidebar.SelectBySessionID("s2") {
		t.Fatalf("expected to select s2")
	}
	_, _ = m.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	if !m.sidebar.HasSelectedKeys() {
		t.Fatalf("expected multi-selection before click")
	}

	workspaceRow := -1
	for y := 0; y < 20; y++ {
		entry := m.sidebar.ItemAtRow(y)
		if entry != nil && entry.kind == sidebarWorkspace {
			workspaceRow = y
			break
		}
	}
	if workspaceRow < 0 {
		t.Fatalf("expected visible workspace row")
	}
	handled := m.reduceSidebarSelectionLeftPressMouse(tea.MouseClickMsg{Button: tea.MouseLeft, X: 4, Y: workspaceRow}, layout)
	if !handled {
		t.Fatalf("expected sidebar click to be handled")
	}
	if m.sidebar.HasSelectedKeys() {
		t.Fatalf("expected clicking non-selected row to clear multi-selection")
	}
}

func TestMouseReducerSidebarClickKeepsMultiSelectionWhenTargetAlreadySelected(t *testing.T) {
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
	layout := m.resolveMouseLayout()

	if !m.sidebar.SelectBySessionID("s1") {
		t.Fatalf("expected to select s1")
	}
	_, _ = m.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	if !m.sidebar.SelectBySessionID("s2") {
		t.Fatalf("expected to select s2")
	}
	_, _ = m.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	if !m.sidebar.IsKeySelected("sess:s1") || !m.sidebar.IsKeySelected("sess:s2") {
		t.Fatalf("expected both sessions selected before click")
	}

	sessionRow := -1
	for y := 0; y < 20; y++ {
		entry := m.sidebar.ItemAtRow(y)
		if entry != nil && entry.kind == sidebarSession && entry.session != nil && entry.session.ID == "s1" {
			sessionRow = y
			break
		}
	}
	if sessionRow < 0 {
		t.Fatalf("expected visible s1 row")
	}
	handled := m.reduceSidebarSelectionLeftPressMouse(tea.MouseClickMsg{Button: tea.MouseLeft, X: 6, Y: sessionRow}, layout)
	if !handled {
		t.Fatalf("expected sidebar click to be handled")
	}
	if !m.sidebar.IsKeySelected("sess:s1") || !m.sidebar.IsKeySelected("sess:s2") {
		t.Fatalf("expected selected-row click to preserve existing multi-selection")
	}
}

func TestMouseReducerLeftPressInputFocusesComposeInput(t *testing.T) {
	m := NewModel(nil)
	m.resize(120, 40)
	m.enterCompose("s1")
	if m.chatInput == nil || m.input == nil {
		t.Fatalf("expected input controllers")
	}
	m.chatInput.Blur()
	m.input.FocusSidebar()
	layout := m.resolveMouseLayout()
	y := m.viewport.Height() + 2

	handled := m.reduceInputFocusLeftPressMouse(tea.MouseClickMsg{Button: tea.MouseLeft, X: layout.rightStart, Y: y}, layout)
	if !handled {
		t.Fatalf("expected compose input click to be handled")
	}
	if !m.chatInput.Focused() {
		t.Fatalf("expected chat input to be focused")
	}
	if !m.input.IsChatFocused() {
		t.Fatalf("expected input controller focus to switch to chat")
	}
}

func TestMouseReducerLeftPressInputFocusesAddNoteInput(t *testing.T) {
	m := NewModel(nil)
	m.resize(120, 40)
	m.mode = uiModeAddNote
	m.notesScope = noteScopeTarget{Scope: types.NoteScopeWorkspace, WorkspaceID: "ws1"}
	if m.noteInput == nil || m.input == nil {
		t.Fatalf("expected note input controllers")
	}
	m.noteInput.Blur()
	m.input.FocusSidebar()
	layout := m.resolveMouseLayout()
	y := m.viewport.Height() + 2

	handled := m.reduceInputFocusLeftPressMouse(tea.MouseClickMsg{Button: tea.MouseLeft, X: layout.rightStart, Y: y}, layout)
	if !handled {
		t.Fatalf("expected add-note input click to be handled")
	}
	if !m.noteInput.Focused() {
		t.Fatalf("expected note input to be focused")
	}
	if !m.input.IsChatFocused() {
		t.Fatalf("expected input controller focus to switch to chat")
	}
}

func TestMouseOverComposeControlsUsesComputedControlsRow(t *testing.T) {
	m := NewModel(nil)
	m.mode = uiModeCompose
	m.newSession = &newSessionTarget{provider: "codex"}
	m.resize(120, 40)

	row := m.composeControlsRow()
	if !m.mouseOverComposeControls(row) {
		t.Fatalf("expected compose controls hitbox at computed row")
	}
	if m.mouseOverComposeControls(row - 1) {
		t.Fatalf("did not expect compose controls hitbox above computed row")
	}
	m.mode = uiModeNormal
	if m.mouseOverComposeControls(row) {
		t.Fatalf("did not expect compose controls hitbox outside compose mode")
	}
}

func TestMouseReducerComposeControlsClickUsesComputedRow(t *testing.T) {
	m := NewModel(nil)
	m.mode = uiModeCompose
	m.newSession = &newSessionTarget{provider: "codex"}
	m.resize(120, 40)

	if line := m.composeControlsLine(); line == "" {
		t.Fatalf("expected compose controls line")
	}
	spans := m.composeControlSpans()
	if len(spans) == 0 {
		t.Fatalf("expected compose control spans")
	}
	layout := m.resolveMouseLayout()
	y := m.composeControlsRow()
	x := layout.rightStart + spans[0].start

	handled := m.reduceComposeControlsLeftPressMouse(tea.MouseClickMsg{Button: tea.MouseLeft, X: x, Y: y}, layout)
	if !handled {
		t.Fatalf("expected compose controls click to be handled at computed row")
	}
}

func TestMouseReducerWheelDownKeepsFollowEnabled(t *testing.T) {
	m := NewModel(nil)
	m.resize(120, 40)
	m.follow = true
	layout := m.resolveMouseLayout()

	handled := m.reduceMouseWheel(tea.MouseClickMsg{Button: tea.MouseWheelDown, X: layout.rightStart, Y: 2}, layout, 1)
	if !handled {
		t.Fatalf("expected wheel event to be handled")
	}
	if !m.follow {
		t.Fatalf("expected follow to stay enabled when wheel-scrolling down")
	}
	if m.status == "follow: paused" {
		t.Fatalf("expected wheel down to avoid pausing follow")
	}
}

func TestMouseReducerWheelDownToBottomResumesFollow(t *testing.T) {
	m := NewModel(nil)
	m.resize(120, 40)
	seedFollowContent(&m, 220)
	layout := m.resolveMouseLayout()

	if !m.reduceMouseWheel(tea.MouseClickMsg{Button: tea.MouseWheelUp, X: layout.rightStart, Y: 2}, layout, -1) {
		t.Fatalf("expected wheel up to be handled")
	}
	if m.follow {
		t.Fatalf("expected follow paused after wheel up")
	}

	if !m.reduceMouseWheel(tea.MouseClickMsg{Button: tea.MouseWheelDown, X: layout.rightStart, Y: 2}, layout, 1) {
		t.Fatalf("expected wheel down to be handled")
	}
	if !m.follow {
		t.Fatalf("expected follow to resume once wheel scroll reaches bottom")
	}
}

func TestMouseReducerWheelDownWhilePausedAtBottomResumesFollow(t *testing.T) {
	m := NewModel(nil)
	m.resize(120, 40)
	seedFollowContent(&m, 220)
	m.pauseFollow(false)
	m.viewport.GotoBottom()
	layout := m.resolveMouseLayout()

	if !m.reduceMouseWheel(tea.MouseClickMsg{Button: tea.MouseWheelDown, X: layout.rightStart, Y: 2}, layout, 1) {
		t.Fatalf("expected wheel down to be handled")
	}
	if !m.follow {
		t.Fatalf("expected follow to resume when wheel-scrolling down at bottom while paused")
	}
	if m.status != "follow: on" {
		t.Fatalf("unexpected status %q", m.status)
	}
}

func TestMouseReducerSidebarWheelScrollKeepsSessionSelection(t *testing.T) {
	m := NewModel(nil)
	m.appState.SidebarCollapsed = false
	m.resize(120, 16)
	m.appState.ActiveWorkspaceGroupIDs = []string{"ungrouped"}
	m.workspaces = []*types.Workspace{{ID: "ws1", Name: "Workspace", RepoPath: "/tmp/ws1"}}
	m.worktrees = map[string][]*types.Worktree{}
	m.sessions = make([]*types.Session, 0, 30)
	m.sessionMeta = make(map[string]*types.SessionMeta, 30)
	for i := 1; i <= 30; i++ {
		id := "s" + strconv.Itoa(i)
		m.sessions = append(m.sessions, &types.Session{ID: id, Status: types.SessionStatusRunning})
		m.sessionMeta[id] = &types.SessionMeta{SessionID: id, WorkspaceID: "ws1"}
	}
	m.applySidebarItems()
	if !m.sidebar.SelectBySessionID("s1") {
		t.Fatalf("expected session selection")
	}
	layout := m.resolveMouseLayout()
	header := m.sidebar.headerRows()
	beforeTop := m.sidebar.ItemAtRow(header)
	if beforeTop == nil {
		t.Fatalf("expected visible sidebar row")
	}
	selectedBefore := m.selectedSessionID()

	handled := m.reduceMouseWheel(tea.MouseClickMsg{Button: tea.MouseWheelDown, X: 1, Y: header + 1}, layout, 1)
	if !handled {
		t.Fatalf("expected sidebar wheel to be handled")
	}
	if got := m.selectedSessionID(); got != selectedBefore {
		t.Fatalf("expected sidebar wheel to preserve selected session, got %q want %q", got, selectedBefore)
	}
	afterTop := m.sidebar.ItemAtRow(header)
	if afterTop == nil {
		t.Fatalf("expected visible sidebar row after wheel scroll")
	}
	if afterTop.key() == beforeTop.key() {
		t.Fatalf("expected sidebar wheel to move viewport")
	}
}

func TestMouseReducerSidebarScrollbarClickKeepsSessionSelection(t *testing.T) {
	m := NewModel(nil)
	m.appState.SidebarCollapsed = false
	m.resize(120, 16)
	m.appState.ActiveWorkspaceGroupIDs = []string{"ungrouped"}
	m.workspaces = []*types.Workspace{{ID: "ws1", Name: "Workspace", RepoPath: "/tmp/ws1"}}
	m.worktrees = map[string][]*types.Worktree{}
	m.sessions = make([]*types.Session, 0, 30)
	m.sessionMeta = make(map[string]*types.SessionMeta, 30)
	for i := 1; i <= 30; i++ {
		id := "s" + strconv.Itoa(i)
		m.sessions = append(m.sessions, &types.Session{ID: id, Status: types.SessionStatusRunning})
		m.sessionMeta[id] = &types.SessionMeta{SessionID: id, WorkspaceID: "ws1"}
	}
	m.applySidebarItems()
	if !m.sidebar.SelectBySessionID("s1") {
		t.Fatalf("expected session selection")
	}
	layout := m.resolveMouseLayout()
	header := m.sidebar.headerRows()
	beforeTop := m.sidebar.ItemAtRow(header)
	if beforeTop == nil {
		t.Fatalf("expected visible sidebar row")
	}
	selectedBefore := m.selectedSessionID()

	y := header + max(1, m.sidebar.list.Height()-header-1)
	handled := m.reduceSidebarScrollbarLeftPressMouse(tea.MouseClickMsg{Button: tea.MouseLeft, X: layout.barStart, Y: y}, layout)
	if !handled {
		t.Fatalf("expected sidebar scrollbar click to be handled")
	}
	if got := m.selectedSessionID(); got != selectedBefore {
		t.Fatalf("expected scrollbar click to preserve selected session, got %q want %q", got, selectedBefore)
	}
	afterTop := m.sidebar.ItemAtRow(header)
	if afterTop == nil {
		t.Fatalf("expected visible sidebar row after scrollbar click")
	}
	if afterTop.key() == beforeTop.key() {
		t.Fatalf("expected scrollbar click to move viewport")
	}
}

func TestMouseReducerPickProviderLeftClickSelects(t *testing.T) {
	m := NewModel(nil)
	m.resize(120, 40)
	m.newSession = &newSessionTarget{}
	m.enterProviderPick()
	layout := m.resolveMouseLayout()

	handled := m.reduceModePickersLeftPressMouse(tea.MouseClickMsg{Button: tea.MouseLeft, X: layout.rightStart, Y: 2}, layout)
	if !handled {
		t.Fatalf("expected provider click to be handled")
	}
	if m.mode != uiModeCompose {
		t.Fatalf("expected compose mode after provider pick, got %v", m.mode)
	}
	if m.newSession == nil || m.newSession.provider == "" {
		t.Fatalf("expected provider to be selected, got %#v", m.newSession)
	}
}

func TestMouseReducerPickProviderLeftReleaseIgnored(t *testing.T) {
	m := NewModel(nil)
	m.resize(120, 40)
	m.newSession = &newSessionTarget{}
	m.enterProviderPick()
	layout := m.resolveMouseLayout()

	handled := m.reduceModePickersLeftPressMouse(tea.MouseReleaseMsg{Button: tea.MouseLeft, X: layout.rightStart, Y: 2}, layout)
	if handled {
		t.Fatalf("expected provider release to be ignored")
	}
	if m.mode != uiModePickProvider {
		t.Fatalf("expected mode to remain pick-provider, got %v", m.mode)
	}
}

func TestMouseReducerGuidedWorkflowLauncherTemplateClickSelects(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	m.resize(120, 40)
	enterGuidedWorkflowForTest(&m, guidedWorkflowLaunchContext{
		workspaceID: "ws1",
		worktreeID:  "wt1",
		sessionID:   "s1",
	})
	if m.mode != uiModeGuidedWorkflow {
		t.Fatalf("expected guided workflow mode, got %v", m.mode)
	}
	if m.guidedWorkflow == nil || strings.TrimSpace(m.guidedWorkflow.templateID) != "custom_triage" {
		t.Fatalf("expected default template selection before click, got template=%q", strings.TrimSpace(m.guidedWorkflow.templateID))
	}

	layout := m.resolveMouseLayout()
	pickerLayout, ok := m.guidedWorkflow.LauncherTemplatePickerLayout()
	if !ok {
		t.Fatalf("expected launcher picker layout metadata")
	}
	start := m.guidedWorkflowLauncherPickerStartRow(pickerLayout)
	if start < 0 {
		t.Fatalf("expected launcher picker start row")
	}
	row := start - m.viewport.YOffset() + 1 + 2 // query row + second option
	handled := m.reduceGuidedWorkflowLauncherLeftPressMouse(tea.MouseClickMsg{Button: tea.MouseLeft, X: layout.rightStart, Y: row}, layout)
	if !handled {
		t.Fatalf("expected guided workflow picker click to be handled")
	}
	if m.guidedWorkflow == nil || strings.TrimSpace(m.guidedWorkflow.templateID) != guidedworkflows.TemplateIDSolidPhaseDelivery {
		t.Fatalf("expected click to select second template, got template=%q", strings.TrimSpace(m.guidedWorkflow.templateID))
	}
}

func TestMouseReducerGuidedWorkflowLauncherIgnoresClicksOutsideTemplatePicker(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	m.resize(120, 40)
	enterGuidedWorkflowForTest(&m, guidedWorkflowLaunchContext{
		workspaceID: "ws1",
		worktreeID:  "wt1",
		sessionID:   "s1",
	})
	if m.mode != uiModeGuidedWorkflow {
		t.Fatalf("expected guided workflow mode, got %v", m.mode)
	}
	layout := m.resolveMouseLayout()
	pickerLayout, ok := m.guidedWorkflow.LauncherTemplatePickerLayout()
	if !ok {
		t.Fatalf("expected launcher picker layout metadata")
	}
	start := m.guidedWorkflowLauncherPickerStartRow(pickerLayout)
	if start < 0 {
		t.Fatalf("expected launcher picker start row")
	}
	queryRow := start - m.viewport.YOffset() + 1
	rowOutside := queryRow - 1
	if rowOutside < 1 {
		rowOutside = queryRow + pickerLayout.height
	}
	handled := m.reduceGuidedWorkflowLauncherLeftPressMouse(tea.MouseClickMsg{Button: tea.MouseLeft, X: layout.rightStart, Y: rowOutside}, layout)
	if handled {
		t.Fatalf("expected click outside picker block to be ignored")
	}
	if m.guidedWorkflow == nil || strings.TrimSpace(m.guidedWorkflow.templateID) != "custom_triage" {
		t.Fatalf("expected selection to remain unchanged, got template=%q", strings.TrimSpace(m.guidedWorkflow.templateID))
	}
}

func TestGuidedWorkflowLauncherPickerStartRowFallbackStripsANSI(t *testing.T) {
	var nilModel *Model
	if got := nilModel.guidedWorkflowLauncherPickerStartRow(guidedWorkflowLauncherTemplatePickerLayout{queryLine: "/"}); got != -1 {
		t.Fatalf("expected nil model to return -1, got %d", got)
	}

	m := NewModel(nil)
	m.renderedPlain = nil
	m.renderedText = "\x1b[38;5;45mTemplate Picker\x1b[0m\n\x1b[38;5;118m/\x1b[0m\n option"
	start := m.guidedWorkflowLauncherPickerStartRow(guidedWorkflowLauncherTemplatePickerLayout{
		queryLine: "/",
		height:    3,
	})
	if start != 1 {
		t.Fatalf("expected query line start row 1 from ANSI-rendered text, got %d", start)
	}

	if got := m.guidedWorkflowLauncherPickerStartRow(guidedWorkflowLauncherTemplatePickerLayout{}); got != -1 {
		t.Fatalf("expected empty query line layout to return -1, got %d", got)
	}
}

func TestMouseReducerGuidedWorkflowTurnLinkClickOpensLinkedSession(t *testing.T) {
	now := time.Date(2026, 2, 18, 10, 0, 0, 0, time.UTC)
	run := newWorkflowRunFixture("gwf-link-click", guidedworkflows.WorkflowRunStatusRunning, now)
	run.CurrentPhaseIndex = 0
	run.CurrentStepIndex = 1
	run.Phases[0].Steps[1].Execution = &guidedworkflows.StepExecutionRef{
		SessionID: "s1",
		TurnID:    "turn-99",
	}
	run.Phases[0].Steps[1].ExecutionState = guidedworkflows.StepExecutionStateLinked

	m := newPhase0ModelWithSession("codex")
	m.resize(120, 40)
	enterGuidedWorkflowForTest(&m, guidedWorkflowLaunchContext{
		workspaceID: "ws1",
		worktreeID:  "wt1",
	})
	updated, _ := m.Update(workflowRunSnapshotMsg{run: run})
	m = asModel(t, updated)
	if m.mode != uiModeGuidedWorkflow {
		t.Fatalf("expected guided workflow mode before link click")
	}
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = asModel(t, updated)

	x, y := findVisualTokenInBody(t, &m, "user turn turn-99")
	handled := m.handleMouse(tea.MouseClickMsg{Button: tea.MouseLeft, X: x, Y: y})
	if !handled {
		t.Fatalf("expected guided workflow turn link click to be handled")
	}
	if m.mode != uiModeNormal {
		t.Fatalf("expected guided workflow to close after link click, got mode=%v", m.mode)
	}
	if m.pendingMouseCmd == nil {
		t.Fatalf("expected linked session load command to be queued")
	}
}

func TestMouseReducerGuidedWorkflowTurnLinkClickResolvesProviderSessionID(t *testing.T) {
	now := time.Date(2026, 2, 18, 10, 5, 0, 0, time.UTC)
	run := newWorkflowRunFixture("gwf-link-click-provider", guidedworkflows.WorkflowRunStatusRunning, now)
	run.CurrentPhaseIndex = 0
	run.CurrentStepIndex = 1
	run.Phases[0].Steps[1].Execution = &guidedworkflows.StepExecutionRef{
		SessionID: "provider-session-1",
		TurnID:    "turn-100",
	}
	run.Phases[0].Steps[1].ExecutionState = guidedworkflows.StepExecutionStateLinked

	m := newPhase0ModelWithSession("codex")
	m.sessionMeta["s1"] = &types.SessionMeta{
		SessionID:         "s1",
		WorkspaceID:       "ws1",
		ProviderSessionID: "provider-session-1",
	}
	m.resize(120, 40)
	enterGuidedWorkflowForTest(&m, guidedWorkflowLaunchContext{
		workspaceID: "ws1",
		worktreeID:  "wt1",
	})
	updated, _ := m.Update(workflowRunSnapshotMsg{run: run})
	m = asModel(t, updated)
	if m.mode != uiModeGuidedWorkflow {
		t.Fatalf("expected guided workflow mode before link click")
	}
	updated, _ = m.Update(tea.KeyPressMsg{Code: 'j', Text: "j"})
	m = asModel(t, updated)

	x, y := findVisualTokenInBody(t, &m, "user turn turn-100")
	handled := m.handleMouse(tea.MouseClickMsg{Button: tea.MouseLeft, X: x, Y: y})
	if !handled {
		t.Fatalf("expected guided workflow turn link click to be handled")
	}
	if selected := m.selectedSessionID(); selected != "s1" {
		t.Fatalf("expected provider session id to resolve to s1, got %q", selected)
	}
	if m.pendingWorkflowTurnFocus == nil {
		t.Fatalf("expected pending workflow turn focus")
	}
	if m.pendingWorkflowTurnFocus.sessionID != "s1" || m.pendingWorkflowTurnFocus.turnID != "turn-100" {
		t.Fatalf("unexpected pending workflow turn focus: %#v", m.pendingWorkflowTurnFocus)
	}
	if m.pendingMouseCmd == nil {
		t.Fatalf("expected linked session load command to be queued")
	}
}

func TestMouseReducerTranscriptCopyClickHandlesPerMessage(t *testing.T) {
	m := NewModel(nil)
	m.resize(120, 40)
	m.applyBlocks([]ChatBlock{
		{Role: ChatRoleAgent, Text: "   ", Status: ChatStatusSending},
	})
	if len(m.contentBlockSpans) != 1 {
		t.Fatalf("expected rendered span metadata, got %d", len(m.contentBlockSpans))
	}
	span := m.contentBlockSpans[0]
	if span.CopyLine < 0 || span.CopyStart < 0 {
		t.Fatalf("expected copy metadata, got %#v", span)
	}
	layout := m.resolveMouseLayout()
	y := span.CopyLine - m.viewport.YOffset() + 1
	x := layout.rightStart + span.CopyStart

	handled := m.reduceTranscriptCopyLeftPressMouse(tea.MouseClickMsg{Button: tea.MouseLeft, X: x, Y: y}, layout)
	if !handled {
		t.Fatalf("expected copy click to be handled")
	}
	if m.messageSelectActive {
		t.Fatalf("expected copy click to avoid message selection")
	}
	if m.status != "nothing to copy" {
		t.Fatalf("unexpected status %q", m.status)
	}
}

func TestMouseReducerTranscriptCopyClickHandlesInNotesMode(t *testing.T) {
	m := NewModel(nil)
	m.resize(120, 40)
	m.mode = uiModeNotes
	m.applyBlocks([]ChatBlock{
		{Role: ChatRoleSystem, Text: "   ", Status: ChatStatusSending},
	})
	if len(m.contentBlockSpans) != 1 {
		t.Fatalf("expected rendered span metadata, got %d", len(m.contentBlockSpans))
	}
	span := m.contentBlockSpans[0]
	if span.CopyLine < 0 || span.CopyStart < 0 {
		t.Fatalf("expected copy metadata, got %#v", span)
	}
	layout := m.resolveMouseLayout()
	y := span.CopyLine - m.viewport.YOffset() + 1
	x := layout.rightStart + span.CopyStart

	handled := m.reduceTranscriptCopyLeftPressMouse(tea.MouseClickMsg{Button: tea.MouseLeft, X: x, Y: y}, layout)
	if !handled {
		t.Fatalf("expected copy click to be handled in notes mode")
	}
	if m.status != "nothing to copy" {
		t.Fatalf("unexpected status %q", m.status)
	}
}

func TestMouseReducerTranscriptPinClickQueuesPinCommand(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	if m.sidebar == nil || !m.sidebar.SelectBySessionID("s1") {
		t.Fatalf("expected session selection")
	}
	m.resize(120, 40)
	m.applyBlocks([]ChatBlock{
		{ID: "m1", Role: ChatRoleAgent, Text: "hello"},
	})
	if len(m.contentBlockSpans) != 1 {
		t.Fatalf("expected rendered span metadata, got %d", len(m.contentBlockSpans))
	}
	span := m.contentBlockSpans[0]
	if span.PinLine < 0 || span.PinStart < 0 {
		t.Fatalf("expected pin metadata, got %#v", span)
	}
	layout := m.resolveMouseLayout()
	y := span.PinLine - m.viewport.YOffset() + 1
	x := layout.rightStart + span.PinStart

	handled := m.reduceTranscriptPinLeftPressMouse(tea.MouseClickMsg{Button: tea.MouseLeft, X: x, Y: y}, layout)
	if !handled {
		t.Fatalf("expected pin click to be handled")
	}
	if m.pendingMouseCmd == nil {
		t.Fatalf("expected pin click to queue command")
	}
	if m.status != "pinning message" {
		t.Fatalf("unexpected status %q", m.status)
	}
}

func TestMouseReducerTranscriptMoveClickOpensMovePickerInNotesMode(t *testing.T) {
	m := NewModel(nil)
	m.resize(120, 40)
	m.mode = uiModeNotes
	m.worktrees["ws1"] = []*types.Worktree{{ID: "wt1", WorkspaceID: "ws1", Name: "feature"}}
	m.sessions = []*types.Session{
		{ID: "s2", Status: types.SessionStatusRunning},
	}
	m.sessionMeta["s2"] = &types.SessionMeta{SessionID: "s2", WorkspaceID: "ws1"}
	m.notes = []*types.Note{
		{
			ID:          "n1",
			Scope:       types.NoteScopeWorktree,
			WorkspaceID: "ws1",
			WorktreeID:  "wt1",
			Title:       "Move me",
		},
	}
	m.applyBlocks([]ChatBlock{
		{ID: "n1", Role: ChatRoleWorktreeNote, Text: "remember this"},
	})
	if len(m.contentBlockSpans) != 1 {
		t.Fatalf("expected rendered span metadata, got %d", len(m.contentBlockSpans))
	}
	span := m.contentBlockSpans[0]
	if span.MoveLine < 0 || span.MoveStart < 0 {
		t.Fatalf("expected move metadata, got %#v", span)
	}
	layout := m.resolveMouseLayout()
	y := span.MoveLine - m.viewport.YOffset() + 1
	x := layout.rightStart + span.MoveStart

	handled := m.reduceTranscriptMoveLeftPressMouse(tea.MouseClickMsg{Button: tea.MouseLeft, X: x, Y: y}, layout)
	if !handled {
		t.Fatalf("expected move click to be handled in notes mode")
	}
	if m.mode != uiModePickNoteMoveTarget {
		t.Fatalf("expected note move target picker mode, got %v", m.mode)
	}
	if m.noteMoveNoteID != "n1" {
		t.Fatalf("expected move state to track note n1, got %q", m.noteMoveNoteID)
	}
}

func TestMouseReducerTranscriptDeleteClickOpensConfirmInNotesMode(t *testing.T) {
	m := NewModel(nil)
	m.resize(120, 40)
	m.mode = uiModeNotes
	m.notes = []*types.Note{{ID: "n1", Title: "Important follow-up"}}
	m.applyBlocks([]ChatBlock{
		{ID: "n1", Role: ChatRoleSessionNote, Text: "remember this"},
	})
	if len(m.contentBlockSpans) != 1 {
		t.Fatalf("expected rendered span metadata, got %d", len(m.contentBlockSpans))
	}
	span := m.contentBlockSpans[0]
	if span.DeleteLine < 0 || span.DeleteStart < 0 {
		t.Fatalf("expected delete metadata, got %#v", span)
	}
	layout := m.resolveMouseLayout()
	y := span.DeleteLine - m.viewport.YOffset() + 1
	x := layout.rightStart + span.DeleteStart

	handled := m.reduceTranscriptDeleteLeftPressMouse(tea.MouseClickMsg{Button: tea.MouseLeft, X: x, Y: y}, layout)
	if !handled {
		t.Fatalf("expected delete click to be handled in notes mode")
	}
	if m.pendingConfirm.kind != confirmDeleteNote || m.pendingConfirm.noteID != "n1" {
		t.Fatalf("unexpected pending confirm: %#v", m.pendingConfirm)
	}
	if m.confirm == nil || !m.confirm.IsOpen() {
		t.Fatalf("expected confirm dialog to open")
	}
}

func TestMouseReducerTranscriptNotesFilterClickTogglesWorkspace(t *testing.T) {
	m := NewModel(nil)
	m.resize(120, 40)
	m.mode = uiModeNotes
	m.setNotesRootScope(noteScopeTarget{
		Scope:       types.NoteScopeSession,
		WorkspaceID: "ws1",
		WorktreeID:  "wt1",
		SessionID:   "s1",
	})
	m.renderNotesViewsFromState()
	lines := m.currentLines()
	targetLine := -1
	targetCol := -1
	for i, line := range lines {
		idx := strings.Index(line, "Workspace")
		if idx >= 0 {
			targetLine = i
			targetCol = idx
			break
		}
	}
	if targetLine < 0 {
		t.Fatalf("expected filter token in notes header")
	}
	layout := m.resolveMouseLayout()
	y := targetLine - m.viewport.YOffset() + 1
	x := layout.rightStart + targetCol
	handled := m.reduceTranscriptNotesFilterLeftPressMouse(tea.MouseClickMsg{
		Button: tea.MouseLeft,
		X:      x,
		Y:      y,
	}, layout)
	if !handled {
		t.Fatalf("expected notes filter click to be handled")
	}
	if m.notesFilters.ShowWorkspace {
		t.Fatalf("expected workspace filter to toggle off")
	}
}

func TestMouseReducerApprovalButtonClickSendsApprovalForRequestZero(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	if m.sidebar == nil || !m.sidebar.SelectBySessionID("s1") {
		t.Fatalf("expected session selection")
	}
	m.resize(120, 40)
	m.applyBlocks([]ChatBlock{
		{Role: ChatRoleApproval, Text: "Approval required", RequestID: 0, SessionID: "s1"},
	})
	if len(m.contentBlockSpans) != 1 {
		t.Fatalf("expected rendered span metadata, got %d", len(m.contentBlockSpans))
	}
	span := m.contentBlockSpans[0]
	if span.ApproveLine < 0 || span.ApproveStart < 0 {
		t.Fatalf("expected approve metadata, got %#v", span)
	}
	layout := m.resolveMouseLayout()
	y := span.ApproveLine - m.viewport.YOffset() + 1
	x := layout.rightStart + span.ApproveStart

	handled := m.reduceTranscriptApprovalButtonLeftPressMouse(tea.MouseClickMsg{Button: tea.MouseLeft, X: x, Y: y}, layout)
	if !handled {
		t.Fatalf("expected approval click to be handled")
	}
	if m.pendingMouseCmd == nil {
		t.Fatalf("expected approval click to queue approval command")
	}
	if m.status != "sending approval" {
		t.Fatalf("unexpected status %q", m.status)
	}
	if m.messageSelectActive {
		t.Fatalf("expected approval click to avoid message selection")
	}
}

func TestMouseReducerApprovalButtonUsesBlockSessionWhenSidebarIsWorkspace(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	m.resize(120, 40)
	if m.sidebar == nil || !m.sidebar.SelectBySessionID("s1") {
		t.Fatalf("expected session selection")
	}
	items := m.sidebar.Items()
	workspaceIndex := -1
	for i, item := range items {
		entry, ok := item.(*sidebarItem)
		if !ok || entry == nil || entry.kind != sidebarWorkspace {
			continue
		}
		workspaceIndex = i
		break
	}
	if workspaceIndex < 0 {
		t.Fatalf("expected workspace item in sidebar")
	}
	m.sidebar.Select(workspaceIndex)
	if m.selectedSessionID() != "" {
		t.Fatalf("expected no selected session after workspace selection")
	}
	m.applyBlocks([]ChatBlock{
		{Role: ChatRoleApproval, Text: "Approval required", RequestID: 3, SessionID: "s1"},
	})
	if len(m.contentBlockSpans) != 1 {
		t.Fatalf("expected rendered span metadata, got %d", len(m.contentBlockSpans))
	}
	span := m.contentBlockSpans[0]
	layout := m.resolveMouseLayout()
	y := span.ApproveLine - m.viewport.YOffset() + 1
	x := layout.rightStart + span.ApproveStart

	handled := m.reduceTranscriptApprovalButtonLeftPressMouse(tea.MouseClickMsg{Button: tea.MouseLeft, X: x, Y: y}, layout)
	if !handled {
		t.Fatalf("expected approval click to be handled")
	}
	if m.pendingMouseCmd == nil {
		t.Fatalf("expected approval click to queue approval command")
	}
	if m.status != "sending approval" {
		t.Fatalf("unexpected status %q", m.status)
	}
}

func TestMouseReducerTranscriptClickSelectsMessage(t *testing.T) {
	m := NewModel(nil)
	m.resize(120, 40)
	m.applyBlocks([]ChatBlock{
		{Role: ChatRoleAgent, Text: "first"},
		{Role: ChatRoleAgent, Text: "second"},
	})
	if len(m.contentBlockSpans) < 2 {
		t.Fatalf("expected span metadata for messages")
	}
	first := m.contentBlockSpans[0]
	layout := m.resolveMouseLayout()
	y := first.CopyLine - m.viewport.YOffset() + 2

	handled := m.reduceTranscriptSelectLeftPressMouse(tea.MouseClickMsg{Button: tea.MouseLeft, X: layout.rightStart, Y: y}, layout)
	if !handled {
		t.Fatalf("expected transcript click to be handled")
	}
	if !m.messageSelectActive {
		t.Fatalf("expected message selection to be active")
	}
	if m.messageSelectIndex != first.BlockIndex {
		t.Fatalf("expected selected index %d, got %d", first.BlockIndex, m.messageSelectIndex)
	}

	second := m.contentBlockSpans[1]
	y = second.CopyLine - m.viewport.YOffset() + 2
	handled = m.reduceTranscriptSelectLeftPressMouse(tea.MouseClickMsg{Button: tea.MouseLeft, X: layout.rightStart, Y: y}, layout)
	if !handled {
		t.Fatalf("expected second transcript click to be handled")
	}
	if m.messageSelectIndex != second.BlockIndex {
		t.Fatalf("expected selected index %d, got %d", second.BlockIndex, m.messageSelectIndex)
	}
}

func TestMouseReducerReasoningBodyClickSelectsWithoutToggle(t *testing.T) {
	m := NewModel(nil)
	m.resize(120, 40)
	m.applyBlocks([]ChatBlock{
		{Role: ChatRoleReasoning, Text: "line one\nline two", Collapsed: true},
	})
	if len(m.contentBlockSpans) != 1 {
		t.Fatalf("expected rendered span metadata, got %d", len(m.contentBlockSpans))
	}
	span := m.contentBlockSpans[0]
	layout := m.resolveMouseLayout()
	bodyY := span.CopyLine - m.viewport.YOffset() + 2

	handled := m.reduceTranscriptSelectLeftPressMouse(tea.MouseClickMsg{Button: tea.MouseLeft, X: layout.rightStart, Y: bodyY}, layout)
	if !handled {
		t.Fatalf("expected body click to be handled")
	}
	if !m.messageSelectActive || m.messageSelectIndex != span.BlockIndex {
		t.Fatalf("expected reasoning message to be selected")
	}
	if !m.contentBlocks[span.BlockIndex].Collapsed {
		t.Fatalf("expected reasoning block to remain collapsed")
	}
}

func TestMouseReducerReasoningButtonClickToggles(t *testing.T) {
	m := NewModel(nil)
	m.resize(120, 40)
	m.applyBlocks([]ChatBlock{
		{Role: ChatRoleReasoning, Text: "line one\nline two", Collapsed: true},
	})
	if len(m.contentBlockSpans) != 1 {
		t.Fatalf("expected rendered span metadata, got %d", len(m.contentBlockSpans))
	}
	span := m.contentBlockSpans[0]
	if span.ToggleLine < 0 || span.ToggleStart < 0 {
		t.Fatalf("expected reasoning toggle metadata, got %#v", span)
	}
	before := xansi.Strip(m.renderedText)
	if !strings.Contains(before, "[Expand]") {
		t.Fatalf("expected expand button before click, got %q", before)
	}
	layout := m.resolveMouseLayout()
	y := span.ToggleLine - m.viewport.YOffset() + 1
	x := layout.rightStart + span.ToggleStart

	handled := m.reduceTranscriptReasoningButtonLeftPressMouse(tea.MouseClickMsg{Button: tea.MouseLeft, X: x, Y: y}, layout)
	if !handled {
		t.Fatalf("expected reasoning button click to be handled")
	}
	if m.contentBlocks[span.BlockIndex].Collapsed {
		t.Fatalf("expected reasoning block to expand")
	}
	if m.messageSelectActive {
		t.Fatalf("expected reasoning button click to avoid message selection")
	}
	after := xansi.Strip(m.renderedText)
	if !strings.Contains(after, "[Collapse]") {
		t.Fatalf("expected collapse button after click, got %q", after)
	}
}

func TestMouseReducerMetaLineClickDoesNotSelectMessage(t *testing.T) {
	m := NewModel(nil)
	m.resize(120, 40)
	m.applyBlocks([]ChatBlock{
		{Role: ChatRoleAgent, Text: "first"},
	})
	if len(m.contentBlockSpans) != 1 {
		t.Fatalf("expected span metadata for message")
	}
	span := m.contentBlockSpans[0]
	layout := m.resolveMouseLayout()
	y := span.CopyLine - m.viewport.YOffset() + 1
	x := layout.rightStart

	handled := m.reduceTranscriptSelectLeftPressMouse(tea.MouseClickMsg{Button: tea.MouseLeft, X: x, Y: y}, layout)
	if handled {
		t.Fatalf("expected meta line click to avoid selection")
	}
	if m.messageSelectActive {
		t.Fatalf("expected message selection to remain inactive")
	}
}

func TestMouseReducerUserStatusLineClickDoesNotSelectMessage(t *testing.T) {
	m := NewModel(nil)
	m.resize(120, 40)
	m.applyBlocks([]ChatBlock{
		{Role: ChatRoleUser, Text: "sending", Status: ChatStatusSending},
	})
	if len(m.contentBlockSpans) != 1 {
		t.Fatalf("expected span metadata for message")
	}
	span := m.contentBlockSpans[0]
	layout := m.resolveMouseLayout()
	y := span.EndLine - m.viewport.YOffset() + 1
	x := layout.rightStart

	handled := m.reduceTranscriptSelectLeftPressMouse(tea.MouseClickMsg{Button: tea.MouseLeft, X: x, Y: y}, layout)
	if handled {
		t.Fatalf("expected status line click to avoid selection")
	}
	if m.messageSelectActive {
		t.Fatalf("expected message selection to remain inactive")
	}
}

func TestMouseReducerGlobalStatusBarClickCopiesStatus(t *testing.T) {
	origWriteAll := clipboardWriteAll
	origWriteOSC52 := clipboardWriteOSC52
	defer func() {
		clipboardWriteAll = origWriteAll
		clipboardWriteOSC52 = origWriteOSC52
	}()

	copied := ""
	clipboardWriteAll = func(text string) error {
		copied = text
		return nil
	}
	clipboardWriteOSC52 = func(string) error {
		t.Fatalf("expected system clipboard copy to succeed without OSC52 fallback")
		return nil
	}

	m := NewModel(nil)
	m.resize(120, 40)
	m.hotkeys = nil
	m.setStatusMessage("follow: paused")
	m.applyBlocks([]ChatBlock{
		{Role: ChatRoleAgent, Text: "first"},
	})
	start, _, ok := m.statusLineStatusHitbox()
	if !ok {
		t.Fatalf("expected global status hitbox")
	}

	handled := m.handleMouse(tea.MouseClickMsg{
		Button: tea.MouseLeft,
		X:      start,
		Y:      m.height,
	})
	if !handled {
		t.Fatalf("expected global status click to be handled")
	}
	flushPendingMouseCmd(t, &m)
	if copied != "follow: paused" {
		t.Fatalf("expected status copy payload %q, got %q", "follow: paused", copied)
	}
	if m.status != "status copied" {
		t.Fatalf("expected copy success status, got %q", m.status)
	}
	if m.messageSelectActive {
		t.Fatalf("expected status line copy click to avoid message selection")
	}
}

func TestMouseReducerGlobalStatusBarHelpClickDoesNotCopy(t *testing.T) {
	origWriteAll := clipboardWriteAll
	origWriteOSC52 := clipboardWriteOSC52
	defer func() {
		clipboardWriteAll = origWriteAll
		clipboardWriteOSC52 = origWriteOSC52
	}()

	copied := false
	clipboardWriteAll = func(string) error {
		copied = true
		return nil
	}
	clipboardWriteOSC52 = func(string) error {
		t.Fatalf("expected no clipboard fallback for non-copy click")
		return nil
	}

	m := NewModel(nil)
	m.resize(120, 40)
	m.hotkeys = nil
	m.setStatusMessage("ready")
	start, _, ok := m.statusLineStatusHitbox()
	if !ok {
		t.Fatalf("expected global status hitbox")
	}
	if start <= 0 {
		t.Fatalf("expected non-empty help segment before status, got status start %d", start)
	}

	handled := m.handleMouse(tea.MouseClickMsg{
		Button: tea.MouseLeft,
		X:      start - 1,
		Y:      m.height,
	})
	if handled {
		t.Fatalf("expected help-segment click to remain unhandled")
	}
	if copied {
		t.Fatalf("expected help-segment click not to invoke clipboard copy")
	}
	if m.status != "ready" {
		t.Fatalf("expected status to remain unchanged, got %q", m.status)
	}
}

func TestMouseReducerGlobalStatusHitboxVisibleWithDefaultHotkeys(t *testing.T) {
	m := NewModel(nil)
	m.resize(40, 20)
	m.setStatusMessage("ready")

	start, end, ok := m.statusLineStatusHitbox()
	if !ok {
		t.Fatalf("expected status hitbox with default hotkeys")
	}
	if end != m.width-1 {
		t.Fatalf("expected status to render at right edge %d, got %d", m.width-1, end)
	}
	if start < 0 || start > end {
		t.Fatalf("invalid status bounds [%d,%d]", start, end)
	}
}

func TestMouseReducerGlobalStatusBarClickCopiesStatusOnZeroBasedBottomRow(t *testing.T) {
	origWriteAll := clipboardWriteAll
	origWriteOSC52 := clipboardWriteOSC52
	defer func() {
		clipboardWriteAll = origWriteAll
		clipboardWriteOSC52 = origWriteOSC52
	}()

	copied := ""
	clipboardWriteAll = func(text string) error {
		copied = text
		return nil
	}
	clipboardWriteOSC52 = func(string) error {
		t.Fatalf("expected system clipboard copy to succeed without OSC52 fallback")
		return nil
	}

	m := NewModel(nil)
	m.resize(120, 40)
	m.hotkeys = nil
	m.setStatusMessage("ready")
	start, _, ok := m.statusLineStatusHitbox()
	if !ok {
		t.Fatalf("expected global status hitbox")
	}

	handled := m.handleMouse(tea.MouseClickMsg{
		Button: tea.MouseLeft,
		X:      start,
		Y:      m.height - 1,
	})
	if !handled {
		t.Fatalf("expected global status click on zero-based bottom row to be handled")
	}
	flushPendingMouseCmd(t, &m)
	if copied != "ready" {
		t.Fatalf("expected status copy payload %q, got %q", "ready", copied)
	}
}

func TestMouseReducerGlobalStatusBarClickCopiesStatusWithOneBasedX(t *testing.T) {
	origWriteAll := clipboardWriteAll
	origWriteOSC52 := clipboardWriteOSC52
	defer func() {
		clipboardWriteAll = origWriteAll
		clipboardWriteOSC52 = origWriteOSC52
	}()

	copied := ""
	clipboardWriteAll = func(text string) error {
		copied = text
		return nil
	}
	clipboardWriteOSC52 = func(string) error {
		t.Fatalf("expected system clipboard copy to succeed without OSC52 fallback")
		return nil
	}

	m := NewModel(nil)
	m.resize(120, 40)
	m.hotkeys = nil
	m.setStatusMessage("ready")
	start, _, ok := m.statusLineStatusHitbox()
	if !ok {
		t.Fatalf("expected global status hitbox")
	}

	handled := m.handleMouse(tea.MouseClickMsg{
		Button: tea.MouseLeft,
		X:      start + 1,
		Y:      m.height - 1,
	})
	if !handled {
		t.Fatalf("expected global status click with one-based X to be handled")
	}
	flushPendingMouseCmd(t, &m)
	if copied != "ready" {
		t.Fatalf("expected status copy payload %q, got %q", "ready", copied)
	}
}

func flushPendingMouseCmd(t *testing.T, m *Model) {
	t.Helper()
	if m == nil || m.pendingMouseCmd == nil {
		return
	}
	cmd := m.pendingMouseCmd
	m.pendingMouseCmd = nil
	msg := cmd()
	if msg == nil {
		return
	}
	handled, next := m.reduceStateMessages(msg)
	if !handled {
		t.Fatalf("expected pending mouse command message to be handled, got %T", msg)
	}
	if next != nil {
		_ = next()
	}
}

func TestMouseReducerComposeOptionPickerClickSelectsOption(t *testing.T) {
	m := NewModel(nil)
	m.resize(120, 40)
	m.mode = uiModeCompose
	m.newSession = &newSessionTarget{provider: "codex"}
	layout := m.resolveMouseLayout()
	if !m.openComposeOptionPicker(composeOptionModel) {
		t.Fatalf("expected model option picker to open")
	}
	popup, row := m.composeOptionPopupView()
	if popup == "" {
		t.Fatalf("expected popup content")
	}

	handled := m.reduceComposeOptionPickerLeftPressMouse(
		tea.MouseClickMsg{Button: tea.MouseLeft, X: layout.rightStart, Y: row + 2},
		layout,
	)
	if !handled {
		t.Fatalf("expected picker click to be handled")
	}
	if m.composeOptionPickerOpen() {
		t.Fatalf("expected picker to close after selection")
	}
	if m.newSession == nil || m.newSession.runtimeOptions == nil {
		t.Fatalf("expected runtime options to be updated")
	}
	if got := m.newSession.runtimeOptions.Model; got != "gpt-5.2-codex" {
		t.Fatalf("expected model gpt-5.2-codex, got %q", got)
	}
}

func TestMouseReducerComposeOptionPickerClickBelowPopupSelectsLastOption(t *testing.T) {
	m := NewModel(nil)
	m.resize(120, 40)
	m.mode = uiModeCompose
	m.newSession = &newSessionTarget{provider: "codex"}
	layout := m.resolveMouseLayout()
	if !m.openComposeOptionPicker(composeOptionModel) {
		t.Fatalf("expected model option picker to open")
	}
	popup, row := m.composeOptionPopupView()
	if popup == "" {
		t.Fatalf("expected popup content")
	}
	height := len(strings.Split(popup, "\n"))
	y := row + height

	handled := m.reduceComposeOptionPickerLeftPressMouse(
		tea.MouseClickMsg{Button: tea.MouseLeft, X: layout.rightStart, Y: y},
		layout,
	)
	if !handled {
		t.Fatalf("expected bottom-edge picker click to be handled")
	}
	if m.composeOptionPickerOpen() {
		t.Fatalf("expected picker to close after selection")
	}
	if m.newSession == nil || m.newSession.runtimeOptions == nil {
		t.Fatalf("expected runtime options to be updated")
	}
	if got := m.newSession.runtimeOptions.Model; got != "gpt-5.1-codex-max" {
		t.Fatalf("expected bottom-edge click to select last model, got %q", got)
	}
}

func TestMouseReducerMenuGroupToggleIgnoresLeftRelease(t *testing.T) {
	m := NewModel(nil)
	m.resize(120, 40)
	m.groups = []*types.WorkspaceGroup{{ID: "g1", Name: "Group 1"}}
	if m.menu == nil {
		t.Fatalf("expected menu controller")
	}
	m.menu.SetGroups(m.groups)
	m.menu.OpenBar()
	m.menu.OpenDropdown()

	handled := m.handleMouse(tea.MouseClickMsg{
		Button: tea.MouseLeft,
		X:      1,
		Y:      4, // dropdown row 4 => second group checkbox row (g1)
	})
	if !handled {
		t.Fatalf("expected group click to be handled")
	}
	if !selectedGroupIDsContain(m.menu.SelectedGroupIDs(), "g1") {
		t.Fatalf("expected click to select group g1, got %#v", m.menu.SelectedGroupIDs())
	}

	handled = m.handleMouse(tea.MouseReleaseMsg{
		Button: tea.MouseLeft,
		X:      1,
		Y:      4,
	})
	if handled {
		t.Fatalf("expected left release to be ignored for menu toggles")
	}
	if !selectedGroupIDsContain(m.menu.SelectedGroupIDs(), "g1") {
		t.Fatalf("expected release not to revert selection, got %#v", m.menu.SelectedGroupIDs())
	}
}

func TestMouseReducerVisualDeleteClickInNotesModeWithSidebarExpandedDoesNotCopy(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	if m.appState.SidebarCollapsed {
		t.Fatalf("expected expanded sidebar state")
	}
	m.resize(120, 40)
	m.mode = uiModeNotes
	m.notes = []*types.Note{{ID: "n1", Scope: types.NoteScopeSession, SessionID: "s1", Title: "Important follow-up"}}
	m.applyBlocks([]ChatBlock{
		{ID: "n1", Role: ChatRoleSessionNote, Text: "remember this"},
	})

	x, y := findVisualTokenInBody(t, &m, "[Delete]")
	handled := m.handleMouse(tea.MouseClickMsg{Button: tea.MouseLeft, X: x, Y: y})
	if !handled {
		t.Fatalf("expected visual delete click to be handled")
	}
	if m.status == "message copied" {
		t.Fatalf("expected delete action, got copy status")
	}
	if m.pendingConfirm.kind != confirmDeleteNote || m.pendingConfirm.noteID != "n1" {
		t.Fatalf("unexpected pending confirm after visual delete click: %#v", m.pendingConfirm)
	}
}

func TestMouseReducerVisualDeleteClickInNotesPanelWithSidebarExpanded(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	if m.appState.SidebarCollapsed {
		t.Fatalf("expected expanded sidebar state")
	}
	m.notesPanelOpen = true
	m.resize(180, 40)
	m.notes = []*types.Note{{ID: "n1", Scope: types.NoteScopeSession, SessionID: "s1", Title: "delete me"}}
	m.notesPanelBlocks = []ChatBlock{{ID: "n1", Role: ChatRoleSessionNote, Text: "delete me"}}
	m.renderNotesPanel()

	layout := m.resolveMouseLayout()
	if !layout.panelVisible {
		t.Fatalf("expected notes panel to be visible")
	}
	x, y := findVisualTokenInBody(t, &m, "[Delete]")
	handled := m.handleMouse(tea.MouseClickMsg{Button: tea.MouseLeft, X: x, Y: y})
	if !handled {
		t.Fatalf("expected panel visual delete click to be handled")
	}
	if m.status == "note copied" {
		t.Fatalf("expected delete action, got copy status")
	}
	if m.pendingConfirm.kind != confirmDeleteNote || m.pendingConfirm.noteID != "n1" {
		t.Fatalf("unexpected pending confirm after panel visual delete click: %#v", m.pendingConfirm)
	}
}

func findVisualTokenInBody(t *testing.T, m *Model, token string) (int, int) {
	t.Helper()
	body := m.renderBodyWithSidebar(m.renderRightPaneView())
	lines := strings.Split(xansi.Strip(body), "\n")
	for y, line := range lines {
		if x := strings.Index(line, token); x >= 0 {
			return xansi.StringWidth(line[:x]) + 1, y
		}
	}
	t.Fatalf("could not find token %q in rendered body:\n%s", token, body)
	return 0, 0
}

func selectedGroupIDsContain(ids []string, want string) bool {
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
}
