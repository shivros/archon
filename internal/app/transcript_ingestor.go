package app

import (
	"log"
	"strings"

	"control/internal/daemon/transcriptdomain"
)

type TranscriptEventCategory string

const (
	transcriptEventCategoryDelta            TranscriptEventCategory = "delta"
	transcriptEventCategoryFinalizedMessage TranscriptEventCategory = "finalized_message"
	transcriptEventCategorySnapshotReplace  TranscriptEventCategory = "snapshot_replace"
	transcriptEventCategoryControl          TranscriptEventCategory = "control"
)

type TranscriptIngestState struct {
	Blocks   []ChatBlock
	Revision transcriptdomain.RevisionToken
}

type TranscriptSnapshotApplyResult struct {
	State   TranscriptIngestState
	Changed bool
	Applied bool
}

type TranscriptEventApplyResult struct {
	State               TranscriptIngestState
	Changed             bool
	Signal              bool
	Content             bool
	Completion          *TranscriptCompletionSignal
	Rewind              bool
	Category            TranscriptEventCategory
	FinalizedDedupeHits int
}

type TranscriptIngestor interface {
	ApplySnapshot(state TranscriptIngestState, snapshot transcriptdomain.TranscriptSnapshot, authoritative bool) TranscriptSnapshotApplyResult
	ApplyEvent(state TranscriptIngestState, event transcriptdomain.TranscriptEvent, firstEvent bool) TranscriptEventApplyResult
}

type defaultTranscriptIngestor struct {
	adapterRegistry TranscriptEventAdapterRegistry
	dedupePolicy    transcriptdomain.TranscriptDedupePolicy
	decisionLogger  transcriptdomain.TranscriptDecisionLogger
}

func NewDefaultTranscriptIngestor() TranscriptIngestor {
	return NewTranscriptIngestorWithAdapterRegistry(nil)
}

func NewTranscriptIngestorWithAdapterRegistry(registry TranscriptEventAdapterRegistry) TranscriptIngestor {
	return NewTranscriptIngestorWithPolicies(registry, nil)
}

func NewTranscriptIngestorWithPolicies(
	registry TranscriptEventAdapterRegistry,
	identityPolicy transcriptdomain.TranscriptIdentityPolicy,
) TranscriptIngestor {
	return NewTranscriptIngestorWithDependencies(registry, nil, nil, identityPolicy)
}

func NewTranscriptIngestorWithDependencies(
	registry TranscriptEventAdapterRegistry,
	dedupePolicy transcriptdomain.TranscriptDedupePolicy,
	decisionLogger transcriptdomain.TranscriptDecisionLogger,
	identityPolicy transcriptdomain.TranscriptIdentityPolicy,
) TranscriptIngestor {
	if registry == nil {
		registry = NewDefaultTranscriptEventAdapterRegistry()
	}
	if dedupePolicy == nil {
		dedupePolicy = transcriptdomain.NewIngestorTranscriptDedupePolicy(identityPolicy, nil)
	}
	if decisionLogger == nil {
		decisionLogger = transcriptIngestorStdDecisionLogger{}
	}
	return defaultTranscriptIngestor{
		adapterRegistry: registry,
		dedupePolicy:    dedupePolicy,
		decisionLogger:  decisionLogger,
	}
}

func (defaultTranscriptIngestor) ApplySnapshot(state TranscriptIngestState, snapshot transcriptdomain.TranscriptSnapshot, authoritative bool) TranscriptSnapshotApplyResult {
	if !authoritative && !isTranscriptRevisionNewer(snapshot.Revision, state.Revision) {
		return TranscriptSnapshotApplyResult{State: state, Changed: false, Applied: false}
	}
	blocks := transcriptBlocksToChatBlocks(snapshot.Blocks)
	changed := !chatBlocksEqual(blocks, state.Blocks) || strings.TrimSpace(snapshot.Revision.String()) != strings.TrimSpace(state.Revision.String())
	return TranscriptSnapshotApplyResult{
		State: TranscriptIngestState{
			Blocks:   blocks,
			Revision: snapshot.Revision,
		},
		Changed: changed,
		Applied: true,
	}
}

