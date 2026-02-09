package app

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	xansi "github.com/charmbracelet/x/ansi"

	"control/internal/client"
	"control/internal/providers"
	"control/internal/types"
)

const (
	defaultTailLines          = 200
	maxViewportLines          = 2000
	maxEventsPerTick          = 64
	tickInterval              = 100 * time.Millisecond
	selectionDebounce         = 500 * time.Millisecond
	sidebarWheelCooldown      = 30 * time.Millisecond
	sidebarWheelSettle        = 120 * time.Millisecond
	historyPollDelay          = 500 * time.Millisecond
	historyPollMax            = 20
	composeHistoryMaxEntries  = 200
	composeHistoryMaxSessions = 200
	toastDuration             = 2 * time.Second
	viewportScrollbarWidth    = 1
	minListWidth              = 24
	maxListWidth              = 40
	minViewportWidth          = 20
	minContentHeight          = 6
	statusLinePadding         = 1
	requestStaleRefreshDelay  = 4 * time.Second
	requestRefreshCooldown    = 3 * time.Second
)

type uiMode int

const (
	uiModeNormal uiMode = iota
	uiModeNotes
	uiModeAddNote
	uiModeAddWorkspace
	uiModeAddWorkspaceGroup
	uiModeAddWorktree
	uiModePickProvider
	uiModeCompose
	uiModeSearch
	uiModeRenameWorkspace
	uiModeEditWorkspaceGroups
	uiModePickWorkspaceRename
	uiModePickWorkspaceGroupEdit
	uiModePickWorkspaceGroupRename
	uiModeRenameWorkspaceGroup
	uiModePickWorkspaceGroupAssign
	uiModePickWorkspaceGroupDelete
	uiModeAssignGroupWorkspaces
)

type Model struct {
	workspaceAPI               WorkspaceAPI
	sessionAPI                 SessionAPI
	notesAPI                   NotesAPI
	stateAPI                   StateAPI
	sidebar                    *SidebarController
	viewport                   viewport.Model
	mode                       uiMode
	addWorkspace               *AddWorkspaceController
	addWorktree                *AddWorktreeController
	providerPicker             *ProviderPicker
	compose                    *ComposeController
	chatInput                  *ChatInput
	searchInput                *ChatInput
	renameInput                *ChatInput
	groupInput                 *ChatInput
	groupPicker                *GroupPicker
	workspacePicker            *SelectPicker
	groupSelectPicker          *SelectPicker
	workspaceMulti             *MultiSelectPicker
	noteInput                  *ChatInput
	renameWorkspaceID          string
	editWorkspaceID            string
	renameGroupID              string
	assignGroupID              string
	status                     string
	toastText                  string
	toastLevel                 toastLevel
	toastUntil                 time.Time
	width                      int
	height                     int
	follow                     bool
	workspaces                 []*types.Workspace
	groups                     []*types.WorkspaceGroup
	worktrees                  map[string][]*types.Worktree
	sessions                   []*types.Session
	sessionMeta                map[string]*types.SessionMeta
	appState                   types.AppState
	hasAppState                bool
	stream                     *StreamController
	codexStream                *CodexStreamController
	itemStream                 *ItemStreamController
	input                      *InputController
	chat                       *SessionChatController
	pendingApproval            *ApprovalRequest
	sessionApprovals           map[string][]*ApprovalRequest
	sessionApprovalResolutions map[string][]*ApprovalResolution
	contentRaw                 string
	contentEsc                 bool
	contentBlocks              []ChatBlock
	contentBlockSpans          []renderedBlockSpan
	reasoningExpanded          map[string]bool
	renderedText               string
	renderedLines              []string
	renderedPlain              []string
	contentVersion             int
	renderVersion              int
	renderedForWidth           int
	renderedForContent         int
	renderedForSelection       int
	searchQuery                string
	searchMatches              []int
	searchIndex                int
	searchVersion              int
	messageSelectActive        bool
	messageSelectIndex         int
	sectionOffsets             []int
	sectionVersion             int
	transcriptCache            map[string][]ChatBlock
	pendingSessionKey          string
	loading                    bool
	loadingKey                 string
	loader                     spinner.Model
	pendingMouseCmd            tea.Cmd
	lastSidebarWheelAt         time.Time
	pendingSidebarWheel        bool
	sidebarDragging            bool
	menu                       *MenuController
	hotkeys                    *HotkeyRenderer
	contextMenu                *ContextMenuController
	confirm                    *ConfirmController
	newSession                 *newSessionTarget
	pendingSelectID            string
	selectSeq                  int
	sendSeq                    int
	pendingSends               map[int]pendingSend
	composeHistory             map[string]*composeHistoryState
	requestActivity            requestActivity
	tickFn                     func() tea.Cmd
	pendingConfirm             confirmAction
	scrollOnLoad               bool
	notes                      []*types.Note
	notesScope                 noteScopeTarget
	notesReturnMode            uiMode
}

type newSessionTarget struct {
	workspaceID string
	worktreeID  string
	provider    string
}

type pendingSend struct {
	key        string
	sessionID  string
	headerLine int
	provider   string
}

type composeHistoryState struct {
	entries []string
	cursor  int
	draft   string
}

type confirmActionKind int

const (
	confirmNone confirmActionKind = iota
	confirmDeleteWorkspace
	confirmDeleteWorkspaceGroup
	confirmDeleteWorktree
	confirmDismissSessions
)

type confirmAction struct {
	kind        confirmActionKind
	workspaceID string
	groupID     string
	worktreeID  string
	sessionIDs  []string
}

func NewModel(client *client.Client) Model {
	vp := viewport.New(minViewportWidth, minContentHeight-1)
	vp.SetContent("No sessions.")

	api := NewClientAPI(client)
	stream := NewStreamController(maxViewportLines, maxEventsPerTick)
	codexStream := NewCodexStreamController(maxViewportLines, maxEventsPerTick)
	itemStream := NewItemStreamController(maxViewportLines, maxEventsPerTick)
	loader := spinner.New()
	loader.Spinner = spinner.Line
	loader.Style = lipgloss.NewStyle()
	hotkeyRenderer := NewHotkeyRenderer(DefaultHotkeys(), DefaultHotkeyResolver{})

	return Model{
		workspaceAPI:               api,
		sessionAPI:                 api,
		notesAPI:                   api,
		stateAPI:                   api,
		sidebar:                    NewSidebarController(),
		viewport:                   vp,
		stream:                     stream,
		codexStream:                codexStream,
		itemStream:                 itemStream,
		input:                      NewInputController(),
		chat:                       NewSessionChatController(api, codexStream),
		mode:                       uiModeNormal,
		addWorkspace:               NewAddWorkspaceController(minViewportWidth),
		addWorktree:                NewAddWorktreeController(minViewportWidth),
		providerPicker:             NewProviderPicker(minViewportWidth, minContentHeight-1),
		compose:                    NewComposeController(minViewportWidth),
		chatInput:                  NewChatInput(minViewportWidth, DefaultChatInputConfig()),
		searchInput:                NewChatInput(minViewportWidth, ChatInputConfig{Height: 1}),
		renameInput:                NewChatInput(minViewportWidth, ChatInputConfig{Height: 1}),
		groupInput:                 NewChatInput(minViewportWidth, ChatInputConfig{Height: 1}),
		groupPicker:                NewGroupPicker(minViewportWidth, minContentHeight-1),
		workspacePicker:            NewSelectPicker(minViewportWidth, minContentHeight-1),
		groupSelectPicker:          NewSelectPicker(minViewportWidth, minContentHeight-1),
		workspaceMulti:             NewMultiSelectPicker(minViewportWidth, minContentHeight-1),
		noteInput:                  NewChatInput(minViewportWidth, ChatInputConfig{Height: 1}),
		status:                     "",
		toastLevel:                 toastLevelInfo,
		follow:                     true,
		groups:                     []*types.WorkspaceGroup{},
		worktrees:                  map[string][]*types.Worktree{},
		sessionMeta:                map[string]*types.SessionMeta{},
		contentRaw:                 "No sessions.",
		contentEsc:                 false,
		searchIndex:                -1,
		searchVersion:              -1,
		messageSelectIndex:         -1,
		renderedForSelection:       -2,
		sectionVersion:             -1,
		transcriptCache:            map[string][]ChatBlock{},
		reasoningExpanded:          map[string]bool{},
		sessionApprovals:           map[string][]*ApprovalRequest{},
		sessionApprovalResolutions: map[string][]*ApprovalResolution{},
		loader:                     loader,
		hotkeys:                    hotkeyRenderer,
		pendingSends:               map[int]pendingSend{},
		composeHistory:             map[string]*composeHistoryState{},
		menu:                       NewMenuController(),
		contextMenu:                NewContextMenuController(),
		confirm:                    NewConfirmController(),
	}
}

func Run(client *client.Client) error {
	model := NewModel(client)
	p := tea.NewProgram(&model, tea.WithAltScreen(), tea.WithMouseAllMotion())
	_, err := p.Run()
	return err
}

func (m *Model) Init() tea.Cmd {
	return tea.Batch(
		fetchAppStateCmd(m.stateAPI),
		fetchWorkspacesCmd(m.workspaceAPI),
		fetchWorkspaceGroupsCmd(m.workspaceAPI),
		fetchSessionsWithMetaCmd(m.sessionAPI),
		m.tickCmd(),
	)
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tickMsg:
		return m, m.handleTick(msg)
	}
	if handled, cmd := m.reduceMutationMessages(msg); handled {
		return m, cmd
	}

	if m.confirm != nil && m.confirm.IsOpen() {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			if handled, choice := m.confirm.HandleKey(msg); handled {
				if choice == confirmChoiceNone {
					return m, nil
				}
				return m, m.handleConfirmChoice(choice)
			}
			return m, nil
		case tea.MouseMsg:
			if handled, choice := m.confirm.HandleMouse(msg, m.width, m.height-1); handled {
				if choice == confirmChoiceNone {
					return m, nil
				}
				return m, m.handleConfirmChoice(choice)
			}
			if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
				if !m.confirm.Contains(msg.X, msg.Y, m.width, m.height-1) {
					m.confirm.Close()
					m.pendingConfirm = confirmAction{}
				}
				return m, nil
			}
			return m, nil
		}
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		if m.contextMenu != nil && m.contextMenu.IsOpen() {
			if handled, action := m.contextMenu.HandleKey(msg); handled {
				if action != ContextMenuNone {
					return m, m.handleContextMenuAction(action)
				}
				return m, nil
			}
		}
	}

	if msg, ok := msg.(tea.MouseMsg); ok {
		previous := []string{}
		if m.menu != nil {
			previous = m.menu.SelectedGroupIDs()
		}
		if m.handleMouse(msg) {
			cmd := m.pendingMouseCmd
			m.pendingMouseCmd = nil
			if m.menu != nil && m.handleMenuGroupChange(previous) {
				save := m.saveAppStateCmd()
				if cmd != nil && save != nil {
					cmd = tea.Batch(cmd, save)
				} else if save != nil {
					cmd = save
				}
			}
			if cmd != nil {
				return m, cmd
			}
			return m, nil
		}
	}

	if handled, cmd := m.reduceAddWorkspaceMode(msg); handled {
		return m, cmd
	}
	if handled, cmd := m.reduceAddWorktreeMode(msg); handled {
		return m, cmd
	}
	if handled, cmd := m.reduceWorkspaceEditModes(msg); handled {
		return m, cmd
	}
	if handled, cmd := m.reducePickProviderMode(msg); handled {
		return m, cmd
	}
	if handled, cmd := m.reduceMenuMode(msg); handled {
		return m, cmd
	}
	if handled, cmd := m.reduceComposeMode(msg); handled {
		return m, cmd
	}
	if handled, cmd := m.reduceAddNoteMode(msg); handled {
		return m, cmd
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.resize(msg.Width, msg.Height)
		return m, nil
	case tea.KeyMsg:
		if handled, cmd := m.reduceNotesModeKey(msg); handled {
			return m, cmd
		}
		if handled, cmd := m.reduceSearchModeKey(msg); handled {
			return m, cmd
		}
		if handled, cmd := m.reduceMessageSelectionKey(msg); handled {
			return m, cmd
		}
		if handled, cmd := m.reducePendingApprovalKey(msg); handled {
			return m, cmd
		}
		if handled, cmd := m.reduceComposeInputKey(msg); handled {
			return m, cmd
		}
		if handled, cmd := m.reduceSidebarArrowKey(msg); handled {
			return m, cmd
		}
		if m.handleViewportScroll(msg) {
			return m, nil
		}
		if m.mode == uiModeNotes || m.mode == uiModeAddNote {
			return m, nil
		}
		if handled, cmd := m.reduceNormalModeKey(msg); handled {
			return m, cmd
		}
	}

	prevKey := m.selectedKey()
	var cmd tea.Cmd
	cmd = m.sidebar.Update(msg)
	if key := m.selectedKey(); key != prevKey {
		return m, tea.Batch(cmd, m.onSelectionChanged())
	}

	if handled, nextCmd := m.reduceStateMessages(msg); handled {
		return m, nextCmd
	}

	return m, cmd
}

