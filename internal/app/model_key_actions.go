package app

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

func (m *Model) reducePendingApprovalKey(msg tea.KeyMsg) (bool, tea.Cmd) {
	if m.pendingApproval == nil {
		return false, nil
	}
	if m.mode != uiModeNormal && (m.input == nil || !m.input.IsSidebarFocused()) {
		return false, nil
	}
	switch msg.String() {
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
	if m.mode != uiModeNormal && m.mode != uiModeCompose && m.mode != uiModeNotes {
		return false, nil
	}
	switch msg.String() {
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

func (m *Model) reduceMenuAndAppKeys(msg tea.KeyMsg) (bool, tea.Cmd) {
	switch msg.String() {
	case "m":
		if m.menu != nil {
			if m.contextMenu != nil {
				m.contextMenu.Close()
			}
			m.menu.Toggle()
		}
		return true, nil
	case "esc":
		return true, nil
	case "q":
		return true, tea.Quit
	case "ctrl+b":
		m.toggleSidebar()
		return true, m.saveAppStateCmd()
	default:
		return false, nil
	}
}

func (m *Model) reduceClipboardAndSearchKeys(msg tea.KeyMsg) (bool, tea.Cmd) {
	switch msg.String() {
	case "ctrl+y":
		id := m.selectedSessionID()
		if id == "" {
			m.setCopyStatusWarning("no session selected")
			return true, nil
		}
		m.copyWithStatus(id, "copied session id")
		return true, nil
	case "/":
		m.enterSearch()
		return true, nil
	default:
		return false, nil
	}
}

func (m *Model) reduceViewportNavigationKeys(msg tea.KeyMsg) (bool, tea.Cmd) {
	switch msg.String() {
	case "g":
		m.viewport.GotoTop()
		m.pauseFollow(true)
		return true, nil
	case "G":
		m.enableFollow(true)
		return true, nil
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
		return false, nil
	}
}

func (m *Model) reduceComposeAndWorkspaceEntryKeys(msg tea.KeyMsg) (bool, tea.Cmd) {
	switch msg.String() {
	case "ctrl+n":
		m.enterNewSession()
		return true, nil
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
	case "enter":
		id := m.selectedSessionID()
		if id == "" {
			m.setValidationStatus("select a session to chat")
			return true, nil
		}
		m.enterCompose(id)
		return true, nil
	default:
		return false, nil
	}
}

func (m *Model) reduceSessionLifecycleKeys(msg tea.KeyMsg) (bool, tea.Cmd) {
	switch msg.String() {
	case "r":
		m.setStatusMessage("refreshing")
		return true, tea.Batch(fetchWorkspacesCmd(m.workspaceAPI), fetchSessionsWithMetaCmd(m.sessionAPI))
	case "x":
		id := m.selectedSessionID()
		if id == "" {
			m.setValidationStatus("no session selected")
			return true, nil
		}
		m.setStatusMessage("killing " + id)
		return true, killSessionCmd(m.sessionAPI, id)
	case "i":
		id := m.selectedSessionID()
		if id == "" {
			m.setValidationStatus("no session selected")
			return true, nil
		}
		m.setStatusMessage("interrupting " + id)
		return true, interruptSessionCmd(m.sessionAPI, id)
	case "d":
		ids := m.selectedSessionIDs()
		if len(ids) == 0 {
			m.setValidationStatus("no session selected")
			return true, nil
		}
		m.confirmDismissSessions(ids)
		return true, nil
	default:
		return false, nil
	}
}

func (m *Model) reduceViewToggleKeys(msg tea.KeyMsg) (bool, tea.Cmd) {
	switch msg.String() {
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
}

func (m *Model) reduceSelectionKeys(msg tea.KeyMsg) (bool, tea.Cmd) {
	switch msg.String() {
	case " ", "space":
		if m.toggleSelection() {
			count := 0
			if m.sidebar != nil {
				count = m.sidebar.SelectionCount()
			}
			m.setStatusMessage(fmt.Sprintf("selected %d", count))
			if m.advanceToNextSession() {
				return true, m.onSelectionChanged()
			}
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
