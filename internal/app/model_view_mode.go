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
	}
	return headerText, bodyText
}

func (m *Model) modeInputView() (line string, scrollable bool) {
	switch m.mode {
	case uiModeCompose:
		return InputPanel{
			Input:  m.chatInput,
			Footer: InputFooterFunc(m.composeControlsLine),
		}.View()
	case uiModeAddNote:
		return InputPanel{Input: m.noteInput}.View()
	case uiModeSearch:
		return InputPanel{Input: m.searchInput}.View()
	}
	return "", false
}
