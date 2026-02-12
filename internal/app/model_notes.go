package app

import (
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"control/internal/client"
	"control/internal/types"
)

type noteScopeTarget struct {
	Scope       types.NoteScope
	WorkspaceID string
	WorktreeID  string
	SessionID   string
}

type notesFilterState struct {
	ShowWorkspace bool
	ShowWorktree  bool
	ShowSession   bool
}

func (s noteScopeTarget) IsZero() bool {
	return s.Scope == ""
}

func (s noteScopeTarget) Label() string {
	switch s.Scope {
	case types.NoteScopeWorkspace:
		if s.WorkspaceID == "" {
			return "workspace"
		}
		return "workspace " + s.WorkspaceID
	case types.NoteScopeWorktree:
		if s.WorktreeID == "" {
			return "worktree"
		}
		return "worktree " + s.WorktreeID
	case types.NoteScopeSession:
		if s.SessionID == "" {
			return "session"
		}
		return "session " + s.SessionID
	default:
		return "notes"
	}
}

func (s noteScopeTarget) ToListRequest() client.ListNotesRequest {
	return client.ListNotesRequest{
		Scope:       s.Scope,
		WorkspaceID: s.WorkspaceID,
		WorktreeID:  s.WorktreeID,
		SessionID:   s.SessionID,
	}
}

func (m *Model) enterNotesForSelection() tea.Cmd {
	scope, ok := m.currentNoteScope()
	if !ok {
		m.setValidationStatus("select a workspace, worktree, or session")
		return nil
	}
	return m.openNotesScope(scope)
}

func (m *Model) enterAddNoteForSelection() tea.Cmd {
	scope, ok := m.currentNoteScope()
	if !ok {
		m.setValidationStatus("select a workspace, worktree, or session")
		return nil
	}
	return m.enterAddNoteForScope(scope)
}

func (m *Model) openNotesScope(scope noteScopeTarget) tea.Cmd {
	m.setNotesRootScope(scope)
	m.notesReturnMode = m.mode
	if m.notesReturnMode != uiModeCompose {
		m.notesReturnMode = uiModeNormal
	}
	m.mode = uiModeNotes
	if m.noteInput != nil {
		m.noteInput.Blur()
		m.noteInput.SetValue("")
	}
	if m.input != nil {
		m.input.FocusSidebar()
	}
	m.setContentText("Loading notes...")
	m.setStatusMessage("loading notes for " + scope.Label())
	m.resize(m.width, m.height)
	return m.refreshNotesForCurrentScope()
}

func (m *Model) enterAddNoteForScope(scope noteScopeTarget) tea.Cmd {
	cmd := m.openNotesScope(scope)
	m.enterAddNote()
	return cmd
}

func (m *Model) exitNotes(status string) tea.Cmd {
	next := m.notesReturnMode
	if next != uiModeCompose {
		next = uiModeNormal
	}
	m.mode = next
	m.notesReturnMode = uiModeNormal
	if m.noteInput != nil {
		m.noteInput.Blur()
		m.noteInput.SetValue("")
	}
	if m.input != nil {
		if next == uiModeCompose {
			m.input.FocusChatInput()
			if m.chatInput != nil {
				m.chatInput.Focus()
			}
		} else {
			m.input.FocusSidebar()
		}
	}
	if status != "" {
		m.setStatusMessage(status)
	}
	m.resize(m.width, m.height)
	return m.onSelectionChangedImmediate()
}

func (m *Model) enterAddNote() {
	if m.notesScope.IsZero() {
		m.setValidationStatus("open notes first")
		return
	}
	m.mode = uiModeAddNote
	if m.noteInput != nil {
		m.noteInput.SetPlaceholder("note")
		m.noteInput.SetValue("")
		m.noteInput.Focus()
	}
	if m.input != nil {
		m.input.FocusChatInput()
	}
	m.setStatusMessage("add note for " + m.notesScope.Label())
	m.resize(m.width, m.height)
}

func (m *Model) exitAddNote(status string) {
	m.mode = uiModeNotes
	if m.noteInput != nil {
		m.noteInput.Blur()
	}
	if m.input != nil {
		m.input.FocusSidebar()
	}
	if status != "" {
		m.setStatusMessage(status)
	}
	m.resize(m.width, m.height)
}

func (m *Model) reduceNotesModeKey(msg tea.KeyMsg) (bool, tea.Cmd) {
	if m.mode != uiModeNotes {
		return false, nil
	}
	if m.keyMatchesCommand(msg, KeyCommandNotesNew, "n") {
		m.enterAddNote()
		return true, nil
	}
	switch m.keyString(msg) {
	case "ctrl+o":
		return true, m.toggleNotesPanel()
	case "1":
		return true, m.toggleNotesFilterScope(types.NoteScopeWorkspace)
	case "2":
		return true, m.toggleNotesFilterScope(types.NoteScopeWorktree)
	case "3":
		return true, m.toggleNotesFilterScope(types.NoteScopeSession)
	case "0":
		return true, m.enableAllNotesFilters()
	case "esc":
		return true, m.exitNotes("notes closed")
	case "r":
		m.setStatusMessage("refreshing notes")
		return true, m.refreshNotesForCurrentScope()
	case "q":
		return true, tea.Quit
	default:
		return false, nil
	}
}

