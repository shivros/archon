package app

import (
	"encoding/json"
	"strings"

	"control/internal/types"
)

type CodexStreamController struct {
	events            <-chan types.CodexEvent
	cancel            func()
	lines             []string
	maxLines          int
	maxEventsPerTick  int
	activeAgentLine   int
	pendingAgentBlock bool
}

func NewCodexStreamController(maxLines, maxEventsPerTick int) *CodexStreamController {
	return &CodexStreamController{
		maxLines:         maxLines,
		maxEventsPerTick: maxEventsPerTick,
		activeAgentLine:  -1,
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
	c.lines = nil
	c.activeAgentLine = -1
	c.pendingAgentBlock = false
}

func (c *CodexStreamController) SetStream(ch <-chan types.CodexEvent, cancel func()) {
	if c == nil {
		return
	}
	c.events = ch
	c.cancel = cancel
}

func (c *CodexStreamController) SetSnapshot(lines []string) {
	if c == nil {
		return
	}
	c.lines = trimLines(lines, c.maxLines)
	c.activeAgentLine = -1
	c.pendingAgentBlock = false
}

func (c *CodexStreamController) AppendUserMessage(text string) {
	if c == nil || strings.TrimSpace(text) == "" {
		return
	}
	c.lines = append(c.lines, "### User", "")
	c.lines = append(c.lines, text, "")
	c.trim()
}

func (c *CodexStreamController) Lines() []string {
	if c == nil {
		return nil
	}
	return c.lines
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
				return c.lines, changed, closed
			}
			if c.applyEvent(event) {
				changed = true
			}
		default:
			return c.lines, changed, closed
		}
	}
	return c.lines, changed, closed
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
			c.startAgentBlock()
			return true
		}
	case "item/agentMessage/delta":
		delta := extractDelta(event.Params)
		if delta == "" {
			return false
		}
		c.appendAgentDelta(delta)
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
			c.finishAgentBlock()
			return true
		}
	}
	return false
}

func (c *CodexStreamController) startAgentBlock() {
	c.lines = append(c.lines, "### Agent", "")
	c.lines = append(c.lines, "")
	c.activeAgentLine = len(c.lines) - 1
	c.pendingAgentBlock = true
	c.trim()
}

func (c *CodexStreamController) finishAgentBlock() {
	c.activeAgentLine = -1
	c.pendingAgentBlock = false
	c.trim()
}

func (c *CodexStreamController) appendAgentDelta(delta string) {
	if c.activeAgentLine < 0 || c.activeAgentLine >= len(c.lines) {
		if !c.pendingAgentBlock {
			c.startAgentBlock()
		}
	}
	if c.activeAgentLine < 0 || c.activeAgentLine >= len(c.lines) {
		return
	}
	parts := strings.Split(delta, "\n")
	if len(parts) == 0 {
		return
	}
	c.lines[c.activeAgentLine] += parts[0]
	for _, part := range parts[1:] {
		c.lines = append(c.lines, part)
		c.activeAgentLine = len(c.lines) - 1
	}
	c.trim()
}

func (c *CodexStreamController) trim() {
	if c.maxLines <= 0 || len(c.lines) <= c.maxLines {
		return
	}
	drop := len(c.lines) - c.maxLines
	c.lines = c.lines[drop:]
	if c.activeAgentLine >= 0 {
		c.activeAgentLine -= drop
		if c.activeAgentLine < 0 {
			c.activeAgentLine = len(c.lines) - 1
		}
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
