package app

import (
	"fmt"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	xansi "github.com/charmbracelet/x/ansi"

	"control/internal/types"
)

const recentsPreviewMaxChars = 220

type recentsPreview struct {
	Revision string
	Loading  bool
	Preview  string
	Full     string
	Err      string
}

type recentsEntryStatus string

const (
	recentsEntryReady   recentsEntryStatus = "ready"
	recentsEntryRunning recentsEntryStatus = "running"
)

type recentsEntry struct {
	SessionID       string
	Session         *types.Session
	Meta            *types.SessionMeta
	Status          recentsEntryStatus
	WorkspaceName   string
	CompletedAt     time.Time
	PreviewRevision string
	Preview         recentsPreview
}

type recentsViewState struct {
	Filter  sidebarRecentsFilter
	Ready   []recentsEntry
	Running []recentsEntry
	Entries []recentsEntry
}

func (m *Model) enterRecentsView(item *sidebarItem) {
	if m == nil {
		return
	}
	if m.mode == uiModeCompose {
		_ = m.saveCurrentComposeDraft()
		m.closeComposeOptionPicker()
	}
	m.mode = uiModeRecents
	m.recentsReplySessionID = ""
	if m.recentsReplyInput != nil {
		m.recentsReplyInput.Blur()
		m.recentsReplyInput.SetValue("")
	}
	if m.input != nil {
		m.input.FocusSidebar()
	}
	m.pauseFollow(false)
	m.refreshRecentsContent()
	if item != nil {
		switch item.kind {
		case sidebarRecentsReady:
			m.setStatusMessage("recents: ready")
		case sidebarRecentsRunning:
			m.setStatusMessage("recents: running")
		default:
			m.setStatusMessage("recents")
		}
	}
}

func (m *Model) exitRecentsView() {
	if m == nil || m.mode != uiModeRecents {
		return
	}
	m.mode = uiModeNormal
	m.recentsReplySessionID = ""
	if m.recentsReplyInput != nil {
		m.recentsReplyInput.Blur()
		m.recentsReplyInput.SetValue("")
	}
	if m.input != nil {
		m.input.FocusSidebar()
	}
}

func (m *Model) recentsHeader() string {
	state := m.recentsState()
	switch state.Filter {
	case sidebarRecentsFilterReady:
		return fmt.Sprintf("Recents • Ready (%d)", len(state.Ready))
	case sidebarRecentsFilterRunning:
		return fmt.Sprintf("Recents • Running (%d)", len(state.Running))
	default:
		return fmt.Sprintf("Recents • Ready %d • Running %d", len(state.Ready), len(state.Running))
	}
}

func (m *Model) recentsReplyFooter() string {
	return "enter send • esc cancel"
}

func (m *Model) recentsFilter() sidebarRecentsFilter {
	item := m.selectedItem()
	if item == nil {
		return sidebarRecentsFilterAll
	}
	switch item.kind {
	case sidebarRecentsReady:
		return sidebarRecentsFilterReady
	case sidebarRecentsRunning:
		return sidebarRecentsFilterRunning
	default:
		return sidebarRecentsFilterAll
	}
}

func (m *Model) recentsState() recentsViewState {
	state := recentsViewState{
		Filter: m.recentsFilter(),
	}
	if m == nil || m.recents == nil {
		return state
	}
	projection := m.buildSidebarProjection()
	sessionByID := make(map[string]*types.Session, len(projection.Sessions))
	for _, session := range projection.Sessions {
		if session == nil || strings.TrimSpace(session.ID) == "" {
			continue
		}
		sessionByID[strings.TrimSpace(session.ID)] = session
	}
	state.Ready = m.recentsReadyEntries(sessionByID)
	state.Running = m.recentsRunningEntries(sessionByID)
	switch state.Filter {
	case sidebarRecentsFilterReady:
		state.Entries = append(state.Entries, state.Ready...)
	case sidebarRecentsFilterRunning:
		state.Entries = append(state.Entries, state.Running...)
	default:
		state.Entries = append(state.Entries, state.Ready...)
		state.Entries = append(state.Entries, state.Running...)
	}
	return state
}

