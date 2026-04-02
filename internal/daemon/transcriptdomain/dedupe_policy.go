package transcriptdomain

import "strings"

type TranscriptDedupeAction string

const (
	TranscriptDedupeActionAcceptNew       TranscriptDedupeAction = "accept_new"
	TranscriptDedupeActionAppend          TranscriptDedupeAction = "append"
	TranscriptDedupeActionDropDuplicate   TranscriptDedupeAction = "drop_duplicate"
	TranscriptDedupeActionReplaceExisting TranscriptDedupeAction = "replace_existing"
	TranscriptDedupeActionRejectAmbiguous TranscriptDedupeAction = "reject_ambiguous"
)

type TranscriptDedupeDecision struct {
	Action    TranscriptDedupeAction
	Reason    string
	Index     int
	Identity  MessageIdentity
	Merged    TranscriptIdentityBlock
	Deduped   bool
	Ambiguous bool
}

type TranscriptDedupePolicy interface {
	ReplayDecision(existing []TranscriptIdentityBlock, candidate TranscriptIdentityBlock) TranscriptDedupeDecision
	FinalizedDecision(existing []TranscriptIdentityBlock, candidate TranscriptIdentityBlock) TranscriptDedupeDecision
}

type TranscriptBlockMergePolicy interface {
	Merge(current, candidate TranscriptIdentityBlock, finalized bool, allowTurnFallback bool) (next TranscriptIdentityBlock, changed bool, deduped bool, reason string)
}

type projectorTranscriptDedupePolicy struct {
	identityPolicy TranscriptIdentityPolicy
}

func NewProjectorTranscriptDedupePolicy(identityPolicy TranscriptIdentityPolicy) TranscriptDedupePolicy {
	if identityPolicy == nil {
		identityPolicy = NewDefaultTranscriptIdentityPolicy()
	}
	return projectorTranscriptDedupePolicy{
		identityPolicy: identityPolicy,
	}
}

func (p projectorTranscriptDedupePolicy) ReplayDecision(existing []TranscriptIdentityBlock, candidate TranscriptIdentityBlock) TranscriptDedupeDecision {
	role := strings.ToLower(strings.TrimSpace(candidate.Role))
	if !transcriptRoleSupportsReplayDedupe(role) {
		return TranscriptDedupeDecision{
			Action:   TranscriptDedupeActionAppend,
			Reason:   "unsupported_role",
			Identity: MessageIdentity{},
		}
	}
	identity := p.identityPolicy.Identity(candidate)
	if !identity.HasStableIdentity() {
		return TranscriptDedupeDecision{
			Action:   TranscriptDedupeActionAppend,
			Reason:   "missing_stable_identity",
			Identity: identity,
		}
	}
	foundIdentity := false
	for i := range existing {
		current := existing[i]
		if strings.ToLower(strings.TrimSpace(current.Role)) != role {
			continue
		}
		if !p.identityPolicy.Equivalent(current, candidate) {
			continue
		}
		foundIdentity = true
		if PreserveText(current.Text) == PreserveText(candidate.Text) {
			return TranscriptDedupeDecision{
				Action:   TranscriptDedupeActionDropDuplicate,
				Reason:   "stable_identity_text_match",
				Index:    i,
				Identity: identity,
				Deduped:  true,
			}
		}
	}
	if foundIdentity {
		return TranscriptDedupeDecision{
			Action:   TranscriptDedupeActionAppend,
			Reason:   "stable_identity_text_mismatch",
			Identity: identity,
		}
	}
	return TranscriptDedupeDecision{
		Action:   TranscriptDedupeActionAppend,
		Reason:   "stable_identity_not_found",
		Identity: identity,
	}
}

func (p projectorTranscriptDedupePolicy) FinalizedDecision(existing []TranscriptIdentityBlock, candidate TranscriptIdentityBlock) TranscriptDedupeDecision {
	return p.ReplayDecision(existing, candidate)
}

type ingestorTranscriptDedupePolicy struct {
	identityPolicy TranscriptIdentityPolicy
	mergePolicy    TranscriptBlockMergePolicy
}