func (m *Model) View() string {
	rightView := m.renderRightPaneView()
	body := m.renderBodyWithSidebar(rightView)
	statusLine := m.renderStatusLineView()
	if m.height <= 0 || m.width <= 0 {
		return body
	}
	body = m.overlayTransientViews(body)
	return lipgloss.JoinVertical(lipgloss.Left, body, statusLine)
}

func (m *Model) resize(width, height int) {
	m.width = width
	m.height = height

	contentHeight := max(minContentHeight, height-2)
	listWidth := 0
	viewportWidth := width
	if !m.appState.SidebarCollapsed {
		listWidth = clamp(width/3, minListWidth, maxListWidth)
		if width-listWidth-1 < minViewportWidth {
			listWidth = max(minListWidth, width/2)
		}
		viewportWidth = max(minViewportWidth, width-listWidth-1)
	}
	contentWidth := viewportWidth
	if m.usesViewport() && viewportWidth > minViewportWidth+viewportScrollbarWidth {
		contentWidth = viewportWidth - viewportScrollbarWidth
	}

	if m.sidebar != nil {
		m.sidebar.SetSize(listWidth, contentHeight)
	}
	extraLines := 0
	if m.mode == uiModeCompose {
		if m.chatInput != nil {
			extraLines = m.chatInput.Height() + 1
		} else {
			extraLines = 2
		}
	} else if m.mode == uiModeAddNote {
		if m.noteInput != nil {
			extraLines = m.noteInput.Height() + 1
		} else {
			extraLines = 2
		}
	} else if m.mode == uiModeSearch {
		extraLines = 2
	}
	vpHeight := max(1, contentHeight-1-extraLines)
	m.viewport.Width = contentWidth
	m.viewport.Height = vpHeight
	if m.addWorkspace != nil {
		m.addWorkspace.Resize(viewportWidth)
	}
	if m.addWorktree != nil {
		m.addWorktree.Resize(viewportWidth)
		m.addWorktree.SetListHeight(max(3, contentHeight-4))
	}
	if m.providerPicker != nil {
		m.providerPicker.SetSize(viewportWidth, max(3, contentHeight-2))
	}
	if m.compose != nil {
		m.compose.Resize(viewportWidth)
	}
	if m.groupPicker != nil {
		m.groupPicker.SetSize(viewportWidth, max(3, contentHeight-2))
	}
	if m.workspacePicker != nil {
		m.workspacePicker.SetSize(viewportWidth, max(3, contentHeight-2))
	}
	if m.groupSelectPicker != nil {
		m.groupSelectPicker.SetSize(viewportWidth, max(3, contentHeight-2))
	}
	if m.workspaceMulti != nil {
		m.workspaceMulti.SetSize(viewportWidth, max(3, contentHeight-2))
	}
	if m.chatInput != nil {
		m.chatInput.Resize(viewportWidth)
	}
	if m.searchInput != nil {
		m.searchInput.Resize(viewportWidth)
	}
	if m.noteInput != nil {
		m.noteInput.Resize(viewportWidth)
	}
	m.renderViewport()
}

func (m *Model) onSelectionChanged() tea.Cmd {
	return m.onSelectionChangedWithDelay(selectionDebounce)
}

func (m *Model) onSelectionChangedImmediate() tea.Cmd {
	return m.onSelectionChangedWithDelay(0)
}

func (m *Model) onSelectionChangedWithDelay(delay time.Duration) tea.Cmd {
	item := m.selectedItem()
	if item == nil {
		m.resetStream()
		m.setContentText("No sessions.")
		return nil
	}
	if item.kind == sidebarWorkspace {
		stateChanged := false
		maybeID := item.workspaceID()
		if maybeID == unassignedWorkspaceID {
			maybeID = ""
		}
		if maybeID != "" && maybeID != m.appState.ActiveWorkspaceID {
			m.appState.ActiveWorkspaceID = maybeID
			m.hasAppState = true
			m.updateDelegate()
			stateChanged = true
		}
		if m.appState.ActiveWorktreeID != "" {
			m.appState.ActiveWorktreeID = ""
			m.hasAppState = true
			m.updateDelegate()
			stateChanged = true
		}
		m.setStatusMessage("workspace selected")
		if stateChanged {
			return m.saveAppStateCmd()
		}
		return nil
	}
	if item.kind == sidebarWorktree {
		stateChanged := false
		wsID := item.workspaceID()
		if wsID != "" && wsID != m.appState.ActiveWorkspaceID {
			m.appState.ActiveWorkspaceID = wsID
			m.hasAppState = true
			m.updateDelegate()
			stateChanged = true
		}
		if item.worktree != nil && item.worktree.ID != m.appState.ActiveWorktreeID {
			m.appState.ActiveWorktreeID = item.worktree.ID
			m.hasAppState = true
			m.updateDelegate()
			stateChanged = true
		}
		m.setStatusMessage("worktree selected")
		if stateChanged {
			return m.saveAppStateCmd()
		}
		return nil
	}
	if !item.isSession() {
		return nil
	}
	if m.mode == uiModeCompose && m.compose != nil && item.session != nil {
		m.compose.SetSession(item.session.ID, sessionTitle(item.session, item.meta))
		m.resetComposeHistoryCursor()
	}
	stateChanged := false
	if wsID := item.workspaceID(); wsID != "" && wsID != unassignedWorkspaceID && wsID != m.appState.ActiveWorkspaceID {
		m.appState.ActiveWorkspaceID = wsID
		m.hasAppState = true
		m.updateDelegate()
		stateChanged = true
	}
	wtID := ""
	if item.meta != nil {
		wtID = item.meta.WorktreeID
	}
	if wtID != m.appState.ActiveWorktreeID {
		m.appState.ActiveWorktreeID = wtID
		m.hasAppState = true
		m.updateDelegate()
		stateChanged = true
	}
	cmd := m.scheduleSessionLoad(item, delay)
	if stateChanged {
		return tea.Batch(cmd, m.saveAppStateCmd())
	}
	return cmd
}

func (m *Model) scheduleSessionLoad(item *sidebarItem, delay time.Duration) tea.Cmd {
	if item == nil || item.session == nil {
		return nil
	}
	if delay <= 0 {
		return m.loadSelectedSession(item)
	}
	m.selectSeq++
	return debounceSelectCmd(item.session.ID, m.selectSeq, delay)
}

func (m *Model) loadSelectedSession(item *sidebarItem) tea.Cmd {
	if item == nil || item.session == nil {
		return nil
	}
	id := item.session.ID
	m.resetStream()
	m.pendingApproval = nil
	m.pendingSessionKey = item.key()
	m.setStatusMessage("loading " + id)
	m.scrollOnLoad = true
	if cached, ok := m.transcriptCache[item.key()]; ok {
		m.setSnapshotBlocks(cached)
		m.loading = false
		m.loadingKey = ""
	} else {
		m.loading = true
		m.loadingKey = item.key()
		m.setLoadingContent()
	}
	cmds := []tea.Cmd{fetchHistoryCmd(m.sessionAPI, id, item.key(), maxViewportLines), fetchApprovalsCmd(m.sessionAPI, id)}
	if isActiveStatus(item.session.Status) {
		if shouldStreamItems(item.session.Provider) {
			cmds = append(cmds, openItemsCmd(m.sessionAPI, id))
		} else {
			cmds = append(cmds, openStreamCmd(m.sessionAPI, id))
		}
	}
	if item.session.Provider == "codex" {
		cmds = append(cmds, openEventsCmd(m.sessionAPI, id))
	}
	return tea.Batch(cmds...)
}

func (m *Model) selectedSessionID() string {
	if m.sidebar == nil {
		return ""
	}
	return m.sidebar.SelectedSessionID()
}

func (m *Model) selectedItem() *sidebarItem {
	if m.sidebar == nil {
		return nil
	}
	return m.sidebar.SelectedItem()
}

func (m *Model) selectedKey() string {
	if m.sidebar == nil {
		return ""
	}
	return m.sidebar.SelectedKey()
}

func sessionIDFromSidebarKey(key string) string {
	key = strings.TrimSpace(key)
	if key == "" || !strings.HasPrefix(key, "sess:") {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(key, "sess:"))
}

func (m *Model) filteredWorkspaces() []*types.Workspace {
	selected := map[string]bool{}
	for _, id := range m.appState.ActiveWorkspaceGroupIDs {
		selected[id] = true
	}
	if len(selected) == 0 {
		return []*types.Workspace{}
	}
	out := make([]*types.Workspace, 0, len(m.workspaces))
	for _, ws := range m.workspaces {
		if ws == nil {
			continue
		}
		groupIDs := ws.GroupIDs
		if len(groupIDs) == 0 {
			if selected["ungrouped"] {
				out = append(out, ws)
			}
			continue
		}
		for _, id := range groupIDs {
			if selected[id] {
				out = append(out, ws)
				break
			}
		}
	}
	return out
}

