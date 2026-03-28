package app

import (
	"context"
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
	case composeFileSearchDebounceMsg:
		return true, m.handleComposeFileSearchDebounce(msg)
	case composeFileSearchStartedMsg:
		return true, m.applyComposeFileSearchStarted(msg)
	case composeFileSearchUpdatedMsg:
		return true, m.applyComposeFileSearchUpdated(msg)
	case composeFileSearchStreamMsg:
		return true, m.applyComposeFileSearchStream(msg)
	case composeFileSearchResultsMsg:
		return true, m.applyComposeFileSearchResults(msg)
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
			runID = strings.TrimSpace(msg.run.ID)
		}
		if runID == "" {
			m.guidedWorkflow.SetCreateError(fmt.Errorf("guided workflow run id is missing"))
			m.setStatusError("guided workflow create error: missing run id")
			m.renderGuidedWorkflowContent()
			return true, nil
		}
		m.setStatusMessage("starting guided workflow run")
		m.guidedWorkflowStateTransitionGatewayOrDefault().ApplyRun(msg.run)
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
		if msg.run != nil && msg.run.Status == guidedworkflows.WorkflowRunStatusQueued {
			m.setStatusInfo("guided workflow queued: waiting for dependencies")
		} else {
			m.setStatusInfo("guided workflow running")
		}
		m.guidedWorkflowStateTransitionGatewayOrDefault().ApplyRun(msg.run)
		runID := strings.TrimSpace(m.guidedWorkflow.RunID())
		if runID == "" {
			return true, nil
		}
		m.guidedWorkflow.MarkRefreshQueued(time.Now().UTC())
		return true, fetchWorkflowRunSnapshotCmd(m.guidedWorkflowAPI, runID)
	case workflowRunStoppedMsg:
		if msg.err != nil {
			if m.guidedWorkflow != nil {
				m.guidedWorkflow.SetSnapshotError(msg.err)
				m.renderGuidedWorkflowContent()
			}
			m.setStatusError("guided workflow stop error: " + msg.err.Error())
			return true, nil
		}
		m.upsertWorkflowRun(msg.run)
		m.applySidebarItemsIfDirty()
		m.setStatusInfo("guided workflow stopped")
		if m.guidedWorkflow == nil {
			return true, fetchWorkflowRunsCmd(m.guidedWorkflowAPI, m.showDismissed)
		}
		m.guidedWorkflowStateTransitionGatewayOrDefault().ApplyRun(msg.run)
		runID := strings.TrimSpace(m.guidedWorkflow.RunID())
		if runID == "" {
			return true, nil
		}
		m.guidedWorkflow.MarkRefreshQueued(time.Now().UTC())
		return true, fetchWorkflowRunSnapshotCmd(m.guidedWorkflowAPI, runID)
	case workflowRunResumedMsg:
		if m.guidedWorkflow == nil {
			return true, nil
		}
		if msg.err != nil {
			m.guidedWorkflow.SetResumeError(msg.err)
			m.setStatusError("guided workflow resume error: " + msg.err.Error())
			m.renderGuidedWorkflowContent()
			return true, nil
		}
		m.upsertWorkflowRun(msg.run)
		m.applySidebarItemsIfDirty()
		m.setStatusInfo("guided workflow resumed")
		m.guidedWorkflowStateTransitionGatewayOrDefault().ApplyRun(msg.run)
		runID := strings.TrimSpace(m.guidedWorkflow.RunID())
		if runID == "" {
			return true, nil
		}
		m.guidedWorkflow.MarkRefreshQueued(time.Now().UTC())
		return true, fetchWorkflowRunSnapshotCmd(m.guidedWorkflowAPI, runID)
	case workflowRunRenamedMsg:
		if msg.err != nil {
			m.setStatusError("guided workflow rename error: " + msg.err.Error())
			return true, nil
		}
		if msg.run != nil {
			m.upsertWorkflowRun(msg.run)
			if m.guidedWorkflow != nil && strings.TrimSpace(m.guidedWorkflow.RunID()) == strings.TrimSpace(msg.run.ID) {
				m.guidedWorkflowStateTransitionGatewayOrDefault().ApplyRun(msg.run)
			}
		}
		m.applySidebarItemsIfDirty()
		m.setStatusInfo("workflow renamed")
		return true, fetchWorkflowRunsCmd(m.guidedWorkflowAPI, m.showDismissed)
	case workflowRunSnapshotMsg:
		if m.guidedWorkflow == nil {
			return true, nil
		}
		if msg.err != nil {
			m.setBackgroundError("guided workflow refresh error: " + msg.err.Error())
			if shouldApplyWorkflowSnapshotErrorToGuidedController(m) {
				m.guidedWorkflow.SetSnapshotError(msg.err)
				m.renderGuidedWorkflowContent()
			}
			return true, nil
		}
		shouldApplySnapshot := shouldApplyWorkflowSnapshotToGuidedController(m, msg.run)
		previousStatus := guidedworkflows.WorkflowRunStatus("")
		previousStatusKnown := false
		if shouldApplySnapshot && msg.run != nil {
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
		if shouldApplySnapshot {
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
			m.guidedWorkflowStateTransitionGatewayOrDefault().ApplySnapshot(msg.run, msg.timeline)
		}
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
		m.guidedWorkflowStateTransitionGatewayOrDefault().ApplyRun(msg.run)
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
			cmds = append(cmds, m.fetchWorktreesForWorkspace(msg.workspaceID))
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
			cmds = append(cmds, m.fetchWorktreesForWorkspace(msg.workspaceID))
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
			cmds = append(cmds, m.fetchWorktreesForWorkspace(msg.workspaceID))
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
			cmds = append(cmds, m.fetchWorktreesForWorkspace(msg.workspaceID))
		}
		return true, tea.Batch(cmds...)
	case updateWorkspaceMsg:
		if msg.err != nil {
			m.setStatusError("update workspace error: " + msg.err.Error())
			return true, nil
		}
		if m.mode == uiModeEditWorkspace {
			m.exitEditWorkspace("")
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

func (m *Model) handleSessionItemsMessage(source sessionProjectionSource, id, key string, items []map[string]any, err error, requestedLines int) tea.Cmd {
	if source == sessionProjectionSourceHistory {
		responseKey := strings.TrimSpace(key)
		if responseKey == "" && strings.TrimSpace(id) != "" {
			responseKey = "sess:" + strings.TrimSpace(id)
		}
		m.recordHistoryWindowFromResponse(responseKey, requestedLines, len(items), err)
	}
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
	ctx.provider = m.providerForSessionID(id)
	return ctx, true
}

func (m *Model) handleSessionItemsMessageError(ctx sessionItemsMessageContext, err error) {
	if m == nil || err == nil {
		return
	}
	if isCanceledRequestError(err) {
		return
	}
	m.setBackgroundError(string(ctx.source) + " error: " + err.Error())
	if ctx.key != "" && ctx.key == m.pendingSessionKey {
		m.finishSessionLoadLatencyForKey(ctx.key, uiLatencyOutcomeError)
	}
	if ctx.key != "" && ctx.key == m.loadingKey {
		m.clearSessionLoadingState()
		m.setContentText("Error loading history.")
	}
}

func (m *Model) applyLiveSessionItemsSnapshot(ctx sessionItemsMessageContext) bool {
	if m == nil {
		return false
	}
	if !m.shouldKeepLiveTranscriptSnapshot(ctx.id) {
		return false
	}
	visibleBlocks := m.applyOptimisticOverlay(ctx.id, m.activeTranscriptBlocks())
	if ctx.key != "" {
		m.cacheTranscriptBlocks(ctx.key, visibleBlocks)
	}
	visible := m.shouldApplySessionProjectionToVisible(ctx.id, ctx.key)
	if visible {
		outcome := m.setSnapshotBlocks(visibleBlocks)
		m.settleSessionLoadProjection(ctx.id, ctx.key, outcome, true, uiLatencyOutcomeOK)
	} else {
		m.settleSessionLoadProjection(ctx.id, ctx.key, viewportRenderOutcome{}, false, uiLatencyOutcomeOK)
	}
	m.setBackgroundStatus(string(ctx.source) + " refreshed")
	return true
}

func (m *Model) projectAndApplySessionItems(ctx sessionItemsMessageContext, items []map[string]any) tea.Cmd {
	previous := m.currentBlocks()
	approvals := normalizeApprovalRequests(m.sessionApprovals[ctx.id])
	resolutions := normalizeApprovalResolutions(m.sessionApprovalResolutions[ctx.id])
	rules := m.sessionBlockProjectionRules(ctx.id, ctx.provider)
	if cmd := m.asyncSessionProjectionCmd(ctx.source, ctx.id, ctx.key, ctx.provider, rules, items, previous, approvals, resolutions); cmd != nil {
		return cmd
	}
	blocks := projectSessionBlocksFromItems(ctx.provider, rules, items, previous, approvals, resolutions)
	m.applySessionProjection(ctx.source, ctx.id, ctx.key, blocks)
	return nil
}

func (m *Model) applySessionProjection(source sessionProjectionSource, id, key string, blocks []ChatBlock) {
	blocks = m.applyOptimisticOverlay(id, blocks)
	visible := m.shouldApplySessionProjectionToVisible(id, key)
	if visible {
		outcome := m.setSnapshotBlocks(blocks)
		m.settleSessionLoadProjection(id, key, outcome, true, uiLatencyOutcomeOK)
		m.noteRequestVisibleUpdate(id)
	} else {
		m.settleSessionLoadProjection(id, key, viewportRenderOutcome{}, false, uiLatencyOutcomeOK)
	}
	m.sessionProjectionPostProcessorOrDefault().PostProcessSessionProjection(m, SessionProjectionPostProcessInput{
		Source:    source,
		SessionID: id,
		Blocks:    blocks,
	})
	if key != "" {
		m.cacheTranscriptBlocks(key, blocks)
	} else if id == m.selectedSessionID() {
		m.cacheTranscriptBlocks(m.selectedKey(), blocks)
	}
	m.setBackgroundStatus(string(source) + " updated")
}

func (m *Model) shouldApplySessionProjectionToVisible(id, key string) bool {
	if m == nil || !m.transcriptViewportVisible() {
		return false
	}
	key = strings.TrimSpace(key)
	activeSessionID := strings.TrimSpace(m.activeContentSessionID())
	selectedKey := strings.TrimSpace(m.selectedKey())
	if key != "" {
		if selectedKey != "" && key == selectedKey {
			return true
		}
		if activeSessionID != "" && sessionIDFromSidebarKey(key) == activeSessionID {
			return true
		}
		return false
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return false
	}
	if activeSessionID == "" {
		return true
	}
	return id == activeSessionID
}

func (m *Model) asyncSessionProjectionCmd(
	source sessionProjectionSource,
	id, key, provider string,
	rules sessionBlockProjectionRules,
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
	ctx, seq := m.sessionProjectionCoordinatorOrDefault().Schedule(token, m.requestScopeContext(requestScopeSessionLoad))
	return projectSessionBlocksCmd(
		ctx,
		m.sessionBlockProjectorOrDefault(),
		source,
		id,
		key,
		provider,
		rules,
		items,
		previous,
		approvals,
		resolutions,
		seq,
	)
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
	ctx context.Context,
	projector SessionBlockProjector,
	source sessionProjectionSource,
	id, key, provider string,
	rules sessionBlockProjectionRules,
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
		if projector == nil {
			projector = defaultSessionBlockProjector{}
		}
		blocks, err := projector.ProjectSessionBlocks(ctx, SessionBlockProjectionInput{
			Provider:    provider,
			Rules:       rules,
			Items:       itemsCopy,
			Previous:    previousCopy,
			Approvals:   approvalsCopy,
			Resolutions: resolutionsCopy,
		})
		return sessionBlocksProjectedMsg{
			source:        source,
			id:            id,
			key:           key,
			provider:      provider,
			blocks:        blocks,
			projectionSeq: seq,
			err:           err,
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
			if m.pendingGuidedWorkflowSessionLookup != nil {
				m.pendingGuidedWorkflowSessionLookup = nil
				m.pendingSelectID = ""
			}
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
		lookupReq := m.pendingGuidedWorkflowSessionLookup
		lookupFailed := false
		if lookupReq != nil {
			resolved := m.resolveGuidedWorkflowSessionID(lookupReq.requestedSessionID)
			if resolved != "" {
				m.pendingSelectID = resolved
				m.ensureGuidedWorkflowSessionVisible(resolved)
			}
		}
		saveSidebarExpansionCmd := tea.Cmd(nil)
		if m.pendingSelectID != "" && m.sidebar != nil {
			if m.sidebar.SelectBySessionID(m.pendingSelectID) {
				m.pendingSelectID = ""
				if m.syncAppStateSidebarExpansion() {
					saveSidebarExpansionCmd = m.requestAppStateSaveCmd()
				}
			}
		}
		if lookupReq != nil {
			resolved := m.resolveGuidedWorkflowSessionID(lookupReq.requestedSessionID)
			if m.sidebar != nil && resolved != "" && m.sidebar.SelectBySessionID(resolved) {
				m.pendingGuidedWorkflowSessionLookup = nil
				m.pendingSelectID = ""
				m.setPendingWorkflowTurnFocus(resolved, lookupReq.turnID)
				m.exitGuidedWorkflow("opened linked session " + resolved)
				if m.syncAppStateSidebarExpansion() {
					saveSidebarExpansionCmd = m.requestAppStateSaveCmd()
				}
			} else {
				m.pendingGuidedWorkflowSessionLookup = nil
				m.pendingSelectID = ""
				lookupFailed = true
				m.setValidationStatus("linked session not found: " + lookupReq.requestedSessionID)
				if m.mode == uiModeGuidedWorkflow {
					m.renderGuidedWorkflowContent()
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
		if lookupFailed {
			if recentsCmd != nil && saveStateCmd != nil {
				return true, tea.Batch(recentsCmd, saveStateCmd)
			}
			if recentsCmd != nil {
				return true, recentsCmd
			}
			return true, saveStateCmd
		}
		reloadDecision := m.sessionReloadPolicyOrDefault().DecideReload(previousSelection, nextSelection)
		if reloadDecision.Reload {
			m.sessionReloadCoalescerOrDefault().Reset()
			if m.shouldSkipSelectionReloadOnSessionsUpdate(previousSelection, nextSelection) {
				m.recordTranscriptBoundaryMetric(newSessionReloadMetric(
					classifySessionReloadSkipReason(previousSelection, nextSelection, m.mode, m.follow),
					transcriptOutcomeSkipped,
					transcriptSourceSessionsWithMeta,
					nextSelection.sessionID,
					m.providerForSessionID(nextSelection.sessionID),
				))
				if recentsCmd != nil && saveStateCmd != nil {
					return true, tea.Batch(recentsCmd, saveStateCmd)
				}
				if recentsCmd != nil {
					return true, recentsCmd
				}
				return true, saveStateCmd
			}
			m.recordTranscriptBoundaryMetric(newSessionReloadMetric(
				reloadDecision.Reason,
				transcriptOutcomeSuccess,
				transcriptSourceSessionsWithMeta,
				nextSelection.sessionID,
				m.providerForSessionID(nextSelection.sessionID),
			))
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
		reason := m.sessionReloadCoalescerOrDefault().NoopReason(reloadDecision, nextSelection, now)
		m.recordTranscriptBoundaryMetric(newSessionReloadMetric(
			reason,
			transcriptOutcomeNoop,
			transcriptSourceSessionsWithMeta,
			nextSelection.sessionID,
			m.providerForSessionID(nextSelection.sessionID),
		))
		if recentsCmd != nil && saveStateCmd != nil {
			return true, tea.Batch(recentsCmd, saveStateCmd)
		}
		if recentsCmd != nil {
			return true, recentsCmd
		}
		return true, saveStateCmd
	case workspacesMsg:
		if msg.err != nil {
			if isCanceledRequestError(msg.err) {
				return true, nil
			}
			m.setBackgroundError("workspaces error: " + msg.err.Error())
			return true, nil
		}
		m.setWorkspacesData(msg.workspaces)
		m.applySidebarItemsIfDirty()
		return true, m.fetchWorktreesForWorkspaces()
	case worktreesMsg:
		if msg.err != nil {
			if isCanceledRequestError(msg.err) {
				return true, nil
			}
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
		// Force a viewport pass after deferred panel layout updates.
		m.renderedForWidth = 0
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
	case themePreferenceSavedMsg:
		if normalizeThemeID(msg.themeID) != normalizeThemeID(m.themeID) {
			return true, nil
		}
		if msg.err != nil {
			m.setStatusError("theme save error: " + msg.err.Error())
			return true, nil
		}
		return true, nil
	case clipboardResultMsg:
		if msg.err != nil {
			m.setCopyStatusError("copy failed: " + msg.err.Error())
			return true, nil
		}
		m.setCopyStatusInfo(msg.success)
		return true, nil
	case fileLinkOpenResultMsg:
		if msg.err != nil {
			m.setStatusError("open link failed: " + msg.err.Error())
			return true, nil
		}
		if strings.TrimSpace(msg.target) == "" {
			m.setStatusInfo("link opened")
			return true, nil
		}
		m.setStatusInfo("opened " + msg.target)
		return true, nil
	case providerOptionsMsg:
		return true, m.handleProviderOptionsMsg(msg)
	case tailMsg:
		return true, m.handleSessionItemsMessage(sessionProjectionSourceTail, msg.id, msg.key, msg.items, msg.err, msg.requestedLines)
	case historyMsg:
		return true, m.handleSessionItemsMessage(sessionProjectionSourceHistory, msg.id, msg.key, msg.items, msg.err, msg.requestedLines)
	case transcriptSnapshotMsg:
		return true, m.applyTranscriptSnapshotMsg(msg)
	case sessionBlocksProjectedMsg:
		token := sessionProjectionToken(msg.key, msg.id)
		coordinator := m.sessionProjectionCoordinatorOrDefault()
		current := coordinator.IsCurrent(token, msg.projectionSeq)
		if msg.err != nil {
			if current {
				coordinator.Consume(token, msg.projectionSeq)
			}
			if isCanceledRequestError(msg.err) {
				return true, nil
			}
			m.setBackgroundError("session projection error: " + msg.err.Error())
			return true, nil
		}
		if !current {
			m.recordTranscriptBoundaryMetric(newStaleRevisionDropMetric(
				classifyProjectionDropReason(msg.key, msg.id, msg.projectionSeq, coordinator.LatestByToken()),
				transcriptSourceSessionBlocksProject,
				msg.id,
				msg.provider,
			))
			return true, nil
		}
		m.applySessionProjection(msg.source, msg.id, msg.key, msg.blocks)
		coordinator.Consume(token, msg.projectionSeq)
		return true, nil
	case debugPanelProjectedMsg:
		m.applyDebugPanelProjection(msg)
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
		if m.sessionProjectionCoordinatorOrDefault().HasPending(sessionProjectionToken(msg.key, msg.id)) {
			return true, historyPollCmd(msg.id, msg.key, msg.attempt+1, historyPollDelay, msg.minAgents)
		}
		provider := m.providerForSessionID(msg.id)
		ctx := m.requestScopeContext(requestScopeSessionLoad)
		cmds := []tea.Cmd{fetchHistoryCmdWithContext(m.sessionHistoryAPI, msg.id, msg.key, m.historyFetchLinesInitial(), ctx)}
		if decision := m.approvalRefreshDecision(msg.id, provider, transcriptSourceAutoRefreshHistory); decision.ShouldFetch {
			cmds = append(cmds, fetchApprovalsCmdWithContext(m.sessionAPI, msg.id, ctx))
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
		m.clearPendingSend(msg.token, msg.turnID)
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
		if decision := m.approvalRefreshDecision(msg.id, provider, transcriptSourceSendMsg); decision.ShouldFetch {
			cmds = append(cmds, fetchApprovalsCmdWithContext(m.sessionAPI, msg.id, m.requestScopeContext(requestScopeSessionLoad)))
		}
		reconnectCmds := m.sessionBootstrapCoordinatorOrDefault().BuildReconnectCommands(SessionReconnectBootstrapInput{
			Provider:                  provider,
			SessionID:                 msg.id,
			AfterRevision:             m.activeTranscriptRevision(),
			TranscriptAPI:             m.sessionTranscriptAPI,
			TranscriptStreamConnected: m.transcriptStream != nil && m.transcriptStream.HasStream(),
			OpenSource:                transcriptAttachmentSourceSendReconnect,
			OpenTranscriptCmdBuilder: func(sessionID, afterRevision string, source TranscriptAttachmentSource) tea.Cmd {
				return m.requestTranscriptStreamOpenCmd(sessionID, afterRevision, source, transcriptSourceSendMsg)
			},
		})
		if len(reconnectCmds) > 0 {
			cmds = append(cmds, reconnectCmds...)
			if m.appState.DebugStreamsEnabled {
				debugCtx := m.replaceRequestScope(requestScopeDebugStream)
				cmds = append(cmds, openDebugStreamCmdWithContext(m.sessionAPI, msg.id, debugCtx))
			}
			return true, tea.Batch(cmds...)
		}
		if m.appState.DebugStreamsEnabled && m.debugStream != nil && !m.debugStream.HasStream() {
			debugCtx := m.replaceRequestScope(requestScopeDebugStream)
			cmds = append(cmds, openDebugStreamCmdWithContext(m.sessionAPI, msg.id, debugCtx))
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
			m.pendingApproval = m.approvalStateServiceOrDefault().LatestRequest(m.sessionApprovals[msg.id])
			m.refreshVisibleApprovalBlocks(msg.id)
		}
		m.setStatusInfo("approval sent")
		return true, nil
	case approvalsMsg:
		if msg.err != nil {
			if isCanceledRequestError(msg.err) {
				return true, nil
			}
			m.setBackgroundError("approvals error: " + msg.err.Error())
			return true, nil
		}
		requests := approvalRequestsFromRecords(msg.approvals)
		m.setApprovalsForSession(msg.id, requests)
		if msg.id != m.selectedSessionID() {
			return true, nil
		}
		m.pendingApproval = m.approvalStateServiceOrDefault().LatestRequest(m.sessionApprovals[msg.id])
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
		m.clearComposeInterruptRequest(msg.id)
		if msg.err != nil {
			m.setStatusError("interrupt error: " + msg.err.Error())
			return true, nil
		}
		m.setStatusInfo("interrupt sent")
		m.stopRequestActivityFor(msg.id)
		if m.recents != nil {
			m.recents.CancelRun(msg.id)
			m.refreshRecentsSidebarState()
			if m.mode == uiModeRecents {
				m.refreshRecentsContent()
			}
		}
		if m.recentsCompletionWatching != nil {
			delete(m.recentsCompletionWatching, strings.TrimSpace(msg.id))
			m.cancelRequestScope(recentsRequestScopeName(msg.id))
		}
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
		m.cancelRequestScope(requestScopeSessionStart)
		if msg.err != nil {
			if isCanceledRequestError(msg.err) {
				m.clearSessionLoadingState()
				m.stopRequestActivity()
				return true, nil
			}
			m.setStatusError("start session error: " + msg.err.Error())
			m.clearSessionLoadingState()
			m.stopRequestActivity()
			return true, nil
		}
		if msg.session == nil || msg.session.ID == "" {
			m.setStatusError("start session error: no session returned")
			m.clearSessionLoadingState()
			return true, nil
		}
		m.resetStreamWithReason(transcriptResetReasonStartSessionResponse)
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
		if m.pendingTranscriptSnapshotRetryCount != nil {
			delete(m.pendingTranscriptSnapshotRetryCount, key)
		}
		m.loading = true
		m.loadingKey = key
		m.loadingRenderGeneration = 0
		m.loadingRenderLatencyOutcome = ""
		m.scrollOnLoad = true
		m.invalidateViewportRender()
		m.applyBlocksNoRender(nil)
		m.renderViewport()
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
		if m.historyWindowBySessionKey == nil {
			m.historyWindowBySessionKey = map[string]int{}
		}
		m.historyWindowBySessionKey[key] = initialLines
		if m.historyTraverseExhausted == nil {
			m.historyTraverseExhausted = map[string]bool{}
		}
		m.historyTraverseExhausted[key] = false
		if m.historyTraverseInFlight != nil {
			delete(m.historyTraverseInFlight, key)
		}
		if m.snapshotHistoryBackfillRequested != nil {
			delete(m.snapshotHistoryBackfillRequested, key)
		}
		loadCtx := m.replaceRequestScope(requestScopeSessionLoad)
		cmds := []tea.Cmd{m.fetchSessionsCmd(false)}
		cmds = append(cmds, m.sessionBootstrapCoordinatorOrDefault().BuildSessionStartCommands(SessionStartBootstrapInput{
			Provider:      msg.session.Provider,
			Status:        msg.session.Status,
			SessionID:     msg.session.ID,
			SessionKey:    key,
			AfterRevision: m.activeTranscriptRevision(),
			InitialLines:  initialLines,
			LoadContext:   loadCtx,
			TranscriptAPI: m.sessionTranscriptAPI,
			SessionAPI:    m.sessionAPI,
			OpenSource:    transcriptAttachmentSourceSessionStart,
		})...)
		if recentsStateSaveCmd != nil {
			cmds = append(cmds, recentsStateSaveCmd)
		}
		if watchCmd != nil {
			cmds = append(cmds, watchCmd)
		}
		if m.appState.DebugStreamsEnabled {
			debugCtx := m.replaceRequestScope(requestScopeDebugStream)
			cmds = append(cmds, openDebugStreamCmdWithContext(m.sessionAPI, msg.session.ID, debugCtx))
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
	case transcriptStreamMsg:
		m.applyTranscriptStreamMsg(msg)
		return true, nil
	case debugStreamMsg:
		m.applyDebugStreamMsg(msg)
		return true, nil
	case metadataStreamMsg:
		return true, m.applyMetadataStreamMsg(msg)
	case metadataStreamReconnectMsg:
		return true, openMetadataStreamCmd(m.metadataStreamAPI, strings.TrimSpace(m.metadataStreamRevision))
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

func (m *Model) shouldKeepLiveTranscriptSnapshot(sessionID string) bool {
	if !m.requestActivity.active {
		return false
	}
	return strings.TrimSpace(m.requestActivity.sessionID) == strings.TrimSpace(sessionID) && m.requestActivity.eventCount > 0
}

func (m *Model) activeTranscriptBlocks() []ChatBlock {
	if m.transcriptStream != nil {
		return m.transcriptStream.Blocks()
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
	rules := m.sessionBlockProjectionRules(sessionID, provider)
	blocks := projectSessionBlocksFromItems(
		provider,
		rules,
		items,
		previous,
		normalizeApprovalRequests(m.sessionApprovals[sessionID]),
		normalizeApprovalResolutions(m.sessionApprovalResolutions[sessionID]),
	)
	return blocks
}

type sessionBlockProjectionRules struct {
	CoalesceReasoning bool
	SupportsApprovals bool
}

func (m *Model) sessionBlockProjectionRules(sessionID, provider string) sessionBlockProjectionRules {
	rules := sessionBlockProjectionRules{
		CoalesceReasoning: true,
		SupportsApprovals: providerSupportsApprovals(provider),
	}
	if capabilities, ok := m.sessionTranscriptCapabilitiesForSession(sessionID); ok && capabilities != nil {
		rules.SupportsApprovals = capabilities.SupportsApprovals
	}
	return rules
}

func projectSessionBlocksFromItems(
	provider string,
	rules sessionBlockProjectionRules,
	items []map[string]any,
	previous []ChatBlock,
	approvals []*ApprovalRequest,
	resolutions []*ApprovalResolution,
) []ChatBlock {
	blocks, _ := projectSessionBlocksFromItemsWithContext(
		context.Background(),
		provider,
		rules,
		items,
		previous,
		approvals,
		resolutions,
	)
	return blocks
}

const projectionCancellationCheckInterval = 32

func projectionContextCheckNeeded(index int) bool {
	return index%projectionCancellationCheckInterval == 0
}

func projectionContextError(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

func projectSessionBlocksFromItemsWithContext(
	ctx context.Context,
	provider string,
	rules sessionBlockProjectionRules,
	items []map[string]any,
	previous []ChatBlock,
	approvals []*ApprovalRequest,
	resolutions []*ApprovalResolution,
) ([]ChatBlock, error) {
	if err := projectionContextError(ctx); err != nil {
		return nil, err
	}
	blocks, err := itemsToBlocksWithContext(ctx, items)
	if err != nil {
		return nil, err
	}
	if rules.CoalesceReasoning {
		blocks, err = coalesceAdjacentReasoningBlocksWithContext(ctx, blocks)
		if err != nil {
			return nil, err
		}
	}
	if rules.SupportsApprovals {
		approvals = filterApprovalRequestsForProvider(provider, approvals)
		resolutions = filterApprovalResolutionsForProvider(provider, resolutions)
		blocks, err = mergeApprovalBlocksWithContext(ctx, blocks, approvals, resolutions)
		if err != nil {
			return nil, err
		}
		blocks, err = preserveApprovalPositionsWithContext(ctx, previous, blocks)
		if err != nil {
			return nil, err
		}
	}
	if err := projectionContextError(ctx); err != nil {
		return nil, err
	}
	return blocks, nil
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
	ok, _ := m.streamMessageTargetDisposition(id, cancel)
	return ok
}

func (m *Model) streamMessageTargetDisposition(id string, cancel func()) (bool, string) {
	if id == m.activeStreamTargetID() {
		return true, transcriptReasonReconnectMatchedSession
	}
	if cancel != nil {
		cancel()
	}
	return false, transcriptReasonReconnectMismatchedSession
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

func (m *Model) applyTranscriptSnapshotMsg(msg transcriptSnapshotMsg) tea.Cmd {
	source := normalizeTranscriptAttachmentSource(msg.source)
	responseKey := transcriptSnapshotResponseKey(msg)
	if handled, cmd := m.handleTranscriptSnapshotError(msg, source, responseKey); handled {
		return cmd
	}
	if msg.snapshot != nil {
		m.recordHistoryWindowFromResponse(responseKey, msg.requestedLines, -1, nil)
	}
	if m.shouldDropTranscriptSnapshotByKey(msg) {
		return nil
	}
	if msg.snapshot == nil {
		m.recordHistoryWindowFromResponse(responseKey, msg.requestedLines, -1, nil)
		return m.maybeOpenTranscriptFollowAfterSnapshot(msg.id, source, "")
	}
	blocks, authoritativeSnapshot, applied := m.applyTranscriptSnapshotPayload(msg, source)
	if !applied {
		return nil
	}
	m.setSessionTranscriptCapabilities(msg.id, msg.snapshot.Capabilities)
	blocks = m.projectTranscriptSnapshotBlocks(msg.id, blocks)
	m.applySessionProjection(sessionProjectionSourceHistory, msg.id, msg.key, blocks)
	m.appendTranscriptSessionTrace(
		msg.id,
		"snapshot_applied source=%s revision=%s authoritative=%t",
		source,
		msg.snapshot.Revision,
		authoritativeSnapshot,
	)
	return m.transcriptSnapshotFollowUps(msg, source, responseKey, blocks)
}

func (m *Model) maybeOpenTranscriptFollowAfterSnapshot(sessionID string, source TranscriptAttachmentSource, afterRevision string) tea.Cmd {
	if m == nil || m.sessionTranscriptAPI == nil {
		return nil
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil
	}
	source = normalizeTranscriptAttachmentSource(source)
	decision := m.transcriptFollowStrategyRegistryOrDefault().StrategyFor(source).Decide(TranscriptFollowOpenRequest{
		SessionID:     sessionID,
		Source:        source,
		AfterRevision: strings.TrimSpace(afterRevision),
	})
	if !decision.Open || strings.TrimSpace(decision.ReconnectSource) == "" {
		return nil
	}
	m.appendTranscriptSessionTrace(
		sessionID,
		"follow_open source=%s after_revision=%s",
		source,
		strings.TrimSpace(afterRevision),
	)
	return m.requestTranscriptStreamOpenCmd(sessionID, afterRevision, source, decision.ReconnectSource)
}

func (m *Model) maybeBackfillSnapshotMissingUserTurns(sessionID, key string, blocks []ChatBlock) tea.Cmd {
	if m == nil || m.sessionHistoryAPI == nil {
		return nil
	}
	key = strings.TrimSpace(key)
	sessionID = strings.TrimSpace(sessionID)
	if key == "" || sessionID == "" {
		return nil
	}
	if hasChatRole(blocks, ChatRoleUser) || !hasChatRole(blocks, ChatRoleAgent) {
		return nil
	}
	if m.snapshotHistoryBackfillRequested == nil {
		m.snapshotHistoryBackfillRequested = map[string]bool{}
	}
	if m.snapshotHistoryBackfillRequested[key] {
		return nil
	}
	m.snapshotHistoryBackfillRequested[key] = true
	ctx := m.requestScopeContext(requestScopeSessionLoad)
	m.setBackgroundStatus("backfilling missing user turns")
	lines := m.historyFetchLinesInitial()
	if m.historyWindowBySessionKey != nil {
		if window := m.historyWindowBySessionKey[key]; window > lines {
			lines = window
		}
	}
	return fetchHistoryCmdWithContext(m.sessionHistoryAPI, sessionID, key, lines, ctx)
}

func hasChatRole(blocks []ChatBlock, role ChatRole) bool {
	for _, block := range blocks {
		if block.Role == role {
			return true
		}
	}
	return false
}

func (m *Model) applyTranscriptStreamMsg(msg transcriptStreamMsg) {
	provider := m.providerForSessionID(msg.id)
	if msg.err != nil {
		m.setBackgroundError("transcript stream error: " + msg.err.Error())
		m.recordReconnectOutcome(msg.id, provider, "transcript", transcriptSourceApplyEventsStream, transcriptOutcomeError, transcriptReasonReconnectStreamError)
		m.appendTranscriptSessionTrace(msg.id, "generation_open_error source=%s generation=%d err=%v", msg.source, msg.generation, msg.err)
		return
	}
	if ok, reason := m.streamMessageTargetDisposition(msg.id, msg.cancel); !ok {
		m.recordReconnectOutcome(msg.id, provider, "transcript", transcriptSourceApplyEventsStream, transcriptOutcomeSkipped, reason)
		m.appendTranscriptSessionTrace(msg.id, "generation_dropped source=%s generation=%d reason=%s", msg.source, msg.generation, reason)
		return
	}
	if msg.generation > 0 {
		decision := m.transcriptAttachmentCoordinatorOrDefault().Evaluate(msg.id, msg.generation)
		if !decision.Accept {
			if msg.cancel != nil {
				msg.cancel()
			}
			m.recordReconnectOutcome(msg.id, provider, "transcript", transcriptSourceApplyEventsStream, transcriptOutcomeDropped, decision.Reason)
			m.appendTranscriptSessionTrace(msg.id, "generation_dropped source=%s generation=%d reason=%s", msg.source, msg.generation, decision.Reason)
			return
		}
		m.appendTranscriptSessionTrace(
			msg.id,
			"generation_attached source=%s generation=%d after_revision=%s",
			msg.source,
			msg.generation,
			msg.revision,
		)
	} else {
		m.appendTranscriptSessionTrace(msg.id, "generation_attached source=%s generation=legacy after_revision=%s", msg.source, msg.revision)
	}
	if m.transcriptStream != nil {
		if msg.generation > 0 {
			m.transcriptStream.SetStreamWithGeneration(msg.ch, msg.cancel, msg.generation)
		} else {
			m.transcriptStream.SetStream(msg.ch, msg.cancel)
		}
	}
	m.setBackgroundStatus("streaming transcript")
	if strings.TrimSpace(msg.revision) != "" {
		m.recordReconnectOutcome(msg.id, provider, "transcript", transcriptSourceApplyEventsStream, transcriptOutcomeSuccess, transcriptReasonReconnectStreamAttached)
	} else {
		m.recordReconnectOutcome(msg.id, provider, "transcript", transcriptSourceApplyEventsStream, transcriptOutcomeSuccess, transcriptReasonReconnectMatchedSession)
	}
}

func (m *Model) applyDebugStreamMsg(msg debugStreamMsg) {
	m.cancelRequestScope(requestScopeDebugStream)
	if msg.err != nil {
		m.setBackgroundError("debug stream error: " + msg.err.Error())
		return
	}
	if !m.streamMessageTargetsActiveSession(msg.id, msg.cancel) {
		return
	}
	if m.debugStream != nil {
		m.debugStream.SetStream(msg.ch, msg.cancel)
	}
	m.setBackgroundStatus("streaming debug")
}

func (m *Model) applyMetadataStreamMsg(msg metadataStreamMsg) tea.Cmd {
	if msg.err != nil {
		m.setBackgroundError("metadata stream error: " + msg.err.Error())
		decision := m.metadataStreamRecoveryPolicyOrDefault().OnError(m.metadataStreamReconnectAttempts)
		m.metadataStreamReconnectAttempts = decision.NextAttempts
		cmds := make([]tea.Cmd, 0, 2)
		if decision.RefreshLists {
			cmds = append(cmds, tea.Batch(m.fetchSessionsCmd(false), fetchWorkflowRunsCmd(m.guidedWorkflowAPI, m.showDismissed)))
		}
		cmds = append(cmds, reconnectMetadataStreamCmd(decision.ReconnectDelay))
		return tea.Batch(cmds...)
	}
	if m == nil || m.metadataStream == nil {
		return nil
	}
	m.metadataStream.SetStream(msg.ch, msg.cancel)
	m.metadataStreamReconnectAttempts = m.metadataStreamRecoveryPolicyOrDefault().OnConnected().NextAttempts
	m.setBackgroundStatus("streaming metadata")
	return nil
}

func (m *Model) applyMetadataEvent(event types.MetadataEvent) {
	if m == nil {
		return
	}
	if revision := strings.TrimSpace(event.Revision); revision != "" {
		m.metadataStreamRevision = revision
	}
	result := m.metadataEventApplierOrDefault().Apply(m, event)
	if result.SidebarDirty {
		m.applySidebarItemsIfDirtyWithReason(sidebarApplyReasonBackground)
	}
	if result.GuidedWorkflowDirty && m.mode == uiModeGuidedWorkflow {
		m.renderGuidedWorkflowContent()
	}
}

func (m *Model) handleProviderOptionsMsg(msg providerOptionsMsg) tea.Cmd {
	provider := strings.ToLower(strings.TrimSpace(msg.provider))
	isPending := provider != "" && strings.EqualFold(provider, strings.TrimSpace(m.pendingComposeOptionFor))
	if msg.err != nil {
		return m.handleProviderOptionsError(msg.err, isPending)
	}
	if provider != "" && msg.options != nil {
		m.cacheProviderOptions(provider, msg.options)
		m.reconcileGuidedWorkflowRuntimeAfterProviderOptions(provider)
	}
	if isPending {
		return m.reopenPendingComposeOptionPicker(provider)
	}
	return nil
}

func (m *Model) handleProviderOptionsError(err error, isPending bool) tea.Cmd {
	if isCanceledRequestError(err) {
		return nil
	}
	if isPending {
		m.clearPendingComposeOptionRequest()
	}
	m.setBackgroundError("provider options error: " + err.Error())
	return nil
}

func (m *Model) cacheProviderOptions(provider string, options *types.ProviderOptionCatalog) {
	if m.providerOptions == nil {
		m.providerOptions = map[string]*types.ProviderOptionCatalog{}
	}
	m.providerOptions[provider] = options
}

func (m *Model) reconcileGuidedWorkflowRuntimeAfterProviderOptions(provider string) {
	if m.mode != uiModeGuidedWorkflow || m.guidedWorkflow == nil || m.guidedWorkflow.Stage() != guidedWorkflowStageSetup {
		return
	}
	if !strings.EqualFold(strings.TrimSpace(m.guidedWorkflow.Provider()), provider) {
		return
	}
	m.syncGuidedWorkflowRuntimeOptionsFromCompose()
	m.renderGuidedWorkflowContent()
}

func (m *Model) reopenPendingComposeOptionPicker(provider string) tea.Cmd {
	target := m.pendingComposeOptionTarget
	m.clearPendingComposeOptionRequest()
	canOpen := m.mode == uiModeCompose ||
		(m.mode == uiModeGuidedWorkflow && m.guidedWorkflow != nil && m.guidedWorkflow.Stage() == guidedWorkflowStageSetup)
	if canOpen && strings.EqualFold(m.composeProvider(), provider) {
		if m.openComposeOptionPicker(target) {
			m.setStatusMessage("select " + composeOptionLabel(target))
		} else {
			m.setValidationStatus("no " + composeOptionLabel(target) + " options available")
		}
	}
	return nil
}
