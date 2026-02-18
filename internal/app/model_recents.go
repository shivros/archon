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

const (
	recentsControlReply   ChatMetaControlID = "recents_reply"
	recentsControlExpand  ChatMetaControlID = "recents_expand"
	recentsControlOpen    ChatMetaControlID = "recents_open"
	recentsControlDismiss ChatMetaControlID = "recents_dismiss"
)

type recentsEntry struct {
	SessionID       string
	Session         *types.Session
	Meta            *types.SessionMeta
	Status          recentsEntryStatus
	LocationName    string
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

type recentsRenderContent struct {
	blocks        []ChatBlock
	metaByBlockID map[string]ChatBlockMetaPresentation
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
	locationName := "Unassigned"
	if meta != nil {
		worktreeName := ""
		worktreeID := strings.TrimSpace(meta.WorktreeID)
		if worktreeID != "" {
			worktreeName = worktreeID
			if wt := m.worktreeByID(worktreeID); wt != nil {
				if name := strings.TrimSpace(wt.Name); name != "" {
					worktreeName = name
				}
			}
		}
		if ws := m.workspaceByID(strings.TrimSpace(meta.WorkspaceID)); ws != nil && strings.TrimSpace(ws.Name) != "" {
			locationName = ws.Name
		}
		if worktreeName != "" {
			locationName += " / " + worktreeName
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
		LocationName:    locationName,
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
	plain := strings.TrimSpace(xansi.Strip(full))
	flat := cleanTitle(strings.ReplaceAll(plain, "\n", " "))
	if flat == "" {
		flat = cleanTitle(plain)
	}
	preview = truncateToWidth(flat, recentsPreviewMaxChars)
	return preview, full
}

func (m *Model) refreshRecentsContent() {
	if m == nil || m.mode != uiModeRecents {
		return
	}
	state := m.recentsState()
	m.syncRecentsSelection(state.Entries)
	content := m.buildRecentsRenderContent(state)
	m.applyBlocksWithMeta(content.blocks, content.metaByBlockID)
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

func (m *Model) buildRecentsRenderContent(state recentsViewState) recentsRenderContent {
	content := recentsRenderContent{
		blocks: []ChatBlock{
			{
				ID:   "recents:help",
				Role: ChatRoleSystem,
				Text: "Use j/k to choose • 1/2/3 or tab to switch all/ready/running • r reply • x expand • enter open • d dismiss ready",
			},
		},
		metaByBlockID: map[string]ChatBlockMetaPresentation{
			"recents:help": {
				Label: "Recents overview",
			},
		},
	}
	switch state.Filter {
	case sidebarRecentsFilterReady:
		content = m.appendRecentsSectionBlocks(content, "Ready", state.Ready)
	case sidebarRecentsFilterRunning:
		content = m.appendRecentsSectionBlocks(content, "Running", state.Running)
	default:
		content = m.appendRecentsSectionBlocks(content, "Ready", state.Ready)
		content = m.appendRecentsSectionBlocks(content, "Running", state.Running)
	}
	return content
}

func (m *Model) appendRecentsSectionBlocks(content recentsRenderContent, title string, entries []recentsEntry) recentsRenderContent {
	sectionID := "recents:section:" + strings.ToLower(strings.TrimSpace(title))
	sectionText := fmt.Sprintf("%s (%d)", title, len(entries))
	if len(entries) == 0 {
		sectionText += "\n" + m.recentsSectionEmptyText(title)
	}
	content.blocks = append(content.blocks, ChatBlock{
		ID:   sectionID,
		Role: ChatRoleSystem,
		Text: sectionText,
	})
	content.metaByBlockID[sectionID] = ChatBlockMetaPresentation{Label: "Section"}
	for _, entry := range entries {
		block, meta := m.buildRecentsEntryBlock(entry)
		content.blocks = append(content.blocks, block)
		content.metaByBlockID[block.ID] = meta
	}
	return content
}

func (m *Model) buildRecentsEntryBlock(entry recentsEntry) (ChatBlock, ChatBlockMetaPresentation) {
	sessionTitleText := sessionTitle(entry.Session, entry.Meta)
	if strings.TrimSpace(sessionTitleText) == "" {
		sessionTitleText = entry.SessionID
	}
	statusLabel := "Running"
	if entry.Status == recentsEntryReady {
		statusLabel = "Ready"
	}
	metaLabel := fmt.Sprintf("%s • %s • %s", statusLabel, sessionTitleText, entry.LocationName)
	if strings.TrimSpace(m.recentsSelectedSessionID) == entry.SessionID {
		metaLabel = "▶ " + metaLabel
	}
	expandLabel := "[Expand]"
	if m.recentsExpandedSessions[entry.SessionID] {
		expandLabel = "[Collapse]"
	}
	controls := []ChatMetaControl{
		{ID: recentsControlReply, Label: "[Reply]", Tone: ChatMetaControlToneCopy},
		{ID: recentsControlExpand, Label: expandLabel, Tone: ChatMetaControlToneCopy},
		{ID: recentsControlOpen, Label: "[Open]", Tone: ChatMetaControlTonePin},
	}
	if entry.Status == recentsEntryReady {
		controls = append(controls, ChatMetaControl{ID: recentsControlDismiss, Label: "[Dismiss]", Tone: ChatMetaControlToneDelete})
	}
	createdAt := time.Time{}
	if lastActive := sessionLastActive(entry.Session, entry.Meta); lastActive != nil {
		createdAt = *lastActive
	}
	if entry.Status == recentsEntryReady && !entry.CompletedAt.IsZero() {
		createdAt = entry.CompletedAt
	}
	block := ChatBlock{
		ID:        fmt.Sprintf("recents:%s:%s", entry.Status, entry.SessionID),
		Role:      ChatRoleAgent,
		Text:      recentsPreviewText(entry, m.recentsExpandedSessions[entry.SessionID]),
		CreatedAt: createdAt,
	}
	return block, ChatBlockMetaPresentation{
		Label:    metaLabel,
		Controls: controls,
	}
}

func recentsSessionIDFromBlockID(blockID string) (string, bool) {
	blockID = strings.TrimSpace(blockID)
	if !strings.HasPrefix(blockID, "recents:") {
		return "", false
	}
	parts := strings.SplitN(blockID, ":", 3)
	if len(parts) != 3 {
		return "", false
	}
	switch strings.TrimSpace(parts[1]) {
	case string(recentsEntryReady), string(recentsEntryRunning):
	default:
		return "", false
	}
	sessionID := strings.TrimSpace(parts[2])
	if sessionID == "" {
		return "", false
	}
	return sessionID, true
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
	detail := recentsEntryDetail(entry)
	if expanded {
		if fullText == "" {
			fullText = previewText
		}
		if detail != "" {
			return detail + "\n\n" + fullText
		}
		return fullText
	}
	if detail != "" {
		return detail + "\n" + previewText
	}
	return previewText
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

func (m *Model) setRecentsSelection(sessionID string) bool {
	if m == nil {
		return false
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return false
	}
	if strings.TrimSpace(m.recentsSelectedSessionID) == sessionID {
		return false
	}
	m.recentsSelectedSessionID = sessionID
	return true
}

type recentsControlClick struct {
	sessionID string
	controlID ChatMetaControlID
}

func recentsControlIDFromLabel(label string) ChatMetaControlID {
	switch strings.ToLower(strings.TrimSpace(label)) {
	case "[reply]":
		return recentsControlReply
	case "[expand]", "[collapse]":
		return recentsControlExpand
	case "[open]":
		return recentsControlOpen
	case "[dismiss]":
		return recentsControlDismiss
	default:
		return ""
	}
}

func (m *Model) hitTestRecentsControlClick(col, absolute int) (recentsControlClick, bool) {
	if m == nil {
		return recentsControlClick{}, false
	}
	for _, span := range m.contentBlockSpans {
		sessionID, ok := recentsSessionIDFromBlockID(span.ID)
		if !ok {
			continue
		}
		if absolute < span.StartLine || absolute > span.EndLine {
			continue
		}
		hit := recentsControlClick{sessionID: sessionID}
		for _, control := range span.MetaControls {
			if control.Line != absolute {
				continue
			}
			if control.Start < 0 || control.End < control.Start {
				continue
			}
			if col < control.Start || col > control.End {
				continue
			}
			controlID := control.ID
			if strings.TrimSpace(string(controlID)) == "" {
				controlID = recentsControlIDFromLabel(control.Label)
			}
			hit.controlID = controlID
			break
		}
		return hit, true
	}
	return recentsControlClick{}, false
}

func (m *Model) applyRecentsControlClick(hit recentsControlClick) tea.Cmd {
	if m == nil || strings.TrimSpace(hit.sessionID) == "" {
		return nil
	}
	if m.setRecentsSelection(hit.sessionID) {
		m.refreshRecentsContent()
	}
	if strings.TrimSpace(string(hit.controlID)) == "" {
		return m.ensureRecentsPreviewForSelection()
	}
	switch hit.controlID {
	case recentsControlReply:
		if !m.startRecentsReply() {
			m.setValidationStatus("select a session to reply")
		}
		return nil
	case recentsControlExpand:
		if m.toggleSelectedRecentsExpand() {
			return m.ensureRecentsPreviewForSelection()
		}
		return nil
	case recentsControlOpen:
		return m.openSelectedRecentsThread()
	case recentsControlDismiss:
		if !m.dismissSelectedRecentsReady() {
			m.setValidationStatus("select a ready session to dismiss")
			return nil
		}
		return m.requestRecentsStateSaveCmd()
	default:
		// Unknown controls are consumed to avoid accidental fallback actions.
		return nil
	}
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
	if m.syncAppStateRecents() {
		m.hasAppState = true
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
	if !m.recentsCompletionPolicyOrDefault().ShouldWatchCompletion(provider) {
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
	completionTurn := m.recentsCompletionPolicyOrDefault().CompletionTurnID(msg.turnID, m.sessionMeta[sessionID])
	if _, ok := m.recents.CompleteRun(sessionID, expectedTurn, completionTurn, time.Now().UTC()); !ok {
		return nil
	}
	m.refreshRecentsSidebarState()
	if m.mode == uiModeRecents {
		m.refreshRecentsContent()
	}
	return m.requestRecentsStateSaveCmd()
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
		case "1":
			return true, m.switchRecentsFilter(sidebarRecentsFilterAll)
		case "2":
			return true, m.switchRecentsFilter(sidebarRecentsFilterReady)
		case "3":
			return true, m.switchRecentsFilter(sidebarRecentsFilterRunning)
		case "tab":
			return true, m.cycleRecentsFilter(1)
		case "shift+tab", "backtab":
			return true, m.cycleRecentsFilter(-1)
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
				return true, m.requestRecentsStateSaveCmd()
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

func (m *Model) recentsMetaFallbackMap() map[string]*types.SessionMeta {
	if m == nil || len(m.sessionMeta) == 0 {
		return nil
	}
	policy := m.recentsCompletionPolicyOrDefault()
	providerBySessionID := make(map[string]string, len(m.sessions))
	for _, session := range m.sessions {
		if session == nil {
			continue
		}
		sessionID := strings.TrimSpace(session.ID)
		if sessionID == "" {
			continue
		}
		providerBySessionID[sessionID] = strings.TrimSpace(session.Provider)
	}
	filtered := make(map[string]*types.SessionMeta, len(m.sessionMeta))
	for sessionID, meta := range m.sessionMeta {
		sessionID = strings.TrimSpace(sessionID)
		if sessionID == "" || meta == nil {
			continue
		}
		provider, ok := providerBySessionID[sessionID]
		if !ok || !policy.ShouldUseMetaFallback(provider) {
			continue
		}
		filtered[sessionID] = meta
	}
	return filtered
}

func (m *Model) recentsSectionEmptyText(title string) string {
	lower := strings.ToLower(strings.TrimSpace(title))
	switch lower {
	case "ready":
		return "No ready sessions waiting for reply."
	case "running":
		if m != nil && m.sessionMetaRefreshPending {
			return "Refreshing running sessions..."
		}
		return "No running sessions yet."
	default:
		return "No recents to show yet."
	}
}

func recentsEntryDetail(entry recentsEntry) string {
	switch entry.Status {
	case recentsEntryReady:
		if !entry.CompletedAt.IsZero() {
			completedAt := entry.CompletedAt
			return "completed " + formatSince(&completedAt)
		}
		return "awaiting reply"
	default:
		if lastActive := sessionLastActive(entry.Session, entry.Meta); lastActive != nil {
			return "updated " + formatSince(lastActive)
		}
		return "running"
	}
}

func (m *Model) switchRecentsFilter(filter sidebarRecentsFilter) tea.Cmd {
	if m == nil || m.sidebar == nil {
		return nil
	}
	targetKey := recentsFilterKey(filter)
	if targetKey == "" || strings.TrimSpace(m.sidebar.SelectedKey()) == targetKey {
		return nil
	}
	for idx, raw := range m.sidebar.Items() {
		entry, ok := raw.(*sidebarItem)
		if !ok || entry == nil {
			continue
		}
		if strings.TrimSpace(entry.key()) != targetKey {
			continue
		}
		m.sidebar.Select(idx)
		return m.onSelectionChangedImmediate()
	}
	return nil
}

func (m *Model) cycleRecentsFilter(step int) tea.Cmd {
	if m == nil || m.sidebar == nil || step == 0 {
		return nil
	}
	order := []sidebarRecentsFilter{
		sidebarRecentsFilterAll,
		sidebarRecentsFilterReady,
		sidebarRecentsFilterRunning,
	}
	current := m.recentsFilter()
	index := 0
	for i, filter := range order {
		if filter == current {
			index = i
			break
		}
	}
	next := index + step
	for next < 0 {
		next += len(order)
	}
	next = next % len(order)
	return m.switchRecentsFilter(order[next])
}

func recentsFilterKey(filter sidebarRecentsFilter) string {
	switch filter {
	case sidebarRecentsFilterAll:
		return "recents:all"
	case sidebarRecentsFilterReady:
		return "recents:ready"
	case sidebarRecentsFilterRunning:
		return "recents:running"
	default:
		return ""
	}
}
