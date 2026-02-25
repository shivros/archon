package app

import "strings"

type SidebarSortPolicy interface {
	Normalize(state sidebarSortState) sidebarSortState
	Cycle(state sidebarSortState, step int) sidebarSortState
	ToggleReverse(state sidebarSortState) sidebarSortState
	Label(key sidebarSortKey) string
}

type SortStripHintPolicy interface {
	BadgeFor(command, fallback string, bindings *Keybindings) string
}

type SortStripVisibilityPolicy interface {
	ShowStrip(mode uiMode, sidebarCollapsed bool) bool
}

type defaultSidebarSortPolicy struct{}

func (defaultSidebarSortPolicy) Normalize(state sidebarSortState) sidebarSortState {
	state.Key = parseSidebarSortKey(string(state.Key))
	return state
}

func (defaultSidebarSortPolicy) Cycle(state sidebarSortState, step int) sidebarSortState {
	state.Key = cycleSidebarSortKey(state.Key, step)
	return state
}

func (defaultSidebarSortPolicy) ToggleReverse(state sidebarSortState) sidebarSortState {
	state.Reverse = !state.Reverse
	return state
}

func (defaultSidebarSortPolicy) Label(key sidebarSortKey) string {
	return sidebarSortLabel(key)
}

type defaultSortStripHintPolicy struct{}

func (defaultSortStripHintPolicy) BadgeFor(command, fallback string, bindings *Keybindings) string {
	if bindings == nil {
		return formatKeyBadge(fallback)
	}
	key := strings.TrimSpace(bindings.KeyFor(command, fallback))
	if key == "" {
		return ""
	}
	return formatKeyBadge(key)
}

type defaultSortStripVisibilityPolicy struct{}

func (defaultSortStripVisibilityPolicy) ShowStrip(mode uiMode, sidebarCollapsed bool) bool {
	if sidebarCollapsed {
		return false
	}
	switch mode {
	case uiModeNormal, uiModeCompose, uiModeRecents, uiModeNotes:
		return true
	default:
		return false
	}
}

func WithSidebarSortPolicy(policy SidebarSortPolicy) ModelOption {
	return func(m *Model) {
		if m == nil {
			return
		}
		if policy == nil {
			m.sidebarSortPolicy = defaultSidebarSortPolicy{}
			return
		}
		m.sidebarSortPolicy = policy
	}
}

func WithSortStripHintPolicy(policy SortStripHintPolicy) ModelOption {
	return func(m *Model) {
		if m == nil {
			return
		}
		if policy == nil {
			m.sortStripHintPolicy = defaultSortStripHintPolicy{}
			return
		}
		m.sortStripHintPolicy = policy
	}
}

func WithSortStripVisibilityPolicy(policy SortStripVisibilityPolicy) ModelOption {
	return func(m *Model) {
		if m == nil {
			return
		}
		if policy == nil {
			m.sortStripVisibilityPolicy = defaultSortStripVisibilityPolicy{}
			return
		}
		m.sortStripVisibilityPolicy = policy
	}
}

func (m *Model) sidebarSortPolicyOrDefault() SidebarSortPolicy {
	if m == nil || m.sidebarSortPolicy == nil {
		return defaultSidebarSortPolicy{}
	}
	return m.sidebarSortPolicy
}

func (m *Model) sortStripHintPolicyOrDefault() SortStripHintPolicy {
	if m == nil || m.sortStripHintPolicy == nil {
		return defaultSortStripHintPolicy{}
	}
	return m.sortStripHintPolicy
}

func (m *Model) sortStripVisibilityPolicyOrDefault() SortStripVisibilityPolicy {
	if m == nil || m.sortStripVisibilityPolicy == nil {
		return defaultSortStripVisibilityPolicy{}
	}
	return m.sortStripVisibilityPolicy
}
