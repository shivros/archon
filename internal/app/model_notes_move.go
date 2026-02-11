package app

import (
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"control/internal/types"
)

const (
	noteMoveTargetWorkspace = "workspace"
	noteMoveTargetWorktree  = "worktree"
	noteMoveTargetSession   = "session"
)

func (m *Model) noteMovePickerModeActive() bool {
	switch m.mode {
	case uiModePickNoteMoveTarget, uiModePickNoteMoveWorktree, uiModePickNoteMoveSession:
		return true
	default:
		return false
	}
}

func (m *Model) reduceNoteMovePickerMode(msg tea.Msg) (bool, tea.Cmd) {
	if !m.noteMovePickerModeActive() {
		return false, nil
	}
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return true, nil
	}
	switch keyMsg.String() {
	case "esc":
		m.exitNoteMovePicker("move canceled")
		return true, nil
	case "enter":
		return true, m.handleNoteMovePickerSelection()
	case "j", "down":
		if m.groupSelectPicker != nil {
			m.groupSelectPicker.Move(1)
		}
		return true, nil
	case "k", "up":
		if m.groupSelectPicker != nil {
			m.groupSelectPicker.Move(-1)
		}
		return true, nil
	default:
		return true, nil
	}
}

func (m *Model) moveNoteByViewportPosition(col, line int) (bool, tea.Cmd) {
	if col < 0 || line < 0 || len(m.contentBlocks) == 0 || len(m.contentBlockSpans) == 0 {
		return false, nil
	}
	absolute := m.viewport.YOffset + line
	for _, span := range m.contentBlockSpans {
		if !isNoteRole(span.Role) {
			continue
		}
		if span.MoveLine != absolute {
			continue
		}
		if span.MoveStart < 0 || span.MoveEnd < span.MoveStart {
			continue
		}
		if col < span.MoveStart || col > span.MoveEnd {
			continue
		}
		return true, m.beginMoveNoteByBlockIndex(span.BlockIndex)
	}
	return false, nil
}

func (m *Model) moveNotesPanelByViewportPosition(col, line int) (bool, tea.Cmd) {
	if col < 0 || line < 0 || len(m.notesPanelBlocks) == 0 || len(m.notesPanelSpans) == 0 {
		return false, nil
	}
	absolute := m.notesPanelViewport.YOffset + line
	for _, span := range m.notesPanelSpans {
		if !isNoteRole(span.Role) {
			continue
		}
		if span.MoveLine != absolute {
			continue
		}
		if span.MoveStart < 0 || span.MoveEnd < span.MoveStart {
			continue
		}
		if col < span.MoveStart || col > span.MoveEnd {
			continue
		}
		return true, m.beginMoveNoteByPanelBlockIndex(span.BlockIndex)
	}
	return false, nil
}

func (m *Model) beginMoveNoteByBlockIndex(index int) tea.Cmd {
	return m.beginMoveNoteByID(m.noteIDByBlockIndex(index))
}

func (m *Model) beginMoveNoteByPanelBlockIndex(index int) tea.Cmd {
	return m.beginMoveNoteByID(m.noteIDByPanelBlockIndex(index))
}

func (m *Model) beginMoveNoteByID(noteID string) tea.Cmd {
	noteID = strings.TrimSpace(noteID)
	if noteID == "" {
		m.setValidationStatus("select a note to move")
		return nil
	}
	note := m.noteByID(noteID)
	if note == nil {
		m.setValidationStatus("note not found")
		return nil
	}
	m.noteMoveNoteID = noteID
	m.noteMoveReturnMode = m.mode
	return m.enterNoteMoveFlow(note)
}

func (m *Model) enterNoteMoveFlow(note *types.Note) tea.Cmd {
	if note == nil {
		m.setValidationStatus("note not found")
		m.clearNoteMoveState()
		return nil
	}
	options := m.noteMoveTargetOptions(note)
	if len(options) == 0 {
		m.setValidationStatus("no move targets available")
		m.exitNoteMovePicker("")
		return nil
	}
	if len(options) == 1 {
		return m.applyNoteMoveTargetSelection(options[0].id)
	}
	m.enterNoteMovePicker(uiModePickNoteMoveTarget, options, "select note target")
	return nil
}

