package daemon

import (
	"testing"
	"time"

	"control/internal/types"
)

func TestNewNotificationIntegrationServerWiresRecorder(t *testing.T) {
	t.Parallel()
	server, manager, stores, recorder := newNotificationIntegrationServer(t)
	defer server.Close()

	if server == nil || manager == nil || stores == nil || recorder == nil {
		t.Fatalf("expected non-nil server wiring components")
	}
	if got := listSessions(t, server); len(got.Sessions) != 0 {
		t.Fatalf("expected empty sessions list on new integration server")
	}

	manager.mu.Lock()
	notifier := manager.notifier
	manager.mu.Unlock()
	if notifier == nil {
		t.Fatalf("expected manager notifier to be wired")
	}

	target := NotificationMatchTarget{
		Trigger:   types.NotificationTriggerTurnCompleted,
		SessionID: "sess-smoke",
		Provider:  "codex",
		TurnID:    "turn-smoke",
	}
	notifier.Publish(types.NotificationEvent{
		Trigger:   types.NotificationTriggerTurnCompleted,
		SessionID: "sess-smoke",
		Provider:  "codex",
		TurnID:    "turn-smoke",
	})
	if _, ok := recorder.WaitForMatch(target, newProviderNotificationMatchPolicy(), time.Second); !ok {
		t.Fatalf("expected published event to reach integration recorder")
	}
}
