package app

import (
	"sort"
	"strings"
	"time"

	"control/internal/guidedworkflows"

	tea "charm.land/bubbletea/v2"
	xansi "github.com/charmbracelet/x/ansi"
)

func (m *Model) enterGuidedWorkflow(context guidedWorkflowLaunchContext) {
	if m == nil {
		return
	}
	context = m.resolveGuidedWorkflowLaunchContext(context)
	if m.guidedWorkflow == nil {
		m.guidedWorkflow = NewGuidedWorkflowUIController()
	}
	m.mode = uiModeGuidedWorkflow
	m.guidedWorkflow.Enter(context)
	m.guidedWorkflow.BeginTemplateLoad()
	m.resetGuidedWorkflowPromptInput()
	m.resetGuidedWorkflowResumeInput()
	if m.input != nil {
		m.input.FocusSidebar()
	}
	m.setStatusMessage("loading workflow templates")
	m.renderGuidedWorkflowContent()
}

func (m *Model) openGuidedWorkflowFromSidebar(item *sidebarItem) tea.Cmd {
	if m == nil || item == nil || item.kind != sidebarWorkflow {
		return nil
	}
	runID := strings.TrimSpace(item.workflowRunID())
	if runID == "" {
		m.setValidationStatus("workflow run id is missing")
		return nil
	}
	context := guidedWorkflowLaunchContext{}
	if item.workflow != nil {
		context.workspaceID = strings.TrimSpace(item.workflow.WorkspaceID)
		context.worktreeID = strings.TrimSpace(item.workflow.WorktreeID)
		context.sessionID = strings.TrimSpace(item.workflow.SessionID)
	}
	if strings.TrimSpace(context.workspaceID) == "" {
		context.workspaceID = strings.TrimSpace(item.workspaceID())
	}
	m.recordWorkflowRunState(&guidedworkflows.WorkflowRun{
		ID:          runID,
		WorkspaceID: strings.TrimSpace(context.workspaceID),
		WorktreeID:  strings.TrimSpace(context.worktreeID),
		SessionID:   strings.TrimSpace(context.sessionID),
	})
	m.enterGuidedWorkflow(context)
	if m.guidedWorkflow != nil {
		m.guidedWorkflow.SetRun(item.workflow)
	}
	m.setStatusMessage("opened guided workflow " + runID)
	m.renderGuidedWorkflowContent()
	return fetchWorkflowRunSnapshotCmd(m.guidedWorkflowAPI, runID)
}

func (m *Model) exitGuidedWorkflow(status string) {
	if m == nil {
		return
	}
	if m.guidedWorkflow != nil {
		m.guidedWorkflow.Exit()
	}
	if m.guidedWorkflowPromptInput != nil {
		m.guidedWorkflowPromptInput.Blur()
	}
	if m.guidedWorkflowResumeInput != nil {
		m.guidedWorkflowResumeInput.Blur()
	}
	if m.guidedWorkflowResumeInput != nil {
		m.guidedWorkflowResumeInput.Blur()
	}
	m.mode = uiModeNormal
	if m.input != nil {
		m.input.FocusSidebar()
	}
	if status != "" {
		m.setStatusMessage(status)
	}
	m.renderViewport()
}

func (m *Model) renderGuidedWorkflowContent() {
	if m == nil || m.mode != uiModeGuidedWorkflow {
		return
	}
	if m.guidedWorkflow == nil {
		m.guidedWorkflow = NewGuidedWorkflowUIController()
	}
	pickerWidth := max(minViewportWidth, m.viewport.Width())
	pickerHeight := clamp(m.viewport.Height()/2, 6, 10)
	m.guidedWorkflow.SetTemplatePickerSize(pickerWidth, pickerHeight)
	if m.guidedWorkflow.Stage() == guidedWorkflowStageSetup {
		m.syncGuidedWorkflowPromptInput()
	}
	if m.guidedWorkflow.Stage() == guidedWorkflowStageSummary && m.guidedWorkflow.CanResumeFailedRun() {
		m.primeGuidedWorkflowResumeInput()
		m.syncGuidedWorkflowResumeInput()
	}
	content := m.guidedWorkflow.Render()
	if m.guidedWorkflow.LauncherRequiresRawANSIRender() {
		m.setContentANSIText(content)
	} else {
		m.setContentText(content)
	}
	m.syncGuidedWorkflowInputFocus()
}

