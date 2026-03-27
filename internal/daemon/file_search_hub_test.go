package daemon

import (
	"context"
	"testing"
	"time"

	"control/internal/types"
)

func TestMemoryFileSearchHubPublishesAndClosesTerminalSubscribers(t *testing.T) {
	hub := NewMemoryFileSearchHub()
	session := &types.FileSearchSession{
		ID:       "fs-1",
		Provider: "codex",
		Status:   types.FileSearchStatusActive,
	}
	if err := hub.Register("fs-1", session, &stubFileSearchRuntime{}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	ch, stop, err := hub.Subscribe(ctx, "fs-1")
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	defer stop()

	updateEvent := types.FileSearchEvent{Kind: types.FileSearchEventUpdated, SearchID: "fs-1", Status: types.FileSearchStatusActive}
	if err := hub.Publish("fs-1", session, updateEvent, false); err != nil {
		t.Fatalf("Publish update: %v", err)
	}
	select {
	case got := <-ch:
		if got.Kind != types.FileSearchEventUpdated {
			t.Fatalf("unexpected update event: %#v", got)
		}
	case <-time.After(time.Second):
		t.Fatalf("timeout waiting for update event")
	}

	closedSession := cloneFileSearchSession(session)
	now := time.Now().UTC()
	closedSession.Status = types.FileSearchStatusClosed
	closedSession.ClosedAt = &now
	closeEvent := types.FileSearchEvent{Kind: types.FileSearchEventClosed, SearchID: "fs-1", Status: types.FileSearchStatusClosed}
	if err := hub.Publish("fs-1", closedSession, closeEvent, true); err != nil {
		t.Fatalf("Publish close: %v", err)
	}
	select {
	case got, ok := <-ch:
		if !ok {
			t.Fatalf("expected close event before channel closes")
		}
		if got.Kind != types.FileSearchEventClosed {
			t.Fatalf("unexpected close event: %#v", got)
		}
	case <-time.After(time.Second):
		t.Fatalf("timeout waiting for close event")
	}
	select {
	case _, ok := <-ch:
		if ok {
			t.Fatalf("expected channel to be closed after terminal publish")
		}
	case <-time.After(time.Second):
		t.Fatalf("timeout waiting for channel closure")
	}
}

func TestMemoryFileSearchHubLookupReturnsNotFoundAfterTerminalPublish(t *testing.T) {
	hub := NewMemoryFileSearchHub()
	session := &types.FileSearchSession{ID: "fs-1", Provider: "codex", Status: types.FileSearchStatusActive}
	if err := hub.Register("fs-1", session, &stubFileSearchRuntime{}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := hub.Publish("fs-1", session, types.FileSearchEvent{
		Kind:     types.FileSearchEventFailed,
		SearchID: "fs-1",
		Status:   types.FileSearchStatusFailed,
	}, true); err != nil {
		t.Fatalf("Publish terminal: %v", err)
	}
	if _, _, err := hub.Lookup("fs-1"); !isFileSearchHubNotFound(err) {
		t.Fatalf("expected not found after terminal publish, got %v", err)
	}
}

func TestMemoryFileSearchHubSubscribeReturnsNotFoundForMissingSearch(t *testing.T) {
	hub := NewMemoryFileSearchHub()
	if _, _, err := hub.Subscribe(context.Background(), "missing"); !isFileSearchHubNotFound(err) {
		t.Fatalf("expected not found, got %v", err)
	}
}

func TestMemoryFileSearchHubContextCancelUnsubscribes(t *testing.T) {
	hub := NewMemoryFileSearchHub()
	session := &types.FileSearchSession{ID: "fs-1", Provider: "codex", Status: types.FileSearchStatusActive}
	if err := hub.Register("fs-1", session, &stubFileSearchRuntime{}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	ch, stop, err := hub.Subscribe(ctx, "fs-1")
	if err != nil {
		t.Fatalf("Subscribe: %v", err)
	}
	cancel()
	select {
	case _, ok := <-ch:
		if ok {
			t.Fatalf("expected subscription channel to close on context cancel")
		}
	case <-time.After(time.Second):
		t.Fatalf("timeout waiting for cancellation close")
	}
	stop()
}
