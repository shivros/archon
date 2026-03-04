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

type TranscriptTickSignals struct {
	Events        int
	ContentEvents int
	ControlEvents int
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

func (c *TranscriptStreamController) ConsumeTick() (changed bool, closed bool, signal bool, signals TranscriptTickSignals) {
	if c == nil || c.events == nil {
		return false, false, false, TranscriptTickSignals{}
	}
	for i := 0; i < c.maxEventsPerTick; i++ {
		select {
		case event, ok := <-c.events:
			if !ok {
				c.events = nil
				c.cancel = nil
				c.streamStatus = transcriptdomain.StreamStatusClosed
				return changed, true, signal, signals
			}
			signals.Events++
			eventChanged, eventSignal, eventContent := c.applyEvent(event)
			if eventChanged {
				changed = true
			}
			if eventSignal {
				signal = true
			}
			if eventContent {
				signals.ContentEvents++
			} else {
				signals.ControlEvents++
			}
		default:
			return changed, closed, signal, signals
		}
	}
	return changed, closed, signal, signals
}

func (c *TranscriptStreamController) applyEvent(event transcriptdomain.TranscriptEvent) (changed bool, signal bool, content bool) {
	switch event.Kind {
	case transcriptdomain.TranscriptEventHeartbeat:
		return false, false, false
	case transcriptdomain.TranscriptEventStreamStatus:
		if !isTranscriptRevisionNewer(event.Revision, c.revision) {
			return false, false, false
		}
		c.revision = event.Revision
		c.streamStatus = event.StreamStatus
		if event.StreamStatus == transcriptdomain.StreamStatusReady {
			return false, true, false
		}
		return false, false, false
	case transcriptdomain.TranscriptEventReplace:
		if !isTranscriptRevisionNewer(event.Revision, c.revision) {
			return false, false, false
		}
		c.revision = event.Revision
		if event.Replace != nil {
			c.blocks = transcriptBlocksToChatBlocks(event.Replace.Blocks)
			return true, true, transcriptBlocksContainUserRelevantContent(event.Replace.Blocks)
		}
		return false, true, false
	case transcriptdomain.TranscriptEventDelta:
		if !isTranscriptRevisionNewer(event.Revision, c.revision) {
			return false, false, false
		}
		c.revision = event.Revision
		delta := transcriptBlocksToChatBlocks(event.Delta)
		if len(delta) == 0 {
			return false, true, false
		}
		c.blocks = append(c.blocks, delta...)
		return true, true, transcriptBlocksContainUserRelevantContent(event.Delta)
	case transcriptdomain.TranscriptEventTurnStarted,
		transcriptdomain.TranscriptEventTurnCompleted,
		transcriptdomain.TranscriptEventTurnFailed,
		transcriptdomain.TranscriptEventApprovalPending,
		transcriptdomain.TranscriptEventApprovalResolved:
		if !isTranscriptRevisionNewer(event.Revision, c.revision) {
			return false, false, false
		}
		c.revision = event.Revision
		return false, true, false
	default:
		return false, false, false
	}
}

func transcriptBlocksContainUserRelevantContent(blocks []transcriptdomain.Block) bool {
	for _, block := range blocks {
		role := strings.ToLower(strings.TrimSpace(block.Role))
		kind := strings.ToLower(strings.TrimSpace(block.Kind))
		text := strings.TrimSpace(block.Text)
		if kind == "provider_event" {
			continue
		}
		if role == "assistant" || role == "user" || role == "reasoning" || role == "agent" || role == "model" {
			return true
		}
		if strings.Contains(kind, "assistant") || strings.Contains(kind, "agent") || strings.Contains(kind, "reasoning") {
			return true
		}
		if text != "" && role != "system" {
			return true
		}
	}
	return false
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