func (m *Model) applyNoteMoveTargetSelection(target string) tea.Cmd {
	note := m.noteByID(m.noteMoveNoteID)
	if note == nil {
		m.setValidationStatus("note not found")
		m.exitNoteMovePicker("")
		return nil
	}
	context := m.resolveNoteScopeTarget(note)
	switch strings.TrimSpace(target) {
	case noteMoveTargetWorkspace:
		if strings.TrimSpace(context.WorkspaceID) == "" {
			m.setValidationStatus("workspace unavailable for note")
			return nil
		}
		targetScope := noteScopeTarget{Scope: types.NoteScopeWorkspace, WorkspaceID: context.WorkspaceID}
		return m.commitNoteMove(note, targetScope)
	case noteMoveTargetWorktree:
		return m.enterNoteMoveWorktreePicker(note)
	case noteMoveTargetSession:
		return m.enterNoteMoveSessionPicker(note)
	default:
		m.setValidationStatus("invalid move target")
		return nil
	}
}

func (m *Model) enterNoteMovePicker(mode uiMode, options []selectOption, status string) {
	if m.groupSelectPicker != nil {
		m.groupSelectPicker.SetOptions(options)
	}
	m.mode = mode
	if m.noteInput != nil {
		m.noteInput.Blur()
	}
	if m.chatInput != nil {
		m.chatInput.Blur()
	}
	if m.input != nil {
		m.input.FocusSidebar()
	}
	if status != "" {
		m.setStatusMessage(status)
	}
	m.resize(m.width, m.height)
}

func (m *Model) clearNoteMoveState() {
	m.noteMoveNoteID = ""
	m.noteMoveReturnMode = uiModeNormal
}

func (m *Model) exitNoteMovePicker(status string) {
	returnMode := m.noteMoveReturnMode
	if returnMode == uiModePickNoteMoveTarget || returnMode == uiModePickNoteMoveWorktree || returnMode == uiModePickNoteMoveSession {
		returnMode = uiModeNotes
	}
	m.clearNoteMoveState()
	m.mode = returnMode
	if m.input != nil {
		switch returnMode {
		case uiModeCompose:
			m.input.FocusChatInput()
			if m.chatInput != nil {
				m.chatInput.Focus()
			}
		case uiModeAddNote:
			m.input.FocusChatInput()
			if m.noteInput != nil {
				m.noteInput.Focus()
			}
		default:
			m.input.FocusSidebar()
		}
	}
	if status != "" {
		m.setStatusMessage(status)
	}
	m.resize(m.width, m.height)
}

func (m *Model) handleNoteMovePickerSelection() tea.Cmd {
	note := m.noteByID(m.noteMoveNoteID)
	if note == nil {
		m.setValidationStatus("note not found")
		m.exitNoteMovePicker("")
		return nil
	}
	selected := ""
	if m.groupSelectPicker != nil {
		selected = strings.TrimSpace(m.groupSelectPicker.SelectedID())
	}
	if selected == "" {
		m.setValidationStatus("select a target")
		return nil
	}
	switch m.mode {
	case uiModePickNoteMoveTarget:
		return m.applyNoteMoveTargetSelection(selected)
	case uiModePickNoteMoveWorktree:
		return m.applyNoteMoveWorktreeSelection(note, selected)
	case uiModePickNoteMoveSession:
		return m.applyNoteMoveSessionSelection(note, selected)
	default:
		return nil
	}
}

func (m *Model) enterNoteMoveWorktreePicker(note *types.Note) tea.Cmd {
	context := m.resolveNoteScopeTarget(note)
	workspaceID := strings.TrimSpace(context.WorkspaceID)
	if workspaceID == "" {
		m.setValidationStatus("workspace unavailable for note")
		m.exitNoteMovePicker("")
		return nil
	}
	options := m.noteMoveWorktreeOptions(workspaceID)
	if len(options) == 0 {
		m.setValidationStatus("no worktrees available")
		m.exitNoteMovePicker("")
		return nil
	}
	m.enterNoteMovePicker(uiModePickNoteMoveWorktree, options, "select worktree")
	return nil
}

func (m *Model) enterNoteMoveSessionPicker(note *types.Note) tea.Cmd {
	context := m.resolveNoteScopeTarget(note)
	workspaceID := strings.TrimSpace(context.WorkspaceID)
	if workspaceID == "" {
		m.setValidationStatus("workspace unavailable for note")
		m.exitNoteMovePicker("")
		return nil
	}
	options := m.noteMoveSessionOptions(workspaceID)
	if len(options) == 0 {
		m.setValidationStatus("no sessions available")
		m.exitNoteMovePicker("")
		return nil
	}
	m.enterNoteMovePicker(uiModePickNoteMoveSession, options, "select session")
	return nil
}