func (m *Model) maybeAutoRefreshGuidedWorkflow(now time.Time) tea.Cmd {
	if m == nil || m.mode != uiModeGuidedWorkflow || m.guidedWorkflow == nil {
		return nil
	}
	if !m.guidedWorkflow.CanRefresh(now, guidedWorkflowPollInterval) {
		return nil
	}
	runID := strings.TrimSpace(m.guidedWorkflow.RunID())
	if runID == "" {
		return nil
	}
	m.guidedWorkflow.MarkRefreshQueued(now)
	return fetchWorkflowRunSnapshotCmd(m.guidedWorkflowAPI, runID)
}

func (m *Model) reduceGuidedWorkflowMode(msg tea.Msg) (bool, tea.Cmd) {
	if m.mode != uiModeGuidedWorkflow {
		return false, nil
	}
	if handled, cmd := m.handleGuidedWorkflowSetupInput(msg); handled {
		return true, cmd
	}
	if handled, cmd := m.handleGuidedWorkflowResumeInput(msg); handled {
		return true, cmd
	}
	if pasteMsg, ok := msg.(tea.PasteMsg); ok {
		if m.guidedWorkflow != nil && m.guidedWorkflow.Stage() == guidedWorkflowStageLauncher {
			if m.applyPickerPaste(pasteMsg, m.guidedWorkflow) {
				m.renderGuidedWorkflowContent()
				return true, nil
			}
		}
	}
	keyMsg, ok := msg.(tea.KeyMsg)
	if ok {
		key := m.keyString(keyMsg)
		if m.guidedWorkflow != nil && m.guidedWorkflow.Stage() == guidedWorkflowStageLauncher {
			if key == "ctrl+r" || m.keyMatchesOverriddenCommand(keyMsg, KeyCommandRefresh, "r") {
				m.guidedWorkflow.BeginTemplateLoad()
				m.setStatusMessage("loading workflow templates")
				m.renderGuidedWorkflowContent()
				return true, fetchWorkflowTemplatesCmd(m.guidedWorkflowTemplateAPI)
			}
			if m.applyPickerTypeAhead(keyMsg, m.guidedWorkflow) {
				m.renderGuidedWorkflowContent()
				return true, nil
			}
			switch key {
			case "backspace", "ctrl+h", "ctrl+u":
				return true, nil
			}
			if pickerTypeAheadText(keyMsg) != "" {
				return true, nil
			}
		}
		switch {
		case m.keyMatchesCommand(keyMsg, KeyCommandQuit, "q"):
			return true, tea.Quit
		case m.keyMatchesCommand(keyMsg, KeyCommandToggleSidebar, "ctrl+b"):
			m.toggleSidebar()
			return true, m.requestAppStateSaveCmd()
		case m.keyMatchesCommand(keyMsg, KeyCommandToggleNotesPanel, "ctrl+o"):
			return true, m.toggleNotesPanel()
		case m.keyMatchesCommand(keyMsg, KeyCommandMenu, "ctrl+m"):
			if m.menu != nil {
				if m.contextMenu != nil {
					m.contextMenu.Close()
				}
				m.menu.Toggle()
			}
			return true, nil
		case m.keyMatchesCommand(keyMsg, KeyCommandDismissSelection, "d"):
			if m.guidedWorkflow != nil {
				if runID := strings.TrimSpace(m.guidedWorkflow.RunID()); runID != "" {
					m.confirmDismissWorkflow(runID)
					return true, nil
				}
			}
			m.enterDismissOrDeleteForSelection()
			return true, nil
		}
		switch key {
		case "esc":
			if m.guidedWorkflow != nil && m.guidedWorkflow.Stage() == guidedWorkflowStageSetup {
				m.openGuidedWorkflowLauncherFromSetup()
				return true, nil
			}
			if m.guidedWorkflow != nil && m.guidedWorkflow.Stage() == guidedWorkflowStageLauncher && m.guidedWorkflow.ClearQuery() {
				m.setStatusMessage("template filter cleared")
				m.renderGuidedWorkflowContent()
				return true, nil
			}
			m.exitGuidedWorkflow("guided workflow closed")
			return true, nil
		case "enter":
			return true, m.handleGuidedWorkflowEnter()
		case "down":
			if m.guidedWorkflow != nil {
				switch m.guidedWorkflow.Stage() {
				case guidedWorkflowStageLauncher:
					if m.guidedWorkflow.MoveTemplateSelection(1) {
						m.renderGuidedWorkflowContent()
					}
					return true, nil
				case guidedWorkflowStageSetup:
					m.guidedWorkflow.CycleSensitivity(1)
					m.renderGuidedWorkflowContent()
					return true, nil
				case guidedWorkflowStageLive, guidedWorkflowStageSummary:
					m.guidedWorkflow.MoveStepSelection(1)
					m.renderGuidedWorkflowContent()
					return true, nil
				}
			}
		case "up":
			if m.guidedWorkflow != nil {
				switch m.guidedWorkflow.Stage() {
				case guidedWorkflowStageLauncher:
					if m.guidedWorkflow.MoveTemplateSelection(-1) {
						m.renderGuidedWorkflowContent()
					}
					return true, nil
				case guidedWorkflowStageSetup:
					m.guidedWorkflow.CycleSensitivity(-1)
					m.renderGuidedWorkflowContent()
					return true, nil
				case guidedWorkflowStageLive, guidedWorkflowStageSummary:
					m.guidedWorkflow.MoveStepSelection(-1)
					m.renderGuidedWorkflowContent()
					return true, nil
				}
			}
		case "j":
			if m.guidedWorkflow != nil && (m.guidedWorkflow.Stage() == guidedWorkflowStageLive || m.guidedWorkflow.Stage() == guidedWorkflowStageSummary) {
				m.guidedWorkflow.MoveStepSelection(1)
				m.renderGuidedWorkflowContent()
				return true, nil
			}
		case "k":
			if m.guidedWorkflow != nil && (m.guidedWorkflow.Stage() == guidedWorkflowStageLive || m.guidedWorkflow.Stage() == guidedWorkflowStageSummary) {
				m.guidedWorkflow.MoveStepSelection(-1)
				m.renderGuidedWorkflowContent()
				return true, nil
			}
		case "r":
			if m.guidedWorkflow != nil {
				switch m.guidedWorkflow.Stage() {
				case guidedWorkflowStageLive, guidedWorkflowStageSummary:
					return true, m.refreshGuidedWorkflowNow("refreshing guided workflow timeline")
				}
			}
		case "a":
			if m.guidedWorkflow != nil && m.guidedWorkflow.NeedsDecision() {
				return true, m.decideGuidedWorkflow(guidedworkflows.DecisionActionApproveContinue)
			}
		case "v":
			if m.guidedWorkflow != nil && m.guidedWorkflow.NeedsDecision() {
				return true, m.decideGuidedWorkflow(guidedworkflows.DecisionActionRequestRevision)
			}
		case "p":
			if m.guidedWorkflow != nil && m.guidedWorkflow.NeedsDecision() {
				return true, m.decideGuidedWorkflow(guidedworkflows.DecisionActionPauseRun)
			}
		case "g":
			m.viewport.GotoTop()
			return true, nil
		case "G":
			m.viewport.GotoBottom()
			return true, nil
		case "o":
			if m.guidedWorkflow != nil && (m.guidedWorkflow.Stage() == guidedWorkflowStageLive || m.guidedWorkflow.Stage() == guidedWorkflowStageSummary) {
				return true, m.openGuidedWorkflowSelectedSession()
			}
		}
		if m.guidedWorkflow != nil && m.guidedWorkflow.Stage() == guidedWorkflowStageLauncher && m.applyPickerTypeAhead(keyMsg, m.guidedWorkflow) {
			m.renderGuidedWorkflowContent()
			return true, nil
		}
	}
	return false, nil
}

