package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"control/internal/daemon/transcriptdomain"
)

func TestTranscriptEventRecentsCompletionSignalPolicy(t *testing.T) {
	policy := transcriptEventRecentsCompletionSignalPolicy{}
	tests := []struct {
		name    string
		event   transcriptdomain.TranscriptEvent
		turnID  string
		matched bool
	}{
		{
			name:    "turn completed with turn id",
			event:   transcriptdomain.TranscriptEvent{Kind: transcriptdomain.TranscriptEventTurnCompleted, Turn: &transcriptdomain.TurnState{TurnID: "turn-a"}},
			turnID:  "turn-a",
			matched: true,
		},
		{
			name:    "turn failed without turn payload",
			event:   transcriptdomain.TranscriptEvent{Kind: transcriptdomain.TranscriptEventTurnFailed},
			turnID:  "",
			matched: true,
		},
		{
			name:    "non completion event",
			event:   transcriptdomain.TranscriptEvent{Kind: transcriptdomain.TranscriptEventDelta},
			turnID:  "",
			matched: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			turnID, matched := policy.CompletionFromTranscriptEvent(tt.event)
			if matched != tt.matched {
				t.Fatalf("matched=%v want=%v", matched, tt.matched)
			}
			if turnID != tt.turnID {
				t.Fatalf("turnID=%q want=%q", turnID, tt.turnID)
			}
		})
	}
}

func TestWatchRecentsTurnCompletionCmdUsesSignalPolicy(t *testing.T) {
	stream := make(chan transcriptdomain.TranscriptEvent, 2)
	stream <- transcriptdomain.TranscriptEvent{Kind: transcriptdomain.TranscriptEventDelta}
	stream <- transcriptdomain.TranscriptEvent{Kind: transcriptdomain.TranscriptEventDelta}
	close(stream)

	api := transcriptStreamAPIStub{stream: stream}
	policy := staticRecentsCompletionSignalPolicy{
		matchKind: transcriptdomain.TranscriptEventDelta,
		turnID:    "turn-policy",
	}

	cmd := watchRecentsTurnCompletionCmdWithContext(api, policy, "s1", "turn-expected", context.Background())
	msg, ok := cmd().(recentsTurnCompletedMsg)
	if !ok {
		t.Fatalf("expected recentsTurnCompletedMsg")
	}
	if msg.err != nil {
		t.Fatalf("expected no error, got %v", msg.err)
	}
	if msg.turnID != "turn-policy" {
		t.Fatalf("expected policy turn id, got %q", msg.turnID)
	}
}

func TestWatchRecentsTurnCompletionCmdWrapper(t *testing.T) {
	stream := make(chan transcriptdomain.TranscriptEvent, 1)
	stream <- transcriptdomain.TranscriptEvent{Kind: transcriptdomain.TranscriptEventDelta}
	close(stream)
	api := transcriptStreamAPIStub{stream: stream}
	policy := staticRecentsCompletionSignalPolicy{
		matchKind: transcriptdomain.TranscriptEventDelta,
		turnID:    "turn-wrapper",
	}

	cmd := watchRecentsTurnCompletionCmd(api, policy, "s1", "turn-expected")
	msg, ok := cmd().(recentsTurnCompletedMsg)
	if !ok {
		t.Fatalf("expected recentsTurnCompletedMsg")
	}
	if msg.err != nil {
		t.Fatalf("expected no error, got %v", msg.err)
	}
	if msg.turnID != "turn-wrapper" {
		t.Fatalf("expected wrapper to preserve policy turn id, got %q", msg.turnID)
	}
}

func TestWatchRecentsTurnCompletionCmdEmptySessionID(t *testing.T) {
	cmd := watchRecentsTurnCompletionCmdWithContext(transcriptStreamAPIStub{}, staticRecentsCompletionSignalPolicy{}, "   ", "turn-expected", context.Background())
	msg, ok := cmd().(recentsTurnCompletedMsg)
	if !ok {
		t.Fatalf("expected recentsTurnCompletedMsg")
	}
	if msg.err == nil {
		t.Fatalf("expected validation error for missing session id")
	}
}