func (m *Model) applyNoteMoveWorktreeSelection(note *types.Note, worktreeID string) tea.Cmd {
	worktreeID = strings.TrimSpace(worktreeID)
	if worktreeID == "" {
		m.setValidationStatus("select a worktree")
		return nil
	}
	context := m.resolveNoteScopeTarget(note)
	workspaceID := strings.TrimSpace(context.WorkspaceID)
	if workspaceID == "" {
		if wt := m.worktreeByID(worktreeID); wt != nil {
			workspaceID = strings.TrimSpace(wt.WorkspaceID)
		}
	}
	if workspaceID == "" {
		m.setValidationStatus("workspace unavailable for worktree")
		return nil
	}
	target := noteScopeTarget{
		Scope:       types.NoteScopeWorktree,
		WorkspaceID: workspaceID,
		WorktreeID:  worktreeID,
	}
	return m.commitNoteMove(note, target)
}

func (m *Model) applyNoteMoveSessionSelection(note *types.Note, sessionID string) tea.Cmd {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		m.setValidationStatus("select a session")
		return nil
	}
	target := m.noteScopeForSession(sessionID, "", "")
	if target.SessionID == "" {
		m.setValidationStatus("session unavailable")
		return nil
	}
	return m.commitNoteMove(note, target)
}

func (m *Model) commitNoteMove(note *types.Note, target noteScopeTarget) tea.Cmd {
	if note == nil {
		m.setValidationStatus("note not found")
		return nil
	}
	source := m.resolveNoteScopeTarget(note)
	sourceWorkspace := strings.TrimSpace(source.WorkspaceID)
	if sourceWorkspace == "" {
		m.setValidationStatus("workspace unavailable for note")
		return nil
	}
	target.WorkspaceID = strings.TrimSpace(target.WorkspaceID)
	target.WorktreeID = strings.TrimSpace(target.WorktreeID)
	target.SessionID = strings.TrimSpace(target.SessionID)
	if target.WorkspaceID == "" {
		target.WorkspaceID = sourceWorkspace
	}
	if target.WorkspaceID != sourceWorkspace {
		m.setValidationStatus("cross-workspace note move is not supported")
		return nil
	}
	switch target.Scope {
	case types.NoteScopeWorkspace:
		target.WorktreeID = ""
		target.SessionID = ""
	case types.NoteScopeWorktree:
		if target.WorktreeID == "" {
			m.setValidationStatus("worktree unavailable for note move")
			return nil
		}
		wt := m.worktreeByID(target.WorktreeID)
		if wt == nil || strings.TrimSpace(wt.WorkspaceID) != sourceWorkspace {
			m.setValidationStatus("invalid worktree target")
			return nil
		}
		target.SessionID = ""
	case types.NoteScopeSession:
		if target.SessionID == "" {
			m.setValidationStatus("session unavailable for note move")
			return nil
		}
		meta := m.sessionMeta[target.SessionID]
		if meta == nil {
			m.setValidationStatus("invalid session target")
			return nil
		}
		if strings.TrimSpace(meta.WorkspaceID) != sourceWorkspace {
			m.setValidationStatus("cross-workspace note move is not supported")
			return nil
		}
		target.WorkspaceID = strings.TrimSpace(meta.WorkspaceID)
		target.WorktreeID = strings.TrimSpace(meta.WorktreeID)
	default:
		m.setValidationStatus("invalid note move target")
		return nil
	}
	if noteMoveTargetSameAsSource(source, target) {
		m.setValidationStatus("note already in target scope")
		return nil
	}
	m.exitNoteMovePicker("moving note")
	return moveNoteCmd(m.notesAPI, note, target)
}

func noteMoveTargetSameAsSource(source noteScopeTarget, target noteScopeTarget) bool {
	if source.Scope != target.Scope {
		return false
	}
	switch source.Scope {
	case types.NoteScopeWorkspace:
		return strings.TrimSpace(source.WorkspaceID) == strings.TrimSpace(target.WorkspaceID)
	case types.NoteScopeWorktree:
		return strings.TrimSpace(source.WorktreeID) == strings.TrimSpace(target.WorktreeID)
	case types.NoteScopeSession:
		return strings.TrimSpace(source.SessionID) == strings.TrimSpace(target.SessionID)
	default:
		return false
	}
}

