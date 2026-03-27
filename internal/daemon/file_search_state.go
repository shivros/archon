package daemon

import (
	"strings"
	"time"

	"control/internal/types"
)

func normalizeFileSearchScope(scope types.FileSearchScope) types.FileSearchScope {
	return types.FileSearchScope{
		Provider:    strings.TrimSpace(scope.Provider),
		SessionID:   strings.TrimSpace(scope.SessionID),
		WorkspaceID: strings.TrimSpace(scope.WorkspaceID),
		WorktreeID:  strings.TrimSpace(scope.WorktreeID),
		Cwd:         strings.TrimSpace(scope.Cwd),
	}
}

func newFileSearchSession(
	snapshot *types.FileSearchSession,
	searchID, provider string,
	scope types.FileSearchScope,
	query string,
	limit int,
	createdAt time.Time,
) *types.FileSearchSession {
	base := &types.FileSearchSession{
		ID:        searchID,
		Provider:  provider,
		Scope:     copyFileSearchScope(scope),
		Query:     query,
		Limit:     limit,
		Status:    types.FileSearchStatusActive,
		CreatedAt: createdAt,
	}
	return coalesceFileSearchSession(snapshot, base)
}

func applyFileSearchCommandUpdate(
	current *types.FileSearchSession,
	req types.FileSearchUpdateRequest,
	snapshot *types.FileSearchSession,
	now time.Time,
) (*types.FileSearchSession, types.FileSearchEvent) {
	next := coalesceFileSearchSession(snapshot, current)
	if req.Scope != nil {
		next.Scope = copyFileSearchScope(*req.Scope)
		next.Provider = strings.TrimSpace(req.Scope.Provider)
	}
	if req.Query != nil {
		next.Query = *req.Query
	}
	if req.Limit != nil {
		next.Limit = *req.Limit
	}
	if next.Status == "" || next.Status == types.FileSearchStatusCreated {
		next.Status = types.FileSearchStatusActive
	}
	next.UpdatedAt = &now
	return next, buildFileSearchEvent(types.FileSearchEventUpdated, next, nil, "", now)
}

func applyFileSearchClose(current *types.FileSearchSession, now time.Time) (*types.FileSearchSession, types.FileSearchEvent) {
	next := coalesceFileSearchSession(nil, current)
	next.Status = types.FileSearchStatusClosed
	next.UpdatedAt = &now
	next.ClosedAt = &now
	return next, buildFileSearchEvent(types.FileSearchEventClosed, next, nil, "", now)
}

func applyFileSearchRuntimeEvent(
	current *types.FileSearchSession,
	event types.FileSearchEvent,
	now time.Time,
) (*types.FileSearchSession, types.FileSearchEvent, bool) {
	normalized := normalizeFileSearchEvent(event, current, now)
	next := coalesceFileSearchSession(nil, current)
	next.Provider = strings.TrimSpace(normalized.Provider)
	next.Scope = copyFileSearchScope(normalized.Scope)
	next.Query = normalized.Query
	next.Limit = normalized.Limit
	next.Status = normalized.Status
	next.UpdatedAt = normalized.OccurredAt
	terminal := isTerminalFileSearchEvent(normalized)
	if terminal {
		next.ClosedAt = normalized.OccurredAt
	}
	return next, normalized, terminal
}

func normalizeFileSearchEvent(event types.FileSearchEvent, session *types.FileSearchSession, now time.Time) types.FileSearchEvent {
	normalized := event
	if session != nil {
		if strings.TrimSpace(normalized.SearchID) == "" {
			normalized.SearchID = strings.TrimSpace(session.ID)
		}
		if strings.TrimSpace(normalized.Provider) == "" {
			normalized.Provider = strings.TrimSpace(session.Provider)
		}
		if isZeroFileSearchScope(normalized.Scope) {
			normalized.Scope = copyFileSearchScope(session.Scope)
		}
		if normalized.Query == "" {
			normalized.Query = session.Query
		}
		if normalized.Limit <= 0 {
			normalized.Limit = session.Limit
		}
	}
	if normalized.Status == "" {
		normalized.Status = fileSearchStatusFromEventKind(normalized.Kind)
	}
	if normalized.Status == "" && session != nil {
		normalized.Status = session.Status
	}
	if normalized.OccurredAt == nil {
		normalized.OccurredAt = &now
	}
	return normalized
}

