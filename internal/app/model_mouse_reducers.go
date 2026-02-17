package app

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

type mouseLayout struct {
	listWidth    int
	barWidth     int
	barStart     int
	rightStart   int
	panelVisible bool
	panelStart   int
	panelWidth   int
}

func (m *Model) resolveMouseLayout() mouseLayout {
	frame := m.layoutFrame()
	layout := mouseLayout{
		listWidth:    frame.sidebarWidth,
		rightStart:   frame.rightStart,
		panelVisible: frame.panelVisible,
		panelStart:   frame.panelStart,
		panelWidth:   frame.panelWidth,
	}
	if m.sidebar != nil {
		layout.barWidth = m.sidebar.ScrollbarWidth()
	}
	layout.barStart = layout.listWidth - layout.barWidth
	if layout.barStart < 0 {
		layout.barStart = 0
	}
	return layout
}

func (m *Model) mouseOverInput(y int) bool {
	if m.mode == uiModeCompose && m.chatInput != nil {
		start := m.viewport.Height() + 2
		end := start + m.chatInput.Height() - 1
		return y >= start && y <= end
	}
	if m.mode == uiModeAddNote && m.noteInput != nil {
		start := m.viewport.Height() + 2
		end := start + m.noteInput.Height() - 1
		return y >= start && y <= end
	}
	if m.mode == uiModeSearch && m.searchInput != nil {
		start := m.viewport.Height() + 2
		end := start + m.searchInput.Height() - 1
		return y >= start && y <= end
	}
	return false
}

func (m *Model) mouseOverComposeControls(y int) bool {
	if m.mode != uiModeCompose || m.chatInput == nil {
		return false
	}
	return y == m.composeControlsRow()
}

func isMouseClickMsg(msg tea.MouseMsg) bool {
	_, ok := msg.(tea.MouseClickMsg)
	return ok
}

func (m *Model) reduceComposeOptionPickerLeftPressMouse(msg tea.MouseMsg, layout mouseLayout) bool {
	if !isMouseClickMsg(msg) {
		return false
	}
	if !m.composeOptionPickerOpen() {
		return false
	}
	popup, row := m.composeOptionPopupView()
	if popup == "" {
		m.closeComposeOptionPicker()
		return false
	}
	height := len(strings.Split(popup, "\n"))
	mouse := msg.Mouse()
	if mouse.X >= layout.rightStart {
		if pickerRow, ok := composePickerRowForClick(mouse.Y, row, height); ok {
			if m.composeOptionPickerHandleClick(pickerRow) {
				cmd := m.applyComposeOptionSelection(m.composeOptionPickerSelectedID())
				m.closeComposeOptionPicker()
				if cmd != nil {
					m.pendingMouseCmd = cmd
				}
				return true
			}
			return true
		}
	}
	m.closeComposeOptionPicker()
	return true
}

func composePickerRowForClick(y, row, height int) (int, bool) {
	if height <= 0 {
		return 0, false
	}
	rel := y - row
	if rel >= 0 && rel < height {
		return rel, true
	}
	// Bubble Tea mouse coordinates can occasionally land one row above/below
	// when overlays are rendered near the bottom input area.
	if rel == -1 {
		return 0, true
	}
	if rel == height {
		return height - 1, true
	}
	return 0, false
}

func (m *Model) reduceComposeControlsLeftPressMouse(msg tea.MouseMsg, layout mouseLayout) bool {
	if !isMouseClickMsg(msg) {
		return false
	}
	mouse := msg.Mouse()
	if m.mode != uiModeCompose || mouse.X < layout.rightStart || !m.mouseOverComposeControls(mouse.Y) {
		return false
	}
	col := mouse.X - layout.rightStart
	for _, span := range m.composeControlSpans() {
		if col < span.start || col > span.end {
			continue
		}
		if cmd := m.requestComposeOptionPicker(span.kind); cmd != nil {
			m.pendingMouseCmd = cmd
		}
		if m.input != nil {
			m.input.FocusChatInput()
		}
		return true
	}
	return false
}

