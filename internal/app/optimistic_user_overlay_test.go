package app

import (
	"testing"
	"time"
)

type stubOptimisticOverlayService struct {
	blocks         []ChatBlock
	resolvedTokens []int
}

func (s stubOptimisticOverlayService) ComposeVisible(_ string, _ []ChatBlock, _ []OptimisticSendOverlayEntry, _ time.Time) OptimisticOverlayResult {
	return OptimisticOverlayResult{
		Blocks:         append([]ChatBlock(nil), s.blocks...),
		ResolvedTokens: append([]int(nil), s.resolvedTokens...),
	}
}

type stubOptimisticReconcilePolicy struct {
	resolved bool
}

func (s stubOptimisticReconcilePolicy) IsResolved(_ OptimisticSendOverlayEntry, _ []ChatBlock) bool {
	return s.resolved
}

type stubFailedOptimisticRetentionPolicy struct {
	retain bool
}

func (s stubFailedOptimisticRetentionPolicy) Retain(_ OptimisticSendOverlayEntry, _ time.Time) bool {
	return s.retain
}

func TestDefaultOptimisticReconcilePolicyDoesNotFallbackToHeuristicWhenTurnIDPresent(t *testing.T) {
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	policy := NewDefaultOptimisticReconcilePolicy()
	entry := OptimisticSendOverlayEntry{
		Token:     1,
		SessionID: "s1",
		Text:      "hello",
		CreatedAt: now,
		TurnID:    "turn-1",
	}
	canonical := []ChatBlock{
		{Role: ChatRoleUser, Text: "hello", CreatedAt: now, TurnID: "turn-2"},
	}
	if policy.IsResolved(entry, canonical) {
		t.Fatalf("expected unresolved when turn id does not match, even if heuristic text/time match exists")
	}
}

func TestDefaultOptimisticReconcilePolicyFallsBackToHeuristicWhenTurnIDMissing(t *testing.T) {
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	policy := NewDefaultOptimisticReconcilePolicy()
	entry := OptimisticSendOverlayEntry{
		Token:     1,
		SessionID: "s1",
		Text:      "hello",
		CreatedAt: now,
	}
	canonical := []ChatBlock{
		{Role: ChatRoleUser, Text: "hello", CreatedAt: now},
	}
	if !policy.IsResolved(entry, canonical) {
		t.Fatalf("expected resolved via heuristic reconciliation when turn id is missing")
	}
}

func TestApplyOptimisticOverlayUsesInjectedServiceAndPrunesResolvedTokens(t *testing.T) {
	m := NewModel(nil)
	m.pendingSends[1] = pendingSend{
		key:       "sess:s1",
		sessionID: "s1",
		state:     pendingSendStateSending,
	}
	m.optimisticSends[1] = optimisticSendEntry{
		token:     1,
		key:       "sess:s1",
		sessionID: "s1",
		text:      "hello",
		status:    ChatStatusSending,
	}
	m.optimisticOverlayService = stubOptimisticOverlayService{
		blocks:         []ChatBlock{{Role: ChatRoleAgent, Text: "from stub"}},
		resolvedTokens: []int{1},
	}

	blocks := m.applyOptimisticOverlay("s1", []ChatBlock{{Role: ChatRoleUser, Text: "canonical"}})
	if len(blocks) != 1 || blocks[0].Role != ChatRoleAgent || blocks[0].Text != "from stub" {
		t.Fatalf("expected injected overlay service to control visible blocks, got %#v", blocks)
	}
	if _, ok := m.pendingSends[1]; ok {
		t.Fatalf("expected resolved token to be pruned from pending sends")
	}
	if _, ok := m.optimisticSends[1]; ok {
		t.Fatalf("expected resolved token to be pruned from optimistic sends")
	}
}

