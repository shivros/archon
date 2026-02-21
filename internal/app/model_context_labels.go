package app

import "strings"

func (m *Model) sessionDisplayName(sessionID string) string {
	if m == nil {
		return ""
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return ""
	}
	for _, session := range m.sessions {
		if session == nil || strings.TrimSpace(session.ID) != sessionID {
			continue
		}
		return strings.TrimSpace(sessionTitle(session, m.sessionMeta[sessionID]))
	}
	return strings.TrimSpace(sessionTitle(nil, m.sessionMeta[sessionID]))
}

func (m *Model) workspaceNameByID(workspaceID string) string {
	if m == nil {
		return ""
	}
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		return ""
	}
	if ws := m.workspaceByID(workspaceID); ws != nil {
		return strings.TrimSpace(ws.Name)
	}
	return ""
}

func (m *Model) worktreeNameByID(worktreeID string) string {
	if m == nil {
		return ""
	}
	worktreeID = strings.TrimSpace(worktreeID)
	if worktreeID == "" {
		return ""
	}
	if wt := m.worktreeByID(worktreeID); wt != nil {
		return strings.TrimSpace(wt.Name)
	}
	return ""
}
