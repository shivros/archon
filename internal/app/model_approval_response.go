package app

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

const approvalMethodRequestUserInput = "tool/requestUserInput"

func (m *Model) approvalRequestForSession(sessionID string, requestID int) *ApprovalRequest {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" || requestID < 0 {
		return nil
	}
	return findApprovalRequestByID(m.sessionApprovals[sessionID], requestID)
}

func approvalRequestNeedsResponse(request *ApprovalRequest) bool {
	if request == nil {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(request.Method), approvalMethodRequestUserInput)
}

func (m *Model) enterApprovalResponse(sessionID string, request *ApprovalRequest) {
	if m == nil || request == nil {
		return
	}
	m.approvalResponseReturnMode = m.mode
	if m.input != nil {
		m.approvalResponseReturnFocus = m.input.focus
		m.input.FocusChatInput()
	}
	m.approvalResponseRequest = cloneApprovalRequest(request)
	m.approvalResponseSessionID = strings.TrimSpace(sessionID)
	m.approvalResponseRequestID = request.RequestID
	if m.approvalInput != nil {
		m.approvalInput.SetPlaceholder("Type a response, then press Enter to approve")
		m.approvalInput.SetValue("")
		m.approvalInput.Focus()
	}
	m.mode = uiModeApprovalResponse
	m.setStatusMessage("approval response required")
	m.resize(m.width, m.height)
}

func (m *Model) exitApprovalResponse(status string) {
	if m == nil {
		return
	}
	returnMode := m.approvalResponseReturnMode
	if returnMode == uiModeApprovalResponse {
		returnMode = uiModeNormal
	}
	returnFocus := m.approvalResponseReturnFocus
	m.approvalResponseRequest = nil
	m.approvalResponseSessionID = ""
	m.approvalResponseRequestID = -1
	m.approvalResponseReturnMode = uiModeNormal
	m.approvalResponseReturnFocus = focusSidebar
	if m.approvalInput != nil {
		m.approvalInput.Blur()
		m.approvalInput.SetPlaceholder("")
		m.approvalInput.SetValue("")
	}
	m.mode = returnMode
	if m.input != nil {
		if returnMode == uiModeCompose && returnFocus == focusChatInput {
			m.input.FocusChatInput()
			if m.chatInput != nil {
				m.chatInput.Focus()
			}
		} else {
			m.input.FocusSidebar()
			if m.chatInput != nil {
				m.chatInput.Blur()
			}
		}
	}
	if status != "" {
		m.setStatusMessage(status)
	}
	m.resize(m.width, m.height)
}

func (m *Model) cancelApprovalResponseInput() tea.Cmd {
	m.exitApprovalResponse("approval input canceled")
	return nil
}

func (m *Model) submitApprovalResponseInput(text string) tea.Cmd {
	text = strings.TrimSpace(text)
	if text == "" {
		m.setValidationStatus("response is required")
		return nil
	}
	sessionID := strings.TrimSpace(m.approvalResponseSessionID)
	if sessionID == "" {
		m.setValidationStatus("select a session to approve")
		return nil
	}
	requestID := m.approvalResponseRequestID
	if requestID < 0 {
		m.setValidationStatus("invalid approval request")
		return nil
	}
	m.exitApprovalResponse("sending approval")
	return approveSessionCmd(m.sessionAPI, sessionID, requestID, "accept", []string{text})
}

func (m *Model) approvalResponseBody() string {
	request := m.approvalResponseRequest
	if request == nil {
		return "Provide a response to continue."
	}
	lines := make([]string, 0, 2+len(request.Context))
	if summary := strings.TrimSpace(request.Summary); summary != "" {
		lines = append(lines, summary)
	}
	if detail := strings.TrimSpace(request.Detail); detail != "" {
		if len(lines) == 0 || !strings.EqualFold(lines[len(lines)-1], detail) {
			lines = append(lines, detail)
		}
	}
	lines = append(lines, request.Context...)
	if len(lines) == 0 {
		return "Provide a response to continue."
	}
	return strings.Join(lines, "\n")
}

func (m *Model) approvalResponseFooter() string {
	return "enter submit  esc cancel"
}
