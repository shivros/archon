package app

import (
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
}

func NewDefaultTranscriptIngestor() TranscriptIngestor {
	return NewTranscriptIngestorWithAdapterRegistry(nil)
}

func NewTranscriptIngestorWithAdapterRegistry(registry TranscriptEventAdapterRegistry) TranscriptIngestor {
	if registry == nil {
		registry = NewDefaultTranscriptEventAdapterRegistry()
	}
	return defaultTranscriptIngestor{adapterRegistry: registry}
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
		nextBlocks, changed, dedupeHits := applyTranscriptDeltaWithFinalizationDedupe(state.Blocks, event.Delta, normalized.FinalizedDeltaBlockIndexes)
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

func applyTranscriptDeltaWithFinalizationDedupe(existing []ChatBlock, delta []transcriptdomain.Block, finalizedIndexes map[int]struct{}) ([]ChatBlock, bool, int) {
	if len(delta) == 0 {
		return existing, false, 0
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
		idx := findTranscriptDedupeCandidateIndex(out, candidate)
		if idx >= 0 {
			next, blockChanged, deduped := mergeTranscriptDedupeCandidate(out[idx], candidate, finalized)
			if blockChanged {
				out[idx] = next
				changed = true
			}
			if deduped {
				dedupeHits++
				continue
			}
		}
		out = append(out, candidate)
		changed = true
	}
	if !changed {
		return out, false, dedupeHits
	}
	out = coalesceAdjacentTranscriptChatBlocks(out)
	return out, true, dedupeHits
}

func findTranscriptDedupeCandidateIndex(existing []ChatBlock, candidate ChatBlock) int {
	if len(existing) == 0 {
		return -1
	}
	for i := len(existing) - 1; i >= 0; i-- {
		if existing[i].Role != candidate.Role {
			continue
		}
		if transcriptBlocksShareIdentity(existing[i], candidate) {
			return i
		}
	}
	candidateTurnID := strings.TrimSpace(candidate.TurnID)
	if candidateTurnID == "" {
		return -1
	}
	for i := len(existing) - 1; i >= 0; i-- {
		if existing[i].Role != candidate.Role {
			continue
		}
		if strings.TrimSpace(existing[i].TurnID) == candidateTurnID {
			return i
		}
	}
	return -1
}

func transcriptBlocksShareIdentity(current, candidate ChatBlock) bool {
	currentProviderID := strings.TrimSpace(current.ProviderMessageID)
	candidateProviderID := strings.TrimSpace(candidate.ProviderMessageID)
	if currentProviderID != "" && candidateProviderID != "" && currentProviderID == candidateProviderID {
		return true
	}
	currentID := strings.TrimSpace(current.ID)
	candidateID := strings.TrimSpace(candidate.ID)
	if currentID != "" && candidateID != "" && currentID == candidateID {
		return true
	}
	currentTurnID := strings.TrimSpace(current.TurnID)
	candidateTurnID := strings.TrimSpace(candidate.TurnID)
	return currentTurnID != "" && candidateTurnID != "" && currentTurnID == candidateTurnID
}

func mergeTranscriptDedupeCandidate(current ChatBlock, candidate ChatBlock, finalized bool) (ChatBlock, bool, bool) {
	next := current
	changed := false
	normalizedCurrent := normalizeTranscriptMessageText(current.Text)
	normalizedCandidate := normalizeTranscriptMessageText(candidate.Text)

	if strings.TrimSpace(next.ProviderMessageID) == "" && strings.TrimSpace(candidate.ProviderMessageID) != "" {
		next.ProviderMessageID = strings.TrimSpace(candidate.ProviderMessageID)
		changed = true
	}
	if strings.TrimSpace(next.TurnID) == "" && strings.TrimSpace(candidate.TurnID) != "" {
		next.TurnID = strings.TrimSpace(candidate.TurnID)
		changed = true
	}
	if next.CreatedAt.IsZero() && !candidate.CreatedAt.IsZero() {
		next.CreatedAt = candidate.CreatedAt
		changed = true
	}

	if normalizedCandidate == "" {
		return next, changed, false
	}
	if normalizedCurrent == "" {
		next.Text = candidate.Text
		return next, true, true
	}
	if normalizedCandidate == normalizedCurrent {
		return next, changed, true
	}
	if strings.Contains(normalizedCandidate, normalizedCurrent) && len(normalizedCandidate) >= len(normalizedCurrent) {
		next.Text = candidate.Text
		return next, true, true
	}
	if strings.Contains(normalizedCurrent, normalizedCandidate) {
		return next, changed, true
	}

	if finalized {
		if len(normalizedCandidate) >= len(normalizedCurrent) {
			next.Text = candidate.Text
			return next, true, true
		}
		return next, changed, true
	}
	return next, changed, false
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
