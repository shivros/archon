package app

import tea "charm.land/bubbletea/v2"

const (
	defaultSessionProjectionAsyncThreshold = 32
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
	ShouldProjectAsync(itemCount int) bool
	MaxTrackedProjectionTokens() int
}

type defaultSessionProjectionPolicy struct{}

func WithSessionProjectionPolicy(policy SessionProjectionPolicy) ModelOption {
	return func(m *Model) {
		if m == nil {
			return
		}
		if policy == nil {
			m.sessionProjectionPolicy = defaultSessionProjectionPolicy{}
			return
		}
		m.sessionProjectionPolicy = policy
	}
}

func (defaultSessionProjectionPolicy) ShouldProjectAsync(itemCount int) bool {
	return itemCount >= defaultSessionProjectionAsyncThreshold
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
