package app

import (
	"context"
	"errors"
	"testing"
	"time"

	"control/internal/apicode"
	"control/internal/client"
	"control/internal/daemon/transcriptdomain"
	"control/internal/types"

	tea "charm.land/bubbletea/v2"
)

type composeFileSearchAPITestStub struct {
	startCalls  []client.StartFileSearchRequest
	updateCalls []struct {
		ID  string
		Req client.UpdateFileSearchRequest
	}
	closeCalls []string
	events     map[string]chan types.FileSearchEvent
	candidates map[string][]types.FileSearchCandidate
	nextID     string
	startErr   error
	updateErr  error
	eventsErr  error
	closeErr   error
	autoEvent  bool
	updateHook func(id, query string, ch chan types.FileSearchEvent)
}

func newComposeFileSearchAPITestStub() *composeFileSearchAPITestStub {
	return &composeFileSearchAPITestStub{
		events:     map[string]chan types.FileSearchEvent{},
		candidates: map[string][]types.FileSearchCandidate{},
		nextID:     "fs-1",
		autoEvent:  true,
	}
}

func (s *composeFileSearchAPITestStub) StartFileSearch(_ context.Context, req client.StartFileSearchRequest) (*types.FileSearchSession, error) {
	s.startCalls = append(s.startCalls, req)
	if s.startErr != nil {
		return nil, s.startErr
	}
	id := s.nextID
	if _, ok := s.events[id]; !ok {
		s.events[id] = make(chan types.FileSearchEvent, 8)
	}
	return &types.FileSearchSession{ID: id, Scope: req.Scope}, nil
}

func (s *composeFileSearchAPITestStub) UpdateFileSearch(_ context.Context, id string, req client.UpdateFileSearchRequest) (*types.FileSearchSession, error) {
	s.updateCalls = append(s.updateCalls, struct {
		ID  string
		Req client.UpdateFileSearchRequest
	}{ID: id, Req: req})
	if s.updateErr != nil {
		return nil, s.updateErr
	}
	query := ""
	if req.Query != nil {
		query = *req.Query
	}
	event := types.FileSearchEvent{
		Kind:       types.FileSearchEventResults,
		SearchID:   id,
		Query:      query,
		Candidates: append([]types.FileSearchCandidate(nil), s.candidates[query]...),
	}
	if ch, ok := s.events[id]; ok {
		if s.updateHook != nil {
			s.updateHook(id, query, ch)
		} else if s.autoEvent {
			ch <- event
		}
	}
	return &types.FileSearchSession{ID: id}, nil
}

func (s *composeFileSearchAPITestStub) CloseFileSearch(_ context.Context, id string) error {
	s.closeCalls = append(s.closeCalls, id)
	return s.closeErr
}

func (s *composeFileSearchAPITestStub) FileSearchEvents(_ context.Context, id string) (<-chan types.FileSearchEvent, func(), error) {
	if s.eventsErr != nil {
		return nil, func() {}, s.eventsErr
	}
	ch, ok := s.events[id]
	if !ok {
		ch = make(chan types.FileSearchEvent, 8)
		s.events[id] = ch
	}
	return ch, func() {}, nil
}

func runModelCmd(t *testing.T, m *Model, cmd tea.Cmd) {
	t.Helper()
	for cmd != nil {
		msg := cmd()
		if msg == nil {
			return
		}
		if batch, ok := msg.(tea.BatchMsg); ok {
			for _, batchedCmd := range batch {
				runModelCmd(t, m, batchedCmd)
			}
			return
		}
		nextModel, nextCmd := m.Update(msg)
		*m = asModel(t, nextModel)
		cmd = nextCmd
	}
}

func pressKey(t *testing.T, m *Model, msg tea.Msg) {
	t.Helper()
	nextModel, cmd := m.Update(msg)
	*m = asModel(t, nextModel)
	runModelCmd(t, m, cmd)
}