func (m *Model) resolveGuidedWorkflowLaunchContext(context guidedWorkflowLaunchContext) guidedWorkflowLaunchContext {
	if m == nil {
		return context
	}
	context.workspaceID = strings.TrimSpace(context.workspaceID)
	context.workspaceName = strings.TrimSpace(context.workspaceName)
	context.worktreeID = strings.TrimSpace(context.worktreeID)
	context.worktreeName = strings.TrimSpace(context.worktreeName)
	context.sessionID = strings.TrimSpace(context.sessionID)
	context.sessionName = strings.TrimSpace(context.sessionName)

	if context.worktreeID != "" {
		if wt := m.worktreeByID(context.worktreeID); wt != nil {
			if context.worktreeName == "" {
				context.worktreeName = strings.TrimSpace(wt.Name)
			}
			if context.workspaceID == "" {
				context.workspaceID = strings.TrimSpace(wt.WorkspaceID)
			}
		}
	}
	if context.workspaceID != "" && context.workspaceName == "" {
		if ws := m.workspaceByID(context.workspaceID); ws != nil {
			context.workspaceName = strings.TrimSpace(ws.Name)
		}
	}
	if context.sessionID != "" && context.sessionName == "" {
		context.sessionName = strings.TrimSpace(m.sessionDisplayName(context.sessionID))
	}
	return context
}

