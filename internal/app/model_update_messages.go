package app

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

func (m *Model) reduceMutationMessages(msg tea.Msg) (bool, tea.Cmd) {
	switch msg := msg.(type) {
	case createWorkspaceMsg:
		if msg.err != nil {
			m.exitAddWorkspace("add workspace error: " + msg.err.Error())
			return true, nil
		}
		if msg.workspace != nil {
			m.appState.ActiveWorkspaceID = msg.workspace.ID
			m.hasAppState = true
			m.updateDelegate()
			m.exitAddWorkspace("workspace added: " + msg.workspace.Name)
			return true, tea.Batch(fetchWorkspacesCmd(m.workspaceAPI), fetchSessionsWithMetaCmd(m.sessionAPI), m.saveAppStateCmd())
		}
		m.exitAddWorkspace("workspace added")
		return true, nil
	case workspaceGroupsMsg:
		if msg.err != nil {
			m.status = "workspace groups error: " + msg.err.Error()
			return true, nil
		}
		m.groups = msg.groups
		if m.menu != nil {
			previous := m.menu.SelectedGroupIDs()
			m.menu.SetGroups(msg.groups)
			if m.handleMenuGroupChange(previous) {
				return true, m.saveAppStateCmd()
			}
		}
		return true, nil
	case createWorkspaceGroupMsg:
		if msg.err != nil {
			m.exitAddWorkspaceGroup("add group error: " + msg.err.Error())
			return true, nil
		}
		m.exitAddWorkspaceGroup("group added")
		return true, fetchWorkspaceGroupsCmd(m.workspaceAPI)
	case updateWorkspaceGroupMsg:
		if msg.err != nil {
			m.status = "update group error: " + msg.err.Error()
			return true, nil
		}
		m.status = "group updated"
		return true, fetchWorkspaceGroupsCmd(m.workspaceAPI)
	case deleteWorkspaceGroupMsg:
		if msg.err != nil {
			m.status = "delete group error: " + msg.err.Error()
			return true, nil
		}
		m.status = "group deleted"
		return true, tea.Batch(fetchWorkspaceGroupsCmd(m.workspaceAPI), fetchWorkspacesCmd(m.workspaceAPI))
	case assignGroupWorkspacesMsg:
		if msg.err != nil {
			m.status = "assign groups error: " + msg.err.Error()
			return true, nil
		}
		m.status = fmt.Sprintf("updated %d workspaces", msg.updated)
		return true, fetchWorkspacesCmd(m.workspaceAPI)
	case availableWorktreesMsg:
		if msg.err != nil {
			m.status = "worktrees error: " + msg.err.Error()
			return true, nil
		}
		if m.addWorktree != nil {
			count := m.addWorktree.SetAvailable(msg.worktrees, m.worktrees[msg.workspaceID], msg.workspacePath)
			m.status = fmt.Sprintf("%d worktrees found", count)
		}
		return true, nil
	case createWorktreeMsg:
		if msg.err != nil {
			m.status = "create worktree error: " + msg.err.Error()
			return true, nil
		}
		m.exitAddWorktree("worktree added")
		cmds := []tea.Cmd{fetchSessionsWithMetaCmd(m.sessionAPI)}
		if msg.workspaceID != "" {
			cmds = append(cmds, fetchWorktreesCmd(m.workspaceAPI, msg.workspaceID))
		}
		return true, tea.Batch(cmds...)
	case addWorktreeMsg:
		if msg.err != nil {
			m.status = "add worktree error: " + msg.err.Error()
			return true, nil
		}
		m.exitAddWorktree("worktree added")
		cmds := []tea.Cmd{fetchSessionsWithMetaCmd(m.sessionAPI)}
		if msg.workspaceID != "" {
			cmds = append(cmds, fetchWorktreesCmd(m.workspaceAPI, msg.workspaceID))
		}
		return true, tea.Batch(cmds...)
	case worktreeDeletedMsg:
		if msg.err != nil {
			m.status = "delete worktree error: " + msg.err.Error()
			return true, nil
		}
		if msg.worktreeID != "" && msg.worktreeID == m.appState.ActiveWorktreeID {
			m.appState.ActiveWorktreeID = ""
			m.hasAppState = true
		}
		m.status = "worktree deleted"
		cmds := []tea.Cmd{fetchSessionsWithMetaCmd(m.sessionAPI)}
		if msg.workspaceID != "" {
			cmds = append(cmds, fetchWorktreesCmd(m.workspaceAPI, msg.workspaceID))
		}
		return true, tea.Batch(cmds...)
	case updateWorkspaceMsg:
		if msg.err != nil {
			m.status = "update workspace error: " + msg.err.Error()
			return true, nil
		}
		m.status = "workspace updated"
		return true, tea.Batch(fetchWorkspacesCmd(m.workspaceAPI), fetchWorkspaceGroupsCmd(m.workspaceAPI), fetchSessionsWithMetaCmd(m.sessionAPI))
	case deleteWorkspaceMsg:
		if msg.err != nil {
			m.status = "delete workspace error: " + msg.err.Error()
			return true, nil
		}
		if msg.id != "" && msg.id == m.appState.ActiveWorkspaceID {
			m.appState.ActiveWorkspaceID = ""
			m.appState.ActiveWorktreeID = ""
			m.hasAppState = true
		}
		m.status = "workspace deleted"
		return true, tea.Batch(fetchWorkspacesCmd(m.workspaceAPI), fetchSessionsWithMetaCmd(m.sessionAPI), m.saveAppStateCmd())
	default:
		return false, nil
	}
}

