package app

import (
	"fmt"
	"hash/fnv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
)

type requestActivity struct {
	active               bool
	sessionID            string
	provider             string
	startedAt            time.Time
	lastEventAt          time.Time
	lastVisibleAt        time.Time
	eventCount           int
	visibleCount         int
	reasoningUpdates     int
	hiddenReasoningCount int
	lastReasoningHash    uint64
	baselineAgentReplies int
	lastHistoryRefreshAt time.Time
	refreshCount         int
}

func (m *Model) startRequestActivity(sessionID, provider string) {
	now := time.Now()
	m.requestActivity = requestActivity{
		active:               true,
		sessionID:            strings.TrimSpace(sessionID),
		provider:             strings.TrimSpace(provider),
		startedAt:            now,
		lastEventAt:          now,
		lastVisibleAt:        now,
		baselineAgentReplies: countAgentRepliesBlocks(m.currentBlocks()),
	}
	hash, hasReasoning, _ := m.currentReasoningSnapshot()
	if hasReasoning {
		m.requestActivity.lastReasoningHash = hash
	}
}

func (m *Model) stopRequestActivity() {
	m.requestActivity = requestActivity{}
}

func (m *Model) stopRequestActivityFor(sessionID string) {
	if !m.requestActivity.active {
		return
	}
	sessionID = strings.TrimSpace(sessionID)
	activeID := strings.TrimSpace(m.requestActivity.sessionID)
	if activeID != "" && sessionID != "" && activeID != sessionID {
		return
	}
	m.stopRequestActivity()
}

func (m *Model) noteRequestEvent(sessionID string, count int) {
	if !m.matchRequestActivitySession(sessionID) {
		return
	}
	if count < 1 {
		count = 1
	}
	m.requestActivity.lastEventAt = time.Now()
	m.requestActivity.eventCount += count
}

func (m *Model) noteRequestVisibleUpdate(sessionID string) {
	if !m.matchRequestActivitySession(sessionID) {
		return
	}
	now := time.Now()
	m.noteSessionMetaActivity(sessionID, "", now.UTC())
	m.requestActivity.lastEventAt = now
	m.requestActivity.lastVisibleAt = now
	m.requestActivity.visibleCount++

	hash, hasReasoning, hasCollapsed := m.currentReasoningSnapshot()
	if hasReasoning && hash != m.requestActivity.lastReasoningHash {
		m.requestActivity.lastReasoningHash = hash
		m.requestActivity.reasoningUpdates++
		if hasCollapsed {
			m.requestActivity.hiddenReasoningCount++
			hidden := m.requestActivity.hiddenReasoningCount
			if hidden == 1 || hidden%3 == 0 {
				m.setBackgroundStatus(fmt.Sprintf("reasoning updated (%d hidden, press e to expand)", hidden))
			}
		}
	}
	m.maybeCompleteRequestActivity(sessionID)
}

func (m *Model) currentReasoningSnapshot() (hash uint64, hasReasoning bool, hasCollapsed bool) {
	if m == nil {
		return 0, false, false
	}
	if m.contentBlocks != nil {
		return m.reasoningSnapshotHash, m.reasoningSnapshotHas, m.reasoningSnapshotCollapsed
	}
	return reasoningSnapshotState(m.currentBlocks())
}

