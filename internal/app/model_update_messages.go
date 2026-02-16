package app

import (
	"fmt"
	"strings"
	"time"

	"control/internal/types"

	tea "charm.land/bubbletea/v2"
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
			return true, tea.Batch(fetchWorkspacesCmd(m.workspaceAPI), m.fetchSessionsCmd(false), m.requestAppStateSaveCmd())
		}
		m.exitAddWorkspace("workspace added")
		return true, nil
	case workspaceGroupsMsg:
		if msg.err != nil {
			m.setStatusError("workspace groups error: " + msg.err.Error())
			return true, nil
		}
		m.groups = msg.groups
		if m.menu != nil {
			previous := m.menu.SelectedGroupIDs()
			m.menu.SetGroups(msg.groups)
			if m.handleMenuGroupChange(previous) {
				return true, m.requestAppStateSaveCmd()
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
			m.setStatusError("update group error: " + msg.err.Error())
			return true, nil
		}
		m.setStatusInfo("group updated")
		return true, fetchWorkspaceGroupsCmd(m.workspaceAPI)
	case deleteWorkspaceGroupMsg:
		if msg.err != nil {
			m.setStatusError("delete group error: " + msg.err.Error())
			return true, nil
		}
		m.setStatusInfo("group deleted")
		return true, tea.Batch(fetchWorkspaceGroupsCmd(m.workspaceAPI), fetchWorkspacesCmd(m.workspaceAPI))
	case assignGroupWorkspacesMsg:
		if msg.err != nil {
			m.setStatusError("assign groups error: " + msg.err.Error())
			return true, nil
		}
		m.setStatusInfo(fmt.Sprintf("updated %d workspaces", msg.updated))
		return true, fetchWorkspacesCmd(m.workspaceAPI)
	case availableWorktreesMsg:
		if msg.err != nil {
			m.setStatusError("worktrees error: " + msg.err.Error())
			return true, nil
		}
		if m.addWorktree != nil {
			count := m.addWorktree.SetAvailable(msg.worktrees, m.worktrees[msg.workspaceID], msg.workspacePath)
			m.setBackgroundStatus(fmt.Sprintf("%d worktrees found", count))
		}
		return true, nil
	case createWorktreeMsg:
		if msg.err != nil {
			m.setStatusError("create worktree error: " + msg.err.Error())
			return true, nil
		}
		m.exitAddWorktree("worktree added")
		cmds := []tea.Cmd{m.fetchSessionsCmd(false)}
		if msg.workspaceID != "" {
			cmds = append(cmds, fetchWorktreesCmd(m.workspaceAPI, msg.workspaceID))
		}
		return true, tea.Batch(cmds...)
	case addWorktreeMsg:
		if msg.err != nil {
			m.setStatusError("add worktree error: " + msg.err.Error())
			return true, nil
		}
		m.exitAddWorktree("worktree added")
		cmds := []tea.Cmd{m.fetchSessionsCmd(false)}
		if msg.workspaceID != "" {
			cmds = append(cmds, fetchWorktreesCmd(m.workspaceAPI, msg.workspaceID))
		}
		return true, tea.Batch(cmds...)
	case updateWorktreeMsg:
		if msg.err != nil {
			m.setStatusError("update worktree error: " + msg.err.Error())
			return true, nil
		}
		m.setStatusInfo("worktree updated")
		cmds := []tea.Cmd{m.fetchSessionsCmd(false)}
		if msg.workspaceID != "" {
			cmds = append(cmds, fetchWorktreesCmd(m.workspaceAPI, msg.workspaceID))
		} else {
			cmds = append(cmds, m.fetchWorktreesForWorkspaces())
		}
		return true, tea.Batch(cmds...)
	case worktreeDeletedMsg:
		if msg.err != nil {
			m.setStatusError("delete worktree error: " + msg.err.Error())
			return true, nil
		}
		if msg.worktreeID != "" && msg.worktreeID == m.appState.ActiveWorktreeID {
			m.appState.ActiveWorktreeID = ""
			m.hasAppState = true
		}
		m.setStatusInfo("worktree deleted")
		cmds := []tea.Cmd{m.fetchSessionsCmd(false)}
		if msg.workspaceID != "" {
			cmds = append(cmds, fetchWorktreesCmd(m.workspaceAPI, msg.workspaceID))
		}
		return true, tea.Batch(cmds...)
	case updateWorkspaceMsg:
		if msg.err != nil {
			m.setStatusError("update workspace error: " + msg.err.Error())
			return true, nil
		}
		m.setStatusInfo("workspace updated")
		return true, tea.Batch(fetchWorkspacesCmd(m.workspaceAPI), fetchWorkspaceGroupsCmd(m.workspaceAPI), m.fetchSessionsCmd(false))
	case updateSessionMsg:
		if msg.err != nil {
			m.setStatusError("update session error: " + msg.err.Error())
			return true, nil
		}
		m.setStatusInfo("session updated")
		return true, m.fetchSessionsCmd(false)
	case deleteWorkspaceMsg:
		if msg.err != nil {
			m.setStatusError("delete workspace error: " + msg.err.Error())
			return true, nil
		}
		if msg.id != "" && msg.id == m.appState.ActiveWorkspaceID {
			m.appState.ActiveWorkspaceID = ""
			m.appState.ActiveWorktreeID = ""
			m.hasAppState = true
		}
		m.setStatusInfo("workspace deleted")
		return true, tea.Batch(fetchWorkspacesCmd(m.workspaceAPI), m.fetchSessionsCmd(false), m.requestAppStateSaveCmd())
	default:
		return false, nil
	}
}

