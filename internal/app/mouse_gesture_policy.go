package app

import tea "charm.land/bubbletea/v2"

// MouseGesturePolicy centralizes modifier-based mouse gesture semantics.
type MouseGesturePolicy interface {
	IsLinkOpenGesture(msg tea.MouseMsg) bool
	PreservesSelection(msg tea.MouseMsg) bool
	AllowsHighlight(msg tea.MouseMsg) bool
}

type defaultMouseGesturePolicy struct{}

func (defaultMouseGesturePolicy) IsLinkOpenGesture(msg tea.MouseMsg) bool {
	_, ok := msg.(tea.MouseClickMsg)
	if !ok {
		return false
	}
	mouse := msg.Mouse()
	return mouse.Button == tea.MouseLeft && mouse.Mod.Contains(tea.ModCtrl)
}

func (p defaultMouseGesturePolicy) PreservesSelection(msg tea.MouseMsg) bool {
	return p.IsLinkOpenGesture(msg)
}

func (defaultMouseGesturePolicy) AllowsHighlight(msg tea.MouseMsg) bool {
	return !msg.Mouse().Mod.Contains(tea.ModCtrl)
}

func WithMouseGesturePolicy(policy MouseGesturePolicy) ModelOption {
	return func(m *Model) {
		if m == nil {
			return
		}
		if policy == nil {
			m.mouseGesturePolicy = defaultMouseGesturePolicy{}
			return
		}
		m.mouseGesturePolicy = policy
	}
}

func (m *Model) mouseGesturePolicyOrDefault() MouseGesturePolicy {
	if m == nil || m.mouseGesturePolicy == nil {
		return defaultMouseGesturePolicy{}
	}
	return m.mouseGesturePolicy
}
