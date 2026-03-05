package app

import "strings"

type SidePanelModePolicy interface {
	Resolve(model *Model) sidePanelMode
}

type SidePanelModeInput struct {
	DebugStreamsEnabled bool
	NotesPanelOpen      bool
	ContextPanelEnabled bool
	HasContextSession   bool
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

func resolveSidePanelMode(input SidePanelModeInput) sidePanelMode {
	if input.DebugStreamsEnabled {
		return sidePanelModeDebug
	}
	if input.NotesPanelOpen {
		return sidePanelModeNotes
	}
	if input.ContextPanelEnabled && input.HasContextSession {
		return sidePanelModeContext
	}
	return sidePanelModeNone
}

func (m *Model) sidePanelModeInput() SidePanelModeInput {
	if m == nil {
		return SidePanelModeInput{}
	}
	return SidePanelModeInput{
		DebugStreamsEnabled: m.appState.DebugStreamsEnabled,
		NotesPanelOpen:      m.notesPanelOpen,
		ContextPanelEnabled: m.contextPanelEnabled(),
		HasContextSession:   strings.TrimSpace(m.contextPanelSessionID()) != "",
	}
}

func (defaultSidePanelModePolicy) Resolve(model *Model) sidePanelMode {
	if model == nil {
		return sidePanelModeNone
	}
	return resolveSidePanelMode(model.sidePanelModeInput())
}
