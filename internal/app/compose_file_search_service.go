package app

import (
	"context"
	"errors"
	"strings"
	"time"

	"control/internal/apicode"
	"control/internal/client"
	"control/internal/types"

	tea "charm.land/bubbletea/v2"
)

const (
	composeFileSearchDebounceDelay = 120 * time.Millisecond
	composeFileSearchTimeout       = 4 * time.Second
	composeFileSearchLimit         = 8
)

type composeFileSearchStartRequest struct {
	Scope types.FileSearchScope
	Query string
	Limit int
}

type composeFileSearchStartResult struct {
	Session     *types.FileSearchSession
	Unsupported bool
	Err         error
}

type composeFileSearchUpdateRequest struct {
	SearchID string
	Scope    types.FileSearchScope
	Query    string
	Limit    int
}

type composeFileSearchUpdateResult struct {
	SearchID    string
	Session     *types.FileSearchSession
	Unsupported bool
	Err         error
}

type composeFileSearchQueryRequest struct {
	SearchID string
	Scope    types.FileSearchScope
	Query    string
	Limit    int
}

type composeFileSearchQueryResult struct {
	SearchID    string
	Candidates  []types.FileSearchCandidate
	Unsupported bool
	Err         error
}

type composeFileSearchService interface {
	Start(ctx context.Context, req composeFileSearchStartRequest) composeFileSearchStartResult
	Update(ctx context.Context, req composeFileSearchUpdateRequest) composeFileSearchUpdateResult
	Events(ctx context.Context, searchID string) (<-chan types.FileSearchEvent, func(), error)
	Query(ctx context.Context, req composeFileSearchQueryRequest) composeFileSearchQueryResult
	Close(ctx context.Context, searchID string)
}

type defaultComposeFileSearchService struct {
	api FileSearchAPI
}

func newDefaultComposeFileSearchService(api FileSearchAPI) composeFileSearchService {
	return defaultComposeFileSearchService{api: api}
}

func (s defaultComposeFileSearchService) Start(ctx context.Context, req composeFileSearchStartRequest) composeFileSearchStartResult {
	if s.api == nil {
		return composeFileSearchStartResult{Err: errors.New("file search api unavailable")}
	}
	limit := req.Limit
	if limit <= 0 {
		limit = composeFileSearchLimit
	}
	search, err := s.api.StartFileSearch(ctx, client.StartFileSearchRequest{
		Scope: req.Scope,
		Query: strings.TrimSpace(req.Query),
		Limit: limit,
	})
	if err != nil {
		return composeFileSearchStartResultFromError(err)
	}
	if search == nil || strings.TrimSpace(search.ID) == "" {
		return composeFileSearchStartResult{Err: errors.New("file search id is required")}
	}
	return composeFileSearchStartResult{Session: search}
}

func (s defaultComposeFileSearchService) Update(ctx context.Context, req composeFileSearchUpdateRequest) composeFileSearchUpdateResult {
	searchID := strings.TrimSpace(req.SearchID)
	if s.api == nil {
		return composeFileSearchUpdateResult{SearchID: searchID, Err: errors.New("file search api unavailable")}
	}
	if searchID == "" {
		return composeFileSearchUpdateResult{Err: errors.New("file search id is required")}
	}
	limit := req.Limit
	if limit <= 0 {
		limit = composeFileSearchLimit
	}
	scope := req.Scope
	query := strings.TrimSpace(req.Query)
	session, err := s.api.UpdateFileSearch(ctx, searchID, client.UpdateFileSearchRequest{
		Scope: &scope,
		Query: &query,
		Limit: &limit,
	})
	if err != nil {
		result := composeFileSearchUpdateResultFromError(err)
		result.SearchID = searchID
		return result
	}
	return composeFileSearchUpdateResult{
		SearchID: searchID,
		Session:  session,
	}
}

func (s defaultComposeFileSearchService) Events(ctx context.Context, searchID string) (<-chan types.FileSearchEvent, func(), error) {
	searchID = strings.TrimSpace(searchID)
	if s.api == nil {
		return nil, func() {}, errors.New("file search api unavailable")
	}
	return s.api.FileSearchEvents(ctx, searchID)
}

func (s defaultComposeFileSearchService) Query(ctx context.Context, req composeFileSearchQueryRequest) composeFileSearchQueryResult {
	query := strings.TrimSpace(req.Query)
	searchID := strings.TrimSpace(req.SearchID)
	if searchID == "" {
		started := s.Start(ctx, composeFileSearchStartRequest{
			Scope: req.Scope,
			Query: "",
			Limit: req.Limit,
		})
		if started.Err != nil {
			return composeFileSearchQueryResult{
				Unsupported: started.Unsupported,
				Err:         started.Err,
			}
		}
		if started.Session != nil {
			searchID = strings.TrimSpace(started.Session.ID)
		}
	}
	if searchID == "" {
		return composeFileSearchQueryResult{Err: errors.New("file search id is required")}
	}
	if query == "" {
		return composeFileSearchQueryResult{SearchID: searchID}
	}
	ch, stop, err := s.Events(ctx, searchID)
	if err != nil {
		return composeFileSearchQueryResultFromError(err)
	}
	defer stop()
	updated := s.Update(ctx, composeFileSearchUpdateRequest{
		SearchID: searchID,
		Scope:    req.Scope,
		Query:    query,
		Limit:    req.Limit,
	})
	if updated.Err != nil {
		return composeFileSearchQueryResult{
			SearchID:    searchID,
			Unsupported: updated.Unsupported,
			Err:         updated.Err,
		}
	}
	for {
		select {
		case <-ctx.Done():
			return composeFileSearchQueryResult{SearchID: searchID, Err: ctx.Err()}
		case event, ok := <-ch:
			if !ok {
				return composeFileSearchQueryResult{SearchID: searchID}
			}
			if strings.TrimSpace(event.SearchID) != searchID {
				continue
			}
			switch event.Kind {
			case types.FileSearchEventResults:
				if strings.TrimSpace(event.Query) != query {
					continue
				}
				return composeFileSearchQueryResult{
					SearchID:   searchID,
					Candidates: append([]types.FileSearchCandidate(nil), event.Candidates...),
				}
			case types.FileSearchEventFailed:
				if strings.TrimSpace(event.Error) == "" {
					return composeFileSearchQueryResult{SearchID: searchID, Err: errors.New("file search failed")}
				}
				return composeFileSearchQueryResult{SearchID: searchID, Err: errors.New(strings.TrimSpace(event.Error))}
			case types.FileSearchEventClosed:
				return composeFileSearchQueryResult{SearchID: searchID}
			}
		}
	}
}

