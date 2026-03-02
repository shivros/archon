package transcriptdomain

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestGoldenTranscriptSnapshot(t *testing.T) {
	snapshot := TranscriptSnapshot{
		SessionID: "session-1",
		Provider:  "codex",
		Revision:  MustParseRevisionToken("12"),
		Blocks: []Block{
			{ID: "b1", Kind: "assistant_message", Role: "assistant", Text: "hello"},
			{ID: "b2", Kind: "user_message", Role: "user", Text: "ship it"},
		},
		Turn: TurnState{State: TurnStateRunning, TurnID: "turn-12"},
		Capabilities: CapabilityEnvelope{
			SupportsGuidedWorkflowDispatch: true,
			SupportsEvents:                 true,
			SupportsApprovals:              true,
			SupportsInterrupt:              true,
		},
	}
	assertGoldenJSON(t, "snapshot.golden.json", snapshot)
}

func TestGoldenTranscriptEventTurnCompleted(t *testing.T) {
	occurredAt := time.Date(2026, 3, 2, 10, 30, 0, 0, time.UTC)
	updatedAt := time.Date(2026, 3, 2, 10, 30, 0, 0, time.UTC)
	event := TranscriptEvent{
		Kind:       TranscriptEventTurnCompleted,
		SessionID:  "session-1",
		Provider:   "codex",
		Revision:   MustParseRevisionToken("13"),
		OccurredAt: &occurredAt,
		Turn: &TurnState{
			State:     TurnStateCompleted,
			TurnID:    "turn-12",
			UpdatedAt: &updatedAt,
		},
	}
	assertGoldenJSON(t, "event.turn_completed.golden.json", event)
}

func TestGoldenTranscriptEventDelta(t *testing.T) {
	event := TranscriptEvent{
		Kind:      TranscriptEventDelta,
		SessionID: "session-1",
		Provider:  "claude",
		Revision:  MustParseRevisionToken("4"),
		Delta: []Block{
			{ID: "b3", Kind: "assistant_delta", Role: "assistant", Text: "partial response"},
		},
	}
	assertGoldenJSON(t, "event.delta.golden.json", event)
}

func TestGoldenTranscriptEventTurnFailed(t *testing.T) {
	occurredAt := time.Date(2026, 3, 2, 10, 31, 0, 0, time.UTC)
	event := TranscriptEvent{
		Kind:       TranscriptEventTurnFailed,
		SessionID:  "session-1",
		Provider:   "codex",
		Revision:   MustParseRevisionToken("14"),
		OccurredAt: &occurredAt,
		Turn: &TurnState{
			State:  TurnStateFailed,
			TurnID: "turn-13",
			Error:  "provider error",
		},
	}
	assertGoldenJSON(t, "event.turn_failed.golden.json", event)
}

func assertGoldenJSON(t *testing.T, name string, value any) {
	t.Helper()
	actual, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	actual = append(actual, '\n')
	goldenPath := filepath.Join("testdata", name)
	expected, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden %s: %v", goldenPath, err)
	}
	if !bytes.Equal(actual, expected) {
		t.Fatalf("golden mismatch for %s\nactual:\n%s\nexpected:\n%s", name, string(actual), string(expected))
	}
}
