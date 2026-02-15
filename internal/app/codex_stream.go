package app

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"control/internal/types"
)

type CodexStreamController struct {
	events           <-chan types.CodexEvent
	cancel           func()
	maxEventsPerTick int
	transcript       *ChatTranscript
	reasoning        *codexReasoningAccumulator
	reasoningSeq     int
	pendingApproval  *ApprovalRequest
	activeAgentID    string
	agentDeltaSeen   bool
	lastError        string
}

func NewCodexStreamController(maxLines, maxEventsPerTick int) *CodexStreamController {
	controller := &CodexStreamController{
		maxEventsPerTick: maxEventsPerTick,
		transcript:       NewChatTranscript(maxLines),
	}
	controller.resetReasoningAccumulator()
	return controller
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
	c.reasoning = nil
	c.reasoningSeq = 0
	c.resetReasoningAccumulator()
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
	c.resetReasoningAccumulator()
}

func (c *CodexStreamController) SetSnapshotBlocks(blocks []ChatBlock) {
	if c == nil {
		return
	}
	if c.transcript != nil {
		c.transcript.SetBlocks(blocks)
	}
	c.resetReasoningAccumulator()
}

func (c *CodexStreamController) AppendUserMessage(text string) int {
	if c == nil || c.transcript == nil {
		return -1
	}
	c.resetReasoningAccumulator()
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

func (c *CodexStreamController) ConsumeTick() (changed bool, closed bool, events int) {
	if c == nil || c.events == nil {
		return false, false, 0
	}
	for i := 0; i < c.maxEventsPerTick; i++ {
		select {
		case event, ok := <-c.events:
			if !ok {
				c.events = nil
				c.cancel = nil
				closed = true
				return changed, closed, events
			}
			events++
			if c.applyEvent(event) {
				changed = true
			}
		default:
			return changed, closed, events
		}
	}
	return changed, closed, events
}

func (c *CodexStreamController) applyEvent(event types.CodexEvent) bool {
	switch event.Method {
	case "item/started":
		item := parseEventItem(event.Params)
		if item == nil {
			return false
		}
		typ, _ := item["type"].(string)
		if typ == "agentMessage" {
			if id, _ := item["id"].(string); id != "" {
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
		if typ == "reasoning" {
			return c.applyReasoningItem(item)
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
	case "item/updated":
		item := parseEventItem(event.Params)
		if item == nil {
			return false
		}
		typ := asString(item["type"])
		if typ == "reasoning" {
			return c.applyReasoningItem(item)
		}
		if typ == "agentMessage" {
			if delta := strings.TrimSpace(asString(item["delta"])); delta != "" && c.transcript != nil {
				c.agentDeltaSeen = true
				c.transcript.AppendAgentDelta(delta)
				return true
			}
		}
	case "item/completed":
		item := parseEventItem(event.Params)
		if item == nil {
			return false
		}
		typ, _ := item["type"].(string)
		if typ == "agentMessage" {
			if !c.agentDeltaSeen {
				text := asString(item["text"])
				if text == "" {
					text = extractContentText(item["content"])
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
		if typ == "reasoning" {
			return c.applyReasoningItem(item)
		}
	case "error", "codex/event/error":
		if msg := parseCodexError(event.Params); msg != "" {
			c.lastError = msg
		}
	case "item/commandExecution/requestApproval", "item/fileChange/requestApproval", "tool/requestUserInput":
		req := parseApprovalRequest(event)
		if req != nil {
			c.pendingApproval = req
			if c.transcript != nil {
				c.transcript.UpsertApproval(req)
			}
			return true
		}
	}
	return false
}

func parseEventItem(params json.RawMessage) map[string]any {
	if len(params) == 0 {
		return nil
	}
	var payload struct {
		Item map[string]any `json:"item"`
	}
	if json.Unmarshal(params, &payload) != nil {
		return nil
	}
	return payload.Item
}

func (c *CodexStreamController) applyReasoningItem(item map[string]any) bool {
	if c == nil || c.transcript == nil || item == nil {
		return false
	}
	text := reasoningText(item)
	if strings.TrimSpace(text) == "" {
		return false
	}
	if c.reasoning == nil {
		c.resetReasoningAccumulator()
	}
	id := strings.TrimSpace(asString(item["id"]))
	aggregateID, aggregateText, _ := c.reasoning.Add(id, text)
	if strings.TrimSpace(aggregateID) == "" || strings.TrimSpace(aggregateText) == "" {
		return false
	}
	return c.transcript.UpsertReasoning(aggregateID, aggregateText)
}

func (c *CodexStreamController) resetReasoningAccumulator() {
	if c == nil {
		return
	}
	c.reasoningSeq++
	groupID := fmt.Sprintf("codex-group-%d", c.reasoningSeq)
	if c.reasoning == nil {
		c.reasoning = newCodexReasoningAccumulator(groupID)
		return
	}
	c.reasoning.Reset(groupID)
}

func parseApprovalRequest(event types.CodexEvent) *ApprovalRequest {
	if event.ID == nil || *event.ID < 0 {
		return nil
	}
	params := map[string]any{}
	if len(event.Params) > 0 {
		if err := json.Unmarshal(event.Params, &params); err != nil {
			params = map[string]any{}
		}
	}
	presentation := approvalPresentationFromParams(event.Method, params)
	createdAt := time.Now().UTC()
	if ts := strings.TrimSpace(event.TS); ts != "" {
		if parsed, err := time.Parse(time.RFC3339Nano, ts); err == nil {
			createdAt = parsed
		}
	}
	return &ApprovalRequest{
		RequestID: *event.ID,
		Method:    event.Method,
		Summary:   presentation.Summary,
		Detail:    presentation.Detail,
		Context:   cloneStringSlice(presentation.Context),
		CreatedAt: createdAt,
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
