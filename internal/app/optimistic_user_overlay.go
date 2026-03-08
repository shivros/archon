package app

import (
	"sort"
	"strings"
	"time"
)

const (
	optimisticUserTimeMatchWindow   = 2 * time.Minute
	failedOptimisticRetentionWindow = 15 * time.Minute
)

type OptimisticSendOverlayEntry struct {
	Token      int
	SessionID  string
	HeaderLine int
	Text       string
	CreatedAt  time.Time
	Status     ChatStatus
	TurnID     string
}

type OptimisticReconcilePolicy interface {
	IsResolved(entry OptimisticSendOverlayEntry, canonical []ChatBlock) bool
}

type FailedOptimisticRetentionPolicy interface {
	Retain(entry OptimisticSendOverlayEntry, now time.Time) bool
}

type OptimisticOverlayResult struct {
	Blocks         []ChatBlock
	ResolvedTokens []int
}

type OptimisticOverlayService interface {
	ComposeVisible(sessionID string, canonical []ChatBlock, entries []OptimisticSendOverlayEntry, now time.Time) OptimisticOverlayResult
}

type strictIdentityOptimisticReconcilePolicy struct{}

func NewStrictIdentityOptimisticReconcilePolicy() OptimisticReconcilePolicy {
	return strictIdentityOptimisticReconcilePolicy{}
}

func (strictIdentityOptimisticReconcilePolicy) IsResolved(entry OptimisticSendOverlayEntry, canonical []ChatBlock) bool {
	turnID := strings.TrimSpace(entry.TurnID)
	if turnID == "" {
		return false
	}
	for _, block := range canonical {
		if block.Role != ChatRoleUser {
			continue
		}
		if strings.TrimSpace(block.TurnID) == turnID {
			return true
		}
	}
	return false
}

type heuristicOptimisticReconcilePolicy struct{}

func NewHeuristicOptimisticReconcilePolicy() OptimisticReconcilePolicy {
	return heuristicOptimisticReconcilePolicy{}
}

func (heuristicOptimisticReconcilePolicy) IsResolved(entry OptimisticSendOverlayEntry, canonical []ChatBlock) bool {
	if strings.TrimSpace(entry.Text) == "" {
		return false
	}
	normalizedText := normalizeTranscriptMessageText(entry.Text)
	for i := range canonical {
		block := canonical[i]
		if block.Role != ChatRoleUser {
			continue
		}
		if normalizeTranscriptMessageText(block.Text) != normalizedText {
			continue
		}
		if !entry.CreatedAt.IsZero() && !block.CreatedAt.IsZero() {
			delta := block.CreatedAt.Sub(entry.CreatedAt)
			if delta < 0 {
				delta = -delta
			}
			if delta <= optimisticUserTimeMatchWindow {
				return true
			}
		}
		if entry.CreatedAt.IsZero() || block.CreatedAt.IsZero() {
			if isRecentTranscriptMessageIndex(i, len(canonical)) {
				return true
			}
		}
	}
	return false
}

type chainedOptimisticReconcilePolicy struct {
	strict    OptimisticReconcilePolicy
	heuristic OptimisticReconcilePolicy
}

func NewDefaultOptimisticReconcilePolicy() OptimisticReconcilePolicy {
	return chainedOptimisticReconcilePolicy{
		strict:    NewStrictIdentityOptimisticReconcilePolicy(),
		heuristic: NewHeuristicOptimisticReconcilePolicy(),
	}
}

func (p chainedOptimisticReconcilePolicy) IsResolved(entry OptimisticSendOverlayEntry, canonical []ChatBlock) bool {
	if p.strict == nil {
		p.strict = NewStrictIdentityOptimisticReconcilePolicy()
	}
	if p.heuristic == nil {
		p.heuristic = NewHeuristicOptimisticReconcilePolicy()
	}
	turnID := strings.TrimSpace(entry.TurnID)
	if turnID != "" {
		// Identity is authoritative; do not fallback to heuristics to avoid false prune.
		return p.strict.IsResolved(entry, canonical)
	}
	if p.strict.IsResolved(entry, canonical) {
		return true
	}
	return p.heuristic.IsResolved(entry, canonical)
}

type timeWindowFailedOptimisticRetentionPolicy struct {
	window time.Duration
}

func NewDefaultFailedOptimisticRetentionPolicy() FailedOptimisticRetentionPolicy {
	return timeWindowFailedOptimisticRetentionPolicy{window: failedOptimisticRetentionWindow}
}

func (p timeWindowFailedOptimisticRetentionPolicy) Retain(entry OptimisticSendOverlayEntry, now time.Time) bool {
	if entry.Status != ChatStatusFailed {
		return true
	}
	if p.window <= 0 {
		return false
	}
	if entry.CreatedAt.IsZero() {
		return true
	}
	return now.Sub(entry.CreatedAt) <= p.window
}

type defaultOptimisticOverlayService struct {
	reconcile OptimisticReconcilePolicy
	retention FailedOptimisticRetentionPolicy
}

func NewDefaultOptimisticOverlayService(
	reconcile OptimisticReconcilePolicy,
	retention FailedOptimisticRetentionPolicy,
) OptimisticOverlayService {
	if reconcile == nil {
		reconcile = NewDefaultOptimisticReconcilePolicy()
	}
	if retention == nil {
		retention = NewDefaultFailedOptimisticRetentionPolicy()
	}
	return defaultOptimisticOverlayService{
		reconcile: reconcile,
		retention: retention,
	}
}

