package app

import (
	"strings"
	"testing"

	"control/internal/types"

	tea "charm.land/bubbletea/v2"
	xansi "github.com/charmbracelet/x/ansi"
)

func TestMessageSelectionEnterWithV(t *testing.T) {
	m := NewModel(nil)
	m.resize(120, 40)
	m.applyBlocks([]ChatBlock{
		{Role: ChatRoleUser, Text: "one"},
		{Role: ChatRoleAgent, Text: "two"},
	})

	handled, cmd := m.reduceViewToggleKeys(tea.KeyPressMsg{Text: "v"})
	if !handled {
		t.Fatalf("expected v to be handled")
	}
	if cmd != nil {
		t.Fatalf("expected no command")
	}
	// The 'v' key now opens the message action modal instead of persistent highlight.
	if m.messageActionModal == nil || !m.messageActionModal.IsOpen() {
		t.Fatalf("expected message action modal to be open")
	}
	if m.messageActionModal.BlockIndex() < 0 || m.messageActionModal.BlockIndex() >= len(m.contentBlocks) {
		t.Fatalf("unexpected modal block index %d", m.messageActionModal.BlockIndex())
	}
}

func TestMessageSelectionMoveAndExit(t *testing.T) {
	m := NewModel(nil)
	m.resize(120, 40)
	m.applyBlocks([]ChatBlock{
		{Role: ChatRoleUser, Text: "one"},
		{Role: ChatRoleAgent, Text: "two"},
	})
	// Manually activate old-style selection mode for keyboard navigation tests.
	m.messageSelectActive = true
	m.messageSelectIndex = 0

	handled, cmd := m.reduceMessageSelectionKey(tea.KeyPressMsg{Text: "j"})
	if !handled || cmd != nil {
		t.Fatalf("expected j to be handled without command")
	}
	if m.messageSelectIndex != 1 {
		t.Fatalf("expected selected index to move to 1, got %d", m.messageSelectIndex)
	}

	handled, cmd = m.reduceMessageSelectionKey(tea.KeyPressMsg{Code: tea.KeyEsc})
	if !handled || cmd != nil {
		t.Fatalf("expected esc to be handled without command")
	}
	if m.messageSelectActive {
		t.Fatalf("expected message selection to deactivate")
	}
}

func TestMessageSelectionCopyUsesPlainText(t *testing.T) {
	m := NewModel(nil)
	m.resize(120, 40)
	m.applyBlocks([]ChatBlock{{Role: ChatRoleAgent, Text: "   ", Status: ChatStatusSending}})
	// Manually activate old-style selection mode.
	m.messageSelectActive = true
	m.messageSelectIndex = 0

	handled, cmd := m.reduceMessageSelectionKey(tea.KeyPressMsg{Text: "y"})
	if !handled || cmd != nil {
		t.Fatalf("expected y to be handled without command")
	}
	if m.status != "nothing to copy" {
		t.Fatalf("expected plain-text copy path status, got %q", m.status)
	}
}

func TestMessageSelectionRenderShowsVisibleMarker(t *testing.T) {
	m := NewModel(nil)
	m.resize(120, 40)
	m.applyBlocks([]ChatBlock{{Role: ChatRoleAgent, Text: "hello"}})
	before := xansi.Strip(m.renderedText)
	if strings.Contains(before, "Selected") {
		t.Fatalf("did not expect selected marker before entering mode: %q", before)
	}

	// Manually activate old-style selection to verify the highlight render still works.
	m.messageSelectActive = true
	m.messageSelectIndex = 0
	m.renderViewport()
	after := xansi.Strip(m.renderedText)
	if !strings.Contains(after, "Selected") {
		t.Fatalf("expected selected marker in rendered text, got %q", after)
	}
}

func TestMessageSelectionExitUsesRemappedToggleCommand(t *testing.T) {
	m := NewModel(nil)
	m.resize(120, 40)
	m.applyBlocks([]ChatBlock{
		{Role: ChatRoleUser, Text: "one"},
		{Role: ChatRoleAgent, Text: "two"},
	})
	m.applyKeybindings(NewKeybindings(map[string]string{
		KeyCommandToggleMessageSelect: "ctrl+j",
	}))
	// Manually activate old-style selection mode.
	m.messageSelectActive = true
	m.messageSelectIndex = 0

	handled, cmd := m.reduceMessageSelectionKey(tea.KeyPressMsg{Code: 'j', Mod: tea.ModCtrl})
	if !handled {
		t.Fatalf("expected remapped toggle command to be handled")
	}
	if cmd != nil {
		t.Fatalf("expected no command")
	}
	if m.messageSelectActive {
		t.Fatalf("expected message selection to deactivate")
	}
}

func TestMessageSelectionEnterExitsToCompose(t *testing.T) {
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
	if m.sidebar != nil {
		m.sidebar.SelectBySessionID("s1")
	}
	m.applyBlocks([]ChatBlock{
		{Role: ChatRoleUser, Text: "one"},
		{Role: ChatRoleAgent, Text: "two"},
	})
	// Manually activate old-style selection mode.
	m.messageSelectActive = true
	m.messageSelectIndex = 0
	sessionID := m.selectedSessionID()
	if sessionID != "s1" {
		t.Fatalf("expected selected session s1, got %q", sessionID)
	}

	handled, _ := m.reduceMessageSelectionKey(tea.KeyPressMsg{Code: tea.KeyEnter})
	if !handled {
		t.Fatalf("expected enter to be handled")
	}
	if m.messageSelectActive {
		t.Fatalf("expected message selection to be deactivated")
	}
	if m.mode != uiModeCompose {
		t.Fatalf("expected compose mode after enter, got %d", m.mode)
	}
}

