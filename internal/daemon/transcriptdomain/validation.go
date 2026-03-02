package transcriptdomain

import (
	"fmt"
	"strings"
)

func ValidateSnapshot(snapshot TranscriptSnapshot) error {
	if strings.TrimSpace(snapshot.SessionID) == "" {
		return fmt.Errorf("snapshot session_id is required")
	}
	if strings.TrimSpace(snapshot.Provider) == "" {
		return fmt.Errorf("snapshot provider is required")
	}
	if _, err := ParseRevisionToken(snapshot.Revision.String()); err != nil {
		return fmt.Errorf("snapshot revision: %w", err)
	}
	if err := ValidateTurnState(snapshot.Turn); err != nil {
		return fmt.Errorf("snapshot turn_state: %w", err)
	}
	if err := ValidateCapabilityEnvelope(snapshot.Capabilities); err != nil {
		return fmt.Errorf("snapshot capabilities: %w", err)
	}
	for i, block := range snapshot.Blocks {
		if err := ValidateBlock(block); err != nil {
			return fmt.Errorf("snapshot block[%d]: %w", i, err)
		}
	}
	return nil
}

func ValidateEvent(event TranscriptEvent) error {
	if strings.TrimSpace(string(event.Kind)) == "" {
		return fmt.Errorf("event kind is required")
	}
	if strings.TrimSpace(event.SessionID) == "" {
		return fmt.Errorf("event session_id is required")
	}
	if strings.TrimSpace(event.Provider) == "" {
		return fmt.Errorf("event provider is required")
	}
	if event.Kind != TranscriptEventHeartbeat {
		if _, err := ParseRevisionToken(event.Revision.String()); err != nil {
			return fmt.Errorf("event revision: %w", err)
		}
	}

	switch event.Kind {
	case TranscriptEventReplace:
		if event.Replace == nil {
			return fmt.Errorf("replace event must include replace snapshot")
		}
		if err := ValidateSnapshot(*event.Replace); err != nil {
			return fmt.Errorf("replace snapshot: %w", err)
		}
		if strings.TrimSpace(event.Replace.SessionID) != strings.TrimSpace(event.SessionID) {
			return fmt.Errorf("replace snapshot session_id mismatch")
		}
		if strings.TrimSpace(event.Replace.Provider) != strings.TrimSpace(event.Provider) {
			return fmt.Errorf("replace snapshot provider mismatch")
		}
		if event.Replace.Revision.String() != event.Revision.String() {
			return fmt.Errorf("replace snapshot revision mismatch")
		}
	case TranscriptEventDelta:
		if len(event.Delta) == 0 {
			return fmt.Errorf("delta event must include at least one block")
		}
		for i, block := range event.Delta {
			if err := ValidateBlock(block); err != nil {
				return fmt.Errorf("delta block[%d]: %w", i, err)
			}
		}
	case TranscriptEventTurnStarted:
		if event.Turn == nil {
			return fmt.Errorf("turn event must include turn state")
		}
		if err := ValidateTurnState(*event.Turn); err != nil {
			return fmt.Errorf("turn event state: %w", err)
		}
		if event.Turn.State != TurnStateRunning {
			return fmt.Errorf("turn.started requires running state")
		}
	case TranscriptEventTurnCompleted:
		if event.Turn == nil {
			return fmt.Errorf("turn event must include turn state")
		}
		if err := ValidateTurnState(*event.Turn); err != nil {
			return fmt.Errorf("turn event state: %w", err)
		}
		if event.Turn.State != TurnStateCompleted {
			return fmt.Errorf("turn.completed requires completed state")
		}
	case TranscriptEventTurnFailed:
		if event.Turn == nil {
			return fmt.Errorf("turn event must include turn state")
		}
		if err := ValidateTurnState(*event.Turn); err != nil {
			return fmt.Errorf("turn event state: %w", err)
		}
		if event.Turn.State != TurnStateFailed {
			return fmt.Errorf("turn.failed requires failed state")
		}
	case TranscriptEventApprovalPending, TranscriptEventApprovalResolved:
		if event.Approval == nil {
			return fmt.Errorf("approval event must include approval payload")
		}
		if strings.TrimSpace(event.Approval.Method) == "" {
			return fmt.Errorf("approval method is required")
		}
	case TranscriptEventStreamStatus:
		if err := ValidateStreamStatus(event.StreamStatus); err != nil {
			return fmt.Errorf("stream status: %w", err)
		}
	case TranscriptEventHeartbeat:
		// Heartbeats are intentionally minimal and may omit revision/payloads.
	default:
		return fmt.Errorf("unsupported event kind %q", event.Kind)
	}

	if event.Capabilities != nil {
		if err := ValidateCapabilityEnvelope(*event.Capabilities); err != nil {
			return fmt.Errorf("event capabilities: %w", err)
		}
	}
	return nil
}

func ValidateTurnState(turn TurnState) error {
	switch turn.State {
	case TurnStateIdle:
		if strings.TrimSpace(turn.TurnID) != "" {
			return fmt.Errorf("idle turn state must not include turn_id")
		}
		if strings.TrimSpace(turn.Error) != "" {
			return fmt.Errorf("idle turn state must not include error")
		}
	case TurnStateRunning:
		if strings.TrimSpace(turn.TurnID) == "" {
			return fmt.Errorf("running turn state requires turn_id")
		}
		if strings.TrimSpace(turn.Error) != "" {
			return fmt.Errorf("running turn state must not include error")
		}
	case TurnStateCompleted:
		if strings.TrimSpace(turn.TurnID) == "" {
			return fmt.Errorf("completed turn state requires turn_id")
		}
		if strings.TrimSpace(turn.Error) != "" {
			return fmt.Errorf("completed turn state must not include error")
		}
	case TurnStateFailed:
		if strings.TrimSpace(turn.TurnID) == "" {
			return fmt.Errorf("failed turn state requires turn_id")
		}
		if strings.TrimSpace(turn.Error) == "" {
			return fmt.Errorf("failed turn state requires error")
		}
	default:
		return fmt.Errorf("unsupported turn state %q", turn.State)
	}
	return nil
}

func ValidateCapabilityEnvelope(c CapabilityEnvelope) error {
	hasTransport := c.SupportsEvents || c.UsesItems
	if c.SupportsApprovals && !hasTransport {
		return fmt.Errorf("supports_approvals requires supports_events or uses_items")
	}
	if c.SupportsInterrupt && !hasTransport {
		return fmt.Errorf("supports_interrupt requires supports_events or uses_items")
	}
	return nil
}

func ValidateBlock(block Block) error {
	if strings.TrimSpace(block.Kind) == "" {
		return fmt.Errorf("block kind is required")
	}
	if strings.TrimSpace(block.Text) == "" {
		return fmt.Errorf("block text is required")
	}
	return nil
}

func ValidateStreamStatus(status StreamStatus) error {
	switch status {
	case StreamStatusReady, StreamStatusClosed, StreamStatusReconnecting, StreamStatusError:
		return nil
	default:
		return fmt.Errorf("unsupported stream status %q", status)
	}
}