func (m *Model) reduceAddNoteMode(msg tea.Msg) (bool, tea.Cmd) {
	if m.mode != uiModeAddNote {
		return false, nil
	}
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return true, nil
	}
	switch m.keyString(keyMsg) {
	case "ctrl+o":
		return true, m.toggleNotesPanel()
	case "q":
		return true, tea.Quit
	}
	controller := textInputModeController{
		input:             m.noteInput,
		keyString:         m.keyString,
		keyMatchesCommand: m.keyMatchesCommand,
		onCancel: func() tea.Cmd {
			m.exitAddNote("add note canceled")
			return nil
		},
		onSubmit: m.submitAddNoteInput,
	}
	return controller.Update(keyMsg)
}

func (m *Model) submitAddNoteInput(body string) tea.Cmd {
	if strings.TrimSpace(body) == "" {
		m.setValidationStatus("note is required")
		return nil
	}
	if m.noteInput != nil {
		m.noteInput.Clear()
	}
	m.exitAddNote("saving note")
	return createNoteCmd(m.notesAPI, m.notesScope, body)
}

func (m *Model) currentNoteScope() (noteScopeTarget, bool) {
	if m.mode == uiModeCompose {
		if id := m.composeSessionID(); strings.TrimSpace(id) != "" {
			scope := noteScopeTarget{Scope: types.NoteScopeSession, SessionID: strings.TrimSpace(id)}
			if meta := m.sessionMeta[id]; meta != nil {
				scope.WorkspaceID = meta.WorkspaceID
				scope.WorktreeID = meta.WorktreeID
			}
			return scope, true
		}
	}
	item := m.selectedItem()
	if item == nil {
		return noteScopeTarget{}, false
	}
	switch item.kind {
	case sidebarSession:
		if item.session == nil {
			return noteScopeTarget{}, false
		}
		scope := noteScopeTarget{
			Scope:     types.NoteScopeSession,
			SessionID: item.session.ID,
		}
		if item.meta != nil {
			scope.WorkspaceID = item.meta.WorkspaceID
			scope.WorktreeID = item.meta.WorktreeID
		}
		return scope, true
	case sidebarWorktree:
		if item.worktree == nil {
			return noteScopeTarget{}, false
		}
		return noteScopeTarget{
			Scope:       types.NoteScopeWorktree,
			WorkspaceID: item.worktree.WorkspaceID,
			WorktreeID:  item.worktree.ID,
		}, true
	case sidebarWorkspace:
		if item.workspace == nil || item.workspace.ID == "" || item.workspace.ID == unassignedWorkspaceID {
			return noteScopeTarget{}, false
		}
		return noteScopeTarget{
			Scope:       types.NoteScopeWorkspace,
			WorkspaceID: item.workspace.ID,
		}, true
	default:
		return noteScopeTarget{}, false
	}
}

func (m *Model) onNotesSelectionChanged() tea.Cmd {
	scope, ok := m.currentNoteScope()
	if !ok {
		m.setValidationStatus("selected item has no notes scope")
		m.notes = nil
		m.notesByScope = map[types.NoteScope][]*types.Note{}
		m.setContentText("No notes scope for this selection.")
		return nil
	}
	m.setNotesRootScope(scope)
	m.setContentText("Loading notes...")
	return m.refreshNotesForCurrentScope()
}

func (m *Model) noteScopeForSession(sessionID, workspaceID, worktreeID string) noteScopeTarget {
	scope := noteScopeTarget{
		Scope:       types.NoteScopeSession,
		SessionID:   strings.TrimSpace(sessionID),
		WorkspaceID: strings.TrimSpace(workspaceID),
		WorktreeID:  strings.TrimSpace(worktreeID),
	}
	if meta := m.sessionMeta[scope.SessionID]; meta != nil {
		if scope.WorkspaceID == "" {
			scope.WorkspaceID = meta.WorkspaceID
		}
		if scope.WorktreeID == "" {
			scope.WorktreeID = meta.WorktreeID
		}
	}
	return scope
}

func notesToBlocks(notes []*types.Note, scope noteScopeTarget, filters notesFilterState) []ChatBlock {
	return notesToBlocksWithNewKey(notes, scope, filters, "n")
}