func (m *Model) reduceContextMenuLeftPressMouse(msg tea.MouseMsg) bool {
	if !isMouseClickMsg(msg) {
		return false
	}
	if m.contextMenu == nil || !m.contextMenu.IsOpen() {
		return false
	}
	mouse := msg.Mouse()
	if mouse.Button != tea.MouseLeft {
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
	if !m.contextMenu.Contains(mouse.X, mouse.Y, m.width, m.height-1) {
		m.contextMenu.Close()
	}
	return false
}

func (m *Model) reduceContextMenuRightPressMouse(msg tea.MouseMsg, layout mouseLayout) bool {
	if !isMouseClickMsg(msg) {
		return false
	}
	mouse := msg.Mouse()
	if mouse.Button != tea.MouseRight {
		return false
	}
	if layout.listWidth > 0 && mouse.X < layout.listWidth && m.sidebar != nil {
		if entry := m.sidebar.ItemAtRow(mouse.Y); entry != nil {
			if m.menu != nil {
				m.menu.CloseAll()
			}
			if m.contextMenu != nil {
				switch entry.kind {
				case sidebarWorkspace:
					if entry.workspace != nil {
						m.contextMenu.OpenWorkspace(entry.workspace.ID, entry.workspace.Name, mouse.X, mouse.Y)
						return true
					}
				case sidebarWorktree:
					if entry.worktree != nil {
						m.contextMenu.OpenWorktree(entry.worktree.ID, entry.worktree.WorkspaceID, entry.worktree.Name, mouse.X, mouse.Y)
						return true
					}
				case sidebarSession:
					if entry.session != nil {
						workspaceID := ""
						worktreeID := ""
						if entry.meta != nil {
							workspaceID = entry.meta.WorkspaceID
							worktreeID = entry.meta.WorktreeID
						}
						m.contextMenu.OpenSession(entry.session.ID, workspaceID, worktreeID, entry.Title(), mouse.X, mouse.Y)
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
	mouse := msg.Mouse()
	switch msg.(type) {
	case tea.MouseReleaseMsg:
		m.sidebarDragging = false
	case tea.MouseMotionMsg:
		if m.sidebarDragging {
			if layout.listWidth > 0 && mouse.X < layout.listWidth && layout.barWidth > 0 && mouse.X >= layout.barStart {
				if m.sidebar != nil && m.sidebar.ScrollbarSelect(mouse.Y) {
					m.lastSidebarWheelAt = time.Now()
					m.pendingSidebarWheel = true
					return true
				}
			}
			return true
		}
	}
	return false
}

func (m *Model) reduceMouseWheel(msg tea.MouseMsg, layout mouseLayout, delta int) bool {
	mouse := msg.Mouse()
	if layout.listWidth > 0 && layout.barWidth > 0 && mouse.X < layout.listWidth {
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
	if m.reduceNotesPanelWheelMouse(msg, layout, delta) {
		return true
	}
	if m.reduceModeWheelMouse(msg, layout, delta) {
		return true
	}
	wasFollowing := m.follow
	if delta < 0 {
		m.pauseFollow(true)
		m.viewport.ScrollUp(3)
	} else {
		m.viewport.ScrollDown(3)
	}
	m.maybeResumeFollowAfterManualScroll(wasFollowing, delta > 0)
	return true
}

func (m *Model) reduceModeWheelMouse(msg tea.MouseMsg, layout mouseLayout, delta int) bool {
	mouse := msg.Mouse()
	if m.mode == uiModePickProvider && mouse.X >= layout.rightStart {
		if m.providerPicker != nil && m.providerPicker.Scroll(delta) {
			return true
		}
	}
	if mouse.X >= layout.rightStart && m.mouseOverInput(mouse.Y) {
		if m.mode == uiModeCompose && m.chatInput != nil {
			m.pendingMouseCmd = m.chatInput.Scroll(delta)
			return true
		}
		if m.mode == uiModeAddNote && m.noteInput != nil {
			m.pendingMouseCmd = m.noteInput.Scroll(delta)
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
	if m.mode == uiModeEditWorkspaceGroups && m.groupPicker != nil && mouse.X >= layout.rightStart {
		if m.groupPicker.Move(delta) {
			return true
		}
	}
	if m.noteMovePickerModeActive() && m.groupSelectPicker != nil && mouse.X >= layout.rightStart {
		if m.groupSelectPicker.Move(delta) {
			return true
		}
	}
	if (m.mode == uiModePickWorkspaceRename || m.mode == uiModePickWorkspaceGroupEdit) && m.workspacePicker != nil && mouse.X >= layout.rightStart {
		if m.workspacePicker.Move(delta) {
			return true
		}
	}
	if m.mode == uiModePickWorkspaceGroupRename && m.groupSelectPicker != nil && mouse.X >= layout.rightStart {
		if m.groupSelectPicker.Move(delta) {
			return true
		}
	}
	if m.mode == uiModePickWorkspaceGroupDelete && m.groupSelectPicker != nil && mouse.X >= layout.rightStart {
		if m.groupSelectPicker.Move(delta) {
			return true
		}
	}
	if m.mode == uiModePickWorkspaceGroupAssign && m.groupSelectPicker != nil && mouse.X >= layout.rightStart {
		if m.groupSelectPicker.Move(delta) {
			return true
		}
	}
	if m.mode == uiModeAssignGroupWorkspaces && m.workspaceMulti != nil && mouse.X >= layout.rightStart {
		if m.workspaceMulti.Move(delta) {
			return true
		}
	}
	return false
}

func (m *Model) reduceMenuLeftPressMouse(msg tea.MouseMsg) bool {
	if !isMouseClickMsg(msg) {
		return false
	}
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
	if !isMouseClickMsg(msg) {
		return false
	}
	mouse := msg.Mouse()
	if layout.listWidth > 0 && mouse.X < layout.listWidth && layout.barWidth > 0 && mouse.X >= layout.barStart {
		if m.sidebar != nil && m.sidebar.ScrollbarSelect(mouse.Y) {
			m.lastSidebarWheelAt = time.Now()
			m.pendingSidebarWheel = true
			m.sidebarDragging = true
			return true
		}
	}
	return false
}

func (m *Model) reduceInputFocusLeftPressMouse(msg tea.MouseMsg, layout mouseLayout) bool {
	if !isMouseClickMsg(msg) {
		return false
	}
	mouse := msg.Mouse()
	if mouse.X < layout.rightStart || !m.mouseOverInput(mouse.Y) {
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

func (m *Model) reduceTranscriptReasoningButtonLeftPressMouse(msg tea.MouseMsg, layout mouseLayout) bool {
	if !isMouseClickMsg(msg) {
		return false
	}
	mouse := msg.Mouse()
	if mouse.X < layout.rightStart {
		return false
	}
	if m.mode != uiModeNormal && m.mode != uiModeCompose {
		return false
	}
	if mouse.Y < 1 || mouse.Y > m.viewport.Height() || m.mouseOverInput(mouse.Y) {
		return false
	}
	return m.toggleReasoningByViewportPosition(mouse.X-layout.rightStart, mouse.Y-1)
}

func (m *Model) reduceTranscriptApprovalButtonLeftPressMouse(msg tea.MouseMsg, layout mouseLayout) bool {
	if !isMouseClickMsg(msg) {
		return false
	}
	mouse := msg.Mouse()
	if mouse.X < layout.rightStart {
		return false
	}
	if m.mode != uiModeNormal && m.mode != uiModeCompose {
		return false
	}
	if mouse.Y < 1 || mouse.Y > m.viewport.Height() || m.mouseOverInput(mouse.Y) {
		return false
	}
	col := mouse.X - layout.rightStart
	absolute := m.viewport.YOffset() + mouse.Y - 1
	for _, span := range m.contentBlockSpans {
		if span.Role != ChatRoleApproval {
			continue
		}
		decision := ""
		if span.ApproveLine == absolute && span.ApproveStart >= 0 && col >= span.ApproveStart && col <= span.ApproveEnd {
			decision = "accept"
		}
		if span.DeclineLine == absolute && span.DeclineStart >= 0 && col >= span.DeclineStart && col <= span.DeclineEnd {
			decision = "decline"
		}
		if decision == "" {
			continue
		}
		if span.BlockIndex < 0 || span.BlockIndex >= len(m.contentBlocks) {
			return true
		}
		requestID, ok := approvalRequestIDFromBlock(m.contentBlocks[span.BlockIndex])
		if !ok {
			m.setValidationStatus("invalid approval request")
			return true
		}
		sessionID := approvalSessionIDFromBlock(m.contentBlocks[span.BlockIndex])
		if cmd := m.approveRequestForSession(sessionID, decision, requestID); cmd != nil {
			m.pendingMouseCmd = cmd
		}
		return true
	}
	return false
}

func (m *Model) reduceGlobalStatusCopyLeftPressMouse(msg tea.MouseMsg) bool {
	if !isMouseClickMsg(msg) {
		return false
	}
	mouse := msg.Mouse()
	if mouse.Button != tea.MouseLeft {
		return false
	}
	if !m.isStatusLineMouseRow(mouse.Y) {
		return false
	}
	start, end, ok := m.statusLineStatusHitbox()
	if !ok {
		return false
	}
	if !isStatusLineMouseCol(mouse.X, start, end) {
		return false
	}
	text := strings.TrimSpace(m.status)
	if text == "" {
		m.setCopyStatusWarning("nothing to copy")
		return true
	}
	if cmd := m.copyWithStatusCmd(text, "status copied"); cmd != nil {
		m.pendingMouseCmd = cmd
	}
	return true
}

func (m *Model) isStatusLineMouseRow(y int) bool {
	if m.height <= 0 {
		return false
	}
	if y == m.height-1 || y == m.height {
		return true
	}
	bodyHeight := m.renderedBodyHeight()
	if bodyHeight <= 0 {
		return false
	}
	return y == bodyHeight || y == bodyHeight+1
}

func isStatusLineMouseCol(x, start, end int) bool {
	if x >= start && x <= end {
		return true
	}
	col := x - 1
	return col >= start && col <= end
}

func (m *Model) reduceTranscriptCopyLeftPressMouse(msg tea.MouseMsg, layout mouseLayout) bool {
	if !isMouseClickMsg(msg) {
		return false
	}
	mouse := msg.Mouse()
	if mouse.X < layout.rightStart {
		return false
	}
	if m.mode != uiModeNormal && m.mode != uiModeCompose && m.mode != uiModeNotes && m.mode != uiModeAddNote {
		return false
	}
	if mouse.Y < 1 || mouse.Y > m.viewport.Height() {
		return false
	}
	handled, cmd := m.copyBlockByViewportPosition(mouse.X-layout.rightStart, mouse.Y-1)
	if cmd != nil {
		m.pendingMouseCmd = cmd
	}
	return handled
}

func (m *Model) reduceTranscriptPinLeftPressMouse(msg tea.MouseMsg, layout mouseLayout) bool {
	if !isMouseClickMsg(msg) {
		return false
	}
	mouse := msg.Mouse()
	if mouse.X < layout.rightStart {
		return false
	}
	if m.mode != uiModeNormal && m.mode != uiModeCompose {
		return false
	}
	if mouse.Y < 1 || mouse.Y > m.viewport.Height() || m.mouseOverInput(mouse.Y) {
		return false
	}
	handled, cmd := m.pinBlockByViewportPosition(mouse.X-layout.rightStart, mouse.Y-1)
	if !handled {
		return false
	}
	if cmd != nil {
		m.pendingMouseCmd = cmd
	}
	return true
}

func (m *Model) reduceTranscriptNotesFilterLeftPressMouse(msg tea.MouseMsg, layout mouseLayout) bool {
	if !isMouseClickMsg(msg) {
		return false
	}
	mouse := msg.Mouse()
	if mouse.X < layout.rightStart {
		return false
	}
	if m.mode != uiModeNotes && m.mode != uiModeAddNote {
		return false
	}
	if mouse.Y < 1 || mouse.Y > m.viewport.Height() {
		return false
	}
	handled, cmd := m.toggleNotesFilterByViewportPosition(mouse.X-layout.rightStart, mouse.Y-1)
	if !handled {
		return false
	}
	if cmd != nil {
		m.pendingMouseCmd = cmd
	}
	return true
}

func (m *Model) reduceTranscriptMoveLeftPressMouse(msg tea.MouseMsg, layout mouseLayout) bool {
	if !isMouseClickMsg(msg) {
		return false
	}
	mouse := msg.Mouse()
	if mouse.X < layout.rightStart {
		return false
	}
	if m.mode != uiModeNotes && m.mode != uiModeAddNote {
		return false
	}
	if mouse.Y < 1 || mouse.Y > m.viewport.Height() {
		return false
	}
	handled, cmd := m.moveNoteByViewportPosition(mouse.X-layout.rightStart, mouse.Y-1)
	if !handled {
		return false
	}
	if cmd != nil {
		m.pendingMouseCmd = cmd
	}
	return true
}

func (m *Model) reduceTranscriptDeleteLeftPressMouse(msg tea.MouseMsg, layout mouseLayout) bool {
	if !isMouseClickMsg(msg) {
		return false
	}
	mouse := msg.Mouse()
	if mouse.X < layout.rightStart {
		return false
	}
	if m.mode != uiModeNotes && m.mode != uiModeAddNote {
		return false
	}
	if mouse.Y < 1 || mouse.Y > m.viewport.Height() {
		return false
	}
	return m.deleteNoteByViewportPosition(mouse.X-layout.rightStart, mouse.Y-1)
}

func (m *Model) reduceTranscriptSelectLeftPressMouse(msg tea.MouseMsg, layout mouseLayout) bool {
	if !isMouseClickMsg(msg) {
		return false
	}
	mouse := msg.Mouse()
	if mouse.X < layout.rightStart {
		return false
	}
	if m.mode != uiModeNormal && m.mode != uiModeCompose {
		return false
	}
	if mouse.Y < 1 || mouse.Y > m.viewport.Height() || m.mouseOverInput(mouse.Y) {
		return false
	}
	return m.selectMessageByViewportPoint(mouse.X-layout.rightStart, mouse.Y-1)
}

func (m *Model) reduceModePickersLeftPressMouse(msg tea.MouseMsg, layout mouseLayout) bool {
	if !isMouseClickMsg(msg) {
		return false
	}
	mouse := msg.Mouse()
	if m.mode == uiModePickProvider && m.providerPicker != nil {
		if mouse.X >= layout.rightStart {
			row := mouse.Y - 1
			if row >= 0 && m.providerPicker.SelectByRow(row) {
				m.pendingMouseCmd = m.selectProvider()
				return true
			}
		}
	}
	if m.mode == uiModeEditWorkspaceGroups && m.groupPicker != nil {
		if mouse.X >= layout.rightStart {
			row := mouse.Y - 1
			if row >= 0 && m.groupPicker.HandleClick(row) {
				return true
			}
		}
	}
	if (m.mode == uiModePickWorkspaceRename || m.mode == uiModePickWorkspaceGroupEdit) && m.workspacePicker != nil {
		if mouse.X >= layout.rightStart {
			row := mouse.Y - 1
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
		if mouse.X >= layout.rightStart {
			row := mouse.Y - 1
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
		if mouse.X >= layout.rightStart {
			row := mouse.Y - 1
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
		if mouse.X >= layout.rightStart {
			row := mouse.Y - 1
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
	if m.noteMovePickerModeActive() && m.groupSelectPicker != nil {
		if mouse.X >= layout.rightStart {
			row := mouse.Y - 1
			if row >= 0 && m.groupSelectPicker.HandleClick(row) {
				if cmd := m.handleNoteMovePickerSelection(); cmd != nil {
					m.pendingMouseCmd = cmd
				}
				return true
			}
		}
	}
	if m.mode == uiModeAssignGroupWorkspaces && m.workspaceMulti != nil {
		if mouse.X >= layout.rightStart {
			row := mouse.Y - 1
			if row >= 0 && m.workspaceMulti.HandleClick(row) {
				return true
			}
		}
	}
	if m.mode == uiModeAddWorktree && m.addWorktree != nil {
		if mouse.X >= layout.rightStart {
			row := mouse.Y - 1
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
	if !isMouseClickMsg(msg) {
		return false
	}
	mouse := msg.Mouse()
	if layout.listWidth > 0 && mouse.X < layout.listWidth {
		if m.sidebar != nil {
			entry := m.sidebar.ItemAtRow(mouse.Y)
			if entry == nil {
				return false
			}
			m.sidebar.SelectByRow(mouse.Y)
			if (entry.kind == sidebarWorkspace || entry.kind == sidebarWorktree) && entry.collapsible {
				if m.sidebar.ToggleSelectedContainer() {
					m.pendingMouseCmd = m.syncSidebarExpansionChange()
					return true
				}
			}
			m.pendingMouseCmd = m.onSelectionChanged()
			return true
		}
	}
	return false
}
