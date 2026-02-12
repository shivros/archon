package app

import (
	"testing"

	"control/internal/config"
)

func TestApplyUIConfigSetsSharedAutoGrowInputBounds(t *testing.T) {
	m := NewModel(nil)
	m.applyUIConfig(config.UIConfig{
		Input: config.UIInputConfig{
			MultilineMinHeight: 4,
			MultilineMaxHeight: 6,
		},
	})

	if m.chatInput == nil || m.noteInput == nil {
		t.Fatalf("expected chat and note inputs")
	}

	m.chatInput.SetValue("1\n2\n3\n4\n5\n6\n7")
	m.noteInput.SetValue("1\n2\n3\n4\n5\n6\n7")
	if got := m.chatInput.Height(); got != 6 {
		t.Fatalf("expected chat input max height 6, got %d", got)
	}
	if got := m.noteInput.Height(); got != 6 {
		t.Fatalf("expected note input max height 6, got %d", got)
	}

	m.chatInput.SetValue("short")
	m.noteInput.SetValue("short")
	if got := m.chatInput.Height(); got != 4 {
		t.Fatalf("expected chat input min height 4, got %d", got)
	}
	if got := m.noteInput.Height(); got != 4 {
		t.Fatalf("expected note input min height 4, got %d", got)
	}
}
