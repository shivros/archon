package app

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"control/internal/client"
	"control/internal/guidedworkflows"
	"control/internal/types"

	tea "charm.land/bubbletea/v2"
)

func (m *Model) reduceMutationMessages(msg tea.Msg) (bool, tea.Cmd) {
	switch msg := msg.(type) {
	case workflowRunCreatedMsg:
		if m.guidedWorkflow == nil {
			return true, nil
		}
		if msg.err != nil {
			m.guidedWorkflow.SetCreateError(msg.err)
			m.setStatusError("guided workflow create error: " + msg.err.Error())
			m.renderGuidedWorkflowContent()
			return true, nil
		}
		runID := ""
		if msg.run != nil {
			m.upsertWorkflowRun(msg.run)
			m.applySidebarItemsIfDirty()
			m.guidedWorkflow.SetRun(msg.run)
			runID = strings.TrimSpace(msg.run.ID)
		}
		if runID == "" {
			m.guidedWorkflow.SetCreateError(fmt.Errorf("guided workflow run id is missing"))
			m.setStatusError("guided workflow create error: missing run id")
			m.renderGuidedWorkflowContent()
			return true, nil
		}
		m.setStatusMessage("starting guided workflow run")
		m.renderGuidedWorkflowContent()
		return true, startWorkflowRunCmd(m.guidedWorkflowAPI, runID)
	case workflowRunStartedMsg:
		if m.guidedWorkflow == nil {
			return true, nil
		}
		if msg.err != nil {
			m.guidedWorkflow.SetStartError(msg.err)
			m.setStatusError("guided workflow start error: " + msg.err.Error())
			m.renderGuidedWorkflowContent()
			return true, nil
		}
		m.upsertWorkflowRun(msg.run)
		m.applySidebarItemsIfDirty()
		m.guidedWorkflow.SetRun(msg.run)
		m.setStatusInfo("guided workflow running")
		m.renderGuidedWorkflowContent()
		runID := strings.TrimSpace(m.guidedWorkflow.RunID())
		if runID == "" {
			return true, nil
		}
		m.guidedWorkflow.MarkRefreshQueued(time.Now().UTC())
		return true, fetchWorkflowRunSnapshotCmd(m.guidedWorkflowAPI, runID)
	case workflowRunSnapshotMsg:
		if m.guidedWorkflow == nil {
			return true, nil
		}
		if msg.err != nil {
			m.guidedWorkflow.SetSnapshotError(msg.err)
			m.setBackgroundError("guided workflow refresh error: " + msg.err.Error())
			m.renderGuidedWorkflowContent()
			return true, nil
		}
		previousStatus := guidedworkflows.WorkflowRunStatus("")
		previousStatusKnown := false
		if msg.run != nil {
			previousStatus, previousStatusKnown = m.workflowRunStatus(strings.TrimSpace(msg.run.ID))
		}
		appStateSaveCmd := tea.Cmd(nil)
		if msg.run != nil {
			runID := strings.TrimSpace(msg.run.ID)
			if runID != "" {
				if msg.run.DismissedAt != nil && !m.showDismissed {
					if m.addDismissedMissingWorkflowRunID(runID) {
						appStateSaveCmd = m.requestAppStateSaveCmd()
					}
				} else {
					if m.removeDismissedMissingWorkflowRunID(runID) {
						appStateSaveCmd = m.requestAppStateSaveCmd()
					}
				}
			}
		}
		m.upsertWorkflowRun(msg.run)
		m.applySidebarItemsIfDirtyWithReason(sidebarApplyReasonBackground)
		m.guidedWorkflow.SetSnapshot(msg.run, msg.timeline)
		if msg.run != nil && (msg.run.DismissedAt == nil || m.showDismissed) {
			switch msg.run.Status {
			case guidedworkflows.WorkflowRunStatusPaused:
				if !previousStatusKnown || previousStatus != guidedworkflows.WorkflowRunStatusPaused {
					m.setStatusInfo("guided workflow paused: decision needed")
				}
			case guidedworkflows.WorkflowRunStatusCompleted:
				if !previousStatusKnown || previousStatus != guidedworkflows.WorkflowRunStatusCompleted {
					m.setStatusInfo("guided workflow completed")
				}
			case guidedworkflows.WorkflowRunStatusFailed:
				if !previousStatusKnown || previousStatus != guidedworkflows.WorkflowRunStatusFailed {
					m.setStatusError("guided workflow failed")
				}
			}
		}
		m.renderGuidedWorkflowContent()
		return true, appStateSaveCmd
	case workflowRunDecisionMsg:
		if m.guidedWorkflow == nil {
			return true, nil
		}
		if msg.err != nil {
			m.guidedWorkflow.SetDecisionError(msg.err)
			m.setStatusError("guided workflow decision error: " + msg.err.Error())
			m.renderGuidedWorkflowContent()
			return true, nil
		}
		m.upsertWorkflowRun(msg.run)
		m.applySidebarItemsIfDirty()
		m.guidedWorkflow.SetRun(msg.run)
		m.renderGuidedWorkflowContent()
		runID := strings.TrimSpace(m.guidedWorkflow.RunID())
		if runID == "" {
			return true, nil
		}
		m.guidedWorkflow.MarkRefreshQueued(time.Now().UTC())
		return true, fetchWorkflowRunSnapshotCmd(m.guidedWorkflowAPI, runID)
	case workflowRunVisibilityMsg:
		if msg.err != nil {
			if msg.dismissed && isWorkflowRunNotFoundError(msg.err) && m.dismissWorkflowRunLocally(msg.runID) {
				m.setStatusWarning("guided workflow missing in backend; dismissed locally")
				return true, m.requestAppStateSaveCmd()
			}
			if msg.dismissed {
				m.setStatusError("guided workflow dismiss error: " + msg.err.Error())
			} else {
				m.setStatusError("guided workflow undismiss error: " + msg.err.Error())
			}
			return true, nil
		}
		if msg.run != nil {
			m.upsertWorkflowRun(msg.run)
		}
		runID := strings.TrimSpace(msg.runID)
		if runID == "" && msg.run != nil {
			runID = strings.TrimSpace(msg.run.ID)
		}
		appStateSaveCmd := tea.Cmd(nil)
		if runID != "" {
			if msg.dismissed {
				if m.addDismissedMissingWorkflowRunID(runID) {
					appStateSaveCmd = m.requestAppStateSaveCmd()
				}
			} else {
				if m.removeDismissedMissingWorkflowRunID(runID) {
					appStateSaveCmd = m.requestAppStateSaveCmd()
				}
			}
		}
		m.applySidebarItemsIfDirty()
		if msg.dismissed {
			m.setStatusInfo("guided workflow dismissed")
		} else {
			m.setStatusInfo("guided workflow restored")
		}
		return true, tea.Batch(fetchWorkflowRunsCmd(m.guidedWorkflowAPI, m.showDismissed), appStateSaveCmd)
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

type sessionItemsMessageContext struct {
	source   sessionProjectionSource
	id       string
	key      string
	provider string
}

func (m *Model) handleSessionItemsMessage(source sessionProjectionSource, id, key string, items []map[string]any, err error) tea.Cmd {
	ctx, ok := m.prepareSessionItemsMessageContext(source, id, key, err)
	if !ok {
		return nil
	}
	if m.applyLiveSessionItemsSnapshot(ctx) {
		return nil
	}
	return m.projectAndApplySessionItems(ctx, items)
}

func (m *Model) prepareSessionItemsMessageContext(source sessionProjectionSource, id, key string, err error) (sessionItemsMessageContext, bool) {
	ctx := sessionItemsMessageContext{
		source: source,
		id:     id,
		key:    key,
	}
	if err != nil {
		m.handleSessionItemsMessageError(ctx, err)
		return ctx, false
	}
	if key != "" && key != m.pendingSessionKey {
		return ctx, false
	}
	if key != "" && key == m.loadingKey {
		m.loading = false
	}
	ctx.provider = m.providerForSessionID(id)
	return ctx, true
}

func (m *Model) handleSessionItemsMessageError(ctx sessionItemsMessageContext, err error) {
	if m == nil || err == nil {
		return
	}
	m.setBackgroundError(string(ctx.source) + " error: " + err.Error())
	if ctx.key != "" && ctx.key == m.pendingSessionKey {
		m.finishUILatencyAction(uiLatencyActionSwitchSession, ctx.key, uiLatencyOutcomeError)
	}
	if ctx.key != "" && ctx.key == m.loadingKey {
		m.loading = false
		m.setContentText("Error loading history.")
	}
}

func (m *Model) applyLiveSessionItemsSnapshot(ctx sessionItemsMessageContext) bool {
	if m == nil {
		return false
	}
	if !m.shouldKeepLiveCodexSnapshot(ctx.provider, ctx.id) {
		return false
	}
	if ctx.key != "" {
		m.cacheTranscriptBlocks(ctx.key, m.activeTranscriptBlocks(ctx.provider))
		m.finishUILatencyAction(uiLatencyActionSwitchSession, ctx.key, uiLatencyOutcomeOK)
	}
	m.setBackgroundStatus(string(ctx.source) + " refreshed")
	return true
}

func (m *Model) projectAndApplySessionItems(ctx sessionItemsMessageContext, items []map[string]any) tea.Cmd {
	previous := m.currentBlocks()
	approvals := normalizeApprovalRequests(m.sessionApprovals[ctx.id])
	resolutions := normalizeApprovalResolutions(m.sessionApprovalResolutions[ctx.id])
	if cmd := m.asyncSessionProjectionCmd(ctx.source, ctx.id, ctx.key, ctx.provider, items, previous, approvals, resolutions); cmd != nil {
		return cmd
	}
	blocks := projectSessionBlocksFromItems(ctx.provider, items, previous, approvals, resolutions)
	blocks = m.hydrateSessionBlocksForProvider(ctx.provider, blocks)
	m.applySessionProjection(ctx.source, ctx.id, ctx.key, blocks)
	return nil
}

func (m *Model) applySessionProjection(source sessionProjectionSource, id, key string, blocks []ChatBlock) {
	if m.transcriptViewportVisible() {
		m.setSnapshotBlocks(blocks)
		m.noteRequestVisibleUpdate(id)
	}
	if key != "" {
		m.cacheTranscriptBlocks(key, blocks)
		m.finishUILatencyAction(uiLatencyActionSwitchSession, key, uiLatencyOutcomeOK)
	} else if id == m.selectedSessionID() {
		m.cacheTranscriptBlocks(m.selectedKey(), blocks)
	}
	m.setBackgroundStatus(string(source) + " updated")
}

func (m *Model) asyncSessionProjectionCmd(
	source sessionProjectionSource,
	id, key, provider string,
	items []map[string]any,
	previous []ChatBlock,
	approvals []*ApprovalRequest,
	resolutions []*ApprovalResolution,
) tea.Cmd {
	if m == nil {
		return nil
	}
	policy := m.sessionProjectionPolicyOrDefault()
	if !policy.ShouldProjectAsync(len(items)) {
		return nil
	}
	token := sessionProjectionToken(key, id)
	seq := m.nextSessionProjectionSeq(token, policy.MaxTrackedProjectionTokens())
	return projectSessionBlocksCmd(source, id, key, provider, items, previous, approvals, resolutions, seq)
}

func (m *Model) nextSessionProjectionSeq(token string, maxTracked int) int {
	if m == nil || strings.TrimSpace(token) == "" {
		return 0
	}
	if m.sessionProjectionLatest == nil {
		m.sessionProjectionLatest = map[string]int{}
	}
	m.sessionProjectionSeq++
	m.sessionProjectionLatest[token] = m.sessionProjectionSeq
	m.pruneSessionProjectionTokens(maxTracked)
	return m.sessionProjectionSeq
}

func (m *Model) pruneSessionProjectionTokens(maxTracked int) {
	if m == nil || maxTracked <= 0 || len(m.sessionProjectionLatest) <= maxTracked {
		return
	}
	excess := len(m.sessionProjectionLatest) - maxTracked
	for excess > 0 {
		oldestToken := ""
		oldestSeq := 0
		for token, seq := range m.sessionProjectionLatest {
			if oldestToken == "" || seq < oldestSeq {
				oldestToken = token
				oldestSeq = seq
			}
		}
		if oldestToken == "" {
			return
		}
		delete(m.sessionProjectionLatest, oldestToken)
		excess--
	}
}

func (m *Model) isCurrentSessionProjection(key, id string, seq int) bool {
	if m == nil || seq <= 0 {
		return true
	}
	token := sessionProjectionToken(key, id)
	if token == "" || m.sessionProjectionLatest == nil {
		return false
	}
	latest, ok := m.sessionProjectionLatest[token]
	if !ok {
		return false
	}
	return seq == latest
}

func (m *Model) consumeSessionProjectionToken(key, id string, seq int) {
	if m == nil || seq <= 0 || m.sessionProjectionLatest == nil {
		return
	}
	token := sessionProjectionToken(key, id)
	if token == "" {
		return
	}
	latest, ok := m.sessionProjectionLatest[token]
	if !ok || latest != seq {
		return
	}
	delete(m.sessionProjectionLatest, token)
}

func sessionProjectionToken(key, id string) string {
	key = strings.TrimSpace(key)
	if key != "" {
		return "key:" + key
	}
	id = strings.TrimSpace(id)
	if id != "" {
		return "id:" + id
	}
	return ""
}

func projectSessionBlocksCmd(
	source sessionProjectionSource,
	id, key, provider string,
	items []map[string]any,
	previous []ChatBlock,
	approvals []*ApprovalRequest,
	resolutions []*ApprovalResolution,
	seq int,
) tea.Cmd {
	itemsCopy := append([]map[string]any(nil), items...)
	previousCopy := append([]ChatBlock(nil), previous...)
	approvalsCopy := normalizeApprovalRequests(approvals)
	resolutionsCopy := normalizeApprovalResolutions(resolutions)
	return func() tea.Msg {
		blocks := projectSessionBlocksFromItems(provider, itemsCopy, previousCopy, approvalsCopy, resolutionsCopy)
		return sessionBlocksProjectedMsg{
			source:        source,
			id:            id,
			key:           key,
			provider:      provider,
			blocks:        blocks,
			projectionSeq: seq,
		}
	}
}

func isWorkflowRunNotFoundError(err error) bool {
	if err == nil {
		return false
	}
	var apiErr *client.APIError
	if errors.As(err, &apiErr) {
		return apiErr.StatusCode == http.StatusNotFound && strings.Contains(strings.ToLower(strings.TrimSpace(apiErr.Message)), "workflow run not found")
	}
	text := strings.ToLower(strings.TrimSpace(err.Error()))
	return strings.Contains(text, "workflow run not found")
}

func (m *Model) reduceStateMessages(msg tea.Msg) (bool, tea.Cmd) {
	switch msg := msg.(type) {
	case workflowTemplatesMsg:
		if m.guidedWorkflow == nil {
			return true, nil
		}
		if m.mode != uiModeGuidedWorkflow {
			return true, nil
		}
		if msg.err != nil {
			m.guidedWorkflow.SetTemplateLoadError(msg.err)
			m.setStatusError("guided workflow template load error: " + msg.err.Error())
			m.renderGuidedWorkflowContent()
			return true, nil
		}
		m.guidedWorkflow.SetTemplates(msg.templates)
		if len(msg.templates) == 0 {
			m.setStatusWarning("no workflow templates available")
		} else {
			m.setStatusInfo(fmt.Sprintf("loaded %d workflow template(s)", len(msg.templates)))
		}
		m.renderGuidedWorkflowContent()
		return true, nil
	case workflowRunsMsg:
		if msg.err != nil {
			m.setBackgroundError("guided workflow runs error: " + msg.err.Error())
			return true, nil
		}
		appStateChanged := m.setWorkflowRunsData(msg.runs)
		m.applySidebarItemsIfDirtyWithReason(sidebarApplyReasonBackground)
		if appStateChanged {
			return true, m.requestAppStateSaveCmd()
		}
		return true, nil
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
		nextSessions, nextMeta := m.reconcileSessionsRefreshPayload(msg.sessions, normalizeSessionMeta(msg.meta))
		m.setSessionsAndMeta(nextSessions, nextMeta)
		recentsStateSaveCmd := tea.Cmd(nil)
		if m.recents != nil {
			m.recents.ObserveSessions(nextSessions)
			m.recents.ObserveMeta(m.recentsMetaFallbackMap(), now)
			m.syncRecentsCompletionWatches()
			recentsStateSaveCmd = m.requestRecentsStateSaveCmd()
		}
		m.applySidebarItemsIfDirtyWithReason(sidebarApplyReasonBackground)
		saveSidebarExpansionCmd := tea.Cmd(nil)
		if m.pendingSelectID != "" && m.sidebar != nil {
			if m.sidebar.SelectBySessionID(m.pendingSelectID) {
				m.pendingSelectID = ""
				if m.syncAppStateSidebarExpansion() {
					saveSidebarExpansionCmd = m.requestAppStateSaveCmd()
				}
			}
		}
		saveStateCmd := tea.Batch(saveSidebarExpansionCmd, recentsStateSaveCmd)
		nextSelection := m.selectedSessionSnapshot()
		m.setBackgroundStatus(fmt.Sprintf("%d sessions", len(nextSessions)))
		recentsCmd := tea.Cmd(nil)
		if m.mode == uiModeRecents {
			m.refreshRecentsContent()
			recentsCmd = m.ensureRecentsPreviewForSelection()
		}
		if m.selectionLoadPolicyOrDefault().ShouldReloadOnSessionsUpdate(previousSelection, nextSelection) {
			if m.shouldSkipSelectionReloadOnSessionsUpdate(previousSelection, nextSelection) {
				if recentsCmd != nil && saveStateCmd != nil {
					return true, tea.Batch(recentsCmd, saveStateCmd)
				}
				if recentsCmd != nil {
					return true, recentsCmd
				}
				return true, saveStateCmd
			}
			selectionCmd := m.onSystemSelectionChangedImmediate()
			if recentsCmd != nil && selectionCmd != nil {
				selectionCmd = tea.Batch(recentsCmd, selectionCmd)
			} else if recentsCmd != nil {
				selectionCmd = recentsCmd
			}
			if selectionCmd != nil && saveStateCmd != nil {
				return true, tea.Batch(selectionCmd, saveStateCmd)
			}
			if selectionCmd != nil {
				return true, selectionCmd
			}
			return true, saveStateCmd
		}
		if recentsCmd != nil && saveStateCmd != nil {
			return true, tea.Batch(recentsCmd, saveStateCmd)
		}
		if recentsCmd != nil {
			return true, recentsCmd
		}
		return true, saveStateCmd
	case workspacesMsg:
		if msg.err != nil {
			m.setBackgroundError("workspaces error: " + msg.err.Error())
			return true, nil
		}
		m.setWorkspacesData(msg.workspaces)
		m.applySidebarItemsIfDirty()
		return true, m.fetchWorktreesForWorkspaces()
	case worktreesMsg:
		if msg.err != nil {
			m.setBackgroundError("worktrees error: " + msg.err.Error())
			return true, nil
		}
		m.setWorktreesData(msg.workspaceID, msg.worktrees)
		m.applySidebarItemsIfDirty()
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
			m.restoreRecentsFromAppState(msg.state)
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
	case clipboardResultMsg:
		if msg.err != nil {
			m.setCopyStatusError("copy failed: " + msg.err.Error())
			return true, nil
		}
		m.setCopyStatusInfo(msg.success)
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
		return true, m.handleSessionItemsMessage(sessionProjectionSourceTail, msg.id, msg.key, msg.items, msg.err)
	case historyMsg:
		return true, m.handleSessionItemsMessage(sessionProjectionSourceHistory, msg.id, msg.key, msg.items, msg.err)
	case sessionBlocksProjectedMsg:
		if !m.isCurrentSessionProjection(msg.key, msg.id, msg.projectionSeq) {
			return true, nil
		}
		blocks := m.hydrateSessionBlocksForProvider(msg.provider, msg.blocks)
		m.applySessionProjection(msg.source, msg.id, msg.key, blocks)
		m.consumeSessionProjectionToken(msg.key, msg.id, msg.projectionSeq)
		return true, nil
	case recentsPreviewMsg:
		return true, m.handleRecentsPreview(msg)
	case recentsTurnCompletedMsg:
		return true, m.handleRecentsTurnCompleted(msg)
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
			if m.mode == uiModeRecents {
				m.refreshRecentsContent()
			}
			return true, nil
		}
		now := time.Now().UTC()
		baselineTurnID := ""
		if m.recents != nil {
			baselineTurnID = m.recentsCompletionPolicyOrDefault().RunBaselineTurnID(msg.turnID, m.sessionMeta[msg.id])
		}
		m.noteSessionMetaActivity(msg.id, msg.turnID, now)
		watchCmd := tea.Cmd(nil)
		recentsStateSaveCmd := tea.Cmd(nil)
		if m.recents != nil {
			m.recents.StartRun(msg.id, baselineTurnID, now)
			m.refreshRecentsSidebarState()
			recentsStateSaveCmd = m.requestRecentsStateSaveCmd()
			watchCmd = m.beginRecentsCompletionWatch(msg.id, baselineTurnID)
		}
		if m.sidebar != nil {
			m.sidebar.updateUnreadSessions(m.sessions, m.sessionMeta)
		}
		m.startRequestActivity(msg.id, m.providerForSessionID(msg.id))
		m.setStatusInfo("message sent")
		m.clearPendingSend(msg.token)
		m.sessionMetaRefreshPending = true
		m.lastSessionMetaRefreshAt = now
		provider := m.providerForSessionID(msg.id)
		cmds := []tea.Cmd{m.fetchSessionsCmd(false)}
		if recentsStateSaveCmd != nil {
			cmds = append(cmds, recentsStateSaveCmd)
		}
		if watchCmd != nil {
			cmds = append(cmds, watchCmd)
		}
		if m.mode == uiModeRecents {
			m.refreshRecentsContent()
		}
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
		watchCmd := tea.Cmd(nil)
		recentsStateSaveCmd := tea.Cmd(nil)
		if m.recents != nil {
			baseline := ""
			baseline = m.recentsCompletionPolicyOrDefault().RunBaselineTurnID("", m.sessionMeta[msg.session.ID])
			m.recents.StartRun(msg.session.ID, baseline, time.Now().UTC())
			m.refreshRecentsSidebarState()
			recentsStateSaveCmd = m.requestRecentsStateSaveCmd()
			watchCmd = m.beginRecentsCompletionWatch(msg.session.ID, baseline)
		}
		m.setStatusInfo("session started")
		initialLines := m.historyFetchLinesInitial()
		cmds := []tea.Cmd{m.fetchSessionsCmd(false), fetchHistoryCmd(m.sessionHistoryAPI, msg.session.ID, key, initialLines)}
		if recentsStateSaveCmd != nil {
			cmds = append(cmds, recentsStateSaveCmd)
		}
		if watchCmd != nil {
			cmds = append(cmds, watchCmd)
		}
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

func (m *Model) reconcileSessionsRefreshPayload(sessions []*types.Session, meta map[string]*types.SessionMeta) ([]*types.Session, map[string]*types.SessionMeta) {
	if m == nil {
		return sessions, meta
	}
	selected := m.selectedItem()
	if selected == nil || selected.session == nil {
		return sessions, meta
	}
	selectedID := strings.TrimSpace(selected.session.ID)
	if selectedID == "" || sessionSliceContainsID(sessions, selectedID) {
		return sessions, meta
	}
	selectedMeta := m.sessionMeta[selectedID]
	if selectedMeta == nil {
		selectedMeta = selected.meta
	}
	if selectedMeta == nil {
		return sessions, meta
	}
	if selectedMeta.DismissedAt != nil {
		return sessions, meta
	}
	if strings.TrimSpace(selectedMeta.WorkflowRunID) == "" {
		return sessions, meta
	}
	if !isVisibleStatus(selected.session.Status) {
		return sessions, meta
	}
	nextSessions := append([]*types.Session(nil), sessions...)
	nextSessions = append(nextSessions, selected.session)
	if meta == nil {
		meta = map[string]*types.SessionMeta{}
	}
	if _, ok := meta[selectedID]; !ok {
		meta[selectedID] = selectedMeta
	}
	return nextSessions, meta
}

func sessionSliceContainsID(sessions []*types.Session, sessionID string) bool {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return false
	}
	for _, session := range sessions {
		if session == nil {
			continue
		}
		if strings.TrimSpace(session.ID) == sessionID {
			return true
		}
	}
	return false
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
	blocks := projectSessionBlocksFromItems(
		provider,
		items,
		previous,
		normalizeApprovalRequests(m.sessionApprovals[sessionID]),
		normalizeApprovalResolutions(m.sessionApprovalResolutions[sessionID]),
	)
	return m.hydrateSessionBlocksForProvider(provider, blocks)
}

func projectSessionBlocksFromItems(
	provider string,
	items []map[string]any,
	previous []ChatBlock,
	approvals []*ApprovalRequest,
	resolutions []*ApprovalResolution,
) []ChatBlock {
	blocks := itemsToBlocks(items)
	if provider == "codex" {
		blocks = coalesceAdjacentReasoningBlocks(blocks)
	}
	if providerSupportsApprovals(provider) {
		blocks = mergeApprovalBlocks(blocks, approvals, resolutions)
		blocks = preserveApprovalPositions(previous, blocks)
	}
	return blocks
}

func (m *Model) hydrateSessionBlocksForProvider(provider string, blocks []ChatBlock) []ChatBlock {
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