func (m *Model) filteredSessions(workspaces []*types.Workspace) []*types.Session {
	selected := map[string]bool{}
	for _, id := range m.appState.ActiveWorkspaceGroupIDs {
		selected[id] = true
	}
	if len(selected) == 0 {
		return []*types.Session{}
	}
	visibleWorkspaces := map[string]struct{}{}
	for _, ws := range workspaces {
		if ws == nil {
			continue
		}
		visibleWorkspaces[ws.ID] = struct{}{}
	}
	visibleWorktrees := map[string]struct{}{}
	for wsID := range visibleWorkspaces {
		for _, wt := range m.worktrees[wsID] {
			if wt == nil {
				continue
			}
			visibleWorktrees[wt.ID] = struct{}{}
		}
	}
	out := make([]*types.Session, 0, len(m.sessions))
	for _, session := range m.sessions {
		if session == nil {
			continue
		}
		meta := m.sessionMeta[session.ID]
		workspaceID := ""
		worktreeID := ""
		if meta != nil {
			workspaceID = meta.WorkspaceID
			worktreeID = meta.WorktreeID
		}
		if worktreeID != "" {
			if _, ok := visibleWorktrees[worktreeID]; ok {
				out = append(out, session)
			}
			continue
		}
		if workspaceID != "" {
			if _, ok := visibleWorkspaces[workspaceID]; ok {
				out = append(out, session)
			}
			continue
		}
		if selected["ungrouped"] {
			out = append(out, session)
		}
	}
	return out
}

func (m *Model) handleMenuGroupChange(previous []string) bool {
	if m.menu == nil {
		return false
	}
	next := m.menu.SelectedGroupIDs()
	if slicesEqual(previous, next) {
		return false
	}
	m.appState.ActiveWorkspaceGroupIDs = next
	m.hasAppState = true
	m.applySidebarItems()
	return true
}

func (m *Model) applySidebarItems() {
	if m.sidebar == nil {
		m.resetStream()
		m.setContentText("No sessions.")
		return
	}
	workspaces := m.filteredWorkspaces()
	sessions := m.filteredSessions(workspaces)
	item := m.sidebar.Apply(workspaces, m.worktrees, sessions, m.sessionMeta, m.appState.ActiveWorkspaceID, m.appState.ActiveWorktreeID)
	if item == nil {
		m.resetStream()
		m.setContentText("No sessions.")
		return
	}
	if !item.isSession() {
		m.resetStream()
		m.setContentText("Select a session.")
	}
}

func (m *Model) sidebarWidth() int {
	if m.appState.SidebarCollapsed {
		return 0
	}
	listWidth := clamp(m.width/3, minListWidth, maxListWidth)
	if m.width-listWidth-1 < minViewportWidth {
		listWidth = max(minListWidth, m.width/2)
	}
	return listWidth
}

func (m *Model) handleMenuAction(action MenuAction) tea.Cmd {
	if m.contextMenu != nil {
		m.contextMenu.Close()
	}
	switch action {
	case MenuActionCreateWorkspace:
		if m.menu != nil {
			m.menu.CloseAll()
		}
		m.enterAddWorkspace()
	case MenuActionRenameWorkspace:
		if m.menu != nil {
			m.menu.CloseAll()
		}
		m.enterWorkspacePicker(uiModePickWorkspaceRename)
	case MenuActionDeleteWorkspace:
		id := m.appState.ActiveWorkspaceID
		if id == "" || id == unassignedWorkspaceID {
			m.setValidationStatus("select a workspace to delete")
			return nil
		}
		if m.menu != nil {
			m.menu.CloseAll()
		}
		m.confirmDeleteWorkspace(id)
		return nil
	case MenuActionEditWorkspaceGroups:
		if m.menu != nil {
			m.menu.CloseAll()
		}
		m.enterWorkspacePicker(uiModePickWorkspaceGroupEdit)
	case MenuActionCreateWorkspaceGroup:
		if m.menu != nil {
			m.menu.CloseAll()
		}
		m.enterAddWorkspaceGroup()
	case MenuActionRenameWorkspaceGroup:
		if m.menu != nil {
			m.menu.CloseAll()
		}
		m.enterGroupPicker()
	case MenuActionDeleteWorkspaceGroup:
		if m.menu != nil {
			m.menu.CloseAll()
		}
		m.enterGroupDeletePicker()
	case MenuActionAssignWorkspacesToGroup:
		if m.menu != nil {
			m.menu.CloseAll()
		}
		m.enterGroupAssignPicker()
	}
	return nil
}

func (m *Model) handleContextMenuAction(action ContextMenuAction) tea.Cmd {
	if m.contextMenu == nil {
		return nil
	}
	target := contextMenuTarget{
		id:          m.contextMenu.TargetID(),
		workspaceID: m.contextMenu.WorkspaceID(),
		worktreeID:  m.contextMenu.WorktreeID(),
		sessionID:   m.contextMenu.SessionID(),
	}
	m.contextMenu.Close()
	if handled, cmd := m.handleWorkspaceContextMenuAction(action, target); handled {
		return cmd
	}
	if handled, cmd := m.handleWorktreeContextMenuAction(action, target); handled {
		return cmd
	}
	if handled, cmd := m.handleSessionContextMenuAction(action, target); handled {
		return cmd
	}
	return nil
}

func (m *Model) handleConfirmChoice(choice confirmChoice) tea.Cmd {
	if m.confirm == nil || !m.confirm.IsOpen() {
		return nil
	}
	defer m.confirm.Close()
	action := m.pendingConfirm
	m.pendingConfirm = confirmAction{}
	switch choice {
	case confirmChoiceConfirm:
		switch action.kind {
		case confirmDeleteWorkspace:
			if action.workspaceID == "" || action.workspaceID == unassignedWorkspaceID {
				m.setValidationStatus("select a workspace to delete")
				return nil
			}
			m.setStatusMessage("deleting workspace")
			return deleteWorkspaceCmd(m.workspaceAPI, action.workspaceID)
		case confirmDeleteWorkspaceGroup:
			if action.groupID == "" {
				m.setValidationStatus("select a group to delete")
				return nil
			}
			m.setStatusMessage("deleting group")
			return deleteWorkspaceGroupCmd(m.workspaceAPI, action.groupID)
		case confirmDeleteWorktree:
			if action.workspaceID == "" || action.worktreeID == "" {
				m.setValidationStatus("select a worktree to delete")
				return nil
			}
			m.setStatusMessage("deleting worktree")
			return deleteWorktreeCmd(m.workspaceAPI, action.workspaceID, action.worktreeID)
		case confirmDismissSessions:
			if len(action.sessionIDs) == 0 {
				m.setValidationStatus("no session selected")
				return nil
			}
			if len(action.sessionIDs) == 1 {
				m.setStatusMessage("marking exited " + action.sessionIDs[0])
				return markExitedCmd(m.sessionAPI, action.sessionIDs[0])
			}
			m.setStatusMessage(fmt.Sprintf("marking exited %d sessions", len(action.sessionIDs)))
			return markExitedManyCmd(m.sessionAPI, action.sessionIDs)
		}
		m.setStatusMessage("confirmed")
		return nil
	case confirmChoiceCancel:
		m.setStatusMessage("canceled")
		return nil
	default:
		return nil
	}
}

func (m *Model) confirmDeleteWorkspace(id string) {
	if m.confirm == nil {
		return
	}
	name := ""
	if ws := m.workspaceByID(id); ws != nil {
		name = ws.Name
	}
	message := "Delete workspace?"
	if strings.TrimSpace(name) != "" {
		message = fmt.Sprintf("Delete workspace %q?", name)
	}
	m.pendingConfirm = confirmAction{
		kind:        confirmDeleteWorkspace,
		workspaceID: id,
	}
	if m.menu != nil {
		m.menu.CloseAll()
	}
	if m.contextMenu != nil {
		m.contextMenu.Close()
	}
	m.confirm.Open("Delete Workspace", message, "Delete", "Cancel")
}

func (m *Model) confirmDismissSessions(ids []string) {
	if m.confirm == nil {
		return
	}
	count := len(ids)
	message := "Dismiss session?"
	if count > 1 {
		message = fmt.Sprintf("Dismiss %d sessions?", count)
	}
	m.pendingConfirm = confirmAction{
		kind:       confirmDismissSessions,
		sessionIDs: append([]string{}, ids...),
	}
	if m.menu != nil {
		m.menu.CloseAll()
	}
	if m.contextMenu != nil {
		m.contextMenu.Close()
	}
	m.confirm.Open("Dismiss Sessions", message, "Dismiss", "Cancel")
}

func (m *Model) confirmDeleteWorkspaceGroup(id string) {
	if m.confirm == nil {
		return
	}
	name := ""
	for _, group := range m.groups {
		if group != nil && group.ID == id {
			name = group.Name
			break
		}
	}
	message := "Delete workspace group?"
	if strings.TrimSpace(name) != "" {
		message = fmt.Sprintf("Delete workspace group %q?", name)
	}
	m.pendingConfirm = confirmAction{
		kind:    confirmDeleteWorkspaceGroup,
		groupID: id,
	}
	if m.menu != nil {
		m.menu.CloseAll()
	}
	if m.contextMenu != nil {
		m.contextMenu.Close()
	}
	m.confirm.Open("Delete Workspace Group", message, "Delete", "Cancel")
}

func (m *Model) confirmDeleteWorktree(workspaceID, worktreeID string) {
	if m.confirm == nil {
		return
	}
	name := ""
	if wt := m.worktreeByID(worktreeID); wt != nil {
		name = wt.Name
	}
	message := "Delete worktree?"
	if strings.TrimSpace(name) != "" {
		message = fmt.Sprintf("Delete worktree %q?", name)
	}
	m.pendingConfirm = confirmAction{
		kind:        confirmDeleteWorktree,
		workspaceID: workspaceID,
		worktreeID:  worktreeID,
	}
	if m.menu != nil {
		m.menu.CloseAll()
	}
	if m.contextMenu != nil {
		m.contextMenu.Close()
	}
	m.confirm.Open("Delete Worktree", message, "Delete", "Cancel")
}

func overlayLine(body, line string, row int) string {
	if body == "" || line == "" || row < 0 {
		return body
	}
	lines := strings.Split(body, "\n")
	if row >= len(lines) {
		return body
	}
	lines[row] = line
	return strings.Join(lines, "\n")
}

func overlayBlock(body, block string, row int) string {
	if body == "" || block == "" || row < 0 {
		return body
	}
	lines := strings.Split(body, "\n")
	blockLines := strings.Split(block, "\n")
	for i := 0; i < len(blockLines); i++ {
		idx := row + i
		if idx >= len(lines) {
			break
		}
		lines[idx] = blockLines[i]
	}
	return strings.Join(lines, "\n")
}

func combineBlocks(left, right string, gap int) string {
	if left == "" {
		return right
	}
	if right == "" {
		return left
	}
	leftLines := strings.Split(left, "\n")
	rightLines := strings.Split(right, "\n")
	leftWidth := blockWidth(leftLines)
	maxLines := max(len(leftLines), len(rightLines))
	lines := make([]string, 0, maxLines)
	for i := 0; i < maxLines; i++ {
		l := ""
		if i < len(leftLines) {
			l = leftLines[i]
		}
		l = padToWidth(l, leftWidth)
		r := ""
		if i < len(rightLines) {
			r = rightLines[i]
		}
		if r == "" {
			lines = append(lines, l)
			continue
		}
		lines = append(lines, l+strings.Repeat(" ", max(1, gap))+r)
	}
	return strings.Join(lines, "\n")
}

