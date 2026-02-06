package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	xansi "github.com/charmbracelet/x/ansi"

	"control/internal/client"
	"control/internal/types"
)

const (
	defaultTailLines  = 200
	maxViewportLines  = 2000
	maxEventsPerTick  = 64
	tickInterval      = 100 * time.Millisecond
	selectionDebounce = 500 * time.Millisecond
	minListWidth      = 24
	maxListWidth      = 40
	minViewportWidth  = 20
	minContentHeight  = 6
	statusLinePadding = 1
)

type uiMode int

const (
	uiModeNormal uiMode = iota
	uiModeAddWorkspace
	uiModeAddWorktree
	uiModePickProvider
	uiModeCompose
	uiModeSearch
)

type Model struct {
	workspaceAPI      WorkspaceAPI
	sessionAPI        SessionAPI
	stateAPI          StateAPI
	sidebar           *SidebarController
	viewport          viewport.Model
	mode              uiMode
	addWorkspace      *AddWorkspaceController
	addWorktree       *AddWorktreeController
	providerPicker    *ProviderPicker
	compose           *ComposeController
	chatInput         *ChatInput
	searchInput       *ChatInput
	status            string
	width             int
	height            int
	follow            bool
	workspaces        []*types.Workspace
	worktrees         map[string][]*types.Worktree
	sessions          []*types.Session
	sessionMeta       map[string]*types.SessionMeta
	appState          types.AppState
	hasAppState       bool
	stream            *StreamController
	codexStream       *CodexStreamController
	itemStream        *ItemStreamController
	input             *InputController
	chat              *SessionChatController
	pendingApproval   *ApprovalRequest
	contentRaw        string
	contentEsc        bool
	renderedText      string
	renderedLines     []string
	renderedPlain     []string
	contentVersion    int
	renderVersion     int
	searchQuery       string
	searchMatches     []int
	searchIndex       int
	searchVersion     int
	sectionOffsets    []int
	sectionVersion    int
	transcriptCache   map[string][]string
	pendingSessionKey string
	loading           bool
	loadingKey        string
	loader            spinner.Model
	pendingMouseCmd   tea.Cmd
	hotkeys           *HotkeyRenderer
	newSession        *newSessionTarget
	pendingSelectID   string
	selectSeq         int
	sendSeq           int
	pendingSends      map[int]pendingSend
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
		workspaceAPI:    api,
		sessionAPI:      api,
		stateAPI:        api,
		sidebar:         NewSidebarController(),
		viewport:        vp,
		stream:          stream,
		codexStream:     codexStream,
		itemStream:      itemStream,
		input:           NewInputController(),
		chat:            NewSessionChatController(api, codexStream),
		mode:            uiModeNormal,
		addWorkspace:    NewAddWorkspaceController(minViewportWidth),
		addWorktree:     NewAddWorktreeController(minViewportWidth),
		providerPicker:  NewProviderPicker(minViewportWidth, minContentHeight-1),
		compose:         NewComposeController(minViewportWidth),
		chatInput:       NewChatInput(minViewportWidth, DefaultChatInputConfig()),
		searchInput:     NewChatInput(minViewportWidth, ChatInputConfig{Height: 1}),
		status:          "",
		follow:          true,
		worktrees:       map[string][]*types.Worktree{},
		sessionMeta:     map[string]*types.SessionMeta{},
		contentRaw:      "No sessions.",
		contentEsc:      false,
		searchIndex:     -1,
		searchVersion:   -1,
		sectionVersion:  -1,
		transcriptCache: map[string][]string{},
		loader:          loader,
		hotkeys:         hotkeyRenderer,
		pendingSends:    map[int]pendingSend{},
	}
}

func Run(client *client.Client) error {
	model := NewModel(client)
	p := tea.NewProgram(&model, tea.WithAltScreen(), tea.WithMouseCellMotion())
	_, err := p.Run()
	return err
}

