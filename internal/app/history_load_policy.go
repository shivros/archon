package app

import "time"

const (
	defaultInitialHistoryLines = 250
	defaultHistoryBackfillWait = 150 * time.Millisecond
)

type SessionHistoryLoadPolicy interface {
	InitialLines(defaultLines int) int
	BackfillLines(defaultLines int) int
	BackfillDelay(base time.Duration) time.Duration
	ShouldBackfill(initialLines, backfillLines int) bool
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

func (defaultSessionHistoryLoadPolicy) BackfillLines(defaultLines int) int {
	if defaultLines <= 0 {
		return defaultInitialHistoryLines
	}
	return defaultLines
}

func (defaultSessionHistoryLoadPolicy) BackfillDelay(base time.Duration) time.Duration {
	if base <= 0 {
		return defaultHistoryBackfillWait
	}
	return base
}

func (defaultSessionHistoryLoadPolicy) ShouldBackfill(initialLines, backfillLines int) bool {
	if initialLines <= 0 || backfillLines <= 0 {
		return false
	}
	return backfillLines > initialLines
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

func (m *Model) historyFetchLinesBackfill() int {
	lines := m.historyLoadPolicyOrDefault().BackfillLines(maxViewportLines)
	if lines <= 0 {
		return maxViewportLines
	}
	return lines
}

func (m *Model) historyBackfillDelay() time.Duration {
	delay := m.historyLoadPolicyOrDefault().BackfillDelay(defaultHistoryBackfillWait)
	if delay <= 0 {
		return defaultHistoryBackfillWait
	}
	return delay
}
