package daemon

import (
	"fmt"
	"log"
	"strconv"
	"strings"

	"control/internal/daemon/transcriptadapters"
	"control/internal/daemon/transcriptdomain"
)

type TranscriptProjector interface {
	Apply(event transcriptdomain.TranscriptEvent) bool
	NextRevision() transcriptdomain.RevisionToken
	Snapshot() transcriptdomain.TranscriptSnapshot
	ActiveTurnID() string
}

type transcriptProjector struct {
	sessionID      string
	provider       string
	blocks         []transcriptdomain.Block
	turn           transcriptdomain.TurnState
	activeTurn     string
	last           transcriptdomain.RevisionToken
	dedupePolicy   transcriptdomain.TranscriptDedupePolicy
	decisionLogger transcriptdomain.TranscriptDecisionLogger

	numericMode bool
	nextNumeric uint64
	nextLexical uint64
	lexicalBase string
}

func NewTranscriptProjector(sessionID, provider string, baseRevision transcriptdomain.RevisionToken) TranscriptProjector {
	return NewTranscriptProjectorWithPolicies(sessionID, provider, baseRevision, nil, nil)
}

func NewTranscriptProjectorWithIdentityPolicy(
	sessionID, provider string,
	baseRevision transcriptdomain.RevisionToken,
	identityPolicy transcriptdomain.TranscriptIdentityPolicy,
) TranscriptProjector {
	return NewTranscriptProjectorWithPolicies(
		sessionID,
		provider,
		baseRevision,
		transcriptdomain.NewProjectorTranscriptDedupePolicy(identityPolicy),
		nil,
	)
}

func NewTranscriptProjectorWithPolicies(
	sessionID, provider string,
	baseRevision transcriptdomain.RevisionToken,
	dedupePolicy transcriptdomain.TranscriptDedupePolicy,
	decisionLogger transcriptdomain.TranscriptDecisionLogger,
) TranscriptProjector {
	sessionID = strings.TrimSpace(sessionID)
	provider = strings.TrimSpace(provider)
	if dedupePolicy == nil {
		dedupePolicy = transcriptdomain.NewProjectorTranscriptDedupePolicy(nil)
	}
	if decisionLogger == nil {
		decisionLogger = transcriptProjectorStdDecisionLogger{}
	}
	projection := &transcriptProjector{
		sessionID:      sessionID,
		provider:       provider,
		blocks:         []transcriptdomain.Block{},
		turn:           transcriptdomain.TurnState{State: transcriptdomain.TurnStateIdle},
		dedupePolicy:   dedupePolicy,
		decisionLogger: decisionLogger,
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
		delta := filterDuplicateTranscriptBlocks(p.blocks, event.Delta, p.dedupePolicy, p.decisionLogger, p.sessionID, p.provider)
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
	dedupePolicy transcriptdomain.TranscriptDedupePolicy,
	decisionLogger transcriptdomain.TranscriptDecisionLogger,
	sessionID string,
	provider string,
) []transcriptdomain.Block {
	if len(incoming) == 0 {
		return nil
	}
	if dedupePolicy == nil {
		dedupePolicy = transcriptdomain.NewProjectorTranscriptDedupePolicy(nil)
	}
	if decisionLogger == nil {
		decisionLogger = transcriptProjectorStdDecisionLogger{}
	}
	out := make([]transcriptdomain.Block, 0, len(incoming))
	seen := append([]transcriptdomain.Block(nil), existing...)
	for _, block := range incoming {
		decision := dedupePolicy.ReplayDecision(
			transcriptIdentityBlocksFromDomainBlocks(seen),
			transcriptIdentityBlockFromDomainBlock(block),
		)
		if decision.Action == transcriptdomain.TranscriptDedupeActionDropDuplicate {
			logTranscriptProjectorDedupeDecision(
				decisionLogger,
				"dropped_replay_duplicate",
				decision.Reason,
				sessionID,
				provider,
				block,
				decision.Identity,
			)
			continue
		}
		if decision.Action == transcriptdomain.TranscriptDedupeActionReplaceExisting {
			logTranscriptProjectorDedupeDecision(
				decisionLogger,
				"dropped_replay_duplicate",
				decision.Reason,
				sessionID,
				provider,
				block,
				decision.Identity,
			)
			continue
		}
		out = append(out, block)
		seen = append(seen, block)
		logTranscriptProjectorDedupeDecision(
			decisionLogger,
			"accepted_new",
			decision.Reason,
			sessionID,
			provider,
			block,
			decision.Identity,
		)
	}
	return out
}

func transcriptIdentityBlocksFromDomainBlocks(blocks []transcriptdomain.Block) []transcriptdomain.TranscriptIdentityBlock {
	if len(blocks) == 0 {
		return nil
	}
	out := make([]transcriptdomain.TranscriptIdentityBlock, 0, len(blocks))
	for _, block := range blocks {
		out = append(out, transcriptIdentityBlockFromDomainBlock(block))
	}
	return out
}

func transcriptIdentityBlockFromDomainBlock(block transcriptdomain.Block) transcriptdomain.TranscriptIdentityBlock {
	return transcriptdomain.TranscriptIdentityBlock{
		ID:   strings.TrimSpace(block.ID),
		Role: strings.TrimSpace(block.Role),
		Text: transcriptdomain.PreserveText(block.Text),
		Meta: block.Meta,
	}
}

func logTranscriptProjectorDedupeDecision(
	decisionLogger transcriptdomain.TranscriptDecisionLogger,
	decision string,
	reason string,
	sessionID string,
	provider string,
	block transcriptdomain.Block,
	identity transcriptdomain.MessageIdentity,
) {
	if decisionLogger == nil {
		return
	}
	decisionLogger.LogDecision(transcriptdomain.TranscriptDecisionLogEntry{
		Layer:    "projector",
		Decision: strings.TrimSpace(decision),
		Reason:   strings.TrimSpace(reason),
		Identity: identity,
		Block:    transcriptIdentityBlockFromDomainBlock(block),
		Context: map[string]string{
			"session_id": strings.TrimSpace(sessionID),
			"provider":   strings.TrimSpace(provider),
		},
	})
}

type transcriptProjectorStdDecisionLogger struct{}

func (transcriptProjectorStdDecisionLogger) LogDecision(entry transcriptdomain.TranscriptDecisionLogEntry) {
	log.Printf(
		"transcript_projector_dedupe decision=%s reason=%s session_id=%s provider=%s role=%s block_id=%s provider_message_id=%s provider_item_id=%s turn_id=%s turn_scoped_id=%s",
		strings.TrimSpace(entry.Decision),
		strings.TrimSpace(entry.Reason),
		strings.TrimSpace(entry.Context["session_id"]),
		strings.TrimSpace(entry.Context["provider"]),
		strings.TrimSpace(entry.Block.Role),
		strings.TrimSpace(entry.Block.ID),
		strings.TrimSpace(entry.Identity.ProviderMessageID),
		strings.TrimSpace(entry.Identity.ProviderItemID),
		strings.TrimSpace(entry.Identity.TurnID),
		strings.TrimSpace(entry.Identity.TurnScopedID),
	)
}
