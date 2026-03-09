package daemon

import (
	"context"
	"errors"
	"testing"

	"control/internal/types"
)

func TestTranscriptTransportSelectorPrefersEventsOverItems(t *testing.T) {
	eventsCalls := 0
	itemsCalls := 0
	selector := NewDefaultTranscriptTransportSelector(
		func(context.Context, string) (<-chan types.CodexEvent, func(), error) {
			eventsCalls++
			ch := make(chan types.CodexEvent)
			return ch, func() { close(ch) }, nil
		},
		func(context.Context, string) (<-chan map[string]any, func(), error) {
			itemsCalls++
			ch := make(chan map[string]any)
			return ch, func() { close(ch) }, nil
		},
	)
	selection, err := selector.Select(context.Background(), "s1", "opencode")
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	if eventsCalls != 1 {
		t.Fatalf("expected events subscribe once, got %d", eventsCalls)
	}
	if itemsCalls != 0 {
		t.Fatalf("expected no items subscribe when events available, got %d", itemsCalls)
	}
	if !selection.followAvailable {
		t.Fatalf("expected follow to be available")
	}
	if selection.transport.eventsCh == nil {
		t.Fatalf("expected events channel")
	}
}

func TestTranscriptTransportSelectorFallsBackToItemsWhenEventsFail(t *testing.T) {
	selector := NewDefaultTranscriptTransportSelector(
		func(context.Context, string) (<-chan types.CodexEvent, func(), error) {
			return nil, nil, errors.New("events down")
		},
		func(context.Context, string) (<-chan map[string]any, func(), error) {
			ch := make(chan map[string]any)
			return ch, func() { close(ch) }, nil
		},
	)
	selection, err := selector.Select(context.Background(), "s1", "opencode")
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	if !selection.followAvailable {
		t.Fatalf("expected follow to be available")
	}
	if selection.transport.itemsCh == nil {
		t.Fatalf("expected items channel fallback")
	}
}

func TestTranscriptTransportSelectorReturnsErrorWhenUnsupported(t *testing.T) {
	selector := NewDefaultTranscriptTransportSelector(nil, nil)
	if _, err := selector.Select(context.Background(), "s1", "custom"); err == nil {
		t.Fatalf("expected unsupported transport error")
	}
}

func TestTranscriptTransportSelectorReturnsErrorWhenEventsOnlyProviderFails(t *testing.T) {
	selector := NewDefaultTranscriptTransportSelector(
		func(context.Context, string) (<-chan types.CodexEvent, func(), error) {
			return nil, nil, errors.New("events unavailable")
		},
		nil,
	)
	if _, err := selector.Select(context.Background(), "s1", "codex"); err == nil {
		t.Fatalf("expected events-only provider failure")
	}
}

func TestTranscriptTransportSelectorReturnsErrorWhenItemsOnlyProviderFails(t *testing.T) {
	selector := NewDefaultTranscriptTransportSelector(
		nil,
		func(context.Context, string) (<-chan map[string]any, func(), error) {
			return nil, nil, errors.New("items unavailable")
		},
	)
	if _, err := selector.Select(context.Background(), "s1", "claude"); err == nil {
		t.Fatalf("expected items-only provider failure")
	}
}

func TestTranscriptTransportSelectorMarksFollowUnavailableWhenItemsSubscriberCannotAttach(t *testing.T) {
	selector := NewDefaultTranscriptTransportSelector(
		nil,
		func(context.Context, string) (<-chan map[string]any, func(), error) {
			return nil, nil, unavailableError("session is not live", ErrTranscriptFollowUnavailable)
		},
	)
	selection, err := selector.Select(context.Background(), "s1", "claude")
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	if selection.followAvailable {
		t.Fatalf("expected follow to be unavailable")
	}
	if selection.transport.eventsCh != nil || selection.transport.itemsCh != nil {
		t.Fatalf("expected no transport channels when follow unavailable")
	}
}

func TestSelectorTranscriptIngressFactoryOpen(t *testing.T) {
	events := make(chan types.CodexEvent)
	selector := fixedTranscriptTransportSelector{
		transport: transcriptTransport{eventsCh: events},
	}
	factory := NewSelectorTranscriptIngressFactory(selector)
	if factory == nil {
		t.Fatalf("expected ingress factory")
	}
	handle, err := factory.Open(context.Background(), "s1", "codex")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if !handle.FollowAvailable {
		t.Fatalf("expected follow available")
	}
	if handle.Events == nil {
		t.Fatalf("expected events channel")
	}
}

func TestSelectorTranscriptIngressFactoryFailsFastWhenSelectorUnset(t *testing.T) {
	factory := NewSelectorTranscriptIngressFactory(nil)
	if factory == nil {
		t.Fatalf("expected non-nil ingress factory")
	}
	_, err := factory.Open(context.Background(), "s1", "codex")
	if err == nil {
		t.Fatalf("expected unavailable error when selector is unset")
	}
	svcErr, ok := err.(*ServiceError)
	if !ok {
		t.Fatalf("expected service error, got %T", err)
	}
	if svcErr.Kind != ServiceErrorUnavailable {
		t.Fatalf("expected unavailable error kind, got %q", svcErr.Kind)
	}
}

func TestSelectorTranscriptIngressFactoryOpenCloseInvokesTransportCancels(t *testing.T) {
	events := make(chan types.CodexEvent)
	items := make(chan map[string]any)
	eventsCanceled := 0
	itemsCanceled := 0
	selector := fixedTranscriptTransportSelector{
		transport: transcriptTransport{
			eventsCh: events,
			eventsCancel: func() {
				eventsCanceled++
			},
			itemsCh: items,
			itemsCancel: func() {
				itemsCanceled++
			},
		},
		followAvailable: true,
	}
	factory := NewSelectorTranscriptIngressFactory(selector)
	handle, err := factory.Open(context.Background(), "s1", "opencode")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if handle.Close == nil {
		t.Fatalf("expected close function")
	}
	handle.Close()
	if eventsCanceled != 1 {
		t.Fatalf("expected events cancel once, got %d", eventsCanceled)
	}
	if itemsCanceled != 1 {
		t.Fatalf("expected items cancel once, got %d", itemsCanceled)
	}
}