func NewIngestorTranscriptDedupePolicy(
	identityPolicy TranscriptIdentityPolicy,
	mergePolicy TranscriptBlockMergePolicy,
) TranscriptDedupePolicy {
	if identityPolicy == nil {
		identityPolicy = NewDefaultTranscriptIdentityPolicy()
	}
	if mergePolicy == nil {
		mergePolicy = NewDefaultTranscriptBlockMergePolicy(identityPolicy)
	}
	return ingestorTranscriptDedupePolicy{
		identityPolicy: identityPolicy,
		mergePolicy:    mergePolicy,
	}
}

func (p ingestorTranscriptDedupePolicy) ReplayDecision(existing []TranscriptIdentityBlock, candidate TranscriptIdentityBlock) TranscriptDedupeDecision {
	candidateIdentity := p.identityPolicy.Identity(candidate)
	if !candidateIdentity.HasStableIdentity() {
		return TranscriptDedupeDecision{
			Action:   TranscriptDedupeActionAppend,
			Reason:   "no_duplicate_identity_match",
			Identity: candidateIdentity,
		}
	}
	for i := len(existing) - 1; i >= 0; i-- {
		if !identityRolesCompatible(existing[i].Role, candidate.Role) {
			continue
		}
		if !p.identityPolicy.Equivalent(existing[i], candidate) {
			continue
		}
		next, changed, deduped, reason := p.mergePolicy.Merge(existing[i], candidate, false, false)
		if !deduped {
			return TranscriptDedupeDecision{
				Action:   TranscriptDedupeActionAppend,
				Reason:   reason,
				Identity: candidateIdentity,
			}
		}
		if changed {
			return TranscriptDedupeDecision{
				Action:   TranscriptDedupeActionReplaceExisting,
				Reason:   reason,
				Index:    i,
				Identity: candidateIdentity,
				Merged:   next,
				Deduped:  true,
			}
		}
		return TranscriptDedupeDecision{
			Action:   TranscriptDedupeActionDropDuplicate,
			Reason:   reason,
			Index:    i,
			Identity: candidateIdentity,
			Deduped:  true,
		}
	}
	return TranscriptDedupeDecision{
		Action:   TranscriptDedupeActionAppend,
		Reason:   "no_duplicate_identity_match",
		Identity: candidateIdentity,
	}
}

func (p ingestorTranscriptDedupePolicy) FinalizedDecision(existing []TranscriptIdentityBlock, candidate TranscriptIdentityBlock) TranscriptDedupeDecision {
	matchIndex, allowTurnFallback, ambiguous, reason, identity := p.findFinalizedCandidate(existing, candidate)
	if ambiguous {
		return TranscriptDedupeDecision{
			Action:    TranscriptDedupeActionRejectAmbiguous,
			Reason:    reason,
			Identity:  identity,
			Ambiguous: true,
		}
	}
	if matchIndex < 0 {
		return TranscriptDedupeDecision{
			Action:   TranscriptDedupeActionAppend,
			Reason:   reason,
			Identity: identity,
		}
	}
	next, changed, deduped, mergeReason := p.mergePolicy.Merge(existing[matchIndex], candidate, true, allowTurnFallback)
	if !deduped {
		return TranscriptDedupeDecision{
			Action:   TranscriptDedupeActionAppend,
			Reason:   mergeReason,
			Identity: identity,
		}
	}
	if changed {
		return TranscriptDedupeDecision{
			Action:   TranscriptDedupeActionReplaceExisting,
			Reason:   mergeReason,
			Index:    matchIndex,
			Identity: identity,
			Merged:   next,
			Deduped:  true,
		}
	}
	return TranscriptDedupeDecision{
		Action:   TranscriptDedupeActionDropDuplicate,
		Reason:   mergeReason,
		Index:    matchIndex,
		Identity: identity,
		Deduped:  true,
	}
}

