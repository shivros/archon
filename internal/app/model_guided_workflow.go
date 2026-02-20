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
	m.resetGuidedWorkflowPromptInput()
	if m.input != nil {
		m.input.FocusSidebar()
	}
	m.setStatusMessage("guided workflow launcher")
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
	if m.guidedWorkflow.Stage() == guidedWorkflowStageSetup {
		m.syncGuidedWorkflowPromptInput()
	}
	content := m.guidedWorkflow.Render()
	m.setContentText(content)
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
	keyMsg, ok := msg.(tea.KeyMsg)
	if ok {
		key := m.keyString(keyMsg)
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
		}
		switch key {
		case "esc":
			if m.guidedWorkflow != nil && m.guidedWorkflow.Stage() == guidedWorkflowStageSetup {
				m.openGuidedWorkflowLauncherFromSetup()
				return true, nil
			}
			m.exitGuidedWorkflow("guided workflow closed")
			return true, nil
		case "enter":
			return true, m.handleGuidedWorkflowEnter()
		case "down":
			if m.guidedWorkflow != nil {
				switch m.guidedWorkflow.Stage() {
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
		case "o":
			if m.guidedWorkflow != nil && (m.guidedWorkflow.Stage() == guidedWorkflowStageLive || m.guidedWorkflow.Stage() == guidedWorkflowStageSummary) {
				return true, m.openGuidedWorkflowSelectedSession()
			}
		}
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
	if sessionID == "" {
		m.setValidationStatus("selected step has no linked session")
		m.renderGuidedWorkflowContent()
		return nil
	}
	if !m.sidebar.SelectBySessionID(sessionID) {
		m.setValidationStatus("linked session not found: " + sessionID)
		m.renderGuidedWorkflowContent()
		return nil
	}
	item := m.selectedItem()
	m.exitGuidedWorkflow("opened linked session " + sessionID)
	return m.batchWithNotesPanelSync(m.loadSelectedSession(item))
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

func (m *Model) guidedWorkflowSetupInputPanel() (InputPanel, bool) {
	if m == nil || m.guidedWorkflow == nil || m.guidedWorkflowPromptInput == nil || m.guidedWorkflow.Stage() != guidedWorkflowStageSetup {
		return InputPanel{}, false
	}
	return InputPanel{
		Input: m.guidedWorkflowPromptInput,
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