func (ingestor defaultTranscriptIngestor) ApplyEvent(state TranscriptIngestState, event transcriptdomain.TranscriptEvent, firstEvent bool) TranscriptEventApplyResult {
	registry := ingestor.adapterRegistry
	if registry == nil {
		registry = NewDefaultTranscriptEventAdapterRegistry()
	}
	adapter := registry.AdapterForProvider(event.Provider)
	normalized := adapter.Normalize(event)
	event = normalized.Event
	result := TranscriptEventApplyResult{State: state, Category: transcriptEventCategoryControl}
	switch event.Kind {
	case transcriptdomain.TranscriptEventHeartbeat:
		return result
	case transcriptdomain.TranscriptEventStreamStatus:
		if firstEvent && isTranscriptRevisionRewind(state.Revision, event.Revision) {
			result.Signal = true
			result.Rewind = true
			return result
		}
		if !isTranscriptRevisionNewer(event.Revision, state.Revision) {
			return result
		}
		result.State.Revision = event.Revision
		if event.StreamStatus == transcriptdomain.StreamStatusReady {
			result.Signal = true
		}
		return result
	case transcriptdomain.TranscriptEventReplace:
		if firstEvent && isTranscriptRevisionRewind(state.Revision, event.Revision) {
			result.Signal = true
			result.Rewind = true
			return result
		}
		if !isTranscriptRevisionNewer(event.Revision, state.Revision) {
			return result
		}
		if event.Replace == nil {
			result.State.Revision = event.Revision
			result.Signal = true
			return result
		}
		nextBlocks := transcriptBlocksToChatBlocks(event.Replace.Blocks)
		result.Changed = !chatBlocksEqual(nextBlocks, state.Blocks)
		result.State.Blocks = nextBlocks
		result.State.Revision = event.Revision
		result.Signal = true
		result.Content = transcriptBlocksContainUserRelevantContent(event.Replace.Blocks)
		result.Category = transcriptEventCategorySnapshotReplace
		return result
	case transcriptdomain.TranscriptEventDelta:
		if firstEvent && isTranscriptRevisionRewind(state.Revision, event.Revision) {
			result.Signal = true
			result.Rewind = true
			return result
		}
		if !isTranscriptRevisionNewer(event.Revision, state.Revision) {
			return result
		}
		result.State.Revision = event.Revision
		nextBlocks, changed, dedupeHits := applyTranscriptDeltaWithFinalizationDedupe(
			state.Blocks,
			event.Delta,
			normalized.FinalizedDeltaBlockIndexes,
			ingestor.dedupePolicy,
			ingestor.decisionLogger,
		)
		if changed {
			result.State.Blocks = nextBlocks
			result.Changed = true
		}
		result.Signal = true
		result.Content = transcriptBlocksContainUserRelevantContent(event.Delta)
		result.Category = normalized.Category
		if normalized.Category == transcriptEventCategoryFinalizedMessage {
			result.FinalizedDedupeHits = dedupeHits
		}
		return result
	case transcriptdomain.TranscriptEventTurnStarted,
		transcriptdomain.TranscriptEventTurnCompleted,
		transcriptdomain.TranscriptEventTurnFailed,
		transcriptdomain.TranscriptEventApprovalPending,
		transcriptdomain.TranscriptEventApprovalResolved:
		if firstEvent && isTranscriptRevisionRewind(state.Revision, event.Revision) {
			result.Signal = true
			result.Rewind = true
			return result
		}
		if !isTranscriptRevisionNewer(event.Revision, state.Revision) {
			return result
		}
		result.State.Revision = event.Revision
		result.Signal = true
		if event.Kind == transcriptdomain.TranscriptEventTurnCompleted || event.Kind == transcriptdomain.TranscriptEventTurnFailed {
			turnID := ""
			if event.Turn != nil {
				turnID = strings.TrimSpace(event.Turn.TurnID)
			}
			result.Completion = &TranscriptCompletionSignal{TurnID: turnID}
		}
		return result
	default:
		return result
	}
}

func isTranscriptRevisionRewind(current, candidate transcriptdomain.RevisionToken) bool {
	if current.IsZero() || candidate.IsZero() {
		return false
	}
	newer, err := transcriptdomain.IsRevisionNewer(current, candidate)
	if err != nil {
		return false
	}
	return newer
}

