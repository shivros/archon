package app

import "control/internal/types"

type PanelLayoutPersistencePolicy interface {
	Width(state *types.AppState) int
	SetWidth(state *types.AppState, width int) bool
}

type panelWidthAccessors struct {
	get func(*types.AppState) int
	set func(*types.AppState, int)
}

func (p panelWidthAccessors) Width(state *types.AppState) int {
	if state == nil || p.get == nil {
		return 0
	}
	return p.get(state)
}

func (p panelWidthAccessors) SetWidth(state *types.AppState, width int) bool {
	if state == nil || p.get == nil || p.set == nil {
		return false
	}
	if p.get(state) == width {
		return false
	}
	p.set(state, width)
	return true
}

type noopPanelLayoutPersistencePolicy struct{}

func (noopPanelLayoutPersistencePolicy) Width(_ *types.AppState) int { return 0 }

func (noopPanelLayoutPersistencePolicy) SetWidth(_ *types.AppState, _ int) bool { return false }

var panelLayoutPersistencePolicies = map[sidePanelMode]PanelLayoutPersistencePolicy{
	sidePanelModeNotes: panelWidthAccessors{
		get: func(state *types.AppState) int { return state.NotesPanelWidth },
		set: func(state *types.AppState, width int) { state.NotesPanelWidth = width },
	},
	sidePanelModeDebug: panelWidthAccessors{
		get: func(state *types.AppState) int { return state.DebugPanelWidth },
		set: func(state *types.AppState, width int) { state.DebugPanelWidth = width },
	},
	sidePanelModeContext: panelWidthAccessors{
		get: func(state *types.AppState) int { return state.ContextPanelWidth },
		set: func(state *types.AppState, width int) { state.ContextPanelWidth = width },
	},
}

func panelLayoutPersistencePolicy(mode sidePanelMode) PanelLayoutPersistencePolicy {
	if policy, ok := panelLayoutPersistencePolicies[mode]; ok {
		return policy
	}
	return noopPanelLayoutPersistencePolicy{}
}
