package app

import (
	"strings"

	"control/internal/types"

	tea "charm.land/bubbletea/v2"
)

type ComposeFileAutocompleteController struct {
	picker       *SelectPicker
	fragment     composeFileSearchFragment
	fragmentOpen bool
	open         bool
	loading      bool
	candidates   map[string]types.FileSearchCandidate
	candidateIDs []string
}

func NewComposeFileAutocompleteController(width, height int) *ComposeFileAutocompleteController {
	return &ComposeFileAutocompleteController{
		picker:     NewSelectPicker(width, height),
		candidates: map[string]types.FileSearchCandidate{},
	}
}

func (c *ComposeFileAutocompleteController) SetSize(width, height int) {
	if c == nil || c.picker == nil {
		return
	}
	c.picker.SetSize(width, height)
}

func (c *ComposeFileAutocompleteController) SetFragment(fragment composeFileSearchFragment, ok bool) {
	if c == nil {
		return
	}
	c.fragment = fragment
	c.fragmentOpen = ok
	if !ok {
		c.open = false
		c.loading = false
		if c.picker != nil {
			c.picker.SetOptions(nil)
		}
		return
	}
	c.refreshPicker()
}

func (c *ComposeFileAutocompleteController) Fragment() (composeFileSearchFragment, bool) {
	if c == nil || !c.fragmentOpen {
		return composeFileSearchFragment{}, false
	}
	return c.fragment, true
}

func (c *ComposeFileAutocompleteController) SetLoading(loading bool) {
	if c == nil {
		return
	}
	c.loading = loading
	c.refreshPicker()
}

func (c *ComposeFileAutocompleteController) Loading() bool {
	return c != nil && c.loading
}

func (c *ComposeFileAutocompleteController) ClosePopup() {
	if c == nil {
		return
	}
	c.open = false
	c.loading = false
	if c.picker != nil {
		c.picker.SetOptions(nil)
	}
	c.candidates = map[string]types.FileSearchCandidate{}
	c.candidateIDs = nil
}

func (c *ComposeFileAutocompleteController) Reset() {
	if c == nil {
		return
	}
	c.fragment = composeFileSearchFragment{}
	c.fragmentOpen = false
	c.ClosePopup()
}

func (c *ComposeFileAutocompleteController) Open() bool {
	return c != nil && c.open
}

func (c *ComposeFileAutocompleteController) Move(delta int) {
	if c == nil || c.picker == nil || !c.open {
		return
	}
	c.picker.Move(delta)
}

func (c *ComposeFileAutocompleteController) HandleClick(row int) bool {
	if c == nil || c.picker == nil || !c.open {
		return false
	}
	return c.picker.HandleClick(row)
}

func (c *ComposeFileAutocompleteController) SelectedCandidate() (types.FileSearchCandidate, bool) {
	if c == nil || c.picker == nil || !c.open {
		return types.FileSearchCandidate{}, false
	}
	id := c.picker.SelectedID()
	if id == "" {
		return types.FileSearchCandidate{}, false
	}
	candidate, ok := c.candidates[id]
	return candidate, ok
}

func (c *ComposeFileAutocompleteController) SetCandidates(candidates []types.FileSearchCandidate) {
	if c == nil || c.picker == nil {
		return
	}
	selectedID := strings.TrimSpace(c.picker.SelectedID())
	c.candidates = map[string]types.FileSearchCandidate{}
	c.candidateIDs = c.candidateIDs[:0]
	for _, candidate := range candidates {
		id := strings.TrimSpace(candidate.Path)
		if id == "" {
			continue
		}
		c.candidates[id] = candidate
		c.candidateIDs = append(c.candidateIDs, id)
	}
	c.refreshPicker()
	if selectedID != "" && c.picker != nil {
		_ = c.picker.SelectID(selectedID)
	}
}

func (c *ComposeFileAutocompleteController) View() string {
	if c == nil || c.picker == nil || !c.open {
		return ""
	}
	view := c.picker.View()
	if supplemental := composeFileSearchSupplementalView(c.loading, len(c.candidates)); supplemental != "" {
		return view + "\n" + supplemental
	}
	return view
}

func (c *ComposeFileAutocompleteController) refreshPicker() {
	if c == nil || c.picker == nil {
		return
	}
	if !c.fragmentOpen {
		c.open = false
		c.picker.SetOptions(nil)
		return
	}
	query := strings.TrimSpace(c.fragment.Query)
	if query == "" && !c.loading && len(c.candidates) == 0 {
		c.open = false
		c.picker.SetOptions(nil)
		return
	}
	options := composeFileSearchPickerOptions(c.candidateIDs, c.candidates, c.loading)
	c.picker.SetQuery("")
	c.picker.SetOptions(options)
	c.open = true
}