func TestWatchRecentsTurnCompletionCmdSubscribeError(t *testing.T) {
	boom := errors.New("subscribe failed")
	cmd := watchRecentsTurnCompletionCmdWithContext(
		transcriptStreamAPIStub{err: boom},
		staticRecentsCompletionSignalPolicy{},
		"s1",
		"turn-expected",
		context.Background(),
	)
	msg, ok := cmd().(recentsTurnCompletedMsg)
	if !ok {
		t.Fatalf("expected recentsTurnCompletedMsg")
	}
	if !errors.Is(msg.err, boom) {
		t.Fatalf("expected subscribe error, got %v", msg.err)
	}
}

func TestWatchRecentsTurnCompletionCmdContextCanceled(t *testing.T) {
	blocking := make(chan transcriptdomain.TranscriptEvent)
	api := transcriptStreamAPIStub{stream: blocking}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	cmd := watchRecentsTurnCompletionCmdWithContext(api, staticRecentsCompletionSignalPolicy{}, "s1", "turn-expected", ctx)
	msg, ok := cmd().(recentsTurnCompletedMsg)
	if !ok {
		t.Fatalf("expected recentsTurnCompletedMsg")
	}
	if !errors.Is(msg.err, context.Canceled) {
		t.Fatalf("expected context canceled error, got %v", msg.err)
	}
}

func TestWatchRecentsTurnCompletionCmdDeadlineFallback(t *testing.T) {
	stream := make(chan transcriptdomain.TranscriptEvent)
	api := transcriptStreamAPIStub{stream: stream}
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
	defer cancel()

	cmd := watchRecentsTurnCompletionCmdWithContext(api, staticRecentsCompletionSignalPolicy{}, "s1", "turn-expected", ctx)
	msg, ok := cmd().(recentsTurnCompletedMsg)
	if !ok {
		t.Fatalf("expected recentsTurnCompletedMsg")
	}
	if msg.err != nil {
		t.Fatalf("expected deadline fallback without error, got %v", msg.err)
	}
}

func TestWatchRecentsTurnCompletionCmdChannelClosedBeforeMatch(t *testing.T) {
	stream := make(chan transcriptdomain.TranscriptEvent)
	close(stream)
	cmd := watchRecentsTurnCompletionCmdWithContext(
		transcriptStreamAPIStub{stream: stream},
		staticRecentsCompletionSignalPolicy{matchKind: transcriptdomain.TranscriptEventDelta},
		"s1",
		"turn-expected",
		context.Background(),
	)
	msg, ok := cmd().(recentsTurnCompletedMsg)
	if !ok {
		t.Fatalf("expected recentsTurnCompletedMsg")
	}
	if msg.err != nil {
		t.Fatalf("expected graceful completion fallback on stream close, got %v", msg.err)
	}
}

func TestWatchRecentsTurnCompletionCmdNilPolicyUsesDefault(t *testing.T) {
	stream := make(chan transcriptdomain.TranscriptEvent, 1)
	stream <- transcriptdomain.TranscriptEvent{
		Kind: transcriptdomain.TranscriptEventTurnCompleted,
		Turn: &transcriptdomain.TurnState{TurnID: "turn-default"},
	}
	close(stream)
	cmd := watchRecentsTurnCompletionCmdWithContext(
		transcriptStreamAPIStub{stream: stream},
		nil,
		"s1",
		"turn-expected",
		context.Background(),
	)
	msg, ok := cmd().(recentsTurnCompletedMsg)
	if !ok {
		t.Fatalf("expected recentsTurnCompletedMsg")
	}
	if msg.err != nil {
		t.Fatalf("expected no error, got %v", msg.err)
	}
	if msg.turnID != "turn-default" {
		t.Fatalf("expected default policy turn id, got %q", msg.turnID)
	}
}

type transcriptStreamAPIStub struct {
	stream <-chan transcriptdomain.TranscriptEvent
	err    error
}

func (s transcriptStreamAPIStub) TranscriptStream(context.Context, string, string) (<-chan transcriptdomain.TranscriptEvent, func(), error) {
	if s.err != nil {
		return nil, func() {}, s.err
	}
	return s.stream, func() {}, nil
}

type staticRecentsCompletionSignalPolicy struct {
	matchKind transcriptdomain.TranscriptEventKind
	turnID    string
}

func (s staticRecentsCompletionSignalPolicy) CompletionFromTranscriptEvent(event transcriptdomain.TranscriptEvent) (string, bool) {
	if event.Kind == s.matchKind {
		return s.turnID, true
	}
	return "", false
}