func fileSearchStatusFromEventKind(kind types.FileSearchEventKind) types.FileSearchStatus {
	switch kind {
	case types.FileSearchEventClosed:
		return types.FileSearchStatusClosed
	case types.FileSearchEventFailed:
		return types.FileSearchStatusFailed
	case types.FileSearchEventStarted, types.FileSearchEventUpdated, types.FileSearchEventResults:
		return types.FileSearchStatusActive
	default:
		return ""
	}
}

func isTerminalFileSearchEvent(event types.FileSearchEvent) bool {
	return event.Kind == types.FileSearchEventClosed ||
		event.Kind == types.FileSearchEventFailed ||
		event.Status == types.FileSearchStatusClosed ||
		event.Status == types.FileSearchStatusFailed
}

func buildFileSearchEvent(
	kind types.FileSearchEventKind,
	session *types.FileSearchSession,
	candidates []types.FileSearchCandidate,
	errText string,
	occurredAt time.Time,
) types.FileSearchEvent {
	if session == nil {
		session = &types.FileSearchSession{}
	}
	return types.FileSearchEvent{
		Kind:       kind,
		SearchID:   strings.TrimSpace(session.ID),
		Provider:   strings.TrimSpace(session.Provider),
		Scope:      copyFileSearchScope(session.Scope),
		Query:      session.Query,
		Status:     session.Status,
		Limit:      session.Limit,
		Candidates: append([]types.FileSearchCandidate(nil), candidates...),
		Error:      strings.TrimSpace(errText),
		OccurredAt: &occurredAt,
	}
}

func coalesceFileSearchSession(primary, fallback *types.FileSearchSession) *types.FileSearchSession {
	if primary == nil && fallback == nil {
		return &types.FileSearchSession{}
	}
	if fallback == nil {
		return cloneFileSearchSession(primary)
	}
	if primary == nil {
		return cloneFileSearchSession(fallback)
	}
	next := cloneFileSearchSession(primary)
	if strings.TrimSpace(next.ID) == "" {
		next.ID = strings.TrimSpace(fallback.ID)
	}
	if strings.TrimSpace(next.Provider) == "" {
		next.Provider = strings.TrimSpace(fallback.Provider)
	}
	if isZeroFileSearchScope(next.Scope) {
		next.Scope = copyFileSearchScope(fallback.Scope)
	}
	if next.Query == "" {
		next.Query = fallback.Query
	}
	if next.Limit <= 0 {
		next.Limit = fallback.Limit
	}
	if next.Status == "" {
		next.Status = fallback.Status
	}
	if next.CreatedAt.IsZero() {
		next.CreatedAt = fallback.CreatedAt
	}
	if next.UpdatedAt == nil {
		next.UpdatedAt = fallback.UpdatedAt
	}
	if next.ClosedAt == nil {
		next.ClosedAt = fallback.ClosedAt
	}
	return next
}

func broadcastFileSearchEvent(channels []chan types.FileSearchEvent, event types.FileSearchEvent) {
	for _, ch := range channels {
		select {
		case ch <- event:
		default:
		}
	}
}

func closeFileSearchSubscribers(channels []chan types.FileSearchEvent) {
	for _, ch := range channels {
		close(ch)
	}
}

func cloneFileSearchSession(session *types.FileSearchSession) *types.FileSearchSession {
	if session == nil {
		return nil
	}
	cloned := *session
	cloned.Scope = copyFileSearchScope(session.Scope)
	return &cloned
}

func copyFileSearchScope(scope types.FileSearchScope) types.FileSearchScope {
	return types.FileSearchScope{
		Provider:    strings.TrimSpace(scope.Provider),
		SessionID:   strings.TrimSpace(scope.SessionID),
		WorkspaceID: strings.TrimSpace(scope.WorkspaceID),
		WorktreeID:  strings.TrimSpace(scope.WorktreeID),
		Cwd:         strings.TrimSpace(scope.Cwd),
	}
}

func isZeroFileSearchScope(scope types.FileSearchScope) bool {
	return strings.TrimSpace(scope.Provider) == "" &&
		strings.TrimSpace(scope.SessionID) == "" &&
		strings.TrimSpace(scope.WorkspaceID) == "" &&
		strings.TrimSpace(scope.WorktreeID) == "" &&
		strings.TrimSpace(scope.Cwd) == ""
}