func TestMessageSelectionQuitUsesRemappedQuitCommand(t *testing.T) {
	m := NewModel(nil)
	m.resize(120, 40)
	m.applyBlocks([]ChatBlock{
		{Role: ChatRoleAgent, Text: "two"},
	})
	m.applyKeybindings(NewKeybindings(map[string]string{
		KeyCommandQuit: "ctrl+q",
	}))
	// Manually activate old-style selection mode.
	m.messageSelectActive = true
	m.messageSelectIndex = 0

	handled, cmd := m.reduceMessageSelectionKey(tea.KeyPressMsg{Code: 'q', Mod: tea.ModCtrl})
	if !handled {
		t.Fatalf("expected remapped quit command to be handled")
	}
	if cmd == nil {
		t.Fatalf("expected quit command")
	}
	if _, ok := cmd().(tea.QuitMsg); !ok {
		t.Fatalf("expected tea.QuitMsg from command")
	}
}

func TestMessageActionModalOpenClose(t *testing.T) {
	m := NewModel(nil)
	m.resize(120, 40)
	m.applyBlocks([]ChatBlock{
		{Role: ChatRoleAgent, Text: "hello"},
	})

	m.openMessageActionModal(0)
	if !m.messageActionModal.IsOpen() {
		t.Fatalf("expected modal to be open")
	}
	if m.messageActionModal.BlockIndex() != 0 {
		t.Fatalf("expected block index 0, got %d", m.messageActionModal.BlockIndex())
	}

	m.messageActionModal.Close()
	if m.messageActionModal.IsOpen() {
		t.Fatalf("expected modal to be closed")
	}
}

func TestMessageActionModalAgentShowsReasoning(t *testing.T) {
	m := NewModel(nil)
	m.resize(120, 40)
	m.applyBlocks([]ChatBlock{
		{Role: ChatRoleAgent, Text: "hello"},
	})

	m.openMessageActionModal(0)
	// Agent messages should show Copy, Pin, and Toggle Reasoning (3 items).
	if len(m.messageActionModal.items) != 3 {
		t.Fatalf("expected 3 items for agent message, got %d", len(m.messageActionModal.items))
	}
}

func TestMessageActionModalUserHidesReasoning(t *testing.T) {
	m := NewModel(nil)
	m.resize(120, 40)
	m.applyBlocks([]ChatBlock{
		{Role: ChatRoleUser, Text: "hello"},
	})

	m.openMessageActionModal(0)
	// User messages should show only Copy and Pin (2 items).
	if len(m.messageActionModal.items) != 2 {
		t.Fatalf("expected 2 items for user message, got %d", len(m.messageActionModal.items))
	}
}

func TestMessageActionModalKeyboardNavigation(t *testing.T) {
	m := NewModel(nil)
	m.resize(120, 40)
	m.applyBlocks([]ChatBlock{
		{Role: ChatRoleAgent, Text: "hello"},
	})
	m.openMessageActionModal(0)

	// Down should move selection.
	handled, action := m.messageActionModal.HandleKey(tea.KeyPressMsg{Text: "j"})
	if !handled || action != MessageActionNone {
		t.Fatalf("expected j to be handled with no action")
	}
	if m.messageActionModal.selected != 1 {
		t.Fatalf("expected selected to be 1, got %d", m.messageActionModal.selected)
	}

	// Up should move back.
	handled, _ = m.messageActionModal.HandleKey(tea.KeyPressMsg{Text: "k"})
	if !handled {
		t.Fatalf("expected k to be handled")
	}
	if m.messageActionModal.selected != 0 {
		t.Fatalf("expected selected to be 0, got %d", m.messageActionModal.selected)
	}

	// Esc should close.
	handled, _ = m.messageActionModal.HandleKey(tea.KeyPressMsg{Code: tea.KeyEsc})
	if !handled {
		t.Fatalf("expected esc to be handled")
	}
	// Note: the modal itself doesn't close on esc — that's handled by the caller.
}

func TestMessageActionModalShortcuts(t *testing.T) {
	m := NewModel(nil)
	m.resize(120, 40)
	m.applyBlocks([]ChatBlock{
		{Role: ChatRoleAgent, Text: "hello"},
	})
	m.openMessageActionModal(0)

	// 'y' shortcut should return Copy action.
	handled, action := m.messageActionModal.HandleKey(tea.KeyPressMsg{Text: "y"})
	if !handled || action != MessageActionCopy {
		t.Fatalf("expected y to return copy action")
	}

	// 'p' shortcut should return Pin action.
	handled, action = m.messageActionModal.HandleKey(tea.KeyPressMsg{Text: "p"})
	if !handled || action != MessageActionPin {
		t.Fatalf("expected p to return pin action")
	}
}

func TestMessageActionModalOverlayRendering(t *testing.T) {
	m := NewModel(nil)
	m.resize(120, 40)
	m.applyBlocks([]ChatBlock{
		{Role: ChatRoleAgent, Text: "hello"},
	})
	m.openMessageActionModal(0)

	block, x, y := m.messageActionModal.ViewBlock(m.width, m.height-1)
	if block == "" {
		t.Fatalf("expected non-empty view block")
	}
	if x < 0 || y < 0 {
		t.Fatalf("expected valid position, got (%d, %d)", x, y)
	}
	stripped := xansi.Strip(block)
	if !strings.Contains(stripped, "Message Actions") {
		t.Fatalf("expected header 'Message Actions' in view, got %q", stripped)
	}
	if !strings.Contains(stripped, "(y) Copy") {
		t.Fatalf("expected '(y) Copy' item in view, got %q", stripped)
	}
	if !strings.Contains(stripped, "(p) Pin") {
		t.Fatalf("expected '(p) Pin' item in view, got %q", stripped)
	}
}