func (m *Model) Init() tea.Cmd {
	return tea.Batch(fetchAppStateCmd(m.stateAPI), fetchWorkspacesCmd(m.workspaceAPI), fetchSessionsWithMetaCmd(m.sessionAPI), tickCmd())
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case createWorkspaceMsg:
		if msg.err != nil {
			m.exitAddWorkspace("add workspace error: " + msg.err.Error())
			return m, nil
		}
		if msg.workspace != nil {
			m.appState.ActiveWorkspaceID = msg.workspace.ID
			m.hasAppState = true
			m.updateDelegate()
			m.exitAddWorkspace("workspace added: " + msg.workspace.Name)
			return m, tea.Batch(fetchWorkspacesCmd(m.workspaceAPI), fetchSessionsWithMetaCmd(m.sessionAPI), m.saveAppStateCmd())
		}
		m.exitAddWorkspace("workspace added")
		return m, nil
	case availableWorktreesMsg:
		if msg.err != nil {
			m.status = "worktrees error: " + msg.err.Error()
			return m, nil
		}
		if m.addWorktree != nil {
			count := m.addWorktree.SetAvailable(msg.worktrees, m.worktrees[msg.workspaceID], msg.workspacePath)
			m.status = fmt.Sprintf("%d worktrees found", count)
		}
		return m, nil
	case createWorktreeMsg:
		if msg.err != nil {
			m.status = "create worktree error: " + msg.err.Error()
			return m, nil
		}
		m.exitAddWorktree("worktree added")
		cmds := []tea.Cmd{fetchSessionsWithMetaCmd(m.sessionAPI)}
		if msg.workspaceID != "" {
			cmds = append(cmds, fetchWorktreesCmd(m.workspaceAPI, msg.workspaceID))
		}
		return m, tea.Batch(cmds...)
	case addWorktreeMsg:
		if msg.err != nil {
			m.status = "add worktree error: " + msg.err.Error()
			return m, nil
		}
		m.exitAddWorktree("worktree added")
		cmds := []tea.Cmd{fetchSessionsWithMetaCmd(m.sessionAPI)}
		if msg.workspaceID != "" {
			cmds = append(cmds, fetchWorktreesCmd(m.workspaceAPI, msg.workspaceID))
		}
		return m, tea.Batch(cmds...)
	}

	if m.mode == uiModeAddWorkspace {
		switch msg := msg.(type) {
		case tickMsg:
			m.consumeStreamTick()
			return m, tickCmd()
		case streamMsg:
			if msg.err != nil {
				m.status = "stream error: " + msg.err.Error()
				return m, nil
			}
			if msg.id != m.selectedSessionID() {
				if msg.cancel != nil {
					msg.cancel()
				}
				return m, nil
			}
			if m.stream != nil {
				m.stream.SetStream(msg.ch, msg.cancel)
			}
			m.status = "streaming"
			return m, nil
		}
		if m.addWorkspace == nil {
			return m, nil
		}
		_, cmd := m.addWorkspace.Update(msg, m)
		return m, cmd
	}
	if m.mode == uiModeAddWorktree {
		switch msg := msg.(type) {
		case tickMsg:
			m.consumeStreamTick()
			return m, tickCmd()
		case streamMsg:
			if msg.err != nil {
				m.status = "stream error: " + msg.err.Error()
				return m, nil
			}
			if msg.id != m.selectedSessionID() {
				if msg.cancel != nil {
					msg.cancel()
				}
				return m, nil
			}
			if m.stream != nil {
				m.stream.SetStream(msg.ch, msg.cancel)
			}
			m.status = "streaming"
			return m, nil
		case tea.MouseMsg:
			if m.handleMouse(msg) {
				return m, nil
			}
		}
		if m.addWorktree == nil {
			return m, nil
		}
		_, cmd := m.addWorktree.Update(msg, m)
		return m, cmd
	}
	if m.mode == uiModePickProvider {
		switch msg := msg.(type) {
		case tickMsg:
			m.consumeStreamTick()
			return m, tickCmd()
		case streamMsg:
			if msg.err != nil {
				m.status = "stream error: " + msg.err.Error()
				return m, nil
			}
			if msg.id != m.selectedSessionID() {
				if msg.cancel != nil {
					msg.cancel()
				}
				return m, nil
			}
			if m.stream != nil {
				m.stream.SetStream(msg.ch, msg.cancel)
			}
			m.status = "streaming"
			return m, nil
		case tea.KeyMsg:
			switch msg.String() {
			case "esc":
				m.exitProviderPick("new session canceled")
				return m, nil
			case "enter":
				return m, m.selectProvider()
			case "j", "down":
				if m.providerPicker != nil {
					m.providerPicker.Move(1)
				}
				return m, nil
			case "k", "up":
				if m.providerPicker != nil {
					m.providerPicker.Move(-1)
				}
				return m, nil
			}
		case tea.MouseMsg:
			if m.handleMouse(msg) {
				if m.pendingMouseCmd != nil {
					cmd := m.pendingMouseCmd
					m.pendingMouseCmd = nil
					return m, cmd
				}
				return m, nil
			}
		}
		return m, nil
	}
	if m.mode == uiModeCompose {
		switch msg := msg.(type) {
		case tickMsg:
			m.consumeStreamTick()
			return m, tickCmd()
		case streamMsg:
			if msg.err != nil {
				m.status = "stream error: " + msg.err.Error()
				return m, nil
			}
			if msg.id != m.selectedSessionID() {
				if msg.cancel != nil {
					msg.cancel()
				}
				return m, nil
			}
			if m.stream != nil {
				m.stream.SetStream(msg.ch, msg.cancel)
			}
			m.status = "streaming"
			return m, nil
		}
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.resize(msg.Width, msg.Height)
		return m, nil
	case tea.KeyMsg:
		if m.mode == uiModeSearch {
			switch msg.String() {
			case "esc":
				m.exitSearch("search canceled")
				return m, nil
			case "enter":
				if m.searchInput != nil {
					query := m.searchInput.Value()
					m.applySearch(query)
				}
				m.exitSearch("")
				return m, nil
			}
			if m.searchInput != nil {
				cmd := m.searchInput.Update(msg)
				return m, cmd
			}
			return m, nil
		}
		if m.pendingApproval != nil && (m.mode == uiModeNormal || (m.input != nil && m.input.IsSidebarFocused())) {
			switch msg.String() {
			case "y":
				return m, m.approvePending("accept")
			case "x":
				return m, m.approvePending("decline")
			}
		}
		if m.input != nil && m.input.IsChatFocused() {
			switch msg.String() {
			case "esc":
				m.exitCompose("compose canceled")
				return m, nil
			case "enter":
				if m.chatInput != nil {
					text := strings.TrimSpace(m.chatInput.Value())
					if text == "" {
						m.status = "message is required"
						return m, nil
					}
					if m.newSession != nil {
						target := m.newSession
						if strings.TrimSpace(target.provider) == "" {
							m.status = "provider is required"
							return m, nil
						}
						m.status = "starting session"
						m.chatInput.Clear()
						return m, m.startWorkspaceSessionCmd(target.workspaceID, target.worktreeID, target.provider, text)
					}
					sessionID := m.composeSessionID()
					if sessionID == "" {
						m.status = "select a session to chat"
						return m, nil
					}
					provider := m.providerForSessionID(sessionID)
					token := m.nextSendToken()
					m.registerPendingSend(token, sessionID, provider)
					headerIndex := m.appendUserMessageLocal(provider, text)
					m.status = "sending message"
					m.chatInput.Clear()
					if headerIndex >= 0 {
						m.registerPendingSendHeader(token, sessionID, provider, headerIndex)
					}
					send := sendSessionCmd(m.sessionAPI, sessionID, text, token)
					if shouldStreamItems(provider) {
						return m, tea.Sequence(openItemsCmd(m.sessionAPI, sessionID), send)
					}
					if provider == "codex" {
						return m, tea.Sequence(openEventsCmd(m.sessionAPI, sessionID), send)
					}
					return m, send
				}
				return m, nil
			case "ctrl+y":
				id := m.selectedSessionID()
				if id == "" {
					m.status = "no session selected"
					return m, nil
				}
				if err := clipboard.WriteAll(id); err != nil {
					m.status = "copy failed: " + err.Error()
					return m, nil
				}
				m.status = "copied session id"
				return m, nil
			}
			if m.chatInput != nil {
				cmd := m.chatInput.Update(msg)
				return m, cmd
			}
			return m, nil
		}
		if m.sidebar != nil {
			if msg.String() == "up" {
				m.sidebar.CursorUp()
				return m, m.onSelectionChanged()
			}
			if msg.String() == "down" {
				m.sidebar.CursorDown()
				return m, m.onSelectionChanged()
			}
		}
		if m.handleViewportScroll(msg) {
			return m, nil
		}
		switch msg.String() {
		case "esc":
			return m, nil
		case "q", "ctrl+c":
			return m, tea.Quit
		case "ctrl+b":
			m.toggleSidebar()
			return m, m.saveAppStateCmd()
		case "ctrl+y":
			id := m.selectedSessionID()
			if id == "" {
				m.status = "no session selected"
				return m, nil
			}
			if err := clipboard.WriteAll(id); err != nil {
				m.status = "copy failed: " + err.Error()
				return m, nil
			}
			m.status = "copied session id"
			return m, nil
		case "/":
			m.enterSearch()
			return m, nil
		case "g":
			m.viewport.GotoTop()
			if m.follow {
				m.follow = false
				m.status = "follow: paused"
			}
			return m, nil
		case "G":
			m.viewport.GotoBottom()
			m.follow = true
			m.status = "follow: on"
			return m, nil
		case "{":
			m.jumpSection(-1)
			return m, nil
		case "}":
			m.jumpSection(1)
			return m, nil
		case "N":
			m.moveSearch(-1)
			return m, nil
		case "n":
			m.moveSearch(1)
			return m, nil
		case "ctrl+n":
			if m.enterNewSession() {
				return m, nil
			}
			return m, nil
		case "a":
			m.enterAddWorkspace()
			return m, nil
		case "t":
			item := m.selectedItem()
			if item == nil || item.kind != sidebarWorkspace || item.workspace == nil || item.workspace.ID == "" {
				m.status = "select a workspace to add a worktree"
				return m, nil
			}
			m.enterAddWorktree(item.workspace.ID)
			return m, nil
		case "c":
			id := m.selectedSessionID()
			if id == "" {
				m.status = "select a session to send"
				return m, nil
			}
			m.enterCompose(id)
			return m, nil
		case "enter":
			id := m.selectedSessionID()
			if id == "" {
				m.status = "select a session to chat"
				return m, nil
			}
			m.enterCompose(id)
			return m, nil
		case "r":
			m.status = "refreshing"
			return m, tea.Batch(fetchWorkspacesCmd(m.workspaceAPI), fetchSessionsWithMetaCmd(m.sessionAPI))
		case "x":
			id := m.selectedSessionID()
			if id == "" {
				m.status = "no session selected"
				return m, nil
			}
			m.status = "killing " + id
			return m, killSessionCmd(m.sessionAPI, id)
		case "i":
			id := m.selectedSessionID()
			if id == "" {
				m.status = "no session selected"
				return m, nil
			}
			m.status = "interrupting " + id
			return m, interruptSessionCmd(m.sessionAPI, id)
		case "d":
			ids := m.selectedSessionIDs()
			if len(ids) == 0 {
				m.status = "no session selected"
				return m, nil
			}
			if len(ids) == 1 {
				m.status = "marking exited " + ids[0]
				return m, markExitedCmd(m.sessionAPI, ids[0])
			}
			m.status = fmt.Sprintf("marking exited %d sessions", len(ids))
			return m, markExitedManyCmd(m.sessionAPI, ids)
		case "p":
			m.follow = !m.follow
			if m.follow {
				m.viewport.GotoBottom()
				m.status = "follow: on"
			} else {
				m.status = "follow: paused"
			}
			return m, nil
		case " ", "space":
			if m.toggleSelection() {
				count := 0
				if m.sidebar != nil {
					count = m.sidebar.SelectionCount()
				}
				m.status = fmt.Sprintf("selected %d", count)
				if m.advanceToNextSession() {
					return m, m.onSelectionChanged()
				}
			}
			return m, nil
		case "j":
			m.sidebar.CursorDown()
			return m, m.onSelectionChanged()
		case "k":
			m.sidebar.CursorUp()
			return m, m.onSelectionChanged()
		}
	case tea.MouseMsg:
		if m.handleMouse(msg) {
			if m.pendingMouseCmd != nil {
				cmd := m.pendingMouseCmd
				m.pendingMouseCmd = nil
				return m, cmd
			}
			return m, nil
		}
	}

	prevKey := m.selectedKey()
	var cmd tea.Cmd
	cmd = m.sidebar.Update(msg)
	if key := m.selectedKey(); key != prevKey {
		return m, tea.Batch(cmd, m.onSelectionChanged())
	}

	switch msg := msg.(type) {
	case sessionsWithMetaMsg:
		if msg.err != nil {
			m.status = "error: " + msg.err.Error()
			return m, nil
		}
		m.sessions = msg.sessions
		m.sessionMeta = normalizeSessionMeta(msg.meta)
		m.pruneSelection()
		m.applySidebarItems()
		if m.pendingSelectID != "" && m.sidebar != nil {
			if m.sidebar.SelectBySessionID(m.pendingSelectID) {
				m.pendingSelectID = ""
			}
		}
		m.status = fmt.Sprintf("%d sessions", len(msg.sessions))
		return m, m.onSelectionChanged()
	case workspacesMsg:
		if msg.err != nil {
			m.status = "workspaces error: " + msg.err.Error()
			return m, nil
		}
		m.workspaces = msg.workspaces
		m.applySidebarItems()
		return m, m.fetchWorktreesForWorkspaces()
	case worktreesMsg:
		if msg.err != nil {
			m.status = "worktrees error: " + msg.err.Error()
			return m, nil
		}
		if msg.workspaceID != "" {
			m.worktrees[msg.workspaceID] = msg.worktrees
		}
		m.applySidebarItems()
		return m, nil
	case appStateMsg:
		if msg.err != nil {
			m.status = "state error: " + msg.err.Error()
			return m, nil
		}
		if msg.state != nil {
			m.applyAppState(msg.state)
			m.applySidebarItems()
			m.resize(m.width, m.height)
		}
		return m, nil
	case appStateSavedMsg:
		if msg.err != nil {
			m.status = "state save error: " + msg.err.Error()
			return m, nil
		}
		if msg.state != nil {
			m.applyAppState(msg.state)
		}
		return m, nil
	case tailMsg:
		if msg.err != nil {
			m.status = "tail error: " + msg.err.Error()
			if msg.key != "" && msg.key == m.loadingKey {
				m.loading = false
				m.setContentText("Error loading history.")
			}
			return m, nil
		}
		if msg.key != "" && msg.key != m.pendingSessionKey {
			return m, nil
		}
		if msg.key != "" && msg.key == m.loadingKey {
			m.loading = false
		}
		lines := itemsToLines(msg.items)
		if shouldStreamItems(m.selectedSessionProvider()) && m.itemStream != nil {
			m.itemStream.SetSnapshot(lines)
			lines = m.itemStream.Lines()
		}
		m.setSnapshot(lines, false)
		m.status = "tail updated"
		return m, nil
	case historyMsg:
		if msg.err != nil {
			m.status = "history error: " + msg.err.Error()
			if msg.key != "" && msg.key == m.loadingKey {
				m.loading = false
				m.setContentText("Error loading history.")
			}
			return m, nil
		}
		if msg.key != "" && msg.key != m.pendingSessionKey {
			return m, nil
		}
		if msg.key != "" && msg.key == m.loadingKey {
			m.loading = false
		}
		lines := itemsToLines(msg.items)
		if shouldStreamItems(m.selectedSessionProvider()) && m.itemStream != nil {
			m.itemStream.SetSnapshot(lines)
			lines = m.itemStream.Lines()
		}
		m.setSnapshot(lines, false)
		if msg.key != "" {
			m.transcriptCache[msg.key] = m.currentLines()
		}
		m.status = "history updated"
		return m, nil
	case sendMsg:
		if msg.err != nil {
			m.status = "send error: " + msg.err.Error()
			m.markPendingSendFailed(msg.token, msg.err)
			return m, nil
		}
		m.status = "message sent"
		m.clearPendingSend(msg.token)
		return m, nil
	case approvalMsg:
		if msg.err != nil {
			m.status = "approval error: " + msg.err.Error()
			return m, nil
		}
		m.status = "approval sent"
		if m.pendingApproval != nil && m.pendingApproval.RequestID == msg.requestID {
			m.pendingApproval = nil
		}
		if m.codexStream != nil {
			m.codexStream.ClearApproval()
		}
		return m, nil
	case approvalsMsg:
		if msg.err != nil {
			m.status = "approvals error: " + msg.err.Error()
			return m, nil
		}
		if msg.id != m.selectedSessionID() {
			return m, nil
		}
		m.pendingApproval = selectApprovalRequest(msg.approvals)
		if m.pendingApproval != nil {
			if m.pendingApproval.Detail != "" {
				m.status = fmt.Sprintf("approval required: %s (%s)", m.pendingApproval.Summary, m.pendingApproval.Detail)
			} else if m.pendingApproval.Summary != "" {
				m.status = "approval required: " + m.pendingApproval.Summary
			} else {
				m.status = "approval required"
			}
		}
		return m, nil
	case interruptMsg:
		if msg.err != nil {
			m.status = "interrupt error: " + msg.err.Error()
			return m, nil
		}
		m.status = "interrupt sent"
		return m, nil
	case selectDebounceMsg:
		if msg.seq != m.selectSeq {
			return m, nil
		}
		item := m.selectedItem()
		if item == nil || !item.isSession() || item.session == nil || item.session.ID != msg.id {
			return m, nil
		}
		return m, m.loadSelectedSession(item)
	case startSessionMsg:
		if msg.err != nil {
			m.status = "start session error: " + msg.err.Error()
			return m, nil
		}
		if msg.session == nil || msg.session.ID == "" {
			m.status = "start session error: no session returned"
			return m, nil
		}
		m.newSession = nil
		m.pendingSelectID = msg.session.ID
		label := msg.session.Title
		if strings.TrimSpace(label) == "" {
			label = msg.session.ID
		}
		if m.compose != nil {
			m.compose.Enter(msg.session.ID, label)
		}
		m.status = "session started"
		return m, fetchSessionsWithMetaCmd(m.sessionAPI)
	case killMsg:
		if msg.err != nil {
			m.status = "kill error: " + msg.err.Error()
			return m, nil
		}
		m.status = "killed " + msg.id
		return m, fetchSessionsWithMetaCmd(m.sessionAPI)
	case exitMsg:
		if msg.err != nil {
			m.status = "exit error: " + msg.err.Error()
			return m, nil
		}
		if m.sidebar != nil {
			m.sidebar.RemoveSelection([]string{msg.id})
		}
		m.status = "marked exited " + msg.id
		return m, fetchSessionsWithMetaCmd(m.sessionAPI)
	case bulkExitMsg:
		if msg.err != nil {
			m.status = "exit error: " + msg.err.Error()
			return m, nil
		}
		if m.sidebar != nil {
			m.sidebar.RemoveSelection(msg.ids)
		}
		m.status = fmt.Sprintf("marked exited %d", len(msg.ids))
		return m, fetchSessionsWithMetaCmd(m.sessionAPI)
	case streamMsg:
		if msg.err != nil {
			m.status = "stream error: " + msg.err.Error()
			return m, nil
		}
		targetID := m.composeSessionID()
		if targetID == "" {
			targetID = m.selectedSessionID()
		}
		if msg.id != targetID {
			if msg.cancel != nil {
				msg.cancel()
			}
			return m, nil
		}
		if m.stream != nil {
			m.stream.SetStream(msg.ch, msg.cancel)
		}
		m.status = "streaming"
		return m, nil
	case eventsMsg:
		if msg.err != nil {
			m.status = "events error: " + msg.err.Error()
			return m, nil
		}
		targetID := m.composeSessionID()
		if targetID == "" {
			targetID = m.selectedSessionID()
		}
		if msg.id != targetID {
			if msg.cancel != nil {
				msg.cancel()
			}
			return m, nil
		}
		if m.codexStream != nil {
			m.codexStream.SetStream(msg.ch, msg.cancel)
		}
		m.status = "streaming events"
		return m, nil
	case itemsStreamMsg:
		if msg.err != nil {
			m.status = "items stream error: " + msg.err.Error()
			return m, nil
		}
		targetID := m.composeSessionID()
		if targetID == "" {
			targetID = m.selectedSessionID()
		}
		if msg.id != targetID {
			if msg.cancel != nil {
				msg.cancel()
			}
			return m, nil
		}
		if m.itemStream != nil {
			m.itemStream.SetSnapshot(m.currentLines())
			m.itemStream.SetStream(msg.ch, msg.cancel)
		}
		m.status = "streaming items"
		return m, nil
	case tickMsg:
		m.consumeStreamTick()
		m.consumeCodexTick()
		m.consumeItemTick()
		if m.loading {
			m.loader, _ = m.loader.Update(spinner.TickMsg{Time: time.Time(msg), ID: m.loader.ID()})
			m.setLoadingContent()
			return m, tickCmd()
		}
		return m, tickCmd()
	}

	return m, cmd
}

