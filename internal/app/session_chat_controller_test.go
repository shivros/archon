package app

import (
	"context"
	"testing"

	"control/internal/client"
	"control/internal/types"
)

func TestSessionChatControllerUsesNarrowSessionChatAPI(t *testing.T) {
	api := &sessionChatAPIMock{}
	controller := NewSessionChatController(api, NewCodexStreamController(10, 10))

	sendCmd := controller.SendMessage("s1", "hello")
	if sendCmd == nil {
		t.Fatalf("expected send command")
	}
	sendMsg, ok := sendCmd().(sendMsg)
	if !ok {
		t.Fatalf("expected sendMsg result")
	}
	if sendMsg.id != "s1" {
		t.Fatalf("expected session id s1, got %q", sendMsg.id)
	}
	if api.sendCalls != 1 {
		t.Fatalf("expected exactly one send call")
	}

	eventCmd := controller.OpenEventStream("s1")
	if eventCmd == nil {
		t.Fatalf("expected events command")
	}
	eventsMsg, ok := eventCmd().(eventsMsg)
	if !ok {
		t.Fatalf("expected eventsMsg result")
	}
	if eventsMsg.id != "s1" {
		t.Fatalf("expected session id s1, got %q", eventsMsg.id)
	}
	if api.eventCalls != 1 {
		t.Fatalf("expected exactly one event stream call")
	}
}

type sessionChatAPIMock struct {
	sendCalls  int
	eventCalls int
}

func (m *sessionChatAPIMock) SendMessage(_ context.Context, _ string, _ client.SendSessionRequest) (*client.SendSessionResponse, error) {
	m.sendCalls++
	return &client.SendSessionResponse{OK: true}, nil
}

func (m *sessionChatAPIMock) EventStream(_ context.Context, _ string) (<-chan types.CodexEvent, func(), error) {
	m.eventCalls++
	ch := make(chan types.CodexEvent)
	close(ch)
	return ch, func() {}, nil
}
