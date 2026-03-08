package app

import (
	"errors"
	"testing"
	"time"

	"control/internal/daemon/transcriptdomain"
)

func TestRegisterPendingSendInitializesOptimisticMapWhenNil(t *testing.T) {
	m := NewModel(nil)
	m.optimisticSends = nil

	m.registerPendingSend(1, "s1", "codex", "hello")

	entry, ok := m.pendingSends[1]
	if !ok {
		t.Fatalf("expected pending send entry to be registered")
	}
	if entry.state != pendingSendStateSending {
		t.Fatalf("expected pending send state to be sending, got %#v", entry)
	}
	view, ok := m.optimisticSends[1]
	if !ok {
		t.Fatalf("expected optimistic send entry to be registered")
	}
	if view.status != ChatStatusSending || view.text != "hello" || view.headerLine != -1 {
		t.Fatalf("unexpected optimistic send entry: %#v", view)
	}
}

func TestRegisterPendingSendHeaderNoopWithoutPendingEntry(t *testing.T) {
	m := NewModel(nil)
	m.optimisticSends[3] = optimisticSendEntry{
		token:      3,
		key:        "unchanged",
		sessionID:  "old-session",
		headerLine: 9,
	}

	m.registerPendingSendHeader(3, "s1", "codex", 0)

	view := m.optimisticSends[3]
	if view.key != "unchanged" || view.sessionID != "old-session" || view.headerLine != 9 {
		t.Fatalf("expected optimistic entry to remain unchanged when pending entry is missing, got %#v", view)
	}
}

func TestRegisterPendingSendHeaderUpdatesPendingEntryWhenViewMissing(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	m.pendingSends[7] = pendingSend{key: "stale", sessionID: "stale", provider: ""}
	delete(m.optimisticSends, 7)

	m.registerPendingSendHeader(7, "s1", "opencode", 2)

	entry := m.pendingSends[7]
	if entry.sessionID != "s1" || entry.provider != "opencode" || entry.key != m.selectedKey() {
		t.Fatalf("expected pending send entry to update from active selection context, got %#v", entry)
	}
	if _, ok := m.optimisticSends[7]; ok {
		t.Fatalf("expected missing optimistic entry to remain missing")
	}
}

func TestClearPendingSendUpdatesCachedTranscriptWhenNotSelected(t *testing.T) {
	m := NewModel(nil)
	m.pendingSends[9] = pendingSend{
		key:       "sess:s2",
		sessionID: "s2",
		state:     pendingSendStateSending,
	}
	m.optimisticSends[9] = optimisticSendEntry{
		token:      9,
		key:        "sess:s2",
		sessionID:  "s2",
		headerLine: 0,
		status:     ChatStatusSending,
	}
	m.transcriptCache["sess:s2"] = []ChatBlock{
		{Role: ChatRoleUser, Text: "hello", Status: ChatStatusSending},
	}

	m.clearPendingSend(9, " turn-9 ")

	entry := m.pendingSends[9]
	if entry.state != pendingSendStateAcknowledged || entry.turnID != "turn-9" {
		t.Fatalf("expected pending send acknowledgement state and turn id, got %#v", entry)
	}
	view := m.optimisticSends[9]
	if view.status != ChatStatusNone || view.turnID != "turn-9" {
		t.Fatalf("expected optimistic send to clear status and carry turn id, got %#v", view)
	}
	cached := m.transcriptCache["sess:s2"]
	if len(cached) != 1 || cached[0].Status != ChatStatusNone {
		t.Fatalf("expected cached transcript status to clear after ack, got %#v", cached)
	}
}

func TestClearPendingSendWithEmptyKeyTransitionsWithoutCacheMutation(t *testing.T) {
	m := NewModel(nil)
	m.pendingSends[5] = pendingSend{
		key:       "",
		sessionID: "s1",
		state:     pendingSendStateSending,
	}
	m.optimisticSends[5] = optimisticSendEntry{
		token:      5,
		key:        "",
		sessionID:  "s1",
		headerLine: 0,
		status:     ChatStatusSending,
	}

	m.clearPendingSend(5, "")

	entry := m.pendingSends[5]
	if entry.state != pendingSendStateAcknowledged || entry.turnID != "" {
		t.Fatalf("expected ack state with empty turn id when none provided, got %#v", entry)
	}
	view := m.optimisticSends[5]
	if view.status != ChatStatusNone {
		t.Fatalf("expected optimistic status to clear even when key is empty, got %#v", view)
	}
	if len(m.transcriptCache) != 0 {
		t.Fatalf("expected transcript cache to remain untouched for empty key branch, got %#v", m.transcriptCache)
	}
}

