package app

type SidePanelModePolicy interface {
	Resolve(model *Model) sidePanelMode
}

type defaultSidePanelModePolicy struct{}

func WithSidePanelModePolicy(policy SidePanelModePolicy) ModelOption {
	return func(m *Model) {
		if m == nil {
			return
		}
		if policy == nil {
			m.sidePanelModePolicy = defaultSidePanelModePolicy{}
			return
		}
		m.sidePanelModePolicy = policy
	}
}

func (m *Model) sidePanelModePolicyOrDefault() SidePanelModePolicy {
	if m == nil || m.sidePanelModePolicy == nil {
		return defaultSidePanelModePolicy{}
	}
	return m.sidePanelModePolicy
}

func (m *Model) activeSidePanelMode() sidePanelMode {
	return m.sidePanelModePolicyOrDefault().Resolve(m)
}

func (defaultSidePanelModePolicy) Resolve(model *Model) sidePanelMode {
	if model == nil {
		return sidePanelModeNone
	}
	if model.appState.DebugStreamsEnabled {
		return sidePanelModeDebug
	}
	if model.mode == uiModeCompose {
		return sidePanelModeContext
	}
	if model.notesPanelOpen {
		return sidePanelModeNotes
	}
	return sidePanelModeNone
}
