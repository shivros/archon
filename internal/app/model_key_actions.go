package app

import (
	tea "charm.land/bubbletea/v2"

	"control/internal/types"
)

func (m *Model) reducePendingApprovalKey(msg tea.KeyMsg) (bool, tea.Cmd) {
	if m.pendingApproval == nil {
		return false, nil
	}
	if m.mode != uiModeNormal && (m.input == nil || !m.input.IsSidebarFocused()) {
		return false, nil
	}
	switch m.keyString(msg) {
	case "y":
		return true, m.approvePending("accept")
	case "x":
		return true, m.approvePending("decline")
	default:
		return false, nil
	}
}

func (m *Model) reduceSidebarArrowKey(msg tea.KeyMsg) (bool, tea.Cmd) {
	if m.sidebar == nil {
		return false, nil
	}
	if m.mode != uiModeNormal && m.mode != uiModeCompose && m.mode != uiModeRecents && m.mode != uiModeNotes {
		return false, nil
	}
	switch msg.String() {
	case "left":
		if m.menu != nil && m.menu.IsActive() {
			return false, nil
		}
		if m.sidebar.SetSelectedContainerExpanded(false) {
			return true, m.syncSidebarExpansionChange()
		}
		return false, nil
	case "right":
		if m.menu != nil && m.menu.IsActive() {
			return false, nil
		}
		if m.sidebar.SetSelectedContainerExpanded(true) {
			return true, m.syncSidebarExpansionChange()
		}
		return false, nil
	case "up":
		m.sidebar.CursorUp()
		if m.mode == uiModeNotes {
			return true, m.onNotesSelectionChanged()
		}
		return true, m.onSelectionChanged()
	case "down":
		m.sidebar.CursorDown()
		if m.mode == uiModeNotes {
			return true, m.onNotesSelectionChanged()
		}
		return true, m.onSelectionChanged()
	default:
		return false, nil
	}
}

func (m *Model) reduceNormalModeKey(msg tea.KeyMsg) (bool, tea.Cmd) {
	if handled, cmd := m.reduceMenuAndAppKeys(msg); handled {
		return true, cmd
	}
	if handled, cmd := m.reduceClipboardAndSearchKeys(msg); handled {
		return true, cmd
	}
	if handled, cmd := m.reduceNotesEntryKeys(msg); handled {
		return true, cmd
	}
	if handled, cmd := m.reduceViewportNavigationKeys(msg); handled {
		return true, cmd
	}
	if handled, cmd := m.reduceComposeAndWorkspaceEntryKeys(msg); handled {
		return true, cmd
	}
	if handled, cmd := m.reduceSessionLifecycleKeys(msg); handled {
		return true, cmd
	}
	if handled, cmd := m.reduceViewToggleKeys(msg); handled {
		return true, cmd
	}
	if handled, cmd := m.reduceSelectionKeys(msg); handled {
		return true, cmd
	}
	return false, nil
}

func (m *Model) reduceNotesEntryKeys(msg tea.KeyMsg) (bool, tea.Cmd) {
	if m.keyMatchesOverriddenCommand(msg, KeyCommandNotesNew, "n") {
		return true, m.enterAddNoteForSelection()
	}
	return false, nil
}

func (m *Model) reduceMenuAndAppKeys(msg tea.KeyMsg) (bool, tea.Cmd) {
	if m.keyMatchesCommand(msg, KeyCommandMenu, "ctrl+m") {
		if m.menu != nil {
			if m.contextMenu != nil {
				m.contextMenu.Close()
			}
			m.menu.Toggle()
		}
		return true, nil
	}
	if m.keyMatchesCommand(msg, KeyCommandToggleSidebar, "ctrl+b") {
		m.toggleSidebar()
		return true, m.requestAppStateSaveCmd()
	}
	if m.keyMatchesCommand(msg, KeyCommandToggleNotesPanel, "ctrl+o") {
		return true, m.toggleNotesPanel()
	}
	if m.keyMatchesCommand(msg, KeyCommandToggleDebugStreams, "ctrl+d") {
		return true, m.toggleDebugStreams()
	}
	switch m.keyString(msg) {
	case "esc":
		return true, nil
	case "q":
		if m.debugStream != nil {
			m.debugStream.Close()
		}
		return true, tea.Quit
	default:
		return false, nil
	}
}

func (m *Model) reduceClipboardAndSearchKeys(msg tea.KeyMsg) (bool, tea.Cmd) {
	if m.keyMatchesCommand(msg, KeyCommandCopySessionID, "ctrl+g") {
		id := m.selectedSessionID()
		if id == "" {
			m.setCopyStatusWarning("no session selected")
			return true, nil
		}
		return true, m.copyWithStatusCmd(id, "copied session id")
	}
	switch m.keyString(msg) {
	case "/":
		m.enterSearch()
		return true, nil
	default:
		return false, nil
	}
}

func (m *Model) reduceViewportNavigationKeys(msg tea.KeyMsg) (bool, tea.Cmd) {
	switch m.keyString(msg) {
	case "{":
		m.jumpSection(-1)
		return true, nil
	case "}":
		m.jumpSection(1)
		return true, nil
	case "N":
		m.moveSearch(-1)
		return true, nil
	case "n":
		m.moveSearch(1)
		return true, nil
	default:
		if m.handleViewportTopBottomCommand(msg) {
			return true, nil
		}
		return false, nil
	}
}

