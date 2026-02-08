package app

import (
	"context"
	"fmt"
	"math"
	"sort"
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
	viewportScrollbarWidth    = 1
	minListWidth              = 24
	maxListWidth              = 40
	minViewportWidth          = 20
	minContentHeight          = 6
	statusLinePadding         = 1
)

type uiMode int

const (
	uiModeNormal uiMode = iota
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
	workspaceAPI        WorkspaceAPI
	sessionAPI          SessionAPI
	stateAPI            StateAPI
	sidebar             *SidebarController
	viewport            viewport.Model
	mode                uiMode
	addWorkspace        *AddWorkspaceController
	addWorktree         *AddWorktreeController
	providerPicker      *ProviderPicker
	compose             *ComposeController
	chatInput           *ChatInput
	searchInput         *ChatInput
	renameInput         *ChatInput
	groupInput          *ChatInput
	groupPicker         *GroupPicker
	workspacePicker     *SelectPicker
	groupSelectPicker   *SelectPicker
	workspaceMulti      *MultiSelectPicker
	renameWorkspaceID   string
	editWorkspaceID     string
	renameGroupID       string
	assignGroupID       string
	status              string
	width               int
	height              int
	follow              bool
	workspaces          []*types.Workspace
	groups              []*types.WorkspaceGroup
	worktrees           map[string][]*types.Worktree
	sessions            []*types.Session
	sessionMeta         map[string]*types.SessionMeta
	appState            types.AppState
	hasAppState         bool
	stream              *StreamController
	codexStream         *CodexStreamController
	itemStream          *ItemStreamController
	input               *InputController
	chat                *SessionChatController
	pendingApproval     *ApprovalRequest
	contentRaw          string
	contentEsc          bool
	contentBlocks       []ChatBlock
	contentBlockSpans   []renderedBlockSpan
	reasoningExpanded   map[string]bool
	renderedText        string
	renderedLines       []string
	renderedPlain       []string
	contentVersion      int
	renderVersion       int
	renderedForWidth    int
	renderedForContent  int
	searchQuery         string
	searchMatches       []int
	searchIndex         int
	searchVersion       int
	sectionOffsets      []int
	sectionVersion      int
	transcriptCache     map[string][]ChatBlock
	pendingSessionKey   string
	loading             bool
	loadingKey          string
	loader              spinner.Model
	pendingMouseCmd     tea.Cmd
	lastSidebarWheelAt  time.Time
	pendingSidebarWheel bool
	sidebarDragging     bool
	menu                *MenuController
	hotkeys             *HotkeyRenderer
	contextMenu         *ContextMenuController
	confirm             *ConfirmController
	newSession          *newSessionTarget
	pendingSelectID     string
	selectSeq           int
	sendSeq             int
	pendingSends        map[int]pendingSend
	composeHistory      map[string]*composeHistoryState
	tickFn              func() tea.Cmd
	pendingConfirm      confirmAction
	scrollOnLoad        bool
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
		workspaceAPI:      api,
		sessionAPI:        api,
		stateAPI:          api,
		sidebar:           NewSidebarController(),
		viewport:          vp,
		stream:            stream,
		codexStream:       codexStream,
		itemStream:        itemStream,
		input:             NewInputController(),
		chat:              NewSessionChatController(api, codexStream),
		mode:              uiModeNormal,
		addWorkspace:      NewAddWorkspaceController(minViewportWidth),
		addWorktree:       NewAddWorktreeController(minViewportWidth),
		providerPicker:    NewProviderPicker(minViewportWidth, minContentHeight-1),
		compose:           NewComposeController(minViewportWidth),
		chatInput:         NewChatInput(minViewportWidth, DefaultChatInputConfig()),
		searchInput:       NewChatInput(minViewportWidth, ChatInputConfig{Height: 1}),
		renameInput:       NewChatInput(minViewportWidth, ChatInputConfig{Height: 1}),
		groupInput:        NewChatInput(minViewportWidth, ChatInputConfig{Height: 1}),
		groupPicker:       NewGroupPicker(minViewportWidth, minContentHeight-1),
		workspacePicker:   NewSelectPicker(minViewportWidth, minContentHeight-1),
		groupSelectPicker: NewSelectPicker(minViewportWidth, minContentHeight-1),
		workspaceMulti:    NewMultiSelectPicker(minViewportWidth, minContentHeight-1),
		status:            "",
		follow:            true,
		groups:            []*types.WorkspaceGroup{},
		worktrees:         map[string][]*types.Worktree{},
		sessionMeta:       map[string]*types.SessionMeta{},
		contentRaw:        "No sessions.",
		contentEsc:        false,
		searchIndex:       -1,
		searchVersion:     -1,
		sectionVersion:    -1,
		transcriptCache:   map[string][]ChatBlock{},
		reasoningExpanded: map[string]bool{},
		loader:            loader,
		hotkeys:           hotkeyRenderer,
		pendingSends:      map[int]pendingSend{},
		composeHistory:    map[string]*composeHistoryState{},
		menu:              NewMenuController(),
		contextMenu:       NewContextMenuController(),
		confirm:           NewConfirmController(),
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
	case workspaceGroupsMsg:
		if msg.err != nil {
			m.status = "workspace groups error: " + msg.err.Error()
			return m, nil
		}
		m.groups = msg.groups
		if m.menu != nil {
			previous := m.menu.SelectedGroupIDs()
			m.menu.SetGroups(msg.groups)
			if m.handleMenuGroupChange(previous) {
				return m, m.saveAppStateCmd()
			}
		}
		return m, nil
	case createWorkspaceGroupMsg:
		if msg.err != nil {
			m.exitAddWorkspaceGroup("add group error: " + msg.err.Error())
			return m, nil
		}
		m.exitAddWorkspaceGroup("group added")
		return m, fetchWorkspaceGroupsCmd(m.workspaceAPI)
	case updateWorkspaceGroupMsg:
		if msg.err != nil {
			m.status = "update group error: " + msg.err.Error()
			return m, nil
		}
		m.status = "group updated"
		return m, fetchWorkspaceGroupsCmd(m.workspaceAPI)
	case deleteWorkspaceGroupMsg:
		if msg.err != nil {
			m.status = "delete group error: " + msg.err.Error()
			return m, nil
		}
		m.status = "group deleted"
		return m, tea.Batch(fetchWorkspaceGroupsCmd(m.workspaceAPI), fetchWorkspacesCmd(m.workspaceAPI))
	case assignGroupWorkspacesMsg:
		if msg.err != nil {
			m.status = "assign groups error: " + msg.err.Error()
			return m, nil
		}
		m.status = fmt.Sprintf("updated %d workspaces", msg.updated)
		return m, fetchWorkspacesCmd(m.workspaceAPI)
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
	case worktreeDeletedMsg:
		if msg.err != nil {
			m.status = "delete worktree error: " + msg.err.Error()
			return m, nil
		}
		if msg.worktreeID != "" && msg.worktreeID == m.appState.ActiveWorktreeID {
			m.appState.ActiveWorktreeID = ""
			m.hasAppState = true
		}
		m.status = "worktree deleted"
		cmds := []tea.Cmd{fetchSessionsWithMetaCmd(m.sessionAPI)}
		if msg.workspaceID != "" {
			cmds = append(cmds, fetchWorktreesCmd(m.workspaceAPI, msg.workspaceID))
		}
		return m, tea.Batch(cmds...)
	case updateWorkspaceMsg:
		if msg.err != nil {
			m.status = "update workspace error: " + msg.err.Error()
			return m, nil
		}
		m.status = "workspace updated"
		return m, tea.Batch(fetchWorkspacesCmd(m.workspaceAPI), fetchWorkspaceGroupsCmd(m.workspaceAPI), fetchSessionsWithMetaCmd(m.sessionAPI))
	case deleteWorkspaceMsg:
		if msg.err != nil {
			m.status = "delete workspace error: " + msg.err.Error()
			return m, nil
		}
		if msg.id != "" && msg.id == m.appState.ActiveWorkspaceID {
			m.appState.ActiveWorkspaceID = ""
			m.appState.ActiveWorktreeID = ""
			m.hasAppState = true
		}
		m.status = "workspace deleted"
		return m, tea.Batch(fetchWorkspacesCmd(m.workspaceAPI), fetchSessionsWithMetaCmd(m.sessionAPI), m.saveAppStateCmd())
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

	if m.mode == uiModeAddWorkspace {
		switch msg := msg.(type) {
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
		}
		if m.addWorkspace == nil {
			return m, nil
		}
		_, cmd := m.addWorkspace.Update(msg, m)
		return m, cmd
	}
	if m.mode == uiModeAddWorktree {
		switch msg := msg.(type) {
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
		}
		if m.addWorktree == nil {
			return m, nil
		}
		_, cmd := m.addWorktree.Update(msg, m)
		return m, cmd
	}
	if m.mode == uiModeRenameWorkspace {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
			case "esc":
				m.exitRenameWorkspace("rename canceled")
				return m, nil
			case "enter":
				if m.renameInput != nil {
					name := strings.TrimSpace(m.renameInput.Value())
					if name == "" {
						m.status = "name is required"
						return m, nil
					}
					id := m.renameWorkspaceID
					if id == "" {
						m.status = "no workspace selected"
						return m, nil
					}
					m.renameInput.SetValue("")
					m.exitRenameWorkspace("renaming workspace")
					return m, updateWorkspaceCmd(m.workspaceAPI, id, name)
				}
				return m, nil
			}
			if m.renameInput != nil {
				cmd := m.renameInput.Update(msg)
				return m, cmd
			}
		}
		return m, nil
	}
	if m.mode == uiModeAddWorkspaceGroup {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
			case "esc":
				m.exitAddWorkspaceGroup("add group canceled")
				return m, nil
			case "enter":
				if m.groupInput != nil {
					name := strings.TrimSpace(m.groupInput.Value())
					if name == "" {
						m.status = "name is required"
						return m, nil
					}
					m.groupInput.SetValue("")
					m.exitAddWorkspaceGroup("creating group")
					return m, createWorkspaceGroupCmd(m.workspaceAPI, name)
				}
				return m, nil
			}
			if m.groupInput != nil {
				cmd := m.groupInput.Update(msg)
				return m, cmd
			}
		}
		return m, nil
	}
	if m.mode == uiModePickWorkspaceRename || m.mode == uiModePickWorkspaceGroupEdit {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
			case "esc":
				m.exitWorkspacePicker("selection canceled")
				return m, nil
			case "enter":
				id := ""
				if m.workspacePicker != nil {
					id = m.workspacePicker.SelectedID()
				}
				if id == "" {
					m.status = "no workspace selected"
					return m, nil
				}
				if m.mode == uiModePickWorkspaceRename {
					m.enterRenameWorkspace(id)
					return m, nil
				}
				m.enterEditWorkspaceGroups(id)
				return m, nil
			case "j", "down":
				if m.workspacePicker != nil {
					m.workspacePicker.Move(1)
				}
				return m, nil
			case "k", "up":
				if m.workspacePicker != nil {
					m.workspacePicker.Move(-1)
				}
				return m, nil
			}
		}
		return m, nil
	}
	if m.mode == uiModePickWorkspaceGroupRename {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
			case "esc":
				m.exitWorkspacePicker("selection canceled")
				return m, nil
			case "enter":
				id := ""
				if m.groupSelectPicker != nil {
					id = m.groupSelectPicker.SelectedID()
				}
				if id == "" {
					m.status = "no group selected"
					return m, nil
				}
				m.enterRenameWorkspaceGroup(id)
				return m, nil
			case "j", "down":
				if m.groupSelectPicker != nil {
					m.groupSelectPicker.Move(1)
				}
				return m, nil
			case "k", "up":
				if m.groupSelectPicker != nil {
					m.groupSelectPicker.Move(-1)
				}
				return m, nil
			}
		}
		return m, nil
	}
	if m.mode == uiModePickWorkspaceGroupDelete {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
			case "esc":
				m.exitWorkspacePicker("selection canceled")
				return m, nil
			case "enter":
				id := ""
				if m.groupSelectPicker != nil {
					id = m.groupSelectPicker.SelectedID()
				}
				if id == "" {
					m.status = "no group selected"
					return m, nil
				}
				m.confirmDeleteWorkspaceGroup(id)
				return m, nil
			case "j", "down":
				if m.groupSelectPicker != nil {
					m.groupSelectPicker.Move(1)
				}
				return m, nil
			case "k", "up":
				if m.groupSelectPicker != nil {
					m.groupSelectPicker.Move(-1)
				}
				return m, nil
			}
		}
		return m, nil
	}
	if m.mode == uiModePickWorkspaceGroupAssign {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
			case "esc":
				m.exitWorkspacePicker("selection canceled")
				return m, nil
			case "enter":
				id := ""
				if m.groupSelectPicker != nil {
					id = m.groupSelectPicker.SelectedID()
				}
				if id == "" {
					m.status = "no group selected"
					return m, nil
				}
				m.enterAssignGroupWorkspaces(id)
				return m, nil
			case "j", "down":
				if m.groupSelectPicker != nil {
					m.groupSelectPicker.Move(1)
				}
				return m, nil
			case "k", "up":
				if m.groupSelectPicker != nil {
					m.groupSelectPicker.Move(-1)
				}
				return m, nil
			}
		}
		return m, nil
	}
	if m.mode == uiModePickWorkspaceGroupAssign {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
			case "esc":
				m.exitWorkspacePicker("selection canceled")
				return m, nil
			case "enter":
				id := ""
				if m.groupSelectPicker != nil {
					id = m.groupSelectPicker.SelectedID()
				}
				if id == "" {
					m.status = "no group selected"
					return m, nil
				}
				m.enterAssignGroupWorkspaces(id)
				return m, nil
			case "j", "down":
				if m.groupSelectPicker != nil {
					m.groupSelectPicker.Move(1)
				}
				return m, nil
			case "k", "up":
				if m.groupSelectPicker != nil {
					m.groupSelectPicker.Move(-1)
				}
				return m, nil
			}
		}
		return m, nil
	}
	if m.mode == uiModeEditWorkspaceGroups {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
			case "esc":
				m.exitEditWorkspaceGroups("edit canceled")
				return m, nil
			case "enter":
				if m.groupPicker != nil {
					ids := m.groupPicker.SelectedIDs()
					id := m.editWorkspaceID
					if id == "" {
						m.status = "no workspace selected"
						return m, nil
					}
					m.exitEditWorkspaceGroups("saving groups")
					return m, updateWorkspaceGroupsCmd(m.workspaceAPI, id, ids)
				}
				return m, nil
			case " ", "space":
				if m.groupPicker != nil && m.groupPicker.Toggle() {
					return m, nil
				}
			case "j", "down":
				if m.groupPicker != nil && m.groupPicker.Move(1) {
					return m, nil
				}
			case "k", "up":
				if m.groupPicker != nil && m.groupPicker.Move(-1) {
					return m, nil
				}
			}
			if m.groupPicker != nil {
				return m, nil
			}
		}
		return m, nil
	}
	if m.mode == uiModeRenameWorkspaceGroup {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
			case "esc":
				m.exitRenameWorkspaceGroup("rename canceled")
				return m, nil
			case "enter":
				if m.groupInput != nil {
					name := strings.TrimSpace(m.groupInput.Value())
					if name == "" {
						m.status = "name is required"
						return m, nil
					}
					id := m.renameGroupID
					if id == "" {
						m.status = "no group selected"
						return m, nil
					}
					m.groupInput.SetValue("")
					m.exitRenameWorkspaceGroup("renaming group")
					return m, updateWorkspaceGroupCmd(m.workspaceAPI, id, name)
				}
				return m, nil
			}
			if m.groupInput != nil {
				cmd := m.groupInput.Update(msg)
				return m, cmd
			}
		}
		return m, nil
	}
	if m.mode == uiModeAssignGroupWorkspaces {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
			case "esc":
				m.exitAssignGroupWorkspaces("assignment canceled")
				return m, nil
			case "enter":
				if m.workspaceMulti != nil {
					ids := m.workspaceMulti.SelectedIDs()
					groupID := m.assignGroupID
					if groupID == "" {
						m.status = "no group selected"
						return m, nil
					}
					m.exitAssignGroupWorkspaces("saving assignments")
					return m, assignGroupWorkspacesCmd(m.workspaceAPI, groupID, ids, m.workspaces)
				}
				return m, nil
			case " ", "space":
				if m.workspaceMulti != nil && m.workspaceMulti.Toggle() {
					return m, nil
				}
			case "j", "down":
				if m.workspaceMulti != nil && m.workspaceMulti.Move(1) {
					return m, nil
				}
			case "k", "up":
				if m.workspaceMulti != nil && m.workspaceMulti.Move(-1) {
					return m, nil
				}
			}
		}
		return m, nil
	}
	if m.mode == uiModeAssignGroupWorkspaces {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
			case "esc":
				m.exitAssignGroupWorkspaces("assignment canceled")
				return m, nil
			case "enter":
				if m.workspaceMulti != nil {
					ids := m.workspaceMulti.SelectedIDs()
					groupID := m.assignGroupID
					if groupID == "" {
						m.status = "no group selected"
						return m, nil
					}
					m.exitAssignGroupWorkspaces("saving assignments")
					return m, assignGroupWorkspacesCmd(m.workspaceAPI, groupID, ids, m.workspaces)
				}
				return m, nil
			case " ", "space":
				if m.workspaceMulti != nil && m.workspaceMulti.Toggle() {
					return m, nil
				}
			case "j", "down":
				if m.workspaceMulti != nil && m.workspaceMulti.Move(1) {
					return m, nil
				}
			case "k", "up":
				if m.workspaceMulti != nil && m.workspaceMulti.Move(-1) {
					return m, nil
				}
			}
		}
		return m, nil
	}
	if m.mode == uiModePickProvider {
		switch msg := msg.(type) {
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
		}
		return m, nil
	}
	if m.menu != nil && m.menu.IsActive() {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			previous := m.menu.SelectedGroupIDs()
			if handled, action := m.menu.HandleKey(msg); handled {
				cmds := []tea.Cmd{}
				if cmd := m.handleMenuAction(action); cmd != nil {
					cmds = append(cmds, cmd)
				}
				if m.handleMenuGroupChange(previous) {
					cmds = append(cmds, m.saveAppStateCmd())
				}
				return m, tea.Batch(cmds...)
			}
		}
	}
	if m.mode == uiModeCompose {
		switch msg := msg.(type) {
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
			case "up":
				if m.chatInput != nil {
					if value, ok := m.composeHistoryNavigate(-1, m.chatInput.Value()); ok {
						m.chatInput.SetValue(value)
						return m, nil
					}
				}
				return m, nil
			case "down":
				if m.chatInput != nil {
					if value, ok := m.composeHistoryNavigate(1, m.chatInput.Value()); ok {
						m.chatInput.SetValue(value)
						return m, nil
					}
				}
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
					m.recordComposeHistory(sessionID, text)
					saveHistoryCmd := m.saveAppStateCmd()
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
						cmds := []tea.Cmd{send}
						if m.itemStream == nil || !m.itemStream.HasStream() {
							cmds = append([]tea.Cmd{openItemsCmd(m.sessionAPI, sessionID)}, cmds...)
						}
						key := m.pendingSessionKey
						if key == "" {
							key = m.selectedKey()
						}
						if key != "" {
							cmds = append(cmds, historyPollCmd(sessionID, key, 0, historyPollDelay, countAgentRepliesBlocks(m.currentBlocks())))
						}
						if saveHistoryCmd != nil {
							cmds = append(cmds, saveHistoryCmd)
						}
						return m, tea.Batch(cmds...)
					}
					if provider == "codex" {
						if m.codexStream == nil || !m.codexStream.HasStream() {
							if saveHistoryCmd != nil {
								return m, tea.Batch(openEventsCmd(m.sessionAPI, sessionID), send, saveHistoryCmd)
							}
							return m, tea.Batch(openEventsCmd(m.sessionAPI, sessionID), send)
						}
						if saveHistoryCmd != nil {
							return m, tea.Batch(send, saveHistoryCmd)
						}
						return m, send
					}
					if saveHistoryCmd != nil {
						return m, tea.Batch(send, saveHistoryCmd)
					}
					return m, send
				}
				return m, nil
			case "ctrl+c":
				if m.chatInput != nil {
					m.chatInput.Clear()
					m.status = "input cleared"
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
				m.resetComposeHistoryCursor()
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
		case "m":
			if m.menu != nil {
				if m.contextMenu != nil {
					m.contextMenu.Close()
				}
				m.menu.Toggle()
			}
			return m, nil
		case "esc":
			return m, nil
		case "q":
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
			m.confirmDismissSessions(ids)
			return m, nil
		case "p":
			m.follow = !m.follow
			if m.follow {
				m.viewport.GotoBottom()
				m.status = "follow: on"
			} else {
				m.status = "follow: paused"
			}
			return m, nil
		case "e":
			if m.toggleVisibleReasoning() {
				return m, nil
			}
			m.status = "no reasoning in view"
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
		blocks := itemsToBlocks(msg.items)
		if shouldStreamItems(m.selectedSessionProvider()) && m.itemStream != nil {
			m.itemStream.SetSnapshotBlocks(blocks)
			blocks = m.itemStream.Blocks()
		}
		m.setSnapshotBlocks(blocks)
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
		blocks := itemsToBlocks(msg.items)
		if shouldStreamItems(m.selectedSessionProvider()) && m.itemStream != nil {
			m.itemStream.SetSnapshotBlocks(blocks)
			blocks = m.itemStream.Blocks()
		}
		m.setSnapshotBlocks(blocks)
		if msg.key != "" {
			m.transcriptCache[msg.key] = blocks
		}
		m.status = "history updated"
		return m, nil
	case historyPollMsg:
		if msg.id == "" || msg.key == "" {
			return m, nil
		}
		if msg.attempt >= historyPollMax {
			return m, nil
		}
		if m.mode != uiModeCompose {
			return m, nil
		}
		targetID := m.composeSessionID()
		if targetID == "" {
			targetID = m.selectedSessionID()
		}
		if targetID != msg.id {
			return m, nil
		}
		currentAgents := countAgentRepliesBlocks(m.currentBlocks())
		if msg.minAgents >= 0 {
			if currentAgents > msg.minAgents {
				return m, nil
			}
		} else if currentAgents > 0 {
			return m, nil
		}
		cmds := []tea.Cmd{fetchHistoryCmd(m.sessionAPI, msg.id, msg.key, maxViewportLines)}
		cmds = append(cmds, historyPollCmd(msg.id, msg.key, msg.attempt+1, historyPollDelay, msg.minAgents))
		return m, tea.Batch(cmds...)
	case sendMsg:
		if msg.err != nil {
			m.status = "send error: " + msg.err.Error()
			m.markPendingSendFailed(msg.token, msg.err)
			return m, nil
		}
		m.status = "message sent"
		m.clearPendingSend(msg.token)
		provider := m.providerForSessionID(msg.id)
		if shouldStreamItems(provider) && m.itemStream != nil && !m.itemStream.HasStream() {
			return m, openItemsCmd(m.sessionAPI, msg.id)
		}
		if provider == "codex" && m.codexStream != nil && !m.codexStream.HasStream() {
			return m, openEventsCmd(m.sessionAPI, msg.id)
		}
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
		key := "sess:" + msg.session.ID
		m.pendingSessionKey = key
		m.status = "session started"
		cmds := []tea.Cmd{fetchSessionsWithMetaCmd(m.sessionAPI), fetchHistoryCmd(m.sessionAPI, msg.session.ID, key, maxViewportLines)}
		if shouldStreamItems(msg.session.Provider) {
			cmds = append(cmds, openItemsCmd(m.sessionAPI, msg.session.ID))
		} else if msg.session.Provider == "codex" {
			cmds = append(cmds, openEventsCmd(m.sessionAPI, msg.session.ID))
		} else if isActiveStatus(msg.session.Status) {
			cmds = append(cmds, openStreamCmd(m.sessionAPI, msg.session.ID))
		}
		if msg.session.Provider == "codex" {
			cmds = append(cmds, historyPollCmd(msg.session.ID, key, 0, historyPollDelay, countAgentRepliesBlocks(m.currentBlocks())))
		}
		return m, tea.Batch(cmds...)
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
			m.itemStream.SetSnapshotBlocks(m.currentBlocks())
			m.itemStream.SetStream(msg.ch, msg.cancel)
		}
		m.status = "streaming items"
		return m, nil
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
	} else if m.mode == uiModeAddWorkspaceGroup {
		headerText = "Add Workspace Group"
		if m.groupInput != nil {
			bodyText = m.groupInput.View()
		}
	} else if m.mode == uiModePickWorkspaceRename {
		headerText = "Select Workspace"
		if m.workspacePicker != nil {
			bodyText = m.workspacePicker.View()
		}
	} else if m.mode == uiModePickWorkspaceGroupEdit {
		headerText = "Select Workspace"
		if m.workspacePicker != nil {
			bodyText = m.workspacePicker.View()
		}
	} else if m.mode == uiModePickWorkspaceGroupRename {
		headerText = "Select Group"
		if m.groupSelectPicker != nil {
			bodyText = m.groupSelectPicker.View()
		}
	} else if m.mode == uiModePickWorkspaceGroupDelete {
		headerText = "Select Group"
		if m.groupSelectPicker != nil {
			bodyText = m.groupSelectPicker.View()
		}
	} else if m.mode == uiModePickWorkspaceGroupAssign {
		headerText = "Select Group"
		if m.groupSelectPicker != nil {
			bodyText = m.groupSelectPicker.View()
		}
	} else if m.mode == uiModeEditWorkspaceGroups {
		headerText = "Edit Workspace Groups"
		if m.groupPicker != nil {
			bodyText = m.groupPicker.View()
		}
	} else if m.mode == uiModeRenameWorkspaceGroup {
		headerText = "Rename Workspace Group"
		if m.groupInput != nil {
			bodyText = m.groupInput.View()
		}
	} else if m.mode == uiModeAssignGroupWorkspaces {
		headerText = "Assign Workspaces"
		if m.workspaceMulti != nil {
			bodyText = m.workspaceMulti.View()
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
	} else if m.mode == uiModeRenameWorkspace {
		headerText = "Rename Workspace"
		if m.renameInput != nil {
			bodyText = m.renameInput.View()
		}
	}
	rightHeader := headerStyle.Render(headerText)
	rightBody := bodyText
	if m.usesViewport() {
		scrollbar := m.viewportScrollbarView()
		if scrollbar != "" {
			rightBody = lipgloss.JoinHorizontal(lipgloss.Top, bodyText, scrollbar)
		}
	}
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
	menuBar := ""
	if m.menu != nil {
		menuBar = m.menu.MenuBarView(m.width)
	}
	body = overlayLine(body, menuBar, 0)
	if m.menu != nil && m.menu.IsDropdownOpen() {
		menuDrop := m.menu.DropdownView(m.sidebarWidth())
		if menuDrop != "" {
			if m.menu.HasSubmenu() {
				submenu := m.menu.SubmenuView(0)
				combined := combineBlocks(menuDrop, submenu, 1)
				body = overlayBlock(body, combined, 1)
			} else {
				body = overlayBlock(body, menuDrop, 1)
			}
		}
	}
	if m.contextMenu != nil && m.contextMenu.IsOpen() {
		bodyHeight := len(strings.Split(body, "\n"))
		menuBlock, row := m.contextMenu.View(m.width, bodyHeight)
		if menuBlock != "" {
			body = overlayBlock(body, menuBlock, row)
		}
	}
	if m.confirm != nil && m.confirm.IsOpen() {
		bodyHeight := len(strings.Split(body, "\n"))
		confirmBlock, row := m.confirm.View(m.width, bodyHeight)
		if confirmBlock != "" {
			body = overlayBlock(body, confirmBlock, row)
		}
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
	m.status = "loading " + id
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
			m.status = "select a workspace to delete"
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
	targetID := m.contextMenu.TargetID()
	targetWorkspaceID := m.contextMenu.WorkspaceID()
	targetWorktreeID := m.contextMenu.WorktreeID()
	targetSessionID := m.contextMenu.SessionID()
	m.contextMenu.Close()
	switch action {
	case ContextMenuWorkspaceCreate:
		m.enterAddWorkspace()
	case ContextMenuWorkspaceRename:
		if targetID == "" {
			m.status = "select a workspace to rename"
			return nil
		}
		m.enterRenameWorkspace(targetID)
	case ContextMenuWorkspaceEditGroups:
		if targetID == "" {
			m.status = "select a workspace"
			return nil
		}
		m.enterEditWorkspaceGroups(targetID)
	case ContextMenuWorkspaceDelete:
		if targetID == "" || targetID == unassignedWorkspaceID {
			m.status = "select a workspace to delete"
			return nil
		}
		m.confirmDeleteWorkspace(targetID)
	case ContextMenuWorktreeAdd:
		if targetWorkspaceID == "" {
			m.status = "select a workspace"
			return nil
		}
		m.enterAddWorktree(targetWorkspaceID)
	case ContextMenuWorktreeDelete:
		if targetWorktreeID == "" || targetWorkspaceID == "" {
			m.status = "select a worktree"
			return nil
		}
		m.confirmDeleteWorktree(targetWorkspaceID, targetWorktreeID)
	case ContextMenuSessionChat:
		if targetSessionID == "" {
			m.status = "select a session"
			return nil
		}
		m.enterCompose(targetSessionID)
	case ContextMenuSessionDismiss:
		if targetSessionID == "" {
			m.status = "select a session"
			return nil
		}
		m.confirmDismissSessions([]string{targetSessionID})
	case ContextMenuSessionKill:
		if targetSessionID == "" {
			m.status = "select a session"
			return nil
		}
		m.status = "killing " + targetSessionID
		return killSessionCmd(m.sessionAPI, targetSessionID)
	case ContextMenuSessionInterrupt:
		if targetSessionID == "" {
			m.status = "select a session"
			return nil
		}
		m.status = "interrupting " + targetSessionID
		return interruptSessionCmd(m.sessionAPI, targetSessionID)
	case ContextMenuSessionCopyID:
		if targetSessionID == "" {
			m.status = "select a session"
			return nil
		}
		if err := clipboard.WriteAll(targetSessionID); err != nil {
			m.status = "copy failed: " + err.Error()
			return nil
		}
		m.status = "copied session id"
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
				m.status = "select a workspace to delete"
				return nil
			}
			m.status = "deleting workspace"
			return deleteWorkspaceCmd(m.workspaceAPI, action.workspaceID)
		case confirmDeleteWorkspaceGroup:
			if action.groupID == "" {
				m.status = "select a group to delete"
				return nil
			}
			m.status = "deleting group"
			return deleteWorkspaceGroupCmd(m.workspaceAPI, action.groupID)
		case confirmDeleteWorktree:
			if action.workspaceID == "" || action.worktreeID == "" {
				m.status = "select a worktree to delete"
				return nil
			}
			m.status = "deleting worktree"
			return deleteWorktreeCmd(m.workspaceAPI, action.workspaceID, action.worktreeID)
		case confirmDismissSessions:
			if len(action.sessionIDs) == 0 {
				m.status = "no session selected"
				return nil
			}
			if len(action.sessionIDs) == 1 {
				m.status = "marking exited " + action.sessionIDs[0]
				return markExitedCmd(m.sessionAPI, action.sessionIDs[0])
			}
			m.status = fmt.Sprintf("marking exited %d sessions", len(action.sessionIDs))
			return markExitedManyCmd(m.sessionAPI, action.sessionIDs)
		}
		m.status = "confirmed"
		return nil
	case confirmChoiceCancel:
		m.status = "canceled"
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
	m.consumeStreamTick()
	m.consumeCodexTick()
	m.consumeItemTick()
	if m.loading {
		m.loader, _ = m.loader.Update(spinner.TickMsg{Time: time.Time(msg), ID: m.loader.ID()})
		m.setLoadingContent()
	}
	if m.pendingSidebarWheel {
		if time.Since(m.lastSidebarWheelAt) >= sidebarWheelSettle {
			m.pendingSidebarWheel = false
			if cmd := m.onSelectionChangedImmediate(); cmd != nil {
				return tea.Batch(cmd, m.tickCmd())
			}
		}
	}
	return m.tickCmd()
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
	changed, closed := m.codexStream.ConsumeTick()
	if closed {
		m.status = "events closed"
	}
	if changed {
		m.applyBlocks(m.codexStream.Blocks())
	}
	if errMsg := m.codexStream.LastError(); errMsg != "" {
		m.status = "codex error: " + errMsg
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
	changed, closed := m.itemStream.ConsumeTick()
	if closed {
		m.status = "items stream closed"
	}
	if changed {
		m.applyBlocks(m.itemStream.Blocks())
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
		for i := range resolved {
			if resolved[i].Role != ChatRoleReasoning {
				continue
			}
			if strings.TrimSpace(resolved[i].ID) == "" {
				resolved[i].ID = makeChatBlockID(resolved[i].Role, i, resolved[i].Text)
			}
			if m.reasoningExpanded != nil && m.reasoningExpanded[resolved[i].ID] {
				resolved[i].Collapsed = false
				continue
			}
			resolved[i].Collapsed = true
		}
		m.contentBlocks = resolved
	}
	m.contentRaw = ""
	m.contentEsc = false
	m.contentVersion++
	m.searchVersion = -1
	m.sectionVersion = -1
	m.renderViewport()
}

func (m *Model) usesViewport() bool {
	switch m.mode {
	case uiModeNormal, uiModeCompose, uiModeSearch:
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
	needsRender := m.renderedForWidth != renderWidth || m.renderedForContent != m.contentVersion
	if needsRender {
		var rendered string
		if m.contentBlocks != nil {
			var spans []renderedBlockSpan
			rendered, spans = renderChatBlocks(m.contentBlocks, renderWidth, maxViewportLines)
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
	m.renderViewport()
	m.persistCurrentBlocks()
	if block.Collapsed {
		m.status = "reasoning collapsed"
	} else {
		m.status = "reasoning expanded"
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
	barWidth := 0
	if m.sidebar != nil {
		barWidth = m.sidebar.ScrollbarWidth()
	}
	barStart := listWidth - barWidth
	if barStart < 0 {
		barStart = 0
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

	if m.contextMenu != nil && m.contextMenu.IsOpen() && msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
		if handled, action := m.contextMenu.HandleMouse(msg, m.width, m.height-1); handled {
			if action != ContextMenuNone {
				if cmd := m.handleContextMenuAction(action); cmd != nil {
					m.pendingMouseCmd = cmd
				}
			}
			return true
		}
		if !m.contextMenu.Contains(msg.X, msg.Y, m.width, m.height-1) {
			m.contextMenu.Close()
		}
	}
	if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonRight {
		if listWidth > 0 && msg.X < listWidth && m.sidebar != nil {
			if entry := m.sidebar.ItemAtRow(msg.Y); entry != nil {
				if m.menu != nil {
					m.menu.CloseAll()
				}
				if m.contextMenu != nil {
					switch entry.kind {
					case sidebarWorkspace:
						if entry.workspace != nil {
							m.contextMenu.OpenWorkspace(entry.workspace.ID, entry.workspace.Name, msg.X, msg.Y)
							return true
						}
					case sidebarWorktree:
						if entry.worktree != nil {
							m.contextMenu.OpenWorktree(entry.worktree.ID, entry.worktree.WorkspaceID, entry.worktree.Name, msg.X, msg.Y)
							return true
						}
					case sidebarSession:
						if entry.session != nil {
							m.contextMenu.OpenSession(entry.session.ID, entry.Title(), msg.X, msg.Y)
							return true
						}
					}
				}
			}
		}
		if m.contextMenu != nil && m.contextMenu.IsOpen() {
			m.contextMenu.Close()
			return true
		}
	}

	if msg.Action == tea.MouseActionRelease {
		m.sidebarDragging = false
	}
	if msg.Action == tea.MouseActionMotion && m.sidebarDragging {
		if listWidth > 0 && msg.X < listWidth && barWidth > 0 && msg.X >= barStart {
			if m.sidebar != nil && m.sidebar.ScrollbarSelect(msg.Y) {
				m.lastSidebarWheelAt = time.Now()
				m.pendingSidebarWheel = true
				return true
			}
		}
		return true
	}

	if msg.Action == tea.MouseActionPress {
		switch msg.Button {
		case tea.MouseButtonWheelUp:
			if listWidth > 0 && msg.X < listWidth {
				now := time.Now()
				if now.Sub(m.lastSidebarWheelAt) < sidebarWheelCooldown {
					return true
				}
				m.lastSidebarWheelAt = now
				if m.sidebar != nil && m.sidebar.Scroll(-1) {
					m.pendingSidebarWheel = true
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
			if m.mode == uiModeEditWorkspaceGroups && m.groupPicker != nil && msg.X >= rightStart {
				if m.groupPicker.Move(-1) {
					return true
				}
			}
			if (m.mode == uiModePickWorkspaceRename || m.mode == uiModePickWorkspaceGroupEdit) && m.workspacePicker != nil && msg.X >= rightStart {
				if m.workspacePicker.Move(-1) {
					return true
				}
			}
			if m.mode == uiModePickWorkspaceGroupRename && m.groupSelectPicker != nil && msg.X >= rightStart {
				if m.groupSelectPicker.Move(-1) {
					return true
				}
			}
			if m.mode == uiModePickWorkspaceGroupDelete && m.groupSelectPicker != nil && msg.X >= rightStart {
				if m.groupSelectPicker.Move(-1) {
					return true
				}
			}
			if m.mode == uiModePickWorkspaceGroupAssign && m.groupSelectPicker != nil && msg.X >= rightStart {
				if m.groupSelectPicker.Move(-1) {
					return true
				}
			}
			if m.mode == uiModeAssignGroupWorkspaces && m.workspaceMulti != nil && msg.X >= rightStart {
				if m.workspaceMulti.Move(-1) {
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
				now := time.Now()
				if now.Sub(m.lastSidebarWheelAt) < sidebarWheelCooldown {
					return true
				}
				m.lastSidebarWheelAt = now
				if m.sidebar != nil && m.sidebar.Scroll(1) {
					m.pendingSidebarWheel = true
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
			if m.mode == uiModeEditWorkspaceGroups && m.groupPicker != nil && msg.X >= rightStart {
				if m.groupPicker.Move(1) {
					return true
				}
			}
			if (m.mode == uiModePickWorkspaceRename || m.mode == uiModePickWorkspaceGroupEdit) && m.workspacePicker != nil && msg.X >= rightStart {
				if m.workspacePicker.Move(1) {
					return true
				}
			}
			if m.mode == uiModePickWorkspaceGroupRename && m.groupSelectPicker != nil && msg.X >= rightStart {
				if m.groupSelectPicker.Move(1) {
					return true
				}
			}
			if m.mode == uiModePickWorkspaceGroupDelete && m.groupSelectPicker != nil && msg.X >= rightStart {
				if m.groupSelectPicker.Move(1) {
					return true
				}
			}
			if m.mode == uiModePickWorkspaceGroupAssign && m.groupSelectPicker != nil && msg.X >= rightStart {
				if m.groupSelectPicker.Move(1) {
					return true
				}
			}
			if m.mode == uiModeAssignGroupWorkspaces && m.workspaceMulti != nil && msg.X >= rightStart {
				if m.workspaceMulti.Move(1) {
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
	if m.menu != nil {
		menuWidth := m.sidebarWidth()
		if menuWidth <= 0 {
			menuWidth = max(minListWidth, minViewportWidth)
		}
		if handled, action := m.menu.HandleMouse(msg, menuWidth); handled {
			if cmd := m.handleMenuAction(action); cmd != nil {
				m.pendingMouseCmd = cmd
			}
			return true
		}
	}
	if listWidth > 0 && msg.X < listWidth && barWidth > 0 && msg.X >= barStart {
		if m.sidebar != nil && m.sidebar.ScrollbarSelect(msg.Y) {
			m.lastSidebarWheelAt = time.Now()
			m.pendingSidebarWheel = true
			m.sidebarDragging = true
			return true
		}
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
	if msg.X >= rightStart && (m.mode == uiModeNormal || m.mode == uiModeCompose) {
		if msg.Y >= 1 && msg.Y <= m.viewport.Height && !isOverInput(msg.Y) {
			if m.toggleReasoningByViewportLine(msg.Y - 1) {
				return true
			}
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
	if m.mode == uiModeEditWorkspaceGroups && m.groupPicker != nil {
		if msg.X >= rightStart {
			row := msg.Y - 1
			if row >= 0 && m.groupPicker.HandleClick(row) {
				return true
			}
		}
	}
	if (m.mode == uiModePickWorkspaceRename || m.mode == uiModePickWorkspaceGroupEdit) && m.workspacePicker != nil {
		if msg.X >= rightStart {
			row := msg.Y - 1
			if row >= 0 && m.workspacePicker.HandleClick(row) {
				id := m.workspacePicker.SelectedID()
				if id == "" {
					return true
				}
				if m.mode == uiModePickWorkspaceRename {
					m.enterRenameWorkspace(id)
				} else {
					m.enterEditWorkspaceGroups(id)
				}
				return true
			}
		}
	}
	if m.mode == uiModePickWorkspaceGroupRename && m.groupSelectPicker != nil {
		if msg.X >= rightStart {
			row := msg.Y - 1
			if row >= 0 && m.groupSelectPicker.HandleClick(row) {
				id := m.groupSelectPicker.SelectedID()
				if id == "" {
					return true
				}
				m.enterRenameWorkspaceGroup(id)
				return true
			}
		}
	}
	if m.mode == uiModePickWorkspaceGroupDelete && m.groupSelectPicker != nil {
		if msg.X >= rightStart {
			row := msg.Y - 1
			if row >= 0 && m.groupSelectPicker.HandleClick(row) {
				id := m.groupSelectPicker.SelectedID()
				if id == "" {
					return true
				}
				m.confirmDeleteWorkspaceGroup(id)
				return true
			}
		}
	}
	if m.mode == uiModePickWorkspaceGroupAssign && m.groupSelectPicker != nil {
		if msg.X >= rightStart {
			row := msg.Y - 1
			if row >= 0 && m.groupSelectPicker.HandleClick(row) {
				id := m.groupSelectPicker.SelectedID()
				if id == "" {
					return true
				}
				m.enterAssignGroupWorkspaces(id)
				return true
			}
		}
	}
	if m.mode == uiModeAssignGroupWorkspaces && m.workspaceMulti != nil {
		if msg.X >= rightStart {
			row := msg.Y - 1
			if row >= 0 && m.workspaceMulti.HandleClick(row) {
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
	m.status = "add workspace group"
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
		m.status = status
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
	m.status = "select workspace"
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
	m.status = "select group"
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
	m.status = "select group"
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
	m.status = "select group"
}

func (m *Model) exitWorkspacePicker(status string) {
	m.mode = uiModeNormal
	if status != "" {
		m.status = status
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
	m.status = "edit workspace groups"
}

func (m *Model) exitEditWorkspaceGroups(status string) {
	m.mode = uiModeNormal
	m.editWorkspaceID = ""
	if status != "" {
		m.status = status
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
	m.status = "rename workspace group"
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
		m.status = status
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
	m.status = "assign workspaces"
}

func (m *Model) exitAssignGroupWorkspaces(status string) {
	m.mode = uiModeNormal
	m.assignGroupID = ""
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
	m.status = "rename workspace"
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
		m.status = status
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
