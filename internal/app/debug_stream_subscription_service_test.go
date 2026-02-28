package app

import (
	"context"
	"testing"

	tea "charm.land/bubbletea/v2"
)

type testDebugSubscriptionMsg struct{}

type stubDebugSubscriptionService struct {
	calls int
}

func (s *stubDebugSubscriptionService) Ensure(DebugStreamSubscriptionContext) tea.Cmd {
	s.calls++
	return nil
}

func TestDefaultDebugStreamSubscriptionServiceEnsuresWhenEligible(t *testing.T) {
	service := defaultDebugStreamSubscriptionService{}
	replaceCalls := 0
	openCalls := 0
	var gotScope string
	var gotSession string

	cmd := service.Ensure(DebugStreamSubscriptionContext{
		Enabled:         true,
		StreamAPIReady:  true,
		ConsumerReady:   true,
		ActiveSessionID: " s1 ",
		HasStream:       false,
		ScopeInFlight:   false,
		ScopeName:       "debug_stream",
		ReplaceScope: func(name string) context.Context {
			replaceCalls++
			gotScope = name
			return context.Background()
		},
		Open: func(sessionID string, parent context.Context) tea.Cmd {
			openCalls++
			gotSession = sessionID
			return func() tea.Msg { return testDebugSubscriptionMsg{} }
		},
	})

	if cmd == nil {
		t.Fatalf("expected subscription command")
	}
	if replaceCalls != 1 || openCalls != 1 {
		t.Fatalf("expected one replace/open call each, got replace=%d open=%d", replaceCalls, openCalls)
	}
	if gotScope != "debug_stream" || gotSession != "s1" {
		t.Fatalf("unexpected scope/session values scope=%q session=%q", gotScope, gotSession)
	}
	if _, ok := cmd().(testDebugSubscriptionMsg); !ok {
		t.Fatalf("expected test debug subscription message")
	}
}

func TestDefaultDebugStreamSubscriptionServiceNoopWhenIneligible(t *testing.T) {
	service := defaultDebugStreamSubscriptionService{}
	cases := []DebugStreamSubscriptionContext{
		{},
		{Enabled: true, StreamAPIReady: true, ConsumerReady: true, ActiveSessionID: "", ReplaceScope: func(string) context.Context { return context.Background() }, Open: func(string, context.Context) tea.Cmd { return nil }},
		{Enabled: true, StreamAPIReady: true, ConsumerReady: true, ActiveSessionID: "s1", HasStream: true, ReplaceScope: func(string) context.Context { return context.Background() }, Open: func(string, context.Context) tea.Cmd { return nil }},
		{Enabled: true, StreamAPIReady: true, ConsumerReady: true, ActiveSessionID: "s1", ScopeInFlight: true, ReplaceScope: func(string) context.Context { return context.Background() }, Open: func(string, context.Context) tea.Cmd { return nil }},
		{Enabled: true, StreamAPIReady: true, ConsumerReady: true, ActiveSessionID: "s1"},
	}
	for i, tc := range cases {
		if cmd := service.Ensure(tc); cmd != nil {
			t.Fatalf("case %d: expected nil command", i)
		}
	}
}

func TestWithDebugStreamSubscriptionServiceConfiguresAndResetsDefault(t *testing.T) {
	custom := &stubDebugSubscriptionService{}
	m := NewModel(nil, WithDebugStreamSubscriptionService(custom))
	if m.debugStreamSubscriptionService != custom {
		t.Fatalf("expected custom debug stream subscription service")
	}
	WithDebugStreamSubscriptionService(nil)(&m)
	if _, ok := m.debugStreamSubscriptionService.(defaultDebugStreamSubscriptionService); !ok {
		t.Fatalf("expected default debug stream subscription service after reset, got %T", m.debugStreamSubscriptionService)
	}
}

func TestWithDebugStreamSubscriptionServiceHandlesNilModel(t *testing.T) {
	WithDebugStreamSubscriptionService(&stubDebugSubscriptionService{})(nil)
}

func TestDebugStreamSubscriptionServiceOrDefaultHandlesNilModel(t *testing.T) {
	var m *Model
	if _, ok := m.debugStreamSubscriptionServiceOrDefault().(defaultDebugStreamSubscriptionService); !ok {
		t.Fatalf("expected default service for nil model")
	}
}