func (m *Model) handleGuidedWorkflowEnter() tea.Cmd {
	if m == nil || m.guidedWorkflow == nil {
		return nil
	}
	switch m.guidedWorkflow.Stage() {
	case guidedWorkflowStageLauncher:
		if m.guidedWorkflow.TemplatesLoading() {
			m.setValidationStatus("workflow templates are still loading")
			m.renderGuidedWorkflowContent()
			return nil
		}
		if loadErr := m.guidedWorkflow.TemplateLoadError(); loadErr != "" {
			m.setValidationStatus("workflow templates failed to load; press r to retry")
			m.renderGuidedWorkflowContent()
			return nil
		}
		if !m.guidedWorkflow.OpenSetup() {
			m.setValidationStatus("select a workflow template to continue")
			m.renderGuidedWorkflowContent()
			return nil
		}
		if m.guidedWorkflowPromptInput != nil {
			m.guidedWorkflowPromptInput.Focus()
		}
		m.setStatusMessage("guided workflow setup")
		m.reflowGuidedWorkflowLayout()
		return nil
	case guidedWorkflowStageSetup:
		return m.startGuidedWorkflowRun()
	case guidedWorkflowStageLive:
		if m.guidedWorkflow.NeedsDecision() {
			return m.decideGuidedWorkflow(m.guidedWorkflow.RecommendedDecisionAction())
		}
		return m.refreshGuidedWorkflowNow("refreshing guided workflow timeline")
	case guidedWorkflowStageSummary:
		if m.guidedWorkflow.CanResumeFailedRun() {
			return m.resumeFailedGuidedWorkflowRun()
		}
		m.exitGuidedWorkflow("guided workflow summary closed")
		return nil
	default:
		return nil
	}
}

func (m *Model) startGuidedWorkflowRun() tea.Cmd {
	if m == nil || m.guidedWorkflow == nil {
		return nil
	}
	m.syncGuidedWorkflowPromptInput()
	req := m.guidedWorkflow.BuildCreateRequest()
	if strings.TrimSpace(req.WorkspaceID) == "" && strings.TrimSpace(req.WorktreeID) == "" {
		m.setValidationStatus("guided workflow requires workspace or worktree context")
		return nil
	}
	if strings.TrimSpace(req.UserPrompt) == "" {
		m.setValidationStatus("enter a workflow prompt before starting")
		m.renderGuidedWorkflowContent()
		return nil
	}
	m.guidedWorkflow.BeginStart()
	if m.guidedWorkflowPromptInput != nil {
		m.guidedWorkflowPromptInput.Blur()
	}
	m.renderGuidedWorkflowContent()
	m.setStatusMessage("creating guided workflow run")
	return createWorkflowRunCmd(m.guidedWorkflowAPI, req)
}

func (m *Model) resumeFailedGuidedWorkflowRun() tea.Cmd {
	if m == nil || m.guidedWorkflow == nil {
		return nil
	}
	if !m.guidedWorkflow.CanResumeFailedRun() {
		m.setValidationStatus("guided workflow run is not resumable")
		return nil
	}
	runID := strings.TrimSpace(m.guidedWorkflow.RunID())
	if runID == "" {
		m.setValidationStatus("guided run id is missing")
		return nil
	}
	m.syncGuidedWorkflowResumeInput()
	req := m.guidedWorkflow.BuildResumeRequest()
	if m.guidedWorkflowResumeInput != nil {
		m.guidedWorkflowResumeInput.Blur()
	}
	m.setStatusMessage("resuming guided workflow run")
	m.renderGuidedWorkflowContent()
	return resumeFailedWorkflowRunCmd(m.guidedWorkflowAPI, runID, req)
}

func (m *Model) refreshGuidedWorkflowNow(status string) tea.Cmd {
	if m == nil || m.guidedWorkflow == nil {
		return nil
	}
	runID := strings.TrimSpace(m.guidedWorkflow.RunID())
	if runID == "" {
		m.setValidationStatus("no guided run to refresh")
		return nil
	}
	if status != "" {
		m.setStatusMessage(status)
	}
	now := m.clockNow
	if now.IsZero() {
		now = time.Now().UTC()
	}
	m.guidedWorkflow.MarkRefreshQueued(now)
	m.renderGuidedWorkflowContent()
	return fetchWorkflowRunSnapshotCmd(m.guidedWorkflowAPI, runID)
}

