package acp

import (
	"context"
	"encoding/json"
)

const (
	ProtocolVersion1 = 1

	ErrorCodeParseError     = -32700
	ErrorCodeInvalidRequest = -32600
	ErrorCodeMethodNotFound = -32601
	ErrorCodeInvalidParams  = -32602
	ErrorCodeInternalError  = -32603
)

const (
	MethodInitialize        = "initialize"
	MethodAuthenticate      = "authenticate"
	MethodSessionNew        = "session/new"
	MethodSessionLoad       = "session/load"
	MethodSessionPrompt     = "session/prompt"
	MethodSessionCancel     = "session/cancel"
	MethodSessionSetMode    = "session/set_mode"
	MethodSessionUpdate     = "session/update"
	MethodRequestPermission = "session/request_permission"
)

const (
	StopReasonEndTurn         = "end_turn"
	StopReasonMaxTokens       = "max_tokens"
	StopReasonMaxTurnRequests = "max_turn_requests"
	StopReasonRefusal         = "refusal"
	StopReasonCancelled       = "cancelled"
)

const (
	PermissionOutcomeSelected  = "selected"
	PermissionOutcomeCancelled = "cancelled"
)

const (
	SessionUpdateAgentMessageChunk = "agent_message_chunk"
	SessionUpdateUserMessageChunk  = "user_message_chunk"
	SessionUpdateAgentThoughtChunk = "agent_thought_chunk"
	SessionUpdateToolCall          = "tool_call"
	SessionUpdateToolCallUpdate    = "tool_call_update"
	SessionUpdatePlan              = "plan"
	SessionUpdateCurrentMode       = "current_mode_update"
	SessionUpdateAvailableCommands = "available_commands_update"
)

// RPCError is a JSON-RPC 2.0 error object. It implements the error interface
// so handlers can return it directly to propagate a specific error code.
type RPCError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data,omitempty"`
}

func (e *RPCError) Error() string {
	if e == nil {
		return ""
	}
	return e.Message
}

// ImplementationInfo identifies a client or agent implementation.
type ImplementationInfo struct {
	Name    string `json:"name"`
	Title   string `json:"title,omitempty"`
	Version string `json:"version,omitempty"`
}

type ClientCapabilities struct {
	FS       FSCapabilities `json:"fs"`
	Terminal bool           `json:"terminal"`
}

type FSCapabilities struct {
	ReadTextFile  bool `json:"readTextFile"`
	WriteTextFile bool `json:"writeTextFile"`
}

type AgentCapabilities struct {
	LoadSession        bool                `json:"loadSession"`
	PromptCapabilities *PromptCapabilities `json:"promptCapabilities,omitempty"`
	McpCapabilities    *McpCapabilities    `json:"mcpCapabilities,omitempty"`
}

type PromptCapabilities struct {
	Image           bool `json:"image"`
	Audio           bool `json:"audio"`
	EmbeddedContext bool `json:"embeddedContext"`
}

type McpCapabilities struct {
	HTTP bool `json:"http"`
	SSE  bool `json:"sse"`
}

type InitializeParams struct {
	ProtocolVersion    int                `json:"protocolVersion"`
	ClientCapabilities ClientCapabilities `json:"clientCapabilities"`
	ClientInfo         ImplementationInfo `json:"clientInfo"`
}

type InitializeResult struct {
	ProtocolVersion   int                `json:"protocolVersion"`
	AgentCapabilities AgentCapabilities  `json:"agentCapabilities"`
	AgentInfo         ImplementationInfo `json:"agentInfo"`
	AuthMethods       []AuthMethod       `json:"authMethods,omitempty"`
}

type AuthMethod struct {
	ID          string `json:"id"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description,omitempty"`
}

type NewSessionParams struct {
	Cwd        string      `json:"cwd"`
	McpServers []McpServer `json:"mcpServers"`
}