func TestActiveComposeFileSearchFragmentDetectsMention(t *testing.T) {
	value := "open @src/main before"
	cursor := len([]rune("open @src/ma"))
	fragment, ok := activeComposeFileSearchFragment(value, cursor)
	if !ok {
		t.Fatalf("expected active file-search fragment")
	}
	if fragment.Start != len([]rune("open ")) {
		t.Fatalf("unexpected start: %d", fragment.Start)
	}
	if fragment.End != len([]rune("open @src/main")) {
		t.Fatalf("unexpected end: %d", fragment.End)
	}
	if fragment.Query != "src/ma" {
		t.Fatalf("unexpected query: %q", fragment.Query)
	}
}

func TestTextInputReplaceRuneRangePreservesSuffix(t *testing.T) {
	input := NewTextInput(80, DefaultTextInputConfig())
	input.Focus()
	input.SetValue("before @src/ma after")
	cursor := len([]rune("before @src/ma"))
	input.MoveCursorToRuneIndex(cursor)
	fragment, ok := activeComposeFileSearchFragment(input.Value(), input.CursorRuneIndex())
	if !ok {
		t.Fatalf("expected active file-search fragment")
	}
	if !input.ReplaceRuneRange(fragment.Start, fragment.End, "@src/main.go") {
		t.Fatalf("expected replacement to apply")
	}
	if got := input.Value(); got != "before @src/main.go after" {
		t.Fatalf("unexpected replaced value: %q", got)
	}
}

func TestComposeFileSearchKeyboardSelectionExistingSession(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	api := newComposeFileSearchAPITestStub()
	api.candidates["ma"] = []types.FileSearchCandidate{
		{Path: "/repo/main.go", DisplayPath: "main.go"},
		{Path: "/repo/pkg/main_test.go", DisplayPath: "pkg/main_test.go"},
	}
	m.fileSearchAPI = api
	m.enterCompose("s1")

	nextModel, cmd := m.Update(tea.KeyPressMsg{Code: '@', Text: "@"})
	m = asModel(t, nextModel)
	if cmd != nil {
		runModelCmd(t, &m, cmd)
	}
	nextModel, _ = m.Update(tea.KeyPressMsg{Code: 'm', Text: "m"})
	m = asModel(t, nextModel)
	nextModel, cmd = m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	m = asModel(t, nextModel)
	runModelCmd(t, &m, cmd)

	if m.composeFileSearch == nil || !m.composeFileSearch.Open() {
		t.Fatalf("expected compose file autocomplete popup to open")
	}
	if len(api.startCalls) != 1 {
		t.Fatalf("expected one file search start call, got %d", len(api.startCalls))
	}
	if got := api.startCalls[0].Scope.SessionID; got != "s1" {
		t.Fatalf("expected existing-session scope, got %q", got)
	}
	if got := api.startCalls[0].Scope.Provider; got != "codex" {
		t.Fatalf("expected codex provider, got %q", got)
	}
	if len(api.updateCalls) != 1 || api.updateCalls[0].Req.Query == nil || *api.updateCalls[0].Req.Query != "ma" {
		t.Fatalf("expected update query ma, got %#v", api.updateCalls)
	}

	pressKey(t, &m, tea.KeyPressMsg{Code: tea.KeyDown})
	pressKey(t, &m, tea.KeyPressMsg{Code: tea.KeyTab})

	if got := m.chatInput.Value(); got != "@pkg/main_test.go " {
		t.Fatalf("expected selected mention insertion, got %q", got)
	}
	if m.composeFileSearch.Open() {
		t.Fatalf("expected popup to close after selection")
	}
}

func TestComposeFileSearchEscDismissesPopup(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	api := newComposeFileSearchAPITestStub()
	api.candidates["ma"] = []types.FileSearchCandidate{
		{Path: "/repo/main.go", DisplayPath: "main.go"},
	}
	m.fileSearchAPI = api
	m.enterCompose("s1")

	nextModel, cmd := m.Update(tea.KeyPressMsg{Code: '@', Text: "@"})
	m = asModel(t, nextModel)
	if cmd != nil {
		runModelCmd(t, &m, cmd)
	}
	nextModel, _ = m.Update(tea.KeyPressMsg{Code: 'm', Text: "m"})
	m = asModel(t, nextModel)
	nextModel, cmd = m.Update(tea.KeyPressMsg{Code: 'a', Text: "a"})
	m = asModel(t, nextModel)
	runModelCmd(t, &m, cmd)
	if !m.composeFileSearch.Open() {
		t.Fatalf("expected popup to open")
	}

	pressKey(t, &m, tea.KeyPressMsg{Code: tea.KeyEsc})

	if m.composeFileSearch.Open() {
		t.Fatalf("expected popup to close on esc")
	}
	if got := m.chatInput.Value(); got != "@ma" {
		t.Fatalf("expected input to remain unchanged, got %q", got)
	}
}