func (m *Model) composeFileSearchController() *ComposeFileAutocompleteController {
	if m == nil {
		return nil
	}
	return m.composeFileSearch
}

func (m *Model) composeFileSearchStreamController() *ComposeFileSearchStreamController {
	if m == nil {
		return nil
	}
	return m.composeFileSearchStream
}

func (m *Model) syncComposeFileSearchControllerFromCoordinator() {
	controller := m.composeFileSearchController()
	coordinator := m.composeFileSearchCoordinatorOrDefault()
	if controller == nil || coordinator == nil {
		return
	}
	fragment, ok := coordinator.ActiveFragment()
	controller.SetFragment(fragment, ok)
}

func (m *Model) composeFileSearchServiceOrDefault() composeFileSearchService {
	if m == nil {
		return newDefaultComposeFileSearchService(nil)
	}
	return newDefaultComposeFileSearchService(m.fileSearchAPI)
}

func (m *Model) composeFileSearchSupported() bool {
	return m.composeFileSearchContextResolverOrDefault().ResolveComposeFileSearchContext(m).Supported
}

func (m *Model) composeFileSearchScope() (types.FileSearchScope, bool) {
	ctx := m.composeFileSearchContextResolverOrDefault().ResolveComposeFileSearchContext(m)
	return ctx.Scope, ctx.HasScope
}

func composeFileSearchScopeKey(scope types.FileSearchScope) string {
	return strings.TrimSpace(scope.Provider) + "|" +
		strings.TrimSpace(scope.SessionID) + "|" +
		strings.TrimSpace(scope.WorkspaceID) + "|" +
		strings.TrimSpace(scope.WorktreeID) + "|" +
		strings.TrimSpace(scope.Cwd)
}

func (m *Model) composeFileSearchPopupPlacement() (string, int, int) {
	controller := m.composeFileSearchController()
	if m == nil || controller == nil || !controller.Open() {
		return "", 0, 0
	}
	view := controller.View()
	if strings.TrimSpace(view) == "" {
		return "", 0, 0
	}
	height := len(strings.Split(view, "\n"))
	row := m.composeControlsRow() - height
	if row < 1 {
		row = 1
	}
	return view, m.resolveMouseLayout().rightStart, row
}

func (m *Model) resetComposeFileSearch() string {
	controller := m.composeFileSearchController()
	coordinator := m.composeFileSearchCoordinatorOrDefault()
	if m == nil || controller == nil || coordinator == nil {
		return ""
	}
	m.cancelRequestScope(requestScopeComposeFileSearch)
	m.cancelRequestScope(requestScopeComposeFileSearchUpdate)
	if stream := m.composeFileSearchStreamController(); stream != nil {
		stream.Reset()
	}
	searchID := coordinator.Reset()
	controller.Reset()
	return searchID
}

func (m *Model) syncComposeFileSearchAfterInput() tea.Cmd {
	controller := m.composeFileSearchController()
	coordinator := m.composeFileSearchCoordinatorOrDefault()
	if m == nil || controller == nil || coordinator == nil || m.mode != uiModeCompose || m.chatInput == nil {
		return nil
	}
	if m.composeOptionPickerOpen() {
		controller.ClosePopup()
		return nil
	}
	if !m.composeFileSearchSupported() {
		searchID := m.resetComposeFileSearch()
		return closeComposeFileSearchServiceCmd(m.composeFileSearchServiceOrDefault(), searchID)
	}
	scope, ok := m.composeFileSearchScope()
	if !ok {
		searchID := m.resetComposeFileSearch()
		return closeComposeFileSearchServiceCmd(m.composeFileSearchServiceOrDefault(), searchID)
	}
	scopeKey := composeFileSearchScopeKey(scope)
	fragment, ok := activeComposeFileSearchFragment(m.chatInput.Value(), m.chatInput.CursorRuneIndex())
	transition := coordinator.SyncInput(scopeKey, fragment, ok)
	m.syncComposeFileSearchControllerFromCoordinator()
	if transition.LoadingChanged {
		controller.SetLoading(transition.Loading)
	}
	if transition.ClosePopup {
		controller.ClosePopup()
	}
	cmds := make([]tea.Cmd, 0, 2)
	if transition.CancelUpdateScope {
		m.cancelRequestScope(requestScopeComposeFileSearchUpdate)
	}
	if transition.SearchIDToClose != "" {
		cmds = append(cmds, closeComposeFileSearchServiceCmd(m.composeFileSearchServiceOrDefault(), transition.SearchIDToClose))
	}
	if transition.ReplaceLifecycleScope {
		m.replaceRequestScope(requestScopeComposeFileSearch)
	}
	if transition.EnsureLifecycleScope && !m.hasRequestScope(requestScopeComposeFileSearch) {
		m.replaceRequestScope(requestScopeComposeFileSearch)
	}
	if transition.ScheduleDebounce {
		cmds = append(cmds, composeFileSearchDebounceCmd(transition.Seq, composeFileSearchDebounceDelay))
	}
	if len(cmds) == 0 {
		return nil
	}
	return tea.Batch(cmds...)
}

