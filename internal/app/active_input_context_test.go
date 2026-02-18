package app

import "testing"

func TestActiveInputContextRecentsReplyRequiresActiveSession(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	m.mode = uiModeRecents
	m.recentsReplySessionID = ""

	if _, ok := m.activeInputContext(); ok {
		t.Fatalf("did not expect recents input context without an active reply target")
	}

	m.recentsReplySessionID = "s1"
	ctx, ok := m.activeInputContext()
	if !ok {
		t.Fatalf("expected recents input context with active reply target")
	}
	if ctx.input != m.recentsReplyInput {
		t.Fatalf("expected recents reply input context")
	}
	if ctx.footer == nil || ctx.footer.InputFooter() != "enter send • esc cancel" {
		t.Fatalf("expected recents reply footer in input context")
	}
}
