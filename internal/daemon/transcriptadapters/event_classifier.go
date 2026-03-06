package transcriptadapters

import (
	"strings"

	"control/internal/providers"
	"control/internal/types"
)

type EventChannel string

const (
	EventChannelTranscript EventChannel = "transcript"
	EventChannelControl    EventChannel = "control"
	EventChannelMetadata   EventChannel = "metadata"
	EventChannelDebug      EventChannel = "debug"
	EventChannelIgnore     EventChannel = "ignore"
)

type EventIntent string

const (
	EventIntentUnknown          EventIntent = "unknown"
	EventIntentAssistantDelta   EventIntent = "assistant_delta"
	EventIntentApprovalPending  EventIntent = "approval_pending"
	EventIntentApprovalResolved EventIntent = "approval_resolved"
	EventIntentTurnStarted      EventIntent = "turn_started"
	EventIntentTurnCompleted    EventIntent = "turn_completed"
	EventIntentTurnFailed       EventIntent = "turn_failed"
	EventIntentStreamReady      EventIntent = "stream_ready"
	EventIntentRateLimitUpdate  EventIntent = "rate_limit_update"
	EventIntentTokenUsageUpdate EventIntent = "token_usage_update"
	EventIntentDebugTrace       EventIntent = "debug_trace"
)

type ClassifiedProviderEvent struct {
	Channel EventChannel
	Intent  EventIntent
	Method  string
}

type ProviderEventClassifier interface {
	Provider() string
	ClassifyEvent(event types.CodexEvent) ClassifiedProviderEvent
}

type codexEventClassifier struct {
	providerName string
}

func NewCodexEventClassifier(providerName string) ProviderEventClassifier {
	providerName = providers.Normalize(providerName)
	if providerName == "" {
		providerName = "codex"
	}
	return codexEventClassifier{providerName: providerName}
}

func (c codexEventClassifier) Provider() string {
	return c.providerName
}

func (c codexEventClassifier) ClassifyEvent(event types.CodexEvent) ClassifiedProviderEvent {
	method := normalizeEventMethod(event.Method)
	switch {
	case strings.Contains(method, "requestapproval"):
		return ClassifiedProviderEvent{Channel: EventChannelTranscript, Intent: EventIntentApprovalPending, Method: method}
	case strings.Contains(method, "approvalresolved"), strings.Contains(method, "replypermission"):
		return ClassifiedProviderEvent{Channel: EventChannelTranscript, Intent: EventIntentApprovalResolved, Method: method}
	case method == "item/agentmessage/delta":
		return ClassifiedProviderEvent{Channel: EventChannelTranscript, Intent: EventIntentAssistantDelta, Method: method}
	case method == "turn/started":
		return ClassifiedProviderEvent{Channel: EventChannelControl, Intent: EventIntentTurnStarted, Method: method}
	case method == "turn/completed":
		return ClassifiedProviderEvent{Channel: EventChannelControl, Intent: EventIntentTurnCompleted, Method: method}
	case method == "session.idle":
		return ClassifiedProviderEvent{Channel: EventChannelControl, Intent: EventIntentStreamReady, Method: method}
	case method == "error":
		return ClassifiedProviderEvent{Channel: EventChannelControl, Intent: EventIntentTurnFailed, Method: method}
	case method == "thread/status/changed":
		if threadStatusFromEventParams(event.Params) == "idle" {
			return ClassifiedProviderEvent{Channel: EventChannelControl, Intent: EventIntentStreamReady, Method: method}
		}
		return ClassifiedProviderEvent{Channel: EventChannelControl, Intent: EventIntentUnknown, Method: method}
	case method == "thread/started", method == "thread/updated", method == "thread/completed":
		return ClassifiedProviderEvent{Channel: EventChannelControl, Intent: EventIntentUnknown, Method: method}
	case strings.HasPrefix(method, "codex/event/mcp_startup"):
		return ClassifiedProviderEvent{Channel: EventChannelControl, Intent: EventIntentUnknown, Method: method}
	case method == "account/ratelimits/updated":
		return ClassifiedProviderEvent{Channel: EventChannelMetadata, Intent: EventIntentRateLimitUpdate, Method: method}
	case method == "thread/tokenusage/updated":
		return ClassifiedProviderEvent{Channel: EventChannelMetadata, Intent: EventIntentTokenUsageUpdate, Method: method}
	case strings.HasPrefix(method, "codex/event/agent_message"), method == "codex/event/exec_command_begin",
		method == "codex/event/item_started", method == "codex/event/task_complete",
		method == "codex/event/token_count", method == "codex/event/turn_diff",
		method == "turn/diff/updated", method == "item/started", method == "item/completed":
		return ClassifiedProviderEvent{Channel: EventChannelDebug, Intent: EventIntentDebugTrace, Method: method}
	default:
		return ClassifiedProviderEvent{Channel: EventChannelIgnore, Intent: EventIntentUnknown, Method: method}
	}
}

type openCodeEventClassifier struct {
	providerName string
}

func NewOpenCodeEventClassifier(providerName string) ProviderEventClassifier {
	providerName = providers.Normalize(providerName)
	if providerName == "" {
		providerName = "opencode"
	}
	return openCodeEventClassifier{providerName: providerName}
}

func (c openCodeEventClassifier) Provider() string {
	return c.providerName
}

func (c openCodeEventClassifier) ClassifyEvent(event types.CodexEvent) ClassifiedProviderEvent {
	method := normalizeEventMethod(event.Method)
	switch {
	case method == "turn/started":
		return ClassifiedProviderEvent{Channel: EventChannelControl, Intent: EventIntentTurnStarted, Method: method}
	case method == "turn/completed":
		return ClassifiedProviderEvent{Channel: EventChannelControl, Intent: EventIntentTurnCompleted, Method: method}
	case method == "session.idle":
		return ClassifiedProviderEvent{Channel: EventChannelControl, Intent: EventIntentStreamReady, Method: method}
	case method == "error":
		return ClassifiedProviderEvent{Channel: EventChannelControl, Intent: EventIntentTurnFailed, Method: method}
	case strings.Contains(method, "requestapproval"):
		return ClassifiedProviderEvent{Channel: EventChannelTranscript, Intent: EventIntentApprovalPending, Method: method}
	case strings.Contains(method, "approvalresolved"), strings.Contains(method, "replypermission"):
		return ClassifiedProviderEvent{Channel: EventChannelTranscript, Intent: EventIntentApprovalResolved, Method: method}
	case method == "account/ratelimits/updated":
		return ClassifiedProviderEvent{Channel: EventChannelMetadata, Intent: EventIntentRateLimitUpdate, Method: method}
	case method == "thread/tokenusage/updated":
		return ClassifiedProviderEvent{Channel: EventChannelMetadata, Intent: EventIntentTokenUsageUpdate, Method: method}
	case method == "thread/status/changed", method == "item/started", method == "item/completed",
		method == "turn/diff/updated", method == "codex/event/exec_command_begin",
		method == "codex/event/item_started", method == "codex/event/task_complete",
		method == "codex/event/token_count", method == "codex/event/turn_diff":
		return ClassifiedProviderEvent{Channel: EventChannelDebug, Intent: EventIntentDebugTrace, Method: method}
	default:
		return ClassifiedProviderEvent{Channel: EventChannelIgnore, Intent: EventIntentUnknown, Method: method}
	}
}

func normalizeEventMethod(method string) string {
	return strings.ToLower(strings.TrimSpace(method))
}
