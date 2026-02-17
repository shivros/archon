package app

import (
	"fmt"
	"sort"
	"strings"

	tea "charm.land/bubbletea/v2"
	xansi "github.com/charmbracelet/x/ansi"

	"control/internal/types"
)

const (
	notesPanelMinWidth = 28
	notesPanelMaxWidth = 56
)

func (m *Model) toggleNotesPanel() tea.Cmd {
	m.startUILatencyAction(uiLatencyActionToggleNotesSidebar, "")
	defer m.finishUILatencyAction(uiLatencyActionToggleNotesSidebar, "", uiLatencyOutcomeOK)

	m.notesPanelOpen = !m.notesPanelOpen
	if m.notesPanelOpen {
		m.setStatusMessage("notes panel opened")
		m.resizeWithoutRender(m.width, m.height)
		syncCmd := m.syncNotesPanelToCurrentSelection(true)
		if syncCmd != nil {
			return tea.Batch(syncCmd, notesPanelReflowCmd())
		}
		return notesPanelReflowCmd()
	}
	m.setStatusMessage("notes panel closed")
	m.resize(m.width, m.height)
	return nil
}

func (m *Model) batchWithNotesPanelSync(cmd tea.Cmd) tea.Cmd {
	panelCmd := m.syncNotesPanelToCurrentSelection(false)
	if cmd != nil && panelCmd != nil {
		return tea.Batch(cmd, panelCmd)
	}
	if panelCmd != nil {
		return panelCmd
	}
	return cmd
}

func (m *Model) syncNotesPanelToCurrentSelection(force bool) tea.Cmd {
	if !m.notesPanelOpen {
		return nil
	}
	scope, ok := m.currentNoteScope()
	if !ok {
		m.notesScope = noteScopeTarget{}
		m.notesByScope = map[types.NoteScope][]*types.Note{}
		m.resetNotesPanelLoadState()
		m.notes = nil
		m.notesPanelBlocks = []ChatBlock{
			{
				ID:   "notes-panel-empty",
				Role: ChatRoleSystem,
				Text: "No notes scope for this selection.",
			},
		}
		m.renderNotesPanel()
		return nil
	}
	if !force && noteScopeEqual(scope, m.notesScope) {
		return nil
	}
	m.setNotesRootScope(scope)
	return m.refreshNotesForCurrentScope()
}

func (m *Model) setNotesRootScope(scope noteScopeTarget) {
	if m.mode == uiModeAddNote && !noteScopeEqual(scope, m.notesScope) {
		m.saveCurrentNoteDraft()
	}
	m.notesScope = scope
	m.notesByScope = map[types.NoteScope][]*types.Note{}
	m.resetNotesPanelLoadState()
	m.notes = nil
	m.notesFilters = defaultNotesFilterState(scope)
	m.renderNotesViewsFromState()
}

func defaultNotesFilterState(scope noteScopeTarget) notesFilterState {
	return notesFilterState{
		ShowWorkspace: isNotesScopeAvailable(scope, types.NoteScopeWorkspace),
		ShowWorktree:  isNotesScopeAvailable(scope, types.NoteScopeWorktree),
		ShowSession:   isNotesScopeAvailable(scope, types.NoteScopeSession),
	}
}

func isNotesScopeAvailable(root noteScopeTarget, scope types.NoteScope) bool {
	switch root.Scope {
	case types.NoteScopeWorkspace:
		return scope == types.NoteScopeWorkspace
	case types.NoteScopeWorktree:
		return scope == types.NoteScopeWorkspace || scope == types.NoteScopeWorktree
	case types.NoteScopeSession:
		return scope == types.NoteScopeWorkspace || scope == types.NoteScopeWorktree || scope == types.NoteScopeSession
	default:
		return false
	}
}

