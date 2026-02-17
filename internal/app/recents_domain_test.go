package app

import (
	"testing"
	"time"

	"control/internal/types"
)

func TestRecentsTrackerTransitionsRunningToReadyFIFO(t *testing.T) {
	tracker := NewRecentsTracker()
	now := time.Now().UTC()

	tracker.StartRun("s1", "turn-u1", now.Add(-2*time.Minute))
	tracker.StartRun("s2", "turn-u2", now.Add(-1*time.Minute))

	meta := map[string]*types.SessionMeta{
		"s1": {SessionID: "s1", LastTurnID: "turn-a1"},
		"s2": {SessionID: "s2", LastTurnID: "turn-a2"},
	}
	ready := tracker.ObserveMeta(meta, now)
	if len(ready) != 2 {
		t.Fatalf("expected 2 ready items, got %d", len(ready))
	}
	if got := tracker.ReadyIDs(); len(got) != 2 || got[0] != "s1" || got[1] != "s2" {
		t.Fatalf("expected FIFO ready order [s1 s2], got %#v", got)
	}
	if got := tracker.RunningIDs(); len(got) != 0 {
		t.Fatalf("expected running to be empty after completion, got %#v", got)
	}
}

func TestRecentsTrackerDismissLifecycle(t *testing.T) {
	tracker := NewRecentsTracker()
	now := time.Now().UTC()

	tracker.StartRun("s1", "turn-u1", now.Add(-time.Minute))
	tracker.ObserveMeta(map[string]*types.SessionMeta{
		"s1": {SessionID: "s1", LastTurnID: "turn-a1"},
	}, now)
	if !tracker.IsReady("s1") {
		t.Fatalf("expected s1 to be ready")
	}
	if !tracker.DismissReady("s1") {
		t.Fatalf("expected dismiss to succeed")
	}
	if tracker.IsReady("s1") {
		t.Fatalf("expected s1 to be removed from ready after dismiss")
	}

	tracker.StartRun("s1", "turn-a1", now.Add(time.Minute))
	tracker.ObserveMeta(map[string]*types.SessionMeta{
		"s1": {SessionID: "s1", LastTurnID: "turn-a2"},
	}, now.Add(2*time.Minute))

	if !tracker.IsReady("s1") {
		t.Fatalf("expected s1 to return to ready on new completion cycle")
	}
}