func (p ingestorTranscriptDedupePolicy) findFinalizedCandidate(
	existing []TranscriptIdentityBlock,
	candidate TranscriptIdentityBlock,
) (index int, allowTurnFallback bool, ambiguous bool, reason string, identity MessageIdentity) {
	if len(existing) == 0 {
		return -1, false, false, "no_existing_blocks", p.identityPolicy.Identity(candidate)
	}
	identity = p.identityPolicy.Identity(candidate)
	if identity.HasStableIdentity() {
		stableMatches := make([]int, 0, len(existing))
		for i := len(existing) - 1; i >= 0; i-- {
			if !identityRolesCompatible(existing[i].Role, candidate.Role) {
				continue
			}
			if p.identityPolicy.Equivalent(existing[i], candidate) {
				stableMatches = append(stableMatches, i)
			}
		}
		switch len(stableMatches) {
		case 1:
			return stableMatches[0], false, false, "stable_identity_match", identity
		case 0:
			return -1, false, false, "stable_identity_not_found", identity
		default:
			return -1, false, true, "stable_identity_ambiguous", identity
		}
	}
	candidateTurnID := strings.TrimSpace(candidate.TurnID)
	if candidateTurnID == "" {
		return -1, false, false, "missing_turn_fallback_identity", identity
	}
	turnMatches := make([]int, 0, len(existing))
	for i := len(existing) - 1; i >= 0; i-- {
		if !identityRolesCompatible(existing[i].Role, candidate.Role) {
			continue
		}
		if strings.TrimSpace(existing[i].TurnID) == candidateTurnID {
			turnMatches = append(turnMatches, i)
		}
	}
	switch len(turnMatches) {
	case 1:
		return turnMatches[0], true, false, "turn_fallback_single_candidate", identity
	case 0:
		return -1, false, false, "turn_fallback_no_candidate", identity
	default:
		return -1, false, true, "turn_fallback_ambiguous", identity
	}
}

type defaultTranscriptBlockMergePolicy struct {
	identityPolicy TranscriptIdentityPolicy
}

func NewDefaultTranscriptBlockMergePolicy(identityPolicy TranscriptIdentityPolicy) TranscriptBlockMergePolicy {
	if identityPolicy == nil {
		identityPolicy = NewDefaultTranscriptIdentityPolicy()
	}
	return defaultTranscriptBlockMergePolicy{
		identityPolicy: identityPolicy,
	}
}

func (p defaultTranscriptBlockMergePolicy) Merge(
	current,
	candidate TranscriptIdentityBlock,
	finalized bool,
	allowTurnFallback bool,
) (TranscriptIdentityBlock, bool, bool, string) {
	next := current
	changed := false

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
	if next.Meta == nil && len(candidate.Meta) > 0 {
		next.Meta = cloneTranscriptMeta(candidate.Meta)
		changed = true
	}

	canFinalizeReplace := p.identityPolicy.CanFinalizeReplace(current, candidate) || allowTurnFallback
	if finalized && !canFinalizeReplace {
		return next, changed, false, "finalized_replace_rejected_by_policy"
	}

	currentText := PreserveText(current.Text)
	candidateText := PreserveText(candidate.Text)
	if IsSemanticallyEmpty(candidateText) {
		if finalized && canFinalizeReplace {
			return next, changed, true, "finalized_candidate_empty"
		}
		return next, changed, false, "candidate_text_empty"
	}
	if IsSemanticallyEmpty(currentText) {
		next.Text = candidate.Text
		return next, true, true, "candidate_filled_empty_current"
	}
	if candidateText == currentText {
		return next, changed, true, "text_exact_match"
	}
	// Incremental streaming deltas carry only new text, not a
	// cumulative replay. Skip substring-containment checks that
	// would incorrectly drop small chunks (spaces, backticks, short
	// words) that happen to appear in the accumulated text.
	// Finalized blocks are excluded: even when their Kind looks
	// incremental (e.g. item/completed with type=agentMessage),
	// finalized text must participate in superset replacement.
	if !finalized && IsIncrementalDeltaKind(candidate.Kind) {
		return next, changed, false, "incremental_delta_diverged"
	}
	if strings.Contains(candidateText, currentText) && len(candidateText) >= len(currentText) {
		next.Text = candidate.Text
		return next, true, true, "candidate_text_superset"
	}
	if strings.Contains(currentText, candidateText) {
		return next, changed, true, "current_text_superset"
	}

	if finalized {
		if len(candidateText) >= len(currentText) {
			next.Text = candidate.Text
			return next, true, true, "finalized_replace_longer_candidate"
		}
		return next, changed, true, "finalized_keep_current_shorter_candidate"
	}
	return next, changed, false, "identity_match_text_diverged"
}

func transcriptRoleSupportsReplayDedupe(role string) bool {
	switch strings.ToLower(strings.TrimSpace(role)) {
	case "assistant", "agent", "model", "user", "reasoning":
		return true
	default:
		return false
	}
}

func cloneTranscriptMeta(meta map[string]any) map[string]any {
	if len(meta) == 0 {
		return nil
	}
	out := make(map[string]any, len(meta))
	for key, value := range meta {
		out[key] = value
	}
	return out
}
