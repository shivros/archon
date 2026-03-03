package app

import (
	"strings"

	"control/internal/daemon/transcriptdomain"
)

type TranscriptStreamController struct {
	events           <-chan transcriptdomain.TranscriptEvent
	cancel           func()
	maxEventsPerTick int
	blocks           []ChatBlock
	revision         transcriptdomain.RevisionToken
	streamStatus     transcriptdomain.StreamStatus
}

func NewTranscriptStreamController(maxEventsPerTick int) *TranscriptStreamController {
	return &TranscriptStreamController{maxEventsPerTick: maxEventsPerTick}
}

func (c *TranscriptStreamController) Reset() {
	if c == nil {
		return
	}
	if c.cancel != nil {
		c.cancel()
	}
	c.events = nil
	c.cancel = nil
	c.blocks = nil
	c.revision = ""
	c.streamStatus = ""
}

func (c *TranscriptStreamController) SetStream(ch <-chan transcriptdomain.TranscriptEvent, cancel func()) {
	if c == nil {
		return
	}
	if c.cancel != nil {
		c.cancel()
	}
	c.events = ch
	c.cancel = cancel
}

func (c *TranscriptStreamController) HasStream() bool {
	return c != nil && c.events != nil
}

func (c *TranscriptStreamController) Blocks() []ChatBlock {
	if c == nil {
		return nil
	}
	return append([]ChatBlock(nil), c.blocks...)
}

func (c *TranscriptStreamController) Revision() string {
	if c == nil {
		return ""
	}
	return strings.TrimSpace(c.revision.String())
}

func (c *TranscriptStreamController) StreamStatus() transcriptdomain.StreamStatus {
	if c == nil {
		return ""
	}
	return c.streamStatus
}

func (c *TranscriptStreamController) SetSnapshot(snapshot transcriptdomain.TranscriptSnapshot) (changed bool, applied bool) {
	if c == nil {
		return false, false
	}
	if !isTranscriptRevisionNewer(snapshot.Revision, c.revision) {
		return false, false
	}
	blocks := transcriptBlocksToChatBlocks(snapshot.Blocks)
	c.blocks = blocks
	c.revision = snapshot.Revision
	return true, true
}

func (c *TranscriptStreamController) ConsumeTick() (changed bool, closed bool, signal bool, events int) {
	if c == nil || c.events == nil {
		return false, false, false, 0
	}
	for i := 0; i < c.maxEventsPerTick; i++ {
		select {
		case event, ok := <-c.events:
			if !ok {
				c.events = nil
				c.cancel = nil
				c.streamStatus = transcriptdomain.StreamStatusClosed
				return changed, true, signal, events
			}
			events++
			eventChanged, eventSignal := c.applyEvent(event)
			if eventChanged {
				changed = true
			}
			if eventSignal {
				signal = true
			}
		default:
			return changed, closed, signal, events
		}
	}
	return changed, closed, signal, events
}

func (c *TranscriptStreamController) applyEvent(event transcriptdomain.TranscriptEvent) (changed bool, signal bool) {
	switch event.Kind {
	case transcriptdomain.TranscriptEventHeartbeat:
		return false, false
	case transcriptdomain.TranscriptEventStreamStatus:
		if !isTranscriptRevisionNewer(event.Revision, c.revision) {
			return false, false
		}
		c.revision = event.Revision
		c.streamStatus = event.StreamStatus
		if event.StreamStatus == transcriptdomain.StreamStatusReady {
			return false, true
		}
		return false, false
	case transcriptdomain.TranscriptEventReplace:
		if !isTranscriptRevisionNewer(event.Revision, c.revision) {
			return false, false
		}
		c.revision = event.Revision
		if event.Replace != nil {
			c.blocks = transcriptBlocksToChatBlocks(event.Replace.Blocks)
			return true, true
		}
		return false, true
	case transcriptdomain.TranscriptEventDelta:
		if !isTranscriptRevisionNewer(event.Revision, c.revision) {
			return false, false
		}
		c.revision = event.Revision
		delta := transcriptBlocksToChatBlocks(event.Delta)
		if len(delta) == 0 {
			return false, true
		}
		c.blocks = append(c.blocks, delta...)
		return true, true
	case transcriptdomain.TranscriptEventTurnStarted,
		transcriptdomain.TranscriptEventTurnCompleted,
		transcriptdomain.TranscriptEventTurnFailed,
		transcriptdomain.TranscriptEventApprovalPending,
		transcriptdomain.TranscriptEventApprovalResolved:
		if !isTranscriptRevisionNewer(event.Revision, c.revision) {
			return false, false
		}
		c.revision = event.Revision
		return false, true
	default:
		return false, false
	}
}

func isTranscriptRevisionNewer(next, current transcriptdomain.RevisionToken) bool {
	if next.IsZero() {
		return false
	}
	if current.IsZero() {
		return true
	}
	newer, err := transcriptdomain.IsRevisionNewer(next, current)
	if err != nil {
		return false
	}
	return newer
}