func TestHeuristicOptimisticReconcilePolicyCoverage(t *testing.T) {
	policy := NewHeuristicOptimisticReconcilePolicy()
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

	if policy.IsResolved(OptimisticSendOverlayEntry{}, []ChatBlock{{Role: ChatRoleUser, Text: "hello"}}) {
		t.Fatalf("expected empty optimistic text to never resolve")
	}
	if policy.IsResolved(OptimisticSendOverlayEntry{Text: "hello"}, []ChatBlock{{Role: ChatRoleAgent, Text: "hello"}}) {
		t.Fatalf("expected non-user canonical blocks to be ignored")
	}
	if policy.IsResolved(OptimisticSendOverlayEntry{Text: "hello"}, []ChatBlock{{Role: ChatRoleUser, Text: "goodbye"}}) {
		t.Fatalf("expected mismatched text to remain unresolved")
	}
	if policy.IsResolved(OptimisticSendOverlayEntry{Text: "hello", CreatedAt: now}, []ChatBlock{{Role: ChatRoleUser, Text: "hello", CreatedAt: now.Add(10 * time.Minute)}}) {
		t.Fatalf("expected large timestamp drift to remain unresolved")
	}
	if !policy.IsResolved(OptimisticSendOverlayEntry{Text: "hello", CreatedAt: now}, []ChatBlock{{Role: ChatRoleUser, Text: "hello", CreatedAt: now.Add(-time.Minute)}}) {
		t.Fatalf("expected absolute timestamp delta within window to resolve")
	}
	if !policy.IsResolved(OptimisticSendOverlayEntry{Text: "hello"}, []ChatBlock{
		{Role: ChatRoleUser, Text: "older"},
		{Role: ChatRoleUser, Text: "hello"},
	}) {
		t.Fatalf("expected recent index fallback when timestamps are unavailable")
	}
}

func TestDefaultOptimisticReconcilePolicyNilSubPolicies(t *testing.T) {
	policy := chainedOptimisticReconcilePolicy{}
	if !policy.IsResolved(
		OptimisticSendOverlayEntry{Text: "hello"},
		[]ChatBlock{{Role: ChatRoleUser, Text: "hello"}},
	) {
		t.Fatalf("expected chained policy with nil members to initialize defaults and reconcile")
	}
}

func TestChainedOptimisticReconcilePolicyUsesStrictBranchWithoutTurnID(t *testing.T) {
	policy := chainedOptimisticReconcilePolicy{
		strict:    stubOptimisticReconcilePolicy{resolved: true},
		heuristic: stubOptimisticReconcilePolicy{resolved: false},
	}
	if !policy.IsResolved(
		OptimisticSendOverlayEntry{Text: "hello"},
		[]ChatBlock{{Role: ChatRoleUser, Text: "hello"}},
	) {
		t.Fatalf("expected chained policy to return strict resolution result before heuristic")
	}
}

func TestFailedOptimisticRetentionPolicyCoverage(t *testing.T) {
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	defaultPolicy := NewDefaultFailedOptimisticRetentionPolicy()
	if !defaultPolicy.Retain(OptimisticSendOverlayEntry{Status: ChatStatusSending}, now) {
		t.Fatalf("expected non-failed optimistic entry to be retained")
	}
	if !defaultPolicy.Retain(OptimisticSendOverlayEntry{Status: ChatStatusFailed}, now) {
		t.Fatalf("expected failed optimistic entry without createdAt to be retained")
	}
	if defaultPolicy.Retain(OptimisticSendOverlayEntry{Status: ChatStatusFailed, CreatedAt: now.Add(-failedOptimisticRetentionWindow - time.Second)}, now) {
		t.Fatalf("expected failed optimistic entry beyond retention window to be pruned")
	}
	if !defaultPolicy.Retain(OptimisticSendOverlayEntry{Status: ChatStatusFailed, CreatedAt: now.Add(-time.Minute)}, now) {
		t.Fatalf("expected failed optimistic entry within retention window to be retained")
	}
	disabled := timeWindowFailedOptimisticRetentionPolicy{window: 0}
	if disabled.Retain(OptimisticSendOverlayEntry{Status: ChatStatusFailed, CreatedAt: now}, now) {
		t.Fatalf("expected disabled retention policy to prune failed optimistic entries")
	}
}

