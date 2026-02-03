package daemon

import (
	"testing"

	"control/internal/types"
)

func TestLogBufferAppendDrain(t *testing.T) {
	buf := newLogBuffer(10)
	buf.Append([]byte("hello"))
	buf.Append([]byte("world"))

	if got := buf.Len(); got != 10 {
		t.Fatalf("expected len 10, got %d", got)
	}

	chunk := buf.Drain(4)
	if string(chunk) != "hell" {
		t.Fatalf("expected 'hell', got %q", string(chunk))
	}
	if got := buf.Len(); got != 6 {
		t.Fatalf("expected len 6, got %d", got)
	}

	buf.Append([]byte("0123456789"))
	if got := buf.Len(); got != 10 {
		t.Fatalf("expected len 10 after overflow, got %d", got)
	}
	chunk = buf.Drain(10)
	if string(chunk) != "0123456789" {
		t.Fatalf("expected last 10 bytes, got %q", string(chunk))
	}
}

func TestSubscriberHubBroadcast(t *testing.T) {
	hub := newSubscriberHub()
	stdoutCh, cancelStdout, err := hub.Add("stdout")
	if err != nil {
		t.Fatalf("add stdout: %v", err)
	}
	combinedCh, cancelCombined, err := hub.Add("combined")
	if err != nil {
		t.Fatalf("add combined: %v", err)
	}
	defer cancelStdout()
	defer cancelCombined()

	hub.Broadcast(types.LogEvent{Type: "log", Stream: "stdout", Chunk: "out"})

	select {
	case event := <-stdoutCh:
		if event.Stream != "stdout" || event.Chunk != "out" {
			t.Fatalf("unexpected stdout event: %+v", event)
		}
	default:
		t.Fatalf("expected stdout event")
	}

	select {
	case event := <-combinedCh:
		if event.Stream != "stdout" || event.Chunk != "out" {
			t.Fatalf("unexpected combined event: %+v", event)
		}
	default:
		t.Fatalf("expected combined event")
	}

	hub.Broadcast(types.LogEvent{Type: "log", Stream: "stderr", Chunk: "err"})

	select {
	case <-stdoutCh:
		t.Fatalf("stdout subscriber should not receive stderr")
	default:
	}
}

func TestSubscriberHubCancel(t *testing.T) {
	hub := newSubscriberHub()
	ch, cancel, err := hub.Add("combined")
	if err != nil {
		t.Fatalf("add combined: %v", err)
	}
	cancel()
	_, ok := <-ch
	if ok {
		t.Fatalf("expected channel to be closed")
	}
}