func (m *Model) reduceStateMessages(msg tea.Msg) (bool, tea.Cmd) {
	switch msg := msg.(type) {
	case sessionsWithMetaMsg:
		m.sessionMetaRefreshPending = false
		m.sessionMetaSyncPending = false
		now := time.Now().UTC()
		m.lastSessionMetaRefreshAt = now
		m.lastSessionMetaSyncAt = now
		if msg.err != nil {
			m.setBackgroundError("error: " + msg.err.Error())
			return true, nil
		}
		previousSelection := m.selectedSessionSnapshot()
		m.sessions = msg.sessions
		m.sessionMeta = normalizeSessionMeta(msg.meta)
		m.applySidebarItems()
		saveSidebarExpansionCmd := tea.Cmd(nil)
		if m.pendingSelectID != "" && m.sidebar != nil {
			if m.sidebar.SelectBySessionID(m.pendingSelectID) {
				m.pendingSelectID = ""
				if m.syncAppStateSidebarExpansion() {
					saveSidebarExpansionCmd = m.requestAppStateSaveCmd()
				}
			}
		}
		nextSelection := m.selectedSessionSnapshot()
		m.setBackgroundStatus(fmt.Sprintf("%d sessions", len(msg.sessions)))
		if m.selectionLoadPolicyOrDefault().ShouldReloadOnSessionsUpdate(previousSelection, nextSelection) {
			if m.shouldSkipSelectionReloadOnSessionsUpdate(previousSelection, nextSelection) {
				return true, saveSidebarExpansionCmd
			}
			selectionCmd := m.onSelectionChangedImmediate()
			if selectionCmd != nil && saveSidebarExpansionCmd != nil {
				return true, tea.Batch(selectionCmd, saveSidebarExpansionCmd)
			}
			if selectionCmd != nil {
				return true, selectionCmd
			}
			return true, saveSidebarExpansionCmd
		}
		return true, saveSidebarExpansionCmd
	case workspacesMsg:
		if msg.err != nil {
			m.setBackgroundError("workspaces error: " + msg.err.Error())
			return true, nil
		}
		m.workspaces = msg.workspaces
		m.applySidebarItems()
		return true, m.fetchWorktreesForWorkspaces()
	case worktreesMsg:
		if msg.err != nil {
			m.setBackgroundError("worktrees error: " + msg.err.Error())
			return true, nil
		}
		if msg.workspaceID != "" {
			m.worktrees[msg.workspaceID] = msg.worktrees
		}
		m.applySidebarItems()
		return true, nil
	case notesMsg:
		m.settleNotesPanelLoadScope(msg.scope, msg.err != nil)
		if msg.err != nil {
			m.setBackgroundError("notes error: " + msg.err.Error())
			if m.notesPanelOpen {
				m.notesPanelBlocks = notesPanelBlocksFromState(m.notes, m.notesScope, m.notesFilters, m.notesPanelLoadState())
				m.renderNotesPanel()
			}
			if (m.mode == uiModeNotes || m.mode == uiModeAddNote) && !m.notesScope.IsZero() {
				m.setContentText("Error loading notes.")
			}
			return true, nil
		}
		if !m.applyNotesScopeResult(msg.scope, msg.notes) {
			return true, nil
		}
		m.setBackgroundStatus("notes updated")
		return true, nil
	case notesPanelReflowMsg:
		if !m.notesPanelOpen {
			return true, nil
		}
		m.renderViewport()
		m.renderNotesPanel()
		return true, nil
	case noteCreatedMsg:
		if msg.err != nil {
			m.setStatusError("note error: " + msg.err.Error())
			return true, nil
		}
		m.setStatusInfo("note saved")
		m.upsertNotesLive(msg.note)
		return true, m.notesRefreshCmdForOpenViews()
	case notePinnedMsg:
		if msg.err != nil {
			m.setStatusError("pin error: " + msg.err.Error())
			return true, nil
		}
		m.setStatusInfo("message pinned")
		m.upsertNotesLive(msg.note)
		return true, m.notesRefreshCmdForOpenViews()
	case noteDeletedMsg:
		if msg.err != nil {
			m.setStatusError("delete note error: " + msg.err.Error())
			return true, nil
		}
		m.setStatusInfo("note deleted")
		m.removeNotesLive(msg.id)
		return true, m.notesRefreshCmdForOpenViews()
	case noteMovedMsg:
		if msg.err != nil {
			m.setStatusError("move note error: " + msg.err.Error())
			return true, nil
		}
		if msg.previous != nil {
			m.removeNotesLive(msg.previous.ID)
		} else if msg.note != nil {
			m.removeNotesLive(msg.note.ID)
		}
		m.upsertNotesLive(msg.note)
		m.setStatusInfo("note moved")
		return true, m.notesRefreshCmdForOpenViews()
	case appStateInitialLoadMsg:
		if msg.err != nil {
			m.setBackgroundError("state error: " + msg.err.Error())
			return true, nil
		}
		if m.initialStateLoaded || m.hasAppState {
			return true, nil
		}
		if msg.state != nil {
			m.applyAppState(msg.state)
			m.initialStateLoaded = true
			m.applySidebarItems()
			m.resize(m.width, m.height)
		}
		return true, nil
	case appStateSaveFlushMsg:
		return true, m.flushAppStateSaveCmd(msg.requestSeq)
	case appStateSavedMsg:
		if msg.requestSeq > 0 && msg.requestSeq < m.appStateSaveSeq {
			return true, nil
		}
		m.appStateSaveInFlight = false
		if msg.err != nil {
			m.setBackgroundError("state save error: " + msg.err.Error())
			if m.appStateSaveDirty {
				return true, m.requestAppStateSaveCmd()
			}
			return true, nil
		}
		if msg.state != nil {
			m.applyAppState(msg.state)
		}
		if m.appStateSaveDirty {
			return true, m.requestAppStateSaveCmd()
		}
		return true, nil
	case providerOptionsMsg:
		provider := strings.ToLower(strings.TrimSpace(msg.provider))
		isPending := provider != "" && strings.EqualFold(provider, strings.TrimSpace(m.pendingComposeOptionFor))
		if msg.err != nil {
			if isPending {
				m.clearPendingComposeOptionRequest()
			}
			m.setBackgroundError("provider options error: " + msg.err.Error())
			return true, nil
		}
		if provider != "" && msg.options != nil {
			if m.providerOptions == nil {
				m.providerOptions = map[string]*types.ProviderOptionCatalog{}
			}
			m.providerOptions[provider] = msg.options
		}
		if isPending {
			target := m.pendingComposeOptionTarget
			m.clearPendingComposeOptionRequest()
			if m.mode == uiModeCompose && strings.EqualFold(m.composeProvider(), provider) {
				if m.openComposeOptionPicker(target) {
					m.setStatusMessage("select " + composeOptionLabel(target))
				} else {
					m.setValidationStatus("no " + composeOptionLabel(target) + " options available")
				}
			}
		}
		return true, nil
	case tailMsg:
		if msg.err != nil {
			m.setBackgroundError("tail error: " + msg.err.Error())
			if msg.key != "" && msg.key == m.pendingSessionKey {
				m.finishUILatencyAction(uiLatencyActionSwitchSession, msg.key, uiLatencyOutcomeError)
			}
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
		provider := m.providerForSessionID(msg.id)
		if m.shouldKeepLiveCodexSnapshot(provider, msg.id) {
			if msg.key != "" {
				m.cacheTranscriptBlocks(msg.key, m.activeTranscriptBlocks(provider))
				m.finishUILatencyAction(uiLatencyActionSwitchSession, msg.key, uiLatencyOutcomeOK)
			}
			m.setBackgroundStatus("tail refreshed")
			return true, nil
		}
		blocks := m.buildSessionBlocksFromItems(msg.id, provider, msg.items, m.currentBlocks())
		if m.transcriptViewportVisible() {
			m.setSnapshotBlocks(blocks)
			m.noteRequestVisibleUpdate(msg.id)
		}
		if msg.key != "" {
			m.cacheTranscriptBlocks(msg.key, blocks)
			m.finishUILatencyAction(uiLatencyActionSwitchSession, msg.key, uiLatencyOutcomeOK)
		} else if msg.id == m.selectedSessionID() {
			m.cacheTranscriptBlocks(m.selectedKey(), blocks)
		}
		m.setBackgroundStatus("tail updated")
		return true, nil
	case historyMsg:
		if msg.err != nil {
			m.setBackgroundError("history error: " + msg.err.Error())
			if msg.key != "" && msg.key == m.pendingSessionKey {
				m.finishUILatencyAction(uiLatencyActionSwitchSession, msg.key, uiLatencyOutcomeError)
			}
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
		provider := m.providerForSessionID(msg.id)
		if m.shouldKeepLiveCodexSnapshot(provider, msg.id) {
			if msg.key != "" {
				m.cacheTranscriptBlocks(msg.key, m.activeTranscriptBlocks(provider))
				m.finishUILatencyAction(uiLatencyActionSwitchSession, msg.key, uiLatencyOutcomeOK)
			}
			m.setBackgroundStatus("history refreshed")
			return true, nil
		}
		blocks := m.buildSessionBlocksFromItems(msg.id, provider, msg.items, m.currentBlocks())
		if m.transcriptViewportVisible() {
			m.setSnapshotBlocks(blocks)
			m.noteRequestVisibleUpdate(msg.id)
		}
		if msg.key != "" {
			m.cacheTranscriptBlocks(msg.key, blocks)
			m.finishUILatencyAction(uiLatencyActionSwitchSession, msg.key, uiLatencyOutcomeOK)
		} else if msg.id == m.selectedSessionID() {
			m.cacheTranscriptBlocks(m.selectedKey(), blocks)
		}
		m.setBackgroundStatus("history updated")
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
		provider := m.providerForSessionID(msg.id)
		cmds := []tea.Cmd{fetchHistoryCmd(m.sessionHistoryAPI, msg.id, msg.key, m.historyFetchLinesInitial())}
		if shouldStreamItems(provider) {
			cmds = append(cmds, fetchApprovalsCmd(m.sessionAPI, msg.id))
		}
		cmds = append(cmds, historyPollCmd(msg.id, msg.key, msg.attempt+1, historyPollDelay, msg.minAgents))
		return true, tea.Batch(cmds...)
	case sendMsg:
		if msg.err != nil {
			m.setStatusError("send error: " + msg.err.Error())
			m.markPendingSendFailed(msg.token, msg.err)
			m.stopRequestActivityFor(msg.id)
			return true, nil
		}
		now := time.Now().UTC()
		m.noteSessionMetaActivity(msg.id, msg.turnID, now)
		m.applySidebarItems()
		m.startRequestActivity(msg.id, m.providerForSessionID(msg.id))
		m.setStatusInfo("message sent")
		m.clearPendingSend(msg.token)
		m.sessionMetaRefreshPending = true
		m.lastSessionMetaRefreshAt = now
		provider := m.providerForSessionID(msg.id)
		cmds := []tea.Cmd{m.fetchSessionsCmd(false)}
		if shouldStreamItems(provider) {
			cmds = append(cmds, fetchApprovalsCmd(m.sessionAPI, msg.id))
		}
		if shouldStreamItems(provider) && m.itemStream != nil && !m.itemStream.HasStream() {
			cmds = append(cmds, openItemsCmd(m.sessionAPI, msg.id))
			return true, tea.Batch(cmds...)
		}
		if provider == "codex" && m.codexStream != nil && !m.codexStream.HasStream() {
			cmds = append(cmds, openEventsCmd(m.sessionAPI, msg.id))
			return true, tea.Batch(cmds...)
		}
		return true, tea.Batch(cmds...)
	case approvalMsg:
		if msg.err != nil {
			m.setStatusError("approval error: " + msg.err.Error())
			return true, nil
		}
		resolution := approvalResolutionFromRequest(findApprovalRequestByID(m.sessionApprovals[msg.id], msg.requestID), msg.decision, time.Now().UTC())
		if resolution == nil {
			resolution = &ApprovalResolution{
				RequestID:  msg.requestID,
				SessionID:  msg.id,
				Decision:   msg.decision,
				ResolvedAt: time.Now().UTC(),
			}
		}
		_ = m.upsertApprovalResolutionForSession(msg.id, resolution)
		_ = m.removeApprovalForSession(msg.id, msg.requestID)
		if msg.id == m.selectedSessionID() {
			m.pendingApproval = latestApprovalRequest(m.sessionApprovals[msg.id])
			m.refreshVisibleApprovalBlocks(msg.id)
		}
		m.setStatusInfo("approval sent")
		if m.codexStream != nil {
			if pending := m.codexStream.PendingApproval(); pending != nil && pending.RequestID == msg.requestID {
				m.codexStream.ClearApproval()
			}
		}
		return true, nil
	case approvalsMsg:
		if msg.err != nil {
			m.setBackgroundError("approvals error: " + msg.err.Error())
			return true, nil
		}
		requests := approvalRequestsFromRecords(msg.approvals)
		m.setApprovalsForSession(msg.id, requests)
		if msg.id != m.selectedSessionID() {
			return true, nil
		}
		m.pendingApproval = latestApprovalRequest(m.sessionApprovals[msg.id])
		m.refreshVisibleApprovalBlocks(msg.id)
		if m.pendingApproval != nil {
			if m.pendingApproval.Detail != "" {
				m.setStatusWarning(fmt.Sprintf("approval required: %s (%s)", m.pendingApproval.Summary, m.pendingApproval.Detail))
			} else if m.pendingApproval.Summary != "" {
				m.setStatusWarning("approval required: " + m.pendingApproval.Summary)
			} else {
				m.setStatusWarning("approval required")
			}
		}
		return true, nil
	case interruptMsg:
		if msg.err != nil {
			m.setStatusError("interrupt error: " + msg.err.Error())
			return true, nil
		}
		m.setStatusInfo("interrupt sent")
		m.stopRequestActivityFor(msg.id)
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
			m.setStatusError("start session error: " + msg.err.Error())
			m.loading = false
			m.loadingKey = ""
			m.stopRequestActivity()
			return true, nil
		}
		if msg.session == nil || msg.session.ID == "" {
			m.setStatusError("start session error: no session returned")
			m.loading = false
			m.loadingKey = ""
			return true, nil
		}
		m.resetStream()
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
		m.loading = true
		m.loadingKey = key
		m.scrollOnLoad = true
		m.setLoadingContent()
		m.startRequestActivity(msg.session.ID, msg.session.Provider)
		m.setStatusInfo("session started")
		initialLines := m.historyFetchLinesInitial()
		cmds := []tea.Cmd{m.fetchSessionsCmd(false), fetchHistoryCmd(m.sessionHistoryAPI, msg.session.ID, key, initialLines)}
		if shouldStreamItems(msg.session.Provider) {
			cmds = append(cmds, fetchApprovalsCmd(m.sessionAPI, msg.session.ID))
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
			m.setStatusError("kill error: " + msg.err.Error())
			return true, nil
		}
		m.setStatusInfo("killed " + msg.id)
		return true, m.fetchSessionsCmd(false)
	case exitMsg:
		if msg.err != nil {
			m.setStatusError("exit error: " + msg.err.Error())
			return true, nil
		}
		m.setStatusInfo("marked exited " + msg.id)
		return true, m.fetchSessionsCmd(false)
	case bulkExitMsg:
		if msg.err != nil {
			m.setStatusError("exit error: " + msg.err.Error())
			return true, nil
		}
		m.setStatusInfo(fmt.Sprintf("marked exited %d", len(msg.ids)))
		return true, m.fetchSessionsCmd(false)
	case dismissMsg:
		if msg.err != nil {
			m.setStatusError("dismiss error: " + msg.err.Error())
			return true, nil
		}
		m.setStatusInfo("dismissed " + msg.id)
		return true, m.fetchSessionsCmd(false)
	case bulkDismissMsg:
		if msg.err != nil {
			m.setStatusError("dismiss error: " + msg.err.Error())
			return true, nil
		}
		m.setStatusInfo(fmt.Sprintf("dismissed %d", len(msg.ids)))
		return true, m.fetchSessionsCmd(false)
	case undismissMsg:
		if msg.err != nil {
			m.setStatusError("undismiss error: " + msg.err.Error())
			return true, nil
		}
		m.setStatusInfo("undismissed " + msg.id)
		return true, m.fetchSessionsCmd(false)
	case bulkUndismissMsg:
		if msg.err != nil {
			m.setStatusError("undismiss error: " + msg.err.Error())
			return true, nil
		}
		m.setStatusInfo(fmt.Sprintf("undismissed %d", len(msg.ids)))
		return true, m.fetchSessionsCmd(false)
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

func (m *Model) shouldKeepLiveCodexSnapshot(provider, sessionID string) bool {
	if provider != "codex" || !m.requestActivity.active {
		return false
	}
	return strings.TrimSpace(m.requestActivity.sessionID) == strings.TrimSpace(sessionID) && m.requestActivity.eventCount > 0
}

func (m *Model) activeTranscriptBlocks(provider string) []ChatBlock {
	if provider == "codex" && m.codexStream != nil {
		return m.codexStream.Blocks()
	}
	if shouldStreamItems(provider) && m.itemStream != nil {
		return m.itemStream.Blocks()
	}
	return m.currentBlocks()
}

func (m *Model) shouldSkipSelectionReloadOnSessionsUpdate(previous, next sessionSelectionSnapshot) bool {
	if !next.isSession {
		return false
	}
	if m.mode == uiModeNotes || m.mode == uiModeAddNote {
		return true
	}
	// Preserve manual reading position when the same selected session updates.
	return !m.follow &&
		previous.isSession &&
		previous.sessionID == next.sessionID &&
		previous.key == next.key
}

func (m *Model) buildSessionBlocksFromItems(sessionID, provider string, items []map[string]any, previous []ChatBlock) []ChatBlock {
	blocks := itemsToBlocks(items)
	if provider == "codex" {
		blocks = coalesceAdjacentReasoningBlocks(blocks)
	}
	if providerSupportsApprovals(provider) {
		blocks = mergeApprovalBlocks(blocks, m.sessionApprovals[sessionID], m.sessionApprovalResolutions[sessionID])
		blocks = preserveApprovalPositions(previous, blocks)
	}
	if shouldStreamItems(provider) && m.itemStream != nil {
		m.itemStream.SetSnapshotBlocks(blocks)
		blocks = m.itemStream.Blocks()
	}
	if provider == "codex" && m.codexStream != nil {
		m.codexStream.SetSnapshotBlocks(blocks)
		blocks = m.codexStream.Blocks()
	}
	return blocks
}

func (m *Model) activeStreamTargetID() string {
	if m.newSession != nil {
		return ""
	}
	targetID := m.composeSessionID()
	if targetID == "" {
		targetID = m.selectedSessionID()
	}
	if targetID == "" {
		targetID = sessionIDFromSidebarKey(m.pendingSessionKey)
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
		m.setBackgroundError("stream error: " + msg.err.Error())
		return
	}
	if !m.streamMessageTargetsActiveSession(msg.id, msg.cancel) {
		return
	}
	if m.stream != nil {
		m.stream.SetStream(msg.ch, msg.cancel)
	}
	m.setBackgroundStatus("streaming")
}

func (m *Model) applyEventsMsg(msg eventsMsg) {
	if msg.err != nil {
		m.setBackgroundError("events error: " + msg.err.Error())
		return
	}
	if !m.streamMessageTargetsActiveSession(msg.id, msg.cancel) {
		return
	}
	if m.codexStream != nil {
		m.codexStream.SetStream(msg.ch, msg.cancel)
	}
	m.setBackgroundStatus("streaming events")
}

func (m *Model) applyItemsStreamMsg(msg itemsStreamMsg) {
	if msg.err != nil {
		m.setBackgroundError("items stream error: " + msg.err.Error())
		return
	}
	if !m.streamMessageTargetsActiveSession(msg.id, msg.cancel) {
		return
	}
	if m.itemStream != nil {
		m.itemStream.SetSnapshotBlocks(m.currentBlocks())
		m.itemStream.SetStream(msg.ch, msg.cancel)
	}
	m.setBackgroundStatus("streaming items")
}