type McpServer struct {
	Name    string            `json:"name"`
	Command string            `json:"command,omitempty"`
	Args    []string          `json:"args,omitempty"`
	Env     map[string]string `json:"env,omitempty"`
	URL     string            `json:"url,omitempty"`
}

type NewSessionResult struct {
	SessionID string `json:"sessionId"`
}

type LoadSessionParams struct {
	SessionID  string      `json:"sessionId"`
	Cwd        string      `json:"cwd,omitempty"`
	McpServers []McpServer `json:"mcpServers"`
}

type LoadSessionResult struct {
	SessionID string `json:"sessionId"`
}

type PromptParams struct {
	SessionID string         `json:"sessionId"`
	Prompt    []ContentBlock `json:"prompt"`
}

type PromptResult struct {
	StopReason string `json:"stopReason"`
}

type CancelParams struct {
	SessionID string `json:"sessionId"`
}

// ContentBlock covers prompt content, message-chunk content, and embedded
// tool-call content. Only Text is guaranteed baseline; callers that need other
// variants should inspect Type and decode the full payload themselves.
type ContentBlock struct {
	Type     string `json:"type"`
	Text     string `json:"text,omitempty"`
	MimeType string `json:"mimeType,omitempty"`
	URI      string `json:"uri,omitempty"`
	Data     string `json:"data,omitempty"`
}

// SessionUpdateNotification is the decoded params of a session/update
// notification. Exactly one of the typed pointer fields is non-nil when
// SessionUpdate is a known variant; otherwise Raw holds the full update
// object so unknown variants are not lost.
type SessionUpdateNotification struct {
	SessionID     string          `json:"-"`
	SessionUpdate string          `json:"-"`
	Raw           json.RawMessage `json:"-"`

	AgentMessageChunk *MessageChunk      `json:"-"`
	UserMessageChunk  *MessageChunk      `json:"-"`
	AgentThoughtChunk *MessageChunk      `json:"-"`
	ToolCall          *ToolCall          `json:"-"`
	ToolCallUpdate    *ToolCallUpdate    `json:"-"`
	Plan              *Plan              `json:"-"`
	CurrentMode       *CurrentModeUpdate `json:"-"`
	AvailableCommands *AvailableCommands `json:"-"`
}

// DecodeSessionUpdate decodes a session/update notification's params into a
// SessionUpdateNotification. Unknown sessionUpdate discriminators produce a
// result where only SessionID, SessionUpdate, and Raw are set.
func DecodeSessionUpdate(params json.RawMessage) (SessionUpdateNotification, error) {
	var out SessionUpdateNotification
	var envelope struct {
		SessionID string          `json:"sessionId"`
		Update    json.RawMessage `json:"update"`
	}
	if err := json.Unmarshal(params, &envelope); err != nil {
		return out, err
	}
	out.SessionID = envelope.SessionID
	out.Raw = append(out.Raw[:0], envelope.Update...)
	if len(envelope.Update) == 0 {
		return out, nil
	}
	var disc struct {
		SessionUpdate string `json:"sessionUpdate"`
	}
	if err := json.Unmarshal(envelope.Update, &disc); err != nil {
		return out, err
	}
	out.SessionUpdate = disc.SessionUpdate
	switch disc.SessionUpdate {
	case SessionUpdateAgentMessageChunk:
		var v MessageChunk
		if err := json.Unmarshal(envelope.Update, &v); err != nil {
			return out, err
		}
		out.AgentMessageChunk = &v
	case SessionUpdateUserMessageChunk:
		var v MessageChunk
		if err := json.Unmarshal(envelope.Update, &v); err != nil {
			return out, err
		}
		out.UserMessageChunk = &v
	case SessionUpdateAgentThoughtChunk:
		var v MessageChunk
		if err := json.Unmarshal(envelope.Update, &v); err != nil {
			return out, err
		}
		out.AgentThoughtChunk = &v
	case SessionUpdateToolCall:
		var v ToolCall
		if err := json.Unmarshal(envelope.Update, &v); err != nil {
			return out, err
		}
		out.ToolCall = &v
	case SessionUpdateToolCallUpdate:
		var v ToolCallUpdate
		if err := json.Unmarshal(envelope.Update, &v); err != nil {
			return out, err
		}
		out.ToolCallUpdate = &v
	case SessionUpdatePlan:
		var v Plan
		if err := json.Unmarshal(envelope.Update, &v); err != nil {
			return out, err
		}
		out.Plan = &v
	case SessionUpdateCurrentMode:
		var v CurrentModeUpdate
		if err := json.Unmarshal(envelope.Update, &v); err != nil {
			return out, err
		}
		out.CurrentMode = &v
	case SessionUpdateAvailableCommands:
		var v AvailableCommands
		if err := json.Unmarshal(envelope.Update, &v); err != nil {
			return out, err
		}
		out.AvailableCommands = &v
	}
	return out, nil
}

