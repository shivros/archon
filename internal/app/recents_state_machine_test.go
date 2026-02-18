package app

import (
	"testing"
	"time"
)

func TestRecentsStateMachineDuplicateCompletionIsIdempotent(t *testing.T) {
	sm := NewRecentsStateMachine()
	now := time.Now().UTC()

	sm.Apply(RecentsEvent{
		Type:           RecentsEventRunStarted,
		SessionID:      "s1",
		BaselineTurnID: "turn-u1",
		At:             now,
	})
	first := sm.Apply(RecentsEvent{
		Type:             RecentsEventRunCompleted,
		SessionID:        "s1",
		ExpectedTurnID:   "turn-u1",
		CompletionTurnID: "turn-a1",
		At:               now.Add(time.Second),
	})
	if !first.Changed || !first.ReadyEnqueued {
		t.Fatalf("expected first completion to enqueue ready, got %#v", first)
	}

	second := sm.Apply(RecentsEvent{
		Type:             RecentsEventRunCompleted,
		SessionID:        "s1",
		ExpectedTurnID:   "turn-u1",
		CompletionTurnID: "turn-a1",
		At:               now.Add(2 * time.Second),
	})
	if second.Changed {
		t.Fatalf("expected duplicate completion to be idempotent, got %#v", second)
	}
	if got := sm.ReadyIDs(); len(got) != 1 || got[0] != "s1" {
		t.Fatalf("expected one ready item [s1], got %#v", got)
	}
}

func TestRecentsStateMachineDuplicateRunStartedIsIdempotent(t *testing.T) {
	sm := NewRecentsStateMachine()
	now := time.Now().UTC()

	first := sm.Apply(RecentsEvent{
		Type:           RecentsEventRunStarted,
		SessionID:      "s1",
		BaselineTurnID: "turn-u1",
		At:             now,
	})
	if !first.Changed {
		t.Fatalf("expected first run start to change state")
	}
	snap := sm.Snapshot()
	startedAt := snap.Running["s1"].StartedAt

	second := sm.Apply(RecentsEvent{
		Type:           RecentsEventRunStarted,
		SessionID:      "s1",
		BaselineTurnID: "turn-u1",
		At:             now.Add(time.Minute),
	})
	if second.Changed {
		t.Fatalf("expected duplicate run start to be idempotent, got %#v", second)
	}
	if got := sm.RunningIDs(); len(got) != 1 || got[0] != "s1" {
		t.Fatalf("expected one running id [s1], got %#v", got)
	}
	snap = sm.Snapshot()
	if !snap.Running["s1"].StartedAt.Equal(startedAt) {
		t.Fatalf("expected duplicate start to preserve StartedAt %v, got %v", startedAt, snap.Running["s1"].StartedAt)
	}
}

func TestRecentsStateMachineReadyFIFOIsDeterministic(t *testing.T) {
	sm := NewRecentsStateMachine()
	now := time.Now().UTC()

	// Same timestamp, deterministic fallback ordering should use SessionID.
	sm.Apply(RecentsEvent{Type: RecentsEventRunStarted, SessionID: "s2", BaselineTurnID: "u2", At: now})
	sm.Apply(RecentsEvent{Type: RecentsEventRunStarted, SessionID: "s1", BaselineTurnID: "u1", At: now})

	sm.Apply(RecentsEvent{Type: RecentsEventMetaObserved, SessionID: "s1", ObservedTurnID: "a1", At: now.Add(time.Second)})
	sm.Apply(RecentsEvent{Type: RecentsEventMetaObserved, SessionID: "s2", ObservedTurnID: "a2", At: now.Add(2 * time.Second)})

	if got := sm.ReadyIDs(); len(got) != 2 || got[0] != "s1" || got[1] != "s2" {
		t.Fatalf("expected deterministic FIFO [s1 s2], got %#v", got)
	}
}

func TestRecentsStateMachineDismissalLifecycleSemantics(t *testing.T) {
	sm := NewRecentsStateMachine()
	now := time.Now().UTC()

	sm.Apply(RecentsEvent{Type: RecentsEventRunStarted, SessionID: "s1", BaselineTurnID: "turn-u1", At: now})
	sm.Apply(RecentsEvent{Type: RecentsEventRunCompleted, SessionID: "s1", CompletionTurnID: "turn-a1", At: now.Add(time.Second)})
	dismiss := sm.Apply(RecentsEvent{Type: RecentsEventReadyDismiss, SessionID: "s1"})
	if !dismiss.Changed {
		t.Fatalf("expected dismiss to change state")
	}
	if sm.IsReady("s1") {
		t.Fatalf("expected dismissed item to leave ready")
	}

	// Duplicate completion for same cycle should not re-enqueue.
	dup := sm.Apply(RecentsEvent{Type: RecentsEventRunCompleted, SessionID: "s1", CompletionTurnID: "turn-a1", At: now.Add(2 * time.Second)})
	if dup.Changed {
		t.Fatalf("expected dismissed duplicate completion to be ignored, got %#v", dup)
	}

	// New run cycle clears dismissal and new completion is eligible.
	sm.Apply(RecentsEvent{Type: RecentsEventRunStarted, SessionID: "s1", BaselineTurnID: "turn-a1", At: now.Add(3 * time.Second)})
	next := sm.Apply(RecentsEvent{Type: RecentsEventRunCompleted, SessionID: "s1", CompletionTurnID: "turn-a2", At: now.Add(4 * time.Second)})
	if !next.ReadyEnqueued {
		t.Fatalf("expected new completion cycle to re-enter ready, got %#v", next)
	}
	if !sm.IsReady("s1") {
		t.Fatalf("expected s1 in ready after new cycle")
	}
}