func blockWidth(lines []string) int {
	width := 0
	for _, line := range lines {
		if w := xansi.StringWidth(line); w > width {
			width = w
		}
	}
	return width
}

func padToWidth(line string, width int) string {
	if width <= 0 {
		return line
	}
	w := xansi.StringWidth(line)
	if w >= width {
		return line
	}
	return line + strings.Repeat(" ", width-w)
}

func tickCmd() tea.Cmd {
	return tea.Tick(tickInterval, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
}

func (m *Model) tickCmd() tea.Cmd {
	if m != nil && m.tickFn != nil {
		return m.tickFn()
	}
	return tickCmd()
}

func (m *Model) handleTick(msg tickMsg) tea.Cmd {
	now := time.Time(msg)
	m.consumeStreamTick()
	m.consumeCodexTick()
	m.consumeItemTick()
	if m.loading {
		m.loader, _ = m.loader.Update(spinner.TickMsg{Time: now, ID: m.loader.ID()})
		m.setLoadingContent()
	}
	if m.toastText != "" && !m.toastActive(now) {
		m.clearToast()
	}
	cmds := make([]tea.Cmd, 0, 3)
	if m.pendingSidebarWheel {
		if time.Since(m.lastSidebarWheelAt) >= sidebarWheelSettle {
			m.pendingSidebarWheel = false
			if cmd := m.onSelectionChangedImmediate(); cmd != nil {
				cmds = append(cmds, cmd)
			}
		}
	}
	if cmd := m.maybeAutoRefreshHistory(now); cmd != nil {
		cmds = append(cmds, cmd)
	}
	cmds = append(cmds, m.tickCmd())
	return tea.Batch(cmds...)
}

func (m *Model) resetStream() {
	if m.stream != nil {
		m.stream.Reset()
	}
	if m.chat != nil {
		m.chat.CloseEventStream()
	} else if m.codexStream != nil {
		m.codexStream.Reset()
	}
	if m.itemStream != nil {
		m.itemStream.Reset()
	}
	m.pendingApproval = nil
	m.pendingSessionKey = ""
	m.loading = false
	m.loadingKey = ""
	m.stopRequestActivity()
}

func (m *Model) consumeStreamTick() {
	if m.stream == nil {
		return
	}
	lines, changed, closed := m.stream.ConsumeTick()
	if closed {
		m.setBackgroundStatus("stream closed")
	}
	if changed {
		m.applyLines(lines, true)
	}
}

func (m *Model) consumeCodexTick() {
	if m.codexStream == nil {
		return
	}
	changed, closed, events := m.codexStream.ConsumeTick()
	sessionID := m.activeStreamTargetID()
	if sessionID != "" && events > 0 {
		m.noteRequestEvent(sessionID, events)
	}
	if closed {
		m.setBackgroundStatus("events closed")
		if sessionID != "" {
			m.stopRequestActivityFor(sessionID)
		}
	}
	if changed {
		blocks := m.codexStream.Blocks()
		if sessionID != "" {
			blocks = mergeApprovalBlocks(blocks, m.sessionApprovals[sessionID], m.sessionApprovalResolutions[sessionID])
			m.codexStream.SetSnapshotBlocks(blocks)
			blocks = m.codexStream.Blocks()
		}
		m.applyBlocks(blocks)
		if sessionID != "" {
			m.noteRequestVisibleUpdate(sessionID)
			if key := m.selectedKey(); key != "" {
				m.transcriptCache[key] = append([]ChatBlock(nil), blocks...)
			}
		}
	}
	if errMsg := m.codexStream.LastError(); errMsg != "" {
		m.setBackgroundError("codex error: " + errMsg)
	}
	if approval := m.codexStream.PendingApproval(); approval != nil {
		if sessionID != "" {
			updated := m.upsertApprovalForSession(sessionID, approval)
			requests := m.sessionApprovals[sessionID]
			if updated {
				blocks := mergeApprovalBlocks(m.currentBlocks(), requests, m.sessionApprovalResolutions[sessionID])
				m.codexStream.SetSnapshotBlocks(blocks)
				blocks = m.codexStream.Blocks()
				m.applyBlocks(blocks)
				if key := m.selectedKey(); key != "" {
					m.transcriptCache[key] = append([]ChatBlock(nil), blocks...)
				}
			}
			m.pendingApproval = latestApprovalRequest(requests)
		} else {
			m.pendingApproval = cloneApprovalRequest(approval)
		}
		if m.pendingApproval != nil {
			if m.pendingApproval.Summary != "" {
				if m.pendingApproval.Detail != "" {
					m.setApprovalStatus(fmt.Sprintf("approval required: %s (%s)", m.pendingApproval.Summary, m.pendingApproval.Detail))
				} else {
					m.setApprovalStatus("approval required: " + m.pendingApproval.Summary)
				}
			} else {
				m.setApprovalStatus("approval required")
			}
		}
	}
}

func (m *Model) consumeItemTick() {
	if m.itemStream == nil {
		return
	}
	changed, closed := m.itemStream.ConsumeTick()
	sessionID := m.activeStreamTargetID()
	if closed {
		m.setBackgroundStatus("items stream closed")
		if sessionID != "" {
			m.stopRequestActivityFor(sessionID)
		}
	}
	if changed {
		m.applyBlocks(m.itemStream.Blocks())
		if sessionID != "" {
			m.noteRequestEvent(sessionID, 1)
			m.noteRequestVisibleUpdate(sessionID)
		}
	}
}

func (m *Model) toggleSelection() bool {
	if m.sidebar == nil {
		return false
	}
	return m.sidebar.ToggleSelection()
}

func (m *Model) selectedSessionIDs() []string {
	if m.sidebar == nil {
		return nil
	}
	return m.sidebar.SelectedSessionIDs()
}

func (m *Model) pruneSelection() {
	if m.sidebar == nil {
		return
	}
	m.sidebar.PruneSelection(m.sessions)
}

func (m *Model) workspaceByID(id string) *types.Workspace {
	for _, ws := range m.workspaces {
		if ws != nil && ws.ID == id {
			return ws
		}
	}
	return nil
}

func (m *Model) worktreeByID(id string) *types.Worktree {
	if id == "" {
		return nil
	}
	for _, entries := range m.worktrees {
		for _, wt := range entries {
			if wt != nil && wt.ID == id {
				return wt
			}
		}
	}
	return nil
}

func (m *Model) fetchWorktreesForWorkspaces() tea.Cmd {
	if m.workspaceAPI == nil || len(m.workspaces) == 0 {
		return nil
	}
	cmds := make([]tea.Cmd, 0, len(m.workspaces))
	for _, ws := range m.workspaces {
		if ws == nil {
			continue
		}
		cmds = append(cmds, fetchWorktreesCmd(m.workspaceAPI, ws.ID))
	}
	return tea.Batch(cmds...)
}

func (m *Model) advanceToNextSession() bool {
	if m.sidebar == nil {
		return false
	}
	return m.sidebar.AdvanceToNextSession()
}

func (m *Model) setSnapshot(lines []string, escape bool) {
	if m.stream != nil {
		m.stream.SetSnapshot(lines)
	}
	m.applyLines(lines, escape)
}

func (m *Model) setSnapshotBlocks(blocks []ChatBlock) {
	if m.stream != nil {
		m.stream.SetSnapshot(nil)
	}
	m.applyBlocks(blocks)
}

func (m *Model) applyLines(lines []string, escape bool) {
	m.contentRaw = strings.Join(lines, "\n")
	m.contentEsc = escape
	m.contentBlocks = nil
	m.contentBlockSpans = nil
	m.clearMessageSelection()
	m.contentVersion++
	m.searchVersion = -1
	m.sectionVersion = -1
	m.renderViewport()
}

func (m *Model) applyBlocks(blocks []ChatBlock) {
	if len(blocks) == 0 {
		m.contentBlocks = nil
		m.contentBlockSpans = nil
	} else {
		resolved := append([]ChatBlock(nil), blocks...)
		autoExpandNewest := m.shouldAutoExpandNewestReasoning()
		newestReasoning := -1
		if autoExpandNewest {
			for i := len(resolved) - 1; i >= 0; i-- {
				if resolved[i].Role == ChatRoleReasoning && strings.TrimSpace(resolved[i].Text) != "" {
					newestReasoning = i
					break
				}
			}
		}
		for i := range resolved {
			if resolved[i].Role != ChatRoleReasoning {
				continue
			}
			if strings.TrimSpace(resolved[i].ID) == "" {
				resolved[i].ID = makeChatBlockID(resolved[i].Role, i, resolved[i].Text)
			}
			if m.reasoningExpanded != nil {
				if expanded, ok := m.reasoningExpanded[resolved[i].ID]; ok {
					resolved[i].Collapsed = !expanded
					continue
				}
			}
			if autoExpandNewest && i == newestReasoning {
				resolved[i].Collapsed = false
				continue
			}
			resolved[i].Collapsed = true
		}
		m.contentBlocks = resolved
	}
	m.clampMessageSelection()
	m.contentRaw = ""
	m.contentEsc = false
	m.contentVersion++
	m.searchVersion = -1
	m.sectionVersion = -1
	m.renderViewport()
}

func (m *Model) shouldAutoExpandNewestReasoning() bool {
	if m == nil || !m.requestActivity.active {
		return false
	}
	active := strings.TrimSpace(m.requestActivity.sessionID)
	target := strings.TrimSpace(m.activeStreamTargetID())
	if active == "" || target == "" {
		return false
	}
	return active == target
}

func (m *Model) usesViewport() bool {
	switch m.mode {
	case uiModeNormal, uiModeCompose, uiModeSearch, uiModeNotes, uiModeAddNote:
		return true
	default:
		return false
	}
}

func (m *Model) viewportScrollbarView() string {
	if m.viewport.Height <= 0 {
		return ""
	}
	height := m.viewport.Height
	total := m.viewport.TotalLineCount()
	if total <= height || total <= 0 {
		return strings.Repeat(" \n", max(0, height-1)) + " "
	}
	trackHeight := height
	thumbHeight := int(math.Round(float64(trackHeight) * float64(height) / float64(total)))
	if thumbHeight < 1 {
		thumbHeight = 1
	}
	maxStart := trackHeight - thumbHeight
	top := 0
	denom := total - height
	if denom > 0 && maxStart > 0 {
		top = int(math.Round(float64(m.viewport.YOffset) / float64(denom) * float64(maxStart)))
	}
	if top < 0 {
		top = 0
	}
	if top > maxStart {
		top = maxStart
	}
	lines := make([]string, 0, trackHeight)
	for i := 0; i < trackHeight; i++ {
		if i >= top && i < top+thumbHeight {
			lines = append(lines, scrollbarThumbStyle.Render("┃"))
		} else {
			lines = append(lines, scrollbarTrackStyle.Render("│"))
		}
	}
	return strings.Join(lines, "\n")
}

func (m *Model) setContentText(text string) {
	m.contentRaw = text
	m.contentEsc = false
	m.contentBlocks = nil
	m.contentBlockSpans = nil
	m.clearMessageSelection()
	m.contentVersion++
	m.searchVersion = -1
	m.sectionVersion = -1
	m.renderViewport()
}

func (m *Model) renderViewport() {
	if m.viewport.Width <= 0 {
		return
	}
	renderWidth := m.viewport.Width
	if !m.appState.SidebarCollapsed && renderWidth > 1 {
		renderWidth--
	}
	selectedRenderIndex := m.selectedMessageRenderIndex()
	needsRender := m.renderedForWidth != renderWidth ||
		m.renderedForContent != m.contentVersion ||
		m.renderedForSelection != selectedRenderIndex
	if needsRender {
		var rendered string
		if m.contentBlocks != nil {
			var spans []renderedBlockSpan
			rendered, spans = renderChatBlocksWithSelection(m.contentBlocks, renderWidth, maxViewportLines, selectedRenderIndex)
			m.contentBlockSpans = spans
		} else {
			content := m.contentRaw
			if m.contentEsc {
				content = escapeMarkdown(content)
			}
			rendered = renderMarkdown(content, renderWidth)
			m.contentBlockSpans = nil
		}
		m.renderedText = rendered
		m.renderedLines = nil
		m.renderedPlain = nil
		if rendered != "" {
			lines := strings.Split(rendered, "\n")
			m.renderedLines = lines
			plain := make([]string, len(lines))
			for i, line := range lines {
				plain[i] = xansi.Strip(line)
			}
			m.renderedPlain = plain
		}
		m.renderedForWidth = renderWidth
		m.renderedForContent = m.contentVersion
		m.renderedForSelection = selectedRenderIndex
		m.renderVersion++
	}
	m.viewport.SetContent(m.renderedText)
	if m.scrollOnLoad && !m.loading {
		m.viewport.GotoBottom()
		m.scrollOnLoad = false
	}
	if m.follow {
		m.viewport.GotoBottom()
	}
}

func (m *Model) setLoadingContent() {
	m.setContentText(m.loader.View() + " Loading...")
}

func (m *Model) appendUserMessageLocal(provider, text string) int {
	if strings.EqualFold(provider, "claude") {
		return -1
	}
	if shouldStreamItems(provider) && m.itemStream != nil {
		m.itemStream.SetSnapshotBlocks(m.currentBlocks())
		headerIndex := m.itemStream.AppendUserMessage(text)
		if headerIndex >= 0 {
			_ = m.itemStream.MarkUserMessageSending(headerIndex)
		}
		blocks := m.itemStream.Blocks()
		m.applyBlocks(blocks)
		if key := m.selectedKey(); key != "" {
			m.transcriptCache[key] = blocks
		}
		return headerIndex
	}
	if m.codexStream == nil {
		return -1
	}
	m.codexStream.SetSnapshotBlocks(m.currentBlocks())
	headerIndex := m.codexStream.AppendUserMessage(text)
	if headerIndex >= 0 {
		_ = m.codexStream.MarkUserMessageSending(headerIndex)
	}
	blocks := m.codexStream.Blocks()
	m.applyBlocks(blocks)
	if key := m.selectedKey(); key != "" {
		m.transcriptCache[key] = blocks
	}
	return headerIndex
}

func (m *Model) nextSendToken() int {
	m.sendSeq++
	return m.sendSeq
}

func (m *Model) registerPendingSend(token int, sessionID, provider string) {
	key := m.selectedKey()
	m.pendingSends[token] = pendingSend{
		key:        key,
		sessionID:  sessionID,
		headerLine: -1,
		provider:   provider,
	}
}

func (m *Model) registerPendingSendHeader(token int, sessionID, provider string, headerIndex int) {
	entry, ok := m.pendingSends[token]
	if !ok {
		return
	}
	entry.sessionID = sessionID
	entry.key = m.selectedKey()
	entry.headerLine = headerIndex
	if provider != "" {
		entry.provider = provider
	}
	m.pendingSends[token] = entry
}

func (m *Model) clearPendingSend(token int) {
	entry, ok := m.pendingSends[token]
	if ok {
		delete(m.pendingSends, token)
		if entry.key == "" {
			return
		}
		if entry.key == m.selectedKey() {
			provider := entry.provider
			if provider == "" {
				provider = m.selectedSessionProvider()
			}
			if shouldStreamItems(provider) && m.itemStream != nil {
				if m.itemStream.MarkUserMessageSent(entry.headerLine) {
					blocks := m.itemStream.Blocks()
					m.applyBlocks(blocks)
					m.transcriptCache[entry.key] = blocks
					return
				}
			}
			if m.codexStream != nil {
				if m.codexStream.MarkUserMessageSent(entry.headerLine) {
					blocks := m.codexStream.Blocks()
					m.applyBlocks(blocks)
					m.transcriptCache[entry.key] = blocks
					return
				}
			}
		}
		if cached, ok := m.transcriptCache[entry.key]; ok {
			if entry.headerLine >= 0 && entry.headerLine < len(cached) {
				cached[entry.headerLine].Status = ChatStatusNone
				m.transcriptCache[entry.key] = cached
			}
		}
	}
}

func (m *Model) markPendingSendFailed(token int, err error) {
	entry, ok := m.pendingSends[token]
	if !ok {
		return
	}
	delete(m.pendingSends, token)
	if entry.key == "" {
		return
	}
	if entry.key == m.selectedKey() {
		provider := entry.provider
		if provider == "" {
			provider = m.selectedSessionProvider()
		}
		if shouldStreamItems(provider) && m.itemStream != nil {
			if m.itemStream.MarkUserMessageFailed(entry.headerLine) {
				blocks := m.itemStream.Blocks()
				m.applyBlocks(blocks)
				m.transcriptCache[entry.key] = blocks
				return
			}
		}
		if m.codexStream != nil {
			if m.codexStream.MarkUserMessageFailed(entry.headerLine) {
				blocks := m.codexStream.Blocks()
				m.applyBlocks(blocks)
				m.transcriptCache[entry.key] = blocks
				return
			}
		}
	}
	if cached, ok := m.transcriptCache[entry.key]; ok {
		if entry.headerLine >= 0 && entry.headerLine < len(cached) {
			cached[entry.headerLine].Status = ChatStatusFailed
			m.transcriptCache[entry.key] = cached
		}
	}
}

func (m *Model) handleViewportScroll(msg tea.KeyMsg) bool {
	if m.mode != uiModeNormal && m.mode != uiModeCompose && m.mode != uiModeNotes && m.mode != uiModeAddNote {
		return false
	}
	before := m.viewport.YOffset
	wasFollowing := m.follow
	switch msg.String() {
	case "up":
		m.pauseFollow(true)
		m.viewport.LineUp(1)
	case "down":
		m.pauseFollow(true)
		m.viewport.LineDown(1)
	case "pgup":
		m.pauseFollow(true)
		m.viewport.PageUp()
	case "pgdown":
		m.pauseFollow(true)
		m.viewport.PageDown()
	case "ctrl+f":
		m.pauseFollow(true)
		m.viewport.PageDown()
	case "ctrl+u":
		m.pauseFollow(true)
		m.viewport.HalfPageUp()
	case "ctrl+d":
		m.pauseFollow(true)
		m.viewport.HalfPageDown()
	case "home":
		m.pauseFollow(true)
		m.viewport.GotoTop()
	case "end":
		m.enableFollow(true)
		return true
	default:
		return false
	}
	if !wasFollowing && before < m.maxViewportYOffset() && m.isViewportAtBottom() {
		m.setFollowEnabled(true, true)
	}
	return true
}

func (m *Model) toggleVisibleReasoning() bool {
	if len(m.contentBlocks) == 0 || len(m.contentBlockSpans) == 0 {
		return false
	}
	start := m.viewport.YOffset
	end := start + m.viewport.Height - 1
	target := -1
	for i := len(m.contentBlockSpans) - 1; i >= 0; i-- {
		span := m.contentBlockSpans[i]
		if span.Role != ChatRoleReasoning {
			continue
		}
		if span.EndLine < start || span.StartLine > end {
			continue
		}
		target = span.BlockIndex
		break
	}
	if target < 0 {
		for i := len(m.contentBlockSpans) - 1; i >= 0; i-- {
			span := m.contentBlockSpans[i]
			if span.Role == ChatRoleReasoning {
				target = span.BlockIndex
				break
			}
		}
	}
	if target < 0 || target >= len(m.contentBlocks) {
		return false
	}
	return m.toggleReasoningByIndex(target)
}

func (m *Model) toggleReasoningByViewportLine(line int) bool {
	if line < 0 || len(m.contentBlocks) == 0 || len(m.contentBlockSpans) == 0 {
		return false
	}
	absolute := m.viewport.YOffset + line
	for _, span := range m.contentBlockSpans {
		if span.Role != ChatRoleReasoning {
			continue
		}
		if absolute >= span.StartLine && absolute <= span.EndLine {
			return m.toggleReasoningByIndex(span.BlockIndex)
		}
	}
	return false
}

func (m *Model) toggleReasoningByIndex(index int) bool {
	if index < 0 || index >= len(m.contentBlocks) {
		return false
	}
	block := m.contentBlocks[index]
	if block.Role != ChatRoleReasoning {
		return false
	}
	block.Collapsed = !block.Collapsed
	m.contentBlocks[index] = block
	if strings.TrimSpace(block.ID) != "" {
		if m.reasoningExpanded == nil {
			m.reasoningExpanded = map[string]bool{}
		}
		m.reasoningExpanded[block.ID] = !block.Collapsed
	}
	m.contentVersion++
	m.searchVersion = -1
	m.sectionVersion = -1
	m.renderViewport()
	m.persistCurrentBlocks()
	if block.Collapsed {
		m.setStatusMessage("reasoning collapsed")
	} else {
		m.setStatusMessage("reasoning expanded")
	}
	return true
}

func (m *Model) persistCurrentBlocks() {
	key := m.selectedKey()
	if key == "" || len(m.contentBlocks) == 0 {
		return
	}
	m.transcriptCache[key] = append([]ChatBlock(nil), m.contentBlocks...)
}

func (m *Model) maxViewportYOffset() int {
	total := m.viewport.TotalLineCount()
	maxOffset := total - m.viewport.Height
	if maxOffset < 0 {
		return 0
	}
	return maxOffset
}

func (m *Model) isViewportAtBottom() bool {
	return m.viewport.YOffset >= m.maxViewportYOffset()
}

func (m *Model) setFollowEnabled(enabled, announce bool) {
	if m.follow == enabled {
		return
	}
	m.follow = enabled
	if !announce {
		return
	}
	if enabled {
		m.setStatusMessage("follow: on")
	} else {
		m.setStatusMessage("follow: paused")
	}
}

func (m *Model) pauseFollow(announce bool) {
	m.setFollowEnabled(false, announce)
}

func (m *Model) enableFollow(announce bool) {
	m.viewport.GotoBottom()
	m.setFollowEnabled(true, announce)
}

func (m *Model) handleMouse(msg tea.MouseMsg) bool {
	if m.width <= 0 || m.height <= 0 {
		return false
	}
	layout := m.resolveMouseLayout()

	if m.reduceContextMenuLeftPressMouse(msg) {
		return true
	}
	if m.reduceContextMenuRightPressMouse(msg, layout) {
		return true
	}
	if m.reduceSidebarDragMouse(msg, layout) {
		return true
	}

	if msg.Action == tea.MouseActionPress {
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			return m.reduceMouseWheel(msg, layout, -1)
		case tea.MouseButtonWheelDown:
			return m.reduceMouseWheel(msg, layout, 1)
		case tea.MouseButtonLeft:
		default:
			return false
		}
	}
	if msg.Action != tea.MouseActionPress || msg.Button != tea.MouseButtonLeft {
		return false
	}
	if m.reduceMenuLeftPressMouse(msg) {
		return true
	}
	if m.reduceSidebarScrollbarLeftPressMouse(msg, layout) {
		return true
	}
	if m.reduceInputFocusLeftPressMouse(msg, layout) {
		return true
	}
	if m.reduceTranscriptApprovalButtonLeftPressMouse(msg, layout) {
		return true
	}
	if m.reduceTranscriptCopyLeftPressMouse(msg, layout) {
		return true
	}
	if m.reduceTranscriptReasoningButtonLeftPressMouse(msg, layout) {
		return true
	}
	if m.reduceTranscriptSelectLeftPressMouse(msg, layout) {
		return true
	}
	if m.reduceModePickersLeftPressMouse(msg, layout) {
		return true
	}
	if m.reduceSidebarSelectionLeftPressMouse(msg, layout) {
		return true
	}
	return false
}

