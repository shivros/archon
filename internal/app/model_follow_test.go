package app

import (
	"fmt"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestViewportScrollDownFromBottomKeepsFollowEnabled(t *testing.T) {
	m := NewModel(nil)
	m.resize(120, 40)
	seedFollowContent(&m, 200)
	if !m.follow {
		t.Fatalf("expected follow to start enabled")
	}

	if !m.handleViewportScroll(tea.KeyPressMsg{Code: tea.KeyDown}) {
		t.Fatalf("expected down scroll to be handled")
	}
	if !m.follow {
		t.Fatalf("expected follow to stay enabled when scrolling down")
	}
	if m.status == "follow: paused" {
		t.Fatalf("expected down scroll to avoid pausing follow")
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

func TestViewportScrollDownWhilePausedAtBottomResumesFollow(t *testing.T) {
	m := NewModel(nil)
	m.resize(120, 40)
	seedFollowContent(&m, 200)
	m.pauseFollow(false)
	m.viewport.GotoBottom()

	if !m.handleViewportScroll(tea.KeyPressMsg{Code: tea.KeyDown}) {
		t.Fatalf("expected down scroll to be handled")
	}
	if !m.follow {
		t.Fatalf("expected follow to resume when scrolling down at bottom while paused")
	}
	if m.status != "follow: on" {
		t.Fatalf("unexpected status %q", m.status)
	}
}

func TestViewportScrollUpWhilePausedAtBottomStaysPaused(t *testing.T) {
	m := NewModel(nil)
	m.resize(120, 40)
	seedFollowContent(&m, 10)
	m.pauseFollow(false)
	m.viewport.GotoBottom()

	if !m.handleViewportScroll(tea.KeyPressMsg{Code: tea.KeyUp}) {
		t.Fatalf("expected up scroll to be handled")
	}
	if m.follow {
		t.Fatalf("expected follow to remain paused when scrolling up")
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

	// enterMessageSelection now opens the action modal. The old
	// messageSelectActive-based behavior is preserved for keyboard-only
	// usage (j/k navigation, y/p shortcuts). Verify the modal opens.
	m.enterMessageSelection()
	if m.messageActionModal == nil || !m.messageActionModal.IsOpen() {
		t.Fatalf("expected message action modal to be open")
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
