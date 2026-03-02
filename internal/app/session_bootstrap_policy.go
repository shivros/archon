package app

import "control/internal/types"

type sessionBootstrapPlan struct {
	FetchHistory   bool
	FetchApprovals bool
	OpenItems      bool
	OpenTail       bool
	OpenEvents     bool
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
	plan := sessionBootstrapPlan{
		FetchApprovals: true,
	}
	if shouldStreamItems(provider) {
		// Item providers bootstrap transcript state from /items stream snapshot.
		plan.OpenItems = true
		return plan
	}
	plan.FetchHistory = true
	if isActiveStatus(status) {
		plan.OpenTail = true
	}
	if provider == "codex" {
		plan.OpenEvents = true
	}
	return plan
}

func (defaultSessionBootstrapPolicy) SessionStartPlan(provider string, status types.SessionStatus) sessionBootstrapPlan {
	plan := sessionBootstrapPlan{
		FetchApprovals: true,
	}
	if shouldStreamItems(provider) {
		plan.OpenItems = true
		return plan
	}
	plan.FetchHistory = true
	if provider == "codex" {
		plan.OpenEvents = true
		return plan
	}
	if isActiveStatus(status) {
		plan.OpenTail = true
	}
	return plan
}

func (defaultSessionBootstrapPolicy) SendReconnectPlan(provider string) sessionBootstrapPlan {
	plan := sessionBootstrapPlan{}
	if shouldStreamItems(provider) {
		plan.OpenItems = true
		return plan
	}
	if provider == "codex" {
		plan.OpenEvents = true
	}
	return plan
}

func (m *Model) sessionBootstrapPolicyOrDefault() SessionBootstrapPolicy {
	if m == nil || m.sessionBootstrapPolicy == nil {
		return defaultSessionBootstrapPolicy{}
	}
	return m.sessionBootstrapPolicy
}
