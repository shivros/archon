package app

import "strings"

func (m *Model) setPendingWorkflowTurnFocus(sessionID, turnID string) {
	if m == nil {
		return
	}
	sessionID = strings.TrimSpace(sessionID)
	turnID = strings.TrimSpace(turnID)
	if sessionID == "" || turnID == "" {
		m.pendingWorkflowTurnFocus = nil
		return
	}
	m.pendingWorkflowTurnFocus = &workflowTurnFocusRequest{
		sessionID: sessionID,
		turnID:    turnID,
	}
}

func (m *Model) applyPendingWorkflowTurnFocus(source sessionProjectionSource, sessionID string, blocks []ChatBlock) {
	if m == nil || m.pendingWorkflowTurnFocus == nil {
		return
	}
	sessionID = strings.TrimSpace(sessionID)
	req := m.pendingWorkflowTurnFocus
	if sessionID == "" || req.sessionID != sessionID {
		return
	}
	idx := firstUserBlockIndexForTurn(blocks, req.turnID)
	if idx >= 0 {
		m.pendingWorkflowTurnFocus = nil
		if m.transcriptViewportVisible() {
			m.setMessageSelectionIndex(idx)
		}
		return
	}
	if source == sessionProjectionSourceHistory {
		m.pendingWorkflowTurnFocus = nil
		m.setStatusWarning("linked workflow turn not found in loaded history")
	}
}

func firstUserBlockIndexForTurn(blocks []ChatBlock, turnID string) int {
	turnID = strings.TrimSpace(turnID)
	if turnID == "" {
		return -1
	}
	for i := range blocks {
		if blocks[i].Role != ChatRoleUser {
			continue
		}
		if strings.TrimSpace(blocks[i].TurnID) == turnID {
			return i
		}
	}
	return -1
}