type MessageChunk struct {
	Content ContentBlock `json:"content"`
}

type ToolCall struct {
	ToolCallID string             `json:"toolCallId"`
	Title      string             `json:"title,omitempty"`
	Kind       string             `json:"kind,omitempty"`
	Status     string             `json:"status,omitempty"`
	Content    []ToolCallContent  `json:"content,omitempty"`
	Locations  []ToolCallLocation `json:"locations,omitempty"`
	RawInput   json.RawMessage    `json:"rawInput,omitempty"`
	RawOutput  json.RawMessage    `json:"rawOutput,omitempty"`
}

// ToolCallUpdate aliases ToolCall: the two ACP variants carry the same
// payload shape. The SessionUpdateNotification discriminator, not the type,
// distinguishes tool_call (initial state) from tool_call_update (delta).
type ToolCallUpdate = ToolCall

type ToolCallContent struct {
	Type    string        `json:"type"`
	Content *ContentBlock `json:"content,omitempty"`
}

type ToolCallLocation struct {
	Path string `json:"path,omitempty"`
	Line *int   `json:"line,omitempty"`
}

type Plan struct {
	Entries []PlanEntry `json:"entries"`
}

type PlanEntry struct {
	Content  string `json:"content"`
	Priority string `json:"priority,omitempty"`
	Status   string `json:"status,omitempty"`
}

type CurrentModeUpdate struct {
	CurrentModeID string `json:"currentModeId"`
}

type AvailableCommands struct {
	AvailableCommands []Command `json:"availableCommands"`
}

type Command struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

type RequestPermissionParams struct {
	SessionID string             `json:"sessionId"`
	ToolCall  ToolCall           `json:"toolCall"`
	Options   []PermissionOption `json:"options"`
}

type PermissionOption struct {
	OptionID string `json:"optionId"`
	Name     string `json:"name,omitempty"`
	Kind     string `json:"kind,omitempty"`
}

type RequestPermissionResult struct {
	Outcome RequestPermissionOutcome `json:"outcome"`
}

type RequestPermissionOutcome struct {
	Outcome  string `json:"outcome"`
	OptionID string `json:"optionId,omitempty"`
}

func Selected(optionID string) RequestPermissionOutcome {
	return RequestPermissionOutcome{Outcome: PermissionOutcomeSelected, OptionID: optionID}
}

func CancelledOutcome() RequestPermissionOutcome {
	return RequestPermissionOutcome{Outcome: PermissionOutcomeCancelled}
}

// Notification is a generic notification delivered to subscribers.
type Notification struct {
	Method string
	Params json.RawMessage
}

// RequestHandler handles an agent-to-client request. It returns the response
// payload (which will be JSON-encoded and sent back) or an error. A returned
// *RPCError propagates the code and message verbatim; any other error becomes
// an internal-error response.
type RequestHandler func(ctx context.Context, params json.RawMessage) (any, error)