func (m *Model) refreshNotesForCurrentScope() tea.Cmd {
	if m.notesScope.IsZero() {
		m.resetNotesPanelLoadState()
		return nil
	}
	requests := notesScopeRequestsForRoot(m.notesScope)
	if len(requests) == 0 {
		m.resetNotesPanelLoadState()
		return nil
	}
	m.startNotesPanelLoadScopes(requests, true)
	cmds := make([]tea.Cmd, 0, len(requests))
	for _, scope := range requests {
		cmds = append(cmds, fetchNotesCmd(m.notesAPI, scope))
	}
	if len(cmds) == 1 {
		return cmds[0]
	}
	return tea.Batch(cmds...)
}

func notesScopeRequestsForRoot(root noteScopeTarget) []noteScopeTarget {
	requests := []noteScopeTarget{}
	if isNotesScopeAvailable(root, types.NoteScopeWorkspace) {
		requests = append(requests, noteScopeRequestForRoot(root, types.NoteScopeWorkspace))
	}
	if isNotesScopeAvailable(root, types.NoteScopeWorktree) {
		requests = append(requests, noteScopeRequestForRoot(root, types.NoteScopeWorktree))
	}
	if isNotesScopeAvailable(root, types.NoteScopeSession) {
		requests = append(requests, noteScopeRequestForRoot(root, types.NoteScopeSession))
	}
	return requests
}

func noteScopeRequestForRoot(root noteScopeTarget, scope types.NoteScope) noteScopeTarget {
	req := noteScopeTarget{Scope: scope}
	switch scope {
	case types.NoteScopeWorkspace:
		req.WorkspaceID = root.WorkspaceID
	case types.NoteScopeWorktree:
		req.WorkspaceID = root.WorkspaceID
		req.WorktreeID = root.WorktreeID
	case types.NoteScopeSession:
		req.WorkspaceID = root.WorkspaceID
		req.WorktreeID = root.WorktreeID
		req.SessionID = root.SessionID
	}
	return req
}

func (m *Model) applyNotesScopeResult(scope noteScopeTarget, notes []*types.Note) bool {
	if m.notesScope.IsZero() {
		return false
	}
	expected := noteScopeRequestForRoot(m.notesScope, scope.Scope)
	if !noteScopeEqual(expected, scope) {
		return false
	}
	m.notesByScope[scope.Scope] = append([]*types.Note(nil), notes...)
	m.renderNotesViewsFromState()
	return true
}

func (m *Model) renderNotesViewsFromState() {
	m.notes = m.filteredNotesForCurrentScope()
	if m.mode == uiModeNotes || m.mode == uiModeAddNote {
		notesNewKey := m.keyForCommand(KeyCommandNotesNew, "n")
		m.setSnapshotBlocks(notesToBlocksWithNewKey(m.notes, m.notesScope, m.notesFilters, notesNewKey))
	}
	if m.notesPanelOpen {
		m.notesPanelBlocks = notesPanelBlocksFromState(m.notes, m.notesScope, m.notesFilters, m.notesPanelLoadState())
		m.renderNotesPanel()
	}
}

func (m *Model) filteredNotesForCurrentScope() []*types.Note {
	if m.notesScope.IsZero() || !hasAnyNotesFilterEnabled(m.notesScope, m.notesFilters) {
		return []*types.Note{}
	}
	merged := map[string]*types.Note{}
	for _, scope := range []types.NoteScope{types.NoteScopeWorkspace, types.NoteScopeWorktree, types.NoteScopeSession} {
		if !isNotesScopeAvailable(m.notesScope, scope) || !m.notesFilters.enabled(scope) {
			continue
		}
		for _, note := range m.notesByScope[scope] {
			if note == nil {
				continue
			}
			id := strings.TrimSpace(note.ID)
			if id == "" {
				continue
			}
			current, ok := merged[id]
			if !ok || note.UpdatedAt.After(current.UpdatedAt) {
				merged[id] = note
			}
		}
	}
	out := make([]*types.Note, 0, len(merged))
	for _, note := range merged {
		out = append(out, note)
	}
	sort.SliceStable(out, func(i, j int) bool {
		ai := out[i]
		aj := out[j]
		if ai == nil || aj == nil {
			return ai != nil
		}
		if !ai.UpdatedAt.Equal(aj.UpdatedAt) {
			return ai.UpdatedAt.After(aj.UpdatedAt)
		}
		if !ai.CreatedAt.Equal(aj.CreatedAt) {
			return ai.CreatedAt.After(aj.CreatedAt)
		}
		return strings.TrimSpace(ai.ID) < strings.TrimSpace(aj.ID)
	})
	return out
}