func applyTranscriptDeltaWithFinalizationDedupe(
	existing []ChatBlock,
	delta []transcriptdomain.Block,
	finalizedIndexes map[int]struct{},
	dedupePolicy transcriptdomain.TranscriptDedupePolicy,
	decisionLogger transcriptdomain.TranscriptDecisionLogger,
) ([]ChatBlock, bool, int) {
	if len(delta) == 0 {
		return existing, false, 0
	}
	if dedupePolicy == nil {
		dedupePolicy = transcriptdomain.NewIngestorTranscriptDedupePolicy(nil, nil)
	}
	if decisionLogger == nil {
		decisionLogger = transcriptIngestorStdDecisionLogger{}
	}
	out := append([]ChatBlock(nil), existing...)
	changed := false
	dedupeHits := 0
	for i, raw := range delta {
		chatBlocks := transcriptBlocksToChatBlocks([]transcriptdomain.Block{raw})
		if len(chatBlocks) == 0 {
			continue
		}
		candidate := chatBlocks[0]
		_, finalized := finalizedIndexes[i]
		existingIdentityBlocks := transcriptIdentityBlocksFromChatBlocks(out)
		candidateIdentityBlock := transcriptIdentityBlockFromChatBlock(candidate)
		var decision transcriptdomain.TranscriptDedupeDecision
		if finalized {
			decision = dedupePolicy.FinalizedDecision(existingIdentityBlocks, candidateIdentityBlock)
		} else {
			decision = dedupePolicy.ReplayDecision(existingIdentityBlocks, candidateIdentityBlock)
		}
		switch decision.Action {
		case transcriptdomain.TranscriptDedupeActionDropDuplicate:
			if finalized && decision.Deduped {
				dedupeHits++
				logTranscriptIngestorDecision(decisionLogger, "finalized_replaced", decision.Reason, candidate, decision.Identity)
			} else {
				logTranscriptIngestorDecision(decisionLogger, "dropped_replay_duplicate", decision.Reason, candidate, decision.Identity)
			}
			continue
		case transcriptdomain.TranscriptDedupeActionReplaceExisting:
			idx := decision.Index
			if idx >= 0 && idx < len(out) {
				next := chatBlockFromTranscriptIdentityBlock(out[idx], decision.Merged)
				if out[idx] != next {
					out[idx] = next
					changed = true
				}
				if finalized && decision.Deduped {
					dedupeHits++
					logTranscriptIngestorDecision(decisionLogger, "finalized_replaced", decision.Reason, candidate, decision.Identity)
				} else {
					logTranscriptIngestorDecision(decisionLogger, "dropped_replay_duplicate", decision.Reason, candidate, decision.Identity)
				}
				continue
			}
			out = append(out, candidate)
			changed = true
			logTranscriptIngestorDecision(decisionLogger, "accepted_new", "invalid_replacement_index", candidate, decision.Identity)
			continue
		case transcriptdomain.TranscriptDedupeActionRejectAmbiguous:
			out = append(out, candidate)
			changed = true
			logTranscriptIngestorDecision(decisionLogger, "rejected_ambiguous_identity", decision.Reason, candidate, decision.Identity)
			continue
		default:
			out = append(out, candidate)
			changed = true
			logTranscriptIngestorDecision(decisionLogger, "accepted_new", decision.Reason, candidate, decision.Identity)
			continue
		}
	}
	if !changed {
		return out, false, dedupeHits
	}
	out = coalesceAdjacentTranscriptChatBlocks(out)
	return out, true, dedupeHits
}

func findTranscriptDedupeCandidateIndex(existing []ChatBlock, candidate ChatBlock) int {
	return findTranscriptReplayDuplicateCandidateIndex(existing, candidate, transcriptdomain.NewDefaultTranscriptIdentityPolicy())
}

func findTranscriptReplayDuplicateCandidateIndex(
	existing []ChatBlock,
	candidate ChatBlock,
	identityPolicy transcriptdomain.TranscriptIdentityPolicy,
) int {
	if len(existing) == 0 {
		return -1
	}
	if identityPolicy == nil {
		identityPolicy = transcriptdomain.NewDefaultTranscriptIdentityPolicy()
	}
	candidateIdentity := identityPolicy.Identity(transcriptIdentityBlockFromChatBlock(candidate))
	if !candidateIdentity.HasStableIdentity() {
		return -1
	}
	candidateIdentityBlock := transcriptIdentityBlockFromChatBlock(candidate)
	for i := len(existing) - 1; i >= 0; i-- {
		if existing[i].Role != candidate.Role {
			continue
		}
		if identityPolicy.Equivalent(transcriptIdentityBlockFromChatBlock(existing[i]), candidateIdentityBlock) {
			return i
		}
	}
	return -1
}

func transcriptBlocksShareIdentity(current, candidate ChatBlock) bool {
	policy := transcriptdomain.NewDefaultTranscriptIdentityPolicy()
	return policy.Equivalent(transcriptIdentityBlockFromChatBlock(current), transcriptIdentityBlockFromChatBlock(candidate))
}

func mergeTranscriptDedupeCandidate(current ChatBlock, candidate ChatBlock, finalized bool) (ChatBlock, bool, bool) {
	next, changed, deduped, _ := mergeTranscriptDedupeCandidateWithPolicy(
		current,
		candidate,
		finalized,
		false,
		transcriptdomain.NewDefaultTranscriptIdentityPolicy(),
	)
	return next, changed, deduped
}

func mergeTranscriptDedupeCandidateWithPolicy(
	current ChatBlock,
	candidate ChatBlock,
	finalized bool,
	allowTurnFallback bool,
	identityPolicy transcriptdomain.TranscriptIdentityPolicy,
) (ChatBlock, bool, bool, string) {
	mergePolicy := transcriptdomain.NewDefaultTranscriptBlockMergePolicy(identityPolicy)
	nextIdentity, changed, deduped, reason := mergePolicy.Merge(
		transcriptIdentityBlockFromChatBlock(current),
		transcriptIdentityBlockFromChatBlock(candidate),
		finalized,
		allowTurnFallback,
	)
	return chatBlockFromTranscriptIdentityBlock(current, nextIdentity), changed, deduped, reason
}