func (s defaultOptimisticOverlayService) ComposeVisible(
	sessionID string,
	canonical []ChatBlock,
	entries []OptimisticSendOverlayEntry,
	now time.Time,
) OptimisticOverlayResult {
	result := OptimisticOverlayResult{
		Blocks: append([]ChatBlock(nil), canonical...),
	}
	if len(entries) == 0 {
		return result
	}
	if s.reconcile == nil {
		s.reconcile = NewDefaultOptimisticReconcilePolicy()
	}
	if s.retention == nil {
		s.retention = NewDefaultFailedOptimisticRetentionPolicy()
	}
	sessionID = strings.TrimSpace(sessionID)
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].Token < entries[j].Token
	})
	for _, entry := range entries {
		if strings.TrimSpace(entry.SessionID) != sessionID {
			continue
		}
		if strings.TrimSpace(entry.Text) == "" {
			result.ResolvedTokens = append(result.ResolvedTokens, entry.Token)
			continue
		}
		if s.reconcile.IsResolved(entry, result.Blocks) {
			result.ResolvedTokens = append(result.ResolvedTokens, entry.Token)
			continue
		}
		if !s.retention.Retain(entry, now) {
			result.ResolvedTokens = append(result.ResolvedTokens, entry.Token)
			continue
		}
		insertAt := len(result.Blocks)
		if entry.HeaderLine >= 0 && entry.HeaderLine <= len(result.Blocks) {
			insertAt = entry.HeaderLine
		}
		overlay := ChatBlock{
			Role:      ChatRoleUser,
			Text:      entry.Text,
			Status:    entry.Status,
			CreatedAt: entry.CreatedAt,
			TurnID:    strings.TrimSpace(entry.TurnID),
		}
		result.Blocks = insertChatBlockAt(result.Blocks, overlay, insertAt)
	}
	return result
}

func insertChatBlockAt(blocks []ChatBlock, block ChatBlock, index int) []ChatBlock {
	if index < 0 {
		index = 0
	}
	if index >= len(blocks) {
		return append(blocks, block)
	}
	out := make([]ChatBlock, 0, len(blocks)+1)
	out = append(out, blocks[:index]...)
	out = append(out, block)
	out = append(out, blocks[index:]...)
	return out
}

func WithOptimisticOverlayService(service OptimisticOverlayService) ModelOption {
	return func(m *Model) {
		if m == nil {
			return
		}
		m.optimisticOverlayService = service
	}
}

func WithOptimisticReconcilePolicy(policy OptimisticReconcilePolicy) ModelOption {
	return func(m *Model) {
		if m == nil {
			return
		}
		m.optimisticReconcilePolicy = policy
	}
}

func WithFailedOptimisticRetentionPolicy(policy FailedOptimisticRetentionPolicy) ModelOption {
	return func(m *Model) {
		if m == nil {
			return
		}
		m.failedOptimisticRetentionPolicy = policy
	}
}

func (m *Model) optimisticOverlayServiceOrDefault() OptimisticOverlayService {
	if m == nil {
		return NewDefaultOptimisticOverlayService(nil, nil)
	}
	if m.optimisticOverlayService != nil {
		return m.optimisticOverlayService
	}
	return NewDefaultOptimisticOverlayService(
		m.optimisticReconcilePolicyOrDefault(),
		m.failedOptimisticRetentionPolicyOrDefault(),
	)
}

func (m *Model) optimisticReconcilePolicyOrDefault() OptimisticReconcilePolicy {
	if m == nil || m.optimisticReconcilePolicy == nil {
		return NewDefaultOptimisticReconcilePolicy()
	}
	return m.optimisticReconcilePolicy
}

func (m *Model) failedOptimisticRetentionPolicyOrDefault() FailedOptimisticRetentionPolicy {
	if m == nil || m.failedOptimisticRetentionPolicy == nil {
		return NewDefaultFailedOptimisticRetentionPolicy()
	}
	return m.failedOptimisticRetentionPolicy
}

func (m *Model) optimisticEntriesForSession(sessionID string) []OptimisticSendOverlayEntry {
	if m == nil || len(m.optimisticSends) == 0 {
		return nil
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil
	}
	out := make([]OptimisticSendOverlayEntry, 0, len(m.optimisticSends))
	for token, entry := range m.optimisticSends {
		if strings.TrimSpace(entry.sessionID) != sessionID {
			continue
		}
		out = append(out, OptimisticSendOverlayEntry{
			Token:      token,
			SessionID:  entry.sessionID,
			HeaderLine: entry.headerLine,
			Text:       entry.text,
			CreatedAt:  entry.createdAt,
			Status:     entry.status,
			TurnID:     entry.turnID,
		})
	}
	return out
}

func (m *Model) applyOptimisticOverlay(sessionID string, canonical []ChatBlock) []ChatBlock {
	if m == nil {
		return append([]ChatBlock(nil), canonical...)
	}
	result := m.optimisticOverlayServiceOrDefault().ComposeVisible(
		sessionID,
		canonical,
		m.optimisticEntriesForSession(sessionID),
		time.Now().UTC(),
	)
	for _, token := range result.ResolvedTokens {
		m.removePendingSendToken(token)
	}
	return result.Blocks
}

func (m *Model) removePendingSendToken(token int) {
	if m == nil {
		return
	}
	delete(m.pendingSends, token)
	delete(m.optimisticSends, token)
}