func (m *Model) currentLines() []string {
	if m.contentBlocks != nil {
		if len(m.renderedPlain) > 0 {
			return m.renderedPlain
		}
		if m.renderedText != "" {
			return strings.Split(xansi.Strip(m.renderedText), "\n")
		}
		return nil
	}
	if strings.TrimSpace(m.contentRaw) == "" {
		return nil
	}
	return strings.Split(m.contentRaw, "\n")
}

func (m *Model) currentBlocks() []ChatBlock {
	if m.contentBlocks != nil {
		return m.contentBlocks
	}
	if m.itemStream != nil {
		if blocks := m.itemStream.Blocks(); len(blocks) > 0 {
			return blocks
		}
	}
	if m.codexStream != nil {
		if blocks := m.codexStream.Blocks(); len(blocks) > 0 {
			return blocks
		}
	}
	return nil
}

func (m *Model) setApprovalsForSession(sessionID string, requests []*ApprovalRequest) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return
	}
	normalized := normalizeApprovalRequests(requests)
	for i := range normalized {
		if normalized[i] == nil {
			continue
		}
		if strings.TrimSpace(normalized[i].SessionID) == "" {
			normalized[i].SessionID = sessionID
		}
	}
	m.sessionApprovals[sessionID] = normalized
	resolutions := m.sessionApprovalResolutions[sessionID]
	for _, req := range normalized {
		if req == nil {
			continue
		}
		updated, _ := removeApprovalResolution(resolutions, req.RequestID)
		resolutions = updated
	}
	m.sessionApprovalResolutions[sessionID] = resolutions
}

