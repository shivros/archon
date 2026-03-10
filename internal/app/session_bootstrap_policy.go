package app

import (
	"strings"

	"control/internal/types"
)

type sessionBootstrapPlan struct {
	FetchTranscript bool
	FetchApprovals  bool
	OpenTranscript  bool
}

type SessionBootstrapPolicy interface {
	SelectionLoadPlan(provider string, status types.SessionStatus) sessionBootstrapPlan
	SessionStartPlan(provider string, status types.SessionStatus) sessionBootstrapPlan
	SendReconnectPlan(provider string) sessionBootstrapPlan
}

type defaultSessionBootstrapPolicy struct{}

func WithSessionBootstrapPolicy(policy SessionBootstrapPolicy) ModelOption {
	return func(m *Model) {
		if m == nil {
			return
		}
		if policy == nil {
			m.sessionBootstrapPolicy = defaultSessionBootstrapPolicy{}
			return
		}
		m.sessionBootstrapPolicy = policy
	}
}

func (defaultSessionBootstrapPolicy) SelectionLoadPlan(provider string, status types.SessionStatus) sessionBootstrapPlan {
	_ = provider
	_ = status
	return sessionBootstrapPlan{
		FetchTranscript: true,
		FetchApprovals:  true,
		OpenTranscript:  true,
	}
}

func (defaultSessionBootstrapPolicy) SessionStartPlan(provider string, status types.SessionStatus) sessionBootstrapPlan {
	_ = provider
	_ = status
	return sessionBootstrapPlan{
		FetchTranscript: true,
		FetchApprovals:  true,
		OpenTranscript:  true,
	}
}

func (defaultSessionBootstrapPolicy) SendReconnectPlan(provider string) sessionBootstrapPlan {
	_ = provider
	return sessionBootstrapPlan{OpenTranscript: true}
}

func (m *Model) sessionBootstrapPolicyOrDefault() SessionBootstrapPolicy {
	if m == nil || m.sessionBootstrapPolicy == nil {
		return defaultSessionBootstrapPolicy{}
	}
	return m.sessionBootstrapPolicy
}

func prefersSharedTranscriptFollow(activeSessionID, candidateSessionID string) bool {
	active := strings.TrimSpace(activeSessionID)
	candidate := strings.TrimSpace(candidateSessionID)
	return active != "" && candidate != "" && active == candidate
}
