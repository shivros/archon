package app

import "control/internal/types"

type MetadataStreamController struct {
	ch               <-chan types.MetadataEvent
	cancel           func()
	maxEventsPerTick int
}

func NewMetadataStreamController(maxEventsPerTick int) *MetadataStreamController {
	return &MetadataStreamController{maxEventsPerTick: maxEventsPerTick}
}

func (c *MetadataStreamController) Reset() {
	if c == nil {
		return
	}
	if c.cancel != nil {
		c.cancel()
	}
	c.ch = nil
	c.cancel = nil
}

func (c *MetadataStreamController) SetStream(ch <-chan types.MetadataEvent, cancel func()) {
	if c == nil {
		return
	}
	if c.cancel != nil {
		c.cancel()
	}
	c.ch = ch
	c.cancel = cancel
}

func (c *MetadataStreamController) HasStream() bool {
	return c != nil && c.ch != nil
}

func (c *MetadataStreamController) ConsumeTick() (events []types.MetadataEvent, changed bool, closed bool) {
	if c == nil || c.ch == nil {
		return nil, false, false
	}
	out := make([]types.MetadataEvent, 0, c.maxEventsPerTick)
	for i := 0; i < c.maxEventsPerTick; i++ {
		select {
		case event, ok := <-c.ch:
			if !ok {
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
