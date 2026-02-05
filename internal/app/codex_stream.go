package app

import (
	"encoding/json"

	"control/internal/types"
)

type CodexStreamController struct {
	events           <-chan types.CodexEvent
	cancel           func()
	maxEventsPerTick int
	transcript       *ChatTranscript
	pendingApproval  *ApprovalRequest
}

func NewCodexStreamController(maxLines, maxEventsPerTick int) *CodexStreamController {
	return &CodexStreamController{
		maxEventsPerTick: maxEventsPerTick,
		transcript:       NewChatTranscript(maxLines),
	}
}

func (c *CodexStreamController) Reset() {
	if c == nil {
		return
	}
	if c.cancel != nil {
		c.cancel()
	}
	c.cancel = nil
	c.events = nil
	if c.transcript != nil {
		c.transcript.Reset()
	}
	c.pendingApproval = nil
}

func (c *CodexStreamController) SetStream(ch <-chan types.CodexEvent, cancel func()) {
	if c == nil {
		return
	}
	if c.cancel != nil {
		c.cancel()
	}
	c.events = ch
	c.cancel = cancel
}

func (c *CodexStreamController) SetSnapshot(lines []string) {
	if c == nil {
		return
	}
	if c.transcript != nil {
		c.transcript.SetLines(lines)
	}
}

func (c *CodexStreamController) AppendUserMessage(text string) int {
	if c == nil || c.transcript == nil {
		return -1
	}
	return c.transcript.AppendUserMessage(text)
}

func (c *CodexStreamController) Lines() []string {
	if c == nil {
		return nil
	}
	if c.transcript == nil {
		return nil
	}
	return c.transcript.Lines()
}

func (c *CodexStreamController) PendingApproval() *ApprovalRequest {
	if c == nil {
		return nil
	}
	return c.pendingApproval
}

func (c *CodexStreamController) ClearApproval() {
	if c == nil {
		return
	}
	c.pendingApproval = nil
}

func (c *CodexStreamController) MarkUserMessageFailed(headerIndex int) bool {
	if c == nil || c.transcript == nil {
		return false
	}
	return c.transcript.MarkUserMessageFailed(headerIndex)
}

func (c *CodexStreamController) MarkUserMessageSending(headerIndex int) bool {
	if c == nil || c.transcript == nil {
		return false
	}
	return c.transcript.MarkUserMessageSending(headerIndex)
}

func (c *CodexStreamController) MarkUserMessageSent(headerIndex int) bool {
	if c == nil || c.transcript == nil {
		return false
	}
	return c.transcript.MarkUserMessageSent(headerIndex)
}

func (c *CodexStreamController) ConsumeTick() (lines []string, changed bool, closed bool) {
	if c == nil || c.events == nil {
		return nil, false, false
	}
	for i := 0; i < c.maxEventsPerTick; i++ {
		select {
		case event, ok := <-c.events:
			if !ok {
				c.events = nil
				c.cancel = nil
				closed = true
				return c.Lines(), changed, closed
			}
			if c.applyEvent(event) {
				changed = true
			}
		default:
			return c.Lines(), changed, closed
		}
	}
	return c.Lines(), changed, closed
}

func (c *CodexStreamController) applyEvent(event types.CodexEvent) bool {
	switch event.Method {
	case "item/started":
		var payload struct {
			Item map[string]any `json:"item"`
		}
		if len(event.Params) == 0 || json.Unmarshal(event.Params, &payload) != nil {
			return false
		}
		if payload.Item == nil {
			return false
		}
		if typ, _ := payload.Item["type"].(string); typ == "agentMessage" {
			if c.transcript != nil {
				c.transcript.StartAgentBlock()
			}
			return true
		}
	case "item/agentMessage/delta":
		delta := extractDelta(event.Params)
		if delta == "" {
			return false
		}
		if c.transcript != nil {
			c.transcript.AppendAgentDelta(delta)
		}
		return true
	case "item/completed":
		var payload struct {
			Item map[string]any `json:"item"`
		}
		if len(event.Params) == 0 || json.Unmarshal(event.Params, &payload) != nil {
			return false
		}
		if payload.Item == nil {
			return false
		}
		if typ, _ := payload.Item["type"].(string); typ == "agentMessage" {
			if c.transcript != nil {
				c.transcript.FinishAgentBlock()
			}
			return true
		}
	case "item/commandExecution/requestApproval", "item/fileChange/requestApproval", "tool/requestUserInput":
		req := parseApprovalRequest(event)
		if req != nil {
			c.pendingApproval = req
		}
	}
	return false
}

func parseApprovalRequest(event types.CodexEvent) *ApprovalRequest {
	if event.ID == nil || *event.ID <= 0 {
		return nil
	}
	params := map[string]any{}
	if len(event.Params) > 0 {
		if err := json.Unmarshal(event.Params, &params); err != nil {
			params = map[string]any{}
		}
	}
	summary, detail := approvalSummary(event.Method, params)
	return &ApprovalRequest{
		RequestID: *event.ID,
		Method:    event.Method,
		Summary:   summary,
		Detail:    detail,
	}
}

func extractDelta(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var payload struct {
		Delta string `json:"delta"`
		Text  string `json:"text"`
	}
	if err := json.Unmarshal(raw, &payload); err != nil {
		return ""
	}
	if payload.Delta != "" {
		return payload.Delta
	}
	return payload.Text
}
