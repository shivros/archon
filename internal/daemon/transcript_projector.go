package daemon

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"control/internal/daemon/transcriptadapters"
	"control/internal/daemon/transcriptdomain"
)

const transcriptDuplicateTimestampWindow = 2 * time.Second

type TranscriptProjector interface {
	Apply(event transcriptdomain.TranscriptEvent) bool
	NextRevision() transcriptdomain.RevisionToken
	Snapshot() transcriptdomain.TranscriptSnapshot
	ActiveTurnID() string
}

type transcriptProjector struct {
	sessionID  string
	provider   string
	blocks     []transcriptdomain.Block
	turn       transcriptdomain.TurnState
	activeTurn string
	last       transcriptdomain.RevisionToken

	numericMode bool
	nextNumeric uint64
	nextLexical uint64
	lexicalBase string
}

func NewTranscriptProjector(sessionID, provider string, baseRevision transcriptdomain.RevisionToken) TranscriptProjector {
	sessionID = strings.TrimSpace(sessionID)
	provider = strings.TrimSpace(provider)
	projection := &transcriptProjector{
		sessionID: sessionID,
		provider:  provider,
		blocks:    []transcriptdomain.Block{},
		turn:      transcriptdomain.TurnState{State: transcriptdomain.TurnStateIdle},
	}
	if baseRevision.IsZero() {
		projection.numericMode = true
		return projection
	}
	projection.last = baseRevision
	if seq, ok := baseRevision.Sequence(); ok {
		projection.numericMode = true
		projection.nextNumeric = seq
		return projection
	}
	projection.numericMode = false
	projection.lexicalBase = baseRevision.String()
	return projection
}

func (p *transcriptProjector) NextRevision() transcriptdomain.RevisionToken {
	if p.numericMode {
		if p.nextNumeric == 0 {
			p.nextNumeric = 1
		} else {
			p.nextNumeric++
		}
		return transcriptdomain.MustParseRevisionToken(strconv.FormatUint(p.nextNumeric, 10))
	}
	p.nextLexical++
	if strings.TrimSpace(p.lexicalBase) == "" {
		return transcriptdomain.MustParseRevisionToken(fmt.Sprintf("r.%020d", p.nextLexical))
	}
	return transcriptdomain.MustParseRevisionToken(fmt.Sprintf("%s.%020d", p.lexicalBase, p.nextLexical))
}

func (p *transcriptProjector) Apply(event transcriptdomain.TranscriptEvent) bool {
	if strings.TrimSpace(event.SessionID) == "" {
		event.SessionID = p.sessionID
	}
	if strings.TrimSpace(event.Provider) == "" {
		event.Provider = p.provider
	}
	if strings.TrimSpace(event.SessionID) != p.sessionID || strings.TrimSpace(event.Provider) != p.provider {
		return false
	}
	if event.Kind != transcriptdomain.TranscriptEventHeartbeat {
		if p.last.IsZero() {
			if _, err := transcriptdomain.ParseRevisionToken(event.Revision.String()); err != nil {
				return false
			}
		} else {
			newer, err := transcriptdomain.IsRevisionNewer(event.Revision, p.last)
			if err != nil || !newer {
				return false
			}
		}
	}

	switch event.Kind {
	case transcriptdomain.TranscriptEventReplace:
		if event.Replace == nil {
			return false
		}
		replace := *event.Replace
		p.blocks = append([]transcriptdomain.Block{}, replace.Blocks...)
		p.turn = replace.Turn
		p.activeTurn = strings.TrimSpace(replace.Turn.TurnID)
	case transcriptdomain.TranscriptEventDelta:
		if len(event.Delta) == 0 {
			return false
		}
		delta := filterDuplicateTranscriptBlocks(p.blocks, event.Delta)
		if len(delta) == 0 {
			return false
		}
		p.blocks = append(p.blocks, delta...)
	case transcriptdomain.TranscriptEventTurnStarted, transcriptdomain.TranscriptEventTurnCompleted, transcriptdomain.TranscriptEventTurnFailed:
		if event.Turn == nil {
			return false
		}
		p.turn = *event.Turn
		p.activeTurn = strings.TrimSpace(event.Turn.TurnID)
	case transcriptdomain.TranscriptEventStreamStatus, transcriptdomain.TranscriptEventApprovalPending, transcriptdomain.TranscriptEventApprovalResolved, transcriptdomain.TranscriptEventHeartbeat:
		// Non-transcript-structural events.
	default:
		return false
	}
	if event.Kind != transcriptdomain.TranscriptEventHeartbeat {
		p.last = event.Revision
	}
	return true
}

