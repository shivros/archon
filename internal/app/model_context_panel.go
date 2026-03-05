package app

import (
	"strings"

	"control/internal/types"
)

func (m *Model) renderContextPanelView() string {
	data := m.threadContextPanelData()
	title := strings.TrimSpace(data.ThreadTitle)
	if title == "" {
		title = "Thread"
	}
	body := []string{
		headerStyle.Render(title),
		"",
		headerStyle.Render("Context"),
		formatTokensOrDash(data.Metrics.Tokens),
		formatContextUsedOrDash(data.Metrics.ContextUsedPct),
		formatSpendOrDash(data.Metrics.SpendUSD),
	}
	return strings.Join(body, "\n")
}

func (m *Model) threadContextPanelData() ThreadContextPanelData {
	sessionID := m.contextPanelSessionID()
	input := ThreadContextMetricsInput{
		SessionID:   sessionID,
		Provider:    m.providerForSessionID(sessionID),
		Session:     m.sessionByID(sessionID),
		SessionMeta: m.sessionMetaByID(sessionID),
	}
	return m.threadContextMetricsServiceOrDefault().BuildPanelData(input)
}

func (m *Model) contextPanelSessionID() string {
	if m == nil {
		return ""
	}
	if id := strings.TrimSpace(m.composeSessionID()); id != "" {
		return id
	}
	return strings.TrimSpace(m.selectedSessionID())
}

func (m *Model) sessionByID(sessionID string) *types.Session {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil
	}
	for _, session := range m.sessions {
		if session != nil && strings.TrimSpace(session.ID) == sessionID {
			return session
		}
	}
	return nil
}

func (m *Model) sessionMetaByID(sessionID string) *types.SessionMeta {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" || m == nil || m.sessionMeta == nil {
		return nil
	}
	return m.sessionMeta[sessionID]
}
