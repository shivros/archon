package daemon

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"control/internal/logging"
	"control/internal/types"
)

func TestSessionManagerSubscribeDebugAndSnapshot(t *testing.T) {
	manager, err := NewSessionManager(t.TempDir())
	if err != nil {
		t.Fatalf("NewSessionManager: %v", err)
	}
	manager.sessions["s1"] = &sessionRuntime{
		debugHub: newDebugHub(),
		debugBuf: newDebugBuffer(8),
	}
	manager.sessions["s1"].debugBuf.Append(types.DebugEvent{Seq: 1, Chunk: "first"})

	snap, err := manager.DebugSnapshot("s1", 10)
	if err != nil {
		t.Fatalf("DebugSnapshot: %v", err)
	}
	if len(snap) != 1 || snap[0].Chunk != "first" {
		t.Fatalf("unexpected snapshot: %#v", snap)
	}

	ch, cancel, err := manager.SubscribeDebug("s1")
	if err != nil {
		t.Fatalf("SubscribeDebug: %v", err)
	}
	manager.sessions["s1"].debugHub.Broadcast(types.DebugEvent{Seq: 2, Chunk: "live"})
	got := <-ch
	if got.Chunk != "live" {
		t.Fatalf("unexpected debug event: %+v", got)
	}
	cancel()
}

func TestSessionManagerSubscribeDebugNotFound(t *testing.T) {
	manager, err := NewSessionManager(t.TempDir())
	if err != nil {
		t.Fatalf("NewSessionManager: %v", err)
	}
	if _, _, err := manager.SubscribeDebug("missing"); err == nil {
		t.Fatalf("expected not found error")
	}
	if _, err := manager.DebugSnapshot("missing", 10); err == nil {
		t.Fatalf("expected not found snapshot error")
	}
}

func TestSessionServiceReadAndSubscribeDebug(t *testing.T) {
	baseDir := t.TempDir()
	manager, err := NewSessionManager(baseDir)
	if err != nil {
		t.Fatalf("NewSessionManager: %v", err)
	}
	manager.sessions["s1"] = &sessionRuntime{
		debugHub: newDebugHub(),
		debugBuf: newDebugBuffer(8),
	}
	service := NewSessionService(manager, nil, logging.Nop())

	sessionDir := filepath.Join(baseDir, "s1")
	if err := os.MkdirAll(sessionDir, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	f, err := os.OpenFile(filepath.Join(sessionDir, "debug.jsonl"), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		t.Fatalf("OpenFile: %v", err)
	}
	_ = json.NewEncoder(f).Encode(types.DebugEvent{SessionID: "s1", Stream: "stdout", Chunk: "line-1"})
	_, _ = f.WriteString("{invalid}\n")
	_ = f.Close()

	events, truncated, err := service.ReadDebug(context.Background(), "s1", 0)
	if err != nil {
		t.Fatalf("ReadDebug: %v", err)
	}
	if truncated {
		t.Fatalf("did not expect truncation")
	}
	if len(events) != 1 || events[0].Chunk != "line-1" {
		t.Fatalf("unexpected events: %#v", events)
	}

	ch, cancel, err := service.SubscribeDebug(context.Background(), "s1")
	if err != nil {
		t.Fatalf("SubscribeDebug: %v", err)
	}
	manager.sessions["s1"].debugHub.Broadcast(types.DebugEvent{Chunk: "line-2"})
	got := <-ch
	if got.Chunk != "line-2" {
		t.Fatalf("unexpected subscribed event: %+v", got)
	}
	cancel()
}

func TestSessionServiceDebugValidationAndManagerUnavailable(t *testing.T) {
	service := NewSessionService(nil, nil, logging.Nop())
	if _, _, err := service.ReadDebug(context.Background(), "", 10); err == nil {
		t.Fatalf("expected validation error")
	}
	if _, _, err := service.SubscribeDebug(context.Background(), "s1"); err == nil {
		t.Fatalf("expected unavailable error")
	}
}