func (p *transcriptProjector) Snapshot() transcriptdomain.TranscriptSnapshot {
	revision := p.last
	if revision.IsZero() {
		revision = transcriptdomain.MustParseRevisionToken("1")
	}
	return transcriptdomain.TranscriptSnapshot{
		SessionID:    p.sessionID,
		Provider:     p.provider,
		Revision:     revision,
		Blocks:       append([]transcriptdomain.Block{}, p.blocks...),
		Turn:         p.turn,
		Capabilities: transcriptadapters.CapabilityEnvelopeFromProvider(p.provider),
	}
}

func (p *transcriptProjector) ActiveTurnID() string {
	return strings.TrimSpace(p.activeTurn)
}

func filterDuplicateTranscriptBlocks(
	existing []transcriptdomain.Block,
	incoming []transcriptdomain.Block,
) []transcriptdomain.Block {
	if len(incoming) == 0 {
		return nil
	}
	out := make([]transcriptdomain.Block, 0, len(incoming))
	seen := append([]transcriptdomain.Block(nil), existing...)
	for _, block := range incoming {
		if isDuplicateTranscriptBlock(seen, block) {
			continue
		}
		out = append(out, block)
		seen = append(seen, block)
	}
	return out
}

func isDuplicateTranscriptBlock(existing []transcriptdomain.Block, next transcriptdomain.Block) bool {
	role := strings.ToLower(strings.TrimSpace(next.Role))
	if !transcriptRoleSupportsReplayDedupe(role) {
		return false
	}
	text := normalizeTranscriptBlockText(next.Text)
	if text == "" {
		return false
	}
	nextMessageID := transcriptBlockMetaString(next.Meta, "provider_message_id", "providerMessageID", "message_id")
	nextTurnID := transcriptBlockMetaString(next.Meta, "turn_id", "turnId")
	nextCreatedAt := transcriptBlockMetaTime(next.Meta, "provider_created_at", "created_at", "createdAt", "timestamp", "ts")
	for _, block := range existing {
		if strings.ToLower(strings.TrimSpace(block.Role)) != role {
			continue
		}
		if normalizeTranscriptBlockText(block.Text) != text {
			continue
		}
		if nextMessageID != "" {
			if currentMessageID := transcriptBlockMetaString(block.Meta, "provider_message_id", "providerMessageID", "message_id"); currentMessageID == nextMessageID {
				return true
			}
		}
		if nextTurnID != "" {
			if currentTurnID := transcriptBlockMetaString(block.Meta, "turn_id", "turnId"); currentTurnID == nextTurnID {
				return true
			}
		}
		currentCreatedAt := transcriptBlockMetaTime(block.Meta, "provider_created_at", "created_at", "createdAt", "timestamp", "ts")
		if !nextCreatedAt.IsZero() && !currentCreatedAt.IsZero() {
			delta := nextCreatedAt.Sub(currentCreatedAt)
			if delta < 0 {
				delta = -delta
			}
			if delta <= transcriptDuplicateTimestampWindow {
				return true
			}
		}
	}
	return false
}

func transcriptRoleSupportsReplayDedupe(role string) bool {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "assistant", "agent", "model", "user", "reasoning":
		return true
	default:
		return false
	}
}

func normalizeTranscriptBlockText(text string) string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	return strings.TrimSpace(text)
}

func transcriptBlockMetaString(meta map[string]any, keys ...string) string {
	if len(meta) == 0 {
		return ""
	}
	for _, key := range keys {
		if value := strings.TrimSpace(asString(meta[key])); value != "" {
			return value
		}
	}
	return ""
}

func transcriptBlockMetaTime(meta map[string]any, keys ...string) time.Time {
	if len(meta) == 0 {
		return time.Time{}
	}
	for _, key := range keys {
		if when := parsePersistedTimestamp(meta[key]); !when.IsZero() {
			return when
		}
	}
	return time.Time{}
}
