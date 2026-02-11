package app

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

func (m *Model) reduceWorkspaceEditModes(msg tea.Msg) (bool, tea.Cmd) {
	switch m.mode {
	case uiModeRenameWorkspace:
		keyMsg, ok := msg.(tea.KeyMsg)
		if !ok {
			return true, nil
		}
		switch keyMsg.String() {
		case "esc":
			m.exitRenameWorkspace("rename canceled")
			return true, nil
		case "enter":
			if m.renameInput == nil {
				return true, nil
			}
			name := strings.TrimSpace(m.renameInput.Value())
			if name == "" {
				m.setValidationStatus("name is required")
				return true, nil
			}
			id := m.renameWorkspaceID
			if id == "" {
				m.setValidationStatus("no workspace selected")
				return true, nil
			}
			m.renameInput.SetValue("")
			m.exitRenameWorkspace("renaming workspace")
			return true, updateWorkspaceCmd(m.workspaceAPI, id, name)
		}
		if m.renameInput != nil {
			return true, m.renameInput.Update(keyMsg)
		}
		return true, nil
	case uiModeRenameSession:
		keyMsg, ok := msg.(tea.KeyMsg)
		if !ok {
			return true, nil
		}
		switch keyMsg.String() {
		case "esc":
			m.exitRenameSession("rename canceled")
			return true, nil
		case "enter":
			if m.renameInput == nil {
				return true, nil
			}
			name := strings.TrimSpace(m.renameInput.Value())
			if name == "" {
				m.setValidationStatus("name is required")
				return true, nil
			}
			id := m.renameSessionID
			if id == "" {
				m.setValidationStatus("no session selected")
				return true, nil
			}
			m.renameInput.SetValue("")
			m.exitRenameSession("renaming session")
			return true, updateSessionCmd(m.sessionAPI, id, name)
		}
		if m.renameInput != nil {
			return true, m.renameInput.Update(keyMsg)
		}
		return true, nil
	case uiModeAddWorkspaceGroup:
		keyMsg, ok := msg.(tea.KeyMsg)
		if !ok {
			return true, nil
		}
		switch keyMsg.String() {
		case "esc":
			m.exitAddWorkspaceGroup("add group canceled")
			return true, nil
		case "enter":
			if m.groupInput == nil {
				return true, nil
			}
			name := strings.TrimSpace(m.groupInput.Value())
			if name == "" {
				m.setValidationStatus("name is required")
				return true, nil
			}
			m.groupInput.SetValue("")
			m.exitAddWorkspaceGroup("creating group")
			return true, createWorkspaceGroupCmd(m.workspaceAPI, name)
		}
		if m.groupInput != nil {
			return true, m.groupInput.Update(keyMsg)
		}
		return true, nil
	case uiModePickWorkspaceRename, uiModePickWorkspaceGroupEdit:
		keyMsg, ok := msg.(tea.KeyMsg)
		if !ok {
			return true, nil
		}
		switch keyMsg.String() {
		case "esc":
			m.exitWorkspacePicker("selection canceled")
			return true, nil
		case "enter":
			id := ""
			if m.workspacePicker != nil {
				id = m.workspacePicker.SelectedID()
			}
			if id == "" {
				m.setValidationStatus("no workspace selected")
				return true, nil
			}
			if m.mode == uiModePickWorkspaceRename {
				m.enterRenameWorkspace(id)
			} else {
				m.enterEditWorkspaceGroups(id)
			}
			return true, nil
		case "j", "down":
			if m.workspacePicker != nil {
				m.workspacePicker.Move(1)
			}
			return true, nil
		case "k", "up":
			if m.workspacePicker != nil {
				m.workspacePicker.Move(-1)
			}
			return true, nil
		}
		return true, nil
	case uiModePickWorkspaceGroupRename, uiModePickWorkspaceGroupDelete, uiModePickWorkspaceGroupAssign:
		keyMsg, ok := msg.(tea.KeyMsg)
		if !ok {
			return true, nil
		}
		switch keyMsg.String() {
		case "esc":
			m.exitWorkspacePicker("selection canceled")
			return true, nil
		case "enter":
			id := ""
			if m.groupSelectPicker != nil {
				id = m.groupSelectPicker.SelectedID()
			}
			if id == "" {
				m.setValidationStatus("no group selected")
				return true, nil
			}
			switch m.mode {
			case uiModePickWorkspaceGroupRename:
				m.enterRenameWorkspaceGroup(id)
			case uiModePickWorkspaceGroupDelete:
				m.confirmDeleteWorkspaceGroup(id)
			case uiModePickWorkspaceGroupAssign:
				m.enterAssignGroupWorkspaces(id)
			}
			return true, nil
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
		}
		return true, nil
	case uiModeEditWorkspaceGroups:
		keyMsg, ok := msg.(tea.KeyMsg)
		if !ok {
			return true, nil
		}
		switch keyMsg.String() {
		case "esc":
			m.exitEditWorkspaceGroups("edit canceled")
			return true, nil
		case "enter":
			if m.groupPicker == nil {
				return true, nil
			}
			ids := m.groupPicker.SelectedIDs()
			id := m.editWorkspaceID
			if id == "" {
				m.setValidationStatus("no workspace selected")
				return true, nil
			}
			m.exitEditWorkspaceGroups("saving groups")
			return true, updateWorkspaceGroupsCmd(m.workspaceAPI, id, ids)
		case " ", "space":
			if m.groupPicker != nil && m.groupPicker.Toggle() {
				return true, nil
			}
		case "j", "down":
			if m.groupPicker != nil && m.groupPicker.Move(1) {
				return true, nil
			}
		case "k", "up":
			if m.groupPicker != nil && m.groupPicker.Move(-1) {
				return true, nil
			}
		}
		return true, nil
	case uiModeRenameWorkspaceGroup:
		keyMsg, ok := msg.(tea.KeyMsg)
		if !ok {
			return true, nil
		}
		switch keyMsg.String() {
		case "esc":
			m.exitRenameWorkspaceGroup("rename canceled")
			return true, nil
		case "enter":
			if m.groupInput == nil {
				return true, nil
			}
			name := strings.TrimSpace(m.groupInput.Value())
			if name == "" {
				m.setValidationStatus("name is required")
				return true, nil
			}
			id := m.renameGroupID
			if id == "" {
				m.setValidationStatus("no group selected")
				return true, nil
			}
			m.groupInput.SetValue("")
			m.exitRenameWorkspaceGroup("renaming group")
			return true, updateWorkspaceGroupCmd(m.workspaceAPI, id, name)
		}
		if m.groupInput != nil {
			return true, m.groupInput.Update(keyMsg)
		}
		return true, nil
	case uiModeAssignGroupWorkspaces:
		keyMsg, ok := msg.(tea.KeyMsg)
		if !ok {
			return true, nil
		}
		switch keyMsg.String() {
		case "esc":
			m.exitAssignGroupWorkspaces("assignment canceled")
			return true, nil
		case "enter":
			if m.workspaceMulti == nil {
				return true, nil
			}
			ids := m.workspaceMulti.SelectedIDs()
			groupID := m.assignGroupID
			if groupID == "" {
				m.setValidationStatus("no group selected")
				return true, nil
			}
			m.exitAssignGroupWorkspaces("saving assignments")
			return true, assignGroupWorkspacesCmd(m.workspaceAPI, groupID, ids, m.workspaces)
		case " ", "space":
			if m.workspaceMulti != nil && m.workspaceMulti.Toggle() {
				return true, nil
			}
		case "j", "down":
			if m.workspaceMulti != nil && m.workspaceMulti.Move(1) {
				return true, nil
			}
		case "k", "up":
			if m.workspaceMulti != nil && m.workspaceMulti.Move(-1) {
				return true, nil
			}
		}
		return true, nil
	default:
		return false, nil
	}
}

