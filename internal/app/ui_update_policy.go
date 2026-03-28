package app

import tea "charm.land/bubbletea/v2"

const (
	// Fetched session/history payloads should hand work off immediately so
	// navigation and input stay responsive; empty payloads remain synchronous
	// because they are trivial to apply.
	defaultSessionProjectionAsyncThreshold = 1
	defaultSessionProjectionMaxTokens      = 256
	defaultDebugPanelProjectionMaxTokens   = 32
)

type SidebarUpdatePolicy interface {
	ShouldUpdateSidebar(msg tea.Msg) bool
}

type defaultSidebarUpdatePolicy struct{}

func WithSidebarUpdatePolicy(policy SidebarUpdatePolicy) ModelOption {
	return func(m *Model) {
		if m == nil {
			return
		}
		if policy == nil {
			m.sidebarUpdatePolicy = defaultSidebarUpdatePolicy{}
			return
		}
		m.sidebarUpdatePolicy = policy
	}
}

func (defaultSidebarUpdatePolicy) ShouldUpdateSidebar(msg tea.Msg) bool {
	switch msg.(type) {
	case tea.KeyMsg, tea.MouseMsg, tea.WindowSizeMsg:
		return true
	default:
		return false
	}
}

func (m *Model) sidebarUpdatePolicyOrDefault() SidebarUpdatePolicy {
	if m == nil || m.sidebarUpdatePolicy == nil {
		return defaultSidebarUpdatePolicy{}
	}
	return m.sidebarUpdatePolicy
}

type SessionProjectionPolicy interface {
	ShouldProjectAsync(input SessionProjectionDecisionInput) bool
	MaxTrackedProjectionTokens() int
}

type SessionProjectionDecisionInput struct {
	ItemCount        int
	Source           sessionProjectionSource
	Provider         string
	HasApprovals     bool
	IsFetchedPayload bool
}

type defaultSessionProjectionPolicy struct{}

func WithSessionProjectionPolicy(policy SessionProjectionPolicy) ModelOption {
	return func(m *Model) {
		if m == nil {
			return
		}
		if policy == nil {
			policy = defaultSessionProjectionPolicy{}
		}
		m.sessionProjectionPolicy = policy
		m.sessionProjectionCoordinator = NewDefaultSessionProjectionCoordinator(policy, nil)
	}
}

func WithSessionProjectionCoordinator(coordinator sessionProjectionCoordinator) ModelOption {
	return func(m *Model) {
		if m == nil {
			return
		}
		if coordinator == nil {
			m.sessionProjectionCoordinator = NewDefaultSessionProjectionCoordinator(m.sessionProjectionPolicyOrDefault(), nil)
			return
		}
		m.sessionProjectionCoordinator = coordinator
	}
}

func (defaultSessionProjectionPolicy) ShouldProjectAsync(input SessionProjectionDecisionInput) bool {
	if !input.IsFetchedPayload {
		return false
	}
	return input.ItemCount >= defaultSessionProjectionAsyncThreshold
}

func (defaultSessionProjectionPolicy) MaxTrackedProjectionTokens() int {
	return defaultSessionProjectionMaxTokens
}

func (m *Model) sessionProjectionPolicyOrDefault() SessionProjectionPolicy {
	if m == nil || m.sessionProjectionPolicy == nil {
		return defaultSessionProjectionPolicy{}
	}
	return m.sessionProjectionPolicy
}

type DebugPanelProjectionPolicy interface {
	MaxTrackedProjectionTokens() int
}

type defaultDebugPanelProjectionPolicy struct{}

func WithDebugPanelProjectionPolicy(policy DebugPanelProjectionPolicy) ModelOption {
	return func(m *Model) {
		if m == nil {
			return
		}
		if policy == nil {
			policy = defaultDebugPanelProjectionPolicy{}
		}
		m.debugPanelProjectionPolicy = policy
		m.debugPanelProjectionCoordinator = NewDefaultDebugPanelProjectionCoordinator(policy, nil)
	}
}

func WithDebugPanelProjectionCoordinator(coordinator debugPanelProjectionCoordinator) ModelOption {
	return func(m *Model) {
		if m == nil {
			return
		}
		if coordinator == nil {
			m.debugPanelProjectionCoordinator = NewDefaultDebugPanelProjectionCoordinator(m.debugPanelProjectionPolicyOrDefault(), nil)
			return
		}
		m.debugPanelProjectionCoordinator = coordinator
	}
}

func (defaultDebugPanelProjectionPolicy) MaxTrackedProjectionTokens() int {
	return defaultDebugPanelProjectionMaxTokens
}

func (m *Model) debugPanelProjectionPolicyOrDefault() DebugPanelProjectionPolicy {
	if m == nil || m.debugPanelProjectionPolicy == nil {
		return defaultDebugPanelProjectionPolicy{}
	}
	return m.debugPanelProjectionPolicy
}
