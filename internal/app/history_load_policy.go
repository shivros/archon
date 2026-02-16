package app

const (
	defaultInitialHistoryLines = 250
)

type SessionHistoryLoadPolicy interface {
	InitialLines(defaultLines int) int
}

type defaultSessionHistoryLoadPolicy struct{}

func WithSessionHistoryLoadPolicy(policy SessionHistoryLoadPolicy) ModelOption {
	return func(m *Model) {
		if m == nil {
			return
		}
		if policy == nil {
			m.historyLoadPolicy = defaultSessionHistoryLoadPolicy{}
			return
		}
		m.historyLoadPolicy = policy
	}
}

func (defaultSessionHistoryLoadPolicy) InitialLines(defaultLines int) int {
	if defaultLines <= 0 {
		return defaultInitialHistoryLines
	}
	if defaultLines < defaultInitialHistoryLines {
		return defaultLines
	}
	return defaultInitialHistoryLines
}

func (m *Model) historyLoadPolicyOrDefault() SessionHistoryLoadPolicy {
	if m == nil || m.historyLoadPolicy == nil {
		return defaultSessionHistoryLoadPolicy{}
	}
	return m.historyLoadPolicy
}

func (m *Model) historyFetchLinesInitial() int {
	lines := m.historyLoadPolicyOrDefault().InitialLines(maxViewportLines)
	if lines <= 0 {
		return maxViewportLines
	}
	return lines
}
