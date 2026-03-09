package app

import (
	"strings"

	tea "charm.land/bubbletea/v2"
)

func (m *Model) composeInterruptSessionID() string {
	if m == nil || m.mode != uiModeCompose || m.newSession != nil {
		return ""
	}
	return strings.TrimSpace(m.composeSessionID())
}

func (m *Model) composeSessionSupportsInterrupt(sessionID string) bool {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return false
	}
	session := m.sessionByID(sessionID)
	if session == nil {
		return false
	}
	capabilities, _ := m.sessionTranscriptCapabilitiesForSession(sessionID)
	return m.composeInterruptCapabilityProbeOrDefault().SupportsInterrupt(ComposeInterruptCapabilityInput{
		SessionID:    sessionID,
		Provider:     session.Provider,
		Capabilities: capabilities,
		ModeResolver: m.sessionCapabilityModeResolverOrDefault(),
	})
}

func (m *Model) composeSessionHasInterruptSignal(sessionID string) bool {
	if m == nil {
		return false
	}
	return m.composeInterruptSignalProbeOrDefault().HasSignal(ComposeInterruptSignalInput{
		SessionID:         sessionID,
		InFlightSessionID: m.composeInterruptInFlightSessionID,
		RequestActivity:   m.requestActivity,
		Recents:           m.recents,
	})
}

func (m *Model) canInterruptComposeSession(sessionID string) bool {
	if m == nil {
		return false
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return false
	}
	session := m.sessionByID(sessionID)
	if session == nil {
		return false
	}
	return m.composeInterruptEligibilityPolicyOrDefault().CanInterrupt(ComposeInterruptEligibilityInput{
		SessionID:         sessionID,
		SessionStatus:     session.Status,
		SupportsInterrupt: m.composeSessionSupportsInterrupt(sessionID),
		HasSignal:         m.composeSessionHasInterruptSignal(sessionID),
	})
}

func (m *Model) composeInterruptControl() (label, sessionID string, ok bool) {
	sessionID = m.composeInterruptSessionID()
	if sessionID == "" || !m.canInterruptComposeSession(sessionID) {
		return "", "", false
	}
	label = "Interrupt"
	if strings.TrimSpace(m.composeInterruptInFlightSessionID) == sessionID {
		label = "Interrupting..."
	}
	return label, sessionID, true
}

func (m *Model) requestComposeInterruptCmd() tea.Cmd {
	if m == nil {
		return nil
	}
	sessionID := m.composeInterruptSessionID()
	if sessionID == "" {
		m.setValidationStatus("select a session to interrupt")
		return nil
	}
	if strings.TrimSpace(m.composeInterruptInFlightSessionID) == sessionID {
		m.setStatusMessage("interrupt already in progress")
		return nil
	}
	if !m.canInterruptComposeSession(sessionID) {
		m.setValidationStatus("no interruptible turn in this session")
		return nil
	}
	m.composeInterruptInFlightSessionID = sessionID
	ctx := m.replaceRequestScope(requestScopeSessionInterrupt)
	m.setStatusMessage("interrupting " + sessionID)
	return interruptSessionCmdWithContext(m.sessionAPI, sessionID, ctx)
}

func (m *Model) clearComposeInterruptRequest(sessionID string) {
	if m == nil {
		return
	}
	sessionID = strings.TrimSpace(sessionID)
	active := strings.TrimSpace(m.composeInterruptInFlightSessionID)
	if sessionID != "" && active != "" && active != sessionID {
		return
	}
	m.composeInterruptInFlightSessionID = ""
	m.cancelRequestScope(requestScopeSessionInterrupt)
}
