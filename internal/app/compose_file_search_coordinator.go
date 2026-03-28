package app

import (
	"context"
	"strings"
	"time"

	"control/internal/types"
)

type composeFileSearchCoordinator interface {
	Reset() string
	SyncInput(scopeKey string, fragment composeFileSearchFragment, ok bool) composeFileSearchInputTransition
	PrepareDebounce(seq int) (composeFileSearchDebounceTransition, bool)
	ApplyStarted(seq int, query string, result composeFileSearchStartResult) composeFileSearchAsyncTransition
	ApplyUpdated(seq int, query string, result composeFileSearchUpdateResult) composeFileSearchAsyncTransition
	ApplyStreamOpen(searchID string, err error) composeFileSearchStreamTransition
	ApplyEvent(event types.FileSearchEvent) composeFileSearchEventTransition
	HandleStreamClosed() composeFileSearchEventTransition
	ActiveFragment() (composeFileSearchFragment, bool)
	CurrentSearchID() string
	CurrentRequestSeq() int
	RememberSearchID(searchID string)
}

type composeFileSearchCloser interface {
	CloseAsync(service composeFileSearchService, searchID string)
}

type composeFileSearchInputTransition struct {
	SearchIDToClose       string
	ReplaceLifecycleScope bool
	EnsureLifecycleScope  bool
	CancelUpdateScope     bool
	ClosePopup            bool
	LoadingChanged        bool
	Loading               bool
	ScheduleDebounce      bool
	Seq                   int
}

type composeFileSearchDebounceTransition struct {
	Start          bool
	SearchID       string
	Query          string
	LoadingChanged bool
	Loading        bool
}

type composeFileSearchAsyncTransition struct {
	SearchIDToClose string
	OpenStream      bool
	SearchID        string
	ClosePopup      bool
	LoadingChanged  bool
	Loading         bool
	Reset           bool
	Unsupported     bool
}

type composeFileSearchStreamTransition struct {
	SearchIDToClose string
	Accept          bool
	ClosePopup      bool
	LoadingChanged  bool
	Loading         bool
	Reset           bool
}

type composeFileSearchEventTransition struct {
	SearchIDToClose string
	ClosePopup      bool
	LoadingChanged  bool
	Loading         bool
	ApplyCandidates bool
	Candidates      []types.FileSearchCandidate
	Reset           bool
}

type defaultComposeFileSearchCoordinator struct {
	searchID     string
	scopeKey     string
	fragment     composeFileSearchFragment
	fragmentOpen bool
	requestSeq   int
}

type defaultComposeFileSearchCloser struct{}

func WithComposeFileSearchCoordinator(coordinator composeFileSearchCoordinator) ModelOption {
	return func(m *Model) {
		if m == nil {
			return
		}
		m.composeFileSearchCoordinator = coordinator
	}
}

func WithComposeFileSearchCloser(closer composeFileSearchCloser) ModelOption {
	return func(m *Model) {
		if m == nil {
			return
		}
		m.composeFileSearchCloser = closer
	}
}

func NewDefaultComposeFileSearchCoordinator() composeFileSearchCoordinator {
	return &defaultComposeFileSearchCoordinator{}
}

func (c *defaultComposeFileSearchCoordinator) Reset() string {
	if c == nil {
		return ""
	}
	searchID := strings.TrimSpace(c.searchID)
	c.searchID = ""
	c.scopeKey = ""
	c.fragment = composeFileSearchFragment{}
	c.fragmentOpen = false
	c.requestSeq = 0
	return searchID
}

