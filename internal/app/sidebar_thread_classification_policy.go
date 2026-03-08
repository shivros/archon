package app

// SidebarThreadClassificationPolicy classifies sidebar items that represent
// chat thread targets whose selection state should be preserved for specific
// gestures.
type SidebarThreadClassificationPolicy interface {
	IsThreadTarget(entry *sidebarItem) bool
}

type defaultSidebarThreadClassificationPolicy struct{}

func (defaultSidebarThreadClassificationPolicy) IsThreadTarget(entry *sidebarItem) bool {
	return entry != nil && entry.kind == sidebarSession
}

func WithSidebarThreadClassificationPolicy(policy SidebarThreadClassificationPolicy) ModelOption {
	return func(m *Model) {
		if m == nil {
			return
		}
		if policy == nil {
			m.sidebarThreadClassificationPolicy = defaultSidebarThreadClassificationPolicy{}
			return
		}
		m.sidebarThreadClassificationPolicy = policy
	}
}

func (m *Model) sidebarThreadClassificationPolicyOrDefault() SidebarThreadClassificationPolicy {
	if m == nil || m.sidebarThreadClassificationPolicy == nil {
		return defaultSidebarThreadClassificationPolicy{}
	}
	return m.sidebarThreadClassificationPolicy
}