func (m *Model) applyComposeFileSearchSelection() tea.Cmd {
	controller := m.composeFileSearchController()
	if m == nil || controller == nil || m.chatInput == nil {
		return nil
	}
	fragment, ok := controller.Fragment()
	if !ok {
		controller.ClosePopup()
		return nil
	}
	candidate, ok := controller.SelectedCandidate()
	if !ok {
		controller.ClosePopup()
		return nil
	}
	if !replaceComposeFileSearchFragment(m.chatInput, fragment, candidate) {
		controller.ClosePopup()
		return nil
	}
	controller.ClosePopup()
	return nil
}

func (m *Model) closeComposeFileSearchCmd() tea.Cmd {
	if m == nil {
		return nil
	}
	searchID := m.resetComposeFileSearch()
	return closeComposeFileSearchServiceCmd(m.composeFileSearchServiceOrDefault(), searchID)
}

func (m *Model) handleComposeFileSearchDebounce(msg composeFileSearchDebounceMsg) tea.Cmd {
	controller := m.composeFileSearchController()
	coordinator := m.composeFileSearchCoordinatorOrDefault()
	if m == nil || controller == nil || coordinator == nil {
		return nil
	}
	transition, ok := coordinator.PrepareDebounce(msg.Seq)
	if !ok {
		return nil
	}
	scope, ok := m.composeFileSearchScope()
	if !ok {
		return nil
	}
	if transition.LoadingChanged {
		controller.SetLoading(transition.Loading)
	}
	if strings.TrimSpace(transition.Query) == "" {
		return nil
	}
	ctx := m.replaceRequestScope(requestScopeComposeFileSearchUpdate)
	if transition.Start {
		return startComposeFileSearchCmd(
			m.composeFileSearchServiceOrDefault(),
			composeFileSearchStartRequest{
				Scope: scope,
				Query: transition.Query,
				Limit: composeFileSearchLimit,
			},
			msg.Seq,
			ctx,
		)
	}
	return updateComposeFileSearchCmd(
		m.composeFileSearchServiceOrDefault(),
		composeFileSearchUpdateRequest{
			SearchID: transition.SearchID,
			Scope:    scope,
			Query:    transition.Query,
			Limit:    composeFileSearchLimit,
		},
		msg.Seq,
		ctx,
	)
}

func (m *Model) applyComposeFileSearchStarted(msg composeFileSearchStartedMsg) tea.Cmd {
	controller := m.composeFileSearchController()
	coordinator := m.composeFileSearchCoordinatorOrDefault()
	if m == nil || controller == nil || coordinator == nil {
		return nil
	}
	transition := coordinator.ApplyStarted(msg.Seq, msg.Query, msg.Result)
	m.syncComposeFileSearchControllerFromCoordinator()
	if transition.LoadingChanged {
		controller.SetLoading(transition.Loading)
	}
	if transition.ClosePopup {
		controller.ClosePopup()
	}
	if transition.SearchIDToClose != "" && !transition.OpenStream {
		return closeComposeFileSearchServiceCmd(m.composeFileSearchServiceOrDefault(), transition.SearchIDToClose)
	}
	if transition.Unsupported {
		return m.closeComposeFileSearchCmd()
	}
	if !transition.OpenStream {
		return nil
	}
	return openComposeFileSearchStreamCmd(
		m.composeFileSearchServiceOrDefault(),
		transition.SearchID,
		m.requestScopeContext(requestScopeComposeFileSearch),
	)
}

func (m *Model) applyComposeFileSearchUpdated(msg composeFileSearchUpdatedMsg) tea.Cmd {
	controller := m.composeFileSearchController()
	coordinator := m.composeFileSearchCoordinatorOrDefault()
	if m == nil || controller == nil || coordinator == nil {
		return nil
	}
	transition := coordinator.ApplyUpdated(msg.Seq, msg.Query, msg.Result)
	m.syncComposeFileSearchControllerFromCoordinator()
	if transition.LoadingChanged {
		controller.SetLoading(transition.Loading)
	}
	if transition.ClosePopup {
		controller.ClosePopup()
	}
	if transition.SearchIDToClose != "" {
		return closeComposeFileSearchServiceCmd(m.composeFileSearchServiceOrDefault(), transition.SearchIDToClose)
	}
	if transition.Unsupported {
		return m.closeComposeFileSearchCmd()
	}
	return nil
}