func (m *Model) upsertApprovalForSession(sessionID string, request *ApprovalRequest) bool {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return false
	}
	if request != nil && strings.TrimSpace(request.SessionID) == "" {
		copy := *request
		copy.SessionID = sessionID
		request = &copy
	}
	updated, changed := upsertApprovalRequest(m.sessionApprovals[sessionID], request)
	m.sessionApprovals[sessionID] = updated
	if request != nil {
		nextResolutions, removed := removeApprovalResolution(m.sessionApprovalResolutions[sessionID], request.RequestID)
		m.sessionApprovalResolutions[sessionID] = nextResolutions
		if removed {
			changed = true
		}
	}
	return changed
}

func (m *Model) removeApprovalForSession(sessionID string, requestID int) bool {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return false
	}
	updated, changed := removeApprovalRequest(m.sessionApprovals[sessionID], requestID)
	m.sessionApprovals[sessionID] = updated
	return changed
}

func (m *Model) upsertApprovalResolutionForSession(sessionID string, resolution *ApprovalResolution) bool {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return false
	}
	if resolution != nil && strings.TrimSpace(resolution.SessionID) == "" {
		copy := *resolution
		copy.SessionID = sessionID
		resolution = &copy
	}
	updated, changed := upsertApprovalResolution(m.sessionApprovalResolutions[sessionID], resolution)
	m.sessionApprovalResolutions[sessionID] = updated
	return changed
}

func (m *Model) sessionIDForApprovalRequest(requestID int) string {
	if requestID < 0 {
		return ""
	}
	for sessionID, requests := range m.sessionApprovals {
		for _, req := range requests {
			if req == nil || req.RequestID != requestID {
				continue
			}
			if strings.TrimSpace(req.SessionID) != "" {
				return strings.TrimSpace(req.SessionID)
			}
			return strings.TrimSpace(sessionID)
		}
	}
	return ""
}

func (m *Model) activeContentSessionID() string {
	if id := strings.TrimSpace(m.composeSessionID()); id != "" {
		return id
	}
	if id := strings.TrimSpace(m.selectedSessionID()); id != "" {
		return id
	}
	if id := sessionIDFromSidebarKey(m.pendingSessionKey); id != "" {
		return id
	}
	if m.pendingApproval != nil {
		if id := strings.TrimSpace(m.pendingApproval.SessionID); id != "" {
			return id
		}
	}
	return ""
}

func (m *Model) removeApprovalResolutionForSession(sessionID string, requestID int) bool {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return false
	}
	updated, changed := removeApprovalResolution(m.sessionApprovalResolutions[sessionID], requestID)
	m.sessionApprovalResolutions[sessionID] = updated
	return changed
}

func (m *Model) refreshVisibleApprovalBlocks(sessionID string) {
	if strings.TrimSpace(sessionID) == "" || sessionID != m.selectedSessionID() {
		return
	}
	base := m.currentBlocks()
	requests := m.sessionApprovals[sessionID]
	if len(base) == 0 && len(requests) == 0 {
		return
	}
	blocks := mergeApprovalBlocks(base, requests, m.sessionApprovalResolutions[sessionID])
	provider := m.providerForSessionID(sessionID)
	if provider == "codex" && m.codexStream != nil {
		m.codexStream.SetSnapshotBlocks(blocks)
		blocks = m.codexStream.Blocks()
	} else if shouldStreamItems(provider) && m.itemStream != nil {
		m.itemStream.SetSnapshotBlocks(blocks)
		blocks = m.itemStream.Blocks()
	}
	m.applyBlocks(blocks)
	if key := m.selectedKey(); key != "" {
		m.transcriptCache[key] = append([]ChatBlock(nil), blocks...)
	}
}

func (m *Model) toggleSidebar() {
	m.appState.SidebarCollapsed = !m.appState.SidebarCollapsed
	m.hasAppState = true
	m.updateDelegate()
	m.resize(m.width, m.height)
}

func (m *Model) applyAppState(state *types.AppState) {
	if state == nil {
		return
	}
	m.appState = *state
	m.composeHistory = importComposeHistory(state.ComposeHistory)
	m.hasAppState = true
	if m.menu != nil {
		if state.ActiveWorkspaceGroupIDs == nil {
			m.menu.SetSelectedGroupIDs([]string{"ungrouped"})
			m.appState.ActiveWorkspaceGroupIDs = []string{"ungrouped"}
		} else {
			m.menu.SetSelectedGroupIDs(state.ActiveWorkspaceGroupIDs)
		}
	}
	m.updateDelegate()
}

func (m *Model) updateDelegate() {
	if m.sidebar != nil {
		m.sidebar.SetActive(m.appState.ActiveWorkspaceID, m.appState.ActiveWorktreeID)
	}
}

func (m *Model) saveAppStateCmd() tea.Cmd {
	if m.stateAPI == nil || !m.hasAppState {
		return nil
	}
	m.syncAppStateComposeHistory()
	state := m.appState
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		defer cancel()
		updated, err := m.stateAPI.UpdateAppState(ctx, &state)
		return appStateSavedMsg{state: updated, err: err}
	}
}

func (m *Model) enterAddWorkspace() {
	m.mode = uiModeAddWorkspace
	if m.addWorkspace != nil {
		m.addWorkspace.Enter()
	}
	m.setStatusMessage("add workspace: enter path")
}

func (m *Model) exitAddWorkspace(status string) {
	m.mode = uiModeNormal
	if m.addWorkspace != nil {
		m.addWorkspace.Exit()
	}
	if status != "" {
		m.setStatusMessage(status)
	}
}

func (m *Model) enterAddWorkspaceGroup() {
	m.mode = uiModeAddWorkspaceGroup
	if m.groupInput != nil {
		m.groupInput.SetPlaceholder("group name")
		m.groupInput.SetValue("")
		m.groupInput.Focus()
	}
	if m.input != nil {
		m.input.FocusChatInput()
	}
	m.setStatusMessage("add workspace group")
}

func (m *Model) exitAddWorkspaceGroup(status string) {
	m.mode = uiModeNormal
	if m.groupInput != nil {
		m.groupInput.SetValue("")
		m.groupInput.Blur()
	}
	if m.input != nil {
		m.input.FocusSidebar()
	}
	if status != "" {
		m.setStatusMessage(status)
	}
}

func (m *Model) enterWorkspacePicker(mode uiMode) {
	m.mode = mode
	if m.workspacePicker != nil {
		options := make([]selectOption, 0, len(m.workspaces))
		for _, ws := range m.workspaces {
			if ws == nil {
				continue
			}
			options = append(options, selectOption{id: ws.ID, label: ws.Name})
		}
		m.workspacePicker.SetOptions(options)
	}
	m.setStatusMessage("select workspace")
}

func (m *Model) enterGroupPicker() {
	m.mode = uiModePickWorkspaceGroupRename
	if m.groupSelectPicker != nil {
		options := make([]selectOption, 0, len(m.groups))
		for _, group := range m.groups {
			if group == nil {
				continue
			}
			if group.ID == "ungrouped" {
				continue
			}
			options = append(options, selectOption{id: group.ID, label: group.Name})
		}
		m.groupSelectPicker.SetOptions(options)
	}
	m.setStatusMessage("select group")
}

func (m *Model) enterGroupAssignPicker() {
	m.mode = uiModePickWorkspaceGroupAssign
	if m.groupSelectPicker != nil {
		options := make([]selectOption, 0, len(m.groups))
		for _, group := range m.groups {
			if group == nil {
				continue
			}
			if group.ID == "ungrouped" {
				continue
			}
			options = append(options, selectOption{id: group.ID, label: group.Name})
		}
		m.groupSelectPicker.SetOptions(options)
	}
	m.setStatusMessage("select group")
}

