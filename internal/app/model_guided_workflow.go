package app

import (
	"strings"
	"time"

	"control/internal/guidedworkflows"

	tea "charm.land/bubbletea/v2"
)

func (m *Model) enterGuidedWorkflow(context guidedWorkflowLaunchContext) {
	if m == nil {
		return
	}
	if m.guidedWorkflow == nil {
		m.guidedWorkflow = NewGuidedWorkflowUIController()
	}
	m.mode = uiModeGuidedWorkflow
	m.guidedWorkflow.Enter(context)
	if m.input != nil {
		m.input.FocusSidebar()
	}
	m.setStatusMessage("guided workflow launcher")
	m.renderGuidedWorkflowContent()
}

func (m *Model) exitGuidedWorkflow(status string) {
	if m == nil {
		return
	}
	if m.guidedWorkflow != nil {
		m.guidedWorkflow.Exit()
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
	m.setContentText(m.guidedWorkflow.Render())
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
	keyMsg, ok := msg.(tea.KeyMsg)
	if !ok {
		return false, nil
	}
	switch m.keyString(keyMsg) {
	case "q":
		return true, tea.Quit
	case "ctrl+b":
		m.toggleSidebar()
		return true, m.requestAppStateSaveCmd()
	case "ctrl+o":
		return true, m.toggleNotesPanel()
	case "ctrl+m":
		if m.menu != nil {
			if m.contextMenu != nil {
				m.contextMenu.Close()
			}
			m.menu.Toggle()
		}
		return true, nil
	case "esc":
		if m.guidedWorkflow != nil && m.guidedWorkflow.Stage() == guidedWorkflowStageSetup {
			m.guidedWorkflow.OpenLauncher()
			m.setStatusMessage("guided workflow launcher")
			m.renderGuidedWorkflowContent()
			return true, nil
		}
		m.exitGuidedWorkflow("guided workflow closed")
		return true, nil
	case "enter":
		return true, m.handleGuidedWorkflowEnter()
	case "j", "down":
		if m.guidedWorkflow != nil && m.guidedWorkflow.Stage() == guidedWorkflowStageSetup {
			m.guidedWorkflow.CycleSensitivity(1)
			m.renderGuidedWorkflowContent()
			return true, nil
		}
	case "k", "up":
		if m.guidedWorkflow != nil && m.guidedWorkflow.Stage() == guidedWorkflowStageSetup {
			m.guidedWorkflow.CycleSensitivity(-1)
			m.renderGuidedWorkflowContent()
			return true, nil
		}
	case "r":
		if m.guidedWorkflow != nil && (m.guidedWorkflow.Stage() == guidedWorkflowStageLive || m.guidedWorkflow.Stage() == guidedWorkflowStageSummary) {
			return true, m.refreshGuidedWorkflowNow("refreshing guided workflow timeline")
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
	}
	return false, nil
}

func (m *Model) handleGuidedWorkflowEnter() tea.Cmd {
	if m == nil || m.guidedWorkflow == nil {
		return nil
	}
	switch m.guidedWorkflow.Stage() {
	case guidedWorkflowStageLauncher:
		m.guidedWorkflow.OpenSetup()
		m.setStatusMessage("guided workflow setup")
		m.renderGuidedWorkflowContent()
		return nil
	case guidedWorkflowStageSetup:
		return m.startGuidedWorkflowRun()
	case guidedWorkflowStageLive:
		if m.guidedWorkflow.NeedsDecision() {
			return m.decideGuidedWorkflow(m.guidedWorkflow.RecommendedDecisionAction())
		}
		return m.refreshGuidedWorkflowNow("refreshing guided workflow timeline")
	case guidedWorkflowStageSummary:
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
	req := m.guidedWorkflow.BuildCreateRequest()
	if strings.TrimSpace(req.WorkspaceID) == "" && strings.TrimSpace(req.WorktreeID) == "" {
		m.setValidationStatus("guided workflow requires workspace or worktree context")
		return nil
	}
	m.guidedWorkflow.BeginStart()
	m.renderGuidedWorkflowContent()
	m.setStatusMessage("creating guided workflow run")
	return createWorkflowRunCmd(m.guidedWorkflowAPI, req)
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
