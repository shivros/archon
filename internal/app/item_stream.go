package app

type ItemStreamController struct {
	items            <-chan map[string]any
	cancel           func()
	maxEventsPerTick int
	transcript       *ChatTranscript
}

func NewItemStreamController(maxLines, maxEventsPerTick int) *ItemStreamController {
	return &ItemStreamController{
		maxEventsPerTick: maxEventsPerTick,
		transcript:       NewChatTranscript(maxLines),
	}
}

func (c *ItemStreamController) Reset() {
	if c == nil {
		return
	}
	if c.cancel != nil {
		c.cancel()
	}
	c.cancel = nil
	c.items = nil
	if c.transcript != nil {
		c.transcript.Reset()
	}
}

func (c *ItemStreamController) HasStream() bool {
	if c == nil {
		return false
	}
	return c.items != nil
}

func (c *ItemStreamController) SetStream(ch <-chan map[string]any, cancel func()) {
	if c == nil {
		return
	}
	if c.cancel != nil {
		c.cancel()
	}
	c.items = ch
	c.cancel = cancel
}

func (c *ItemStreamController) SetSnapshot(lines []string) {
	if c == nil {
		return
	}
	if c.transcript != nil {
		c.transcript.SetLines(lines)
	}
}

func (c *ItemStreamController) AppendUserMessage(text string) int {
	if c == nil || c.transcript == nil {
		return -1
	}
	return c.transcript.AppendUserMessage(text)
}

func (c *ItemStreamController) Lines() []string {
	if c == nil || c.transcript == nil {
		return nil
	}
	return c.transcript.Lines()
}

func (c *ItemStreamController) MarkUserMessageFailed(headerIndex int) bool {
	if c == nil || c.transcript == nil {
		return false
	}
	return c.transcript.MarkUserMessageFailed(headerIndex)
}

func (c *ItemStreamController) MarkUserMessageSending(headerIndex int) bool {
	if c == nil || c.transcript == nil {
		return false
	}
	return c.transcript.MarkUserMessageSending(headerIndex)
}

func (c *ItemStreamController) MarkUserMessageSent(headerIndex int) bool {
	if c == nil || c.transcript == nil {
		return false
	}
	return c.transcript.MarkUserMessageSent(headerIndex)
}

func (c *ItemStreamController) ConsumeTick() (lines []string, changed bool, closed bool) {
	if c == nil || c.items == nil {
		return nil, false, false
	}
	for i := 0; i < c.maxEventsPerTick; i++ {
		select {
		case item, ok := <-c.items:
			if !ok {
				c.items = nil
				c.cancel = nil
				closed = true
				return c.Lines(), changed, closed
			}
			if c.transcript != nil {
				c.transcript.AppendItem(item)
				changed = true
			}
		default:
			return c.Lines(), changed, closed
		}
	}
	return c.Lines(), changed, closed
}
