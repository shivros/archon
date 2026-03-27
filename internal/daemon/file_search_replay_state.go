package daemon

import (
	"strings"

	"control/internal/types"
)

type fileSearchReplayState struct {
	latestEvent   *types.FileSearchEvent
	latestResults *types.FileSearchEvent
}

func newFileSearchReplayState() *fileSearchReplayState {
	return &fileSearchReplayState{}
}

func (s *fileSearchReplayState) Apply(session *types.FileSearchSession, event types.FileSearchEvent) {
	if s == nil {
		return
	}
	s.latestEvent = cloneFileSearchEvent(&event)
	if !fileSearchResultsEventMatchesSession(s.latestResults, session) {
		s.latestResults = nil
	}
	if fileSearchResultsEventMatchesSession(&event, session) {
		s.latestResults = cloneFileSearchEvent(&event)
	}
}

func (s *fileSearchReplayState) ReplayEvent(session *types.FileSearchSession) *types.FileSearchEvent {
	if s == nil {
		return nil
	}
	if fileSearchResultsEventMatchesSession(s.latestResults, session) {
		return cloneFileSearchEvent(s.latestResults)
	}
	if s.latestEvent == nil || isTerminalFileSearchEvent(*s.latestEvent) {
		return nil
	}
	return cloneFileSearchEvent(s.latestEvent)
}

func cloneFileSearchEvent(event *types.FileSearchEvent) *types.FileSearchEvent {
	if event == nil {
		return nil
	}
	cloned := *event
	cloned.Scope = copyFileSearchScope(event.Scope)
	if event.Candidates != nil {
		cloned.Candidates = append([]types.FileSearchCandidate(nil), event.Candidates...)
	}
	if event.OccurredAt != nil {
		occurredAt := *event.OccurredAt
		cloned.OccurredAt = &occurredAt
	}
	return &cloned
}

func fileSearchResultsEventMatchesSession(event *types.FileSearchEvent, session *types.FileSearchSession) bool {
	if event == nil || session == nil {
		return false
	}
	if event.Kind != types.FileSearchEventResults {
		return false
	}
	if strings.TrimSpace(event.SearchID) != strings.TrimSpace(session.ID) {
		return false
	}
	if strings.TrimSpace(event.Provider) != strings.TrimSpace(session.Provider) {
		return false
	}
	if event.Query != session.Query {
		return false
	}
	if event.Limit != session.Limit {
		return false
	}
	return copyFileSearchScope(event.Scope) == copyFileSearchScope(session.Scope)
}
