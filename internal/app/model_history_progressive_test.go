package app

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"control/internal/client"
	"control/internal/daemon/transcriptdomain"
	"control/internal/types"
)

func TestDefaultHistoryLoadPolicyUsesSinglePassLines(t *testing.T) {
	m := NewModel(nil)
	if got := m.historyFetchLinesInitial(); got != defaultInitialHistoryLines {
		t.Fatalf("expected initial history lines %d, got %d", defaultInitialHistoryLines, got)
	}
}

func TestLoadSelectedSessionSkipsHistoryBackfillByDefault(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	m.sessionTranscriptAPI = bootstrapTranscriptAPIStub{}
	item := m.selectedItem()
	if item == nil || item.session == nil {
		t.Fatalf("expected selected session item")
	}

	cmd := m.loadSelectedSession(item)
	if cmd == nil {
		t.Fatalf("expected load command")
	}
	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		t.Fatalf("expected load batch, got %T", msg)
	}
	if len(batch) != 2 {
		t.Fatalf("expected 2 commands (transcript snapshot, approvals), got %d", len(batch))
	}
}

func TestLoadSelectedSessionUsesItemsSnapshotBootstrapForItemProviders(t *testing.T) {
	m := newPhase0ModelWithSession("kilocode")
	m.sessionTranscriptAPI = bootstrapTranscriptAPIStub{}
	item := m.selectedItem()
	if item == nil || item.session == nil {
		t.Fatalf("expected selected session item")
	}

	cmd := m.loadSelectedSession(item)
	if cmd == nil {
		t.Fatalf("expected load command")
	}
	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		t.Fatalf("expected load batch, got %T", msg)
	}
	if len(batch) != 2 {
		t.Fatalf("expected 2 commands (transcript snapshot, approvals), got %d", len(batch))
	}
}

func TestStartSessionClearsPreviousContentAndSkipsBackfillCommand(t *testing.T) {
	m := NewModel(nil)
	m.sessionTranscriptAPI = bootstrapTranscriptAPIStub{}
	m.setSnapshotBlocks([]ChatBlock{{Role: ChatRoleAgent, Text: "old reply should clear"}})

	session := &types.Session{
		ID:       "s1",
		Provider: "codex",
		Status:   types.SessionStatusRunning,
		Title:    "Session",
	}
	handled, cmd := m.reduceStateMessages(startSessionMsg{session: session})
	if !handled {
		t.Fatalf("expected start session message to be handled")
	}
	if cmd == nil {
		t.Fatalf("expected follow-up commands for started session")
	}
	if blocks := m.currentBlocks(); len(blocks) != 0 {
		t.Fatalf("expected previous transcript blocks to clear, got %#v", blocks)
	}
	if raw := strings.Join(m.currentLines(), "\n"); strings.Contains(raw, "old reply should clear") {
		t.Fatalf("expected previous transcript text to clear, got %q", raw)
	}
	if !m.loading {
		t.Fatalf("expected loading to start")
	}
	if m.loadingKey != "sess:s1" {
		t.Fatalf("expected loading key sess:s1, got %q", m.loadingKey)
	}
	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		t.Fatalf("expected batch command, got %T", msg)
	}
	plan := m.sessionBootstrapPolicyOrDefault().SessionStartPlan(session.Provider, session.Status)
	expected := 1 // fetch sessions
	if plan.FetchTranscript {
		expected++ // initial transcript snapshot
	}
	if plan.FetchApprovals {
		expected++ // approvals
	}
	if plan.OpenTranscript {
		expected++ // transcript stream
	}
	expected++ // recents state save debounce
	if len(batch) != expected {
		t.Fatalf("expected %d start-session commands without backfill, got %d", expected, len(batch))
	}
}

