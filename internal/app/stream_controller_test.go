package app

import (
	"testing"

	"control/internal/types"
)

func TestStreamControllerConsumeTickAccumulatesAndCloses(t *testing.T) {
	stream := NewStreamController(10, 10)
	ch := make(chan types.LogEvent, 4)
	stream.SetStream(ch, nil)

	ch <- types.LogEvent{Chunk: "hello"}
	ch <- types.LogEvent{Chunk: " world\nnext"}

	lines, changed, closed := stream.ConsumeTick()
	if closed {
		t.Fatalf("expected open stream")
	}
	if !changed {
		t.Fatalf("expected changed=true on chunk consumption")
	}
	if len(lines) != 1 || lines[0] != "hello world" {
		t.Fatalf("unexpected lines after first tick: %#v", lines)
	}

	ch <- types.LogEvent{Chunk: "\npartial"}
	lines, changed, closed = stream.ConsumeTick()
	if closed {
		t.Fatalf("expected stream to still be open")
	}
	if !changed {
		t.Fatalf("expected changed=true after second chunk")
	}
	if len(lines) != 2 || lines[1] != "next" {
		t.Fatalf("unexpected lines after second tick: %#v", lines)
	}

	close(ch)
	_, changed, closed = stream.ConsumeTick()
	if !closed {
		t.Fatalf("expected closed=true when channel is closed")
	}
	if changed {
		t.Fatalf("expected changed=false on close without new chunks")
	}
}

func TestStreamControllerSetSnapshotTrimsLines(t *testing.T) {
	stream := NewStreamController(2, 10)
	stream.SetSnapshot([]string{"first", "second", "third"})
	lines := stream.Lines()
	if len(lines) != 2 || lines[0] != "second" || lines[1] != "third" {
		t.Fatalf("unexpected snapshot lines: %#v", lines)
	}
}