func (m *Model) recentsReadyEntries(sessionByID map[string]*types.Session) []recentsEntry {
	ids := m.recents.ReadyIDs()
	entries := make([]recentsEntry, 0, len(ids))
	for _, id := range ids {
		session := sessionByID[id]
		if session == nil {
			continue
		}
		entry := m.buildRecentsEntry(session, recentsEntryReady)
		if ready, ok := m.recents.ReadyItem(id); ok {
			entry.CompletedAt = ready.CompletedAt
		}
		entries = append(entries, entry)
	}
	return entries
}

func (m *Model) recentsRunningEntries(sessionByID map[string]*types.Session) []recentsEntry {
	ids := m.recents.RunningIDs()
	entries := make([]recentsEntry, 0, len(ids))
	for _, id := range ids {
		session := sessionByID[id]
		if session == nil {
			continue
		}
		entries = append(entries, m.buildRecentsEntry(session, recentsEntryRunning))
	}
	return entries
}

func (m *Model) buildRecentsEntry(session *types.Session, status recentsEntryStatus) recentsEntry {
	if session == nil {
		return recentsEntry{}
	}
	sessionID := strings.TrimSpace(session.ID)
	meta := m.sessionMeta[sessionID]
	workspaceName := "Unassigned"
	if meta != nil {
		if ws := m.workspaceByID(strings.TrimSpace(meta.WorkspaceID)); ws != nil && strings.TrimSpace(ws.Name) != "" {
			workspaceName = ws.Name
		}
	}
	revision := ""
	if meta != nil {
		revision = strings.TrimSpace(meta.LastTurnID)
	}
	preview := m.previewForSession(sessionID, revision)
	return recentsEntry{
		SessionID:       sessionID,
		Session:         session,
		Meta:            meta,
		Status:          status,
		WorkspaceName:   workspaceName,
		PreviewRevision: revision,
		Preview:         preview,
	}
}

func (m *Model) previewForSession(sessionID, revision string) recentsPreview {
	sessionID = strings.TrimSpace(sessionID)
	revision = strings.TrimSpace(revision)
	if sessionID == "" {
		return recentsPreview{}
	}
	if m.recentsPreviews == nil {
		m.recentsPreviews = map[string]recentsPreview{}
	}
	entry := m.recentsPreviews[sessionID]
	if strings.TrimSpace(entry.Revision) != revision {
		entry = recentsPreview{Revision: revision}
	}
	if strings.TrimSpace(entry.Preview) == "" {
		if blocks := m.transcriptCache["sess:"+sessionID]; len(blocks) > 0 {
			if latest := latestAssistantBlockText(blocks); latest != "" {
				entry.Preview, entry.Full = formatRecentsPreviewText(latest)
			}
		}
	}
	m.recentsPreviews[sessionID] = entry
	return entry
}

func latestAssistantBlockText(blocks []ChatBlock) string {
	for i := len(blocks) - 1; i >= 0; i-- {
		block := blocks[i]
		if block.Role != ChatRoleAgent {
			continue
		}
		text := strings.TrimSpace(block.Text)
		if text == "" {
			continue
		}
		return text
	}
	return ""
}

func formatRecentsPreviewText(text string) (preview string, full string) {
	full = strings.TrimSpace(text)
	if full == "" {
		return "", ""
	}
	flat := cleanTitle(strings.ReplaceAll(full, "\n", " "))
	if flat == "" {
		flat = cleanTitle(full)
	}
	preview = truncateText(flat, recentsPreviewMaxChars)
	return preview, full
}

func (m *Model) refreshRecentsContent() {
	if m == nil || m.mode != uiModeRecents {
		return
	}
	state := m.recentsState()
	m.syncRecentsSelection(state.Entries)
	content := m.renderRecentsContent(state)
	m.setContentText(content)
}

func (m *Model) syncRecentsSelection(entries []recentsEntry) {
	if len(entries) == 0 {
		m.recentsSelectedSessionID = ""
		m.recentsReplySessionID = ""
		return
	}
	current := strings.TrimSpace(m.recentsSelectedSessionID)
	if current != "" {
		for _, entry := range entries {
			if entry.SessionID == current {
				return
			}
		}
	}
	m.recentsSelectedSessionID = entries[0].SessionID
}

