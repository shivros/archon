package app

import (
	"testing"

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

func TestReduceClipboardAndSearchKeysUsesCtrlGForCopySessionID(t *testing.T) {
	m := NewModel(nil)

	handled, cmd := m.reduceClipboardAndSearchKeys(tea.KeyPressMsg{Code: 'g', Mod: tea.ModCtrl})
	if !handled {
		t.Fatalf("expected ctrl+g to be handled for copy session id")
	}
	if cmd != nil {
		t.Fatalf("expected no async command for copy session id")
	}
	if m.status != "no session selected" {
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
