package daemon

import (
	"testing"

	"control/internal/types"
)

func TestDebugHubAddBroadcastAndCancel(t *testing.T) {
	hub := newDebugHub()
	ch, cancel := hub.Add()
	if ch == nil {
		t.Fatalf("expected subscriber channel")
	}
	event := types.DebugEvent{Type: "debug", SessionID: "s1", Chunk: "hello"}
	hub.Broadcast(event)

	got := <-ch
	if got.Chunk != "hello" || got.SessionID != "s1" {
		t.Fatalf("unexpected event: %+v", got)
	}
	cancel()
	if _, ok := <-ch; ok {
		t.Fatalf("expected closed channel after cancel")
	}
}

func TestDebugBufferSnapshotAndRollover(t *testing.T) {
	buf := newDebugBuffer(2)
	buf.Append(types.DebugEvent{Seq: 1, Chunk: "a"})
	buf.Append(types.DebugEvent{Seq: 2, Chunk: "b"})
	buf.Append(types.DebugEvent{Seq: 3, Chunk: "c"})

	all := buf.Snapshot(0)
	if len(all) != 2 || all[0].Seq != 2 || all[1].Seq != 3 {
		t.Fatalf("expected rollover to retain last 2 events, got %#v", all)
	}

	lastOne := buf.Snapshot(1)
	if len(lastOne) != 1 || lastOne[0].Seq != 3 {
		t.Fatalf("expected one trailing event, got %#v", lastOne)
	}
}
