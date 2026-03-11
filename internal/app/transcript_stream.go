package app

import (
	"strings"

	"control/internal/daemon/transcriptdomain"
)

type TranscriptStreamController struct {
	events           <-chan transcriptdomain.TranscriptEvent
	cancel           func()
	maxEventsPerTick int
	ingestor         TranscriptIngestor
	blocks           []ChatBlock
	revision         transcriptdomain.RevisionToken
	streamStatus     transcriptdomain.StreamStatus
	awaitingFirst    bool
	generation       uint64
}

type TranscriptTickSignals struct {
	Events            int
	ContentEvents     int
	DeltaEvents       int
	FinalizedEvents   int
	SnapshotEvents    int
	FinalizedDedupes  int
	ControlEvents     int
	CompletionSignals []TranscriptCompletionSignal
	RevisionRewind    bool
	Generation        uint64
	FirstRevision     string
}

type TranscriptCompletionSignal struct {
	TurnID string
}

func NewTranscriptStreamController(maxEventsPerTick int) *TranscriptStreamController {
	return &TranscriptStreamController{
		maxEventsPerTick: maxEventsPerTick,
		ingestor:         NewDefaultTranscriptIngestor(),
	}
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
	c.awaitingFirst = false
	c.generation = 0
}

func (c *TranscriptStreamController) SetStream(ch <-chan transcriptdomain.TranscriptEvent, cancel func()) {
	c.SetStreamWithGeneration(ch, cancel, 0)
}

func (c *TranscriptStreamController) SetStreamWithGeneration(ch <-chan transcriptdomain.TranscriptEvent, cancel func(), generation uint64) {
	if c == nil {
		return
	}
	if c.cancel != nil {
		c.cancel()
	}
	c.events = ch
	c.cancel = cancel
	c.awaitingFirst = ch != nil
	c.generation = generation
}

func (c *TranscriptStreamController) DetachStream() {
	if c == nil {
		return
	}
	if c.cancel != nil {
		c.cancel()
	}
	c.events = nil
	c.cancel = nil
	c.awaitingFirst = false
	c.streamStatus = transcriptdomain.StreamStatusReconnecting
}

func (c *TranscriptStreamController) Generation() uint64 {
	if c == nil {
		return 0
	}
	return c.generation
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
	return c.applySnapshot(snapshot, false)
}

func (c *TranscriptStreamController) SetAuthoritativeSnapshot(snapshot transcriptdomain.TranscriptSnapshot) (changed bool, applied bool) {
	return c.applySnapshot(snapshot, true)
}

func (c *TranscriptStreamController) applySnapshot(snapshot transcriptdomain.TranscriptSnapshot, authoritative bool) (changed bool, applied bool) {
	if c == nil {
		return false, false
	}
	ingestor := c.ingestor
	if ingestor == nil {
		ingestor = NewDefaultTranscriptIngestor()
	}
	result := ingestor.ApplySnapshot(TranscriptIngestState{Blocks: c.blocks, Revision: c.revision}, snapshot, authoritative)
	if !result.Applied {
		return false, false
	}
	c.blocks = result.State.Blocks
	c.revision = result.State.Revision
	return result.Changed, true
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
			signals.Generation = c.generation
			if signals.FirstRevision == "" && c.awaitingFirst && transcriptEventConsumesFirstRevision(event.Kind) {
				signals.FirstRevision = strings.TrimSpace(event.Revision.String())
			}
			eventChanged, eventSignal, eventContent, completion, rewind, category, dedupeHits := c.applyEvent(event)
			if eventChanged {
				changed = true
			}
			if eventSignal {
				signal = true
			}
			if rewind {
				signals.RevisionRewind = true
			}
			if completion != nil {
				signals.CompletionSignals = append(signals.CompletionSignals, *completion)
			}
			signals.FinalizedDedupes += dedupeHits
			switch category {
			case transcriptEventCategoryDelta:
				signals.DeltaEvents++
			case transcriptEventCategoryFinalizedMessage:
				signals.FinalizedEvents++
			case transcriptEventCategorySnapshotReplace:
				signals.SnapshotEvents++
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

func (c *TranscriptStreamController) applyEvent(event transcriptdomain.TranscriptEvent) (changed bool, signal bool, content bool, completion *TranscriptCompletionSignal, rewind bool, category TranscriptEventCategory, finalizedDedupeHits int) {
	ingestor := c.ingestor
	if ingestor == nil {
		ingestor = NewDefaultTranscriptIngestor()
	}
	firstEvent := c.awaitingFirst
	result := ingestor.ApplyEvent(
		TranscriptIngestState{Blocks: c.blocks, Revision: c.revision},
		event,
		firstEvent,
	)
	c.blocks = result.State.Blocks
	c.revision = result.State.Revision
	if event.Kind == transcriptdomain.TranscriptEventStreamStatus {
		c.streamStatus = event.StreamStatus
	}
	if firstEvent && transcriptEventConsumesFirstRevision(event.Kind) {
		c.awaitingFirst = false
	}
	return result.Changed, result.Signal, result.Content, result.Completion, result.Rewind, result.Category, result.FinalizedDedupeHits
}

func transcriptEventConsumesFirstRevision(kind transcriptdomain.TranscriptEventKind) bool {
	switch kind {
	case transcriptdomain.TranscriptEventStreamStatus,
		transcriptdomain.TranscriptEventReplace,
		transcriptdomain.TranscriptEventDelta,
		transcriptdomain.TranscriptEventTurnStarted,
		transcriptdomain.TranscriptEventTurnCompleted,
		transcriptdomain.TranscriptEventTurnFailed,
		transcriptdomain.TranscriptEventApprovalPending,
		transcriptdomain.TranscriptEventApprovalResolved:
		return true
	default:
		return false
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