func TestStartSessionUsesItemsSnapshotBootstrapForItemProviders(t *testing.T) {
	m := NewModel(nil)
	m.sessionTranscriptAPI = bootstrapTranscriptAPIStub{}
	session := &types.Session{
		ID:       "s1",
		Provider: "kilocode",
		Status:   types.SessionStatusRunning,
		Title:    "Session",
	}

	handled, cmd := m.reduceStateMessages(startSessionMsg{session: session})
	if !handled {
		t.Fatalf("expected start session message to be handled")
	}
	if cmd == nil {
		t.Fatalf("expected follow-up commands for started session")
	}
	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	if !ok {
		t.Fatalf("expected batch command, got %T", msg)
	}
	// fetch sessions + recents save + transcript snapshot + approvals
	if len(batch) != 4 {
		t.Fatalf("expected 4 start-session commands for snapshot-first bootstrap, got %d", len(batch))
	}
}

func TestApplyProviderSelectionClearsPreviousContent(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	m.applyBlocks([]ChatBlock{{Role: ChatRoleAgent, Text: "old reply should clear"}})
	m.newSession = &newSessionTarget{workspaceID: "ws1"}

	cmd := m.applyProviderSelection("codex")
	if cmd == nil {
		t.Fatalf("expected provider options fetch command")
	}
	lines := strings.Join(m.currentLines(), "\n")
	if strings.Contains(lines, "old reply should clear") {
		t.Fatalf("expected previous transcript text to clear, got %q", lines)
	}
	if !strings.Contains(lines, "New session. Send your first message to start.") {
		t.Fatalf("expected new-session placeholder, got %q", lines)
	}
	if m.mode != uiModeCompose {
		t.Fatalf("expected compose mode, got %v", m.mode)
	}
}

func TestActiveStreamTargetIDEmptyDuringNewSession(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	m.newSession = &newSessionTarget{
		workspaceID: "ws1",
		provider:    "codex",
	}

	if got := m.activeStreamTargetID(); got != "" {
		t.Fatalf("expected empty stream target while new session is pending, got %q", got)
	}
}

func TestHistoryPollUsesInitialHistoryLines(t *testing.T) {
	history := &historyRecorderAPI{}
	m := newPhase0ModelWithSession("codex")
	m.sessionHistoryAPI = history
	m.mode = uiModeCompose
	if m.compose != nil {
		m.compose.Enter("s1", "Session")
	}

	handled, cmd := m.reduceStateMessages(historyPollMsg{
		id:        "s1",
		key:       "sess:s1",
		attempt:   0,
		minAgents: -1,
	})
	if !handled {
		t.Fatalf("expected history poll message to be handled")
	}
	if cmd == nil {
		t.Fatalf("expected history poll command")
	}
	msg := cmd()
	batch, ok := msg.(tea.BatchMsg)
	if !ok || len(batch) == 0 {
		t.Fatalf("expected batch command, got %T", msg)
	}
	_ = batch[0]()
	if len(history.calls) != 1 || history.calls[0] != defaultInitialHistoryLines {
		t.Fatalf("expected one history call with %d lines, got %#v", defaultInitialHistoryLines, history.calls)
	}
}

func TestHistoryPollWaitsForPendingAsyncProjection(t *testing.T) {
	history := &historyRecorderAPI{}
	m := newPhase0ModelWithSession("custom")
	m.sessionHistoryAPI = history
	m.mode = uiModeCompose
	m.pendingSessionKey = "sess:s1"
	if m.compose != nil {
		m.compose.Enter("s1", "Session")
	}
	ctx, seq := m.sessionProjectionCoordinatorOrDefault().Schedule(sessionProjectionToken("sess:s1", "s1"), context.Background())
	if ctx == nil || seq == 0 {
		t.Fatalf("expected pending session projection to be scheduled")
	}

	handled, cmd := m.reduceStateMessages(historyPollMsg{
		id:        "s1",
		key:       "sess:s1",
		attempt:   0,
		minAgents: -1,
	})
	if !handled {
		t.Fatalf("expected history poll message to be handled")
	}
	if cmd == nil {
		t.Fatalf("expected history poll retry command")
	}
	if len(history.calls) != 0 {
		t.Fatalf("expected history fetch to wait for pending projection, got %#v", history.calls)
	}
	if _, ok := cmd().(historyPollMsg); !ok {
		t.Fatalf("expected retry historyPollMsg, got %T", cmd())
	}
}

type historyRecorderAPI struct {
	calls []int
}

func (h *historyRecorderAPI) History(_ context.Context, _ string, lines int) (*client.TailItemsResponse, error) {
	h.calls = append(h.calls, lines)
	return &client.TailItemsResponse{Items: []map[string]any{}}, nil
}

