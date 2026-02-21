package app

import (
	"strings"

	"charm.land/bubbles/v2/list"
)

type sidebarApplyReason int

const (
	sidebarApplyReasonUser sidebarApplyReason = iota
	sidebarApplyReasonBackground
)

type sidebarSelectionLookup interface {
	HasSession(sessionID string) bool
}

type sidebarSelectionDecisionInput struct {
	Items             []list.Item
	SelectedKey       string
	ActiveWorkspaceID string
	ActiveWorktreeID  string
	Reason            sidebarApplyReason
	Lookup            sidebarSelectionLookup
}

type sidebarSelectionDecision struct {
	Candidates []string
	Reason     sidebarSelectionResolutionReason
}

type sidebarSelectionResolutionReason string

const (
	sidebarSelectionReasonEmpty           sidebarSelectionResolutionReason = "empty"
	sidebarSelectionReasonPreserveKey     sidebarSelectionResolutionReason = "preserve_key"
	sidebarSelectionReasonPreserveSession sidebarSelectionResolutionReason = "preserve_session"
	sidebarSelectionReasonActiveContext   sidebarSelectionResolutionReason = "active_context"
	sidebarSelectionReasonVisibleSession  sidebarSelectionResolutionReason = "visible_session"
	sidebarSelectionReasonVisibleFallback sidebarSelectionResolutionReason = "visible_fallback"
)

type sidebarSelectionDecisionService interface {
	Decide(input sidebarSelectionDecisionInput) sidebarSelectionDecision
}

type sidebarSelectionStrategy interface {
	Reason() sidebarSelectionResolutionReason
	Candidate(input sidebarSelectionDecisionInput) string
}

type defaultSidebarSelectionDecisionService struct {
	strategies []sidebarSelectionStrategy
}

func NewDefaultSidebarSelectionDecisionService() sidebarSelectionDecisionService {
	return defaultSidebarSelectionDecisionService{
		strategies: []sidebarSelectionStrategy{
			preserveExactSelectionStrategy{},
			preserveSessionIdentityStrategy{},
			activeContextFallbackStrategy{},
			firstVisibleSessionFallbackStrategy{},
			firstVisibleItemFallbackStrategy{},
		},
	}
}

func (s defaultSidebarSelectionDecisionService) Decide(input sidebarSelectionDecisionInput) sidebarSelectionDecision {
	if len(input.Items) == 0 {
		return sidebarSelectionDecision{Reason: sidebarSelectionReasonEmpty}
	}
	candidates := make([]string, 0, len(s.strategies))
	seen := map[string]struct{}{}
	reason := sidebarSelectionReasonVisibleFallback
	for _, strategy := range s.strategies {
		if strategy == nil {
			continue
		}
		key := strings.TrimSpace(strategy.Candidate(input))
		if key == "" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		candidates = append(candidates, key)
		if len(candidates) == 1 {
			reason = strategy.Reason()
		}
	}
	return sidebarSelectionDecision{
		Candidates: candidates,
		Reason:     reason,
	}
}

type preserveExactSelectionStrategy struct{}

func (preserveExactSelectionStrategy) Reason() sidebarSelectionResolutionReason {
	return sidebarSelectionReasonPreserveKey
}

func (preserveExactSelectionStrategy) Candidate(input sidebarSelectionDecisionInput) string {
	selectedKey := strings.TrimSpace(input.SelectedKey)
	if selectedKey == "" {
		return ""
	}
	if sidebarItemsContainKey(input.Items, selectedKey) {
		return selectedKey
	}
	return ""
}

type preserveSessionIdentityStrategy struct{}

func (preserveSessionIdentityStrategy) Reason() sidebarSelectionResolutionReason {
	return sidebarSelectionReasonPreserveSession
}

func (preserveSessionIdentityStrategy) Candidate(input sidebarSelectionDecisionInput) string {
	selectedSessionID := sessionIDFromSidebarKey(input.SelectedKey)
	if selectedSessionID == "" || input.Lookup == nil {
		return ""
	}
	if !input.Lookup.HasSession(selectedSessionID) {
		return ""
	}
	return "sess:" + selectedSessionID
}

type activeContextFallbackStrategy struct{}

func (activeContextFallbackStrategy) Reason() sidebarSelectionResolutionReason {
	return sidebarSelectionReasonActiveContext
}

func (activeContextFallbackStrategy) Candidate(input sidebarSelectionDecisionInput) string {
	if len(input.Items) == 0 {
		return ""
	}
	selectedIdx := selectSidebarIndex(input.Items, strings.TrimSpace(input.SelectedKey), input.ActiveWorkspaceID, input.ActiveWorktreeID)
	if selectedIdx < 0 || selectedIdx >= len(input.Items) {
		selectedIdx = 0
	}
	entry, ok := input.Items[selectedIdx].(*sidebarItem)
	if !ok || entry == nil {
		return ""
	}
	candidate := strings.TrimSpace(entry.key())
	if candidate == "" {
		return ""
	}
	if input.Reason == sidebarApplyReasonBackground &&
		sessionIDFromSidebarKey(input.SelectedKey) != "" &&
		!isSessionSidebarKey(candidate) {
		if sessionFallback := firstVisibleSessionKey(input.Items); sessionFallback != "" {
			return sessionFallback
		}
	}
	return candidate
}

type firstVisibleSessionFallbackStrategy struct{}

func (firstVisibleSessionFallbackStrategy) Reason() sidebarSelectionResolutionReason {
	return sidebarSelectionReasonVisibleSession
}

func (firstVisibleSessionFallbackStrategy) Candidate(input sidebarSelectionDecisionInput) string {
	return firstVisibleSessionKey(input.Items)
}

type firstVisibleItemFallbackStrategy struct{}

func (firstVisibleItemFallbackStrategy) Reason() sidebarSelectionResolutionReason {
	return sidebarSelectionReasonVisibleFallback
}

func (firstVisibleItemFallbackStrategy) Candidate(input sidebarSelectionDecisionInput) string {
	return firstVisibleItemKey(input.Items)
}

func firstVisibleSessionKey(items []list.Item) string {
	for _, item := range items {
		entry, ok := item.(*sidebarItem)
		if !ok || entry == nil || !entry.isSession() {
			continue
		}
		return strings.TrimSpace(entry.key())
	}
	return ""
}

func firstVisibleItemKey(items []list.Item) string {
	for _, item := range items {
		entry, ok := item.(*sidebarItem)
		if !ok || entry == nil {
			continue
		}
		return strings.TrimSpace(entry.key())
	}
	return ""
}

func isSessionSidebarKey(key string) bool {
	return sessionIDFromSidebarKey(strings.TrimSpace(key)) != ""
}
