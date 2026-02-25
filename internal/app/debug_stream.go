package app

import (
	"strings"

	"control/internal/types"
)

type DebugStreamController struct {
	events           <-chan types.DebugEvent
	cancel           func()
	lines            []string
	pending          string
	maxLines         int
	maxEventsPerTick int
}

func NewDebugStreamController(maxLines, maxEventsPerTick int) *DebugStreamController {
	return &DebugStreamController{
		maxLines:         maxLines,
		maxEventsPerTick: maxEventsPerTick,
	}
}

func (c *DebugStreamController) Reset() {
	if c == nil {
		return
	}
	if c.cancel != nil {
		c.cancel()
	}
	c.cancel = nil
	c.events = nil
	c.lines = nil
	c.pending = ""
}

func (c *DebugStreamController) SetStream(ch <-chan types.DebugEvent, cancel func()) {
	if c == nil {
		return
	}
	c.events = ch
	c.cancel = cancel
}

func (c *DebugStreamController) Lines() []string {
	if c == nil {
		return nil
	}
	return c.lines
}

func (c *DebugStreamController) HasStream() bool {
	return c != nil && c.events != nil
}

func (c *DebugStreamController) ConsumeTick() (lines []string, changed bool, closed bool) {
	if c == nil || c.events == nil {
		return nil, false, false
	}
	var builder strings.Builder
	drain := true
	for i := 0; i < c.maxEventsPerTick && drain; i++ {
		select {
		case event, ok := <-c.events:
			if !ok {
				c.events = nil
				c.cancel = nil
				closed = true
				drain = false
				break
			}
			builder.WriteString(event.Chunk)
		default:
			drain = false
		}
	}
	if builder.Len() > 0 {
		c.appendText(builder.String())
		changed = true
	}
	return c.lines, changed, closed
}

func (c *DebugStreamController) appendText(text string) {
	combined := c.pending + text
	parts := strings.Split(combined, "\n")
	if len(parts) == 0 {
		return
	}
	if len(parts) == 1 {
		c.pending = parts[0]
		return
	}
	c.pending = parts[len(parts)-1]
	c.lines = append(c.lines, parts[:len(parts)-1]...)
	if c.maxLines > 0 && len(c.lines) > c.maxLines {
		c.lines = c.lines[len(c.lines)-c.maxLines:]
	}
}