func TestComposeFileSearchCapabilityGatingUsesTranscriptCapabilities(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	api := newComposeFileSearchAPITestStub()
	m.fileSearchAPI = api
	m.setSessionTranscriptCapabilities("s1", transcriptdomain.CapabilityEnvelope{SupportsFileSearch: false})
	m.enterCompose("s1")

	nextModel, _ := m.Update(tea.KeyPressMsg{Code: '@', Text: "@"})
	m = asModel(t, nextModel)
	nextModel, cmd := m.Update(tea.KeyPressMsg{Code: 'm', Text: "m"})
	m = asModel(t, nextModel)
	runModelCmd(t, &m, cmd)

	if m.composeFileSearch != nil && m.composeFileSearch.Open() {
		t.Fatalf("did not expect popup for unsupported session capability")
	}
	if len(api.startCalls) != 0 || len(api.updateCalls) != 0 {
		t.Fatalf("did not expect backend file search calls when capability is disabled")
	}
	if got := m.chatInput.Value(); got != "@m" {
		t.Fatalf("expected plain text to remain, got %q", got)
	}
}

func TestComposeFileSearchUsesNewSessionScope(t *testing.T) {
	m := NewModel(nil)
	api := newComposeFileSearchAPITestStub()
	api.candidates["fi"] = []types.FileSearchCandidate{
		{Path: "/repo/file.go", DisplayPath: "file.go"},
	}
	m.fileSearchAPI = api
	m.newSession = &newSessionTarget{workspaceID: "ws1", worktreeID: "wt1"}
	_ = m.applyProviderSelection("codex")

	nextModel, cmd := m.Update(tea.KeyPressMsg{Code: '@', Text: "@"})
	m = asModel(t, nextModel)
	if cmd != nil {
		runModelCmd(t, &m, cmd)
	}
	nextModel, _ = m.Update(tea.KeyPressMsg{Code: 'f', Text: "f"})
	m = asModel(t, nextModel)
	nextModel, cmd = m.Update(tea.KeyPressMsg{Code: 'i', Text: "i"})
	m = asModel(t, nextModel)
	runModelCmd(t, &m, cmd)

	if len(api.startCalls) != 1 {
		t.Fatalf("expected one start call, got %d", len(api.startCalls))
	}
	scope := api.startCalls[0].Scope
	if scope.SessionID != "" {
		t.Fatalf("did not expect session scope for new session compose, got %q", scope.SessionID)
	}
	if scope.WorkspaceID != "ws1" || scope.WorktreeID != "wt1" {
		t.Fatalf("unexpected new session scope: %#v", scope)
	}
	if scope.Provider != "codex" {
		t.Fatalf("expected codex provider scope, got %q", scope.Provider)
	}
}

func TestDefaultComposeFileSearchServiceClassifiesUnsupportedError(t *testing.T) {
	api := newComposeFileSearchAPITestStub()
	api.startErr = &client.APIError{
		StatusCode: 400,
		Code:       apicode.ErrorCodeFileSearchUnsupported,
		Message:    "unsupported",
	}
	service := newDefaultComposeFileSearchService(api)

	result := service.Query(context.Background(), composeFileSearchQueryRequest{
		Scope: types.FileSearchScope{Provider: "claude"},
		Query: "main",
		Limit: 5,
	})

	if !result.Unsupported {
		t.Fatalf("expected unsupported result classification")
	}
	if result.Err == nil {
		t.Fatalf("expected original error to be preserved")
	}
}

func TestComposeFileSearchMentionTextFallsBackToPath(t *testing.T) {
	candidate := types.FileSearchCandidate{Path: "/repo/main.go"}
	if got := composeFileSearchMentionText(candidate); got != "@/repo/main.go" {
		t.Fatalf("unexpected mention text: %q", got)
	}
}