func (m *Model) enterGroupDeletePicker() {
	m.mode = uiModePickWorkspaceGroupDelete
	if m.groupSelectPicker != nil {
		options := make([]selectOption, 0, len(m.groups))
		for _, group := range m.groups {
			if group == nil {
				continue
			}
			if group.ID == "ungrouped" {
				continue
			}
			options = append(options, selectOption{id: group.ID, label: group.Name})
		}
		m.groupSelectPicker.SetOptions(options)
	}
	m.setStatusMessage("select group")
}

func (m *Model) exitWorkspacePicker(status string) {
	m.mode = uiModeNormal
	if status != "" {
		m.setStatusMessage(status)
	}
}

func (m *Model) enterEditWorkspaceGroups(id string) {
	m.mode = uiModeEditWorkspaceGroups
	m.editWorkspaceID = id
	selected := map[string]bool{}
	if ws := m.workspaceByID(id); ws != nil {
		for _, gid := range ws.GroupIDs {
			selected[gid] = true
		}
	}
	if m.groupPicker != nil {
		m.groupPicker.SetGroups(m.groups, selected)
	}
	if m.input != nil {
		m.input.FocusSidebar()
	}
	m.setStatusMessage("edit workspace groups")
}

func (m *Model) exitEditWorkspaceGroups(status string) {
	m.mode = uiModeNormal
	m.editWorkspaceID = ""
	if status != "" {
		m.setStatusMessage(status)
	}
}

func (m *Model) enterRenameWorkspaceGroup(id string) {
	m.mode = uiModeRenameWorkspaceGroup
	m.renameGroupID = id
	if m.groupInput != nil {
		name := ""
		for _, group := range m.groups {
			if group != nil && group.ID == id {
				name = group.Name
				break
			}
		}
		m.groupInput.SetPlaceholder("group name")
		m.groupInput.SetValue(name)
		m.groupInput.Focus()
	}
	if m.input != nil {
		m.input.FocusChatInput()
	}
	m.setStatusMessage("rename workspace group")
}

func (m *Model) exitRenameWorkspaceGroup(status string) {
	m.mode = uiModeNormal
	if m.groupInput != nil {
		m.groupInput.SetValue("")
		m.groupInput.Blur()
	}
	m.renameGroupID = ""
	if m.input != nil {
		m.input.FocusSidebar()
	}
	if status != "" {
		m.setStatusMessage(status)
	}
}

func (m *Model) enterAssignGroupWorkspaces(groupID string) {
	m.mode = uiModeAssignGroupWorkspaces
	m.assignGroupID = groupID
	if m.workspaceMulti != nil {
		options := make([]multiSelectOption, 0, len(m.workspaces))
		for _, ws := range m.workspaces {
			if ws == nil {
				continue
			}
			selected := false
			for _, gid := range ws.GroupIDs {
				if gid == groupID {
					selected = true
					break
				}
			}
			options = append(options, multiSelectOption{id: ws.ID, label: ws.Name, selected: selected})
		}
		m.workspaceMulti.SetOptions(options)
	}
	m.setStatusMessage("assign workspaces")
}

func (m *Model) exitAssignGroupWorkspaces(status string) {
	m.mode = uiModeNormal
	m.assignGroupID = ""
	if status != "" {
		m.setStatusMessage(status)
	}
}

func (m *Model) enterAddWorktree(workspaceID string) {
	m.mode = uiModeAddWorktree
	workspacePath := ""
	if ws := m.workspaceByID(workspaceID); ws != nil {
		workspacePath = ws.RepoPath
	}
	if m.addWorktree != nil {
		m.addWorktree.Enter(workspaceID, workspacePath)
	}
	m.setStatusMessage("add worktree: new or existing")
}

func (m *Model) exitAddWorktree(status string) {
	m.mode = uiModeNormal
	if m.addWorktree != nil {
		m.addWorktree.Exit()
	}
	if status != "" {
		m.setStatusMessage(status)
	}
}

func (m *Model) enterRenameWorkspace(id string) {
	m.mode = uiModeRenameWorkspace
	m.renameWorkspaceID = id
	if m.renameInput != nil {
		name := ""
		if ws := m.workspaceByID(id); ws != nil {
			name = ws.Name
		}
		m.renameInput.SetValue(name)
		m.renameInput.Focus()
	}
	if m.input != nil {
		m.input.FocusChatInput()
	}
	m.setStatusMessage("rename workspace")
}

func (m *Model) exitRenameWorkspace(status string) {
	m.mode = uiModeNormal
	if m.renameInput != nil {
		m.renameInput.SetValue("")
		m.renameInput.Blur()
	}
	m.renameWorkspaceID = ""
	if m.input != nil {
		m.input.FocusSidebar()
	}
	if status != "" {
		m.setStatusMessage(status)
	}
}

func (m *Model) enterCompose(sessionID string) {
	m.mode = uiModeCompose
	label := m.selectedSessionLabel()
	if m.compose != nil {
		m.compose.Enter(sessionID, label)
	}
	m.resetComposeHistoryCursor()
	if m.chatInput != nil {
		m.chatInput.SetPlaceholder("message")
		m.chatInput.Focus()
	}
	if m.input != nil {
		m.input.FocusChatInput()
	}
	m.setStatusMessage("compose message")
	m.resize(m.width, m.height)
}

func (m *Model) exitCompose(status string) {
	m.mode = uiModeNormal
	if m.compose != nil {
		m.compose.Exit()
	}
	if m.chatInput != nil {
		m.chatInput.Blur()
		m.chatInput.Clear()
	}
	m.newSession = nil
	if m.input != nil {
		m.input.FocusSidebar()
	}
	if status != "" {
		m.setStatusMessage(status)
	}
	m.resize(m.width, m.height)
}

func (m *Model) enterProviderPick() {
	m.mode = uiModePickProvider
	if m.providerPicker != nil {
		m.providerPicker.Enter("")
	}
	if m.chatInput != nil {
		m.chatInput.Blur()
	}
	if m.input != nil {
		m.input.FocusSidebar()
	}
	m.setStatusMessage("choose provider")
	m.resize(m.width, m.height)
}

func (m *Model) exitProviderPick(status string) {
	m.mode = uiModeNormal
	if m.providerPicker != nil {
		m.providerPicker.Enter("")
	}
	m.newSession = nil
	if m.input != nil {
		m.input.FocusSidebar()
	}
	if status != "" {
		m.setStatusMessage(status)
	}
	m.resize(m.width, m.height)
}

func (m *Model) selectProvider() tea.Cmd {
	if m.providerPicker == nil {
		return nil
	}
	provider := m.providerPicker.Selected()
	if provider == "" {
		m.setValidationStatus("provider is required")
		return nil
	}
	return m.applyProviderSelection(provider)
}

func (m *Model) applyProviderSelection(provider string) tea.Cmd {
	if m.newSession == nil {
		return nil
	}
	m.newSession.provider = provider
	m.mode = uiModeCompose
	if m.compose != nil {
		m.compose.Enter("", "New session")
	}
	if m.chatInput != nil {
		m.chatInput.SetPlaceholder("new session message")
		m.chatInput.Focus()
	}
	if m.input != nil {
		m.input.FocusChatInput()
	}
	m.setStatusMessage("provider set: " + provider)
	m.resize(m.width, m.height)
	return nil
}

func (m *Model) enterSearch() {
	m.mode = uiModeSearch
	if m.searchInput != nil {
		m.searchInput.SetPlaceholder("search")
		m.searchInput.SetValue(m.searchQuery)
		m.searchInput.Focus()
	}
	m.setStatusMessage("search")
	m.resize(m.width, m.height)
}

func (m *Model) exitSearch(status string) {
	m.mode = uiModeNormal
	if m.searchInput != nil {
		m.searchInput.Blur()
		m.searchInput.Clear()
	}
	if m.input != nil {
		m.input.FocusSidebar()
	}
	if status != "" {
		m.setStatusMessage(status)
	}
	m.resize(m.width, m.height)
}

func (m *Model) applySearch(query string) {
	query = strings.TrimSpace(query)
	if query == "" {
		m.searchQuery = ""
		m.searchMatches = nil
		m.searchIndex = -1
		m.setStatusMessage("search cleared")
		return
	}
	m.searchQuery = query
	m.searchMatches = m.findSearchMatches(query)
	m.searchVersion = m.renderVersion
	if len(m.searchMatches) == 0 {
		m.searchIndex = -1
		m.setStatusMessage("no matches")
		return
	}
	m.searchIndex = selectSearchIndex(m.searchMatches, m.viewport.YOffset, 1)
	if m.searchIndex < 0 {
		m.searchIndex = 0
	}
	m.jumpToLine(m.searchMatches[m.searchIndex])
	m.setStatusMessage(fmt.Sprintf("match %d/%d", m.searchIndex+1, len(m.searchMatches)))
}

func (m *Model) moveSearch(delta int) {
	if m.searchQuery == "" {
		m.setStatusMessage("no search")
		return
	}
	if m.searchVersion != m.renderVersion {
		m.searchMatches = m.findSearchMatches(m.searchQuery)
		m.searchVersion = m.renderVersion
		m.searchIndex = -1
	}
	if len(m.searchMatches) == 0 {
		m.searchIndex = -1
		m.setStatusMessage("no matches")
		return
	}
	if m.searchIndex < 0 {
		m.searchIndex = selectSearchIndex(m.searchMatches, m.viewport.YOffset, delta)
		if m.searchIndex < 0 {
			m.searchIndex = 0
		}
	} else {
		m.searchIndex = (m.searchIndex + delta + len(m.searchMatches)) % len(m.searchMatches)
	}
	m.jumpToLine(m.searchMatches[m.searchIndex])
	m.setStatusMessage(fmt.Sprintf("match %d/%d", m.searchIndex+1, len(m.searchMatches)))
}

func (m *Model) approvePending(decision string) tea.Cmd {
	if m.pendingApproval == nil {
		return nil
	}
	return m.approveRequestForSession(m.pendingApproval.SessionID, decision, m.pendingApproval.RequestID)
}

func (m *Model) approveRequest(decision string, requestID int) tea.Cmd {
	return m.approveRequestForSession("", decision, requestID)
}

func (m *Model) approveRequestForSession(sessionID string, decision string, requestID int) tea.Cmd {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		sessionID = m.activeContentSessionID()
	}
	if sessionID == "" {
		sessionID = m.sessionIDForApprovalRequest(requestID)
	}
	if sessionID == "" {
		m.setValidationStatus("select a session to approve")
		return nil
	}
	if requestID < 0 {
		m.setValidationStatus("invalid approval request")
		return nil
	}
	m.setStatusMessage("sending approval")
	return approveSessionCmd(m.sessionAPI, sessionID, requestID, decision)
}

func selectApprovalRequest(records []*types.Approval) *ApprovalRequest {
	return latestApprovalRequest(approvalRequestsFromRecords(records))
}

func (m *Model) findSearchMatches(query string) []int {
	lines := m.renderedPlain
	if len(lines) == 0 && m.renderedText != "" {
		lines = strings.Split(xansi.Strip(m.renderedText), "\n")
	}
	if len(lines) == 0 {
		return nil
	}
	q := strings.ToLower(query)
	matches := make([]int, 0, 8)
	for i, line := range lines {
		if strings.Contains(strings.ToLower(line), q) {
			matches = append(matches, i)
		}
	}
	return matches
}