func (m *Model) resolveNoteScopeTarget(note *types.Note) noteScopeTarget {
	if note == nil {
		return noteScopeTarget{}
	}
	target := noteScopeTarget{
		Scope:       note.Scope,
		WorkspaceID: strings.TrimSpace(note.WorkspaceID),
		WorktreeID:  strings.TrimSpace(note.WorktreeID),
		SessionID:   strings.TrimSpace(note.SessionID),
	}
	if target.Scope == types.NoteScopeSession && target.SessionID != "" {
		if meta := m.sessionMeta[target.SessionID]; meta != nil {
			if target.WorkspaceID == "" {
				target.WorkspaceID = strings.TrimSpace(meta.WorkspaceID)
			}
			if target.WorktreeID == "" {
				target.WorktreeID = strings.TrimSpace(meta.WorktreeID)
			}
		}
	}
	if target.Scope == types.NoteScopeWorktree && target.WorktreeID != "" && target.WorkspaceID == "" {
		if wt := m.worktreeByID(target.WorktreeID); wt != nil {
			target.WorkspaceID = strings.TrimSpace(wt.WorkspaceID)
		}
	}
	return target
}

func (m *Model) noteMoveTargetOptions(note *types.Note) []selectOption {
	context := m.resolveNoteScopeTarget(note)
	workspaceID := strings.TrimSpace(context.WorkspaceID)
	if workspaceID == "" {
		return nil
	}
	options := []selectOption{}
	if note.Scope != types.NoteScopeWorkspace {
		options = append(options, selectOption{id: noteMoveTargetWorkspace, label: "Move to Workspace"})
	}
	if note.Scope != types.NoteScopeWorktree && len(m.noteMoveWorktreeOptions(workspaceID)) > 0 {
		options = append(options, selectOption{id: noteMoveTargetWorktree, label: "Move to Worktree..."})
	}
	if note.Scope != types.NoteScopeSession && len(m.noteMoveSessionOptions(workspaceID)) > 0 {
		options = append(options, selectOption{id: noteMoveTargetSession, label: "Move to Session..."})
	}
	return options
}

func (m *Model) noteMoveWorktreeOptions(workspaceID string) []selectOption {
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		return nil
	}
	worktrees := append([]*types.Worktree(nil), m.worktrees[workspaceID]...)
	sort.SliceStable(worktrees, func(i, j int) bool {
		left := ""
		right := ""
		if worktrees[i] != nil {
			left = strings.TrimSpace(worktrees[i].Name)
		}
		if worktrees[j] != nil {
			right = strings.TrimSpace(worktrees[j].Name)
		}
		return strings.ToLower(left) < strings.ToLower(right)
	})
	options := make([]selectOption, 0, len(worktrees))
	for _, wt := range worktrees {
		if wt == nil || strings.TrimSpace(wt.ID) == "" {
			continue
		}
		name := strings.TrimSpace(wt.Name)
		if name == "" {
			name = strings.TrimSpace(wt.ID)
		}
		label := fmt.Sprintf("%s • %s", name, strings.TrimSpace(wt.ID))
		options = append(options, selectOption{id: strings.TrimSpace(wt.ID), label: label})
	}
	return options
}

func (m *Model) noteMoveSessionOptions(workspaceID string) []selectOption {
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		return nil
	}
	sessions := append([]*types.Session(nil), m.sessions...)
	sort.SliceStable(sessions, func(i, j int) bool {
		if sessions[i] == nil || sessions[j] == nil {
			return sessions[i] != nil
		}
		return sessions[i].CreatedAt.After(sessions[j].CreatedAt)
	})
	seen := map[string]struct{}{}
	options := []selectOption{}
	for _, session := range sessions {
		if session == nil || strings.TrimSpace(session.ID) == "" {
			continue
		}
		if !isVisibleStatus(session.Status) {
			continue
		}
		sessionID := strings.TrimSpace(session.ID)
		if _, ok := seen[sessionID]; ok {
			continue
		}
		meta := m.sessionMeta[sessionID]
		if isDismissedSession(session, meta) {
			continue
		}
		if meta == nil || strings.TrimSpace(meta.WorkspaceID) != workspaceID {
			continue
		}
		title := strings.TrimSpace(sessionTitle(session, meta))
		if title == "" {
			title = sessionID
		}
		location := "workspace"
		worktreeID := strings.TrimSpace(meta.WorktreeID)
		if worktreeID != "" {
			location = "worktree " + worktreeID
			if wt := m.worktreeByID(worktreeID); wt != nil && strings.TrimSpace(wt.Name) != "" {
				location = "worktree " + strings.TrimSpace(wt.Name)
			}
		}
		label := fmt.Sprintf("%s • %s • %s", title, location, sessionID)
		options = append(options, selectOption{id: sessionID, label: label})
		seen[sessionID] = struct{}{}
	}
	return options
}

func (m *Model) noteIDByPanelBlockIndex(index int) string {
	if index < 0 || index >= len(m.notesPanelBlocks) {
		return ""
	}
	block := m.notesPanelBlocks[index]
	if !isNoteRole(block.Role) {
		return ""
	}
	return strings.TrimSpace(block.ID)
}