func (m *Model) reduceStateMessages(msg tea.Msg) (bool, tea.Cmd) {
	switch msg := msg.(type) {
	case sessionsWithMetaMsg:
		if msg.err != nil {
			m.status = "error: " + msg.err.Error()
			return true, nil
		}
		m.sessions = msg.sessions
		m.sessionMeta = normalizeSessionMeta(msg.meta)
		m.pruneSelection()
		m.applySidebarItems()
		if m.pendingSelectID != "" && m.sidebar != nil {
			if m.sidebar.SelectBySessionID(m.pendingSelectID) {
				m.pendingSelectID = ""
			}
		}
		m.status = fmt.Sprintf("%d sessions", len(msg.sessions))
		return true, m.onSelectionChanged()
	case workspacesMsg:
		if msg.err != nil {
			m.status = "workspaces error: " + msg.err.Error()
			return true, nil
		}
		m.workspaces = msg.workspaces
		m.applySidebarItems()
		return true, m.fetchWorktreesForWorkspaces()
	case worktreesMsg:
		if msg.err != nil {
			m.status = "worktrees error: " + msg.err.Error()
			return true, nil
		}
		if msg.workspaceID != "" {
			m.worktrees[msg.workspaceID] = msg.worktrees
		}
		m.applySidebarItems()
		return true, nil
	case appStateMsg:
		if msg.err != nil {
			m.status = "state error: " + msg.err.Error()
			return true, nil
		}
		if msg.state != nil {
			m.applyAppState(msg.state)
			m.applySidebarItems()
			m.resize(m.width, m.height)
		}
		return true, nil
	case appStateSavedMsg:
		if msg.err != nil {
			m.status = "state save error: " + msg.err.Error()
			return true, nil
		}
		if msg.state != nil {
			m.applyAppState(msg.state)
		}
		return true, nil
	case tailMsg:
		if msg.err != nil {
			m.status = "tail error: " + msg.err.Error()
			if msg.key != "" && msg.key == m.loadingKey {
				m.loading = false
				m.setContentText("Error loading history.")
			}
			return true, nil
		}
		if msg.key != "" && msg.key != m.pendingSessionKey {
			return true, nil
		}
		if msg.key != "" && msg.key == m.loadingKey {
			m.loading = false
		}
		blocks := itemsToBlocks(msg.items)
		if shouldStreamItems(m.selectedSessionProvider()) && m.itemStream != nil {
			m.itemStream.SetSnapshotBlocks(blocks)
			blocks = m.itemStream.Blocks()
		}
		m.setSnapshotBlocks(blocks)
		m.status = "tail updated"
		return true, nil
	case historyMsg:
		if msg.err != nil {
			m.status = "history error: " + msg.err.Error()
			if msg.key != "" && msg.key == m.loadingKey {
				m.loading = false
				m.setContentText("Error loading history.")
			}
			return true, nil
		}
		if msg.key != "" && msg.key != m.pendingSessionKey {
			return true, nil
		}
		if msg.key != "" && msg.key == m.loadingKey {
			m.loading = false
		}
		blocks := itemsToBlocks(msg.items)
		if shouldStreamItems(m.selectedSessionProvider()) && m.itemStream != nil {
			m.itemStream.SetSnapshotBlocks(blocks)
			blocks = m.itemStream.Blocks()
		}
		m.setSnapshotBlocks(blocks)
		if msg.key != "" {
			m.transcriptCache[msg.key] = blocks
		}
		m.status = "history updated"
		return true, nil
	case historyPollMsg:
		if msg.id == "" || msg.key == "" {
			return true, nil
		}
		if msg.attempt >= historyPollMax {
			return true, nil
		}
		if m.mode != uiModeCompose {
			return true, nil
		}
		targetID := m.activeStreamTargetID()
		if targetID != msg.id {
			return true, nil
		}
		currentAgents := countAgentRepliesBlocks(m.currentBlocks())
		if msg.minAgents >= 0 {
			if currentAgents > msg.minAgents {
				return true, nil
			}
		} else if currentAgents > 0 {
			return true, nil
		}
		cmds := []tea.Cmd{fetchHistoryCmd(m.sessionAPI, msg.id, msg.key, maxViewportLines)}
		cmds = append(cmds, historyPollCmd(msg.id, msg.key, msg.attempt+1, historyPollDelay, msg.minAgents))
		return true, tea.Batch(cmds...)
	case sendMsg:
		if msg.err != nil {
			m.status = "send error: " + msg.err.Error()
			m.markPendingSendFailed(msg.token, msg.err)
			return true, nil
		}
		m.status = "message sent"
		m.clearPendingSend(msg.token)
		provider := m.providerForSessionID(msg.id)
		if shouldStreamItems(provider) && m.itemStream != nil && !m.itemStream.HasStream() {
			return true, openItemsCmd(m.sessionAPI, msg.id)
		}
		if provider == "codex" && m.codexStream != nil && !m.codexStream.HasStream() {
			return true, openEventsCmd(m.sessionAPI, msg.id)
		}
		return true, nil
	case approvalMsg:
		if msg.err != nil {
			m.status = "approval error: " + msg.err.Error()
			return true, nil
		}
		m.status = "approval sent"
		if m.pendingApproval != nil && m.pendingApproval.RequestID == msg.requestID {
			m.pendingApproval = nil
		}
		if m.codexStream != nil {
			m.codexStream.ClearApproval()
		}
		return true, nil
	case approvalsMsg:
		if msg.err != nil {
			m.status = "approvals error: " + msg.err.Error()
			return true, nil
		}
		if msg.id != m.selectedSessionID() {
			return true, nil
		}
		m.pendingApproval = selectApprovalRequest(msg.approvals)
		if m.pendingApproval != nil {
			if m.pendingApproval.Detail != "" {
				m.status = fmt.Sprintf("approval required: %s (%s)", m.pendingApproval.Summary, m.pendingApproval.Detail)
			} else if m.pendingApproval.Summary != "" {
				m.status = "approval required: " + m.pendingApproval.Summary
			} else {
				m.status = "approval required"
			}
		}
		return true, nil
	case interruptMsg:
		if msg.err != nil {
			m.status = "interrupt error: " + msg.err.Error()
			return true, nil
		}
		m.status = "interrupt sent"
		return true, nil
	case selectDebounceMsg:
		if msg.seq != m.selectSeq {
			return true, nil
		}
		item := m.selectedItem()
		if item == nil || !item.isSession() || item.session == nil || item.session.ID != msg.id {
			return true, nil
		}
		return true, m.loadSelectedSession(item)
	case startSessionMsg:
		if msg.err != nil {
			m.status = "start session error: " + msg.err.Error()
			return true, nil
		}
		if msg.session == nil || msg.session.ID == "" {
			m.status = "start session error: no session returned"
			return true, nil
		}
		m.newSession = nil
		m.pendingSelectID = msg.session.ID
		label := msg.session.Title
		if strings.TrimSpace(label) == "" {
			label = msg.session.ID
		}
		if m.compose != nil {
			m.compose.Enter(msg.session.ID, label)
		}
		key := "sess:" + msg.session.ID
		m.pendingSessionKey = key
		m.status = "session started"
		cmds := []tea.Cmd{fetchSessionsWithMetaCmd(m.sessionAPI), fetchHistoryCmd(m.sessionAPI, msg.session.ID, key, maxViewportLines)}
		if shouldStreamItems(msg.session.Provider) {
			cmds = append(cmds, openItemsCmd(m.sessionAPI, msg.session.ID))
		} else if msg.session.Provider == "codex" {
			cmds = append(cmds, openEventsCmd(m.sessionAPI, msg.session.ID))
		} else if isActiveStatus(msg.session.Status) {
			cmds = append(cmds, openStreamCmd(m.sessionAPI, msg.session.ID))
		}
		if msg.session.Provider == "codex" {
			cmds = append(cmds, historyPollCmd(msg.session.ID, key, 0, historyPollDelay, countAgentRepliesBlocks(m.currentBlocks())))
		}
		return true, tea.Batch(cmds...)
	case killMsg:
		if msg.err != nil {
			m.status = "kill error: " + msg.err.Error()
			return true, nil
		}
		m.status = "killed " + msg.id
		return true, fetchSessionsWithMetaCmd(m.sessionAPI)
	case exitMsg:
		if msg.err != nil {
			m.status = "exit error: " + msg.err.Error()
			return true, nil
		}
		if m.sidebar != nil {
			m.sidebar.RemoveSelection([]string{msg.id})
		}
		m.status = "marked exited " + msg.id
		return true, fetchSessionsWithMetaCmd(m.sessionAPI)
	case bulkExitMsg:
		if msg.err != nil {
			m.status = "exit error: " + msg.err.Error()
			return true, nil
		}
		if m.sidebar != nil {
			m.sidebar.RemoveSelection(msg.ids)
		}
		m.status = fmt.Sprintf("marked exited %d", len(msg.ids))
		return true, fetchSessionsWithMetaCmd(m.sessionAPI)
	case streamMsg:
		m.applyStreamMsg(msg)
		return true, nil
	case eventsMsg:
		m.applyEventsMsg(msg)
		return true, nil
	case itemsStreamMsg:
		m.applyItemsStreamMsg(msg)
		return true, nil
	default:
		return false, nil
	}
}