func hasAnyNotesFilterEnabled(scope noteScopeTarget, filters notesFilterState) bool {
	if isNotesScopeAvailable(scope, types.NoteScopeWorkspace) && filters.ShowWorkspace {
		return true
	}
	if isNotesScopeAvailable(scope, types.NoteScopeWorktree) && filters.ShowWorktree {
		return true
	}
	if isNotesScopeAvailable(scope, types.NoteScopeSession) && filters.ShowSession {
		return true
	}
	return false
}

func (f notesFilterState) enabled(scope types.NoteScope) bool {
	switch scope {
	case types.NoteScopeWorkspace:
		return f.ShowWorkspace
	case types.NoteScopeWorktree:
		return f.ShowWorktree
	case types.NoteScopeSession:
		return f.ShowSession
	default:
		return false
	}
}

func (m *Model) toggleNotesFilterScope(scope types.NoteScope) tea.Cmd {
	if !isNotesScopeAvailable(m.notesScope, scope) {
		return nil
	}
	switch scope {
	case types.NoteScopeWorkspace:
		m.notesFilters.ShowWorkspace = !m.notesFilters.ShowWorkspace
	case types.NoteScopeWorktree:
		m.notesFilters.ShowWorktree = !m.notesFilters.ShowWorktree
	case types.NoteScopeSession:
		m.notesFilters.ShowSession = !m.notesFilters.ShowSession
	}
	m.renderNotesViewsFromState()
	if m.notesByScope == nil {
		m.notesByScope = map[types.NoteScope][]*types.Note{}
	}
	if _, ok := m.notesByScope[scope]; !ok {
		request := noteScopeRequestForRoot(m.notesScope, scope)
		m.startNotesPanelLoadScopes([]noteScopeTarget{request}, false)
		return fetchNotesCmd(m.notesAPI, request)
	}
	return nil
}

func (m *Model) enableAllNotesFilters() tea.Cmd {
	if m.notesScope.IsZero() {
		return nil
	}
	m.notesFilters = defaultNotesFilterState(m.notesScope)
	m.renderNotesViewsFromState()
	return nil
}

type notesPanelLoadState struct {
	Loading    bool
	ErrorCount int
}

func (m *Model) notesPanelLoadState() notesPanelLoadState {
	return notesPanelLoadState{
		Loading:    len(m.notesPanelPendingScopes) > 0,
		ErrorCount: m.notesPanelLoadErrors,
	}
}

func (m *Model) resetNotesPanelLoadState() {
	m.notesPanelLoadErrors = 0
	if m.notesPanelPendingScopes == nil {
		m.notesPanelPendingScopes = map[types.NoteScope]struct{}{}
		return
	}
	for scope := range m.notesPanelPendingScopes {
		delete(m.notesPanelPendingScopes, scope)
	}
}

func (m *Model) startNotesPanelLoadScopes(requests []noteScopeTarget, reset bool) {
	if reset {
		m.resetNotesPanelLoadState()
	}
	if m.notesPanelPendingScopes == nil {
		m.notesPanelPendingScopes = map[types.NoteScope]struct{}{}
	}
	for _, request := range requests {
		if request.Scope == "" {
			continue
		}
		m.notesPanelPendingScopes[request.Scope] = struct{}{}
	}
	if m.notesPanelOpen {
		m.notesPanelBlocks = notesPanelBlocksFromState(m.notes, m.notesScope, m.notesFilters, m.notesPanelLoadState())
		m.renderNotesPanel()
	}
}