func (m *Model) reduceComposeAndWorkspaceEntryKeys(msg tea.KeyMsg) (bool, tea.Cmd) {
	if m.keyMatchesCommand(msg, KeyCommandOpenChat, "enter") {
		return m.handleSidebarEnterSelection()
	}
	if m.keyMatchesCommand(msg, KeyCommandStartGuidedWorkflow, "w") {
		return true, m.startGuidedWorkflowFromSelectionTarget(m.selectionTarget(), GuidedWorkflowNameHints{})
	}

	switch m.keyString(msg) {
	case "m":
		m.enterRenameForSelection()
		return true, nil
	case "ctrl+n":
		return true, m.enterNewSessionCmd()
	case "a":
		m.enterAddWorkspace()
		return true, nil
	case "t":
		item := m.selectedItem()
		if item == nil || item.kind != sidebarWorkspace || item.workspace == nil || item.workspace.ID == "" {
			m.setValidationStatus("select a workspace to add a worktree")
			return true, nil
		}
		m.enterAddWorktree(item.workspace.ID)
		return true, nil
	case "c":
		id := m.selectedSessionID()
		if id == "" {
			m.setValidationStatus("select a session to send")
			return true, nil
		}
		m.enterCompose(id)
		return true, nil
	case "O":
		return true, m.enterNotesForSelection()
	case "o":
		target := m.selectionTarget()
		if target.Kind == SelectionKindWorkflow {
			if handled, cmd := m.activateSelectionTarget(target); handled {
				return true, cmd
			}
			return true, nil
		}
		return false, nil
	default:
		return false, nil
	}
}

func (m *Model) activateSelectionFromSidebar() (bool, tea.Cmd) {
	return m.activateSelectionTarget(m.selectionTarget())
}

func (m *Model) selectionTarget() SelectionTarget {
	return selectionTargetFromSidebarItem(m.selectedItem())
}

func (m *Model) handleSidebarEnterSelection() (bool, tea.Cmd) {
	service := m.selectionEnterActionServiceOrDefault()
	return service.HandleEnter(m.selectionTarget(), newModelSelectionEnterActionContext(m))
}

func (m *Model) activateSelectionTarget(target SelectionTarget) (bool, tea.Cmd) {
	service := m.selectionActivationServiceOrDefault()
	return service.ActivateSelection(target, newModelSelectionActivationContext(m))
}

func (m *Model) reduceSessionLifecycleKeys(msg tea.KeyMsg) (bool, tea.Cmd) {
	if m.keyMatchesCommand(msg, KeyCommandDismissSelection, "d") {
		m.enterDismissOrDeleteForSelection()
		return true, nil
	}
	switch m.keyString(msg) {
	case "r":
		m.setStatusMessage("refreshing")
		return true, tea.Batch(fetchWorkspacesCmd(m.workspaceAPI), m.fetchSessionsCmd(true))
	case "x":
		m.enterKillForSelection()
		return true, nil
	case "i":
		m.enterInterruptOrStopForSelection()
		return true, nil
	case "u":
		id := m.selectedSessionID()
		if id == "" {
			m.setValidationStatus("no session selected")
			return true, nil
		}
		m.setStatusMessage("undismissing " + id)
		return true, undismissSessionCmd(m.sessionAPI, id)
	case "D":
		return true, m.toggleShowDismissed()
	default:
		return false, nil
	}
}

func (m *Model) reduceViewToggleKeys(msg tea.KeyMsg) (bool, tea.Cmd) {
	switch m.keyString(msg) {
	case "1":
		if m.notesPanelOpen {
			return true, m.toggleNotesFilterScope(types.NoteScopeWorkspace)
		}
	case "2":
		if m.notesPanelOpen {
			return true, m.toggleNotesFilterScope(types.NoteScopeWorktree)
		}
	case "3":
		if m.notesPanelOpen {
			return true, m.toggleNotesFilterScope(types.NoteScopeSession)
		}
	case "0":
		if m.notesPanelOpen {
			return true, m.enableAllNotesFilters()
		}
	case "p":
		if m.follow {
			m.pauseFollow(true)
		} else {
			m.enableFollow(true)
		}
		return true, nil
	case "e":
		if m.toggleVisibleReasoning() {
			return true, nil
		}
		m.setValidationStatus("no reasoning in view")
		return true, nil
	case "v":
		m.enterMessageSelection()
		return true, nil
	default:
		return false, nil
	}
	return false, nil
}

func (m *Model) reduceSelectionKeys(msg tea.KeyMsg) (bool, tea.Cmd) {
	if m.keyMatchesCommand(msg, KeyCommandHistoryBack, "alt+left") {
		return true, m.navigateSelectionHistoryBack()
	}
	if m.keyMatchesCommand(msg, KeyCommandHistoryForward, "alt+right") {
		return true, m.navigateSelectionHistoryForward()
	}
	switch m.keyString(msg) {
	case " ", "space":
		if m.sidebar == nil || !m.sidebar.ToggleFocusedSelection() {
			m.setValidationStatus("select an item")
			return true, nil
		}
		return true, nil
	case "j":
		m.sidebar.CursorDown()
		return true, m.onSelectionChanged()
	case "k":
		m.sidebar.CursorUp()
		return true, m.onSelectionChanged()
	default:
		return false, nil
	}
}