func (m *Model) renderRecentsContent(state recentsViewState) string {
	var builder strings.Builder
	builder.WriteString("Recents overview\n")
	builder.WriteString("Use j/k to choose • r reply • x expand • enter open • d dismiss ready\n")
	builder.WriteString("Buttons are shown above each preview bubble.\n\n")
	switch state.Filter {
	case sidebarRecentsFilterReady:
		m.renderRecentsSection(&builder, "Ready", state.Ready)
	case sidebarRecentsFilterRunning:
		m.renderRecentsSection(&builder, "Running", state.Running)
	default:
		m.renderRecentsSection(&builder, "Ready", state.Ready)
		builder.WriteString("\n")
		m.renderRecentsSection(&builder, "Running", state.Running)
	}
	return strings.TrimRight(builder.String(), "\n")
}

func (m *Model) renderRecentsSection(builder *strings.Builder, title string, entries []recentsEntry) {
	heading := fmt.Sprintf("%s (%d)", title, len(entries))
	builder.WriteString(heading)
	builder.WriteString("\n")
	builder.WriteString(strings.Repeat("-", min(72, max(16, len(heading)))))
	builder.WriteString("\n")
	if len(entries) == 0 {
		builder.WriteString("No sessions.\n")
		return
	}
	for _, entry := range entries {
		m.renderRecentsCard(builder, entry)
	}
}

func (m *Model) renderRecentsCard(builder *strings.Builder, entry recentsEntry) {
	width := m.recentsContentWidth()
	sessionTitleText := sessionTitle(entry.Session, entry.Meta)
	if strings.TrimSpace(sessionTitleText) == "" {
		sessionTitleText = entry.SessionID
	}
	selected := strings.TrimSpace(m.recentsSelectedSessionID) == entry.SessionID
	if selected {
		builder.WriteString("▶ Selected")
		builder.WriteString("\n")
	}
	statusLabel := "Running"
	if entry.Status == recentsEntryReady {
		statusLabel = "Ready"
	}
	expanded := m.recentsExpandedSessions[entry.SessionID]
	controlsLine := m.renderRecentsControlsLine(entry, expanded, selected, width)
	if controlsLine != "" {
		builder.WriteString(controlsLine)
		builder.WriteString("\n")
	}
	metaSummary := fmt.Sprintf("%s • %s • %s • %s", statusLabel, sessionTitleText, entry.WorkspaceName, formatSince(sessionLastActive(entry.Session, entry.Meta)))
	if width > 0 {
		metaSummary = truncateToWidth(metaSummary, width)
	}
	builder.WriteString(metaSummary)
	builder.WriteString("\n")
	text := recentsPreviewText(entry, expanded)
	bubble := m.renderRecentsBubble(text, selected, width)
	builder.WriteString(bubble)
	builder.WriteString("\n\n")
}

func (m *Model) recentsContentWidth() int {
	if m == nil {
		return 80
	}
	width := m.viewport.Width()
	if width <= 0 {
		width = m.width - m.sidebarWidth() - 2
	}
	if width <= 0 {
		width = 80
	}
	return max(40, width)
}

func (m *Model) renderRecentsControlsLine(entry recentsEntry, expanded bool, selected bool, width int) string {
	expandLabel := "[Expand]"
	if expanded {
		expandLabel = "[Collapse]"
	}
	controls := []string{"[Reply]", expandLabel, "[Open]"}
	if entry.Status == recentsEntryReady {
		controls = append(controls, "[Dismiss]")
	}
	line := strings.Join(controls, " ")
	if selected {
		line = "▶ " + line
	}
	if width > 0 {
		line = truncateToWidth(line, width)
	}
	return line
}

func recentsPreviewText(entry recentsEntry, expanded bool) string {
	previewText := strings.TrimSpace(entry.Preview.Preview)
	fullText := strings.TrimSpace(entry.Preview.Full)
	if entry.Preview.Loading {
		previewText = "Loading latest assistant response..."
	}
	if strings.TrimSpace(entry.Preview.Err) != "" {
		previewText = "Preview error: " + cleanTitle(entry.Preview.Err)
	}
	if previewText == "" {
		previewText = "No assistant response cached yet."
	}
	if expanded {
		if fullText == "" {
			fullText = previewText
		}
		return fullText
	}
	return previewText
}