func (m *Model) decideGuidedWorkflow(action guidedworkflows.DecisionAction) tea.Cmd {
	if m == nil || m.guidedWorkflow == nil {
		return nil
	}
	if !m.guidedWorkflow.NeedsDecision() {
		m.setValidationStatus("no decision required")
		return nil
	}
	runID := strings.TrimSpace(m.guidedWorkflow.RunID())
	if runID == "" {
		m.setValidationStatus("guided run id is missing")
		return nil
	}
	switch action {
	case guidedworkflows.DecisionActionApproveContinue:
		m.setStatusMessage("approving guided workflow checkpoint")
	case guidedworkflows.DecisionActionRequestRevision:
		m.setStatusMessage("requesting guided workflow revision")
	case guidedworkflows.DecisionActionPauseRun:
		m.setStatusMessage("pausing guided workflow run")
	}
	return decideWorkflowRunCmd(m.guidedWorkflowAPI, runID, m.guidedWorkflow.BuildDecisionRequest(action))
}

func (m *Model) openGuidedWorkflowSelectedSession() tea.Cmd {
	if m == nil || m.guidedWorkflow == nil || m.sidebar == nil {
		return nil
	}
	sessionID := strings.TrimSpace(m.guidedWorkflow.SelectedStepSessionID())
	turnID := strings.TrimSpace(m.guidedWorkflow.SelectedStepTurnID())
	if sessionID == "" {
		m.setValidationStatus("selected step has no linked session")
		m.renderGuidedWorkflowContent()
		return nil
	}
	return m.openGuidedWorkflowSessionTurn(sessionID, turnID)
}

func (m *Model) openGuidedWorkflowSessionTurn(sessionID, turnID string) tea.Cmd {
	if m == nil || m.sidebar == nil {
		return nil
	}
	sessionID = strings.TrimSpace(sessionID)
	resolvedSessionID := m.resolveGuidedWorkflowSessionID(sessionID)
	turnID = strings.TrimSpace(turnID)
	if sessionID == "" {
		m.setValidationStatus("selected step has no linked session")
		m.renderGuidedWorkflowContent()
		return nil
	}
	if !m.sidebar.SelectBySessionID(resolvedSessionID) {
		m.ensureGuidedWorkflowSessionVisible(resolvedSessionID)
		if !m.sidebar.SelectBySessionID(resolvedSessionID) {
			m.pendingGuidedWorkflowSessionLookup = &guidedWorkflowSessionLookupRequest{
				requestedSessionID: sessionID,
				turnID:             turnID,
			}
			m.pendingSelectID = resolvedSessionID
			m.setStatusMessage("locating linked session " + sessionID)
			return m.fetchSessionsCmd(false)
		}
	}
	m.pendingGuidedWorkflowSessionLookup = nil
	m.pendingSelectID = ""
	m.setPendingWorkflowTurnFocus(resolvedSessionID, turnID)
	item := m.selectedItem()
	m.exitGuidedWorkflow("opened linked session " + resolvedSessionID)
	return m.batchWithNotesPanelSync(m.loadSelectedSession(item))
}

func (m *Model) ensureGuidedWorkflowSessionVisible(sessionID string) {
	if m == nil {
		return
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return
	}
	targetGroupIDs := m.workspaceGroupIDsForSession(sessionID)
	if len(targetGroupIDs) == 0 {
		return
	}
	current := append([]string(nil), m.appState.ActiveWorkspaceGroupIDs...)
	selected := map[string]struct{}{}
	for _, id := range current {
		trimmed := strings.TrimSpace(id)
		if trimmed == "" {
			continue
		}
		selected[trimmed] = struct{}{}
	}
	changed := false
	for _, id := range targetGroupIDs {
		if _, ok := selected[id]; ok {
			continue
		}
		selected[id] = struct{}{}
		current = append(current, id)
		changed = true
	}
	if !changed {
		return
	}
	next := normalizedWorkspaceGroupIDs(current)
	if m.menu != nil {
		m.menu.SetSelectedGroupIDs(next)
	}
	if m.setActiveWorkspaceGroupIDs(next) {
		m.applySidebarItemsIfDirty()
	}
}

func (m *Model) workspaceGroupIDsForSession(sessionID string) []string {
	if m == nil {
		return nil
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil
	}
	meta := m.sessionMeta[sessionID]
	if meta == nil {
		return nil
	}
	workspaceID := strings.TrimSpace(meta.WorkspaceID)
	if workspaceID == "" {
		return []string{"ungrouped"}
	}
	workspace := m.workspaceByID(workspaceID)
	if workspace == nil || len(workspace.GroupIDs) == 0 {
		return []string{"ungrouped"}
	}
	ids := make([]string, 0, len(workspace.GroupIDs))
	for _, id := range workspace.GroupIDs {
		trimmed := strings.TrimSpace(id)
		if trimmed == "" {
			continue
		}
		ids = append(ids, trimmed)
	}
	if len(ids) == 0 {
		return []string{"ungrouped"}
	}
	return normalizedWorkspaceGroupIDs(ids)
}