func (c *defaultComposeFileSearchCoordinator) SyncInput(
	scopeKey string,
	fragment composeFileSearchFragment,
	ok bool,
) composeFileSearchInputTransition {
	if c == nil {
		return composeFileSearchInputTransition{}
	}
	scopeKey = strings.TrimSpace(scopeKey)
	previous := c.fragment
	previousOK := c.fragmentOpen
	previousScopeKey := c.scopeKey
	c.fragment = fragment
	c.fragmentOpen = ok
	if !ok {
		return composeFileSearchInputTransition{
			SearchIDToClose:   c.Reset(),
			CancelUpdateScope: true,
			ClosePopup:        true,
			LoadingChanged:    true,
			Loading:           false,
		}
	}
	sameLifecycle := previousOK && previous.Start == fragment.Start && previousScopeKey == scopeKey
	if !sameLifecycle {
		searchID := c.Reset()
		c.scopeKey = scopeKey
		c.fragment = fragment
		c.fragmentOpen = true
		transition := composeFileSearchInputTransition{
			SearchIDToClose:       searchID,
			ReplaceLifecycleScope: true,
			CancelUpdateScope:     true,
		}
		if strings.TrimSpace(fragment.Query) == "" {
			transition.ClosePopup = true
			transition.LoadingChanged = true
			return transition
		}
		c.requestSeq++
		transition.LoadingChanged = true
		transition.Loading = true
		transition.ScheduleDebounce = true
		transition.Seq = c.requestSeq
		return transition
	}
	c.scopeKey = scopeKey
	if strings.TrimSpace(fragment.Query) == "" {
		return composeFileSearchInputTransition{
			CancelUpdateScope: true,
			ClosePopup:        true,
			LoadingChanged:    true,
			Loading:           false,
		}
	}
	if previousOK &&
		previous.Start == fragment.Start &&
		previous.End == fragment.End &&
		previous.Query == fragment.Query {
		return composeFileSearchInputTransition{}
	}
	c.requestSeq++
	return composeFileSearchInputTransition{
		EnsureLifecycleScope: true,
		LoadingChanged:       true,
		Loading:              true,
		ScheduleDebounce:     true,
		Seq:                  c.requestSeq,
	}
}

func (c *defaultComposeFileSearchCoordinator) PrepareDebounce(seq int) (composeFileSearchDebounceTransition, bool) {
	if c == nil || seq != c.requestSeq || !c.fragmentOpen {
		return composeFileSearchDebounceTransition{}, false
	}
	query := strings.TrimSpace(c.fragment.Query)
	if query == "" {
		return composeFileSearchDebounceTransition{
			LoadingChanged: true,
			Loading:        false,
		}, true
	}
	return composeFileSearchDebounceTransition{
		Start:    strings.TrimSpace(c.searchID) == "",
		SearchID: strings.TrimSpace(c.searchID),
		Query:    query,
	}, true
}

func (c *defaultComposeFileSearchCoordinator) ApplyStarted(
	seq int,
	query string,
	result composeFileSearchStartResult,
) composeFileSearchAsyncTransition {
	if c == nil {
		return composeFileSearchAsyncTransition{}
	}
	query = strings.TrimSpace(query)
	searchID := ""
	if result.Session != nil {
		searchID = strings.TrimSpace(result.Session.ID)
	}
	if seq != c.requestSeq || !c.fragmentOpen || strings.TrimSpace(c.fragment.Query) != query {
		return composeFileSearchAsyncTransition{SearchIDToClose: searchID}
	}
	if result.Err != nil {
		if isCanceledRequestError(result.Err) {
			return composeFileSearchAsyncTransition{SearchIDToClose: searchID}
		}
		stale := searchID
		if result.Unsupported {
			stale = c.Reset()
			if stale == "" {
				stale = searchID
			}
		}
		return composeFileSearchAsyncTransition{
			SearchIDToClose: stale,
			ClosePopup:      true,
			LoadingChanged:  true,
			Loading:         false,
			Reset:           result.Unsupported,
			Unsupported:     result.Unsupported,
		}
	}
	c.searchID = searchID
	if searchID == "" {
		return composeFileSearchAsyncTransition{
			LoadingChanged: true,
			Loading:        false,
		}
	}
	return composeFileSearchAsyncTransition{
		OpenStream: true,
		SearchID:   searchID,
	}
}

func (c *defaultComposeFileSearchCoordinator) ApplyUpdated(
	seq int,
	query string,
	result composeFileSearchUpdateResult,
) composeFileSearchAsyncTransition {
	if c == nil {
		return composeFileSearchAsyncTransition{}
	}
	query = strings.TrimSpace(query)
	searchID := strings.TrimSpace(result.SearchID)
	if seq != c.requestSeq || !c.fragmentOpen || strings.TrimSpace(c.fragment.Query) != query {
		return composeFileSearchAsyncTransition{}
	}
	if result.Err != nil {
		if isCanceledRequestError(result.Err) {
			return composeFileSearchAsyncTransition{}
		}
		stale := searchID
		if stale == "" {
			stale = strings.TrimSpace(c.searchID)
		}
		if result.Unsupported {
			stale = c.Reset()
			if stale == "" {
				stale = searchID
			}
		}
		return composeFileSearchAsyncTransition{
			SearchIDToClose: stale,
			ClosePopup:      true,
			LoadingChanged:  true,
			Loading:         false,
			Reset:           result.Unsupported,
			Unsupported:     result.Unsupported,
		}
	}
	if searchID != "" {
		c.searchID = searchID
	}
	return composeFileSearchAsyncTransition{}
}