func selectSearchIndex(matches []int, offset int, delta int) int {
	if len(matches) == 0 {
		return -1
	}
	if delta < 0 {
		for i := len(matches) - 1; i >= 0; i-- {
			if matches[i] < offset {
				return i
			}
		}
		return len(matches) - 1
	}
	for i, line := range matches {
		if line >= offset {
			return i
		}
	}
	return 0
}

func (m *Model) jumpToLine(offset int) {
	m.viewport.SetYOffset(offset)
	if m.follow {
		m.pauseFollow(false)
	}
}

func (m *Model) jumpSection(delta int) {
	offsets := m.sectionOffsetsCached()
	if len(offsets) == 0 {
		m.setValidationStatus("no sections")
		return
	}
	current := m.viewport.YOffset
	index := -1
	if delta < 0 {
		for i := len(offsets) - 1; i >= 0; i-- {
			if offsets[i] < current {
				index = i
				break
			}
		}
		if index < 0 {
			index = 0
		}
	} else {
		for i, off := range offsets {
			if off > current {
				index = i
				break
			}
		}
		if index < 0 {
			index = len(offsets) - 1
		}
	}
	m.jumpToLine(offsets[index])
}

func (m *Model) sectionOffsetsCached() []int {
	if m.sectionVersion == m.renderVersion {
		return m.sectionOffsets
	}
	lines := m.currentLines()
	if len(lines) == 0 {
		m.sectionOffsets = nil
		m.sectionVersion = m.renderVersion
		return nil
	}
	headings := make([]string, 0, 8)
	for _, line := range lines {
		if strings.HasPrefix(line, "### ") {
			headings = append(headings, strings.TrimSpace(strings.TrimPrefix(line, "### ")))
		}
	}
	if len(headings) == 0 {
		m.sectionOffsets = nil
		m.sectionVersion = m.renderVersion
		return nil
	}
	rendered := m.renderedPlain
	if len(rendered) == 0 && m.renderedText != "" {
		rendered = strings.Split(xansi.Strip(m.renderedText), "\n")
	}
	offsets := make([]int, 0, len(headings))
	start := 0
	for _, heading := range headings {
		found := -1
		needle := strings.ToLower(strings.TrimSpace(heading))
		for i := start; i < len(rendered); i++ {
			candidate := strings.ToLower(strings.TrimSpace(rendered[i]))
			if candidate == needle {
				found = i
				break
			}
		}
		if found < 0 {
			for i := start; i < len(rendered); i++ {
				candidate := strings.ToLower(strings.TrimSpace(rendered[i]))
				if strings.Contains(candidate, needle) {
					found = i
					break
				}
			}
		}
		if found < 0 {
			continue
		}
		offsets = append(offsets, found)
		start = found + 1
	}
	m.sectionOffsets = offsets
	m.sectionVersion = m.renderVersion
	return offsets
}

func (m *Model) enterNewSession() bool {
	item := m.selectedItem()
	workspaceID := ""
	worktreeID := ""
	if item != nil {
		workspaceID = item.workspaceID()
		if item.worktree != nil {
			worktreeID = item.worktree.ID
		} else if item.meta != nil {
			worktreeID = item.meta.WorktreeID
		}
	}
	if workspaceID == "" {
		workspaceID = m.appState.ActiveWorkspaceID
		worktreeID = m.appState.ActiveWorktreeID
	}
	if workspaceID == "" {
		m.setValidationStatus("select a workspace or worktree")
		return false
	}
	m.newSession = &newSessionTarget{
		workspaceID: workspaceID,
		worktreeID:  worktreeID,
	}
	m.enterProviderPick()
	return true
}

func (m *Model) selectedSessionLabel() string {
	item := m.selectedItem()
	if item == nil || item.session == nil {
		return ""
	}
	return sessionTitle(item.session, item.meta)
}

func (m *Model) selectedSessionProvider() string {
	item := m.selectedItem()
	if item == nil || item.session == nil {
		return ""
	}
	return item.session.Provider
}

func (m *Model) providerForSessionID(sessionID string) string {
	if strings.TrimSpace(sessionID) == "" {
		return m.selectedSessionProvider()
	}
	for _, session := range m.sessions {
		if session != nil && session.ID == sessionID {
			return session.Provider
		}
	}
	return m.selectedSessionProvider()
}

func shouldStreamItems(provider string) bool {
	return providers.CapabilitiesFor(provider).UsesItems
}

func (m *Model) composeSessionID() string {
	if m.compose == nil {
		return ""
	}
	return m.compose.sessionID
}

func (m *Model) composeHistorySessionID() string {
	id := m.composeSessionID()
	if strings.TrimSpace(id) != "" {
		return id
	}
	return m.selectedSessionID()
}

func (m *Model) historyStateFor(sessionID string) *composeHistoryState {
	if strings.TrimSpace(sessionID) == "" {
		return nil
	}
	if m.composeHistory == nil {
		m.composeHistory = map[string]*composeHistoryState{}
	}
	state, ok := m.composeHistory[sessionID]
	if !ok || state == nil {
		state = &composeHistoryState{cursor: -1}
		m.composeHistory[sessionID] = state
	}
	return state
}

func (m *Model) recordComposeHistory(sessionID, text string) {
	sessionID = strings.TrimSpace(sessionID)
	text = strings.TrimSpace(text)
	if sessionID == "" || text == "" {
		return
	}
	state := m.historyStateFor(sessionID)
	if state == nil {
		return
	}
	state.entries = append(state.entries, text)
	if len(state.entries) > composeHistoryMaxEntries {
		state.entries = state.entries[len(state.entries)-composeHistoryMaxEntries:]
	}
	state.cursor = -1
	state.draft = ""
	m.hasAppState = true
	m.syncAppStateComposeHistory()
}

func (m *Model) composeHistoryNavigate(direction int, current string) (string, bool) {
	sessionID := m.composeHistorySessionID()
	if strings.TrimSpace(sessionID) == "" {
		return "", false
	}
	state := m.historyStateFor(sessionID)
	if state == nil || len(state.entries) == 0 {
		return "", false
	}
	if direction < 0 {
		if state.cursor == -1 {
			state.draft = current
			state.cursor = len(state.entries) - 1
			return state.entries[state.cursor], true
		}
		if state.cursor > 0 {
			state.cursor--
			return state.entries[state.cursor], true
		}
		return state.entries[0], true
	}
	if direction > 0 {
		if state.cursor == -1 {
			return "", false
		}
		if state.cursor < len(state.entries)-1 {
			state.cursor++
			return state.entries[state.cursor], true
		}
		state.cursor = -1
		state.draft = ""
		return "", true
	}
	return "", false
}

func (m *Model) resetComposeHistoryCursor() {
	sessionID := m.composeHistorySessionID()
	if strings.TrimSpace(sessionID) == "" {
		return
	}
	state := m.historyStateFor(sessionID)
	if state == nil {
		return
	}
	if state.cursor != -1 {
		state.cursor = -1
		state.draft = ""
	}
}

func (m *Model) syncAppStateComposeHistory() {
	if m == nil {
		return
	}
	if m.appState.ComposeHistory == nil {
		m.appState.ComposeHistory = map[string][]string{}
	}
	m.appState.ComposeHistory = exportComposeHistory(m.composeHistory)
}

func importComposeHistory(raw map[string][]string) map[string]*composeHistoryState {
	out := map[string]*composeHistoryState{}
	if len(raw) == 0 {
		return out
	}
	keys := make([]string, 0, len(raw))
	for sessionID := range raw {
		id := strings.TrimSpace(sessionID)
		if id == "" {
			continue
		}
		keys = append(keys, id)
	}
	sort.Strings(keys)
	if len(keys) > composeHistoryMaxSessions {
		keys = keys[len(keys)-composeHistoryMaxSessions:]
	}
	for _, id := range keys {
		entries := raw[id]
		if len(entries) == 0 {
			continue
		}
		start := 0
		if len(entries) > composeHistoryMaxEntries {
			start = len(entries) - composeHistoryMaxEntries
		}
		cleaned := make([]string, 0, len(entries)-start)
		for _, entry := range entries[start:] {
			text := strings.TrimSpace(entry)
			if text == "" {
				continue
			}
			cleaned = append(cleaned, text)
		}
		if len(cleaned) == 0 {
			continue
		}
		out[id] = &composeHistoryState{
			entries: cleaned,
			cursor:  -1,
		}
	}
	return out
}

func exportComposeHistory(raw map[string]*composeHistoryState) map[string][]string {
	out := map[string][]string{}
	if len(raw) == 0 {
		return out
	}
	keys := make([]string, 0, len(raw))
	for sessionID := range raw {
		id := strings.TrimSpace(sessionID)
		if id == "" {
			continue
		}
		keys = append(keys, id)
	}
	sort.Strings(keys)
	if len(keys) > composeHistoryMaxSessions {
		keys = keys[len(keys)-composeHistoryMaxSessions:]
	}
	for _, id := range keys {
		state := raw[id]
		if state == nil || len(state.entries) == 0 {
			continue
		}
		start := 0
		if len(state.entries) > composeHistoryMaxEntries {
			start = len(state.entries) - composeHistoryMaxEntries
		}
		cleaned := make([]string, 0, len(state.entries)-start)
		for _, entry := range state.entries[start:] {
			text := strings.TrimSpace(entry)
			if text == "" {
				continue
			}
			cleaned = append(cleaned, text)
		}
		if len(cleaned) == 0 {
			continue
		}
		out[id] = cleaned
	}
	return out
}

func renderStatusLine(width int, help, status string) string {
	if width <= 0 {
		return help + " " + status
	}
	helpWidth := lipgloss.Width(help)
	statusWidth := lipgloss.Width(status)
	padding := width - helpWidth - statusWidth
	if padding < statusLinePadding {
		padding = statusLinePadding
	}
	return help + strings.Repeat(" ", padding) + status
}

func renderInputDivider(width int, scrollable bool) string {
	if width <= 0 {
		return ""
	}
	indicator := ""
	if scrollable {
		indicator = " ^v"
	}
	lineWidth := width
	if indicator != "" && width > len(indicator) {
		lineWidth = width - len(indicator)
	}
	line := strings.Repeat("─", max(1, lineWidth)) + indicator
	if lipgloss.Width(line) > width {
		line = strings.Repeat("─", width)
	}
	return dividerStyle.Render(line)
}

func hasAgentReplyBlocks(blocks []ChatBlock) bool {
	if len(blocks) == 0 {
		return false
	}
	for _, block := range blocks {
		if block.Role == ChatRoleAgent {
			return true
		}
	}
	return false
}

func countAgentRepliesBlocks(blocks []ChatBlock) int {
	if len(blocks) == 0 {
		return 0
	}
	count := 0
	for _, block := range blocks {
		if block.Role == ChatRoleAgent {
			count++
		}
	}
	return count
}

func clamp(value, minValue, maxValue int) int {
	if value < minValue {
		return minValue
	}
	if value > maxValue {
		return maxValue
	}
	return value
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