func normalizedWorkspaceGroupIDs(ids []string) []string {
	selected := map[string]struct{}{}
	for _, id := range ids {
		trimmed := strings.TrimSpace(id)
		if trimmed == "" {
			continue
		}
		selected[trimmed] = struct{}{}
	}
	if len(selected) == 0 {
		return nil
	}
	normalized := make([]string, 0, len(selected))
	for id := range selected {
		normalized = append(normalized, id)
	}
	sort.Strings(normalized)
	return normalized
}

func (m *Model) resolveGuidedWorkflowSessionID(sessionID string) string {
	if m == nil {
		return strings.TrimSpace(sessionID)
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return ""
	}
	for _, session := range m.sessions {
		if session == nil {
			continue
		}
		localSessionID := strings.TrimSpace(session.ID)
		if localSessionID == "" {
			continue
		}
		if localSessionID == sessionID {
			return localSessionID
		}
		meta := m.sessionMeta[localSessionID]
		if meta == nil {
			continue
		}
		if strings.TrimSpace(meta.ProviderSessionID) == sessionID ||
			strings.TrimSpace(meta.ThreadID) == sessionID ||
			strings.TrimSpace(meta.SessionID) == sessionID {
			return localSessionID
		}
	}
	for localSessionID, meta := range m.sessionMeta {
		localSessionID = strings.TrimSpace(localSessionID)
		if localSessionID == "" && meta != nil {
			localSessionID = strings.TrimSpace(meta.SessionID)
		}
		if localSessionID == "" {
			continue
		}
		if localSessionID == sessionID {
			return localSessionID
		}
		if meta == nil {
			continue
		}
		if strings.TrimSpace(meta.ProviderSessionID) == sessionID ||
			strings.TrimSpace(meta.ThreadID) == sessionID ||
			strings.TrimSpace(meta.SessionID) == sessionID {
			return localSessionID
		}
	}
	return sessionID
}

func (m *Model) guidedWorkflowTurnLinkAtPosition(col, absolute int) (guidedWorkflowTurnLinkTarget, bool) {
	if m == nil || m.guidedWorkflow == nil || col < 0 || absolute < 0 {
		return guidedWorkflowTurnLinkTarget{}, false
	}
	stage := m.guidedWorkflow.Stage()
	if stage != guidedWorkflowStageLive && stage != guidedWorkflowStageSummary {
		return guidedWorkflowTurnLinkTarget{}, false
	}
	lines := m.renderedPlain
	if len(lines) == 0 && m.renderedText != "" {
		lines = strings.Split(xansi.Strip(m.renderedText), "\n")
	}
	if absolute >= len(lines) {
		return guidedWorkflowTurnLinkTarget{}, false
	}
	line := lines[absolute]
	if strings.TrimSpace(line) == "" {
		return guidedWorkflowTurnLinkTarget{}, false
	}
	for _, target := range m.guidedWorkflow.TurnLinkTargets() {
		label := strings.TrimSpace(target.label)
		if label == "" {
			continue
		}
		for startSearch := 0; startSearch < len(line); {
			idx := strings.Index(line[startSearch:], label)
			if idx < 0 {
				break
			}
			idx += startSearch
			start := xansi.StringWidth(line[:idx])
			end := start + xansi.StringWidth(label) - 1
			if end >= start && col >= start && col <= end {
				return target, true
			}
			startSearch = idx + len(label)
		}
	}
	return guidedWorkflowTurnLinkTarget{}, false
}