func (m *Model) reduceAddWorkspaceMode(msg tea.Msg) (bool, tea.Cmd) {
	if m.mode != uiModeAddWorkspace {
		return false, nil
	}
	if stream, ok := msg.(streamMsg); ok {
		m.applyStreamMsg(stream)
		return true, nil
	}
	if m.addWorkspace == nil {
		return true, nil
	}
	_, cmd := m.addWorkspace.Update(msg, m)
	return true, cmd
}

func (m *Model) reduceAddWorktreeMode(msg tea.Msg) (bool, tea.Cmd) {
	if m.mode != uiModeAddWorktree {
		return false, nil
	}
	if stream, ok := msg.(streamMsg); ok {
		m.applyStreamMsg(stream)
		return true, nil
	}
	if m.addWorktree == nil {
		return true, nil
	}
	_, cmd := m.addWorktree.Update(msg, m)
	return true, cmd
}

func (m *Model) reducePickProviderMode(msg tea.Msg) (bool, tea.Cmd) {
	if m.mode != uiModePickProvider {
		return false, nil
	}
	switch msg := msg.(type) {
	case streamMsg:
		m.applyStreamMsg(msg)
		return true, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			m.exitProviderPick("new session canceled")
			return true, nil
		case "enter":
			return true, m.selectProvider()
		case "j", "down":
			if m.providerPicker != nil {
				m.providerPicker.Move(1)
			}
			return true, nil
		case "k", "up":
			if m.providerPicker != nil {
				m.providerPicker.Move(-1)
			}
			return true, nil
		}
	}
	return true, nil
}

