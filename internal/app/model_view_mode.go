package app

func (m *Model) modeViewContent() (headerText, bodyText string) {
	headerText = "Tail"
	bodyText = m.viewport.View()
	switch m.mode {
	case uiModeNotes:
		headerText = "Notes"
	case uiModeAddNote:
		headerText = "Add Note"
	case uiModeAddWorkspace:
		headerText = "Add Workspace"
		if m.addWorkspace != nil {
			bodyText = m.addWorkspace.View()
		}
	case uiModeAddWorkspaceGroup:
		headerText = "Add Workspace Group"
		if m.groupInput != nil {
			bodyText = m.groupInput.View()
		}
	case uiModePickWorkspaceRename, uiModePickWorkspaceGroupEdit:
		headerText = "Select Workspace"
		if m.workspacePicker != nil {
			bodyText = m.workspacePicker.View()
		}
	case uiModePickWorkspaceGroupRename, uiModePickWorkspaceGroupDelete, uiModePickWorkspaceGroupAssign:
		headerText = "Select Group"
		if m.groupSelectPicker != nil {
			bodyText = m.groupSelectPicker.View()
		}
	case uiModePickNoteMoveTarget:
		headerText = "Move Note"
		if m.groupSelectPicker != nil {
			bodyText = m.groupSelectPicker.View()
		}
	case uiModePickNoteMoveWorktree:
		headerText = "Select Worktree"
		if m.groupSelectPicker != nil {
			bodyText = m.groupSelectPicker.View()
		}
	case uiModePickNoteMoveSession:
		headerText = "Select Session"
		if m.groupSelectPicker != nil {
			bodyText = m.groupSelectPicker.View()
		}
	case uiModeEditWorkspaceGroups:
		headerText = "Edit Workspace Groups"
		if m.groupPicker != nil {
			bodyText = m.groupPicker.View()
		}
	case uiModeRenameWorkspaceGroup:
		headerText = "Rename Workspace Group"
		if m.groupInput != nil {
			bodyText = m.groupInput.View()
		}
	case uiModeAssignGroupWorkspaces:
		headerText = "Assign Workspaces"
		if m.workspaceMulti != nil {
			bodyText = m.workspaceMulti.View()
		}
	case uiModeAddWorktree:
		headerText = "Add Worktree"
		if m.addWorktree != nil {
			bodyText = m.addWorktree.View()
		}
	case uiModePickProvider:
		headerText = "Provider"
		if m.providerPicker != nil {
			bodyText = m.providerPicker.View()
		}
	case uiModeCompose:
		headerText = "Chat"
	case uiModeRecents:
		headerText = m.recentsHeader()
	case uiModeApprovalResponse:
		headerText = "Approval Response"
		bodyText = m.approvalResponseBody()
	case uiModeSearch:
		headerText = "Search"
	case uiModeRenameWorkspace:
		headerText = "Rename Workspace"
		if m.renameInput != nil {
			bodyText = m.renameInput.View()
		}
	case uiModeRenameWorktree:
		headerText = "Rename Worktree"
		if m.renameInput != nil {
			bodyText = m.renameInput.View()
		}
	case uiModeRenameSession:
		headerText = "Rename Session"
		if m.renameInput != nil {
			bodyText = m.renameInput.View()
		}
	case uiModeRenameWorkflow:
		headerText = "Rename Workflow"
		if m.renameInput != nil {
			bodyText = m.renameInput.View()
		}
	case uiModeGuidedWorkflow:
		headerText = "Guided Workflow"
	}
	return headerText, bodyText
}

func (m *Model) modeInputView() (line string, scrollable bool) {
	layout, ok := visibleInputPanelLayout(m.modeInputPanelLayout())
	if !ok {
		return "", false
	}
	return layout.View()
}

func (m *Model) modeInputLineCount() int {
	layout, ok := visibleInputPanelLayout(m.modeInputPanelLayout())
	if !ok {
		return 0
	}
	return layout.LineCount()
}

func visibleInputPanelLayout(layout InputPanelLayout, ok bool) (InputPanelLayout, bool) {
	if !ok {
		return InputPanelLayout{}, false
	}
	line, _ := layout.View()
	if line == "" {
		return InputPanelLayout{}, false
	}
	return layout, true
}

func (m *Model) modeInputPanel() (InputPanel, bool) {
	if panel, ok := m.activeInputPanel(); ok {
		return panel, true
	}
	if panel, ok := m.guidedWorkflowSetupInputPanel(); ok {
		return panel, true
	}
	if panel, ok := m.guidedWorkflowResumeInputPanel(); ok {
		return panel, true
	}
	return InputPanel{}, false
}

func (m *Model) modeInputPanelLayout() (InputPanelLayout, bool) {
	panel, ok := m.modeInputPanel()
	if !ok {
		return InputPanelLayout{}, false
	}
	return BuildInputPanelLayout(panel), true
}
