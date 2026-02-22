package app

import (
	"fmt"
	"net/url"
	"strings"

	"control/internal/guidedworkflows"
)

const unavailableUserTurnLink = "(unavailable)"

type WorkflowUserTurnLinkBuilder interface {
	BuildUserTurnLink(sessionID, turnID string) string
}

type archonWorkflowUserTurnLinkBuilder struct{}

func NewArchonWorkflowUserTurnLinkBuilder() WorkflowUserTurnLinkBuilder {
	return archonWorkflowUserTurnLinkBuilder{}
}

func (archonWorkflowUserTurnLinkBuilder) BuildUserTurnLink(sessionID, turnID string) string {
	sessionID = strings.TrimSpace(sessionID)
	turnID = strings.TrimSpace(turnID)
	if sessionID == "" || turnID == "" {
		return unavailableUserTurnLink
	}
	escapedSessionID := url.PathEscape(sessionID)
	escapedTurnID := url.QueryEscape(turnID)
	deepLink := fmt.Sprintf("archon://session/%s?turn=%s", escapedSessionID, escapedTurnID)
	return fmt.Sprintf("[user turn %s](%s)", turnID, deepLink)
}

func workflowUserTurnLinkBuilderOrDefault(builder WorkflowUserTurnLinkBuilder) WorkflowUserTurnLinkBuilder {
	if builder == nil {
		return NewArchonWorkflowUserTurnLinkBuilder()
	}
	return builder
}

func workflowUserTurnLinkLabel(link string) string {
	link = strings.TrimSpace(link)
	if link == "" || link == unavailableUserTurnLink {
		return ""
	}
	if strings.HasPrefix(link, "[") {
		openParen := strings.Index(link, "](")
		if openParen > 1 {
			return strings.TrimSpace(link[1:openParen])
		}
	}
	return link
}

func stepSessionAndTurn(step guidedworkflows.StepRun) (sessionID string, turnID string) {
	if step.Execution != nil {
		sessionID = strings.TrimSpace(step.Execution.SessionID)
		turnID = strings.TrimSpace(step.Execution.TurnID)
	}
	if turnID == "" {
		turnID = strings.TrimSpace(step.TurnID)
	}
	return strings.TrimSpace(sessionID), strings.TrimSpace(turnID)
}
