package app

import (
	"testing"
	"time"
)

func TestDefaultTranscriptRecoveryCoordinatorLifecycle(t *testing.T) {
	coordIface := NewDefaultTranscriptRecoveryCoordinator()
	coord, ok := coordIface.(*defaultTranscriptRecoveryCoordinator)
	if !ok {
		t.Fatalf("expected default coordinator implementation")
	}

	detectedAt := time.Date(2026, 3, 11, 10, 0, 0, 0, time.UTC)
	recoveredAt := detectedAt.Add(5 * time.Second)
	coord.nowFn = func() time.Time { return detectedAt }

	coord.FlagRewind(" s1 ", 7, "")
	if !coord.ShouldApplyAuthoritativeSnapshot("s1") {
		t.Fatalf("expected rewound session to require authoritative snapshot")
	}
	state, ok := coord.State("s1")
	if !ok {
		t.Fatalf("expected recovery state to exist")
	}
	if state.Generation != 7 || state.Reason != transcriptReasonRecoveryRevisionRewind || !state.DetectedAt.Equal(detectedAt) {
		t.Fatalf("unexpected rewind state: %#v", state)
	}

	coord.nowFn = func() time.Time { return recoveredAt }
	coord.MarkRecovered("s1")
	state, ok = coord.State("s1")
	if !ok {
		t.Fatalf("expected recovered state to persist")
	}
	if state.Rewound {
		t.Fatalf("expected recovered state to clear rewound flag")
	}
	if !state.RecoveredAt.Equal(recoveredAt) {
		t.Fatalf("expected recovered timestamp %v, got %#v", recoveredAt, state)
	}

	coord.Clear("s1")
	if _, ok := coord.State("s1"); ok {
		t.Fatalf("expected clear to remove recovery state")
	}
}

func TestDefaultTranscriptRecoveryCoordinatorReset(t *testing.T) {
	coord := NewDefaultTranscriptRecoveryCoordinator()
	coord.FlagRewind("s1", 1, transcriptReasonRecoveryRevisionRewind)
	coord.FlagRewind("s2", 2, transcriptReasonRecoveryRevisionRewind)

	coord.Reset()

	if _, ok := coord.State("s1"); ok {
		t.Fatalf("expected s1 state cleared after reset")
	}
	if _, ok := coord.State("s2"); ok {
		t.Fatalf("expected s2 state cleared after reset")
	}
}

func TestWithTranscriptRecoveryCoordinatorOption(t *testing.T) {
	custom := NewDefaultTranscriptRecoveryCoordinator()
	model := NewModel(nil, WithTranscriptRecoveryCoordinator(custom))
	if model.transcriptRecoveryCoordinator != custom {
		t.Fatalf("expected custom recovery coordinator to be installed")
	}

	model = NewModel(nil, WithTranscriptRecoveryCoordinator(nil))
	if model.transcriptRecoveryCoordinator == nil {
		t.Fatalf("expected nil option to install default recovery coordinator")
	}
}