func TestReplaceComposeFileSearchFragmentDoesNotDoubleSpace(t *testing.T) {
	input := NewTextInput(80, DefaultTextInputConfig())
	input.Focus()
	input.SetValue("before @ma after")
	input.MoveCursorToRuneIndex(len([]rune("before @ma")))
	fragment, ok := activeComposeFileSearchFragment(input.Value(), input.CursorRuneIndex())
	if !ok {
		t.Fatalf("expected active file-search fragment")
	}

	if !replaceComposeFileSearchFragment(input, fragment, types.FileSearchCandidate{DisplayPath: "main.go"}) {
		t.Fatalf("expected replacement to apply")
	}
	if got := input.Value(); got != "before @main.go after" {
		t.Fatalf("unexpected replacement with existing whitespace: %q", got)
	}
}

func TestReplaceComposeFileSearchFragmentRejectsEmptyCandidate(t *testing.T) {
	if replaceComposeFileSearchFragment(nil, composeFileSearchFragment{}, types.FileSearchCandidate{DisplayPath: "main.go"}) {
		t.Fatalf("expected nil input to be rejected")
	}
	input := NewTextInput(80, DefaultTextInputConfig())
	if replaceComposeFileSearchFragment(input, composeFileSearchFragment{}, types.FileSearchCandidate{}) {
		t.Fatalf("expected empty candidate to be rejected")
	}
}

func TestDefaultComposeFileSearchServiceQueryBranches(t *testing.T) {
	t.Run("nil api", func(t *testing.T) {
		service := newDefaultComposeFileSearchService(nil)
		result := service.Query(context.Background(), composeFileSearchQueryRequest{Query: "main"})
		if result.Err == nil {
			t.Fatalf("expected missing api error")
		}
	})

	t.Run("missing search id from start", func(t *testing.T) {
		api := newComposeFileSearchAPITestStub()
		api.nextID = "   "
		service := newDefaultComposeFileSearchService(api)

		result := service.Query(context.Background(), composeFileSearchQueryRequest{
			Scope: types.FileSearchScope{Provider: "codex"},
			Query: "main",
		})
		if result.Err == nil {
			t.Fatalf("expected missing search id error")
		}
	})

	t.Run("events error", func(t *testing.T) {
		api := newComposeFileSearchAPITestStub()
		api.eventsErr = errors.New("events unavailable")
		service := newDefaultComposeFileSearchService(api)

		result := service.Query(context.Background(), composeFileSearchQueryRequest{
			Scope: types.FileSearchScope{Provider: "codex"},
			Query: "main",
		})
		if result.Err == nil || result.Err.Error() != "events unavailable" {
			t.Fatalf("expected events error, got %v", result.Err)
		}
	})

	t.Run("update error", func(t *testing.T) {
		api := newComposeFileSearchAPITestStub()
		api.updateErr = errors.New("update failed")
		service := newDefaultComposeFileSearchService(api)

		result := service.Query(context.Background(), composeFileSearchQueryRequest{
			Scope: types.FileSearchScope{Provider: "codex"},
			Query: "main",
		})
		if result.Err == nil || result.Err.Error() != "update failed" {
			t.Fatalf("expected update error, got %v", result.Err)
		}
	})

	t.Run("closed event", func(t *testing.T) {
		api := newComposeFileSearchAPITestStub()
		api.autoEvent = false
		api.updateHook = func(id, query string, ch chan types.FileSearchEvent) {
			ch <- types.FileSearchEvent{Kind: types.FileSearchEventClosed, SearchID: id}
		}
		service := newDefaultComposeFileSearchService(api)

		result := service.Query(context.Background(), composeFileSearchQueryRequest{
			Scope: types.FileSearchScope{Provider: "codex"},
			Query: "main",
		})
		if result.Err != nil {
			t.Fatalf("expected closed event to return without error, got %v", result.Err)
		}
		if result.SearchID == "" {
			t.Fatalf("expected search id to be preserved")
		}
	})

	t.Run("failed event", func(t *testing.T) {
		api := newComposeFileSearchAPITestStub()
		api.autoEvent = false
		api.updateHook = func(id, query string, ch chan types.FileSearchEvent) {
			ch <- types.FileSearchEvent{Kind: types.FileSearchEventFailed, SearchID: id, Error: "provider failed"}
		}
		service := newDefaultComposeFileSearchService(api)

		result := service.Query(context.Background(), composeFileSearchQueryRequest{
			Scope: types.FileSearchScope{Provider: "codex"},
			Query: "main",
		})
		if result.Err == nil || result.Err.Error() != "provider failed" {
			t.Fatalf("expected failed event error, got %v", result.Err)
		}
	})

	t.Run("mismatched events are skipped", func(t *testing.T) {
		api := newComposeFileSearchAPITestStub()
		api.autoEvent = false
		api.updateHook = func(id, query string, ch chan types.FileSearchEvent) {
			ch <- types.FileSearchEvent{Kind: types.FileSearchEventResults, SearchID: "other", Query: query}
			ch <- types.FileSearchEvent{Kind: types.FileSearchEventResults, SearchID: id, Query: "other"}
			ch <- types.FileSearchEvent{
				Kind:       types.FileSearchEventResults,
				SearchID:   id,
				Query:      query,
				Candidates: []types.FileSearchCandidate{{Path: "/repo/main.go", DisplayPath: "main.go"}},
			}
		}
		service := newDefaultComposeFileSearchService(api)

		result := service.Query(context.Background(), composeFileSearchQueryRequest{
			Scope: types.FileSearchScope{Provider: "codex"},
			Query: "main",
		})
		if result.Err != nil {
			t.Fatalf("expected final matching result, got %v", result.Err)
		}
		if len(result.Candidates) != 1 || result.Candidates[0].DisplayPath != "main.go" {
			t.Fatalf("unexpected candidates after skipping mismatched events: %#v", result.Candidates)
		}
	})

	t.Run("context canceled while waiting", func(t *testing.T) {
		api := newComposeFileSearchAPITestStub()
		api.autoEvent = false
		service := newDefaultComposeFileSearchService(api)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		result := service.Query(ctx, composeFileSearchQueryRequest{
			SearchID: "fs-1",
			Scope:    types.FileSearchScope{Provider: "codex"},
			Query:    "main",
		})
		if !errors.Is(result.Err, context.Canceled) {
			t.Fatalf("expected context canceled, got %v", result.Err)
		}
	})
}