func (m *Model) View() string {
	headerText := "Tail"
	bodyText := m.viewport.View()
	if m.mode == uiModeAddWorkspace {
		headerText = "Add Workspace"
		if m.addWorkspace != nil {
			bodyText = m.addWorkspace.View()
		}
	} else if m.mode == uiModeAddWorktree {
		headerText = "Add Worktree"
		if m.addWorktree != nil {
			bodyText = m.addWorktree.View()
		}
	} else if m.mode == uiModePickProvider {
		headerText = "Provider"
		if m.providerPicker != nil {
			bodyText = m.providerPicker.View()
		}
	} else if m.mode == uiModeCompose {
		headerText = "Chat"
	} else if m.mode == uiModeSearch {
		headerText = "Search"
	}
	rightHeader := headerStyle.Render(headerText)
	rightBody := bodyText
	inputLine := ""
	inputScrollable := false
	if m.mode == uiModeCompose && m.chatInput != nil {
		inputLine = m.chatInput.View()
		inputScrollable = m.chatInput.CanScroll()
	}
	if m.mode == uiModeSearch && m.searchInput != nil {
		inputLine = m.searchInput.View()
		inputScrollable = m.searchInput.CanScroll()
	}
	rightLines := []string{rightHeader, rightBody}
	if inputLine != "" {
		dividerWidth := m.viewport.Width
		if dividerWidth <= 0 {
			dividerWidth = max(1, m.width)
		}
		dividerLine := renderInputDivider(dividerWidth, inputScrollable)
		if dividerLine != "" {
			rightLines = append(rightLines, dividerLine)
		}
		rightLines = append(rightLines, inputLine)
	}
	rightView := lipgloss.JoinVertical(lipgloss.Left, rightLines...)
	body := rightView
	if !m.appState.SidebarCollapsed {
		listView := ""
		if m.sidebar != nil {
			listView = m.sidebar.View()
		}
		height := max(lipgloss.Height(listView), lipgloss.Height(rightView))
		if height < 1 {
			height = 1
		}
		divider := strings.Repeat("│\n", height-1) + "│"
		body = lipgloss.JoinHorizontal(lipgloss.Top, listView, dividerStyle.Render(divider), rightView)
	}

	helpText := ""
	if m.hotkeys != nil {
		helpText = m.hotkeys.Render(m)
	}
	if helpText == "" {
		helpText = "q quit"
	}
	help := helpStyle.Render(helpText)
	status := statusStyle.Render(m.status)
	statusLine := renderStatusLine(m.width, help, status)

	if m.height <= 0 || m.width <= 0 {
		return body
	}
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
	} else if m.mode == uiModeSearch {
		extraLines = 2
	}
	vpHeight := max(1, contentHeight-1-extraLines)
	m.viewport.Width = viewportWidth
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
	if m.chatInput != nil {
		m.chatInput.Resize(viewportWidth)
	}
	if m.searchInput != nil {
		m.searchInput.Resize(viewportWidth)
	}
	m.renderViewport()
}

