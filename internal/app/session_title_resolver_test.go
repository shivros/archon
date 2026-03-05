package app

import (
	"testing"

	"control/internal/types"
)

func TestResolveSessionTitleOrder(t *testing.T) {
	session := &types.Session{ID: "s1", Title: "session title"}
	meta := &types.SessionMeta{Title: "meta title", InitialInput: "initial"}
	if got := ResolveSessionTitle(session, meta, "fallback"); got != "meta title" {
		t.Fatalf("expected meta title, got %q", got)
	}

	meta.Title = ""
	if got := ResolveSessionTitle(session, meta, "fallback"); got != "initial" {
		t.Fatalf("expected initial input, got %q", got)
	}

	meta.InitialInput = ""
	if got := ResolveSessionTitle(session, meta, "fallback"); got != "session title" {
		t.Fatalf("expected session title, got %q", got)
	}

	session.Title = ""
	if got := ResolveSessionTitle(session, meta, "fallback"); got != "s1" {
		t.Fatalf("expected session id fallback, got %q", got)
	}

	session.ID = ""
	if got := ResolveSessionTitle(session, meta, "fallback"); got != "fallback" {
		t.Fatalf("expected explicit fallback id, got %q", got)
	}
}
