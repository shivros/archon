package app

import (
	"context"
	"testing"

	"control/internal/guidedworkflows"
	"control/internal/types"

	tea "charm.land/bubbletea/v2"
)

func TestSearchReducerEscExitsSearchMode(t *testing.T) {
	m := NewModel(nil)
	m.enterSearch()

	handled, cmd := m.reduceSearchModeKey(tea.KeyPressMsg{Code: tea.KeyEsc})
	if !handled {
		t.Fatalf("expected search reducer to handle esc")
	}
	if cmd != nil {
		t.Fatalf("expected no command on esc")
	}
	if m.mode != uiModeNormal {
		t.Fatalf("expected mode to return to normal, got %v", m.mode)
	}
	if m.status != "search canceled" {
		t.Fatalf("expected cancel status, got %q", m.status)
	}
}

func TestApprovePendingRequestUserInputEntersResponseMode(t *testing.T) {
	m := NewModel(nil)
	req := &ApprovalRequest{
		RequestID: 7,
		SessionID: "s1",
		Method:    approvalMethodRequestUserInput,
		Summary:   "user input",
		Detail:    "Provide a reason",
	}
	m.pendingApproval = cloneApprovalRequest(req)
	m.sessionApprovals["s1"] = []*ApprovalRequest{cloneApprovalRequest(req)}

	cmd := m.approvePending("accept")
	if cmd != nil {
		t.Fatalf("expected no command until approval response is submitted")
	}
	if m.mode != uiModeApprovalResponse {
		t.Fatalf("expected approval response mode, got %v", m.mode)
	}
	if got := m.approvalResponseSessionID; got != "s1" {
		t.Fatalf("expected response session s1, got %q", got)
	}
	if got := m.approvalResponseRequestID; got != 7 {
		t.Fatalf("expected response request id 7, got %d", got)
	}
}

func TestApprovalResponseReducerEnterSubmits(t *testing.T) {
	m := NewModel(nil)
	req := &ApprovalRequest{
		RequestID: 7,
		SessionID: "s1",
		Method:    approvalMethodRequestUserInput,
		Summary:   "user input",
		Detail:    "Provide a reason",
	}
	m.enterApprovalResponse("s1", req)
	if m.approvalInput == nil {
		t.Fatalf("expected approval input")
	}
	m.approvalInput.SetValue("because tests")

	handled, cmd := m.reduceApprovalResponseMode(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !handled {
		t.Fatalf("expected approval response reducer to handle enter")
	}
	if cmd == nil {
		t.Fatalf("expected approval submit command")
	}
	if m.mode != uiModeNormal {
		t.Fatalf("expected mode to return to normal, got %v", m.mode)
	}
	if m.status != "sending approval" {
		t.Fatalf("expected sending status, got %q", m.status)
	}
}

func TestApprovalResponseReducerEscCancels(t *testing.T) {
	m := NewModel(nil)
	req := &ApprovalRequest{
		RequestID: 7,
		SessionID: "s1",
		Method:    approvalMethodRequestUserInput,
		Summary:   "user input",
		Detail:    "Provide a reason",
	}
	m.enterApprovalResponse("s1", req)

	handled, cmd := m.reduceApprovalResponseMode(tea.KeyPressMsg{Code: tea.KeyEsc})
	if !handled {
		t.Fatalf("expected approval response reducer to handle esc")
	}
	if cmd != nil {
		t.Fatalf("expected no command for cancel")
	}
	if m.mode != uiModeNormal {
		t.Fatalf("expected mode to return to normal, got %v", m.mode)
	}
	if m.status != "approval input canceled" {
		t.Fatalf("expected cancel status, got %q", m.status)
	}
}

func TestApprovalResponseReducerClearCommandClearsInput(t *testing.T) {
	m := NewModel(nil)
	req := &ApprovalRequest{
		RequestID: 7,
		SessionID: "s1",
		Method:    approvalMethodRequestUserInput,
		Summary:   "user input",
		Detail:    "Provide a reason",
	}
	m.enterApprovalResponse("s1", req)
	if m.approvalInput == nil {
		t.Fatalf("expected approval input")
	}
	m.approvalInput.SetValue("because tests")

	handled, cmd := m.reduceApprovalResponseMode(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if !handled {
		t.Fatalf("expected approval response reducer to handle clear command")
	}
	if cmd != nil {
		t.Fatalf("expected no command for clear action")
	}
	if got := m.approvalInput.Value(); got != "" {
		t.Fatalf("expected approval input to clear, got %q", got)
	}
	if m.mode != uiModeApprovalResponse {
		t.Fatalf("expected mode to remain approval response, got %v", m.mode)
	}
}

func TestApprovalResponseReducerShiftEnterInsertsNewline(t *testing.T) {
	m := NewModel(nil)
	req := &ApprovalRequest{
		RequestID: 7,
		SessionID: "s1",
		Method:    approvalMethodRequestUserInput,
		Summary:   "user input",
		Detail:    "Provide a reason",
	}
	m.enterApprovalResponse("s1", req)
	if m.approvalInput == nil {
		t.Fatalf("expected approval input")
	}
	m.approvalInput.SetValue("line one")

	handled, cmd := m.reduceApprovalResponseMode(tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModShift})
	if !handled {
		t.Fatalf("expected approval response reducer to handle shift+enter")
	}
	_ = cmd
	if got := m.approvalInput.Value(); got != "line one\n" {
		t.Fatalf("expected shift+enter to insert newline, got %q", got)
	}
	if m.mode != uiModeApprovalResponse {
		t.Fatalf("expected mode to remain approval response, got %v", m.mode)
	}
}

func TestSearchReducerSupportsRemappedSubmitCommand(t *testing.T) {
	m := NewModel(nil)
	m.applyKeybindings(NewKeybindings(map[string]string{
		KeyCommandInputSubmit: "f6",
	}))
	m.enterSearch()
	if m.searchInput == nil {
		t.Fatalf("expected search input")
	}
	m.searchInput.SetValue("hello")

	handled, cmd := m.reduceSearchModeKey(tea.KeyPressMsg{Code: tea.KeyF6})
	if !handled {
		t.Fatalf("expected search reducer to handle remapped submit")
	}
	if cmd != nil {
		t.Fatalf("expected no async command for search submit")
	}
	if m.mode != uiModeNormal {
		t.Fatalf("expected search submit to exit search mode, got %v", m.mode)
	}
	if m.searchQuery != "hello" {
		t.Fatalf("expected search query to be applied, got %q", m.searchQuery)
	}
}

func TestSearchReducerClearCommandClearsInput(t *testing.T) {
	m := NewModel(nil)
	m.enterSearch()
	if m.searchInput == nil {
		t.Fatalf("expected search input")
	}
	m.searchInput.SetValue("hello")

	handled, cmd := m.reduceSearchModeKey(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if !handled {
		t.Fatalf("expected search reducer to handle clear command")
	}
	if cmd != nil {
		t.Fatalf("expected no command for clear action")
	}
	if got := m.searchInput.Value(); got != "" {
		t.Fatalf("expected search input to clear, got %q", got)
	}
	if m.mode != uiModeSearch {
		t.Fatalf("expected mode to remain search, got %v", m.mode)
	}
}

func TestUpdateSearchModePasteUpdatesInput(t *testing.T) {
	m := NewModel(nil)
	m.enterSearch()
	if m.searchInput == nil {
		t.Fatalf("expected search input")
	}
	m.searchInput.Focus()

	_, _ = m.Update(tea.PasteMsg{Content: "hello search"})
	if got := m.searchInput.Value(); got != "hello search" {
		t.Fatalf("expected search paste to update input, got %q", got)
	}
}

func TestComposeReducerEnterEmptyShowsValidation(t *testing.T) {
	m := NewModel(nil)
	m.enterCompose("")
	if m.chatInput != nil {
		m.chatInput.SetValue("   ")
	}

	handled, cmd := m.reduceComposeInputKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !handled {
		t.Fatalf("expected compose reducer to handle enter")
	}
	if cmd != nil {
		t.Fatalf("expected no command for empty message")
	}
	if m.status != "message is required" {
		t.Fatalf("expected validation status, got %q", m.status)
	}
}

func TestComposeReducerArrowKeysMoveInputCursorLine(t *testing.T) {
	m := NewModel(nil)
	m.resize(120, 40)
	m.enterCompose("s1")
	if m.chatInput == nil {
		t.Fatalf("expected chat input")
	}
	m.recordComposeHistory("s1", "history one")
	m.chatInput.SetValue("top\nbottom")

	handled, _ := m.reduceComposeInputKey(tea.KeyPressMsg{Code: tea.KeyUp})
	if !handled {
		t.Fatalf("expected compose reducer to handle up")
	}
	handled, _ = m.reduceComposeInputKey(tea.KeyPressMsg{Code: 'x', Text: "x"})
	if !handled {
		t.Fatalf("expected compose reducer to handle text input")
	}
	if got := m.chatInput.Value(); got != "topx\nbottom" {
		t.Fatalf("expected up to move cursor to previous line, got %q", got)
	}
}

func TestComposeReducerShiftEnterInsertsNewline(t *testing.T) {
	m := NewModel(nil)
	m.enterCompose("s1")
	if m.chatInput == nil {
		t.Fatalf("expected chat input")
	}
	m.chatInput.SetValue("hello")

	handled, cmd := m.reduceComposeInputKey(tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModShift})
	if !handled {
		t.Fatalf("expected compose reducer to handle shift+enter")
	}
	_ = cmd
	if got := m.chatInput.Value(); got != "hello\n" {
		t.Fatalf("expected shift+enter to insert newline, got %q", got)
	}
}

func TestComposeReducerCtrlEnterInsertsNewline(t *testing.T) {
	m := NewModel(nil)
	m.enterCompose("s1")
	if m.chatInput == nil {
		t.Fatalf("expected chat input")
	}
	m.chatInput.SetValue("hello")

	handled, cmd := m.reduceComposeInputKey(tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModCtrl})
	if !handled {
		t.Fatalf("expected compose reducer to handle ctrl+enter")
	}
	_ = cmd
	if got := m.chatInput.Value(); got != "hello\n" {
		t.Fatalf("expected ctrl+enter to insert newline, got %q", got)
	}
}

