package transcriptdomain

import "testing"

func TestValidateSnapshotAcceptsCanonicalSnapshot(t *testing.T) {
	snapshot := TranscriptSnapshot{
		SessionID: "s1",
		Provider:  "codex",
		Revision:  MustParseRevisionToken("1"),
		Blocks: []Block{{
			Kind: "assistant_message",
			Text: "hello",
		}},
		Turn: TurnState{State: TurnStateIdle},
		Capabilities: CapabilityEnvelope{
			SupportsEvents: true,
		},
	}
	if err := ValidateSnapshot(snapshot); err != nil {
		t.Fatalf("ValidateSnapshot: %v", err)
	}
}

func TestValidateSnapshotRejectsMissingProvider(t *testing.T) {
	snapshot := TranscriptSnapshot{
		SessionID: "s1",
		Revision:  MustParseRevisionToken("1"),
		Turn:      TurnState{State: TurnStateIdle},
	}
	if err := ValidateSnapshot(snapshot); err == nil {
		t.Fatal("expected provider validation error")
	}
}

func TestValidateSnapshotRejectsInvalidBlock(t *testing.T) {
	snapshot := TranscriptSnapshot{
		SessionID: "s1",
		Provider:  "codex",
		Revision:  MustParseRevisionToken("1"),
		Blocks:    []Block{{Kind: "assistant_message"}},
		Turn:      TurnState{State: TurnStateIdle},
	}
	if err := ValidateSnapshot(snapshot); err == nil {
		t.Fatal("expected snapshot block validation error")
	}
}

func TestValidateEventRejectsInvalidDelta(t *testing.T) {
	event := TranscriptEvent{
		Kind:      TranscriptEventDelta,
		SessionID: "s1",
		Provider:  "codex",
		Revision:  MustParseRevisionToken("2"),
	}
	if err := ValidateEvent(event); err == nil {
		t.Fatal("expected delta validation error")
	}
}

func TestValidateEventAcceptsTurnCompleted(t *testing.T) {
	event := TranscriptEvent{
		Kind:      TranscriptEventTurnCompleted,
		SessionID: "s1",
		Provider:  "codex",
		Revision:  MustParseRevisionToken("3"),
		Turn: &TurnState{
			State:  TurnStateCompleted,
			TurnID: "turn-3",
		},
	}
	if err := ValidateEvent(event); err != nil {
		t.Fatalf("ValidateEvent: %v", err)
	}
}

func TestValidateEventRejectsTurnCompletedWithFailedState(t *testing.T) {
	event := TranscriptEvent{
		Kind:      TranscriptEventTurnCompleted,
		SessionID: "s1",
		Provider:  "codex",
		Revision:  MustParseRevisionToken("3"),
		Turn: &TurnState{
			State:  TurnStateFailed,
			TurnID: "turn-3",
			Error:  "boom",
		},
	}
	if err := ValidateEvent(event); err == nil {
		t.Fatal("expected turn completed mismatch error")
	}
}

func TestValidateEventAcceptsTurnFailed(t *testing.T) {
	event := TranscriptEvent{
		Kind:      TranscriptEventTurnFailed,
		SessionID: "s1",
		Provider:  "codex",
		Revision:  MustParseRevisionToken("4"),
		Turn: &TurnState{
			State:  TurnStateFailed,
			TurnID: "turn-4",
			Error:  "boom",
		},
	}
	if err := ValidateEvent(event); err != nil {
		t.Fatalf("ValidateEvent: %v", err)
	}
}

func TestValidateEventAcceptsTurnStarted(t *testing.T) {
	event := TranscriptEvent{
		Kind:      TranscriptEventTurnStarted,
		SessionID: "s1",
		Provider:  "codex",
		Revision:  MustParseRevisionToken("4"),
		Turn: &TurnState{
			State:  TurnStateRunning,
			TurnID: "turn-4",
		},
	}
	if err := ValidateEvent(event); err != nil {
		t.Fatalf("ValidateEvent: %v", err)
	}
}

func TestValidateEventRejectsTurnStartedWithCompletedState(t *testing.T) {
	event := TranscriptEvent{
		Kind:      TranscriptEventTurnStarted,
		SessionID: "s1",
		Provider:  "codex",
		Revision:  MustParseRevisionToken("4"),
		Turn: &TurnState{
			State:  TurnStateCompleted,
			TurnID: "turn-4",
		},
	}
	if err := ValidateEvent(event); err == nil {
		t.Fatal("expected turn started mismatch")
	}
}

