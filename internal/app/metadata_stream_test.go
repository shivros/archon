package app

import (
	"testing"

	"control/internal/types"
)

func TestMetadataStreamControllerSetHasResetLifecycle(t *testing.T) {
	controller := NewMetadataStreamController(4)
	if controller.HasStream() {
		t.Fatalf("expected empty stream controller")
	}
	canceled := false
	ch := make(chan types.MetadataEvent)
	controller.SetStream(ch, func() { canceled = true })
	if !controller.HasStream() {
		t.Fatalf("expected stream to be set")
	}
	controller.Reset()
	if !canceled {
		t.Fatalf("expected reset to call cancel callback")
	}
	if controller.HasStream() {
		t.Fatalf("expected stream to be cleared after reset")
	}
}

func TestMetadataStreamControllerConsumeTickClosedChannel(t *testing.T) {
	controller := NewMetadataStreamController(4)
	ch := make(chan types.MetadataEvent)
	close(ch)
	controller.SetStream(ch, nil)
	events, changed, closed := controller.ConsumeTick()
	if len(events) != 0 || changed {
		t.Fatalf("expected no events from closed channel, got %#v changed=%v", events, changed)
	}
	if !closed {
		t.Fatalf("expected closed=true when channel is closed")
	}
}

func TestMetadataStreamControllerConsumeTickNoEvents(t *testing.T) {
	controller := NewMetadataStreamController(4)
	ch := make(chan types.MetadataEvent, 1)
	controller.SetStream(ch, nil)
	events, changed, closed := controller.ConsumeTick()
	if len(events) != 0 || changed || closed {
		t.Fatalf("expected no-op consume tick, got events=%d changed=%v closed=%v", len(events), changed, closed)
	}
}