func (m *Model) renderRecentsBubble(text string, selected bool, width int) string {
	if width <= 0 {
		width = 80
	}
	maxBubbleWidth := width - 2
	if maxBubbleWidth < 10 {
		maxBubbleWidth = width
	}
	innerWidth := maxBubbleWidth - 4
	if innerWidth < 1 {
		innerWidth = 1
	}
	rendered := strings.TrimSpace(xansi.Strip(renderChatText(ChatRoleAgent, text, innerWidth)))
	if rendered == "" {
		rendered = " "
	}
	lines := strings.Split(rendered, "\n")
	if len(lines) == 0 {
		lines = []string{" "}
	}
	var builder strings.Builder
	borderRune := "-"
	if selected {
		borderRune = "="
	}
	hRule := strings.Repeat(borderRune, innerWidth+2)
	builder.WriteString("+")
	builder.WriteString(hRule)
	builder.WriteString("+")
	builder.WriteString("\n")
	for i, line := range lines {
		if innerWidth > 0 {
			line = truncateToWidth(line, innerWidth)
		}
		padding := innerWidth - xansi.StringWidth(line)
		if padding < 0 {
			padding = 0
		}
		builder.WriteString("| ")
		builder.WriteString(line)
		if padding > 0 {
			builder.WriteString(strings.Repeat(" ", padding))
		}
		builder.WriteString(" |")
		if i < len(lines)-1 {
			builder.WriteString("\n")
		}
	}
	builder.WriteString("\n")
	builder.WriteString("+")
	builder.WriteString(hRule)
	builder.WriteString("+")
	return builder.String()
}

func (m *Model) selectedRecentsEntry() (recentsEntry, bool) {
	state := m.recentsState()
	if len(state.Entries) == 0 {
		return recentsEntry{}, false
	}
	selected := strings.TrimSpace(m.recentsSelectedSessionID)
	if selected != "" {
		for _, entry := range state.Entries {
			if entry.SessionID == selected {
				return entry, true
			}
		}
	}
	return state.Entries[0], true
}

func (m *Model) moveRecentsSelection(delta int) bool {
	if m == nil || delta == 0 {
		return false
	}
	state := m.recentsState()
	entries := state.Entries
	if len(entries) == 0 {
		return false
	}
	current := 0
	selected := strings.TrimSpace(m.recentsSelectedSessionID)
	if selected != "" {
		for i, entry := range entries {
			if entry.SessionID == selected {
				current = i
				break
			}
		}
	}
	next := current + delta
	if next < 0 {
		next = 0
	}
	if next >= len(entries) {
		next = len(entries) - 1
	}
	if next == current {
		return false
	}
	m.recentsSelectedSessionID = entries[next].SessionID
	m.refreshRecentsContent()
	return true
}

func (m *Model) toggleSelectedRecentsExpand() bool {
	entry, ok := m.selectedRecentsEntry()
	if !ok {
		return false
	}
	if m.recentsExpandedSessions == nil {
		m.recentsExpandedSessions = map[string]bool{}
	}
	current := m.recentsExpandedSessions[entry.SessionID]
	m.recentsExpandedSessions[entry.SessionID] = !current
	m.refreshRecentsContent()
	return true
}

func (m *Model) startRecentsReply() bool {
	entry, ok := m.selectedRecentsEntry()
	if !ok {
		return false
	}
	if m.recentsReplyInput == nil {
		return false
	}
	m.recentsReplySessionID = entry.SessionID
	m.recentsReplyInput.SetPlaceholder("reply")
	m.recentsReplyInput.SetValue("")
	m.recentsReplyInput.Focus()
	if m.input != nil {
		m.input.FocusChatInput()
	}
	m.setStatusMessage("replying inline")
	return true
}