func (m *Model) onSelectionChanged() tea.Cmd {
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
		m.status = "workspace selected"
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
		m.status = "worktree selected"
		if stateChanged {
			return m.saveAppStateCmd()
		}
		return nil
	}
	if !item.isSession() {
		return nil
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
	cmd := m.scheduleSessionLoad(item)
	if stateChanged {
		return tea.Batch(cmd, m.saveAppStateCmd())
	}
	return cmd
}

func (m *Model) scheduleSessionLoad(item *sidebarItem) tea.Cmd {
	if item == nil || item.session == nil {
		return nil
	}
	m.selectSeq++
	return debounceSelectCmd(item.session.ID, m.selectSeq, selectionDebounce)
}

func (m *Model) loadSelectedSession(item *sidebarItem) tea.Cmd {
	if item == nil || item.session == nil {
		return nil
	}
	id := item.session.ID
	m.resetStream()
	m.pendingApproval = nil
	m.pendingSessionKey = item.key()
	m.status = "loading " + id
	if cached, ok := m.transcriptCache[item.key()]; ok {
		m.setSnapshot(cached, false)
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

func (m *Model) applySidebarItems() {
	if m.sidebar == nil {
		m.resetStream()
		m.setContentText("No sessions.")
		return
	}
	item := m.sidebar.Apply(m.workspaces, m.worktrees, m.sessions, m.sessionMeta, m.appState.ActiveWorkspaceID, m.appState.ActiveWorktreeID)
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

func tickCmd() tea.Cmd {
	return tea.Tick(tickInterval, func(t time.Time) tea.Msg {
		return tickMsg(t)
	})
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
}

func (m *Model) consumeStreamTick() {
	if m.stream == nil {
		return
	}
	lines, changed, closed := m.stream.ConsumeTick()
	if closed {
		m.status = "stream closed"
	}
	if changed {
		m.applyLines(lines, true)
	}
}

func (m *Model) consumeCodexTick() {
	if m.codexStream == nil {
		return
	}
	lines, changed, closed := m.codexStream.ConsumeTick()
	if closed {
		m.status = "events closed"
	}
	if changed {
		m.applyLines(lines, false)
	}
	if approval := m.codexStream.PendingApproval(); approval != nil {
		m.pendingApproval = approval
		if approval.Summary != "" {
			if approval.Detail != "" {
				m.status = fmt.Sprintf("approval required: %s (%s)", approval.Summary, approval.Detail)
			} else {
				m.status = "approval required: " + approval.Summary
			}
		} else {
			m.status = "approval required"
		}
	}
}

func (m *Model) consumeItemTick() {
	if m.itemStream == nil {
		return
	}
	lines, changed, closed := m.itemStream.ConsumeTick()
	if closed {
		m.status = "items stream closed"
	}
	if changed {
		m.applyLines(lines, false)
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

func (m *Model) applyLines(lines []string, escape bool) {
	m.contentRaw = strings.Join(lines, "\n")
	m.contentEsc = escape
	m.contentVersion++
	m.searchVersion = -1
	m.sectionVersion = -1
	m.renderViewport()
}

func (m *Model) setContentText(text string) {
	m.contentRaw = text
	m.contentEsc = false
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
	content := m.contentRaw
	if m.contentEsc {
		content = escapeMarkdown(content)
	}
	rendered := renderMarkdown(content, renderWidth)
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
	m.renderVersion++
	m.viewport.SetContent(rendered)
	if m.follow {
		m.viewport.GotoBottom()
	}
}

func (m *Model) setLoadingContent() {
	m.setContentText(m.loader.View() + " Loading...")
}

func (m *Model) appendUserMessageLocal(provider, text string) int {
	if shouldStreamItems(provider) && m.itemStream != nil {
		m.itemStream.SetSnapshot(m.currentLines())
		headerIndex := m.itemStream.AppendUserMessage(text)
		if headerIndex >= 0 {
			_ = m.itemStream.MarkUserMessageSending(headerIndex)
		}
		lines := m.itemStream.Lines()
		m.applyLines(lines, false)
		if key := m.selectedKey(); key != "" {
			m.transcriptCache[key] = lines
		}
		return headerIndex
	}
	if m.codexStream == nil {
		return -1
	}
	m.codexStream.SetSnapshot(m.currentLines())
	headerIndex := m.codexStream.AppendUserMessage(text)
	if headerIndex >= 0 {
		_ = m.codexStream.MarkUserMessageSending(headerIndex)
	}
	lines := m.codexStream.Lines()
	m.applyLines(lines, false)
	if key := m.selectedKey(); key != "" {
		m.transcriptCache[key] = lines
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
					lines := m.itemStream.Lines()
					m.applyLines(lines, false)
					m.transcriptCache[entry.key] = lines
					return
				}
			}
			if m.codexStream != nil {
				if m.codexStream.MarkUserMessageSent(entry.headerLine) {
					lines := m.codexStream.Lines()
					m.applyLines(lines, false)
					m.transcriptCache[entry.key] = lines
					return
				}
			}
		}
		if cached, ok := m.transcriptCache[entry.key]; ok {
			if entry.headerLine >= 0 && entry.headerLine < len(cached) {
				cached[entry.headerLine] = "### User"
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
				lines := m.itemStream.Lines()
				m.applyLines(lines, false)
				m.transcriptCache[entry.key] = lines
				return
			}
		}
		if m.codexStream != nil {
			if m.codexStream.MarkUserMessageFailed(entry.headerLine) {
				lines := m.codexStream.Lines()
				m.applyLines(lines, false)
				m.transcriptCache[entry.key] = lines
				return
			}
		}
	}
	if cached, ok := m.transcriptCache[entry.key]; ok {
		if entry.headerLine >= 0 && entry.headerLine < len(cached) {
			cached[entry.headerLine] = "### User (failed)"
			m.transcriptCache[entry.key] = cached
		}
	}
}

func (m *Model) handleViewportScroll(msg tea.KeyMsg) bool {
	if m.mode != uiModeNormal && m.mode != uiModeCompose {
		return false
	}
	switch msg.String() {
	case "up":
		m.viewport.LineUp(1)
	case "down":
		m.viewport.LineDown(1)
	case "pgup":
		m.viewport.PageUp()
	case "pgdown":
		m.viewport.PageDown()
	case "ctrl+f":
		m.viewport.PageDown()
	case "ctrl+u":
		m.viewport.HalfPageUp()
	case "ctrl+d":
		m.viewport.HalfPageDown()
	case "home":
		m.viewport.GotoTop()
	case "end":
		m.viewport.GotoBottom()
		m.follow = true
		m.status = "follow: on"
		return true
	default:
		return false
	}
	if m.follow {
		m.follow = false
		m.status = "follow: paused"
	}
	return true
}

func (m *Model) handleMouse(msg tea.MouseMsg) bool {
	if m.width <= 0 || m.height <= 0 {
		return false
	}
	listWidth := 0
	if !m.appState.SidebarCollapsed {
		listWidth = clamp(m.width/3, minListWidth, maxListWidth)
		if m.width-listWidth-1 < minViewportWidth {
			listWidth = max(minListWidth, m.width/2)
		}
	}
	rightStart := 0
	if listWidth > 0 {
		rightStart = listWidth + 1
	}
	inputBounds := func() (int, int, bool) {
		if m.mode == uiModeCompose && m.chatInput != nil {
			start := m.viewport.Height + 2
			end := start + m.chatInput.Height() - 1
			return start, end, true
		}
		if m.mode == uiModeSearch && m.searchInput != nil {
			start := m.viewport.Height + 2
			end := start + m.searchInput.Height() - 1
			return start, end, true
		}
		return 0, 0, false
	}
	isOverInput := func(y int) bool {
		start, end, ok := inputBounds()
		if !ok {
			return false
		}
		return y >= start && y <= end
	}

	if msg.Action == tea.MouseActionPress {
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			if listWidth > 0 && msg.X < listWidth {
				if m.sidebar != nil && m.sidebar.Scroll(-1) {
					m.pendingMouseCmd = m.onSelectionChanged()
					return true
				}
			}
			if m.mode == uiModePickProvider && msg.X >= rightStart {
				if m.providerPicker != nil && m.providerPicker.Scroll(-1) {
					return true
				}
			}
			if msg.X >= rightStart && isOverInput(msg.Y) {
				if m.mode == uiModeCompose && m.chatInput != nil {
					m.pendingMouseCmd = m.chatInput.Scroll(-1)
					return true
				}
				if m.mode == uiModeSearch && m.searchInput != nil {
					m.pendingMouseCmd = m.searchInput.Scroll(-1)
					return true
				}
			}
			if m.mode == uiModeAddWorktree && m.addWorktree != nil {
				if m.addWorktree.Scroll(-1) {
					return true
				}
			}
			m.viewport.LineUp(3)
			if m.follow {
				m.follow = false
				m.status = "follow: paused"
			}
			return true
		case tea.MouseButtonWheelDown:
			if listWidth > 0 && msg.X < listWidth {
				if m.sidebar != nil && m.sidebar.Scroll(1) {
					m.pendingMouseCmd = m.onSelectionChanged()
					return true
				}
			}
			if m.mode == uiModePickProvider && msg.X >= rightStart {
				if m.providerPicker != nil && m.providerPicker.Scroll(1) {
					return true
				}
			}
			if msg.X >= rightStart && isOverInput(msg.Y) {
				if m.mode == uiModeCompose && m.chatInput != nil {
					m.pendingMouseCmd = m.chatInput.Scroll(1)
					return true
				}
				if m.mode == uiModeSearch && m.searchInput != nil {
					m.pendingMouseCmd = m.searchInput.Scroll(1)
					return true
				}
			}
			if m.mode == uiModeAddWorktree && m.addWorktree != nil {
				if m.addWorktree.Scroll(1) {
					return true
				}
			}
			m.viewport.LineDown(3)
			if m.follow {
				m.follow = false
				m.status = "follow: paused"
			}
			return true
		case tea.MouseButtonLeft:
			break
		default:
			return false
		}
	}
	if msg.Action != tea.MouseActionPress || msg.Button != tea.MouseButtonLeft {
		return false
	}
	if msg.X >= rightStart && isOverInput(msg.Y) {
		if m.mode == uiModeCompose && m.chatInput != nil {
			m.chatInput.Focus()
			if m.input != nil {
				m.input.FocusChatInput()
			}
			return true
		}
		if m.mode == uiModeSearch && m.searchInput != nil {
			m.searchInput.Focus()
			if m.input != nil {
				m.input.FocusChatInput()
			}
			return true
		}
	}
	if m.mode == uiModePickProvider && m.providerPicker != nil {
		if msg.X >= rightStart {
			row := msg.Y - 1
			if row >= 0 && m.providerPicker.SelectByRow(row) {
				m.pendingMouseCmd = m.selectProvider()
				return true
			}
		}
	}
	if m.mode == uiModeAddWorktree && m.addWorktree != nil {
		if msg.X >= rightStart {
			row := msg.Y - 1
			if row >= 0 {
				if handled, cmd := m.addWorktree.HandleClick(row, m); handled {
					m.pendingMouseCmd = cmd
					return true
				}
			}
		}
	}
	if listWidth > 0 && msg.X < listWidth {
		if m.sidebar != nil {
			m.sidebar.SelectByRow(msg.Y)
			m.pendingMouseCmd = m.onSelectionChanged()
			return true
		}
	}
	return false
}

func (m *Model) currentLines() []string {
	if strings.TrimSpace(m.contentRaw) == "" {
		return nil
	}
	return strings.Split(m.contentRaw, "\n")
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
	m.hasAppState = true
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
	m.status = "add workspace: enter path"
}

func (m *Model) exitAddWorkspace(status string) {
	m.mode = uiModeNormal
	if m.addWorkspace != nil {
		m.addWorkspace.Exit()
	}
	if status != "" {
		m.status = status
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
	m.status = "add worktree: new or existing"
}

func (m *Model) exitAddWorktree(status string) {
	m.mode = uiModeNormal
	if m.addWorktree != nil {
		m.addWorktree.Exit()
	}
	if status != "" {
		m.status = status
	}
}

func (m *Model) enterCompose(sessionID string) {
	m.mode = uiModeCompose
	label := m.selectedSessionLabel()
	if m.compose != nil {
		m.compose.Enter(sessionID, label)
	}
	if m.chatInput != nil {
		m.chatInput.SetPlaceholder("message")
		m.chatInput.Focus()
	}
	if m.input != nil {
		m.input.FocusChatInput()
	}
	m.status = "compose message"
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
		m.status = status
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
	m.status = "choose provider"
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
		m.status = status
	}
	m.resize(m.width, m.height)
}

func (m *Model) selectProvider() tea.Cmd {
	if m.providerPicker == nil {
		return nil
	}
	provider := m.providerPicker.Selected()
	if provider == "" {
		m.status = "provider is required"
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
	m.status = "provider set: " + provider
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
	m.status = "search"
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
		m.status = status
	}
	m.resize(m.width, m.height)
}

func (m *Model) applySearch(query string) {
	query = strings.TrimSpace(query)
	if query == "" {
		m.searchQuery = ""
		m.searchMatches = nil
		m.searchIndex = -1
		m.status = "search cleared"
		return
	}
	m.searchQuery = query
	m.searchMatches = m.findSearchMatches(query)
	m.searchVersion = m.renderVersion
	if len(m.searchMatches) == 0 {
		m.searchIndex = -1
		m.status = "no matches"
		return
	}
	m.searchIndex = selectSearchIndex(m.searchMatches, m.viewport.YOffset, 1)
	if m.searchIndex < 0 {
		m.searchIndex = 0
	}
	m.jumpToLine(m.searchMatches[m.searchIndex])
	m.status = fmt.Sprintf("match %d/%d", m.searchIndex+1, len(m.searchMatches))
}

func (m *Model) moveSearch(delta int) {
	if m.searchQuery == "" {
		m.status = "no search"
		return
	}
	if m.searchVersion != m.renderVersion {
		m.searchMatches = m.findSearchMatches(m.searchQuery)
		m.searchVersion = m.renderVersion
		m.searchIndex = -1
	}
	if len(m.searchMatches) == 0 {
		m.searchIndex = -1
		m.status = "no matches"
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
	m.status = fmt.Sprintf("match %d/%d", m.searchIndex+1, len(m.searchMatches))
}

func (m *Model) approvePending(decision string) tea.Cmd {
	if m.pendingApproval == nil {
		return nil
	}
	sessionID := m.selectedSessionID()
	if sessionID == "" {
		m.status = "select a session to approve"
		return nil
	}
	reqID := m.pendingApproval.RequestID
	if reqID <= 0 {
		m.status = "invalid approval request"
		return nil
	}
	m.status = "sending approval"
	return approveSessionCmd(m.sessionAPI, sessionID, reqID, decision)
}

func selectApprovalRequest(records []*types.Approval) *ApprovalRequest {
	if len(records) == 0 {
		return nil
	}
	var latest *types.Approval
	for _, record := range records {
		if record == nil {
			continue
		}
		if latest == nil {
			latest = record
			continue
		}
		if record.CreatedAt.After(latest.CreatedAt) {
			latest = record
		}
	}
	if latest == nil {
		return nil
	}
	return approvalFromRecord(latest)
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
		m.follow = false
	}
}

func (m *Model) jumpSection(delta int) {
	offsets := m.sectionOffsetsCached()
	if len(offsets) == 0 {
		m.status = "no sections"
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
		m.status = "select a workspace or worktree"
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
	return types.Capabilities(provider).UsesItems
}

func (m *Model) composeSessionID() string {
	if m.compose == nil {
		return ""
	}
	return m.compose.sessionID
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