func TestComposeReducerCtrlJInsertsNewline(t *testing.T) {
	m := NewModel(nil)
	m.enterCompose("s1")
	if m.chatInput == nil {
		t.Fatalf("expected chat input")
	}
	m.chatInput.SetValue("hello")

	handled, cmd := m.reduceComposeInputKey(tea.KeyPressMsg{Code: 'j', Mod: tea.ModCtrl})
	if !handled {
		t.Fatalf("expected compose reducer to handle ctrl+j")
	}
	_ = cmd
	if got := m.chatInput.Value(); got != "hello\n" {
		t.Fatalf("expected ctrl+j to insert newline, got %q", got)
	}
}

func TestComposeReducerCtrlArrowKeysNavigateHistory(t *testing.T) {
	m := NewModel(nil)
	m.enterCompose("s1")
	if m.chatInput == nil {
		t.Fatalf("expected chat input")
	}
	m.recordComposeHistory("s1", "first")
	m.recordComposeHistory("s1", "second")
	m.chatInput.SetValue("draft")

	handled, _ := m.reduceComposeInputKey(tea.KeyPressMsg{Code: tea.KeyUp, Mod: tea.ModCtrl})
	if !handled {
		t.Fatalf("expected compose reducer to handle ctrl+up history navigation")
	}
	if got := m.chatInput.Value(); got != "second" {
		t.Fatalf("expected ctrl+up to load latest history entry, got %q", got)
	}

	handled, _ = m.reduceComposeInputKey(tea.KeyPressMsg{Code: tea.KeyDown, Mod: tea.ModCtrl})
	if !handled {
		t.Fatalf("expected compose reducer to handle ctrl+down history navigation")
	}
	if got := m.chatInput.Value(); got != "" {
		t.Fatalf("expected ctrl+down at newest entry to restore draft, got %q", got)
	}
}

func TestComposeReducerNotesNewOverrideOpensAddNote(t *testing.T) {
	m := NewModel(nil)
	m.enterCompose("s1")
	m.applyKeybindings(NewKeybindings(map[string]string{
		KeyCommandNotesNew:   "ctrl+n",
		KeyCommandNewSession: "ctrl+s",
	}))

	handled, cmd := m.reduceComposeInputKey(tea.KeyPressMsg{Code: 'n', Mod: tea.ModCtrl})
	if !handled {
		t.Fatalf("expected notes-new override to be handled from compose input")
	}
	if cmd == nil {
		t.Fatalf("expected notes refresh command when entering add note")
	}
	if m.mode != uiModeAddNote {
		t.Fatalf("expected add note mode, got %v", m.mode)
	}
	if m.notesScope.SessionID != "s1" {
		t.Fatalf("expected notes scope to target compose session, got %#v", m.notesScope)
	}
}

