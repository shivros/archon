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
	Query(ctx context.Context, req composeFileSearchQueryRequest) composeFileSearchQueryResult
	Close(ctx context.Context, searchID string)
}

type defaultComposeFileSearchService struct {
	api FileSearchAPI
}

func newDefaultComposeFileSearchService(api FileSearchAPI) composeFileSearchService {
	return defaultComposeFileSearchService{api: api}
}

func (s defaultComposeFileSearchService) Query(ctx context.Context, req composeFileSearchQueryRequest) composeFileSearchQueryResult {
	query := strings.TrimSpace(req.Query)
	if s.api == nil {
		return composeFileSearchQueryResult{Err: errors.New("file search api unavailable")}
	}
	limit := req.Limit
	if limit <= 0 {
		limit = composeFileSearchLimit
	}

	searchID := strings.TrimSpace(req.SearchID)
	if searchID == "" {
		search, err := s.api.StartFileSearch(ctx, client.StartFileSearchRequest{
			Scope: req.Scope,
			Limit: limit,
		})
		if err != nil {
			return composeFileSearchQueryResultFromError(err)
		}
		if search != nil {
			searchID = strings.TrimSpace(search.ID)
		}
		if searchID == "" {
			return composeFileSearchQueryResult{Err: errors.New("file search id is required")}
		}
	}
	if query == "" {
		return composeFileSearchQueryResult{SearchID: searchID}
	}

	ch, stop, err := s.api.FileSearchEvents(ctx, searchID)
	if err != nil {
		return composeFileSearchQueryResultFromError(err)
	}
	defer stop()

	updateScope := req.Scope
	updateQuery := query
	updateLimit := limit
	if _, err := s.api.UpdateFileSearch(ctx, searchID, client.UpdateFileSearchRequest{
		Scope: &updateScope,
		Query: &updateQuery,
		Limit: &updateLimit,
	}); err != nil {
		return composeFileSearchQueryResultFromError(err)
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
				if event.Query != "" && strings.TrimSpace(event.Query) != query {
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

func composeFileSearchQueryResultFromError(err error) composeFileSearchQueryResult {
	result := composeFileSearchQueryResult{Err: err}
	if err == nil {
		return result
	}
	var apiErr *client.APIError
	if errors.As(err, &apiErr) && strings.EqualFold(strings.TrimSpace(apiErr.Code), apicode.ErrorCodeFileSearchUnsupported) {
		result.Unsupported = true
	}
	return result
}

type composeFileSearchDebounceMsg struct {
	Seq int
}

type composeFileSearchResultsMsg struct {
	Seq    int
	Query  string
	Result composeFileSearchQueryResult
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