func (m *Model) settleNotesPanelLoadScope(scope noteScopeTarget, failed bool) {
	if m.notesScope.IsZero() || scope.Scope == "" {
		return
	}
	expected := noteScopeRequestForRoot(m.notesScope, scope.Scope)
	if !noteScopeEqual(expected, scope) {
		return
	}
	if _, ok := m.notesPanelPendingScopes[scope.Scope]; ok {
		delete(m.notesPanelPendingScopes, scope.Scope)
		if failed {
			m.notesPanelLoadErrors++
		}
	}
}

func notesPanelBlocksFromState(notes []*types.Note, scope noteScopeTarget, filters notesFilterState, loadState notesPanelLoadState) []ChatBlock {
	filterSection := notesFilterSection(scope, filters)
	header := ChatBlock{
		ID:   "notes-panel-scope",
		Role: ChatRoleSystem,
		Text: "Notes Panel\n\n" +
			"Scope: " + scope.Label() + "\n\n" +
			filterSection + "\n\n" +
			"Keys: 1/2/3 toggle filters, 0 all",
	}
	if !hasAnyNotesFilterEnabled(scope, filters) {
		return []ChatBlock{
			header,
			{
				ID:   "notes-panel-empty-filters",
				Role: ChatRoleSystem,
				Text: "No scopes selected.\n\nToggle a filter to view notes.",
			},
		}
	}
	if loadState.Loading {
		loadingText := "Loading notes..."
		if loadState.ErrorCount > 0 {
			loadingText += fmt.Sprintf("\n\nRetrying after %d failed request(s).", loadState.ErrorCount)
		}
		if len(notes) == 0 {
			return []ChatBlock{
				header,
				{
					ID:   "notes-panel-loading",
					Role: ChatRoleSystem,
					Text: loadingText,
				},
			}
		}
		blocks := make([]ChatBlock, 0, len(notes)+2)
		blocks = append(blocks, header)
		blocks = append(blocks, ChatBlock{
			ID:   "notes-panel-loading",
			Role: ChatRoleSystem,
			Text: loadingText,
		})
		for _, note := range notes {
			if note == nil {
				continue
			}
			blocks = append(blocks, ChatBlock{
				ID:   note.ID,
				Role: chatRoleForNoteScope(note.Scope),
				Text: renderNoteBlockText(note),
			})
		}
		return blocks
	}
	if loadState.ErrorCount > 0 && len(notes) == 0 {
		return []ChatBlock{
			header,
			{
				ID:   "notes-panel-load-error",
				Role: ChatRoleSystem,
				Text: "Unable to load notes for one or more scopes.\n\nPress r in notes view to retry.",
			},
		}
	}
	if len(notes) == 0 {
		return []ChatBlock{
			header,
			{
				ID:   "notes-panel-empty",
				Role: ChatRoleSystem,
				Text: "No notes yet.",
			},
		}
	}
	blocks := make([]ChatBlock, 0, len(notes)+1)
	blocks = append(blocks, header)
	for _, note := range notes {
		if note == nil {
			continue
		}
		blocks = append(blocks, ChatBlock{
			ID:   note.ID,
			Role: chatRoleForNoteScope(note.Scope),
			Text: renderNoteBlockText(note),
		})
	}
	return blocks
}

func (m *Model) renderNotesPanel() {
	if !m.notesPanelOpen {
		return
	}
	width := m.notesPanelViewport.Width()
	if width <= 0 {
		m.notesPanelSpans = nil
		m.notesPanelViewport.SetContent("")
		return
	}
	if len(m.notesPanelBlocks) == 0 {
		m.notesPanelBlocks = notesPanelBlocksFromState(m.notes, m.notesScope, m.notesFilters, m.notesPanelLoadState())
	}
	rendered, spans := renderChatBlocks(m.notesPanelBlocks, width, maxViewportLines)
	m.notesPanelSpans = spans
	m.notesPanelViewport.SetContent(rendered)
}