func (m *Model) handleGuidedWorkflowSetupInput(msg tea.Msg) (bool, tea.Cmd) {
	if m == nil || m.guidedWorkflow == nil || m.guidedWorkflowPromptInput == nil || m.guidedWorkflow.Stage() != guidedWorkflowStageSetup {
		return false, nil
	}
	if !isTextInputMsg(msg) {
		return false, nil
	}
	controller := textInputModeController{
		input:             m.guidedWorkflowPromptInput,
		keyString:         m.keyString,
		keyMatchesCommand: m.keyMatchesCommand,
		onCancel: func() tea.Cmd {
			m.openGuidedWorkflowLauncherFromSetup()
			return nil
		},
		onSubmit: func(string) tea.Cmd {
			return m.startGuidedWorkflowRun()
		},
		preHandle: func(key string, keyMsg tea.KeyMsg) (bool, tea.Cmd) {
			switch {
			case m.keyMatchesCommand(keyMsg, KeyCommandToggleSidebar, "ctrl+b"):
				m.toggleSidebar()
				return true, m.requestAppStateSaveCmd()
			case m.keyMatchesCommand(keyMsg, KeyCommandToggleNotesPanel, "ctrl+o"):
				return true, m.toggleNotesPanel()
			case m.keyMatchesCommand(keyMsg, KeyCommandMenu, "ctrl+m"):
				if m.menu != nil {
					if m.contextMenu != nil {
						m.contextMenu.Close()
					}
					m.menu.Toggle()
				}
				return true, nil
			}
			switch key {
			case "down":
				m.guidedWorkflow.CycleSensitivity(1)
				m.renderGuidedWorkflowContent()
				return true, nil
			case "up":
				m.guidedWorkflow.CycleSensitivity(-1)
				m.renderGuidedWorkflowContent()
				return true, nil
			}
			return false, nil
		},
	}
	handled, cmd := controller.Update(msg)
	if !handled {
		return false, nil
	}
	if m.guidedWorkflow == nil || m.guidedWorkflow.Stage() != guidedWorkflowStageSetup {
		return true, cmd
	}
	m.syncGuidedWorkflowPromptInput()
	if m.consumeInputHeightChanges(m.guidedWorkflowPromptInput) && m.width > 0 && m.height > 0 {
		m.resize(m.width, m.height)
	} else {
		m.renderGuidedWorkflowContent()
	}
	return handled, cmd
}

func (m *Model) handleGuidedWorkflowResumeInput(msg tea.Msg) (bool, tea.Cmd) {
	if m == nil || m.guidedWorkflow == nil || m.guidedWorkflowResumeInput == nil || m.guidedWorkflow.Stage() != guidedWorkflowStageSummary || !m.guidedWorkflow.CanResumeFailedRun() {
		return false, nil
	}
	if !isTextInputMsg(msg) {
		return false, nil
	}
	controller := textInputModeController{
		input:             m.guidedWorkflowResumeInput,
		keyString:         m.keyString,
		keyMatchesCommand: m.keyMatchesCommand,
		onCancel: func() tea.Cmd {
			m.exitGuidedWorkflow("guided workflow summary closed")
			return nil
		},
		onSubmit: func(string) tea.Cmd {
			return m.resumeFailedGuidedWorkflowRun()
		},
		preHandle: func(string, tea.KeyMsg) (bool, tea.Cmd) {
			return false, nil
		},
	}
	handled, cmd := controller.Update(msg)
	if !handled {
		return false, nil
	}
	if m.guidedWorkflow == nil || m.guidedWorkflow.Stage() != guidedWorkflowStageSummary || !m.guidedWorkflow.CanResumeFailedRun() {
		return true, cmd
	}
	m.syncGuidedWorkflowResumeInput()
	if m.consumeInputHeightChanges(m.guidedWorkflowResumeInput) && m.width > 0 && m.height > 0 {
		m.resize(m.width, m.height)
	} else {
		m.renderGuidedWorkflowContent()
	}
	return handled, cmd
}

func (m *Model) resetGuidedWorkflowPromptInput() {
	if m == nil {
		return
	}
	if m.guidedWorkflowPromptInput == nil {
		m.guidedWorkflowPromptInput = NewTextInput(minViewportWidth, TextInputConfig{Height: 5, MinHeight: 4, MaxHeight: 10, AutoGrow: true})
	}
	m.guidedWorkflowPromptInput.SetPlaceholder("Describe the feature request or bug fix this workflow should execute.")
	m.guidedWorkflowPromptInput.Clear()
	m.guidedWorkflowPromptInput.Focus()
}

func (m *Model) resetGuidedWorkflowResumeInput() {
	if m == nil {
		return
	}
	if m.guidedWorkflowResumeInput == nil {
		m.guidedWorkflowResumeInput = NewTextInput(minViewportWidth, TextInputConfig{Height: 4, MinHeight: 3, MaxHeight: 8, AutoGrow: true})
	}
	m.guidedWorkflowResumeInput.SetPlaceholder("Edit the resume message before submitting.")
	m.guidedWorkflowResumeInput.Clear()
	m.guidedWorkflowResumeInput.Blur()
}

