package app

import (
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

type mouseLayout struct {
	listWidth  int
	barWidth   int
	barStart   int
	rightStart int
}

func (m *Model) resolveMouseLayout() mouseLayout {
	layout := mouseLayout{}
	if !m.appState.SidebarCollapsed {
		layout.listWidth = clamp(m.width/3, minListWidth, maxListWidth)
		if m.width-layout.listWidth-1 < minViewportWidth {
			layout.listWidth = max(minListWidth, m.width/2)
		}
	}
	if m.sidebar != nil {
		layout.barWidth = m.sidebar.ScrollbarWidth()
	}
	layout.barStart = layout.listWidth - layout.barWidth
	if layout.barStart < 0 {
		layout.barStart = 0
	}
	if layout.listWidth > 0 {
		layout.rightStart = layout.listWidth + 1
	}
	return layout
}

func (m *Model) mouseOverInput(y int) bool {
	if m.mode == uiModeCompose && m.chatInput != nil {
		start := m.viewport.Height + 2
		end := start + m.chatInput.Height() - 1
		return y >= start && y <= end
	}
	if m.mode == uiModeSearch && m.searchInput != nil {
		start := m.viewport.Height + 2
		end := start + m.searchInput.Height() - 1
		return y >= start && y <= end
	}
	return false
}

func (m *Model) reduceContextMenuLeftPressMouse(msg tea.MouseMsg) bool {
	if m.contextMenu == nil || !m.contextMenu.IsOpen() {
		return false
	}
	if msg.Action != tea.MouseActionPress || msg.Button != tea.MouseButtonLeft {
		return false
	}
	if handled, action := m.contextMenu.HandleMouse(msg, m.width, m.height-1); handled {
		if action != ContextMenuNone {
			if cmd := m.handleContextMenuAction(action); cmd != nil {
				m.pendingMouseCmd = cmd
			}
		}
		return true
	}
	if !m.contextMenu.Contains(msg.X, msg.Y, m.width, m.height-1) {
		m.contextMenu.Close()
	}
	return false
}

func (m *Model) reduceContextMenuRightPressMouse(msg tea.MouseMsg, layout mouseLayout) bool {
	if msg.Action != tea.MouseActionPress || msg.Button != tea.MouseButtonRight {
		return false
	}
	if layout.listWidth > 0 && msg.X < layout.listWidth && m.sidebar != nil {
		if entry := m.sidebar.ItemAtRow(msg.Y); entry != nil {
			if m.menu != nil {
				m.menu.CloseAll()
			}
			if m.contextMenu != nil {
				switch entry.kind {
				case sidebarWorkspace:
					if entry.workspace != nil {
						m.contextMenu.OpenWorkspace(entry.workspace.ID, entry.workspace.Name, msg.X, msg.Y)
						return true
					}
				case sidebarWorktree:
					if entry.worktree != nil {
						m.contextMenu.OpenWorktree(entry.worktree.ID, entry.worktree.WorkspaceID, entry.worktree.Name, msg.X, msg.Y)
						return true
					}
				case sidebarSession:
					if entry.session != nil {
						m.contextMenu.OpenSession(entry.session.ID, entry.Title(), msg.X, msg.Y)
						return true
					}
				}
			}
		}
	}
	if m.contextMenu != nil && m.contextMenu.IsOpen() {
		m.contextMenu.Close()
		return true
	}
	return false
}

func (m *Model) reduceSidebarDragMouse(msg tea.MouseMsg, layout mouseLayout) bool {
	if msg.Action == tea.MouseActionRelease {
		m.sidebarDragging = false
	}
	if msg.Action == tea.MouseActionMotion && m.sidebarDragging {
		if layout.listWidth > 0 && msg.X < layout.listWidth && layout.barWidth > 0 && msg.X >= layout.barStart {
			if m.sidebar != nil && m.sidebar.ScrollbarSelect(msg.Y) {
				m.lastSidebarWheelAt = time.Now()
				m.pendingSidebarWheel = true
				return true
			}
		}
		return true
	}
	return false
}

func (m *Model) reduceMouseWheel(msg tea.MouseMsg, layout mouseLayout, delta int) bool {
	if layout.listWidth > 0 && msg.X < layout.listWidth {
		now := time.Now()
		if now.Sub(m.lastSidebarWheelAt) < sidebarWheelCooldown {
			return true
		}
		m.lastSidebarWheelAt = now
		if m.sidebar != nil && m.sidebar.Scroll(delta) {
			m.pendingSidebarWheel = true
			return true
		}
	}
	if m.reduceModeWheelMouse(msg, layout, delta) {
		return true
	}
	if delta < 0 {
		m.viewport.LineUp(3)
	} else {
		m.viewport.LineDown(3)
	}
	if m.follow {
		m.follow = false
		m.status = "follow: paused"
	}
	return true
}

func (m *Model) reduceModeWheelMouse(msg tea.MouseMsg, layout mouseLayout, delta int) bool {
	if m.mode == uiModePickProvider && msg.X >= layout.rightStart {
		if m.providerPicker != nil && m.providerPicker.Scroll(delta) {
			return true
		}
	}
	if msg.X >= layout.rightStart && m.mouseOverInput(msg.Y) {
		if m.mode == uiModeCompose && m.chatInput != nil {
			m.pendingMouseCmd = m.chatInput.Scroll(delta)
			return true
		}
		if m.mode == uiModeSearch && m.searchInput != nil {
			m.pendingMouseCmd = m.searchInput.Scroll(delta)
			return true
		}
	}
	if m.mode == uiModeAddWorktree && m.addWorktree != nil {
		if m.addWorktree.Scroll(delta) {
			return true
		}
	}
	if m.mode == uiModeEditWorkspaceGroups && m.groupPicker != nil && msg.X >= layout.rightStart {
		if m.groupPicker.Move(delta) {
			return true
		}
	}
	if (m.mode == uiModePickWorkspaceRename || m.mode == uiModePickWorkspaceGroupEdit) && m.workspacePicker != nil && msg.X >= layout.rightStart {
		if m.workspacePicker.Move(delta) {
			return true
		}
	}
	if m.mode == uiModePickWorkspaceGroupRename && m.groupSelectPicker != nil && msg.X >= layout.rightStart {
		if m.groupSelectPicker.Move(delta) {
			return true
		}
	}
	if m.mode == uiModePickWorkspaceGroupDelete && m.groupSelectPicker != nil && msg.X >= layout.rightStart {
		if m.groupSelectPicker.Move(delta) {
			return true
		}
	}
	if m.mode == uiModePickWorkspaceGroupAssign && m.groupSelectPicker != nil && msg.X >= layout.rightStart {
		if m.groupSelectPicker.Move(delta) {
			return true
		}
	}
	if m.mode == uiModeAssignGroupWorkspaces && m.workspaceMulti != nil && msg.X >= layout.rightStart {
		if m.workspaceMulti.Move(delta) {
			return true
		}
	}
	return false
}

func (m *Model) reduceMenuLeftPressMouse(msg tea.MouseMsg) bool {
	if m.menu == nil {
		return false
	}
	menuWidth := m.sidebarWidth()
	if menuWidth <= 0 {
		menuWidth = max(minListWidth, minViewportWidth)
	}
	if handled, action := m.menu.HandleMouse(msg, menuWidth); handled {
		if cmd := m.handleMenuAction(action); cmd != nil {
			m.pendingMouseCmd = cmd
		}
		return true
	}
	return false
}

func (m *Model) reduceSidebarScrollbarLeftPressMouse(msg tea.MouseMsg, layout mouseLayout) bool {
	if layout.listWidth > 0 && msg.X < layout.listWidth && layout.barWidth > 0 && msg.X >= layout.barStart {
		if m.sidebar != nil && m.sidebar.ScrollbarSelect(msg.Y) {
			m.lastSidebarWheelAt = time.Now()
			m.pendingSidebarWheel = true
			m.sidebarDragging = true
			return true
		}
	}
	return false
}

func (m *Model) reduceInputFocusLeftPressMouse(msg tea.MouseMsg, layout mouseLayout) bool {
	if msg.X < layout.rightStart || !m.mouseOverInput(msg.Y) {
		return false
	}
	if m.mode == uiModeCompose && m.chatInput != nil {
		m.chatInput.Focus()
		if m.input != nil {
			m.input.FocusChatInput()
		}
		return true
	}
	if m.mode == uiModeSearch && m.searchInput != nil {
		m.searchInput.Focus()
		if m.input != nil {
			m.input.FocusChatInput()
		}
		return true
	}
	return false
}

func (m *Model) reduceReasoningToggleLeftPressMouse(msg tea.MouseMsg, layout mouseLayout) bool {
	if msg.X < layout.rightStart {
		return false
	}
	if m.mode != uiModeNormal && m.mode != uiModeCompose {
		return false
	}
	if msg.Y < 1 || msg.Y > m.viewport.Height || m.mouseOverInput(msg.Y) {
		return false
	}
	return m.toggleReasoningByViewportLine(msg.Y - 1)
}

func (m *Model) reduceTranscriptCopyLeftPressMouse(msg tea.MouseMsg, layout mouseLayout) bool {
	if msg.X < layout.rightStart {
		return false
	}
	if m.mode != uiModeNormal && m.mode != uiModeCompose {
		return false
	}
	if msg.Y < 1 || msg.Y > m.viewport.Height {
		return false
	}
	return m.copyBlockByViewportPosition(msg.X-layout.rightStart, msg.Y-1)
}

func (m *Model) reduceModePickersLeftPressMouse(msg tea.MouseMsg, layout mouseLayout) bool {
	if m.mode == uiModePickProvider && m.providerPicker != nil {
		if msg.X >= layout.rightStart {
			row := msg.Y - 1
			if row >= 0 && m.providerPicker.SelectByRow(row) {
				m.pendingMouseCmd = m.selectProvider()
				return true
			}
		}
	}
	if m.mode == uiModeEditWorkspaceGroups && m.groupPicker != nil {
		if msg.X >= layout.rightStart {
			row := msg.Y - 1
			if row >= 0 && m.groupPicker.HandleClick(row) {
				return true
			}
		}
	}
	if (m.mode == uiModePickWorkspaceRename || m.mode == uiModePickWorkspaceGroupEdit) && m.workspacePicker != nil {
		if msg.X >= layout.rightStart {
			row := msg.Y - 1
			if row >= 0 && m.workspacePicker.HandleClick(row) {
				id := m.workspacePicker.SelectedID()
				if id == "" {
					return true
				}
				if m.mode == uiModePickWorkspaceRename {
					m.enterRenameWorkspace(id)
				} else {
					m.enterEditWorkspaceGroups(id)
				}
				return true
			}
		}
	}
	if m.mode == uiModePickWorkspaceGroupRename && m.groupSelectPicker != nil {
		if msg.X >= layout.rightStart {
			row := msg.Y - 1
			if row >= 0 && m.groupSelectPicker.HandleClick(row) {
				id := m.groupSelectPicker.SelectedID()
				if id == "" {
					return true
				}
				m.enterRenameWorkspaceGroup(id)
				return true
			}
		}
	}
	if m.mode == uiModePickWorkspaceGroupDelete && m.groupSelectPicker != nil {
		if msg.X >= layout.rightStart {
			row := msg.Y - 1
			if row >= 0 && m.groupSelectPicker.HandleClick(row) {
				id := m.groupSelectPicker.SelectedID()
				if id == "" {
					return true
				}
				m.confirmDeleteWorkspaceGroup(id)
				return true
			}
		}
	}
	if m.mode == uiModePickWorkspaceGroupAssign && m.groupSelectPicker != nil {
		if msg.X >= layout.rightStart {
			row := msg.Y - 1
			if row >= 0 && m.groupSelectPicker.HandleClick(row) {
				id := m.groupSelectPicker.SelectedID()
				if id == "" {
					return true
				}
				m.enterAssignGroupWorkspaces(id)
				return true
			}
		}
	}
	if m.mode == uiModeAssignGroupWorkspaces && m.workspaceMulti != nil {
		if msg.X >= layout.rightStart {
			row := msg.Y - 1
			if row >= 0 && m.workspaceMulti.HandleClick(row) {
				return true
			}
		}
	}
	if m.mode == uiModeAddWorktree && m.addWorktree != nil {
		if msg.X >= layout.rightStart {
			row := msg.Y - 1
			if row >= 0 {
				if handled, cmd := m.addWorktree.HandleClick(row, m); handled {
					m.pendingMouseCmd = cmd
					return true
				}
			}
		}
	}
	return false
}

func (m *Model) reduceSidebarSelectionLeftPressMouse(msg tea.MouseMsg, layout mouseLayout) bool {
	if layout.listWidth > 0 && msg.X < layout.listWidth {
		if m.sidebar != nil {
			m.sidebar.SelectByRow(msg.Y)
			m.pendingMouseCmd = m.onSelectionChanged()
			return true
		}
	}
	return false
}
