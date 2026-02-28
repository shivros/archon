package daemon

import "testing"

func TestTurnEvidenceFreshnessTrackerMarksDuplicateEvidenceStale(t *testing.T) {
	tracker := NewTurnEvidenceFreshnessTracker()
	if !tracker.MarkFresh("sess-1", "id:msg-1", "hello") {
		t.Fatalf("expected first evidence to be fresh")
	}
	if tracker.MarkFresh("sess-1", "id:msg-1", "hello again") {
		t.Fatalf("expected duplicate evidence to be stale")
	}
	if !tracker.MarkFresh("sess-1", "id:msg-2", "new") {
		t.Fatalf("expected new evidence key to be fresh")
	}
}

func TestTurnEvidenceFreshnessTrackerFallbacksToOutputPresence(t *testing.T) {
	tracker := NewTurnEvidenceFreshnessTracker()
	if tracker.MarkFresh("sess-1", "", "") {
		t.Fatalf("expected empty output without key to be stale")
	}
	if !tracker.MarkFresh("sess-1", "", "output") {
		t.Fatalf("expected non-empty output without key to be fresh")
	}
}

func TestTurnEvidenceFreshnessTrackerSessionScopedKeys(t *testing.T) {
	tracker := NewTurnEvidenceFreshnessTracker()
	if !tracker.MarkFresh("sess-a", "id:msg-1", "a") {
		t.Fatalf("expected first key in session A to be fresh")
	}
	if !tracker.MarkFresh("sess-b", "id:msg-1", "b") {
		t.Fatalf("expected same key in different session to be fresh")
	}
}

func TestTurnEvidenceFreshnessTrackerAllowsSessionlessKey(t *testing.T) {
	tracker := NewTurnEvidenceFreshnessTracker()
	if !tracker.MarkFresh("", "id:msg-1", "") {
		t.Fatalf("expected sessionless key to be treated as fresh")
	}
}
