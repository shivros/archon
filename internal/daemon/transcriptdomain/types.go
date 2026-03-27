package transcriptdomain

import "time"

type TranscriptEventKind string

const (
	TranscriptEventReplace          TranscriptEventKind = "transcript.replace"
	TranscriptEventDelta            TranscriptEventKind = "transcript.delta"
	TranscriptEventTurnStarted      TranscriptEventKind = "turn.started"
	TranscriptEventTurnCompleted    TranscriptEventKind = "turn.completed"
	TranscriptEventTurnFailed       TranscriptEventKind = "turn.failed"
	TranscriptEventApprovalPending  TranscriptEventKind = "approval.pending"
	TranscriptEventApprovalResolved TranscriptEventKind = "approval.resolved"
	TranscriptEventStreamStatus     TranscriptEventKind = "stream.status"
	TranscriptEventHeartbeat        TranscriptEventKind = "heartbeat"
)

type TurnLifecycleState string

const (
	TurnStateIdle      TurnLifecycleState = "idle"
	TurnStateRunning   TurnLifecycleState = "running"
	TurnStateCompleted TurnLifecycleState = "completed"
	TurnStateFailed    TurnLifecycleState = "failed"
)

type StreamStatus string

const (
	StreamStatusReady        StreamStatus = "ready"
	StreamStatusClosed       StreamStatus = "closed"
	StreamStatusReconnecting StreamStatus = "reconnecting"
	StreamStatusError        StreamStatus = "error"
)

type Block struct {
	ID      string         `json:"id,omitempty"`
	Kind    string         `json:"kind,omitempty"`
	Role    string         `json:"role,omitempty"`
	Text    string         `json:"text,omitempty"`
	Variant string         `json:"variant,omitempty"`
	Meta    map[string]any `json:"meta,omitempty"`
}

type TurnState struct {
	State     TurnLifecycleState `json:"state"`
	TurnID    string             `json:"turn_id,omitempty"`
	Error     string             `json:"error,omitempty"`
	UpdatedAt *time.Time         `json:"updated_at,omitempty"`
}

type CapabilityEnvelope struct {
	SupportsGuidedWorkflowDispatch bool `json:"supports_guided_workflow_dispatch"`
	UsesItems                      bool `json:"uses_items"`
	SupportsEvents                 bool `json:"supports_events"`
	SupportsApprovals              bool `json:"supports_approvals"`
	SupportsInterrupt              bool `json:"supports_interrupt"`
	SupportsFileSearch             bool `json:"supports_file_search"`
	NoProcess                      bool `json:"no_process"`
}

type ApprovalState struct {
	RequestID int    `json:"request_id,omitempty"`
	State     string `json:"state,omitempty"`
	Method    string `json:"method,omitempty"`
}

type TranscriptSnapshot struct {
	SessionID    string             `json:"session_id"`
	Provider     string             `json:"provider"`
	Revision     RevisionToken      `json:"revision"`
	Blocks       []Block            `json:"blocks"`
	Turn         TurnState          `json:"turn_state"`
	Capabilities CapabilityEnvelope `json:"capabilities"`
}

type TranscriptEvent struct {
	Kind         TranscriptEventKind `json:"kind"`
	SessionID    string              `json:"session_id"`
	Provider     string              `json:"provider"`
	Revision     RevisionToken       `json:"revision,omitempty"`
	OccurredAt   *time.Time          `json:"occurred_at,omitempty"`
	Delta        []Block             `json:"delta,omitempty"`
	Replace      *TranscriptSnapshot `json:"replace,omitempty"`
	Turn         *TurnState          `json:"turn,omitempty"`
	Capabilities *CapabilityEnvelope `json:"capabilities,omitempty"`
	StreamStatus StreamStatus        `json:"stream_status,omitempty"`
	Approval     *ApprovalState      `json:"approval,omitempty"`
}