func (m *Model) renderNotesPanelView() string {
	header := "Notes"
	if !m.notesScope.IsZero() {
		header += " • " + m.notesScope.Label()
	}
	body := m.notesPanelViewport.View()
	if strings.TrimSpace(body) == "" {
		body = "No notes."
	}
	return headerStyle.Render(header) + "\n" + body
}

func (m *Model) reduceNotesPanelWheelMouse(msg tea.MouseMsg, layout mouseLayout, delta int) bool {
	if !layout.panelVisible || layout.panelWidth <= 0 || m.notesPanelViewport.Height() <= 0 {
		return false
	}
	mouse := msg.Mouse()
	if mouse.X < layout.panelStart || mouse.X >= layout.panelStart+layout.panelWidth {
		return false
	}
	if mouse.Y < 0 || mouse.Y > m.notesPanelViewport.Height() {
		return false
	}
	if delta < 0 {
		m.notesPanelViewport.ScrollUp(3)
	} else {
		m.notesPanelViewport.ScrollDown(3)
	}
	return true
}

func (m *Model) reduceNotesPanelLeftPressMouse(msg tea.MouseMsg, layout mouseLayout) bool {
	if !isMouseClickMsg(msg) {
		return false
	}
	if !layout.panelVisible || layout.panelWidth <= 0 || m.notesPanelViewport.Height() <= 0 {
		return false
	}
	mouse := msg.Mouse()
	if mouse.X < layout.panelStart || mouse.X >= layout.panelStart+layout.panelWidth {
		return false
	}
	if mouse.Y < 1 || mouse.Y > m.notesPanelViewport.Height() {
		return false
	}
	col := mouse.X - layout.panelStart
	line := mouse.Y - 1
	if handled, cmd := m.toggleNotesFilterByPanelViewportPosition(col, line); handled {
		if cmd != nil {
			m.pendingMouseCmd = cmd
		}
		return true
	}
	if handled, cmd := m.moveNotesPanelByViewportPosition(col, line); handled {
		if cmd != nil {
			m.pendingMouseCmd = cmd
		}
		return true
	}
	if m.deleteNotesPanelByViewportPosition(col, line) {
		return true
	}
	handled, cmd := m.copyNotesPanelByViewportPosition(col, line)
	if cmd != nil {
		m.pendingMouseCmd = cmd
	}
	return handled
}

func (m *Model) copyNotesPanelByViewportPosition(col, line int) (bool, tea.Cmd) {
	if col < 0 || line < 0 || len(m.notesPanelBlocks) == 0 || len(m.notesPanelSpans) == 0 {
		return false, nil
	}
	absolute := m.notesPanelViewport.YOffset() + line
	for _, span := range m.notesPanelSpans {
		if span.CopyLine != absolute {
			continue
		}
		if span.CopyStart < 0 || span.CopyEnd < span.CopyStart {
			continue
		}
		if col < span.CopyStart || col > span.CopyEnd {
			continue
		}
		return m.copyNotesPanelBlockByIndex(span.BlockIndex)
	}
	return false, nil
}

func (m *Model) copyNotesPanelBlockByIndex(index int) (bool, tea.Cmd) {
	if index < 0 || index >= len(m.notesPanelBlocks) {
		return false, nil
	}
	text := strings.TrimSpace(m.notesPanelBlocks[index].Text)
	if text == "" {
		m.setCopyStatusWarning("nothing to copy")
		return true, nil
	}
	return true, m.copyWithStatusCmd(text, "note copied")
}

func (m *Model) deleteNotesPanelByViewportPosition(col, line int) bool {
	if col < 0 || line < 0 || len(m.notesPanelBlocks) == 0 || len(m.notesPanelSpans) == 0 {
		return false
	}
	absolute := m.notesPanelViewport.YOffset() + line
	for _, span := range m.notesPanelSpans {
		if !isNoteRole(span.Role) {
			continue
		}
		if span.DeleteLine != absolute {
			continue
		}
		if span.DeleteStart < 0 || span.DeleteEnd < span.DeleteStart {
			continue
		}
		if col < span.DeleteStart || col > span.DeleteEnd {
			continue
		}
		return m.confirmDeleteNoteByPanelBlockIndex(span.BlockIndex)
	}
	return false
}

