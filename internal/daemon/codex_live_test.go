package daemon

import (
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"control/internal/types"
)

func TestIsCodexMissingThreadError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "nil",
			err:  nil,
			want: false,
		},
		{
			name: "rollout missing",
			err:  errors.New("RPC error -32600: No rollout found for thread ID thr_123"),
			want: true,
		},
		{
			name: "thread missing",
			err:  errors.New("thread not found"),
			want: true,
		},
		{
			name: "other error",
			err:  errors.New("permission denied"),
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isCodexMissingThreadError(tt.err); got != tt.want {
				t.Fatalf("isCodexMissingThreadError(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestReserveSessionTurnRejectsConcurrentStart(t *testing.T) {
	ls := &codexLiveSession{client: &codexAppServer{}}
	started := make(chan struct{})
	release := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(2)

	firstTurnID := ""
	firstErr := error(nil)
	go func() {
		defer wg.Done()
		firstTurnID, firstErr = reserveSessionTurn(ls, func() (string, error) {
			close(started)
			<-release
			return "turn-1", nil
		})
	}()

	<-started
	secondTurnID := ""
	secondErr := error(nil)
	go func() {
		defer wg.Done()
		secondTurnID, secondErr = reserveSessionTurn(ls, func() (string, error) {
			return "turn-2", nil
		})
	}()

	close(release)
	wg.Wait()

	if firstErr != nil || firstTurnID != "turn-1" {
		t.Fatalf("expected first start to succeed, got turn=%q err=%v", firstTurnID, firstErr)
	}
	if secondErr == nil || secondErr.Error() != "turn already in progress" {
		t.Fatalf("expected second start to fail with turn already in progress, got turn=%q err=%v", secondTurnID, secondErr)
	}
	if ls.activeTurn != "turn-1" {
		t.Fatalf("expected active turn to remain turn-1, got %q", ls.activeTurn)
	}
	if ls.lastActive.IsZero() || time.Since(ls.lastActive) > time.Minute {
		t.Fatalf("expected recent lastActive timestamp, got %v", ls.lastActive)
	}
}

func TestReserveSessionTurnClearsNothingOnStartError(t *testing.T) {
	ls := &codexLiveSession{client: &codexAppServer{}}
	_, err := reserveSessionTurn(ls, func() (string, error) {
		return "", errors.New("start failed")
	})
	if err == nil || err.Error() != "start failed" {
		t.Fatalf("expected start error to propagate, got %v", err)
	}
	if ls.activeTurn != "" {
		t.Fatalf("expected active turn to stay empty on start failure, got %q", ls.activeTurn)
	}
}

func TestCodexLiveSessionHandleNoteClearsActiveTurnBeforePublishingCompletion(t *testing.T) {
	ls := &codexLiveSession{
		sessionID:  "sess-1",
		client:     &codexAppServer{},
		hub:        newCodexSubscriberHub(),
		activeTurn: "turn-1",
	}
	probe := &activeTurnProbeNotifier{session: ls}
	ls.notifier = probe

	ls.handleNote(rpcMessage{
		Method: "turn/completed",
		Params: json.RawMessage(`{"turn":{"id":"turn-1"}}`),
	})

	if probe.activeTurnAtPublish != "" {
		t.Fatalf("expected active turn to be cleared before publish, got %q", probe.activeTurnAtPublish)
	}
	if ls.activeTurn != "" {
		t.Fatalf("expected active turn to stay cleared after completion, got %q", ls.activeTurn)
	}
}

func TestCodexLiveSessionHandleRequestPublishesApprovalNotification(t *testing.T) {
	notifier := &captureCodexNotificationPublisher{}
	ls := &codexLiveSession{
		sessionID: "sess-approval",
		client:    &codexAppServer{},
		hub:       newCodexSubscriberHub(),
		notifier:  notifier,
	}
	requestID := 42
	ls.handleRequest(rpcMessage{
		ID:     &requestID,
		Method: "item/commandExecution/requestApproval",
		Params: json.RawMessage(`{"command":"touch file.txt"}`),
	})

	if len(notifier.events) != 1 {
		t.Fatalf("expected one approval notification, got %d", len(notifier.events))
	}
	event := notifier.events[0]
	if event.Trigger != types.NotificationTriggerTurnCompleted {
		t.Fatalf("unexpected trigger: %q", event.Trigger)
	}
	if event.Status != "approval_required" {
		t.Fatalf("unexpected status: %q", event.Status)
	}
	if event.SessionID != "sess-approval" {
		t.Fatalf("unexpected session id: %q", event.SessionID)
	}
	if event.Source != "approval_request:sess-approval:42" {
		t.Fatalf("unexpected source: %q", event.Source)
	}
}

type activeTurnProbeNotifier struct {
	session             *codexLiveSession
	activeTurnAtPublish string
}

func (n *activeTurnProbeNotifier) Publish(_ types.NotificationEvent) {
	if n == nil || n.session == nil {
		return
	}
	n.session.mu.Lock()
	n.activeTurnAtPublish = n.session.activeTurn
	n.session.mu.Unlock()
}

type captureCodexNotificationPublisher struct {
	events []types.NotificationEvent
}

func (p *captureCodexNotificationPublisher) Publish(event types.NotificationEvent) {
	p.events = append(p.events, event)
}
