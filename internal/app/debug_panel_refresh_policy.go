package app

type DebugPanelRefreshInput struct {
	DebugStreamsEnabled bool
	PanelVisible        bool
	PanelWidth          int
	ProjectionInFlight  bool
}

type DebugPanelRefreshPolicy interface {
	ShouldDefer(input DebugPanelRefreshInput) bool
}

type defaultDebugPanelRefreshPolicy struct{}

func (defaultDebugPanelRefreshPolicy) ShouldDefer(input DebugPanelRefreshInput) bool {
	if input.ProjectionInFlight {
		return true
	}
	return input.DebugStreamsEnabled && (!input.PanelVisible || input.PanelWidth <= 0)
}

func WithDebugPanelRefreshPolicy(policy DebugPanelRefreshPolicy) ModelOption {
	return func(m *Model) {
		if m == nil {
			return
		}
		if policy == nil {
			m.debugPanelRefreshPolicy = defaultDebugPanelRefreshPolicy{}
			return
		}
		m.debugPanelRefreshPolicy = policy
	}
}

func (m *Model) debugPanelRefreshPolicyOrDefault() DebugPanelRefreshPolicy {
	if m == nil || m.debugPanelRefreshPolicy == nil {
		return defaultDebugPanelRefreshPolicy{}
	}
	return m.debugPanelRefreshPolicy
}