func TestMarkPendingSendFailedNoopWhenTokenMissing(t *testing.T) {
	m := NewModel(nil)

	m.markPendingSendFailed(99, errors.New("boom"))

	if len(m.pendingSends) != 0 || len(m.optimisticSends) != 0 {
		t.Fatalf("expected missing-token failure to be a no-op")
	}
}

func TestApplyLiveSessionItemsSnapshotNoopForNilModel(t *testing.T) {
	var m *Model
	if m.applyLiveSessionItemsSnapshot(sessionItemsMessageContext{id: "s1"}) {
		t.Fatalf("expected nil model snapshot apply to return false")
	}
}

func TestLiveAndProjectedSnapshotsRetainThenResolveOptimisticUser(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	key := m.selectedKey()
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	m.pendingSends[1] = pendingSend{
		key:       key,
		sessionID: "s1",
		turnID:    "turn-1",
		state:     pendingSendStateAcknowledged,
	}
	m.optimisticSends[1] = optimisticSendEntry{
		token:      1,
		key:        key,
		sessionID:  "s1",
		headerLine: 0,
		text:       "hello from compose",
		createdAt:  now,
		status:     ChatStatusNone,
		turnID:     "turn-1",
	}

	m.applySessionProjection(sessionProjectionSourceHistory, "s1", key, []ChatBlock{
		{ID: "a-1", Role: ChatRoleAgent, Text: "assistant reply", TurnID: "turn-1"},
	})
	blocks := m.currentBlocks()
	if countBlocksByRoleAndText(blocks, ChatRoleUser, "hello from compose") != 1 {
		t.Fatalf("expected optimistic user block during projection, got %#v", blocks)
	}

	m.transcriptStream.SetSnapshot(transcriptdomain.TranscriptSnapshot{
		Revision: transcriptdomain.MustParseRevisionToken("2"),
		Blocks: []transcriptdomain.Block{
			{ID: "a-1", Kind: "assistant_message", Role: "assistant", Text: "assistant reply"},
		},
	})
	m.requestActivity = requestActivity{active: true, sessionID: "s1", eventCount: 1}
	applied := m.applyLiveSessionItemsSnapshot(sessionItemsMessageContext{
		source: sessionProjectionSourceTail,
		id:     "s1",
		key:    key,
	})
	if !applied {
		t.Fatalf("expected live snapshot overlay path to apply")
	}
	blocks = m.currentBlocks()
	if countBlocksByRoleAndText(blocks, ChatRoleUser, "hello from compose") != 1 {
		t.Fatalf("expected optimistic user block retained on live snapshot refresh, got %#v", blocks)
	}

	m.applySessionProjection(sessionProjectionSourceHistory, "s1", key, []ChatBlock{
		{ID: "u-1", Role: ChatRoleUser, Text: "hello from compose", TurnID: "turn-1"},
		{ID: "a-1", Role: ChatRoleAgent, Text: "assistant reply", TurnID: "turn-1"},
	})
	blocks = m.currentBlocks()
	if countBlocksByRoleAndText(blocks, ChatRoleUser, "hello from compose") != 1 {
		t.Fatalf("expected canonical user block to replace optimistic overlay without duplicates, got %#v", blocks)
	}
	if _, ok := m.pendingSends[1]; ok {
		t.Fatalf("expected pending send token to prune once canonical user arrives")
	}
	if _, ok := m.optimisticSends[1]; ok {
		t.Fatalf("expected optimistic send token to prune once canonical user arrives")
	}
}

func countBlocksByRoleAndText(blocks []ChatBlock, role ChatRole, text string) int {
	count := 0
	for _, block := range blocks {
		if block.Role == role && block.Text == text {
			count++
		}
	}
	return count
}
