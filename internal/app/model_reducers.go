package app

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

func (m *Model) reduceWorkspaceEditModes(msg tea.Msg) (bool, tea.Cmd) {
	switch m.mode {
	case uiModeEditWorkspace:
		if stream, ok := msg.(streamMsg); ok {
			m.applyStreamMsg(stream)
			return true, nil
		}
		if m.editWorkspace == nil {
			return true, nil
		}
		_, cmd := m.editWorkspace.Update(msg, m)
		return true, cmd
	case uiModeRenameWorktree:
		if !isTextInputMsg(msg) {
			return true, nil
		}
		controller := m.newSingleLineInputController(
			m.renameInput,
			func() tea.Cmd {
				m.exitRenameWorktree("rename canceled")
				return nil
			},
			m.submitRenameWorktreeInput,
		)
		return controller.Update(msg)
	case uiModeRenameSession:
		if !isTextInputMsg(msg) {
			return true, nil
		}
		controller := m.newSingleLineInputController(
			m.renameInput,
			func() tea.Cmd {
				m.exitRenameSession("rename canceled")
				return nil
			},
			m.submitRenameSessionInput,
		)
		return controller.Update(msg)
	case uiModeRenameWorkflow:
		if !isTextInputMsg(msg) {
			return true, nil
		}
		controller := m.newSingleLineInputController(
			m.renameInput,
			func() tea.Cmd {
				m.exitRenameWorkflow("rename canceled")
				return nil
			},
			m.submitRenameWorkflowInput,
		)
		return controller.Update(msg)
	case uiModeAddWorkspaceGroup:
		if !isTextInputMsg(msg) {
			return true, nil
		}
		controller := m.newSingleLineInputController(
			m.groupInput,
			func() tea.Cmd {
				m.exitAddWorkspaceGroup("add group canceled")
				return nil
			},
			m.submitAddWorkspaceGroupInput,
		)
		return controller.Update(msg)
	case uiModePickWorkspaceRename, uiModePickWorkspaceGroupEdit:
		switch msg := msg.(type) {
		case tea.PasteMsg:
			if m.workspacePicker != nil && m.applyPickerPaste(msg, m.workspacePicker) {
				return true, nil
			}
		case tea.KeyMsg:
			switch msg.String() {
			case "esc":
				if m.workspacePicker != nil && m.workspacePicker.ClearQuery() {
					m.setStatusMessage("filter cleared")
					return true, nil
				}
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
					m.enterEditWorkspace(id)
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
			if m.workspacePicker != nil {
				m.applyPickerTypeAhead(msg, m.workspacePicker)
			}
		}
		return true, nil
	case uiModePickWorkspaceGroupRename, uiModePickWorkspaceGroupDelete, uiModePickWorkspaceGroupAssign:
		switch msg := msg.(type) {
		case tea.PasteMsg:
			if m.groupSelectPicker != nil && m.applyPickerPaste(msg, m.groupSelectPicker) {
				return true, nil
			}
		case tea.KeyMsg:
			switch msg.String() {
			case "esc":
				if m.groupSelectPicker != nil && m.groupSelectPicker.ClearQuery() {
					m.setStatusMessage("filter cleared")
					return true, nil
				}
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
			if m.groupSelectPicker != nil {
				m.applyPickerTypeAhead(msg, m.groupSelectPicker)
			}
		}
		return true, nil
	case uiModeEditWorkspaceGroups:
		h := groupPickerStepHandler{
			picker:          m.groupPicker,
			keys:            m,
			pasteNormalizer: m.pickerPasteNormalizer,
			setStatus:       m.setStatusMessage,
			onCancel:        func() { m.exitEditWorkspaceGroups("edit canceled") },
			onConfirm: func() tea.Cmd {
				if m.groupPicker == nil {
					return nil
				}
				ids := m.groupPicker.SelectedIDs()
				id := m.editWorkspaceID
				if id == "" {
					m.setValidationStatus("no workspace selected")
					return nil
				}
				m.exitEditWorkspaceGroups("saving groups")
				return updateWorkspaceGroupsCmd(m.workspaceAPI, id, ids)
			},
		}
		return h.Update(msg)
	case uiModeRenameWorkspaceGroup:
		if !isTextInputMsg(msg) {
			return true, nil
		}
		controller := m.newSingleLineInputController(
			m.groupInput,
			func() tea.Cmd {
				m.exitRenameWorkspaceGroup("rename canceled")
				return nil
			},
			m.submitRenameWorkspaceGroupInput,
		)
		return controller.Update(msg)
	case uiModeAssignGroupWorkspaces:
		switch msg := msg.(type) {
		case tea.PasteMsg:
			if m.workspaceMulti != nil && m.applyPickerPaste(msg, m.workspaceMulti) {
				return true, nil
			}
		case tea.KeyMsg:
			switch msg.String() {
			case "esc":
				if m.workspaceMulti != nil && m.workspaceMulti.ClearQuery() {
					m.setStatusMessage("filter cleared")
					return true, nil
				}
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
			if m.workspaceMulti != nil {
				m.applyPickerTypeAhead(msg, m.workspaceMulti)
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
	case tea.PasteMsg:
		if m.providerPicker != nil && m.applyPickerPaste(msg, m.providerPicker) {
			return true, nil
		}
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			if m.providerPicker != nil && m.providerPicker.ClearQuery() {
				m.setStatusMessage("filter cleared")
				return true, nil
			}
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
		if m.providerPicker != nil && m.applyPickerTypeAhead(msg, m.providerPicker) {
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
			cmds = append(cmds, m.requestAppStateSaveCmd())
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

func (m *Model) reduceSearchModeKey(msg tea.Msg) (bool, tea.Cmd) {
	if m.mode != uiModeSearch {
		return false, nil
	}
	if !isTextInputMsg(msg) {
		return false, nil
	}
	controller := m.newSingleLineInputController(
		m.searchInput,
		func() tea.Cmd {
			m.exitSearch("search canceled")
			return nil
		},
		m.submitSearchInput,
	)
	handled, cmd := controller.Update(msg)
	if handled && m.consumeInputHeightChanges(m.searchInput) {
		m.resize(m.width, m.height)
	}
	return handled, cmd
}

func (m *Model) reduceApprovalResponseMode(msg tea.Msg) (bool, tea.Cmd) {
	if m.mode != uiModeApprovalResponse {
		return false, nil
	}
	if !isTextInputMsg(msg) {
		return true, nil
	}
	controller := textInputModeController{
		input:             m.approvalInput,
		keyString:         m.keyString,
		keyMatchesCommand: m.keyMatchesCommand,
		onCancel:          m.cancelApprovalResponseInput,
		onSubmit:          m.submitApprovalResponseInput,
	}
	handled, cmd := controller.Update(msg)
	if handled && m.consumeInputHeightChanges(m.approvalInput) {
		m.resize(m.width, m.height)
	}
	return handled, cmd
}

func (m *Model) newSingleLineInputController(input *TextInput, onCancel func() tea.Cmd, onSubmit func(text string) tea.Cmd) textInputModeController {
	return textInputModeController{
		input:             input,
		keyString:         m.keyString,
		keyMatchesCommand: m.keyMatchesCommand,
		onCancel:          onCancel,
		onSubmit:          onSubmit,
	}
}

func (m *Model) submitSearchInput(query string) tea.Cmd {
	m.applySearch(query)
	m.exitSearch("")
	return nil
}

func (m *Model) submitRenameWorktreeInput(name string) tea.Cmd {
	name = strings.TrimSpace(name)
	if name == "" {
		m.setValidationStatus("name is required")
		return nil
	}
	worktreeID := m.renameWorktreeID
	if worktreeID == "" {
		m.setValidationStatus("no worktree selected")
		return nil
	}
	workspaceID := m.renameWorktreeWorkspaceID
	if workspaceID == "" {
		if wt := m.worktreeByID(worktreeID); wt != nil {
			workspaceID = wt.WorkspaceID
		}
	}
	if workspaceID == "" {
		m.setValidationStatus("no worktree selected")
		return nil
	}
	if m.renameInput != nil {
		m.renameInput.SetValue("")
	}
	m.exitRenameWorktree("renaming worktree")
	return updateWorktreeCmd(m.workspaceAPI, workspaceID, worktreeID, name)
}

func (m *Model) submitRenameSessionInput(name string) tea.Cmd {
	name = strings.TrimSpace(name)
	if name == "" {
		m.setValidationStatus("name is required")
		return nil
	}
	id := m.renameSessionID
	if id == "" {
		m.setValidationStatus("no session selected")
		return nil
	}
	if m.renameInput != nil {
		m.renameInput.SetValue("")
	}
	m.exitRenameSession("renaming session")
	return updateSessionCmd(m.sessionAPI, id, name)
}

func (m *Model) submitRenameWorkflowInput(name string) tea.Cmd {
	name = strings.TrimSpace(name)
	if name == "" {
		m.setValidationStatus("name is required")
		return nil
	}
	runID := strings.TrimSpace(m.renameWorkflowRunID)
	if runID == "" {
		m.setValidationStatus("no workflow selected")
		return nil
	}
	if m.renameInput != nil {
		m.renameInput.SetValue("")
	}
	m.exitRenameWorkflow("renaming workflow")
	return renameWorkflowRunCmd(m.guidedWorkflowAPI, runID, name)
}

func (m *Model) submitAddWorkspaceGroupInput(name string) tea.Cmd {
	name = strings.TrimSpace(name)
	if name == "" {
		m.setValidationStatus("name is required")
		return nil
	}
	if m.groupInput != nil {
		m.groupInput.SetValue("")
	}
	m.exitAddWorkspaceGroup("creating group")
	return createWorkspaceGroupCmd(m.workspaceAPI, name)
}

func (m *Model) submitRenameWorkspaceGroupInput(name string) tea.Cmd {
	name = strings.TrimSpace(name)
	if name == "" {
		m.setValidationStatus("name is required")
		return nil
	}
	id := m.renameGroupID
	if id == "" {
		m.setValidationStatus("no group selected")
		return nil
	}
	if m.groupInput != nil {
		m.groupInput.SetValue("")
	}
	m.exitRenameWorkspaceGroup("renaming group")
	return updateWorkspaceGroupCmd(m.workspaceAPI, id, name)
}

func (m *Model) reduceComposeInputKey(msg tea.Msg) (bool, tea.Cmd) {
	if m.input == nil || !m.input.IsChatFocused() {
		return false, nil
	}
	if !isTextInputMsg(msg) {
		return false, nil
	}
	if pasteMsg, ok := msg.(tea.PasteMsg); ok && m.composeOptionPickerOpen() {
		composePicker := composeOptionQueryPicker{model: m}
		m.applyPickerPaste(pasteMsg, composePicker)
		return true, nil
	}
	controller := textInputModeController{
		input:             m.chatInput,
		keyString:         m.keyString,
		keyMatchesCommand: m.keyMatchesCommand,
		onCancel:          m.cancelComposeInput,
		onSubmit:          m.submitComposeInput,
		onClear: func() tea.Cmd {
			m.setStatusMessage("input cleared")
			return nil
		},
		beforeInputUpdate: m.resetComposeHistoryCursor,
		shouldPassthrough: m.shouldComposePassthrough,
		preHandle: func(key string, msg tea.KeyMsg) (bool, tea.Cmd) {
			if m.composeOptionPickerOpen() {
				switch msg.String() {
				case "esc":
					if m.composeOptionPickerClearQuery() {
						m.setStatusMessage("session option filter cleared")
						return true, nil
					}
					m.closeComposeOptionPicker()
					m.setStatusMessage("session options picker closed")
					return true, nil
				case "enter":
					value := m.composeOptionPickerSelectedID()
					cmd := m.applyComposeOptionSelection(value)
					m.closeComposeOptionPicker()
					return true, cmd
				case "j", "down":
					m.moveComposeOptionPicker(1)
					return true, nil
				case "k", "up":
					m.moveComposeOptionPicker(-1)
					return true, nil
				}
				composePicker := composeOptionQueryPicker{model: m}
				if m.applyPickerTypeAhead(msg, composePicker) {
					return true, nil
				}
				if pickerTypeAheadText(msg) != "" {
					// Consume plain text input while picker is open, even if query does not change.
					return true, nil
				}
			}
			if m.keyMatchesCommand(msg, KeyCommandCopySessionID, "ctrl+g") {
				id := m.selectedSessionID()
				if id == "" {
					m.setCopyStatusWarning("no session selected")
					return true, nil
				}
				return true, m.copyWithStatusCmd(id, "copied session id")
			}
			if m.keyMatchesCommand(msg, KeyCommandToggleDebugStreams, "ctrl+d") {
				return true, m.toggleDebugStreams()
			}
			if m.keyMatchesOverriddenCommand(msg, KeyCommandNotesNew, "n") {
				return true, m.enterAddNoteForSelection()
			}
			switch key {
			case "ctrl+o":
				return true, m.toggleNotesPanel()
			case "ctrl+1":
				return true, m.requestComposeOptionPicker(composeOptionModel)
			case "ctrl+2":
				return true, m.requestComposeOptionPicker(composeOptionReasoning)
			case "ctrl+3":
				return true, m.requestComposeOptionPicker(composeOptionAccess)
			case "ctrl+up":
				if m.chatInput != nil {
					if value, ok := m.composeHistoryNavigate(-1, m.chatInput.Value()); ok {
						m.chatInput.SetValue(value)
						return true, nil
					}
				}
				return true, nil
			case "ctrl+down":
				if m.chatInput != nil {
					if value, ok := m.composeHistoryNavigate(1, m.chatInput.Value()); ok {
						m.chatInput.SetValue(value)
						return true, nil
					}
				}
				return true, nil
			}
			return false, nil
		},
	}
	handled, cmd := controller.Update(msg)
	if handled && m.consumeInputHeightChanges(m.chatInput) {
		m.resize(m.width, m.height)
	}
	return handled, cmd
}

func isTextInputMsg(msg tea.Msg) bool {
	switch msg.(type) {
	case tea.KeyMsg, tea.PasteMsg:
		return true
	default:
		return false
	}
}

func (m *Model) shouldComposePassthrough(msg tea.KeyMsg) bool {
	key := msg.Key()
	if !key.Mod.Contains(tea.ModCtrl) && !key.Mod.Contains(tea.ModSuper) {
		return false
	}
	if m.keybindings == nil {
		return false
	}
	keyStr := strings.TrimSpace(msg.String())
	bindings := m.keybindings.Bindings()
	matchesNonInput := false
	for command, bound := range bindings {
		if bound != keyStr {
			continue
		}
		if isComposeInputCommand(command) {
			return false
		}
		matchesNonInput = true
	}
	if !matchesNonInput {
		return false
	}
	m.exitCompose("")
	return true
}

func isComposeInputCommand(command string) bool {
	switch command {
	case KeyCommandInputSubmit, KeyCommandInputNewline, KeyCommandInputClear,
		KeyCommandInputSelectAll, KeyCommandInputUndo, KeyCommandInputRedo,
		KeyCommandInputLineUp, KeyCommandInputLineDown,
		KeyCommandInputWordLeft, KeyCommandInputWordRight,
		KeyCommandInputDeleteWordLeft, KeyCommandInputDeleteWordRight,
		KeyCommandComposeModel, KeyCommandComposeReasoning, KeyCommandComposeAccess,
		KeyCommandCopySessionID,
		KeyCommandToggleNotesPanel, KeyCommandToggleDebugStreams:
		return true
	default:
		return false
	}
}

func (m *Model) cancelComposeInput() tea.Cmd {
	m.closeComposeOptionPicker()
	m.exitCompose("compose canceled")
	return m.requestAppStateSaveCmd()
}

func (m *Model) submitComposeInput(text string) tea.Cmd {
	if strings.TrimSpace(text) == "" {
		m.setValidationStatus("message is required")
		return nil
	}
	if m.newSession != nil {
		target := m.newSession
		if strings.TrimSpace(target.provider) == "" {
			m.setValidationStatus("provider is required")
			return nil
		}
		m.resetStream()
		m.setContentText("Starting new session...")
		m.enableFollow(false)
		m.setStatusMessage("starting session")
		if m.chatInput != nil {
			m.chatInput.Clear()
		}
		return m.startWorkspaceSessionCmd(target.workspaceID, target.worktreeID, target.provider, text, target.runtimeOptions)
	}
	sessionID := m.composeSessionID()
	if sessionID == "" {
		m.setValidationStatus("select a session to chat")
		return nil
	}
	m.clearComposeDraft(sessionID)
	m.recordComposeHistory(sessionID, text)
	saveHistoryCmd := m.requestAppStateSaveCmd()
	provider := m.providerForSessionID(sessionID)
	m.enableFollow(false)
	m.startRequestActivity(sessionID, provider)
	token := m.nextSendToken()
	m.registerPendingSend(token, sessionID, provider)
	headerIndex := m.appendUserMessageLocal(provider, text)
	m.setStatusMessage("sending message")
	if m.chatInput != nil {
		m.chatInput.Clear()
	}
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
		return tea.Batch(cmds...)
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
		return tea.Batch(cmds...)
	}
	if saveHistoryCmd != nil {
		return tea.Batch(send, saveHistoryCmd)
	}
	return send
}
