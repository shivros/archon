package app

import "control/internal/types"

type ComposeFileSearchStreamController struct {
	searchID         string
	ch               <-chan types.FileSearchEvent
	cancel           func()
	maxEventsPerTick int
}

func NewComposeFileSearchStreamController(maxEventsPerTick int) *ComposeFileSearchStreamController {
	return &ComposeFileSearchStreamController{maxEventsPerTick: maxEventsPerTick}
}

func (c *ComposeFileSearchStreamController) Reset() {
	if c == nil {
		return
	}
	if c.cancel != nil {
		c.cancel()
	}
	c.searchID = ""
	c.ch = nil
	c.cancel = nil
}

func (c *ComposeFileSearchStreamController) SetStream(searchID string, ch <-chan types.FileSearchEvent, cancel func()) {
	if c == nil {
		return
	}
	if c.cancel != nil {
		c.cancel()
	}
	c.searchID = searchID
	c.ch = ch
	c.cancel = cancel
}

func (c *ComposeFileSearchStreamController) HasStream() bool {
	return c != nil && c.ch != nil
}

func (c *ComposeFileSearchStreamController) SearchID() string {
	if c == nil {
		return ""
	}
	return c.searchID
}

func (c *ComposeFileSearchStreamController) ConsumeTick() (events []types.FileSearchEvent, changed bool, closed bool) {
	if c == nil || c.ch == nil {
		return nil, false, false
	}
	out := make([]types.FileSearchEvent, 0, c.maxEventsPerTick)
	for i := 0; i < c.maxEventsPerTick; i++ {
		select {
		case event, ok := <-c.ch:
			if !ok {
				c.searchID = ""
				c.ch = nil
				c.cancel = nil
				return out, len(out) > 0, true
			}
			out = append(out, event)
		default:
			return out, len(out) > 0, false
		}
	}
	return out, len(out) > 0, false
}