func (m *Model) applyComposeFileSearchStream(msg composeFileSearchStreamMsg) tea.Cmd {
	controller := m.composeFileSearchController()
	stream := m.composeFileSearchStreamController()
	coordinator := m.composeFileSearchCoordinatorOrDefault()
	if m == nil || controller == nil || stream == nil || coordinator == nil {
		if msg.Cancel != nil {
			msg.Cancel()
		}
		return nil
	}
	transition := coordinator.ApplyStreamOpen(msg.SearchID, msg.Err)
	m.syncComposeFileSearchControllerFromCoordinator()
	if transition.LoadingChanged {
		controller.SetLoading(transition.Loading)
	}
	if transition.ClosePopup {
		controller.ClosePopup()
	}
	if transition.SearchIDToClose != "" && !transition.Accept {
		if msg.Cancel != nil {
			msg.Cancel()
		}
		return closeComposeFileSearchServiceCmd(m.composeFileSearchServiceOrDefault(), transition.SearchIDToClose)
	}
	if !transition.Accept {
		if msg.Cancel != nil {
			msg.Cancel()
		}
		return nil
	}
	stream.SetStream(msg.SearchID, msg.Ch, msg.Cancel)
	return nil
}

func (m *Model) applyComposeFileSearchResults(msg composeFileSearchResultsMsg) tea.Cmd {
	coordinator := m.composeFileSearchCoordinatorOrDefault()
	if m == nil || coordinator == nil || msg.Seq != coordinator.CurrentRequestSeq() {
		return nil
	}
	coordinator.RememberSearchID(msg.Result.SearchID)
	fragment, ok := coordinator.ActiveFragment()
	if !ok || fragment.Query != msg.Query {
		return nil
	}
	controller := m.composeFileSearchController()
	if msg.Result.Err != nil {
		if isCanceledRequestError(msg.Result.Err) {
			return nil
		}
		controller.SetLoading(false)
		controller.ClosePopup()
		if msg.Result.Unsupported {
			return m.closeComposeFileSearchCmd()
		}
		return nil
	}
	controller.SetLoading(false)
	controller.SetCandidates(msg.Result.Candidates)
	return nil
}

func (m *Model) applyComposeFileSearchEvent(event types.FileSearchEvent) tea.Cmd {
	controller := m.composeFileSearchController()
	stream := m.composeFileSearchStreamController()
	coordinator := m.composeFileSearchCoordinatorOrDefault()
	if m == nil || controller == nil || stream == nil || coordinator == nil {
		return nil
	}
	transition := coordinator.ApplyEvent(event)
	m.syncComposeFileSearchControllerFromCoordinator()
	if transition.LoadingChanged {
		controller.SetLoading(transition.Loading)
	}
	if transition.ClosePopup {
		controller.ClosePopup()
	}
	if transition.ApplyCandidates {
		controller.SetCandidates(transition.Candidates)
	}
	if transition.SearchIDToClose != "" {
		return closeComposeFileSearchServiceCmd(m.composeFileSearchServiceOrDefault(), transition.SearchIDToClose)
	}
	return nil
}

func (m *Model) consumeComposeFileSearchTick() tea.Cmd {
	stream := m.composeFileSearchStreamController()
	if m == nil || stream == nil {
		return nil
	}
	events, changed, closed := stream.ConsumeTick()
	if !changed && !closed {
		return nil
	}
	cmds := make([]tea.Cmd, 0, len(events))
	for _, event := range events {
		if cmd := m.applyComposeFileSearchEvent(event); cmd != nil {
			cmds = append(cmds, cmd)
		}
	}
	if closed {
		if controller := m.composeFileSearchController(); controller != nil {
			transition := m.composeFileSearchCoordinatorOrDefault().HandleStreamClosed()
			if transition.LoadingChanged {
				controller.SetLoading(transition.Loading)
			}
		}
	}
	if len(cmds) == 0 {
		return nil
	}
	return tea.Batch(cmds...)
}

func (m *Model) handleComposeFileSearchKey(key string) (bool, tea.Cmd) {
	controller := m.composeFileSearchController()
	if controller == nil || !controller.Open() {
		return false, nil
	}
	switch key {
	case "esc":
		return true, m.closeComposeFileSearchCmd()
	case "tab", "enter":
		return true, m.applyComposeFileSearchSelection()
	case "up":
		controller.Move(-1)
		return true, nil
	case "down":
		controller.Move(1)
		return true, nil
	default:
		return false, nil
	}
}
