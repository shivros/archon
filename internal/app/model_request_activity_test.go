package app

import (
	"testing"
	"time"
)

func TestRequestActivityStopsAfterFirstAgentReply(t *testing.T) {
	m := NewModel(nil)
	m.enterCompose("s1")
	m.startRequestActivity("s1", "codex")
	if !m.requestActivity.active {
		t.Fatalf("expected request activity to start")
	}

	m.applyBlocks([]ChatBlock{{Role: ChatRoleAgent, Text: "hello"}})
	m.noteRequestVisibleUpdate("s1")
	if m.requestActivity.active {
		t.Fatalf("expected request activity to stop after agent reply")
	}
}

func TestRequestActivityTracksHiddenReasoningUpdates(t *testing.T) {
	m := NewModel(nil)
	m.enterCompose("s1")
	m.startRequestActivity("s1", "codex")

	m.applyBlocks([]ChatBlock{
		{ID: "reasoning:older", Role: ChatRoleReasoning, Text: "Reasoning\nolder"},
		{ID: "reasoning:1", Role: ChatRoleReasoning, Text: "Reasoning\nstep one"},
	})
	m.noteRequestVisibleUpdate("s1")

	if m.requestActivity.reasoningUpdates != 1 {
		t.Fatalf("expected reasoningUpdates=1, got %d", m.requestActivity.reasoningUpdates)
	}
	if m.requestActivity.hiddenReasoningCount != 1 {
		t.Fatalf("expected hiddenReasoningCount=1, got %d", m.requestActivity.hiddenReasoningCount)
	}
	line := m.composeActivityLine(time.Now())
	if line == "" {
		t.Fatalf("expected compose activity line")
	}
}

func TestRequestActivityAutoRefreshWhenVisibleOutputIsStale(t *testing.T) {
	m := NewModel(nil)
	m.enterCompose("s1")
	m.pendingSessionKey = "sess:s1"
	m.startRequestActivity("s1", "codex")
	m.requestActivity.lastVisibleAt = time.Now().Add(-2 * requestStaleRefreshDelay)

	cmd := m.maybeAutoRefreshHistory(time.Now())
	if cmd == nil {
		t.Fatalf("expected stale history refresh command")
	}
	if m.requestActivity.refreshCount != 1 {
		t.Fatalf("expected refreshCount=1, got %d", m.requestActivity.refreshCount)
	}
}

func TestRequestActivityAutoExpandsNewestReasoningWhileActive(t *testing.T) {
	m := NewModel(nil)
	m.enterCompose("s1")
	m.startRequestActivity("s1", "codex")

	m.applyBlocks([]ChatBlock{
		{ID: "reasoning:old", Role: ChatRoleReasoning, Text: "Reasoning\nfirst"},
		{ID: "reasoning:new", Role: ChatRoleReasoning, Text: "Reasoning\nsecond"},
	})

	if len(m.contentBlocks) != 2 {
		t.Fatalf("expected two reasoning blocks, got %d", len(m.contentBlocks))
	}
	if !m.contentBlocks[0].Collapsed {
		t.Fatalf("expected older reasoning block to remain collapsed")
	}
	if m.contentBlocks[1].Collapsed {
		t.Fatalf("expected newest reasoning block to auto-expand while request is active")
	}
}

func TestRequestActivityAutoExpandHonorsManualCollapse(t *testing.T) {
	m := NewModel(nil)
	m.enterCompose("s1")
	m.startRequestActivity("s1", "codex")

	m.applyBlocks([]ChatBlock{
		{ID: "reasoning:old", Role: ChatRoleReasoning, Text: "Reasoning\nfirst"},
		{ID: "reasoning:new", Role: ChatRoleReasoning, Text: "Reasoning\nsecond"},
	})
	if m.contentBlocks[1].Collapsed {
		t.Fatalf("expected newest reasoning to start expanded")
	}
	if !m.toggleReasoningByIndex(1) {
		t.Fatalf("expected manual toggle on newest reasoning block")
	}
	if !m.contentBlocks[1].Collapsed {
		t.Fatalf("expected newest reasoning block to be manually collapsed")
	}

	m.applyBlocks([]ChatBlock{
		{ID: "reasoning:old", Role: ChatRoleReasoning, Text: "Reasoning\nfirst"},
		{ID: "reasoning:new", Role: ChatRoleReasoning, Text: "Reasoning\nsecond updated"},
	})
	if !m.contentBlocks[1].Collapsed {
		t.Fatalf("expected manual collapse preference to be preserved")
	}
}
