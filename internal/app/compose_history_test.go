package app

import "testing"

func TestComposeHistoryNavigateUpDown(t *testing.T) {
	m := NewModel(nil)
	if m.compose != nil {
		m.compose.Enter("s1", "session")
	}

	m.recordComposeHistory("s1", "first")
	m.recordComposeHistory("s1", "second")

	value, ok := m.composeHistoryNavigate(-1, "")
	if !ok || value != "second" {
		t.Fatalf("expected first up to return second, got ok=%v value=%q", ok, value)
	}
	value, ok = m.composeHistoryNavigate(-1, "")
	if !ok || value != "first" {
		t.Fatalf("expected second up to return first, got ok=%v value=%q", ok, value)
	}
	value, ok = m.composeHistoryNavigate(1, "")
	if !ok || value != "second" {
		t.Fatalf("expected down to return second, got ok=%v value=%q", ok, value)
	}
	value, ok = m.composeHistoryNavigate(1, "")
	if !ok || value != "" {
		t.Fatalf("expected down at latest to clear input, got ok=%v value=%q", ok, value)
	}
	value, ok = m.composeHistoryNavigate(1, "")
	if ok {
		t.Fatalf("expected extra down to be a no-op, got ok=%v value=%q", ok, value)
	}
}

func TestComposeHistoryIsSessionScoped(t *testing.T) {
	m := NewModel(nil)

	m.recordComposeHistory("s1", "alpha")
	m.recordComposeHistory("s2", "beta")

	if m.compose != nil {
		m.compose.Enter("s1", "session one")
	}
	value, ok := m.composeHistoryNavigate(-1, "")
	if !ok || value != "alpha" {
		t.Fatalf("expected s1 history, got ok=%v value=%q", ok, value)
	}

	if m.compose != nil {
		m.compose.Enter("s2", "session two")
	}
	value, ok = m.composeHistoryNavigate(-1, "")
	if !ok || value != "beta" {
		t.Fatalf("expected s2 history, got ok=%v value=%q", ok, value)
	}
}

func TestComposeHistoryPersistsViaAppState(t *testing.T) {
	m := NewModel(nil)
	m.recordComposeHistory("s1", "alpha")
	m.recordComposeHistory("s1", "beta")
	m.recordComposeHistory("s2", "gamma")

	m.syncAppStateComposeHistory()
	state := m.appState

	n := NewModel(nil)
	n.applyAppState(&state)
	if n.compose != nil {
		n.compose.Enter("s1", "session one")
	}
	value, ok := n.composeHistoryNavigate(-1, "")
	if !ok || value != "beta" {
		t.Fatalf("expected persisted latest s1 history, got ok=%v value=%q", ok, value)
	}

	if n.compose != nil {
		n.compose.Enter("s2", "session two")
	}
	value, ok = n.composeHistoryNavigate(-1, "")
	if !ok || value != "gamma" {
		t.Fatalf("expected persisted s2 history, got ok=%v value=%q", ok, value)
	}
}