func TestCloseComposeFileSearchServiceCmdInvokesClose(t *testing.T) {
	api := newComposeFileSearchAPITestStub()
	cmd := closeComposeFileSearchServiceCmd(newDefaultComposeFileSearchService(api), "fs-1")
	if cmd == nil {
		t.Fatalf("expected close command")
	}
	if msg := cmd(); msg != nil {
		t.Fatalf("expected close command to return nil message, got %T", msg)
	}
	if len(api.closeCalls) != 1 || api.closeCalls[0] != "fs-1" {
		t.Fatalf("expected close call for fs-1, got %#v", api.closeCalls)
	}
}

func TestComposeFileSearchPopupPlacementAndUnsupportedClose(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	api := newComposeFileSearchAPITestStub()
	m.fileSearchAPI = api
	m.enterCompose("s1")
	m.chatInput.SetValue("@ma")
	m.chatInput.MoveCursorToRuneIndex(len([]rune("@ma")))
	fragment, ok := activeComposeFileSearchFragment(m.chatInput.Value(), m.chatInput.CursorRuneIndex())
	if !ok {
		t.Fatalf("expected active fragment")
	}
	controller := m.composeFileSearchController()
	controller.SetFragment(fragment, true)
	controller.SetCandidates([]types.FileSearchCandidate{{Path: "/repo/main.go", DisplayPath: "main.go"}})
	controller.SetSearchID("fs-1")
	controller.NextRequestSeq()

	popup, _, _ := m.composeFileSearchPopupPlacement()
	if popup == "" {
		t.Fatalf("expected popup placement content")
	}

	cmd := m.applyComposeFileSearchResults(composeFileSearchResultsMsg{
		Seq:   controller.RequestSeq(),
		Query: fragment.Query,
		Result: composeFileSearchQueryResult{
			SearchID:    "fs-1",
			Unsupported: true,
			Err: &client.APIError{
				StatusCode: 400,
				Code:       apicode.ErrorCodeFileSearchUnsupported,
				Message:    "unsupported",
			},
		},
	})
	runModelCmd(t, &m, cmd)

	if controller.Open() {
		t.Fatalf("expected popup to close after unsupported result")
	}
	if len(api.closeCalls) != 1 || api.closeCalls[0] != "fs-1" {
		t.Fatalf("expected unsupported result to close search, got %#v", api.closeCalls)
	}
}

