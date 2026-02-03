package app

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"control/internal/client"
	"control/internal/types"
)

const (
	defaultTailLines  = 200
	maxViewportLines  = 2000
	maxEventsPerTick  = 64
	tickInterval      = 100 * time.Millisecond
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
)

type Model struct {
	workspaceAPI WorkspaceAPI
	sessionAPI   SessionAPI
	stateAPI     StateAPI
	sidebar      *SidebarController
	viewport     viewport.Model
	mode         uiMode
	addWorkspace *AddWorkspaceController
	addWorktree  *AddWorktreeController
	status       string
	width        int
	height       int
	follow       bool
	workspaces   []*types.Workspace
	worktrees    map[string][]*types.Worktree
	sessions     []*types.Session
	sessionMeta  map[string]*types.SessionMeta
	appState     types.AppState
	hasAppState  bool
	stream       *StreamController
}

func NewModel(client *client.Client) Model {
	vp := viewport.New(minViewportWidth, minContentHeight-1)
	vp.SetContent("No sessions.")

	api := NewClientAPI(client)

	return Model{
		workspaceAPI: api,
		sessionAPI:   api,
		stateAPI:     api,
		sidebar:      NewSidebarController(),
		viewport:     vp,
		stream:       NewStreamController(maxViewportLines, maxEventsPerTick),
		mode:         uiModeNormal,
		addWorkspace: NewAddWorkspaceController(minViewportWidth),
		addWorktree:  NewAddWorktreeController(minViewportWidth),
		status:       "",
		follow:       true,
		worktrees:    map[string][]*types.Worktree{},
		sessionMeta:  map[string]*types.SessionMeta{},
	}
}

func Run(client *client.Client) error {
	model := NewModel(client)
	p := tea.NewProgram(&model, tea.WithAltScreen())
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
		}
		if m.addWorktree == nil {
			return m, nil
		}
		_, cmd := m.addWorktree.Update(msg, m)
		return m, cmd
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.resize(msg.Width, msg.Height)
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "ctrl+b":
			m.toggleSidebar()
			return m, m.saveAppStateCmd()
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
			return m, nil
		}
		m.setSnapshot(itemsToLines(msg.items))
		m.status = "tail updated"
		return m, nil
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
	case tickMsg:
		m.consumeStreamTick()
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
	}
	rightHeader := headerStyle.Render(headerText)
	rightBody := bodyText
	rightView := lipgloss.JoinVertical(lipgloss.Left, rightHeader, rightBody)
	body := rightView
	if !m.appState.SidebarCollapsed {
		listView := ""
		if m.sidebar != nil {
			listView = m.sidebar.View()
		}
		body = lipgloss.JoinHorizontal(lipgloss.Top, listView, lipgloss.NewStyle().PaddingLeft(1).Render(rightView))
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
	vpHeight := max(1, contentHeight-1)
	m.viewport.Width = viewportWidth
	m.viewport.Height = vpHeight
	if m.addWorkspace != nil {
		m.addWorkspace.Resize(viewportWidth)
	}
	if m.addWorktree != nil {
		m.addWorktree.Resize(viewportWidth)
	}
}

func (m *Model) onSelectionChanged() tea.Cmd {
	item := m.selectedItem()
	if item == nil {
		m.resetStream()
		m.viewport.SetContent("No sessions.")
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
	id := item.session.ID
	m.resetStream()
	m.status = "loading " + id
	cmds := []tea.Cmd{fetchTailCmd(m.sessionAPI, id)}
	if isActiveStatus(item.session.Status) {
		cmds = append(cmds, openStreamCmd(m.sessionAPI, id))
	}
	if stateChanged {
		cmds = append(cmds, m.saveAppStateCmd())
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
		m.viewport.SetContent("No sessions.")
		return
	}
	item := m.sidebar.Apply(m.workspaces, m.worktrees, m.sessions, m.sessionMeta, m.appState.ActiveWorkspaceID, m.appState.ActiveWorktreeID)
	if item == nil {
		m.resetStream()
		m.viewport.SetContent("No sessions.")
		return
	}
	if !item.isSession() {
		m.resetStream()
		m.viewport.SetContent("Select a session.")
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
		m.applyLines(lines)
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

func (m *Model) setSnapshot(lines []string) {
	if m.stream != nil {
		m.stream.SetSnapshot(lines)
	}
	m.applyLines(lines)
}

func (m *Model) applyLines(lines []string) {
	m.viewport.SetContent(strings.Join(lines, "\n"))
	if m.follow {
		m.viewport.GotoBottom()
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