func (m *Model) reduceMenuMode(msg tea.Msg) (bool, tea.Cmd) {
	if m.menu == nil || !m.menu.IsActive() {
		return false, nil
	}
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return false, nil
	}
	previous := m.menu.SelectedGroupIDs()
	if handled, action := m.menu.HandleKey(keyMsg); handled {
		cmds := []tea.Cmd{}
		if cmd := m.handleMenuAction(action); cmd != nil {
			cmds = append(cmds, cmd)
		}
		if m.handleMenuGroupChange(previous) {
			cmds = append(cmds, m.saveAppStateCmd())
		}
		return true, tea.Batch(cmds...)
	}
	return false, nil
}

func (m *Model) reduceComposeMode(msg tea.Msg) (bool, tea.Cmd) {
	if m.mode != uiModeCompose {
		return false, nil
	}
	stream, ok := msg.(streamMsg)
	if !ok {
		return false, nil
	}
	m.applyStreamMsg(stream)
	return true, nil
}

func (m *Model) reduceSearchModeKey(msg tea.KeyMsg) (bool, tea.Cmd) {
	if m.mode != uiModeSearch {
		return false, nil
	}
	switch msg.String() {
	case "esc":
		m.exitSearch("search canceled")
		return true, nil
	case "enter":
		if m.searchInput != nil {
			query := m.searchInput.Value()
			m.applySearch(query)
		}
		m.exitSearch("")
		return true, nil
	}
	if m.searchInput != nil {
		return true, m.searchInput.Update(msg)
	}
	return true, nil
}