func notesToBlocksWithNewKey(notes []*types.Note, scope noteScopeTarget, filters notesFilterState, notesNewKey string) []ChatBlock {
	notesNewKey = strings.TrimSpace(notesNewKey)
	if notesNewKey == "" {
		notesNewKey = "n"
	}
	filterSection := notesFilterSection(scope, filters)
	header := ChatBlock{
		ID:   "notes-scope",
		Role: ChatRoleSystem,
		Text: fmt.Sprintf("Notes\n\nScope: %s\n\n%s\n\nKeys: 1/2/3 toggle filters, 0 all, %s add note, r refresh, esc back", scope.Label(), filterSection, notesNewKey),
	}
	if !hasAnyNotesFilterEnabled(scope, filters) {
		return []ChatBlock{
			header,
			{
				ID:   "notes-empty",
				Role: ChatRoleSystem,
				Text: "No scopes selected.\n\nToggle a filter to view notes.",
			},
		}
	}
	if len(notes) == 0 {
		return []ChatBlock{
			header,
			{
				ID:   "notes-empty",
				Role: ChatRoleSystem,
				Text: fmt.Sprintf("No notes yet.\n\nUse Add Note from the context menu or press %s.", notesNewKey),
			},
		}
	}

	blocks := make([]ChatBlock, 0, len(notes)+1)
	blocks = append(blocks, header)
	for _, note := range notes {
		if note == nil {
			continue
		}
		role := chatRoleForNoteScope(note.Scope)
		blocks = append(blocks, ChatBlock{
			ID:   note.ID,
			Role: role,
			Text: renderNoteBlockText(note),
		})
	}
	return blocks
}

func notesFilterSection(scope noteScopeTarget, filters notesFilterState) string {
	parts := []string{"Filters:"}
	if isNotesScopeAvailable(scope, types.NoteScopeWorkspace) {
		parts = append(parts, notesFilterToken("Workspace", filters.ShowWorkspace))
	}
	if isNotesScopeAvailable(scope, types.NoteScopeWorktree) {
		parts = append(parts, notesFilterToken("Worktree", filters.ShowWorktree))
	}
	if isNotesScopeAvailable(scope, types.NoteScopeSession) {
		parts = append(parts, notesFilterToken("Session", filters.ShowSession))
	}
	return strings.Join(parts, "\n")
}

func notesFilterToken(label string, enabled bool) string {
	if enabled {
		return "[x] " + label
	}
	return "[ ] " + label
}

func chatRoleForNoteScope(scope types.NoteScope) ChatRole {
	switch scope {
	case types.NoteScopeSession:
		return ChatRoleSessionNote
	case types.NoteScopeWorkspace:
		return ChatRoleWorkspaceNote
	case types.NoteScopeWorktree:
		return ChatRoleWorktreeNote
	default:
		return ChatRoleSystem
	}
}

func renderNoteBlockText(note *types.Note) string {
	var b strings.Builder
	title := strings.TrimSpace(note.Title)
	if title != "" {
		b.WriteString(title)
	}
	if note.Body != "" {
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(note.Body)
	}
	if note.Kind == types.NoteKindPin && note.Source != nil {
		if note.Source.Snippet != "" {
			if b.Len() > 0 {
				b.WriteString("\n\n")
			}
			b.WriteString("Pinned from conversation:\n")
			b.WriteString(note.Source.Snippet)
		}
		if note.Source.SessionID != "" {
			b.WriteString("\n\nSession: ")
			b.WriteString(note.Source.SessionID)
		}
	}
	if len(note.Tags) > 0 {
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString("Tags: ")
		b.WriteString(strings.Join(note.Tags, ", "))
	}
	if note.Status != "" {
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString("Status: ")
		b.WriteString(string(note.Status))
	}
	if !note.UpdatedAt.IsZero() {
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString("Updated: ")
		b.WriteString(note.UpdatedAt.Local().Format(time.RFC822))
	}
	if b.Len() == 0 {
		b.WriteString(noteKindLabel(note.Kind))
	}
	return b.String()
}

func noteTitle(note *types.Note) string {
	title := strings.TrimSpace(note.Title)
	if title != "" {
		return title
	}
	body := strings.TrimSpace(note.Body)
	if body == "" {
		return "Untitled"
	}
	const maxLen = 64
	if len(body) > maxLen {
		return body[:maxLen] + "..."
	}
	return body
}

func (m *Model) noteByID(id string) *types.Note {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil
	}
	for _, note := range m.notes {
		if note == nil {
			continue
		}
		if strings.TrimSpace(note.ID) == id {
			return note
		}
	}
	for _, scope := range []types.NoteScope{types.NoteScopeWorkspace, types.NoteScopeWorktree, types.NoteScopeSession} {
		for _, note := range m.notesByScope[scope] {
			if note == nil {
				continue
			}
			if strings.TrimSpace(note.ID) == id {
				return note
			}
		}
	}
	return nil
}

func noteKindLabel(kind types.NoteKind) string {
	if kind == types.NoteKindPin {
		return "pin"
	}
	return "note"
}

func (m *Model) pinSelectedMessage() tea.Cmd {
	if m.messageSelectIndex < 0 || m.messageSelectIndex >= len(m.contentBlocks) {
		m.setValidationStatus("no message selected")
		return nil
	}
	return m.pinBlockByIndex(m.messageSelectIndex)
}