func (m *Model) confirmDeleteNoteByPanelBlockIndex(index int) bool {
	noteID := m.noteIDByPanelBlockIndex(index)
	if noteID == "" {
		m.setValidationStatus("select a note to delete")
		return true
	}
	m.confirmDeleteNote(noteID)
	return true
}

func (m *Model) toggleNotesFilterByViewportPosition(col, line int) (bool, tea.Cmd) {
	if col < 0 || line < 0 || len(m.contentBlockSpans) == 0 {
		return false, nil
	}
	absolute := m.viewport.YOffset() + line
	lines := m.currentLines()
	if absolute < 0 || absolute >= len(lines) {
		return false, nil
	}
	for _, span := range m.contentBlockSpans {
		if span.ID != "notes-scope" {
			continue
		}
		if absolute < span.StartLine || absolute > span.EndLine {
			continue
		}
		if scope, ok := notesFilterScopeFromSpanClick(span, absolute, col, m.notesScope); ok {
			return true, m.toggleNotesFilterScope(scope)
		}
		scope, ok := notesFilterScopeFromLineClick(lines[absolute], col, m.notesScope, m.notesFilters)
		if !ok {
			return false, nil
		}
		return true, m.toggleNotesFilterScope(scope)
	}
	return false, nil
}

func (m *Model) toggleNotesFilterByPanelViewportPosition(col, line int) (bool, tea.Cmd) {
	if col < 0 || line < 0 || len(m.notesPanelSpans) == 0 {
		return false, nil
	}
	absolute := m.notesPanelViewport.YOffset() + line
	lines := strings.Split(xansi.Strip(m.notesPanelViewport.View()), "\n")
	if line < 0 || line >= len(lines) {
		return false, nil
	}
	lineText := lines[line]
	for _, span := range m.notesPanelSpans {
		if span.ID != "notes-panel-scope" {
			continue
		}
		if absolute < span.StartLine || absolute > span.EndLine {
			continue
		}
		if scope, ok := notesFilterScopeFromSpanClick(span, absolute, col, m.notesScope); ok {
			return true, m.toggleNotesFilterScope(scope)
		}
		scope, ok := notesFilterScopeFromLineClick(lineText, col, m.notesScope, m.notesFilters)
		if !ok {
			return false, nil
		}
		return true, m.toggleNotesFilterScope(scope)
	}
	return false, nil
}

func notesFilterScopeFromLineClick(line string, col int, scope noteScopeTarget, filters notesFilterState) (types.NoteScope, bool) {
	for _, target := range []types.NoteScope{types.NoteScopeWorkspace, types.NoteScopeWorktree, types.NoteScopeSession} {
		if !isNotesScopeAvailable(scope, target) {
			continue
		}
		label := notesScopeLabel(target)
		labelIdx := strings.Index(line, label)
		if labelIdx < 0 {
			continue
		}
		if col >= labelIdx && col <= labelIdx+len(label)-1 {
			return target, true
		}
		marker := "[ ]"
		if filters.enabled(target) {
			marker = "[x]"
		}
		markerIdx := strings.Index(line, marker)
		if markerIdx < 0 {
			continue
		}
		start := labelIdx
		end := labelIdx + len(label) - 1
		if markerIdx < start {
			start = markerIdx
		}
		markerEnd := markerIdx + len(marker) - 1
		if markerEnd > end {
			end = markerEnd
		}
		if col >= start && col <= end {
			return target, true
		}
	}
	return "", false
}