func (m *Model) reduceComposeInputKey(msg tea.KeyMsg) (bool, tea.Cmd) {
	if m.input == nil || !m.input.IsChatFocused() {
		return false, nil
	}
	if m.composeOptionPickerOpen() {
		switch msg.String() {
		case "esc":
			m.closeComposeOptionPicker()
			m.setStatusMessage("session options picker closed")
			return true, nil
		case "enter":
			value := ""
			if m.composeOptionPicker != nil {
				value = m.composeOptionPicker.SelectedID()
			}
			cmd := m.applyComposeOptionSelection(value)
			m.closeComposeOptionPicker()
			return true, cmd
		case "j", "down":
			if m.composeOptionPicker != nil {
				m.composeOptionPicker.Move(1)
			}
			return true, nil
		case "k", "up":
			if m.composeOptionPicker != nil {
				m.composeOptionPicker.Move(-1)
			}
			return true, nil
		}
	}
	switch msg.String() {
	case "ctrl+o":
		return true, m.toggleNotesPanel()
	case "ctrl+1":
		if m.openComposeOptionPicker(composeOptionModel) {
			m.setStatusMessage("select model")
		}
		return true, nil
	case "ctrl+2":
		if m.openComposeOptionPicker(composeOptionReasoning) {
			m.setStatusMessage("select reasoning")
		}
		return true, nil
	case "ctrl+3":
		if m.openComposeOptionPicker(composeOptionAccess) {
			m.setStatusMessage("select access")
		}
		return true, nil
	case "esc":
		m.closeComposeOptionPicker()
		m.exitCompose("compose canceled")
		return true, nil
	case "up":
		if m.chatInput != nil {
			if value, ok := m.composeHistoryNavigate(-1, m.chatInput.Value()); ok {
				m.chatInput.SetValue(value)
				return true, nil
			}
		}
		return true, nil
	case "down":
		if m.chatInput != nil {
			if value, ok := m.composeHistoryNavigate(1, m.chatInput.Value()); ok {
				m.chatInput.SetValue(value)
				return true, nil
			}
		}
		return true, nil
	case "enter":
		if m.chatInput == nil {
			return true, nil
		}
		text := strings.TrimSpace(m.chatInput.Value())
		if text == "" {
			m.setValidationStatus("message is required")
			return true, nil
		}
		if m.newSession != nil {
			target := m.newSession
			if strings.TrimSpace(target.provider) == "" {
				m.setValidationStatus("provider is required")
				return true, nil
			}
			m.enableFollow(false)
			m.setStatusMessage("starting session")
			m.chatInput.Clear()
			return true, m.startWorkspaceSessionCmd(target.workspaceID, target.worktreeID, target.provider, text, target.runtimeOptions)
		}
		sessionID := m.composeSessionID()
		if sessionID == "" {
			m.setValidationStatus("select a session to chat")
			return true, nil
		}
		m.recordComposeHistory(sessionID, text)
		saveHistoryCmd := m.saveAppStateCmd()
		provider := m.providerForSessionID(sessionID)
		m.enableFollow(false)
		m.startRequestActivity(sessionID, provider)
		token := m.nextSendToken()
		m.registerPendingSend(token, sessionID, provider)
		headerIndex := m.appendUserMessageLocal(provider, text)
		m.setStatusMessage("sending message")
		m.chatInput.Clear()
		if headerIndex >= 0 {
			m.registerPendingSendHeader(token, sessionID, provider, headerIndex)
		}
		send := sendSessionCmd(m.sessionAPI, sessionID, text, token)
		if shouldStreamItems(provider) {
			cmds := []tea.Cmd{send}
			if m.itemStream == nil || !m.itemStream.HasStream() {
				cmds = append([]tea.Cmd{openItemsCmd(m.sessionAPI, sessionID)}, cmds...)
			}
			key := m.pendingSessionKey
			if key == "" {
				key = m.selectedKey()
			}
			if key != "" {
				cmds = append(cmds, historyPollCmd(sessionID, key, 0, historyPollDelay, countAgentRepliesBlocks(m.currentBlocks())))
			}
			if saveHistoryCmd != nil {
				cmds = append(cmds, saveHistoryCmd)
			}
			return true, tea.Batch(cmds...)
		}
		if provider == "codex" {
			cmds := make([]tea.Cmd, 0, 4)
			if m.codexStream == nil || !m.codexStream.HasStream() {
				cmds = append(cmds, openEventsCmd(m.sessionAPI, sessionID))
			}
			cmds = append(cmds, send)
			key := m.pendingSessionKey
			if key == "" {
				key = m.selectedKey()
			}
			if key != "" {
				cmds = append(cmds, historyPollCmd(sessionID, key, 0, historyPollDelay, countAgentRepliesBlocks(m.currentBlocks())))
			}
			if saveHistoryCmd != nil {
				cmds = append(cmds, saveHistoryCmd)
			}
			return true, tea.Batch(cmds...)
		}
		if saveHistoryCmd != nil {
			return true, tea.Batch(send, saveHistoryCmd)
		}
		return true, send
	case "ctrl+c":
		if m.chatInput != nil {
			m.chatInput.Clear()
			m.setStatusMessage("input cleared")
		}
		return true, nil
	case "ctrl+y":
		id := m.selectedSessionID()
		if id == "" {
			m.setCopyStatusWarning("no session selected")
			return true, nil
		}
		m.copyWithStatus(id, "copied session id")
		return true, nil
	}
	if m.chatInput != nil {
		m.resetComposeHistoryCursor()
		return true, m.chatInput.Update(msg)
	}
	return true, nil
}
