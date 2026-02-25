package app

import (
	"testing"

	"control/internal/types"
)

func TestDebugStreamControllerConsumeTickAccumulatesAndCloses(t *testing.T) {
	controller := NewDebugStreamController(2, 10)
	ch := make(chan types.DebugEvent, 4)
	controller.SetStream(ch, nil)

	ch <- types.DebugEvent{Chunk: "first\nsecond\n"}
	lines, changed, closed := controller.ConsumeTick()
	if closed {
		t.Fatalf("expected open stream")
	}
	if !changed {
		t.Fatalf("expected changed=true on chunk consumption")
	}
	if len(lines) != 2 || lines[0] != "first" || lines[1] != "second" {
		t.Fatalf("unexpected lines after first tick: %#v", lines)
	}

	ch <- types.DebugEvent{Chunk: "third\n"}
	lines, changed, closed = controller.ConsumeTick()
	if closed {
		t.Fatalf("expected stream to still be open")
	}
	if !changed {
		t.Fatalf("expected changed=true after second chunk")
	}
	if len(lines) != 2 || lines[0] != "second" || lines[1] != "third" {
		t.Fatalf("expected max-lines trimming, got %#v", lines)
	}

	close(ch)
	_, changed, closed = controller.ConsumeTick()
	if !closed {
		t.Fatalf("expected closed=true when channel is closed")
	}
	if changed {
		t.Fatalf("expected changed=false on close without new chunks")
	}
	if controller.HasStream() {
		t.Fatalf("expected stream to be detached when closed")
	}
}

func TestDebugStreamControllerResetCancelsAndClearsState(t *testing.T) {
	controller := NewDebugStreamController(10, 10)
	called := 0
	controller.lines = []string{"line"}
	controller.pending = "partial"
	controller.SetStream(make(chan types.DebugEvent), func() {
		called++
	})

	controller.Reset()

	if called != 1 {
		t.Fatalf("expected cancel function to be called once, got %d", called)
	}
	if controller.HasStream() {
		t.Fatalf("expected stream to be cleared")
	}
	if len(controller.Lines()) != 0 {
		t.Fatalf("expected lines to be cleared, got %#v", controller.Lines())
	}
	if controller.pending != "" {
		t.Fatalf("expected pending buffer to be cleared, got %q", controller.pending)
	}
}
