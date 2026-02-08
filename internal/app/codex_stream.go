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
	activeAgentID    string
	agentDeltaSeen   bool
	lastError        string
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
	c.activeAgentID = ""
	c.agentDeltaSeen = false
	c.lastError = ""
}

func (c *CodexStreamController) HasStream() bool {
	if c == nil {
		return false
	}
	return c.events != nil
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
		c.transcript.SetBlocks(nil)
	}
}

func (c *CodexStreamController) SetSnapshotBlocks(blocks []ChatBlock) {
	if c == nil {
		return
	}
	if c.transcript != nil {
		c.transcript.SetBlocks(blocks)
	}
}

func (c *CodexStreamController) AppendUserMessage(text string) int {
	if c == nil || c.transcript == nil {
		return -1
	}
	return c.transcript.AppendUserMessage(text)
}

func (c *CodexStreamController) Blocks() []ChatBlock {
	if c == nil {
		return nil
	}
	if c.transcript == nil {
		return nil
	}
	return c.transcript.Blocks()
}

func (c *CodexStreamController) PendingApproval() *ApprovalRequest {
	if c == nil {
		return nil
	}
	return c.pendingApproval
}

func (c *CodexStreamController) LastError() string {
	if c == nil {
		return ""
	}
	return c.lastError
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

func (c *CodexStreamController) ConsumeTick() (changed bool, closed bool) {
	if c == nil || c.events == nil {
		return false, false
	}
	for i := 0; i < c.maxEventsPerTick; i++ {
		select {
		case event, ok := <-c.events:
			if !ok {
				c.events = nil
				c.cancel = nil
				closed = true
				return changed, closed
			}
			if c.applyEvent(event) {
				changed = true
			}
		default:
			return changed, closed
		}
	}
	return changed, closed
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
			if id, _ := payload.Item["id"].(string); id != "" {
				c.activeAgentID = id
			} else {
				c.activeAgentID = ""
			}
			c.agentDeltaSeen = false
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
		c.agentDeltaSeen = true
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
			if !c.agentDeltaSeen {
				text := asString(payload.Item["text"])
				if text == "" {
					text = extractContentText(payload.Item["content"])
				}
				if text != "" && c.transcript != nil {
					c.transcript.AppendAgentDelta(text)
				}
			}
			if c.transcript != nil {
				c.transcript.FinishAgentBlock()
			}
			c.activeAgentID = ""
			c.agentDeltaSeen = false
			return true
		}
	case "error", "codex/event/error":
		if msg := parseCodexError(event.Params); msg != "" {
			c.lastError = msg
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

func parseCodexError(params []byte) string {
	if len(params) == 0 {
		return ""
	}
	payload := map[string]any{}
	if err := json.Unmarshal(params, &payload); err != nil {
		return ""
	}
	if errVal, ok := payload["error"]; ok {
		if msg := extractErrorMessage(errVal); msg != "" {
			return msg
		}
	}
	if msgVal, ok := payload["msg"]; ok {
		if msg := extractErrorMessage(msgVal); msg != "" {
			return msg
		}
	}
	return ""
}

func extractErrorMessage(raw any) string {
	switch val := raw.(type) {
	case map[string]any:
		if msg, ok := val["message"].(string); ok {
			return msg
		}
	case string:
		return val
	}
	return ""
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