func (m *Model) cancelRecentsReply() {
	m.recentsReplySessionID = ""
	if m.recentsReplyInput != nil {
		m.recentsReplyInput.Blur()
		m.recentsReplyInput.SetValue("")
	}
	if m.input != nil {
		m.input.FocusSidebar()
	}
}

func (m *Model) submitRecentsReplyInput(text string) tea.Cmd {
	text = strings.TrimSpace(text)
	if text == "" {
		m.setValidationStatus("message is required")
		return nil
	}
	sessionID := strings.TrimSpace(m.recentsReplySessionID)
	if sessionID == "" {
		m.setValidationStatus("select a session to reply")
		return nil
	}
	provider := m.providerForSessionID(sessionID)
	token := m.nextSendToken()
	m.registerPendingSend(token, sessionID, provider)
	m.cancelRecentsReply()
	m.setStatusMessage("sending reply")
	return sendSessionCmd(m.sessionAPI, sessionID, text, token)
}

func (m *Model) dismissSelectedRecentsReady() bool {
	entry, ok := m.selectedRecentsEntry()
	if !ok || entry.Status != recentsEntryReady || m.recents == nil {
		return false
	}
	if !m.recents.DismissReady(entry.SessionID) {
		return false
	}
	m.refreshRecentsSidebarState()
	m.setStatusInfo("dismissed from recents")
	m.refreshRecentsContent()
	return true
}

func (m *Model) openSelectedRecentsThread() tea.Cmd {
	entry, ok := m.selectedRecentsEntry()
	if !ok || strings.TrimSpace(entry.SessionID) == "" {
		m.setValidationStatus("select a session")
		return nil
	}
	if m.sidebar == nil || !m.sidebar.SelectBySessionID(entry.SessionID) {
		m.setValidationStatus("session unavailable")
		return nil
	}
	m.exitRecentsView()
	return m.onSelectionChangedImmediate()
}

func (m *Model) ensureRecentsPreviewForSelection() tea.Cmd {
	entry, ok := m.selectedRecentsEntry()
	if !ok {
		return nil
	}
	if strings.TrimSpace(entry.SessionID) == "" || strings.TrimSpace(entry.PreviewRevision) == "" {
		return nil
	}
	preview := entry.Preview
	if strings.TrimSpace(preview.Preview) != "" || preview.Loading {
		return nil
	}
	preview.Loading = true
	preview.Err = ""
	preview.Revision = entry.PreviewRevision
	if m.recentsPreviews == nil {
		m.recentsPreviews = map[string]recentsPreview{}
	}
	m.recentsPreviews[entry.SessionID] = preview
	m.refreshRecentsContent()
	lines := m.historyFetchLinesInitial()
	if lines < defaultTailLines {
		lines = defaultTailLines
	}
	return fetchRecentsPreviewCmd(m.sessionHistoryAPI, entry.SessionID, entry.PreviewRevision, lines)
}

func (m *Model) handleRecentsPreview(msg recentsPreviewMsg) tea.Cmd {
	if m == nil {
		return nil
	}
	id := strings.TrimSpace(msg.id)
	if id == "" {
		return nil
	}
	if m.recentsPreviews == nil {
		m.recentsPreviews = map[string]recentsPreview{}
	}
	entry := m.recentsPreviews[id]
	entry.Revision = strings.TrimSpace(msg.revision)
	entry.Loading = false
	entry.Err = ""
	if msg.err != nil {
		entry.Err = msg.err.Error()
		m.recentsPreviews[id] = entry
		if m.mode == uiModeRecents {
			m.refreshRecentsContent()
		}
		return nil
	}
	entry.Preview, entry.Full = formatRecentsPreviewText(msg.text)
	m.recentsPreviews[id] = entry
	if m.mode == uiModeRecents {
		m.refreshRecentsContent()
	}
	return nil
}

