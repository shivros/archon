package daemon

import (
	"testing"
	"time"

	"control/internal/types"
)

func TestNewNotificationIntegrationServerWiresRecorder(t *testing.T) {
	t.Parallel()
	env := newNotificationIntegrationServer(t)
	defer env.Close()

	if env == nil || env.server == nil || env.manager == nil || env.stores == nil || env.live == nil || env.recorder == nil {
		t.Fatalf("expected non-nil server wiring components")
	}
	if got := listSessions(t, env.server); len(got.Sessions) != 0 {
		t.Fatalf("expected empty sessions list on new integration server")
	}

	env.manager.mu.Lock()
	notifier := env.manager.notifier
	env.manager.mu.Unlock()
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
	if _, ok := env.recorder.WaitForMatch(target, newProviderNotificationMatchPolicy(), time.Second); !ok {
		t.Fatalf("expected published event to reach integration recorder")
	}
}