func (m *Model) activeStreamTargetID() string {
	targetID := m.composeSessionID()
	if targetID == "" {
		targetID = m.selectedSessionID()
	}
	return targetID
}

func (m *Model) streamMessageTargetsActiveSession(id string, cancel func()) bool {
	if id == m.activeStreamTargetID() {
		return true
	}
	if cancel != nil {
		cancel()
	}
	return false
}

func (m *Model) applyStreamMsg(msg streamMsg) {
	if msg.err != nil {
		m.status = "stream error: " + msg.err.Error()
		return
	}
	if !m.streamMessageTargetsActiveSession(msg.id, msg.cancel) {
		return
	}
	if m.stream != nil {
		m.stream.SetStream(msg.ch, msg.cancel)
	}
	m.status = "streaming"
}

func (m *Model) applyEventsMsg(msg eventsMsg) {
	if msg.err != nil {
		m.status = "events error: " + msg.err.Error()
		return
	}
	if !m.streamMessageTargetsActiveSession(msg.id, msg.cancel) {
		return
	}
	if m.codexStream != nil {
		m.codexStream.SetStream(msg.ch, msg.cancel)
	}
	m.status = "streaming events"
}

func (m *Model) applyItemsStreamMsg(msg itemsStreamMsg) {
	if msg.err != nil {
		m.status = "items stream error: " + msg.err.Error()
		return
	}
	if !m.streamMessageTargetsActiveSession(msg.id, msg.cancel) {
		return
	}
	if m.itemStream != nil {
		m.itemStream.SetSnapshotBlocks(m.currentBlocks())
		m.itemStream.SetStream(msg.ch, msg.cancel)
	}
	m.status = "streaming items"
}