func (m *Model) beginRecentsCompletionWatch(sessionID, expectedTurn string) tea.Cmd {
	if m == nil || m.sessionAPI == nil {
		return nil
	}
	sessionID = strings.TrimSpace(sessionID)
	expectedTurn = strings.TrimSpace(expectedTurn)
	if sessionID == "" {
		return nil
	}
	provider := m.providerForSessionID(sessionID)
	if !providerSupportsEvents(provider) {
		return nil
	}
	if m.recentsCompletionWatching == nil {
		m.recentsCompletionWatching = map[string]string{}
	}
	if current, ok := m.recentsCompletionWatching[sessionID]; ok && strings.TrimSpace(current) == expectedTurn {
		return nil
	}
	m.recentsCompletionWatching[sessionID] = expectedTurn
	return watchRecentsTurnCompletionCmd(m.sessionAPI, sessionID, expectedTurn)
}

func (m *Model) handleRecentsTurnCompleted(msg recentsTurnCompletedMsg) tea.Cmd {
	if m == nil || m.recents == nil {
		return nil
	}
	sessionID := strings.TrimSpace(msg.id)
	if sessionID == "" {
		return nil
	}
	expectedTurn := strings.TrimSpace(msg.expectedTurn)
	if len(m.recentsCompletionWatching) > 0 {
		if current, ok := m.recentsCompletionWatching[sessionID]; ok {
			if expectedTurn == "" || strings.TrimSpace(current) == expectedTurn {
				delete(m.recentsCompletionWatching, sessionID)
			}
		}
	}
	if msg.err != nil {
		return nil
	}
	completionTurn := strings.TrimSpace(msg.turnID)
	if completionTurn == "" {
		if meta := m.sessionMeta[sessionID]; meta != nil {
			completionTurn = strings.TrimSpace(meta.LastTurnID)
		}
	}
	if _, ok := m.recents.CompleteRun(sessionID, expectedTurn, completionTurn, time.Now().UTC()); !ok {
		return nil
	}
	m.refreshRecentsSidebarState()
	if m.mode == uiModeRecents {
		m.refreshRecentsContent()
	}
	return nil
}

func (m *Model) syncRecentsCompletionWatches() {
	if m == nil || m.recents == nil || len(m.recentsCompletionWatching) == 0 {
		return
	}
	for sessionID := range m.recentsCompletionWatching {
		if !m.recents.IsRunning(sessionID) {
			delete(m.recentsCompletionWatching, sessionID)
		}
	}
}

func (m *Model) reduceRecentsMode(msg tea.Msg) (bool, tea.Cmd) {
	if m.mode != uiModeRecents {
		return false, nil
	}
	switch msg := msg.(type) {
	case tea.PasteMsg:
		if m.recentsReplySessionID == "" || m.recentsReplyInput == nil {
			return false, nil
		}
		cmd := m.recentsReplyInput.Update(msg)
		return true, cmd
	case tea.KeyMsg:
		if m.recentsReplySessionID != "" && m.recentsReplyInput != nil {
			switch m.keyString(msg) {
			case "esc":
				m.cancelRecentsReply()
				m.setStatusMessage("reply canceled")
				return true, nil
			case "enter":
				return true, m.submitRecentsReplyInput(m.recentsReplyInput.Value())
			default:
				cmd := m.recentsReplyInput.Update(msg)
				return true, cmd
			}
		}
		switch m.keyString(msg) {
		case "j":
			if m.moveRecentsSelection(1) {
				return true, m.ensureRecentsPreviewForSelection()
			}
			return true, nil
		case "k":
			if m.moveRecentsSelection(-1) {
				return true, m.ensureRecentsPreviewForSelection()
			}
			return true, nil
		case "x", "e":
			if m.toggleSelectedRecentsExpand() {
				return true, m.ensureRecentsPreviewForSelection()
			}
			return true, nil
		case "r":
			if m.startRecentsReply() {
				return true, nil
			}
			m.setValidationStatus("select a session to reply")
			return true, nil
		case "d":
			if m.dismissSelectedRecentsReady() {
				return true, nil
			}
			m.setValidationStatus("select a ready session to dismiss")
			return true, nil
		case "enter":
			return true, m.openSelectedRecentsThread()
		default:
			return false, nil
		}
	default:
		return false, nil
	}
}

func (m *Model) refreshRecentsSidebarState() {
	if m == nil || m.sidebar == nil {
		return
	}
	projection := m.buildSidebarProjection()
	m.sidebar.SetRecentsState(m.sidebarRecentsState(projection.Sessions))
}