func TestComposeReducerCtrlGCopiesSelectionIDs(t *testing.T) {
	clipboard := &recordingClipboardService{}
	m := NewModel(nil, WithClipboardService(clipboard))
	m.workspaces = []*types.Workspace{{ID: "ws1", Name: "Workspace", RepoPath: "/tmp/ws1"}}
	m.sessions = []*types.Session{{ID: "s1", Title: "Session One", Status: types.SessionStatusExited}}
	m.sessionMeta = map[string]*types.SessionMeta{
		"s1": {SessionID: "s1", WorkspaceID: "ws1"},
	}
	workflows := []*guidedworkflows.WorkflowRun{
		{ID: "gwf-1", WorkspaceID: "ws1", TemplateName: "SOLID", Status: guidedworkflows.WorkflowRunStatusRunning},
	}
	m.sidebar.Apply(m.workspaces, map[string][]*types.Worktree{}, m.sessions, workflows, m.sessionMeta, "", "", false)
	selectSidebarItemKind(t, &m, sidebarWorkspace)
	_, _ = m.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	selectSidebarItemKind(t, &m, sidebarWorkflow)
	_, _ = m.Update(tea.KeyPressMsg{Code: tea.KeySpace})

	m.enterCompose("s1")
	if m.chatInput == nil {
		t.Fatalf("expected chat input")
	}
	m.chatInput.Focus()

	handled, cmd := m.reduceComposeInputKey(tea.KeyPressMsg{Code: 'g', Mod: tea.ModCtrl})
	if !handled {
		t.Fatalf("expected compose reducer to handle ctrl+g")
	}
	if cmd == nil {
		t.Fatalf("expected copy command from ctrl+g in compose")
	}
	msg := cmd()
	result, ok := msg.(clipboardResultMsg)
	if !ok {
		t.Fatalf("expected clipboardResultMsg, got %T", msg)
	}
	if result.err != nil {
		t.Fatalf("expected successful clipboard copy, got %v", result.err)
	}
	const wantPayload = "/tmp/ws1\ngwf-1"
	if clipboard.text != wantPayload {
		t.Fatalf("unexpected clipboard payload\nwant:\n%s\n\ngot:\n%s", wantPayload, clipboard.text)
	}
	if result.success != "copied 2 id(s)" {
		t.Fatalf("expected success text %q, got %q", "copied 2 id(s)", result.success)
	}
}

func TestComposeReducerCtrlDTogglesDebugStreamsWithToast(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	if m.sidebar == nil || !m.sidebar.SelectBySessionID("s1") {
		t.Fatalf("expected selected session")
	}
	m.enterCompose("s1")
	m.resize(180, 40)
	if m.chatInput == nil {
		t.Fatalf("expected chat input")
	}
	m.chatInput.Focus()

	handled, cmd := m.reduceComposeInputKey(tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})
	if !handled {
		t.Fatalf("expected ctrl+d to be handled from compose input")
	}
	if cmd == nil {
		t.Fatalf("expected debug toggle command batch")
	}
	if !m.appState.DebugStreamsEnabled {
		t.Fatalf("expected debug streams to be enabled")
	}
	if !m.debugPanelVisible {
		t.Fatalf("expected debug panel to be visible")
	}
	if m.toastText != "debug streams enabled" {
		t.Fatalf("expected toggle toast, got %q", m.toastText)
	}
}

func TestUpdateNormalModeCtrlDTogglesDebugStreamsBeforeViewportScroll(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	if m.sidebar == nil || !m.sidebar.SelectBySessionID("s1") {
		t.Fatalf("expected selected session")
	}
	m.mode = uiModeNormal
	m.resize(180, 40)

	_, _ = m.Update(tea.KeyPressMsg{Code: 'd', Mod: tea.ModCtrl})

	if !m.appState.DebugStreamsEnabled {
		t.Fatalf("expected ctrl+d to toggle debug streams in normal mode")
	}
	if m.toastText != "debug streams enabled" {
		t.Fatalf("expected toggle toast, got %q", m.toastText)
	}
}

func TestUpdateNormalModeCtrlLTogglesContextPanelPreference(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	if m.sidebar == nil || !m.sidebar.SelectBySessionID("s1") {
		t.Fatalf("expected selected session")
	}
	m.mode = uiModeNormal
	m.resize(180, 40)

	_, _ = m.Update(tea.KeyPressMsg{Code: 'l', Mod: tea.ModCtrl})
	if !m.appState.ContextPanelHidden {
		t.Fatalf("expected ctrl+l to hide context panel")
	}
	if m.contextPanelVisible {
		t.Fatalf("expected context panel to be hidden")
	}

	_, _ = m.Update(tea.KeyPressMsg{Code: 'l', Mod: tea.ModCtrl})
	if m.appState.ContextPanelHidden {
		t.Fatalf("expected ctrl+l to show context panel")
	}
	if !m.contextPanelVisible {
		t.Fatalf("expected context panel to be visible")
	}
}

func TestUpdateNormalModeUsesRemappedContextPanelToggleKey(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	if m.sidebar == nil || !m.sidebar.SelectBySessionID("s1") {
		t.Fatalf("expected selected session")
	}
	m.mode = uiModeNormal
	m.applyKeybindings(NewKeybindings(map[string]string{
		KeyCommandToggleContextPanel: "f6",
	}))
	m.resize(180, 40)

	_, _ = m.Update(tea.KeyPressMsg{Code: tea.KeyF6})
	if !m.appState.ContextPanelHidden {
		t.Fatalf("expected remapped key to hide context panel")
	}
}

func TestUpdateComposeModePasteUpdatesInput(t *testing.T) {
	m := NewModel(nil)
	m.enterCompose("s1")
	if m.chatInput == nil {
		t.Fatalf("expected chat input")
	}
	m.chatInput.Focus()

	_, _ = m.Update(tea.PasteMsg{Content: "hello compose"})
	if got := m.chatInput.Value(); got != "hello compose" {
		t.Fatalf("expected compose paste to update input, got %q", got)
	}
}

func TestReduceClipboardAndSearchKeysUsesCtrlGForCopySelectionIDs(t *testing.T) {
	m := NewModel(nil)

	handled, cmd := m.reduceClipboardAndSearchKeys(tea.KeyPressMsg{Code: 'g', Mod: tea.ModCtrl})
	if !handled {
		t.Fatalf("expected ctrl+g to be handled for copy session id")
	}
	if cmd != nil {
		t.Fatalf("expected no async command for copy session id")
	}
	if m.status != "no workspace/workflow/session selected" {
		t.Fatalf("expected missing session status, got %q", m.status)
	}
}

func TestReduceClipboardAndSearchKeysCtrlYNotReservedForCopyByDefault(t *testing.T) {
	m := NewModel(nil)

	handled, _ := m.reduceClipboardAndSearchKeys(tea.KeyPressMsg{Code: 'y', Mod: tea.ModCtrl})
	if handled {
		t.Fatalf("expected ctrl+y not to trigger copy session id by default")
	}
}

func TestReduceClipboardAndSearchKeysCopySessionIDRemappable(t *testing.T) {
	m := NewModel(nil)
	m.applyKeybindings(NewKeybindings(map[string]string{
		KeyCommandCopySessionID: "ctrl+y",
	}))

	handled, _ := m.reduceClipboardAndSearchKeys(tea.KeyPressMsg{Code: 'y', Mod: tea.ModCtrl})
	if !handled {
		t.Fatalf("expected remapped copy session id command to be handled")
	}
}

