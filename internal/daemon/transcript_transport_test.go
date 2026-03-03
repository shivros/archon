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
	transport, err := selector.Select(context.Background(), "s1", "opencode")
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	if eventsCalls != 1 {
		t.Fatalf("expected events subscribe once, got %d", eventsCalls)
	}
	if itemsCalls != 0 {
		t.Fatalf("expected no items subscribe when events available, got %d", itemsCalls)
	}
	if transport.eventsCh == nil {
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
	transport, err := selector.Select(context.Background(), "s1", "opencode")
	if err != nil {
		t.Fatalf("Select: %v", err)
	}
	if transport.itemsCh == nil {
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