func TestValidateEventAcceptsStreamStatus(t *testing.T) {
	event := TranscriptEvent{
		Kind:         TranscriptEventStreamStatus,
		SessionID:    "s1",
		Provider:     "codex",
		Revision:     MustParseRevisionToken("4"),
		StreamStatus: StreamStatusReconnecting,
	}
	if err := ValidateEvent(event); err != nil {
		t.Fatalf("ValidateEvent: %v", err)
	}
}

func TestValidateEventRejectsInvalidStreamStatus(t *testing.T) {
	event := TranscriptEvent{
		Kind:         TranscriptEventStreamStatus,
		SessionID:    "s1",
		Provider:     "codex",
		Revision:     MustParseRevisionToken("4"),
		StreamStatus: StreamStatus("bogus"),
	}
	if err := ValidateEvent(event); err == nil {
		t.Fatal("expected stream status validation error")
	}
}

func TestValidateEventAcceptsHeartbeatWithoutRevision(t *testing.T) {
	event := TranscriptEvent{
		Kind:      TranscriptEventHeartbeat,
		SessionID: "s1",
		Provider:  "codex",
	}
	if err := ValidateEvent(event); err != nil {
		t.Fatalf("ValidateEvent heartbeat: %v", err)
	}
}

func TestValidateEventRejectsUnsupportedKind(t *testing.T) {
	event := TranscriptEvent{
		Kind:      TranscriptEventKind("unsupported"),
		SessionID: "s1",
		Provider:  "codex",
		Revision:  MustParseRevisionToken("4"),
	}
	if err := ValidateEvent(event); err == nil {
		t.Fatal("expected unsupported-kind error")
	}
}

func TestValidateEventApprovalPendingAndResolved(t *testing.T) {
	pending := TranscriptEvent{
		Kind:      TranscriptEventApprovalPending,
		SessionID: "s1",
		Provider:  "codex",
		Revision:  MustParseRevisionToken("9"),
		Approval:  &ApprovalState{RequestID: 1, Method: "item/requestApproval", State: "pending"},
	}
	if err := ValidateEvent(pending); err != nil {
		t.Fatalf("ValidateEvent pending: %v", err)
	}

	resolved := TranscriptEvent{
		Kind:      TranscriptEventApprovalResolved,
		SessionID: "s1",
		Provider:  "codex",
		Revision:  MustParseRevisionToken("10"),
		Approval:  &ApprovalState{RequestID: 1, Method: "item/replyPermission", State: "resolved"},
	}
	if err := ValidateEvent(resolved); err != nil {
		t.Fatalf("ValidateEvent resolved: %v", err)
	}
}

func TestValidateEventRejectsApprovalWithoutMethod(t *testing.T) {
	event := TranscriptEvent{
		Kind:      TranscriptEventApprovalPending,
		SessionID: "s1",
		Provider:  "codex",
		Revision:  MustParseRevisionToken("9"),
		Approval:  &ApprovalState{RequestID: 1},
	}
	if err := ValidateEvent(event); err == nil {
		t.Fatal("expected approval method validation error")
	}
}

func TestValidateTurnStateRequiresErrorForFailed(t *testing.T) {
	if err := ValidateTurnState(TurnState{State: TurnStateFailed, TurnID: "turn-1"}); err == nil {
		t.Fatal("expected failed turn state to require error")
	}
}