func TestReduceClipboardAndSearchKeysCopySelectionIDsMultiSelect(t *testing.T) {
	clipboard := &recordingClipboardService{}
	m := NewModel(nil, WithClipboardService(clipboard))
	m.workspaces = []*types.Workspace{{ID: "ws1", Name: "Workspace", RepoPath: "/tmp/ws1"}}
	m.worktrees = map[string][]*types.Worktree{}
	m.sessions = []*types.Session{
		{ID: "s1", Title: "Session One", Status: types.SessionStatusExited},
		{ID: "s2", Title: "Session Two", Status: types.SessionStatusExited},
	}
	m.sessionMeta = map[string]*types.SessionMeta{
		"s1": {SessionID: "s1", WorkspaceID: "ws1"},
		"s2": {SessionID: "s2", WorkspaceID: "ws1"},
	}
	workflows := []*guidedworkflows.WorkflowRun{
		{ID: "gwf-1", WorkspaceID: "ws1", TemplateName: "SOLID", Status: guidedworkflows.WorkflowRunStatusRunning},
	}
	m.sidebar.Apply(m.workspaces, m.worktrees, m.sessions, workflows, m.sessionMeta, "", "", false)
	selectSidebarItemKind(t, &m, sidebarWorkspace)
	_, _ = m.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	selectSidebarItemKind(t, &m, sidebarWorkflow)
	_, _ = m.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	selectSidebarItemKind(t, &m, sidebarSession)
	_, _ = m.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	m.sidebar.CursorDown()
	_, _ = m.Update(tea.KeyPressMsg{Code: tea.KeySpace})

	handled, cmd := m.reduceClipboardAndSearchKeys(tea.KeyPressMsg{Code: 'g', Mod: tea.ModCtrl})
	if !handled {
		t.Fatalf("expected ctrl+g to be handled")
	}
	if cmd == nil {
		t.Fatalf("expected copy command")
	}
	msg := cmd()
	result, ok := msg.(clipboardResultMsg)
	if !ok {
		t.Fatalf("expected clipboardResultMsg, got %T", msg)
	}
	if result.err != nil {
		t.Fatalf("expected successful clipboard copy, got %v", result.err)
	}
	const wantPayload = "/tmp/ws1\ngwf-1\ns1\ns2"
	if clipboard.text != wantPayload {
		t.Fatalf("unexpected clipboard payload\nwant:\n%s\n\ngot:\n%s", wantPayload, clipboard.text)
	}
	if result.success != "copied 4 id(s)" {
		t.Fatalf("expected success text %q, got %q", "copied 4 id(s)", result.success)
	}
}

func TestReduceClipboardAndSearchKeysCopySelectionIDsPrefersSelectionSetOverFocus(t *testing.T) {
	clipboard := &recordingClipboardService{}
	m := NewModel(nil, WithClipboardService(clipboard))
	m.workspaces = []*types.Workspace{{ID: "ws1", Name: "Workspace", RepoPath: "/tmp/ws1"}}
	m.sessions = []*types.Session{{ID: "s1", Title: "Session One", Status: types.SessionStatusExited}}
	m.sessionMeta = map[string]*types.SessionMeta{
		"s1": {SessionID: "s1", WorkspaceID: "ws1"},
	}
	m.sidebar.Apply(m.workspaces, map[string][]*types.Worktree{}, m.sessions, nil, m.sessionMeta, "", "", false)
	if !m.sidebar.SelectBySessionID("s1") {
		t.Fatalf("expected to focus session s1")
	}
	_, _ = m.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	selectSidebarItemKind(t, &m, sidebarWorkspace)

	handled, cmd := m.reduceClipboardAndSearchKeys(tea.KeyPressMsg{Code: 'g', Mod: tea.ModCtrl})
	if !handled {
		t.Fatalf("expected ctrl+g to be handled")
	}
	if cmd == nil {
		t.Fatalf("expected copy command")
	}
	msg := cmd()
	result, ok := msg.(clipboardResultMsg)
	if !ok {
		t.Fatalf("expected clipboardResultMsg, got %T", msg)
	}
	if result.err != nil {
		t.Fatalf("expected successful clipboard copy, got %v", result.err)
	}
	if clipboard.text != "s1" {
		t.Fatalf("expected copy to use selected items only, got %q", clipboard.text)
	}
}

func TestReduceClipboardAndSearchKeysCopySelectionIDsWarnsWhenNothingCopyable(t *testing.T) {
	m := NewModel(nil)
	m.workspaces = []*types.Workspace{{ID: "ws1", Name: "Workspace", RepoPath: "/tmp/ws1"}}
	m.worktrees = map[string][]*types.Worktree{
		"ws1": {
			{ID: "wt1", WorkspaceID: "ws1", Name: "Worktree", Path: "/tmp/ws1/wt1"},
		},
	}
	m.sidebar.Apply(m.workspaces, m.worktrees, nil, nil, nil, "", "", false)
	selectSidebarItemKind(t, &m, sidebarWorktree)

	handled, cmd := m.reduceClipboardAndSearchKeys(tea.KeyPressMsg{Code: 'g', Mod: tea.ModCtrl})
	if !handled {
		t.Fatalf("expected ctrl+g to be handled")
	}
	if cmd != nil {
		t.Fatalf("expected no copy command for unsupported selection")
	}
	if m.status != "no workspace/workflow/session selected" {
		t.Fatalf("unexpected status %q", m.status)
	}
}

func TestReduceClipboardAndSearchKeysCopySelectionIDsIncludesSkippedInSuccess(t *testing.T) {
	clipboard := &recordingClipboardService{}
	m := NewModel(nil, WithClipboardService(clipboard))
	m.workspaces = []*types.Workspace{{ID: "ws1", Name: "Workspace", RepoPath: "/tmp/ws1"}}
	m.worktrees = map[string][]*types.Worktree{
		"ws1": {
			{ID: "wt1", WorkspaceID: "ws1", Name: "Worktree", Path: "/tmp/ws1/wt1"},
		},
	}
	m.sidebar.Apply(m.workspaces, m.worktrees, nil, nil, nil, "", "", false)
	selectSidebarItemKind(t, &m, sidebarWorkspace)
	_, _ = m.Update(tea.KeyPressMsg{Code: tea.KeySpace})
	selectSidebarItemKind(t, &m, sidebarWorktree)
	_, _ = m.Update(tea.KeyPressMsg{Code: tea.KeySpace})

	handled, cmd := m.reduceClipboardAndSearchKeys(tea.KeyPressMsg{Code: 'g', Mod: tea.ModCtrl})
	if !handled {
		t.Fatalf("expected ctrl+g to be handled")
	}
	if cmd == nil {
		t.Fatalf("expected copy command")
	}
	msg := cmd()
	result, ok := msg.(clipboardResultMsg)
	if !ok {
		t.Fatalf("expected clipboardResultMsg, got %T", msg)
	}
	if result.err != nil {
		t.Fatalf("expected successful clipboard copy, got %v", result.err)
	}
	if clipboard.text != "/tmp/ws1" {
		t.Fatalf("unexpected clipboard payload %q", clipboard.text)
	}
	if result.success != "copied 1 id(s), skipped 1" {
		t.Fatalf("expected success text %q, got %q", "copied 1 id(s), skipped 1", result.success)
	}
}

