package app

import (
	"strings"

	"control/internal/types"

	tea "charm.land/bubbletea/v2"
)

type ComposeFileAutocompleteController struct {
	picker       *SelectPicker
	searchID     string
	fragment     composeFileSearchFragment
	fragmentOpen bool
	open         bool
	candidates   map[string]types.FileSearchCandidate
	requestSeq   int
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

func (c *ComposeFileAutocompleteController) NextRequestSeq() int {
	if c == nil {
		return 0
	}
	c.requestSeq++
	return c.requestSeq
}

func (c *ComposeFileAutocompleteController) RequestSeq() int {
	if c == nil {
		return 0
	}
	return c.requestSeq
}

func (c *ComposeFileAutocompleteController) SetSearchID(id string) {
	if c == nil {
		return
	}
	c.searchID = strings.TrimSpace(id)
}

func (c *ComposeFileAutocompleteController) SearchID() string {
	if c == nil {
		return ""
	}
	return strings.TrimSpace(c.searchID)
}

func (c *ComposeFileAutocompleteController) SetFragment(fragment composeFileSearchFragment, ok bool) {
	if c == nil {
		return
	}
	c.fragment = fragment
	c.fragmentOpen = ok
	if !ok {
		c.open = false
	}
}

func (c *ComposeFileAutocompleteController) Fragment() (composeFileSearchFragment, bool) {
	if c == nil || !c.fragmentOpen {
		return composeFileSearchFragment{}, false
	}
	return c.fragment, true
}

func (c *ComposeFileAutocompleteController) ClosePopup() {
	if c == nil {
		return
	}
	c.open = false
	if c.picker != nil {
		c.picker.SetOptions(nil)
	}
	c.candidates = map[string]types.FileSearchCandidate{}
}

func (c *ComposeFileAutocompleteController) Reset() {
	if c == nil {
		return
	}
	c.searchID = ""
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
	c.candidates = map[string]types.FileSearchCandidate{}
	if len(candidates) == 0 {
		c.open = false
		c.picker.SetOptions(nil)
		return
	}
	options := make([]selectOption, 0, len(candidates))
	for _, candidate := range candidates {
		id := strings.TrimSpace(candidate.Path)
		if id == "" {
			continue
		}
		label := strings.TrimSpace(candidate.DisplayPath)
		if label == "" {
			label = id
		}
		options = append(options, selectOption{
			id:     id,
			label:  label,
			search: label + " " + id,
		})
		c.candidates[id] = candidate
	}
	if len(options) == 0 {
		c.open = false
		c.picker.SetOptions(nil)
		return
	}
	c.picker.SetQuery("")
	c.picker.SetOptions(options)
	c.open = true
}

func (c *ComposeFileAutocompleteController) View() string {
	if c == nil || c.picker == nil || !c.open {
		return ""
	}
	return c.picker.View()
}

func (m *Model) composeFileSearchController() *ComposeFileAutocompleteController {
	if m == nil {
		return nil
	}
	return m.composeFileSearch
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
	if m == nil || controller == nil {
		return ""
	}
	m.cancelRequestScope(requestScopeComposeFileSearch)
	searchID := controller.SearchID()
	controller.Reset()
	return searchID
}

func (m *Model) syncComposeFileSearchAfterInput() tea.Cmd {
	controller := m.composeFileSearchController()
	if m == nil || controller == nil || m.mode != uiModeCompose || m.chatInput == nil {
		return nil
	}
	if m.composeOptionPickerOpen() {
		controller.ClosePopup()
		return nil
	}
	if !m.composeFileSearchSupported() {
		m.resetComposeFileSearch()
		return nil
	}
	if _, ok := m.composeFileSearchScope(); !ok {
		m.resetComposeFileSearch()
		return nil
	}
	previous, previousOK := controller.Fragment()
	fragment, ok := activeComposeFileSearchFragment(m.chatInput.Value(), m.chatInput.CursorRuneIndex())
	controller.SetFragment(fragment, ok)
	if !ok {
		searchID := m.resetComposeFileSearch()
		return closeComposeFileSearchServiceCmd(m.composeFileSearchServiceOrDefault(), searchID)
	}
	if fragment.Query == "" && controller.SearchID() == "" {
		controller.ClosePopup()
		return nil
	}
	if previousOK &&
		previous.Start == fragment.Start &&
		previous.End == fragment.End &&
		previous.Query == fragment.Query {
		return nil
	}
	controller.ClosePopup()
	seq := controller.NextRequestSeq()
	return composeFileSearchDebounceCmd(seq, composeFileSearchDebounceDelay)
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
	if m == nil || controller == nil || msg.Seq != controller.RequestSeq() {
		return nil
	}
	fragment, ok := controller.Fragment()
	if !ok {
		return nil
	}
	scope, ok := m.composeFileSearchScope()
	if !ok {
		return nil
	}
	ctx := m.replaceRequestScope(requestScopeComposeFileSearch)
	return queryComposeFileSearchCmd(
		m.composeFileSearchServiceOrDefault(),
		composeFileSearchQueryRequest{
			SearchID: controller.SearchID(),
			Scope:    scope,
			Query:    fragment.Query,
			Limit:    composeFileSearchLimit,
		},
		msg.Seq,
		ctx,
	)
}

func (m *Model) applyComposeFileSearchResults(msg composeFileSearchResultsMsg) tea.Cmd {
	controller := m.composeFileSearchController()
	if m == nil || controller == nil || msg.Seq != controller.RequestSeq() {
		return nil
	}
	controller.SetSearchID(msg.Result.SearchID)
	fragment, ok := controller.Fragment()
	if !ok || fragment.Query != msg.Query {
		return nil
	}
	if msg.Result.Err != nil {
		if isCanceledRequestError(msg.Result.Err) {
			return nil
		}
		controller.ClosePopup()
		if msg.Result.Unsupported {
			return m.closeComposeFileSearchCmd()
		}
		return nil
	}
	controller.SetCandidates(msg.Result.Candidates)
	return nil
}

func (m *Model) handleComposeFileSearchKey(key string) (bool, tea.Cmd) {
	controller := m.composeFileSearchController()
	if controller == nil || !controller.Open() {
		return false, nil
	}
	switch key {
	case "esc":
		controller.ClosePopup()
		return true, nil
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