func TestComposeFileSearchResultHandlingBranches(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	api := newComposeFileSearchAPITestStub()
	m.fileSearchAPI = api
	m.enterCompose("s1")
	m.chatInput.SetValue("@ma")
	m.chatInput.MoveCursorToRuneIndex(len([]rune("@ma")))
	fragment, ok := activeComposeFileSearchFragment(m.chatInput.Value(), m.chatInput.CursorRuneIndex())
	if !ok {
		t.Fatalf("expected active fragment")
	}
	controller := m.composeFileSearchController()
	controller.SetFragment(fragment, true)
	controller.SetSearchID("fs-1")
	controller.NextRequestSeq()

	if cmd := m.applyComposeFileSearchResults(composeFileSearchResultsMsg{
		Seq:   controller.RequestSeq() + 1,
		Query: fragment.Query,
		Result: composeFileSearchQueryResult{
			SearchID:   "fs-1",
			Candidates: []types.FileSearchCandidate{{Path: "/repo/ignored.go", DisplayPath: "ignored.go"}},
		},
	}); cmd != nil {
		t.Fatalf("expected stale seq to be ignored")
	}

	cmd := m.applyComposeFileSearchResults(composeFileSearchResultsMsg{
		Seq:   controller.RequestSeq(),
		Query: fragment.Query,
		Result: composeFileSearchQueryResult{
			SearchID: "fs-1",
			Err:      context.Canceled,
		},
	})
	if cmd != nil {
		t.Fatalf("expected canceled result to be ignored")
	}
}

func TestHandleComposeFileSearchKeyBranches(t *testing.T) {
	m := NewModel(nil)
	if handled, _ := m.handleComposeFileSearchKey("enter"); handled {
		t.Fatalf("expected closed popup to ignore keys")
	}

	m.composeFileSearch.SetCandidates([]types.FileSearchCandidate{{Path: "/repo/main.go", DisplayPath: "main.go"}})
	if handled, _ := m.handleComposeFileSearchKey("x"); handled {
		t.Fatalf("expected unknown key to fall through")
	}
}

func TestComposeFileSearchQueryCmdReturnsServiceResult(t *testing.T) {
	api := newComposeFileSearchAPITestStub()
	service := newDefaultComposeFileSearchService(api)
	api.candidates["ma"] = []types.FileSearchCandidate{{Path: "/repo/main.go", DisplayPath: "main.go"}}

	cmd := queryComposeFileSearchCmd(service, composeFileSearchQueryRequest{
		Scope: types.FileSearchScope{Provider: "codex"},
		Query: "ma",
		Limit: 5,
	}, 7, context.Background())
	msg, ok := cmd().(composeFileSearchResultsMsg)
	if !ok {
		t.Fatalf("expected compose file search results msg")
	}
	if msg.Seq != 7 || msg.Query != "ma" {
		t.Fatalf("unexpected command result metadata: %#v", msg)
	}
	if len(msg.Result.Candidates) != 1 {
		t.Fatalf("expected candidate result, got %#v", msg.Result.Candidates)
	}
}

func TestComposeFileSearchDebounceCmdEmitsMessage(t *testing.T) {
	cmd := composeFileSearchDebounceCmd(3, time.Nanosecond)
	msg, ok := cmd().(composeFileSearchDebounceMsg)
	if !ok {
		t.Fatalf("expected debounce msg")
	}
	if msg.Seq != 3 {
		t.Fatalf("unexpected debounce seq: %d", msg.Seq)
	}
}