func (c *defaultComposeFileSearchCoordinator) ApplyStreamOpen(
	searchID string,
	err error,
) composeFileSearchStreamTransition {
	if c == nil {
		return composeFileSearchStreamTransition{}
	}
	searchID = strings.TrimSpace(searchID)
	if err != nil {
		if isCanceledRequestError(err) {
			return composeFileSearchStreamTransition{}
		}
		stale := c.Reset()
		if stale == "" {
			stale = searchID
		}
		return composeFileSearchStreamTransition{
			SearchIDToClose: stale,
			ClosePopup:      true,
			LoadingChanged:  true,
			Loading:         false,
			Reset:           true,
		}
	}
	if searchID == "" || searchID != strings.TrimSpace(c.searchID) {
		return composeFileSearchStreamTransition{SearchIDToClose: searchID}
	}
	return composeFileSearchStreamTransition{Accept: true}
}

func (c *defaultComposeFileSearchCoordinator) ApplyEvent(event types.FileSearchEvent) composeFileSearchEventTransition {
	if c == nil {
		return composeFileSearchEventTransition{}
	}
	searchID := strings.TrimSpace(event.SearchID)
	if searchID == "" || searchID != strings.TrimSpace(c.searchID) {
		return composeFileSearchEventTransition{}
	}
	if !c.fragmentOpen {
		return composeFileSearchEventTransition{
			SearchIDToClose: c.Reset(),
			Reset:           true,
		}
	}
	switch event.Kind {
	case types.FileSearchEventResults:
		if strings.TrimSpace(event.Query) != strings.TrimSpace(c.fragment.Query) {
			return composeFileSearchEventTransition{}
		}
		return composeFileSearchEventTransition{
			LoadingChanged:  true,
			Loading:         false,
			ApplyCandidates: true,
			Candidates:      append([]types.FileSearchCandidate(nil), event.Candidates...),
		}
	case types.FileSearchEventFailed:
		if strings.TrimSpace(event.Query) != "" && strings.TrimSpace(event.Query) != strings.TrimSpace(c.fragment.Query) {
			return composeFileSearchEventTransition{}
		}
		return composeFileSearchEventTransition{
			ClosePopup:     true,
			LoadingChanged: true,
			Loading:        false,
		}
	case types.FileSearchEventClosed:
		return composeFileSearchEventTransition{
			LoadingChanged: true,
			Loading:        false,
		}
	default:
		return composeFileSearchEventTransition{}
	}
}

func (c *defaultComposeFileSearchCoordinator) HandleStreamClosed() composeFileSearchEventTransition {
	return composeFileSearchEventTransition{
		LoadingChanged: true,
		Loading:        false,
	}
}

func (c *defaultComposeFileSearchCoordinator) ActiveFragment() (composeFileSearchFragment, bool) {
	if c == nil || !c.fragmentOpen {
		return composeFileSearchFragment{}, false
	}
	return c.fragment, true
}

func (c *defaultComposeFileSearchCoordinator) CurrentSearchID() string {
	if c == nil {
		return ""
	}
	return strings.TrimSpace(c.searchID)
}

func (c *defaultComposeFileSearchCoordinator) CurrentRequestSeq() int {
	if c == nil {
		return 0
	}
	return c.requestSeq
}

func (c *defaultComposeFileSearchCoordinator) RememberSearchID(searchID string) {
	if c == nil {
		return
	}
	c.searchID = strings.TrimSpace(searchID)
}

func (defaultComposeFileSearchCloser) CloseAsync(service composeFileSearchService, searchID string) {
	searchID = strings.TrimSpace(searchID)
	if service == nil || searchID == "" {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		service.Close(ctx, searchID)
	}()
}

func (m *Model) composeFileSearchCoordinatorOrDefault() composeFileSearchCoordinator {
	if m == nil || m.composeFileSearchCoordinator == nil {
		if m == nil {
			return NewDefaultComposeFileSearchCoordinator()
		}
		m.composeFileSearchCoordinator = NewDefaultComposeFileSearchCoordinator()
		return m.composeFileSearchCoordinator
	}
	return m.composeFileSearchCoordinator
}

func (m *Model) composeFileSearchCloserOrDefault() composeFileSearchCloser {
	if m == nil || m.composeFileSearchCloser == nil {
		return defaultComposeFileSearchCloser{}
	}
	return m.composeFileSearchCloser
}