func notesFilterScopeFromSpanClick(span renderedBlockSpan, absolute, col int, root noteScopeTarget) (types.NoteScope, bool) {
	if isNotesScopeAvailable(root, types.NoteScopeWorkspace) &&
		span.WorkspaceFilterLine == absolute &&
		span.WorkspaceFilterStart >= 0 &&
		col >= span.WorkspaceFilterStart && col <= span.WorkspaceFilterEnd {
		return types.NoteScopeWorkspace, true
	}
	if isNotesScopeAvailable(root, types.NoteScopeWorktree) &&
		span.WorktreeFilterLine == absolute &&
		span.WorktreeFilterStart >= 0 &&
		col >= span.WorktreeFilterStart && col <= span.WorktreeFilterEnd {
		return types.NoteScopeWorktree, true
	}
	if isNotesScopeAvailable(root, types.NoteScopeSession) &&
		span.SessionFilterLine == absolute &&
		span.SessionFilterStart >= 0 &&
		col >= span.SessionFilterStart && col <= span.SessionFilterEnd {
		return types.NoteScopeSession, true
	}
	return "", false
}

func notesScopeLabel(scope types.NoteScope) string {
	switch scope {
	case types.NoteScopeWorkspace:
		return "Workspace"
	case types.NoteScopeWorktree:
		return "Worktree"
	case types.NoteScopeSession:
		return "Session"
	default:
		return "Scope"
	}
}

func noteScopeEqual(a, b noteScopeTarget) bool {
	return a.Scope == b.Scope &&
		strings.TrimSpace(a.WorkspaceID) == strings.TrimSpace(b.WorkspaceID) &&
		strings.TrimSpace(a.WorktreeID) == strings.TrimSpace(b.WorktreeID) &&
		strings.TrimSpace(a.SessionID) == strings.TrimSpace(b.SessionID)
}

func noteMatchesCurrentRoot(note *types.Note, root noteScopeTarget) bool {
	if note == nil || root.IsZero() {
		return false
	}
	switch note.Scope {
	case types.NoteScopeWorkspace:
		return strings.TrimSpace(note.WorkspaceID) == strings.TrimSpace(root.WorkspaceID)
	case types.NoteScopeWorktree:
		return strings.TrimSpace(note.WorkspaceID) == strings.TrimSpace(root.WorkspaceID) &&
			strings.TrimSpace(note.WorktreeID) == strings.TrimSpace(root.WorktreeID)
	case types.NoteScopeSession:
		return strings.TrimSpace(note.SessionID) == strings.TrimSpace(root.SessionID)
	default:
		return false
	}
}

func (m *Model) upsertNotesLive(note *types.Note) {
	if note == nil || !noteMatchesCurrentRoot(note, m.notesScope) {
		return
	}
	scope := note.Scope
	list := m.notesByScope[scope]
	next := make([]*types.Note, 0, len(list)+1)
	next = append(next, note)
	for _, existing := range list {
		if existing == nil || strings.TrimSpace(existing.ID) == strings.TrimSpace(note.ID) {
			continue
		}
		next = append(next, existing)
	}
	m.notesByScope[scope] = next
	m.renderNotesViewsFromState()
}

func (m *Model) removeNotesLive(id string) {
	id = strings.TrimSpace(id)
	if id == "" {
		return
	}
	for _, scope := range []types.NoteScope{types.NoteScopeWorkspace, types.NoteScopeWorktree, types.NoteScopeSession} {
		list := m.notesByScope[scope]
		if len(list) == 0 {
			continue
		}
		next := make([]*types.Note, 0, len(list))
		changed := false
		for _, note := range list {
			if note == nil {
				continue
			}
			if strings.TrimSpace(note.ID) == id {
				changed = true
				continue
			}
			next = append(next, note)
		}
		if changed {
			m.notesByScope[scope] = next
		}
	}
	m.renderNotesViewsFromState()
}

func (m *Model) notesRefreshCmdForOpenViews() tea.Cmd {
	if (m.mode != uiModeNotes && m.mode != uiModeAddNote) && !m.notesPanelOpen {
		return nil
	}
	return m.refreshNotesForCurrentScope()
}
