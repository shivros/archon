package app

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

func applyProjectedSessionCmd(t *testing.T, m *Model, cmd tea.Cmd) {
	t.Helper()
	if cmd == nil {
		return
	}
	msg := cmd()
	projected, ok := msg.(sessionBlocksProjectedMsg)
	if !ok {
		t.Fatalf("expected sessionBlocksProjectedMsg, got %T", msg)
	}
	handled, followUp := m.reduceStateMessages(projected)
	if !handled {
		t.Fatalf("expected projected session blocks message to be handled")
	}
	if followUp != nil {
		t.Fatalf("expected no follow-up command after projected session blocks")
	}
}