type transcriptFinalizedReplacementCandidate struct {
	Index             int
	Reason            string
	Ambiguous         bool
	AllowTurnFallback bool
}

func findTranscriptFinalizedReplacementCandidate(
	existing []ChatBlock,
	candidate ChatBlock,
	identityPolicy transcriptdomain.TranscriptIdentityPolicy,
) transcriptFinalizedReplacementCandidate {
	dedupePolicy := transcriptdomain.NewIngestorTranscriptDedupePolicy(identityPolicy, nil)
	decision := dedupePolicy.FinalizedDecision(
		transcriptIdentityBlocksFromChatBlocks(existing),
		transcriptIdentityBlockFromChatBlock(candidate),
	)
	match := transcriptFinalizedReplacementCandidate{
		Index:     -1,
		Reason:    decision.Reason,
		Ambiguous: decision.Action == transcriptdomain.TranscriptDedupeActionRejectAmbiguous,
	}
	if decision.Action == transcriptdomain.TranscriptDedupeActionDropDuplicate ||
		decision.Action == transcriptdomain.TranscriptDedupeActionReplaceExisting {
		match.Index = decision.Index
		if strings.HasPrefix(strings.TrimSpace(decision.Reason), "turn_fallback_") {
			match.AllowTurnFallback = true
		}
	}
	return match
}

func transcriptIdentityBlockFromChatBlock(block ChatBlock) transcriptdomain.TranscriptIdentityBlock {
	return transcriptdomain.TranscriptIdentityBlock{
		ID:                strings.TrimSpace(block.ID),
		Role:              strings.TrimSpace(string(block.Role)),
		Text:              transcriptdomain.PreserveText(block.Text),
		TurnID:            strings.TrimSpace(block.TurnID),
		ProviderMessageID: strings.TrimSpace(block.ProviderMessageID),
		CreatedAt:         block.CreatedAt,
	}
}

func logTranscriptIngestorDecision(
	decisionLogger transcriptdomain.TranscriptDecisionLogger,
	decision string,
	reason string,
	block ChatBlock,
	identity transcriptdomain.MessageIdentity,
) {
	if decisionLogger == nil {
		return
	}
	decisionLogger.LogDecision(transcriptdomain.TranscriptDecisionLogEntry{
		Layer:    "ingestor",
		Decision: strings.TrimSpace(decision),
		Reason:   strings.TrimSpace(reason),
		Identity: identity,
		Block:    transcriptIdentityBlockFromChatBlock(block),
	})
}

type transcriptIngestorStdDecisionLogger struct{}

func (transcriptIngestorStdDecisionLogger) LogDecision(entry transcriptdomain.TranscriptDecisionLogEntry) {
	log.Printf(
		"transcript_ingestor_dedupe decision=%s reason=%s role=%s block_id=%s provider_message_id=%s provider_item_id=%s turn_id=%s turn_scoped_id=%s",
		strings.TrimSpace(entry.Decision),
		strings.TrimSpace(entry.Reason),
		strings.TrimSpace(entry.Block.Role),
		strings.TrimSpace(entry.Block.ID),
		strings.TrimSpace(entry.Identity.ProviderMessageID),
		strings.TrimSpace(entry.Identity.ProviderItemID),
		strings.TrimSpace(entry.Identity.TurnID),
		strings.TrimSpace(entry.Identity.TurnScopedID),
	)
}

func transcriptIdentityBlocksFromChatBlocks(blocks []ChatBlock) []transcriptdomain.TranscriptIdentityBlock {
	if len(blocks) == 0 {
		return nil
	}
	out := make([]transcriptdomain.TranscriptIdentityBlock, 0, len(blocks))
	for _, block := range blocks {
		out = append(out, transcriptIdentityBlockFromChatBlock(block))
	}
	return out
}

func chatBlockFromTranscriptIdentityBlock(current ChatBlock, block transcriptdomain.TranscriptIdentityBlock) ChatBlock {
	next := current
	if id := strings.TrimSpace(block.ID); id != "" {
		next.ID = id
	}
	if role := strings.TrimSpace(block.Role); role != "" {
		next.Role = ChatRole(role)
	}
	next.Text = block.Text
	next.TurnID = strings.TrimSpace(block.TurnID)
	next.ProviderMessageID = strings.TrimSpace(block.ProviderMessageID)
	next.CreatedAt = block.CreatedAt
	return next
}

func chatBlocksEqual(left, right []ChatBlock) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}