func TestMenuReducerEscClosesMenu(t *testing.T) {
	m := NewModel(nil)
	if m.menu == nil {
		t.Fatalf("expected menu controller")
	}
	m.menu.OpenBar()
	if !m.menu.IsActive() {
		t.Fatalf("expected menu to be active")
	}

	handled, _ := m.reduceMenuMode(tea.KeyPressMsg{Code: tea.KeyEsc})
	if !handled {
		t.Fatalf("expected menu reducer to handle esc")
	}
	if m.menu.IsActive() {
		t.Fatalf("expected menu to close on esc")
	}
}

func TestWorkspaceEditReducerRequiresWorkspaceSelection(t *testing.T) {
	m := NewModel(nil)
	m.mode = uiModePickWorkspaceRename
	if m.workspacePicker == nil {
		t.Fatalf("expected workspace picker")
	}
	m.workspacePicker.SetOptions(nil)

	handled, cmd := m.reduceWorkspaceEditModes(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !handled {
		t.Fatalf("expected workspace edit reducer to handle enter")
	}
	if cmd != nil {
		t.Fatalf("expected no command when selection is missing")
	}
	if m.status != "no workspace selected" {
		t.Fatalf("expected missing selection status, got %q", m.status)
	}
}

func TestWorkspaceEditReducerPickerTypingUpdatesQuery(t *testing.T) {
	m := NewModel(nil)
	m.mode = uiModePickWorkspaceRename
	if m.workspacePicker == nil {
		t.Fatalf("expected workspace picker")
	}
	m.workspacePicker.SetOptions([]selectOption{
		{id: "ws1", label: "Alpha"},
		{id: "ws2", label: "Beta"},
	})

	handled, _ := m.reduceWorkspaceEditModes(tea.KeyPressMsg{Code: 'j', Text: "j"})
	if !handled {
		t.Fatalf("expected picker typing to be handled")
	}
	if got := m.workspacePicker.Query(); got != "j" {
		t.Fatalf("expected workspace picker query to update, got %q", got)
	}
}

func TestWorkspaceEditReducerPickerArrowMovesSelection(t *testing.T) {
	m := NewModel(nil)
	m.mode = uiModePickWorkspaceRename
	if m.workspacePicker == nil {
		t.Fatalf("expected workspace picker")
	}
	m.workspacePicker.SetOptions([]selectOption{
		{id: "ws1", label: "Alpha"},
		{id: "ws2", label: "Beta"},
	})

	handled, _ := m.reduceWorkspaceEditModes(tea.KeyPressMsg{Code: tea.KeyDown})
	if !handled {
		t.Fatalf("expected picker down key to be handled")
	}
	if got := m.workspacePicker.SelectedID(); got != "ws2" {
		t.Fatalf("expected down key to move selection to ws2, got %q", got)
	}
}

func TestWorkspaceEditReducerRenameSessionEnterReturnsCommand(t *testing.T) {
	m := NewModel(nil)
	m.mode = uiModeRenameSession
	m.renameSessionID = "s1"
	if m.renameInput == nil {
		t.Fatalf("expected rename input")
	}
	m.renameInput.SetValue("Renamed Session")

	handled, cmd := m.reduceWorkspaceEditModes(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !handled {
		t.Fatalf("expected workspace edit reducer to handle enter")
	}
	if cmd == nil {
		t.Fatalf("expected update session command")
	}
	if m.mode != uiModeNormal {
		t.Fatalf("expected mode to return to normal, got %v", m.mode)
	}
	if m.status != "renaming session" {
		t.Fatalf("expected renaming status, got %q", m.status)
	}
	if m.renameSessionID != "" {
		t.Fatalf("expected rename session id to clear")
	}
}

func TestWorkspaceEditReducerRenameSessionPasteUpdatesInput(t *testing.T) {
	m := NewModel(nil)
	m.mode = uiModeRenameSession
	if m.renameInput == nil {
		t.Fatalf("expected rename input")
	}
	m.renameInput.Focus()

	handled, _ := m.reduceWorkspaceEditModes(tea.PasteMsg{Content: "Renamed Session"})
	if !handled {
		t.Fatalf("expected workspace edit reducer to handle paste")
	}
	if got := m.renameInput.Value(); got != "Renamed Session" {
		t.Fatalf("expected paste to update rename input, got %q", got)
	}
}

func TestWorkspaceEditReducerRenameSessionClearCommandClearsInput(t *testing.T) {
	m := NewModel(nil)
	m.mode = uiModeRenameSession
	if m.renameInput == nil {
		t.Fatalf("expected rename input")
	}
	m.renameInput.SetValue("Renamed Session")

	handled, cmd := m.reduceWorkspaceEditModes(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if !handled {
		t.Fatalf("expected workspace edit reducer to handle clear command")
	}
	if cmd != nil {
		t.Fatalf("expected no command for clear action")
	}
	if got := m.renameInput.Value(); got != "" {
		t.Fatalf("expected clear command to reset rename input, got %q", got)
	}
	if m.mode != uiModeRenameSession {
		t.Fatalf("expected clear command to keep rename mode, got %v", m.mode)
	}
}

func TestWorkspaceEditReducerRenameSessionSupportsRemappedClearCommand(t *testing.T) {
	m := NewModel(nil)
	m.applyKeybindings(NewKeybindings(map[string]string{
		KeyCommandInputClear: "f7",
	}))
	m.mode = uiModeRenameSession
	if m.renameInput == nil {
		t.Fatalf("expected rename input")
	}
	m.renameInput.SetValue("Renamed Session")

	handled, _ := m.reduceWorkspaceEditModes(tea.KeyPressMsg{Code: tea.KeyF7})
	if !handled {
		t.Fatalf("expected workspace edit reducer to handle remapped clear command")
	}
	if got := m.renameInput.Value(); got != "" {
		t.Fatalf("expected remapped clear command to reset rename input, got %q", got)
	}
}

func TestWorkspaceEditReducerRenameSessionSupportsRemappedSubmit(t *testing.T) {
	m := NewModel(nil)
	m.applyKeybindings(NewKeybindings(map[string]string{
		KeyCommandInputSubmit: "f6",
	}))
	m.mode = uiModeRenameSession
	m.renameSessionID = "s1"
	if m.renameInput == nil {
		t.Fatalf("expected rename input")
	}
	m.renameInput.SetValue("Renamed Session")

	handled, cmd := m.reduceWorkspaceEditModes(tea.KeyPressMsg{Code: tea.KeyF6})
	if !handled {
		t.Fatalf("expected workspace edit reducer to handle remapped submit")
	}
	if cmd == nil {
		t.Fatalf("expected update session command")
	}
	if m.mode != uiModeNormal {
		t.Fatalf("expected mode to return to normal, got %v", m.mode)
	}
}

func TestWorkspaceEditReducerRenameSessionRequiresSelection(t *testing.T) {
	m := NewModel(nil)
	m.mode = uiModeRenameSession
	if m.renameInput == nil {
		t.Fatalf("expected rename input")
	}
	m.renameInput.SetValue("Renamed Session")

	handled, cmd := m.reduceWorkspaceEditModes(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !handled {
		t.Fatalf("expected workspace edit reducer to handle enter")
	}
	if cmd != nil {
		t.Fatalf("expected no command without session id")
	}
	if m.status != "no session selected" {
		t.Fatalf("expected missing selection status, got %q", m.status)
	}
}

func TestWorkspaceEditReducerRenameWorkflowEnterReturnsCommand(t *testing.T) {
	m := NewModel(nil)
	m.mode = uiModeRenameWorkflow
	m.renameWorkflowRunID = "gwf-1"
	if m.renameInput == nil {
		t.Fatalf("expected rename input")
	}
	m.renameInput.SetValue("Renamed Workflow")

	handled, cmd := m.reduceWorkspaceEditModes(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !handled {
		t.Fatalf("expected workspace edit reducer to handle enter")
	}
	if cmd == nil {
		t.Fatalf("expected rename workflow command")
	}
	if m.mode != uiModeNormal {
		t.Fatalf("expected mode to return to normal, got %v", m.mode)
	}
	if m.status != "renaming workflow" {
		t.Fatalf("expected renaming status, got %q", m.status)
	}
	if m.renameWorkflowRunID != "" {
		t.Fatalf("expected rename workflow id to clear")
	}
}

func TestWorkspaceEditReducerRenameWorkflowRequiresSelection(t *testing.T) {
	m := NewModel(nil)
	m.mode = uiModeRenameWorkflow
	if m.renameInput == nil {
		t.Fatalf("expected rename input")
	}
	m.renameInput.SetValue("Renamed Workflow")

	handled, cmd := m.reduceWorkspaceEditModes(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !handled {
		t.Fatalf("expected workspace edit reducer to handle enter")
	}
	if cmd != nil {
		t.Fatalf("expected no command without workflow id")
	}
	if m.status != "no workflow selected" {
		t.Fatalf("expected missing selection status, got %q", m.status)
	}
}

func TestWorkspaceEditReducerRenameWorktreeEnterReturnsCommand(t *testing.T) {
	m := NewModel(nil)
	m.mode = uiModeRenameWorktree
	m.renameWorktreeWorkspaceID = "ws1"
	m.renameWorktreeID = "wt1"
	if m.renameInput == nil {
		t.Fatalf("expected rename input")
	}
	m.renameInput.SetValue("Renamed Worktree")

	handled, cmd := m.reduceWorkspaceEditModes(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !handled {
		t.Fatalf("expected workspace edit reducer to handle enter")
	}
	if cmd == nil {
		t.Fatalf("expected update worktree command")
	}
	if m.mode != uiModeNormal {
		t.Fatalf("expected mode to return to normal, got %v", m.mode)
	}
	if m.status != "renaming worktree" {
		t.Fatalf("expected renaming status, got %q", m.status)
	}
	if m.renameWorktreeWorkspaceID != "" || m.renameWorktreeID != "" {
		t.Fatalf("expected rename worktree ids to clear")
	}
}

func TestIsComposeInputCommandIncludesContextToggle(t *testing.T) {
	if !isComposeInputCommand(KeyCommandToggleContextPanel) {
		t.Fatalf("expected context panel toggle to be treated as compose input command")
	}
}

func TestAddWorkspaceReducerHandlesNilController(t *testing.T) {
	m := NewModel(nil)
	m.mode = uiModeAddWorkspace
	m.addWorkspace = nil

	handled, cmd := m.reduceAddWorkspaceMode(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !handled {
		t.Fatalf("expected add workspace reducer to handle mode")
	}
	if cmd != nil {
		t.Fatalf("expected no command with nil controller")
	}
}

func TestAddWorktreeReducerHandlesStreamMsg(t *testing.T) {
	m := NewModel(nil)
	m.mode = uiModeAddWorktree

	handled, cmd := m.reduceAddWorktreeMode(streamMsg{})
	if !handled {
		t.Fatalf("expected add worktree reducer to handle stream messages")
	}
	if cmd != nil {
		t.Fatalf("expected no command for stream message")
	}
}

func TestPickProviderReducerEscExits(t *testing.T) {
	m := NewModel(nil)
	m.newSession = &newSessionTarget{}
	m.enterProviderPick()

	handled, cmd := m.reducePickProviderMode(tea.KeyPressMsg{Code: tea.KeyEsc})
	if !handled {
		t.Fatalf("expected pick provider reducer to handle esc")
	}
	if cmd != nil {
		t.Fatalf("expected no command on esc")
	}
	if m.mode != uiModeNormal {
		t.Fatalf("expected normal mode after esc, got %v", m.mode)
	}
	if m.newSession != nil {
		t.Fatalf("expected new session target to clear")
	}
	if m.status != "new session canceled" {
		t.Fatalf("expected cancel status, got %q", m.status)
	}
}

func TestPickProviderReducerEnterSelectsProvider(t *testing.T) {
	m := NewModel(nil)
	m.newSession = &newSessionTarget{}
	m.enterProviderPick()

	handled, cmd := m.reducePickProviderMode(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !handled {
		t.Fatalf("expected pick provider reducer to handle enter")
	}
	if cmd == nil {
		t.Fatalf("expected provider options fetch command after selection")
	}
	if m.mode != uiModeCompose {
		t.Fatalf("expected compose mode after selection, got %v", m.mode)
	}
	if m.newSession == nil || m.newSession.provider == "" {
		t.Fatalf("expected provider to be selected, got %#v", m.newSession)
	}
}

func TestPickProviderReducerPasteAppendsToQuery(t *testing.T) {
	m := NewModel(nil)
	m.newSession = &newSessionTarget{}
	m.enterProviderPick()

	handled, _ := m.reducePickProviderMode(tea.PasteMsg{Content: "claude"})
	if !handled {
		t.Fatalf("expected pick provider reducer to handle paste")
	}
	if m.providerPicker == nil {
		t.Fatalf("expected provider picker to exist")
	}
	if m.providerPicker.Query() != "claude" {
		t.Fatalf("expected query to be 'claude', got %q", m.providerPicker.Query())
	}
}

func TestPickProviderReducerTypingAppendsToQuery(t *testing.T) {
	m := NewModel(nil)
	m.newSession = &newSessionTarget{}
	m.enterProviderPick()

	handled, _ := m.reducePickProviderMode(tea.KeyPressMsg{Code: 'h', Text: "h"})
	if !handled {
		t.Fatalf("expected pick provider reducer to handle key typing")
	}
	if m.providerPicker == nil {
		t.Fatalf("expected provider picker to exist")
	}
	if got := m.providerPicker.Query(); got != "h" {
		t.Fatalf("expected query to be 'h', got %q", got)
	}
}

func TestAssignGroupWorkspacesReducerSpaceTogglesSelection(t *testing.T) {
	m := NewModel(nil)
	m.mode = uiModeAssignGroupWorkspaces
	m.assignGroupID = "g1"
	m.workspaces = []*types.Workspace{
		{ID: "ws1", Name: "Workspace One"},
		{ID: "ws2", Name: "Workspace Two"},
	}
	if m.workspaceMulti == nil {
		t.Fatalf("expected workspace multi picker")
	}
	m.workspaceMulti.SetOptions([]multiSelectOption{
		{id: "ws1", label: "Workspace One"},
		{id: "ws2", label: "Workspace Two"},
	})

	handled, _ := m.reduceWorkspaceEditModes(tea.KeyPressMsg{Code: tea.KeySpace, Text: " "})
	if !handled {
		t.Fatalf("expected space to be handled in assign group picker")
	}
	if selected := m.workspaceMulti.SelectedIDs(); len(selected) != 1 || selected[0] != "ws1" {
		t.Fatalf("expected ws1 to be toggled selected, got %#v", selected)
	}
}

func TestPickProviderReducerClearCommandClearsQuery(t *testing.T) {
	m := NewModel(nil)
	m.newSession = &newSessionTarget{}
	m.enterProviderPick()
	if m.providerPicker == nil {
		t.Fatalf("expected provider picker to exist")
	}
	m.providerPicker.AppendQuery("claude")

	handled, cmd := m.reducePickProviderMode(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if !handled {
		t.Fatalf("expected pick provider reducer to handle clear command")
	}
	if cmd != nil {
		t.Fatalf("expected no command for clear query action")
	}
	if got := m.providerPicker.Query(); got != "" {
		t.Fatalf("expected query to be cleared, got %q", got)
	}
	if m.mode != uiModePickProvider {
		t.Fatalf("expected picker mode to remain active, got %v", m.mode)
	}
}

func TestPickProviderPasteViaUpdate(t *testing.T) {
	m := NewModel(nil)
	m.newSession = &newSessionTarget{}
	m.enterProviderPick()

	_, _ = m.Update(tea.PasteMsg{Content: "claude"})
	if m.providerPicker == nil {
		t.Fatalf("expected provider picker to exist")
	}
	if m.providerPicker.Query() != "claude" {
		t.Fatalf("expected query to be 'claude' via Update, got %q", m.providerPicker.Query())
	}
}

func TestPickProviderReducerPasteSanitizesContent(t *testing.T) {
	m := NewModel(nil)
	m.newSession = &newSessionTarget{}
	m.enterProviderPick()

	handled, _ := m.reducePickProviderMode(tea.PasteMsg{Content: " \x1b[31mcld\x1b[0m\n "})
	if !handled {
		t.Fatalf("expected pick provider reducer to handle paste")
	}
	if m.providerPicker == nil {
		t.Fatalf("expected provider picker to exist")
	}
	if got := m.providerPicker.Query(); got != "cld" {
		t.Fatalf("expected sanitized query 'cld', got %q", got)
	}
}

func TestReduceComposeInputKeyPasteRoutesToComposeOptionPicker(t *testing.T) {
	m := NewModel(nil)
	m.mode = uiModeCompose
	m.newSession = &newSessionTarget{provider: "codex"}
	if m.input == nil || m.chatInput == nil {
		t.Fatalf("expected compose input controllers")
	}
	if !m.openComposeOptionPicker(composeOptionModel) {
		t.Fatalf("expected compose option picker to open")
	}
	m.input.FocusChatInput()
	m.chatInput.Focus()
	m.chatInput.SetValue("existing")

	handled, _ := m.reduceComposeInputKey(tea.PasteMsg{Content: "\x1b[32m53c\x1b[0m\n"})
	if !handled {
		t.Fatalf("expected compose reducer to handle paste while option picker is open")
	}
	if got := m.composeOptionPickerQuery(); got != "53c" {
		t.Fatalf("expected option picker query to be updated, got %q", got)
	}
	if got := m.chatInput.Value(); got != "existing" {
		t.Fatalf("expected compose input to remain unchanged, got %q", got)
	}
}

func TestReduceComposeInputKeyPickerTypingWinsOverAlphanumericHotkeys(t *testing.T) {
	m := NewModel(nil)
	m.mode = uiModeCompose
	m.newSession = &newSessionTarget{provider: "codex"}
	if m.input == nil || m.chatInput == nil {
		t.Fatalf("expected compose input controllers")
	}
	if !m.openComposeOptionPicker(composeOptionModel) {
		t.Fatalf("expected compose option picker to open")
	}
	m.input.FocusChatInput()
	m.chatInput.Focus()

	handled, cmd := m.reduceComposeInputKey(tea.KeyPressMsg{Code: 'n', Text: "n"})
	if !handled {
		t.Fatalf("expected compose reducer to handle picker text input")
	}
	if cmd != nil {
		t.Fatalf("expected no command for picker text input")
	}
	if got := m.composeOptionPickerQuery(); got != "n" {
		t.Fatalf("expected picker query to capture typed key, got %q", got)
	}
	if m.mode != uiModeCompose {
		t.Fatalf("expected compose mode to remain active, got %v", m.mode)
	}
}

func TestReduceComposeInputKeyPickerAllowsTypingJAndK(t *testing.T) {
	m := NewModel(nil)
	m.mode = uiModeCompose
	m.newSession = &newSessionTarget{provider: "codex"}
	if !m.openComposeOptionPicker(composeOptionModel) {
		t.Fatalf("expected compose option picker to open")
	}
	m.input.FocusChatInput()
	m.chatInput.Focus()

	handled, _ := m.reduceComposeInputKey(tea.KeyPressMsg{Code: 'j', Text: "j"})
	if !handled {
		t.Fatalf("expected j key to be handled as picker input")
	}
	handled, _ = m.reduceComposeInputKey(tea.KeyPressMsg{Code: 'k', Text: "k"})
	if !handled {
		t.Fatalf("expected k key to be handled as picker input")
	}
	if got := m.composeOptionPickerQuery(); got != "jk" {
		t.Fatalf("expected picker query to include typed j/k, got %q", got)
	}
}

func TestReduceComposeInputKeySupportsRemappedClearCommand(t *testing.T) {
	m := NewModel(nil)
	m.applyKeybindings(NewKeybindings(map[string]string{
		KeyCommandInputClear: "f7",
	}))
	m.enterCompose("s1")
	if m.chatInput == nil {
		t.Fatalf("expected chat input")
	}
	m.chatInput.SetValue("hello compose")

	handled, cmd := m.reduceComposeInputKey(tea.KeyPressMsg{Code: tea.KeyF7})
	if !handled {
		t.Fatalf("expected compose reducer to handle remapped clear command")
	}
	if cmd != nil {
		t.Fatalf("expected no command for clear action")
	}
	if got := m.chatInput.Value(); got != "" {
		t.Fatalf("expected compose input to clear, got %q", got)
	}
	if m.status != "input cleared" {
		t.Fatalf("expected input cleared status, got %q", m.status)
	}
}

func TestReduceComposeInputKeySupportsRemappedInputNewline(t *testing.T) {
	m := NewModel(nil)
	m.applyKeybindings(NewKeybindings(map[string]string{
		KeyCommandInputNewline: "f7",
	}))
	m.enterCompose("s1")
	if m.chatInput == nil {
		t.Fatalf("expected chat input")
	}
	m.chatInput.SetValue("hello compose")

	handled, cmd := m.reduceComposeInputKey(tea.KeyPressMsg{Code: tea.KeyF7})
	if !handled {
		t.Fatalf("expected compose reducer to handle remapped input newline command")
	}
	_ = cmd
	if got := m.chatInput.Value(); got != "hello compose\n" {
		t.Fatalf("expected compose input newline from remap, got %q", got)
	}
}

func TestSearchReducerRemappedInputNewlineIgnoredForSingleLineInput(t *testing.T) {
	m := NewModel(nil)
	m.applyKeybindings(NewKeybindings(map[string]string{
		KeyCommandInputNewline: "f7",
	}))
	m.enterSearch()
	if m.searchInput == nil {
		t.Fatalf("expected search input")
	}
	m.searchInput.SetValue("hello")

	handled, cmd := m.reduceSearchModeKey(tea.KeyPressMsg{Code: tea.KeyF7})
	if !handled {
		t.Fatalf("expected search reducer to handle remapped newline command")
	}
	if cmd != nil {
		t.Fatalf("expected no command for single-line newline handling")
	}
	if got := m.searchInput.Value(); got != "hello" {
		t.Fatalf("expected single-line search input to ignore newline insertion, got %q", got)
	}
}

func TestComposeCtrlPassthroughExitsComposeForNonInputCommand(t *testing.T) {
	m := NewModel(nil)
	m.applyKeybindings(NewKeybindings(map[string]string{
		KeyCommandAddWorkspace: "ctrl+shift+y",
	}))
	m.enterCompose("s1")
	if m.mode != uiModeCompose {
		t.Fatalf("expected compose mode")
	}
	if m.chatInput == nil {
		t.Fatalf("expected chat input")
	}
	m.chatInput.SetValue("hello")

	handled, _ := m.reduceComposeInputKey(tea.KeyPressMsg{Code: 'y', Mod: tea.ModCtrl | tea.ModShift})
	if handled {
		t.Fatalf("expected compose reducer to release ctrl+shift+y for passthrough")
	}
	if m.mode == uiModeCompose {
		t.Fatalf("expected compose mode to be exited after passthrough")
	}
}

func TestComposeCtrlPassthroughKeepsComposeForInputCommand(t *testing.T) {
	m := NewModel(nil)
	m.enterCompose("s1")
	if m.mode != uiModeCompose {
		t.Fatalf("expected compose mode")
	}
	if m.chatInput == nil {
		t.Fatalf("expected chat input")
	}
	m.chatInput.SetValue("hello world")

	// ctrl+a is bound to ui.inputSelectAll — should NOT passthrough
	handled, _ := m.reduceComposeInputKey(tea.KeyPressMsg{Code: 'a', Mod: tea.ModCtrl})
	if !handled {
		t.Fatalf("expected compose reducer to handle ctrl+a as select-all")
	}
	if m.mode != uiModeCompose {
		t.Fatalf("expected compose mode to be preserved for input command")
	}
}

func TestIsComposeInputCommandTreatsLegacyAndCanonicalCopyAsInputCommands(t *testing.T) {
	if !isComposeInputCommand(KeyCommandCopySelectionIDs) {
		t.Fatalf("expected canonical copy-selection command to be compose input command")
	}
	if !isComposeInputCommand(KeyCommandCopySessionID) {
		t.Fatalf("expected legacy copy-session command to remain compose input command")
	}
}

func TestEditWorkspaceGroupsReducerEnterSubmitsGroups(t *testing.T) {
	m := NewModel(nil)
	m.groups = []*types.WorkspaceGroup{
		{ID: "g1", Name: "Alpha"},
		{ID: "g2", Name: "Beta"},
	}
	m.mode = uiModeEditWorkspaceGroups
	m.editWorkspaceID = "ws1"
	if m.groupPicker == nil {
		t.Fatalf("expected group picker from NewModel")
	}
	m.groupPicker.SetGroups(m.groups, map[string]bool{"g1": true})

	handled, cmd := m.reduceWorkspaceEditModes(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !handled {
		t.Fatalf("expected enter to be handled")
	}
	if cmd == nil {
		t.Fatalf("expected command from enter on edit workspace groups")
	}
	if m.mode != uiModeNormal {
		t.Fatalf("expected mode to return to normal, got %v", m.mode)
	}
	if m.status != "saving groups" {
		t.Fatalf("expected saving status, got %q", m.status)
	}
}

func TestEditWorkspaceGroupsReducerEnterRejectsEmptyWorkspaceID(t *testing.T) {
	m := NewModel(nil)
	m.groups = []*types.WorkspaceGroup{
		{ID: "g1", Name: "Alpha"},
	}
	m.mode = uiModeEditWorkspaceGroups
	m.editWorkspaceID = ""
	if m.groupPicker == nil {
		t.Fatalf("expected group picker from NewModel")
	}
	m.groupPicker.SetGroups(m.groups, nil)

	handled, cmd := m.reduceWorkspaceEditModes(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !handled {
		t.Fatalf("expected enter to be handled")
	}
	if cmd != nil {
		t.Fatalf("expected no command when workspace id is missing")
	}
	if m.status != "no workspace selected" {
		t.Fatalf("expected validation status, got %q", m.status)
	}
}

func TestEditWorkspaceGroupsReducerEscCancels(t *testing.T) {
	m := NewModel(nil)
	m.mode = uiModeEditWorkspaceGroups
	m.editWorkspaceID = "ws1"

	handled, cmd := m.reduceWorkspaceEditModes(tea.KeyPressMsg{Code: tea.KeyEsc})
	if !handled {
		t.Fatalf("expected esc to be handled")
	}
	if cmd != nil {
		t.Fatalf("expected no command on esc")
	}
	if m.mode != uiModeNormal {
		t.Fatalf("expected mode to return to normal, got %v", m.mode)
	}
	if m.status != "edit canceled" {
		t.Fatalf("expected cancel status, got %q", m.status)
	}
	if m.editWorkspaceID != "" {
		t.Fatalf("expected edit workspace id to clear, got %q", m.editWorkspaceID)
	}
}

type recordingClipboardService struct {
	text   string
	method clipboardMethod
	err    error
	calls  int
}

func (s *recordingClipboardService) Copy(_ context.Context, text string) (clipboardMethod, error) {
	s.calls++
	s.text = text
	method := s.method
	if method == 0 {
		method = clipboardMethodSystem
	}
	return method, s.err
}
