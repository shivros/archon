package app

import (
	"testing"

	"control/internal/types"
)

func TestComposeFileSearchStreamControllerLifecycle(t *testing.T) {
	controller := NewComposeFileSearchStreamController(2)
	if controller == nil {
		t.Fatalf("expected stream controller")
	}
	if controller.HasStream() {
		t.Fatalf("did not expect stream before initialization")
	}
	if got := controller.SearchID(); got != "" {
		t.Fatalf("expected blank search id before initialization, got %q", got)
	}

	cancelCalls := 0
	ch := make(chan types.FileSearchEvent, 4)
	controller.SetStream("fs-1", ch, func() { cancelCalls++ })
	if !controller.HasStream() {
		t.Fatalf("expected stream after SetStream")
	}
	if got := controller.SearchID(); got != "fs-1" {
		t.Fatalf("expected active search id fs-1, got %q", got)
	}

	next := make(chan types.FileSearchEvent, 4)
	controller.SetStream("fs-2", next, func() { cancelCalls++ })
	if cancelCalls != 1 {
		t.Fatalf("expected replacing stream to cancel prior subscription once, got %d", cancelCalls)
	}
	if got := controller.SearchID(); got != "fs-2" {
		t.Fatalf("expected active search id fs-2 after replacement, got %q", got)
	}

	controller.Reset()
	if cancelCalls != 2 {
		t.Fatalf("expected reset to cancel active subscription once, got %d", cancelCalls)
	}
	if controller.HasStream() {
		t.Fatalf("expected reset to clear stream")
	}
}

func TestComposeFileSearchStreamControllerConsumeTickBranches(t *testing.T) {
	t.Run("no stream returns no changes", func(t *testing.T) {
		controller := NewComposeFileSearchStreamController(2)
		events, changed, closed := controller.ConsumeTick()
		if len(events) != 0 || changed || closed {
			t.Fatalf("expected empty consume result, got events=%#v changed=%v closed=%v", events, changed, closed)
		}
	})

	t.Run("consumes up to max events per tick", func(t *testing.T) {
		controller := NewComposeFileSearchStreamController(2)
		ch := make(chan types.FileSearchEvent, 4)
		ch <- types.FileSearchEvent{Kind: types.FileSearchEventResults, SearchID: "fs-1", Query: "a"}
		ch <- types.FileSearchEvent{Kind: types.FileSearchEventResults, SearchID: "fs-1", Query: "b"}
		ch <- types.FileSearchEvent{Kind: types.FileSearchEventResults, SearchID: "fs-1", Query: "c"}
		controller.SetStream("fs-1", ch, func() {})

		events, changed, closed := controller.ConsumeTick()
		if !changed || closed || len(events) != 2 {
			t.Fatalf("expected two consumed events without closure, got events=%#v changed=%v closed=%v", events, changed, closed)
		}

		events, changed, closed = controller.ConsumeTick()
		if !changed || closed || len(events) != 1 || events[0].Query != "c" {
			t.Fatalf("expected remaining event on second tick, got events=%#v changed=%v closed=%v", events, changed, closed)
		}
	})

	t.Run("closed stream reports closed and clears state", func(t *testing.T) {
		controller := NewComposeFileSearchStreamController(4)
		ch := make(chan types.FileSearchEvent, 2)
		ch <- types.FileSearchEvent{Kind: types.FileSearchEventResults, SearchID: "fs-1", Query: "a"}
		close(ch)
		controller.SetStream("fs-1", ch, func() {})

		events, changed, closed := controller.ConsumeTick()
		if !changed || !closed || len(events) != 1 {
			t.Fatalf("expected consume to return buffered event and closed=true, got events=%#v changed=%v closed=%v", events, changed, closed)
		}
		if controller.HasStream() {
			t.Fatalf("expected closed stream to clear active subscription")
		}
		if got := controller.SearchID(); got != "" {
			t.Fatalf("expected closed stream to clear search id, got %q", got)
		}
	})
}