func (m *Model) maybeAutoRefreshHistory(now time.Time) tea.Cmd {
	if !m.requestActivity.active || m.mode != uiModeCompose {
		return nil
	}
	sessionID := strings.TrimSpace(m.requestActivity.sessionID)
	if sessionID == "" || sessionID != m.activeStreamTargetID() {
		return nil
	}
	lastVisibleAt := m.requestActivity.lastVisibleAt
	if lastVisibleAt.IsZero() {
		lastVisibleAt = m.requestActivity.startedAt
	}
	if now.Sub(lastVisibleAt) < requestStaleRefreshDelay {
		return nil
	}
	if !m.requestActivity.lastHistoryRefreshAt.IsZero() && now.Sub(m.requestActivity.lastHistoryRefreshAt) < requestRefreshCooldown {
		return nil
	}

	key := strings.TrimSpace(m.pendingSessionKey)
	if key == "" {
		key = strings.TrimSpace(m.selectedKey())
	}
	if key == "" {
		key = "sess:" + sessionID
		m.pendingSessionKey = key
	}

	m.requestActivity.lastHistoryRefreshAt = now
	m.requestActivity.refreshCount++
	if m.requestActivity.refreshCount == 1 || m.requestActivity.refreshCount%3 == 0 {
		m.setBackgroundStatus("still working; refreshing thread")
	}
	provider := m.providerForSessionID(sessionID)
	ctx := m.requestScopeContext(requestScopeSessionLoad)
	historyCmd := fetchHistoryCmdWithContext(m.sessionHistoryAPI, sessionID, key, m.historyFetchLinesInitial(), ctx)
	cmds := []tea.Cmd{}
	if shouldStreamItems(provider) {
		if m.itemStream != nil && !m.itemStream.HasStream() {
			cmds = append(cmds, openItemsCmd(m.sessionAPI, sessionID))
		}
	} else if provider == "codex" {
		if m.codexStream != nil && !m.codexStream.HasStream() {
			cmds = append(cmds, openEventsCmd(m.sessionAPI, sessionID))
		}
	}
	cmds = append(cmds, historyCmd)
	if shouldStreamItems(provider) && providerSupportsApprovals(provider) {
		cmds = append(cmds, fetchApprovalsCmdWithContext(m.sessionAPI, sessionID, ctx))
	}
	return tea.Batch(cmds...)
}

func (m *Model) composeActivityLine(now time.Time) string {
	if !m.requestActivity.active || m.mode != uiModeCompose {
		return ""
	}
	req := m.requestActivity
	phase := "thinking"
	if req.reasoningUpdates > 0 {
		phase = "reasoning"
	}
	lastEventAt := req.lastEventAt
	if lastEventAt.IsZero() {
		lastEventAt = req.startedAt
	}
	lastVisibleAt := req.lastVisibleAt
	if lastVisibleAt.IsZero() {
		lastVisibleAt = req.startedAt
	}

	line := fmt.Sprintf("AI %s %s | events %d | last %s ago | visible %s ago",
		phase,
		formatClock(now.Sub(req.startedAt)),
		req.eventCount,
		formatAgo(now.Sub(lastEventAt)),
		formatAgo(now.Sub(lastVisibleAt)),
	)
	if req.hiddenReasoningCount > 0 {
		line += fmt.Sprintf(" | reasoning hidden: %d (press e)", req.hiddenReasoningCount)
	}
	if w := m.viewport.Width(); w > 0 {
		line = truncateToWidth(line, w)
	}
	return line
}

func (m *Model) matchRequestActivitySession(sessionID string) bool {
	if !m.requestActivity.active {
		return false
	}
	sessionID = strings.TrimSpace(sessionID)
	activeID := strings.TrimSpace(m.requestActivity.sessionID)
	if activeID == "" {
		if sessionID == "" {
			return true
		}
		m.requestActivity.sessionID = sessionID
		return true
	}
	return sessionID != "" && sessionID == activeID
}

func (m *Model) maybeCompleteRequestActivity(sessionID string) {
	if !m.matchRequestActivitySession(sessionID) {
		return
	}
	currentAgents := countAgentRepliesBlocks(m.currentBlocks())
	if currentAgents > m.requestActivity.baselineAgentReplies {
		m.stopRequestActivity()
	}
}

func reasoningSnapshotState(blocks []ChatBlock) (hash uint64, hasReasoning bool, hasCollapsed bool) {
	if len(blocks) == 0 {
		return 0, false, false
	}
	h := fnv.New64a()
	for _, block := range blocks {
		if block.Role != ChatRoleReasoning {
			continue
		}
		text := strings.TrimSpace(block.Text)
		if text == "" {
			continue
		}
		hasReasoning = true
		if block.Collapsed {
			hasCollapsed = true
		}
		_, _ = h.Write([]byte(text))
		_, _ = h.Write([]byte{'\n'})
	}
	if !hasReasoning {
		return 0, false, hasCollapsed
	}
	return h.Sum64(), true, hasCollapsed
}

func formatClock(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	secs := int(d.Round(time.Second).Seconds())
	if secs < 0 {
		secs = 0
	}
	return fmt.Sprintf("%02d:%02d", secs/60, secs%60)
}

func formatAgo(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	if d < time.Second {
		return "<1s"
	}
	secs := int(d.Round(time.Second).Seconds())
	if secs < 60 {
		return fmt.Sprintf("%ds", secs)
	}
	mins := secs / 60
	rem := secs % 60
	return fmt.Sprintf("%dm%02ds", mins, rem)
}