func TestUnifiedBootstrapSharesTranscriptFollowBetweenComposeAndRecents(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	api := newCountingTranscriptAPIStub("s1")
	m.sessionTranscriptAPI = api
	m.enterCompose("s1")

	streamMsg := openTranscriptStreamCmd(api, "s1", "")().(transcriptStreamMsg)
	m.applyTranscriptStreamMsg(streamMsg)
	if api.OpenCount("s1") != 1 {
		t.Fatalf("expected exactly one transcript attach after compose open, got %d", api.OpenCount("s1"))
	}

	now := time.Now().UTC()
	if m.recents != nil {
		m.recents.StartRun("s1", "turn-0", now)
	}
	watchCmd := m.beginRecentsCompletionWatch("s1", "turn-0")
	if watchCmd != nil {
		t.Fatalf("expected recents completion watch to reuse shared follow for active session")
	}
	if api.OpenCount("s1") != 1 {
		t.Fatalf("expected shared follow to avoid duplicate attach, got %d", api.OpenCount("s1"))
	}

	stream := api.Stream("s1")
	stream <- transcriptdomain.TranscriptEvent{
		Kind:     transcriptdomain.TranscriptEventDelta,
		Revision: transcriptdomain.MustParseRevisionToken("1"),
		Delta: []transcriptdomain.Block{{
			Kind: "assistant_message",
			Role: "assistant",
			Text: "shared transcript preview",
		}},
	}
	stream <- transcriptdomain.TranscriptEvent{
		Kind:     transcriptdomain.TranscriptEventTurnCompleted,
		Revision: transcriptdomain.MustParseRevisionToken("2"),
		Turn: &transcriptdomain.TurnState{
			State:  transcriptdomain.TurnStateCompleted,
			TurnID: "turn-1",
		},
	}
	close(stream)

	if cmd := m.consumeTranscriptTick(time.Now()); cmd != nil {
		_ = cmd()
	}
	if !m.recents.IsReady("s1") {
		t.Fatalf("expected shared transcript completion signal to move recents run to ready")
	}

	m.enterRecentsView(&sidebarItem{kind: sidebarRecentsAll})
	preview := m.previewForSession("s1", "turn-1")
	if strings.TrimSpace(preview.Preview) == "" {
		t.Fatalf("expected recents to observe shared transcript preview from compose stream")
	}
	if api.OpenCount("s1") != 1 {
		t.Fatalf("expected one transcript attach for compose+recents shared session, got %d", api.OpenCount("s1"))
	}
}

type countingTranscriptAPIStub struct {
	mu       sync.Mutex
	streams  map[string]chan transcriptdomain.TranscriptEvent
	openByID map[string]int
}

func newCountingTranscriptAPIStub(sessionIDs ...string) *countingTranscriptAPIStub {
	streams := make(map[string]chan transcriptdomain.TranscriptEvent, len(sessionIDs))
	for _, sessionID := range sessionIDs {
		streams[sessionID] = make(chan transcriptdomain.TranscriptEvent, 16)
	}
	return &countingTranscriptAPIStub{
		streams:  streams,
		openByID: map[string]int{},
	}
}

func (s *countingTranscriptAPIStub) GetTranscriptSnapshot(context.Context, string, int) (*client.TranscriptSnapshotResponse, error) {
	return &client.TranscriptSnapshotResponse{}, nil
}

func (s *countingTranscriptAPIStub) TranscriptStream(_ context.Context, id string, _ string) (<-chan transcriptdomain.TranscriptEvent, func(), error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id = strings.TrimSpace(id)
	s.openByID[id]++
	ch, ok := s.streams[id]
	if !ok {
		ch = make(chan transcriptdomain.TranscriptEvent, 16)
		s.streams[id] = ch
	}
	return ch, func() {}, nil
}

func (s *countingTranscriptAPIStub) OpenCount(id string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.openByID[strings.TrimSpace(id)]
}

func (s *countingTranscriptAPIStub) Stream(id string) chan transcriptdomain.TranscriptEvent {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.streams[strings.TrimSpace(id)]
}
