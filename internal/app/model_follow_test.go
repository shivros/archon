package app

import (
	"fmt"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestViewportScrollDownFromBottomPausesFollow(t *testing.T) {
	m := NewModel(nil)
	m.resize(120, 40)
	seedFollowContent(&m, 200)
	if !m.follow {
		t.Fatalf("expected follow to start enabled")
	}

	if !m.handleViewportScroll(tea.KeyPressMsg{Code: tea.KeyDown}) {
		t.Fatalf("expected down scroll to be handled")
	}
	if m.follow {
		t.Fatalf("expected follow to pause after manual scroll")
	}
	if m.status != "follow: paused" {
		t.Fatalf("unexpected status %q", m.status)
	}
}

func TestViewportScrollDownAtBottomResumesFollow(t *testing.T) {
	m := NewModel(nil)
	m.resize(120, 40)
	seedFollowContent(&m, 200)

	if !m.handleViewportScroll(tea.KeyPressMsg{Code: tea.KeyUp}) {
		t.Fatalf("expected up scroll to be handled")
	}
	if m.follow {
		t.Fatalf("expected follow to pause after scrolling up")
	}

	if !m.handleViewportScroll(tea.KeyPressMsg{Code: tea.KeyDown}) {
		t.Fatalf("expected down scroll to be handled")
	}
	if !m.follow {
		t.Fatalf("expected follow to resume after returning to bottom")
	}
	if m.status != "follow: on" {
		t.Fatalf("unexpected status %q", m.status)
	}
}

func TestEnterMessageSelectionPausesFollow(t *testing.T) {
	m := NewModel(nil)
	m.resize(120, 40)
	seedFollowContent(&m, 120)
	m.applyBlocks([]ChatBlock{
		{Role: ChatRoleAgent, Text: "first"},
		{Role: ChatRoleAgent, Text: "second"},
	})
	if !m.follow {
		t.Fatalf("expected follow enabled before message selection")
	}

	m.enterMessageSelection()
	if !m.messageSelectActive {
		t.Fatalf("expected message selection to activate")
	}
	if m.follow {
		t.Fatalf("expected follow to pause in message selection mode")
	}
}

func seedFollowContent(m *Model, lineCount int) {
	blocks := make([]ChatBlock, 0, lineCount)
	for i := 0; i < lineCount; i++ {
		blocks = append(blocks, ChatBlock{Role: ChatRoleAgent, Text: fmt.Sprintf("line %d", i)})
	}
	m.applyBlocks(blocks)
	m.enableFollow(false)
}