func TestRecentsStateMachineStaleExpectedTurnIgnored(t *testing.T) {
	sm := NewRecentsStateMachine()
	now := time.Now().UTC()

	sm.Apply(RecentsEvent{Type: RecentsEventRunStarted, SessionID: "s1", BaselineTurnID: "turn-new", At: now})
	stale := sm.Apply(RecentsEvent{
		Type:             RecentsEventRunCompleted,
		SessionID:        "s1",
		ExpectedTurnID:   "turn-old",
		CompletionTurnID: "turn-old",
		At:               now.Add(time.Second),
	})
	if stale.Changed {
		t.Fatalf("expected stale completion to be ignored, got %#v", stale)
	}
	if !sm.IsRunning("s1") {
		t.Fatalf("expected running state to remain after stale completion")
	}
}

func TestRecentsStateMachineRunCanceledLifecycle(t *testing.T) {
	sm := NewRecentsStateMachine()
	now := time.Now().UTC()

	sm.Apply(RecentsEvent{Type: RecentsEventRunStarted, SessionID: "s1", BaselineTurnID: "turn-u1", At: now})
	cancel := sm.Apply(RecentsEvent{Type: RecentsEventRunCanceled, SessionID: "s1"})
	if !cancel.Changed {
		t.Fatalf("expected cancel to change state")
	}
	if sm.IsRunning("s1") {
		t.Fatalf("expected canceled run to leave running set")
	}
	if got := sm.ReadyIDs(); len(got) != 0 {
		t.Fatalf("expected no ready items after cancel, got %#v", got)
	}

	duplicateCancel := sm.Apply(RecentsEvent{Type: RecentsEventRunCanceled, SessionID: "s1"})
	if duplicateCancel.Changed {
		t.Fatalf("expected duplicate cancel to be idempotent, got %#v", duplicateCancel)
	}
}

func TestRecentsStateMachinePruneRemovesMissingSessions(t *testing.T) {
	sm := NewRecentsStateMachine()
	now := time.Now().UTC()

	sm.Apply(RecentsEvent{Type: RecentsEventRunStarted, SessionID: "s1", BaselineTurnID: "u1", At: now})
	sm.Apply(RecentsEvent{Type: RecentsEventRunStarted, SessionID: "s2", BaselineTurnID: "u2", At: now.Add(time.Second)})
	sm.Apply(RecentsEvent{Type: RecentsEventRunCompleted, SessionID: "s1", CompletionTurnID: "a1", At: now.Add(2 * time.Second)})
	sm.Apply(RecentsEvent{Type: RecentsEventReadyDismiss, SessionID: "s1"})

	pruned := sm.Apply(RecentsEvent{
		Type:              RecentsEventSessionsPrune,
		PresentSessionIDs: []string{"s2"},
	})
	if !pruned.Changed {
		t.Fatalf("expected prune to change state")
	}
	if sm.IsReady("s1") || sm.IsRunning("s1") {
		t.Fatalf("expected s1 to be fully pruned")
	}
	if got := sm.ReadyIDs(); len(got) != 0 {
		t.Fatalf("expected empty ready queue after pruning missing sessions, got %#v", got)
	}
}

func TestRecentsStateMachineRestoreNormalizesAndPreservesDeterministicFIFO(t *testing.T) {
	sm := NewRecentsStateMachine()
	now := time.Now().UTC()

	sm.Restore(RecentsSnapshot{
		Running: map[string]recentsRun{
			"s1": {SessionID: "s1", BaselineTurnID: "turn-u1", StartedAt: now},
		},
		Ready: map[string]recentsReadyItem{
			"s2": {SessionID: "s2", CompletionTurn: "turn-a2", CompletedAt: now.Add(2 * time.Second)},
			"s3": {SessionID: "s3", CompletionTurn: "turn-a3", CompletedAt: now.Add(3 * time.Second)},
		},
		ReadyQueue: []recentsReadyQueueEntry{
			{SessionID: "s2", Seq: 2},
			{SessionID: "s2", Seq: 1}, // duplicate should collapse
			{SessionID: "s3", Seq: 3},
		},
		DismissedTurn: map[string]string{"s4": "turn-a4"},
	})

	if !sm.IsRunning("s1") {
		t.Fatalf("expected running session s1 after restore")
	}
	if !sm.IsReady("s2") || !sm.IsReady("s3") {
		t.Fatalf("expected ready sessions after restore")
	}
	if got := sm.ReadyIDs(); len(got) != 2 || got[0] != "s2" || got[1] != "s3" {
		t.Fatalf("expected deterministic ready fifo [s2 s3], got %#v", got)
	}

	snap := sm.Snapshot()
	if len(snap.DismissedTurn) != 1 || snap.DismissedTurn["s4"] != "turn-a4" {
		t.Fatalf("expected dismissed turn to round-trip after restore")
	}
}
