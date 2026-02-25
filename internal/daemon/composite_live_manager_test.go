package daemon

import (
	"context"
	"errors"
	"testing"

	"control/internal/types"
)

type stubTurnCapableFactory struct {
	provider string
	create   func(context.Context, *types.Session, *types.SessionMeta) (TurnCapableSession, error)
}

func (f *stubTurnCapableFactory) ProviderName() string { return f.provider }

func (f *stubTurnCapableFactory) CreateTurnCapable(ctx context.Context, session *types.Session, meta *types.SessionMeta) (TurnCapableSession, error) {
	if f.create == nil {
		return nil, errors.New("create not configured")
	}
	return f.create(ctx, session, meta)
}

type stubManagedSession struct {
	startTurnID        string
	managedStartTurnID string
	managedSessionID   string
	managedMeta        *types.SessionMeta
}

func (s *stubManagedSession) Events() (<-chan types.CodexEvent, func()) {
	ch := make(chan types.CodexEvent)
	close(ch)
	return ch, func() {}
}

func (s *stubManagedSession) Close() {}

func (s *stubManagedSession) SessionID() string { return "sess-1" }

func (s *stubManagedSession) StartTurn(context.Context, []map[string]any, *types.SessionRuntimeOptions) (string, error) {
	return s.startTurnID, nil
}

func (s *stubManagedSession) StartTurnForSession(_ context.Context, session *types.Session, meta *types.SessionMeta, _ []map[string]any, _ *types.SessionRuntimeOptions) (string, error) {
	if session != nil {
		s.managedSessionID = session.ID
	}
	if meta != nil {
		copy := *meta
		s.managedMeta = &copy
	}
	return s.managedStartTurnID, nil
}

func (s *stubManagedSession) Interrupt(context.Context) error { return nil }

func (s *stubManagedSession) ActiveTurnID() string { return "" }

type stubCloseAwareSession struct {
	closed bool
}

func (s *stubCloseAwareSession) Events() (<-chan types.CodexEvent, func()) {
	ch := make(chan types.CodexEvent)
	close(ch)
	return ch, func() {}
}

func (s *stubCloseAwareSession) Close() {}

func (s *stubCloseAwareSession) SessionID() string { return "sess-1" }

func (s *stubCloseAwareSession) StartTurn(context.Context, []map[string]any, *types.SessionRuntimeOptions) (string, error) {
	return "turn", nil
}

func (s *stubCloseAwareSession) Interrupt(context.Context) error { return nil }

func (s *stubCloseAwareSession) ActiveTurnID() string { return "" }

func (s *stubCloseAwareSession) IsClosed() bool { return s.closed }

func TestCompositeLiveManagerStartTurnPrefersManagedStarter(t *testing.T) {
	stub := &stubManagedSession{
		startTurnID:        "plain-turn",
		managedStartTurnID: "managed-turn",
	}
	manager := NewCompositeLiveManager(nil, nil, &stubTurnCapableFactory{
		provider: "codex",
		create: func(context.Context, *types.Session, *types.SessionMeta) (TurnCapableSession, error) {
			return stub, nil
		},
	})
	session := &types.Session{ID: "sess-1", Provider: "codex"}
	meta := &types.SessionMeta{SessionID: session.ID, ThreadID: "thread-1"}

	turnID, err := manager.StartTurn(context.Background(), session, meta, []map[string]any{{"type": "message"}}, nil)
	if err != nil {
		t.Fatalf("StartTurn: %v", err)
	}
	if turnID != "managed-turn" {
		t.Fatalf("expected managed StartTurn path, got %q", turnID)
	}
	if stub.managedSessionID != "sess-1" {
		t.Fatalf("expected managed path to receive session id, got %q", stub.managedSessionID)
	}
	if stub.managedMeta == nil || stub.managedMeta.ThreadID != "thread-1" {
		t.Fatalf("expected managed path to receive meta, got %#v", stub.managedMeta)
	}
}

func TestCompositeLiveManagerEnsureEvictsClosedSession(t *testing.T) {
	first := &stubCloseAwareSession{closed: true}
	second := &stubCloseAwareSession{closed: false}
	createCalls := 0
	manager := NewCompositeLiveManager(nil, nil, &stubTurnCapableFactory{
		provider: "codex",
		create: func(context.Context, *types.Session, *types.SessionMeta) (TurnCapableSession, error) {
			createCalls++
			if createCalls == 1 {
				return first, nil
			}
			return second, nil
		},
	})
	session := &types.Session{ID: "sess-1", Provider: "codex"}
	meta := &types.SessionMeta{SessionID: session.ID}

	if _, err := manager.ensure(context.Background(), session, meta); err != nil {
		t.Fatalf("ensure first: %v", err)
	}
	got, err := manager.ensure(context.Background(), session, meta)
	if err != nil {
		t.Fatalf("ensure second: %v", err)
	}
	if createCalls != 2 {
		t.Fatalf("expected closed cached session to be recreated, got %d create calls", createCalls)
	}
	if got != second {
		t.Fatalf("expected recreated live session, got %#v", got)
	}
}