func TestDefaultOptimisticOverlayServiceCoverage(t *testing.T) {
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	service := defaultOptimisticOverlayService{}
	result := service.ComposeVisible(
		"s1",
		[]ChatBlock{{Role: ChatRoleUser, Text: "canonical", TurnID: "turn-1"}, {Role: ChatRoleAgent, Text: "reply"}},
		[]OptimisticSendOverlayEntry{
			{Token: 1, SessionID: "other", Text: "skip"},
			{Token: 2, SessionID: "s1", Text: ""},
			{Token: 3, SessionID: "s1", Text: "canonical", TurnID: "turn-1"},
			{Token: 4, SessionID: "s1", Text: "failed", Status: ChatStatusFailed, CreatedAt: now.Add(-failedOptimisticRetentionWindow - time.Second)},
			{Token: 5, SessionID: "s1", Text: "optimistic", HeaderLine: 0, CreatedAt: now, Status: ChatStatusSending},
		},
		now,
	)
	if len(result.Blocks) != 3 {
		t.Fatalf("expected one unresolved optimistic block to be inserted, got %#v", result.Blocks)
	}
	if result.Blocks[0].Role != ChatRoleUser || result.Blocks[0].Text != "optimistic" {
		t.Fatalf("expected optimistic block inserted at requested header line, got %#v", result.Blocks[0])
	}
	if len(result.ResolvedTokens) != 3 {
		t.Fatalf("expected empty/resolved/expired tokens to be pruned, got %#v", result.ResolvedTokens)
	}
}

func TestInsertChatBlockAtCoverage(t *testing.T) {
	base := []ChatBlock{{Role: ChatRoleAgent, Text: "a"}}
	withNegative := insertChatBlockAt(base, ChatBlock{Role: ChatRoleUser, Text: "b"}, -1)
	if len(withNegative) != 2 || withNegative[0].Text != "b" {
		t.Fatalf("expected negative index to insert at beginning, got %#v", withNegative)
	}
	withAppend := insertChatBlockAt(base, ChatBlock{Role: ChatRoleUser, Text: "c"}, 99)
	if len(withAppend) != 2 || withAppend[1].Text != "c" {
		t.Fatalf("expected out-of-range index to append, got %#v", withAppend)
	}
}

func TestOptimisticOverlayOptionsAndNilGuards(t *testing.T) {
	var nilModel *Model
	WithOptimisticOverlayService(stubOptimisticOverlayService{})(nilModel)
	WithOptimisticReconcilePolicy(stubOptimisticReconcilePolicy{})(nilModel)
	WithFailedOptimisticRetentionPolicy(stubFailedOptimisticRetentionPolicy{})(nilModel)
	if svc := nilModel.optimisticOverlayServiceOrDefault(); svc == nil {
		t.Fatal("expected nil model fallback overlay service")
	}
	if policy := nilModel.optimisticReconcilePolicyOrDefault(); policy == nil {
		t.Fatal("expected nil model fallback reconcile policy")
	}
	if policy := nilModel.failedOptimisticRetentionPolicyOrDefault(); policy == nil {
		t.Fatal("expected nil model fallback retention policy")
	}
	if entries := nilModel.optimisticEntriesForSession("s1"); entries != nil {
		t.Fatalf("expected nil model optimistic entries to be nil, got %#v", entries)
	}
	if blocks := nilModel.applyOptimisticOverlay("s1", []ChatBlock{{Role: ChatRoleUser, Text: "x"}}); len(blocks) != 1 {
		t.Fatalf("expected nil model overlay to preserve canonical blocks, got %#v", blocks)
	}
	nilModel.removePendingSendToken(1)

	m := NewModel(nil)
	WithOptimisticOverlayService(stubOptimisticOverlayService{})(&m)
	WithOptimisticReconcilePolicy(stubOptimisticReconcilePolicy{resolved: true})(&m)
	WithFailedOptimisticRetentionPolicy(stubFailedOptimisticRetentionPolicy{retain: true})(&m)
	if m.optimisticOverlayService == nil || m.optimisticReconcilePolicy == nil || m.failedOptimisticRetentionPolicy == nil {
		t.Fatalf("expected option setters to populate model dependencies")
	}
	if m.optimisticReconcilePolicyOrDefault() == nil || m.failedOptimisticRetentionPolicyOrDefault() == nil {
		t.Fatalf("expected explicit injected policies to be returned by helper accessors")
	}
	m.optimisticSends[10] = optimisticSendEntry{
		token:     10,
		sessionID: "other",
		text:      "hello",
	}
	if entries := m.optimisticEntriesForSession(" "); entries != nil {
		t.Fatalf("expected empty session lookup to return nil entries, got %#v", entries)
	}
	if entries := m.optimisticEntriesForSession("s1"); len(entries) != 0 {
		t.Fatalf("expected mismatched session optimistic entries to be filtered out, got %#v", entries)
	}
}
