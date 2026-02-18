package app

import (
	"reflect"
	"testing"
	"time"

	"control/internal/types"
)

func TestHistoryMsgDoesNotOverwriteRecentsView(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	m.enterRecentsView(&sidebarItem{kind: sidebarRecentsAll})
	m.pendingSessionKey = "sess:s1"
	before := append([]ChatBlock(nil), m.currentBlocks()...)

	handled, cmd := m.reduceStateMessages(historyMsg{
		id:  "s1",
		key: "sess:s1",
		items: []map[string]any{
			{"type": "assistant", "content": []any{map[string]any{"type": "text", "text": "latest agent reply"}}},
		},
	})
	if !handled {
		t.Fatalf("expected historyMsg to be handled")
	}
	if cmd != nil {
		t.Fatalf("expected no follow-up command for historyMsg")
	}
	if m.mode != uiModeRecents {
		t.Fatalf("expected to remain in recents mode, got %v", m.mode)
	}
	after := m.currentBlocks()
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("expected recents blocks to remain unchanged, before=%#v after=%#v", before, after)
	}
	cached := m.transcriptCache["sess:s1"]
	if len(cached) == 0 {
		t.Fatalf("expected transcript cache to update while recents view is active")
	}
	if cached[len(cached)-1].Role != ChatRoleAgent {
		t.Fatalf("expected cached transcript to contain agent reply, got %#v", cached)
	}
}

func TestTailMsgDoesNotOverwriteRecentsView(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	m.enterRecentsView(&sidebarItem{kind: sidebarRecentsAll})
	m.pendingSessionKey = "sess:s1"
	before := append([]ChatBlock(nil), m.currentBlocks()...)

	handled, cmd := m.reduceStateMessages(tailMsg{
		id:  "s1",
		key: "sess:s1",
		items: []map[string]any{
			{"type": "assistant", "content": []any{map[string]any{"type": "text", "text": "tail agent reply"}}},
		},
	})
	if !handled {
		t.Fatalf("expected tailMsg to be handled")
	}
	if cmd != nil {
		t.Fatalf("expected no follow-up command for tailMsg")
	}
	if m.mode != uiModeRecents {
		t.Fatalf("expected to remain in recents mode, got %v", m.mode)
	}
	after := m.currentBlocks()
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("expected recents blocks to remain unchanged, before=%#v after=%#v", before, after)
	}
	cached := m.transcriptCache["sess:s1"]
	if len(cached) == 0 {
		t.Fatalf("expected transcript cache to update while recents view is active")
	}
	if cached[len(cached)-1].Role != ChatRoleAgent {
		t.Fatalf("expected cached transcript to contain agent reply, got %#v", cached)
	}
}

func TestConsumeCodexTickDoesNotOverwriteRecentsView(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	m.enterRecentsView(&sidebarItem{kind: sidebarRecentsAll})
	m.pendingSessionKey = "sess:s1"
	before := append([]ChatBlock(nil), m.currentBlocks()...)

	ch := make(chan types.CodexEvent, 1)
	ch <- types.CodexEvent{
		Method: "item/updated",
		Params: []byte(`{"item":{"type":"agentMessage","delta":"stream update"}}`),
	}
	close(ch)
	m.codexStream.SetStream(ch, nil)

	m.consumeCodexTick(time.Now())

	after := m.currentBlocks()
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("expected recents blocks to remain unchanged, before=%#v after=%#v", before, after)
	}
}