func (s defaultComposeFileSearchService) Close(ctx context.Context, searchID string) {
	searchID = strings.TrimSpace(searchID)
	if s.api == nil || searchID == "" {
		return
	}
	_ = s.api.CloseFileSearch(ctx, searchID)
}

func composeFileSearchStartResultFromError(err error) composeFileSearchStartResult {
	return composeFileSearchStartResult{
		Unsupported: isComposeFileSearchUnsupportedError(err),
		Err:         err,
	}
}

func composeFileSearchUpdateResultFromError(err error) composeFileSearchUpdateResult {
	return composeFileSearchUpdateResult{
		Unsupported: isComposeFileSearchUnsupportedError(err),
		Err:         err,
	}
}

func composeFileSearchQueryResultFromError(err error) composeFileSearchQueryResult {
	return composeFileSearchQueryResult{
		Unsupported: isComposeFileSearchUnsupportedError(err),
		Err:         err,
	}
}

func isComposeFileSearchUnsupportedError(err error) bool {
	if err == nil {
		return false
	}
	var apiErr *client.APIError
	return errors.As(err, &apiErr) && strings.EqualFold(strings.TrimSpace(apiErr.Code), apicode.ErrorCodeFileSearchUnsupported)
}

type composeFileSearchDebounceMsg struct {
	Seq int
}

type composeFileSearchStartedMsg struct {
	Seq    int
	Query  string
	Result composeFileSearchStartResult
}

type composeFileSearchUpdatedMsg struct {
	Seq    int
	Query  string
	Result composeFileSearchUpdateResult
}

type composeFileSearchResultsMsg struct {
	Seq    int
	Query  string
	Result composeFileSearchQueryResult
}

type composeFileSearchStreamMsg struct {
	SearchID string
	Ch       <-chan types.FileSearchEvent
	Cancel   func()
	Err      error
}

func composeFileSearchDebounceCmd(seq int, delay time.Duration) tea.Cmd {
	return tea.Tick(delay, func(time.Time) tea.Msg {
		return composeFileSearchDebounceMsg{Seq: seq}
	})
}

func closeComposeFileSearchServiceCmd(service composeFileSearchService, id string) tea.Cmd {
	id = strings.TrimSpace(id)
	if service == nil || id == "" {
		return nil
	}
	return func() tea.Msg {
		ctx, cancel := commandWithTimeout(nil, 2*time.Second)
		defer cancel()
		service.Close(ctx, id)
		return nil
	}
}

func startComposeFileSearchCmd(
	service composeFileSearchService,
	req composeFileSearchStartRequest,
	seq int,
	parent context.Context,
) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := commandWithTimeout(parent, composeFileSearchTimeout)
		defer cancel()
		return composeFileSearchStartedMsg{
			Seq:    seq,
			Query:  strings.TrimSpace(req.Query),
			Result: service.Start(ctx, req),
		}
	}
}

func updateComposeFileSearchCmd(
	service composeFileSearchService,
	req composeFileSearchUpdateRequest,
	seq int,
	parent context.Context,
) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := commandWithTimeout(parent, composeFileSearchTimeout)
		defer cancel()
		return composeFileSearchUpdatedMsg{
			Seq:    seq,
			Query:  strings.TrimSpace(req.Query),
			Result: service.Update(ctx, req),
		}
	}
}

func openComposeFileSearchStreamCmd(
	service composeFileSearchService,
	searchID string,
	parent context.Context,
) tea.Cmd {
	searchID = strings.TrimSpace(searchID)
	if service == nil || searchID == "" {
		return nil
	}
	return func() tea.Msg {
		ch, cancel, err := service.Events(commandParentContext(parent), searchID)
		return composeFileSearchStreamMsg{
			SearchID: searchID,
			Ch:       ch,
			Cancel:   cancel,
			Err:      err,
		}
	}
}

func queryComposeFileSearchCmd(
	service composeFileSearchService,
	req composeFileSearchQueryRequest,
	seq int,
	parent context.Context,
) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := commandWithTimeout(parent, composeFileSearchTimeout)
		defer cancel()
		return composeFileSearchResultsMsg{
			Seq:    seq,
			Query:  strings.TrimSpace(req.Query),
			Result: service.Query(ctx, req),
		}
	}
}