func (m *Model) primeGuidedWorkflowResumeInput() {
	if m == nil || m.guidedWorkflow == nil || !m.guidedWorkflow.CanResumeFailedRun() {
		return
	}
	if m.guidedWorkflowResumeInput == nil {
		m.guidedWorkflowResumeInput = NewTextInput(minViewportWidth, TextInputConfig{Height: 4, MinHeight: 3, MaxHeight: 8, AutoGrow: true})
	}
	m.guidedWorkflowResumeInput.SetPlaceholder("Edit the resume message before submitting.")
	if !m.guidedWorkflowResumeInput.Focused() && strings.TrimSpace(m.guidedWorkflowResumeInput.Value()) == "" {
		m.guidedWorkflowResumeInput.SetValue(m.guidedWorkflow.ResumeMessage())
	}
	m.guidedWorkflowResumeInput.Focus()
}

func (m *Model) syncGuidedWorkflowPromptInput() {
	if m == nil || m.guidedWorkflow == nil {
		return
	}
	if m.guidedWorkflowPromptInput == nil {
		m.guidedWorkflow.SetUserPrompt("")
		return
	}
	m.guidedWorkflow.SetUserPrompt(m.guidedWorkflowPromptInput.Value())
}

func (m *Model) syncGuidedWorkflowResumeInput() {
	if m == nil || m.guidedWorkflow == nil {
		return
	}
	if m.guidedWorkflowResumeInput == nil {
		m.guidedWorkflow.SetResumeMessage("")
		return
	}
	m.guidedWorkflow.SetResumeMessage(m.guidedWorkflowResumeInput.Value())
}

func (m *Model) syncGuidedWorkflowInputFocus() {
	if m == nil || m.guidedWorkflow == nil {
		return
	}
	switch m.guidedWorkflow.Stage() {
	case guidedWorkflowStageSetup:
		if m.guidedWorkflowPromptInput != nil {
			m.guidedWorkflowPromptInput.Focus()
		}
		if m.guidedWorkflowResumeInput != nil {
			m.guidedWorkflowResumeInput.Blur()
		}
	case guidedWorkflowStageSummary:
		if m.guidedWorkflow.CanResumeFailedRun() {
			m.primeGuidedWorkflowResumeInput()
			if m.guidedWorkflowPromptInput != nil {
				m.guidedWorkflowPromptInput.Blur()
			}
			return
		}
		if m.guidedWorkflowResumeInput != nil {
			m.guidedWorkflowResumeInput.Blur()
		}
	default:
		if m.guidedWorkflowPromptInput != nil {
			m.guidedWorkflowPromptInput.Blur()
		}
		if m.guidedWorkflowResumeInput != nil {
			m.guidedWorkflowResumeInput.Blur()
		}
	}
}

func (m *Model) guidedWorkflowSetupInputPanel() (InputPanel, bool) {
	if m == nil || m.guidedWorkflow == nil || m.guidedWorkflowPromptInput == nil || m.guidedWorkflow.Stage() != guidedWorkflowStageSetup {
		return InputPanel{}, false
	}
	return InputPanel{
		Input: m.guidedWorkflowPromptInput,
		Frame: m.inputFrame(InputFrameTargetGuidedWorkflowSetup),
	}, true
}

func (m *Model) guidedWorkflowResumeInputPanel() (InputPanel, bool) {
	if m == nil || m.guidedWorkflow == nil || m.guidedWorkflowResumeInput == nil || m.guidedWorkflow.Stage() != guidedWorkflowStageSummary || !m.guidedWorkflow.CanResumeFailedRun() {
		return InputPanel{}, false
	}
	return InputPanel{
		Input: m.guidedWorkflowResumeInput,
		Frame: m.inputFrame(InputFrameTargetGuidedWorkflowSetup),
	}, true
}

func (m *Model) guidedWorkflowSetupInputLineCount() int {
	panel, ok := m.guidedWorkflowSetupInputPanel()
	if !ok {
		return 0
	}
	return BuildInputPanelLayout(panel).LineCount()
}

func (m *Model) openGuidedWorkflowLauncherFromSetup() {
	if m == nil || m.guidedWorkflow == nil {
		return
	}
	m.guidedWorkflow.OpenLauncher()
	if m.guidedWorkflowPromptInput != nil {
		m.guidedWorkflowPromptInput.Blur()
	}
	m.setStatusMessage("guided workflow launcher")
	m.reflowGuidedWorkflowLayout()
}

func (m *Model) reflowGuidedWorkflowLayout() {
	if m == nil {
		return
	}
	if m.width > 0 && m.height > 0 {
		m.resize(m.width, m.height)
		return
	}
	m.renderGuidedWorkflowContent()
}
