package app

import (
	"strings"

	"control/internal/types"
)

type DebugStreamController struct {
	events           <-chan types.DebugEvent
	cancel           func()
	lines            []string
	lineBytes        []int
	totalBytes       int
	pending          string
	contentCache     string
	contentDirty     bool
	retention        DebugStreamRetentionPolicy
	maxEventsPerTick int
}

func NewDebugStreamController(retention DebugStreamRetentionPolicy, maxEventsPerTick int) *DebugStreamController {
	retention = retention.normalize()
	return &DebugStreamController{
		retention:        retention,
		maxEventsPerTick: maxEventsPerTick,
		contentDirty:     true,
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
	c.lineBytes = nil
	c.totalBytes = 0
	c.pending = ""
	c.contentCache = ""
	c.contentDirty = true
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

func (c *DebugStreamController) Content() string {
	if c == nil {
		return ""
	}
	if c.contentDirty {
		c.contentCache = strings.Join(c.lines, "\n")
		c.contentDirty = false
	}
	return c.contentCache
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
	for _, line := range parts[:len(parts)-1] {
		c.lines = append(c.lines, line)
		lineBytes := len(line)
		c.lineBytes = append(c.lineBytes, lineBytes)
		c.totalBytes += lineBytes
	}
	for {
		trimLines := c.retention.MaxLines > 0 && len(c.lines) > c.retention.MaxLines
		trimBytes := c.retention.MaxBytes > 0 && c.totalBytes > c.retention.MaxBytes
		if !trimLines && !trimBytes {
			break
		}
		if len(c.lines) == 0 {
			break
		}
		c.totalBytes -= c.lineBytes[0]
		c.lines = c.lines[1:]
		c.lineBytes = c.lineBytes[1:]
	}
	c.contentDirty = true
}
