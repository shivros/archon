package app

import (
	"context"
	"errors"
	"testing"

	"control/internal/types"
)

type debugStreamCommandMock struct {
	ch     <-chan types.DebugEvent
	cancel func()
	err    error
	id     string
	calls  int
}

func (m *debugStreamCommandMock) DebugStream(_ context.Context, id string) (<-chan types.DebugEvent, func(), error) {
	m.calls++
	m.id = id
	return m.ch, m.cancel, m.err
}

func TestOpenDebugStreamCmdReturnsDebugStreamMsg(t *testing.T) {
	stream := make(chan types.DebugEvent)
	mock := &debugStreamCommandMock{ch: stream}

	cmd := openDebugStreamCmd(mock, "s1")
	if cmd == nil {
		t.Fatalf("expected debug stream command")
	}
	msg, ok := cmd().(debugStreamMsg)
	if !ok {
		t.Fatalf("expected debugStreamMsg, got %T", cmd())
	}
	if msg.id != "s1" {
		t.Fatalf("expected id s1, got %q", msg.id)
	}
	if msg.ch == nil {
		t.Fatalf("expected stream channel")
	}
	if msg.err != nil {
		t.Fatalf("unexpected error: %v", msg.err)
	}
	if mock.calls != 1 || mock.id != "s1" {
		t.Fatalf("expected one DebugStream call for s1, got calls=%d id=%q", mock.calls, mock.id)
	}
}

func TestOpenDebugStreamCmdPropagatesErrors(t *testing.T) {
	mock := &debugStreamCommandMock{err: errors.New("connect failed")}

	msg, ok := openDebugStreamCmd(mock, "s2")().(debugStreamMsg)
	if !ok {
		t.Fatalf("expected debugStreamMsg")
	}
	if msg.id != "s2" {
		t.Fatalf("expected id s2, got %q", msg.id)
	}
	if msg.err == nil || msg.err.Error() != "connect failed" {
		t.Fatalf("expected propagated error, got %v", msg.err)
	}
}