func TestValidateTurnStateRules(t *testing.T) {
	cases := []struct {
		name    string
		turn    TurnState
		wantErr bool
	}{
		{name: "idle ok", turn: TurnState{State: TurnStateIdle}},
		{name: "idle with turn id invalid", turn: TurnState{State: TurnStateIdle, TurnID: "x"}, wantErr: true},
		{name: "running requires turn id", turn: TurnState{State: TurnStateRunning}, wantErr: true},
		{name: "running with error invalid", turn: TurnState{State: TurnStateRunning, TurnID: "x", Error: "oops"}, wantErr: true},
		{name: "completed requires turn id", turn: TurnState{State: TurnStateCompleted}, wantErr: true},
		{name: "completed with error invalid", turn: TurnState{State: TurnStateCompleted, TurnID: "x", Error: "oops"}, wantErr: true},
		{name: "failed requires turn id", turn: TurnState{State: TurnStateFailed, Error: "boom"}, wantErr: true},
		{name: "unknown invalid", turn: TurnState{State: TurnLifecycleState("unknown")}, wantErr: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateTurnState(tc.turn)
			if tc.wantErr && err == nil {
				t.Fatal("expected error")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestValidateCapabilityEnvelopeRejectsApprovalsWithoutTransport(t *testing.T) {
	caps := CapabilityEnvelope{SupportsApprovals: true}
	if err := ValidateCapabilityEnvelope(caps); err == nil {
		t.Fatal("expected approvals transport validation error")
	}
}

func TestValidateCapabilityEnvelopeRejectsInterruptWithoutTransport(t *testing.T) {
	caps := CapabilityEnvelope{SupportsInterrupt: true}
	if err := ValidateCapabilityEnvelope(caps); err == nil {
		t.Fatal("expected interrupt transport validation error")
	}
}

func TestValidateCapabilityEnvelopeAcceptsTransportBackedCapabilities(t *testing.T) {
	caps := CapabilityEnvelope{SupportsEvents: true, SupportsApprovals: true, SupportsInterrupt: true}
	if err := ValidateCapabilityEnvelope(caps); err != nil {
		t.Fatalf("unexpected capability validation error: %v", err)
	}
}

func TestValidateBlockAndStreamStatus(t *testing.T) {
	if err := ValidateBlock(Block{Kind: "assistant", Text: "hi"}); err != nil {
		t.Fatalf("valid block rejected: %v", err)
	}
	if err := ValidateBlock(Block{Kind: "assistant"}); err == nil {
		t.Fatal("expected missing block text error")
	}
	if err := ValidateBlock(Block{Kind: "assistant", Text: " \n"}); err == nil {
		t.Fatal("expected whitespace-only snapshot block text error")
	}
	if err := ValidateBlock(Block{Text: "hi"}); err == nil {
		t.Fatal("expected missing block kind error")
	}
	if err := ValidateDeltaBlock(Block{Kind: "assistant", Text: " \n"}); err != nil {
		t.Fatalf("expected whitespace-only delta block text to remain valid, got %v", err)
	}
	if err := ValidateStreamStatus(StreamStatusReady); err != nil {
		t.Fatalf("valid stream status rejected: %v", err)
	}
	if err := ValidateStreamStatus(StreamStatus("invalid")); err == nil {
		t.Fatal("expected invalid stream status error")
	}
}

func TestValidateEventReplaceRejectsMismatchedSnapshotEnvelope(t *testing.T) {
	event := TranscriptEvent{
		Kind:      TranscriptEventReplace,
		SessionID: "s1",
		Provider:  "codex",
		Revision:  MustParseRevisionToken("5"),
		Replace: &TranscriptSnapshot{
			SessionID: "s1",
			Provider:  "codex",
			Revision:  MustParseRevisionToken("6"),
			Blocks:    []Block{{Kind: "assistant", Text: "hello"}},
			Turn:      TurnState{State: TurnStateIdle},
			Capabilities: CapabilityEnvelope{
				SupportsEvents: true,
			},
		},
	}
	if err := ValidateEvent(event); err == nil {
		t.Fatal("expected replace revision mismatch validation error")
	}
}

func TestValidateEventReplaceAcceptsMatchingSnapshotEnvelope(t *testing.T) {
	event := TranscriptEvent{
		Kind:      TranscriptEventReplace,
		SessionID: "s1",
		Provider:  "codex",
		Revision:  MustParseRevisionToken("5"),
		Replace: &TranscriptSnapshot{
			SessionID: "s1",
			Provider:  "codex",
			Revision:  MustParseRevisionToken("5"),
			Blocks:    []Block{{Kind: "assistant", Text: "hello"}},
			Turn:      TurnState{State: TurnStateIdle},
			Capabilities: CapabilityEnvelope{
				SupportsEvents: true,
			},
		},
	}
	if err := ValidateEvent(event); err != nil {
		t.Fatalf("unexpected replace validation error: %v", err)
	}
}
