package app

import (
	"testing"

	"control/internal/config"
)

func TestApplyUIConfigSetsSharedAutoGrowInputBounds(t *testing.T) {
	m := NewModel(nil)
	expandByDefault := false
	m.applyUIConfig(config.UIConfig{
		Input: config.UIInputConfig{
			MultilineMinHeight: 4,
			MultilineMaxHeight: 6,
		},
		Chat: config.UIChatConfig{
			TimestampMode: "iso",
		},
		Sidebar: config.UISidebarConfig{
			ExpandByDefault: &expandByDefault,
		},
	})

	if m.chatInput == nil || m.noteInput == nil || m.recentsReplyInput == nil {
		t.Fatalf("expected chat, note, and recents reply inputs")
	}

	m.chatInput.SetValue("1\n2\n3\n4\n5\n6\n7")
	m.noteInput.SetValue("1\n2\n3\n4\n5\n6\n7")
	m.recentsReplyInput.SetValue("1\n2\n3\n4\n5\n6\n7")
	if got := m.chatInput.Height(); got != 6 {
		t.Fatalf("expected chat input max height 6, got %d", got)
	}
	if got := m.noteInput.Height(); got != 6 {
		t.Fatalf("expected note input max height 6, got %d", got)
	}
	if got := m.recentsReplyInput.Height(); got != 6 {
		t.Fatalf("expected recents reply input max height 6, got %d", got)
	}

	m.chatInput.SetValue("short")
	m.noteInput.SetValue("short")
	m.recentsReplyInput.SetValue("short")
	if got := m.chatInput.Height(); got != 4 {
		t.Fatalf("expected chat input min height 4, got %d", got)
	}
	if got := m.noteInput.Height(); got != 4 {
		t.Fatalf("expected note input min height 4, got %d", got)
	}
	if got := m.recentsReplyInput.Height(); got != 4 {
		t.Fatalf("expected recents reply input min height 4, got %d", got)
	}
	if m.timestampMode != ChatTimestampModeISO {
		t.Fatalf("expected timestamp mode ISO, got %q", m.timestampMode)
	}
	if m.sidebar == nil {
		t.Fatalf("expected sidebar controller")
	}
	if m.sidebar.IsWorkspaceExpanded("ws-any") {
		t.Fatalf("expected sidebar expand_by_default=false from UI config")
	}
	if !m.showRecents {
		t.Fatalf("expected recents to be shown by default when not overridden")
	}
}
