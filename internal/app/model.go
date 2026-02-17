package app

import (
	"context"
	"fmt"
	"log"
	"math"
	"sort"
	"strings"
	"time"

	"charm.land/bubbles/v2/spinner"
	"charm.land/bubbles/v2/viewport"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	xansi "github.com/charmbracelet/x/ansi"

	"control/internal/client"
	"control/internal/config"
	"control/internal/providers"
	"control/internal/types"
)

const (
	defaultTailLines          = 200
	maxViewportLines          = 2000
	maxEventsPerTick          = 64
	tickInterval              = 100 * time.Millisecond
	sidebarWheelCooldown      = 30 * time.Millisecond
	sidebarWheelSettle        = 120 * time.Millisecond
	appStateSaveDebounce      = 250 * time.Millisecond
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
	sessionMetaRefreshDelay   = 15 * time.Second
	sessionMetaSyncDelay      = 2 * time.Minute
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
	uiModeRecents
	uiModeApprovalResponse
	uiModeSearch
	uiModeRenameWorkspace
	uiModeRenameWorktree
	uiModeRenameSession
	uiModeEditWorkspaceGroups
	uiModePickWorkspaceRename
	uiModePickWorkspaceGroupEdit
	uiModePickWorkspaceGroupRename
	uiModeRenameWorkspaceGroup
	uiModePickWorkspaceGroupAssign
	uiModePickWorkspaceGroupDelete
	uiModeAssignGroupWorkspaces
	uiModePickNoteMoveTarget
	uiModePickNoteMoveWorktree
	uiModePickNoteMoveSession
)

type Model struct {
	workspaceAPI                        WorkspaceAPI
	sessionAPI                          SessionAPI
	sessionSelectionAPI                 SessionSelectionAPI
	sessionHistoryAPI                   SessionHistoryAPI
	notesAPI                            NotesAPI
	stateAPI                            StateAPI
	clipboard                           ClipboardService
	pickerPasteNormalizer               PickerPasteNormalizer
	sidebar                             *SidebarController
	viewport                            viewport.Model
	mode                                uiMode
	addWorkspace                        *AddWorkspaceController
	addWorktree                         *AddWorktreeController
	providerPicker                      *ProviderPicker
	compose                             *ComposeController
	chatAddonController                 *ChatInputAddonController
	chatInput                           *TextInput
	searchInput                         *TextInput
	renameInput                         *TextInput
	groupInput                          *TextInput
	groupPicker                         *GroupPicker
	workspacePicker                     *SelectPicker
	groupSelectPicker                   *SelectPicker
	workspaceMulti                      *MultiSelectPicker
	noteInput                           *TextInput
	approvalInput                       *TextInput
	recentsReplyInput                   *TextInput
	renameWorkspaceID                   string
	renameWorktreeWorkspaceID           string
	renameWorktreeID                    string
	renameSessionID                     string
	editWorkspaceID                     string
	renameGroupID                       string
	assignGroupID                       string
	status                              string
	toastText                           string
	toastLevel                          toastLevel
	toastUntil                          time.Time
	startupToasts                       []queuedToast
	width                               int
	height                              int
	follow                              bool
	showDismissed                       bool
	showRecents                         bool
	workspaces                          []*types.Workspace
	groups                              []*types.WorkspaceGroup
	worktrees                           map[string][]*types.Worktree
	sessions                            []*types.Session
	sessionMeta                         map[string]*types.SessionMeta
	providerOptions                     map[string]*types.ProviderOptionCatalog
	appState                            types.AppState
	hasAppState                         bool
	initialStateLoaded                  bool
	appStateSaveSeq                     int
	appStateSaveDirty                   bool
	appStateSaveScheduled               bool
	appStateSaveScheduledSeq            int
	appStateSaveInFlight                bool
	stream                              *StreamController
	codexStream                         *CodexStreamController
	itemStream                          *ItemStreamController
	input                               *InputController
	chat                                *SessionChatController
	pendingApproval                     *ApprovalRequest
	approvalResponseRequest             *ApprovalRequest
	approvalResponseSessionID           string
	approvalResponseRequestID           int
	approvalResponseReturnMode          uiMode
	approvalResponseReturnFocus         inputFocus
	sessionApprovals                    map[string][]*ApprovalRequest
	sessionApprovalResolutions          map[string][]*ApprovalResolution
	contentRaw                          string
	contentEsc                          bool
	contentBlocks                       []ChatBlock
	contentBlockMetaByID                map[string]ChatBlockMetaPresentation
	contentBlockSpans                   []renderedBlockSpan
	reasoningExpanded                   map[string]bool
	renderedText                        string
	renderedLines                       []string
	renderedPlain                       []string
	contentVersion                      int
	renderVersion                       int
	renderedForWidth                    int
	renderedForContent                  int
	renderedForSelection                int
	renderedForTimestampMode            ChatTimestampMode
	renderedForRelativeBucket           int64
	searchQuery                         string
	searchMatches                       []int
	searchIndex                         int
	searchVersion                       int
	messageSelectActive                 bool
	messageSelectIndex                  int
	sectionOffsets                      []int
	sectionVersion                      int
	transcriptCache                     map[string][]ChatBlock
	pendingSessionKey                   string
	loading                             bool
	loadingKey                          string
	loader                              spinner.Model
	pendingMouseCmd                     tea.Cmd
	lastSidebarWheelAt                  time.Time
	pendingSidebarWheel                 bool
	sidebarDragging                     bool
	lastSessionMetaRefreshAt            time.Time
	lastSessionMetaSyncAt               time.Time
	sessionMetaRefreshPending           bool
	sessionMetaSyncPending              bool
	streamRenderScheduler               RenderScheduler
	pendingComposeOptionTarget          composeOptionKind
	pendingComposeOptionFor             string
	menu                                *MenuController
	hotkeys                             *HotkeyRenderer
	keybindings                         *Keybindings
	contextMenu                         *ContextMenuController
	confirm                             *ConfirmController
	recents                             *RecentsTracker
	recentsSelectedSessionID            string
	recentsExpandedSessions             map[string]bool
	recentsReplySessionID               string
	recentsPreviews                     map[string]recentsPreview
	recentsCompletionWatching           map[string]string
	newSession                          *newSessionTarget
	pendingSelectID                     string
	selectSeq                           int
	sendSeq                             int
	pendingSends                        map[int]pendingSend
	composeHistory                      map[string]*composeHistoryState
	composeDrafts                       map[string]string
	noteDrafts                          map[string]string
	requestActivity                     requestActivity
	tickFn                              func() tea.Cmd
	pendingConfirm                      confirmAction
	scrollOnLoad                        bool
	notes                               []*types.Note
	notesByScope                        map[types.NoteScope][]*types.Note
	notesFilters                        notesFilterState
	notesScope                          noteScopeTarget
	notesReturnMode                     uiMode
	notesPanelOpen                      bool
	notesPanelVisible                   bool
	notesPanelWidth                     int
	notesPanelMainWidth                 int
	notesPanelPendingScopes             map[types.NoteScope]struct{}
	notesPanelLoadErrors                int
	notesPanelBlocks                    []ChatBlock
	notesPanelSpans                     []renderedBlockSpan
	notesPanelViewport                  viewport.Model
	noteMoveNoteID                      string
	noteMoveReturnMode                  uiMode
	uiLatency                           *uiLatencyTracker
	selectionLoadPolicy                 SessionSelectionLoadPolicy
	historyLoadPolicy                   SessionHistoryLoadPolicy
	sidebarProjectionBuilder            SidebarProjectionBuilder
	sidebarProjectionInvalidationPolicy SidebarProjectionInvalidationPolicy
	sidebarProjectionRevision           uint64
	sidebarProjectionApplied            uint64
	renderPipeline                      RenderPipeline
	layerComposer                       LayerComposer
	timestampMode                       ChatTimestampMode
	clockNow                            time.Time
	reasoningSnapshotHash               uint64
	reasoningSnapshotHas                bool
	reasoningSnapshotCollapsed          bool
}

type newSessionTarget struct {
	workspaceID    string
	worktreeID     string
	provider       string
	runtimeOptions *types.SessionRuntimeOptions
}

type composeOptionKind int

const (
	composeOptionNone composeOptionKind = iota
	composeOptionModel
	composeOptionReasoning
	composeOptionAccess
)

type composeControlSpan struct {
	kind  composeOptionKind
	start int
	end   int
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
	confirmDeleteNote
	confirmDismissSessions
)

type confirmAction struct {
	kind        confirmActionKind
	workspaceID string
	groupID     string
	worktreeID  string
	noteID      string
	sessionIDs  []string
}

func NewModel(client *client.Client, opts ...ModelOption) Model {
	vp := viewport.New(viewport.WithWidth(minViewportWidth), viewport.WithHeight(minContentHeight-1))
	vp.SetContent("No sessions.")
	notesPanelVP := viewport.New(viewport.WithWidth(minViewportWidth), viewport.WithHeight(minContentHeight-1))
	notesPanelVP.SetContent("No notes.")

	api := NewClientAPI(client)
	stream := NewStreamController(maxViewportLines, maxEventsPerTick)
	codexStream := NewCodexStreamController(maxViewportLines, maxEventsPerTick)
	itemStream := NewItemStreamController(maxViewportLines, maxEventsPerTick)
	loader := spinner.New()
	loader.Spinner = spinner.Line
	loader.Style = lipgloss.NewStyle()
	keybindings := DefaultKeybindings()
	hotkeyRenderer := NewHotkeyRenderer(ResolveHotkeys(DefaultHotkeys(), keybindings), DefaultHotkeyResolver{})
	now := time.Now().UTC()
	chatAddon := NewChatInputAddon(minViewportWidth, 8)

	model := Model{
		workspaceAPI:                        api,
		sessionAPI:                          api,
		sessionSelectionAPI:                 api,
		sessionHistoryAPI:                   api,
		notesAPI:                            api,
		stateAPI:                            api,
		clipboard:                           defaultClipboardService{},
		pickerPasteNormalizer:               defaultPickerPasteNormalizer{},
		sidebar:                             NewSidebarController(),
		viewport:                            vp,
		notesPanelViewport:                  notesPanelVP,
		stream:                              stream,
		codexStream:                         codexStream,
		itemStream:                          itemStream,
		input:                               NewInputController(),
		chat:                                NewSessionChatController(api, codexStream),
		mode:                                uiModeNormal,
		addWorkspace:                        NewAddWorkspaceController(minViewportWidth),
		addWorktree:                         NewAddWorktreeController(minViewportWidth),
		providerPicker:                      NewProviderPicker(minViewportWidth, minContentHeight-1),
		compose:                             NewComposeController(minViewportWidth),
		chatAddonController:                 NewChatInputAddonController(chatAddon),
		chatInput:                           NewTextInput(minViewportWidth, DefaultTextInputConfig()),
		searchInput:                         NewTextInput(minViewportWidth, TextInputConfig{Height: 1, SingleLine: true}),
		renameInput:                         NewTextInput(minViewportWidth, TextInputConfig{Height: 1, SingleLine: true}),
		groupInput:                          NewTextInput(minViewportWidth, TextInputConfig{Height: 1, SingleLine: true}),
		groupPicker:                         NewGroupPicker(minViewportWidth, minContentHeight-1),
		workspacePicker:                     NewSelectPicker(minViewportWidth, minContentHeight-1),
		groupSelectPicker:                   NewSelectPicker(minViewportWidth, minContentHeight-1),
		workspaceMulti:                      NewMultiSelectPicker(minViewportWidth, minContentHeight-1),
		noteInput:                           NewTextInput(minViewportWidth, DefaultTextInputConfig()),
		approvalInput:                       NewTextInput(minViewportWidth, DefaultTextInputConfig()),
		recentsReplyInput:                   NewTextInput(minViewportWidth, TextInputConfig{Height: 1, SingleLine: true}),
		status:                              "",
		toastLevel:                          toastLevelInfo,
		follow:                              true,
		groups:                              []*types.WorkspaceGroup{},
		worktrees:                           map[string][]*types.Worktree{},
		sessionMeta:                         map[string]*types.SessionMeta{},
		providerOptions:                     map[string]*types.ProviderOptionCatalog{},
		contentRaw:                          "No sessions.",
		contentEsc:                          false,
		searchIndex:                         -1,
		searchVersion:                       -1,
		approvalResponseRequestID:           -1,
		messageSelectIndex:                  -1,
		renderedForSelection:                -2,
		renderedForRelativeBucket:           -1,
		sectionVersion:                      -1,
		transcriptCache:                     map[string][]ChatBlock{},
		reasoningExpanded:                   map[string]bool{},
		sessionApprovals:                    map[string][]*ApprovalRequest{},
		sessionApprovalResolutions:          map[string][]*ApprovalResolution{},
		loader:                              loader,
		lastSessionMetaRefreshAt:            now,
		lastSessionMetaSyncAt:               now,
		hotkeys:                             hotkeyRenderer,
		keybindings:                         keybindings,
		pendingSends:                        map[int]pendingSend{},
		composeHistory:                      map[string]*composeHistoryState{},
		composeDrafts:                       map[string]string{},
		noteDrafts:                          map[string]string{},
		menu:                                NewMenuController(),
		contextMenu:                         NewContextMenuController(),
		confirm:                             NewConfirmController(),
		recents:                             NewRecentsTracker(),
		recentsExpandedSessions:             map[string]bool{},
		recentsPreviews:                     map[string]recentsPreview{},
		recentsCompletionWatching:           map[string]string{},
		notesByScope:                        map[types.NoteScope][]*types.Note{},
		notesPanelPendingScopes:             map[types.NoteScope]struct{}{},
		uiLatency:                           newUILatencyTracker(nil),
		selectionLoadPolicy:                 defaultSessionSelectionLoadPolicy{},
		historyLoadPolicy:                   defaultSessionHistoryLoadPolicy{},
		sidebarProjectionBuilder:            NewDefaultSidebarProjectionBuilder(),
		sidebarProjectionInvalidationPolicy: NewDefaultSidebarProjectionInvalidationPolicy(),
		sidebarProjectionRevision:           1,
		renderPipeline:                      NewDefaultRenderPipeline(),
		streamRenderScheduler:               NewDefaultRenderScheduler(),
		layerComposer:                       NewTextLayerComposer(),
		timestampMode:                       ChatTimestampModeRelative,
		clockNow:                            now,
	}
	for _, opt := range opts {
		if opt != nil {
			opt(&model)
		}
	}
	return model
}

func Run(client *client.Client) error {
	model := NewModel(client)
	uiConfig, err := config.LoadUIConfig()
	if err != nil {
		return err
	}
	model.applyUIConfig(uiConfig)
	keybindingsPath, err := uiConfig.ResolveKeybindingsPath()
	if err != nil {
		return err
	}
	keybindings, err := LoadKeybindings(keybindingsPath)
	if err != nil {
		log.Printf("keybindings: load %s: %v", keybindingsPath, err)
	} else {
		model.applyKeybindings(keybindings)
		conflicts := DetectKeybindingConflicts(keybindings)
		model.enqueueStartupKeybindingConflictToasts(conflicts)
	}
	p := tea.NewProgram(&model)
	_, err = p.Run()
	return err
}

func (m *Model) Init() tea.Cmd {
	return tea.Batch(
		fetchAppStateCmd(m.stateAPI),
		fetchWorkspacesCmd(m.workspaceAPI),
		fetchWorkspaceGroupsCmd(m.workspaceAPI),
		m.fetchSessionsCmd(false),
		fetchProviderOptionsCmd(m.sessionAPI, "codex"),
		fetchProviderOptionsCmd(m.sessionAPI, "claude"),
		m.tickCmd(),
	)
}

func (m *Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	startedAt := time.Now()
	defer m.recordUILatencySpan(uiLatencySpanModelUpdate, startedAt)

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
			if _, ok := msg.(tea.MouseClickMsg); !ok {
				return m, nil
			}
			if handled, choice := m.confirm.HandleMouse(msg, m.width, m.height-1); handled {
				if choice == confirmChoiceNone {
					return m, nil
				}
				return m, m.handleConfirmChoice(choice)
			}
			mouse := msg.Mouse()
			if mouse.Button == tea.MouseLeft {
				if !m.confirm.Contains(mouse.X, mouse.Y, m.width, m.height-1) {
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
				save := m.requestAppStateSaveCmd()
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
	if handled, cmd := m.reduceApprovalResponseMode(msg); handled {
		return m, cmd
	}
	if handled, cmd := m.reduceWorkspaceEditModes(msg); handled {
		return m, cmd
	}
	if handled, cmd := m.reduceNoteMovePickerMode(msg); handled {
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
	if handled, cmd := m.reduceRecentsMode(msg); handled {
		return m, cmd
	}
	if handled, cmd := m.reduceAddNoteMode(msg); handled {
		return m, cmd
	}
	if _, ok := msg.(tea.PasteMsg); ok {
		if handled, cmd := m.reduceSearchModeKey(msg); handled {
			return m, cmd
		}
		if handled, cmd := m.reduceComposeInputKey(msg); handled {
			return m, cmd
		}
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

func (m *Model) View() tea.View {
	startedAt := time.Now()
	defer m.recordUILatencySpan(uiLatencySpanModelView, startedAt)

	rightView := m.renderRightPaneView()
	body := m.renderBodyWithSidebar(rightView)
	statusLine := m.renderStatusLineView()
	var content string
	if m.height <= 0 || m.width <= 0 {
		content = body
	} else {
		body = m.overlayTransientViews(body)
		content = lipgloss.JoinVertical(lipgloss.Left, body, statusLine)
	}
	v := tea.NewView(content)
	v.AltScreen = true
	v.MouseMode = tea.MouseModeAllMotion
	return v
}

func (m *Model) applyUIConfig(uiConfig config.UIConfig) {
	minHeight, maxHeight := uiConfig.SharedMultilineInputHeights()
	nextTimestampMode := parseChatTimestampMode(uiConfig.ChatTimestampMode())
	sidebarExpandByDefault := uiConfig.SidebarExpandByDefault()
	showRecents := uiConfig.SidebarShowRecents()
	timestampModeChanged := nextTimestampMode != m.timestampMode
	if timestampModeChanged {
		m.timestampMode = nextTimestampMode
		m.renderedForTimestampMode = ""
		m.renderedForRelativeBucket = -1
	}
	recentsVisibilityChanged := m.showRecents != showRecents
	m.showRecents = showRecents
	sidebarExpansionChanged := false
	if m.sidebar != nil {
		sidebarExpansionChanged = m.sidebar.SetExpandByDefault(sidebarExpandByDefault)
	}
	cfg := DefaultTextInputConfig()
	cfg.Height = minHeight
	cfg.MinHeight = minHeight
	cfg.MaxHeight = maxHeight
	cfg.AutoGrow = true
	if m.chatInput != nil {
		m.chatInput.SetConfig(cfg)
	}
	if m.noteInput != nil {
		m.noteInput.SetConfig(cfg)
	}
	if m.approvalInput != nil {
		m.approvalInput.SetConfig(cfg)
	}
	inputHeightChanged := m.consumeInputHeightChanges(m.chatInput, m.noteInput, m.approvalInput)
	if inputHeightChanged && m.width > 0 && m.height > 0 {
		m.resize(m.width, m.height)
		return
	}
	if timestampModeChanged && m.width > 0 && m.height > 0 {
		m.renderViewport()
		m.renderNotesPanel()
	}
	if sidebarExpansionChanged || recentsVisibilityChanged {
		m.applySidebarItems()
	}
}

func (m *Model) consumeInputHeightChanges(inputs ...*TextInput) bool {
	changed := false
	for _, input := range inputs {
		if input != nil && input.ConsumeHeightChanged() {
			changed = true
		}
	}
	return changed
}

func (m *Model) resize(width, height int) {
	m.resizeWithoutRender(width, height)
	m.renderViewport()
	m.renderNotesPanel()
}

func (m *Model) resizeWithoutRender(width, height int) {
	m.width = width
	m.height = height

	layout := resolveResizeLayout(width, height, m.appState.SidebarCollapsed, m.notesPanelOpen, m.usesViewport())
	contentHeight := layout.contentHeight
	contentWidth := layout.contentWidth
	mainViewportWidth := layout.panelMain
	m.notesPanelVisible = layout.panelVisible
	m.notesPanelWidth = layout.panelWidth
	m.notesPanelMainWidth = layout.panelMain

	if m.sidebar != nil {
		m.sidebar.SetSize(layout.sidebarWidth, contentHeight)
	}
	extraLines := 0
	if m.mode == uiModeCompose {
		if m.chatInput != nil {
			extraLines = m.chatInput.Height() + 2
		} else {
			extraLines = 3
		}
	} else if m.mode == uiModeApprovalResponse {
		if m.approvalInput != nil {
			extraLines = m.approvalInput.Height() + 2
		} else {
			extraLines = 3
		}
	} else if m.mode == uiModeAddNote {
		if m.noteInput != nil {
			extraLines = m.noteInput.Height() + 1
		} else {
			extraLines = 2
		}
	} else if m.mode == uiModeSearch {
		extraLines = 2
	} else if m.mode == uiModeRecents && strings.TrimSpace(m.recentsReplySessionID) != "" {
		if m.recentsReplyInput != nil {
			extraLines = m.recentsReplyInput.Height() + 2
		} else {
			extraLines = 3
		}
	}
	vpHeight := max(1, contentHeight-1-extraLines)
	m.viewport.SetWidth(contentWidth)
	m.viewport.SetHeight(vpHeight)
	if layout.panelVisible {
		m.notesPanelViewport.SetWidth(layout.panelWidth)
		m.notesPanelViewport.SetHeight(max(1, contentHeight-1))
	} else {
		m.notesPanelViewport.SetWidth(0)
		m.notesPanelViewport.SetHeight(0)
	}
	if m.addWorkspace != nil {
		m.addWorkspace.Resize(mainViewportWidth)
	}
	if m.addWorktree != nil {
		m.addWorktree.Resize(mainViewportWidth)
		m.addWorktree.SetListHeight(max(3, contentHeight-4))
	}
	if m.providerPicker != nil {
		m.providerPicker.SetSize(mainViewportWidth, max(3, contentHeight-2))
	}
	if m.compose != nil {
		m.compose.Resize(mainViewportWidth)
	}
	if m.groupPicker != nil {
		m.groupPicker.SetSize(mainViewportWidth, max(3, contentHeight-2))
	}
	if m.workspacePicker != nil {
		m.workspacePicker.SetSize(mainViewportWidth, max(3, contentHeight-2))
	}
	if m.groupSelectPicker != nil {
		m.groupSelectPicker.SetSize(mainViewportWidth, max(3, contentHeight-2))
	}
	if m.workspaceMulti != nil {
		m.workspaceMulti.SetSize(mainViewportWidth, max(3, contentHeight-2))
	}
	if m.chatAddonController != nil {
		m.chatAddonController.setPickerSize(mainViewportWidth, 8)
	}
	if m.chatInput != nil {
		m.chatInput.Resize(mainViewportWidth)
	}
	if m.searchInput != nil {
		m.searchInput.Resize(mainViewportWidth)
	}
	if m.noteInput != nil {
		m.noteInput.Resize(mainViewportWidth)
	}
	if m.recentsReplyInput != nil {
		m.recentsReplyInput.Resize(mainViewportWidth)
	}
}

func (m *Model) onSelectionChanged() tea.Cmd {
	return m.onSelectionChangedWithDelay(0)
}

func (m *Model) onSelectionChangedImmediate() tea.Cmd {
	return m.onSelectionChangedWithDelay(0)
}

func (m *Model) onSelectionChangedWithDelay(delay time.Duration) tea.Cmd {
	item := m.selectedItem()
	handled, stateChanged, draftChanged := m.applySelectionState(item)
	var cmd tea.Cmd
	if !handled {
		cmd = m.scheduleSessionLoad(item, delay)
	} else if m.mode == uiModeRecents {
		cmd = m.ensureRecentsPreviewForSelection()
	}
	if stateChanged || draftChanged {
		save := m.requestAppStateSaveCmd()
		if cmd != nil && save != nil {
			cmd = tea.Batch(cmd, save)
		} else if save != nil {
			cmd = save
		}
	}
	return m.batchWithNotesPanelSync(cmd)
}

func (m *Model) applySelectionState(item *sidebarItem) (handled bool, stateChanged bool, draftChanged bool) {
	if item == nil {
		m.cancelUILatencyAction(uiLatencyActionSwitchSession, "")
		m.resetStream()
		m.setContentText("No sessions.")
		return true, false, false
	}
	switch item.kind {
	case sidebarRecentsAll, sidebarRecentsReady, sidebarRecentsRunning:
		m.cancelUILatencyAction(uiLatencyActionSwitchSession, "")
		m.enterRecentsView(item)
		return true, false, false
	case sidebarWorkspace:
		if m.mode == uiModeRecents {
			m.exitRecentsView()
		}
		m.cancelUILatencyAction(uiLatencyActionSwitchSession, "")
		if m.mode == uiModeRecents {
			m.exitRecentsView()
		}
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
		return true, stateChanged, false
	case sidebarWorktree:
		if m.mode == uiModeRecents {
			m.exitRecentsView()
		}
		m.cancelUILatencyAction(uiLatencyActionSwitchSession, "")
		if m.mode == uiModeRecents {
			m.exitRecentsView()
		}
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
		return true, stateChanged, false
	default:
		if !item.isSession() {
			m.cancelUILatencyAction(uiLatencyActionSwitchSession, "")
			return true, false, false
		}
	}

	if m.mode == uiModeCompose && m.compose != nil && item.session != nil {
		nextSessionID := strings.TrimSpace(item.session.ID)
		previousSessionID := strings.TrimSpace(m.composeSessionID())
		sessionChanged := previousSessionID != nextSessionID
		if sessionChanged {
			draftChanged = m.saveCurrentComposeDraft() || draftChanged
			m.closeComposeOptionPicker()
		}
		m.compose.SetSession(nextSessionID, sessionTitle(item.session, item.meta))
		m.resetComposeHistoryCursor()
		if m.chatInput != nil {
			m.chatInput.SetPlaceholder("message")
			if sessionChanged {
				m.restoreComposeDraft(nextSessionID)
			}
		}
		if m.consumeInputHeightChanges(m.chatInput) {
			m.resize(m.width, m.height)
		}
	}
	if m.mode == uiModeRecents {
		m.exitRecentsView()
	}
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
	return false, stateChanged, draftChanged
}

func (m *Model) scheduleSessionLoad(item *sidebarItem, delay time.Duration) tea.Cmd {
	if item == nil || item.session == nil {
		return nil
	}
	delay = m.selectionLoadPolicyOrDefault().SelectionLoadDelay(delay)
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
	token := item.key()
	m.startUILatencyAction(uiLatencyActionSwitchSession, token)

	id := item.session.ID
	m.resetStream()
	m.pendingApproval = nil
	m.pendingSessionKey = token
	m.setStatusMessage("loading " + id)
	m.scrollOnLoad = true
	if cached, ok := m.transcriptCache[token]; ok {
		m.setSnapshotBlocks(cached)
		m.loading = false
		m.loadingKey = ""
		m.finishUILatencyAction(uiLatencyActionSwitchSession, token, uiLatencyOutcomeCacheHit)
	} else {
		m.loading = true
		m.loadingKey = token
		m.setLoadingContent()
	}
	initialLines := m.historyFetchLinesInitial()
	cmds := []tea.Cmd{fetchHistoryCmd(m.sessionHistoryAPI, id, token, initialLines), fetchApprovalsCmd(m.sessionAPI, id)}
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

func (m *Model) syncSidebarExpansionChange() tea.Cmd {
	cmd := m.onSelectionChangedImmediate()
	if !m.syncAppStateSidebarExpansion() {
		return cmd
	}
	save := m.requestAppStateSaveCmd()
	if cmd != nil && save != nil {
		return tea.Batch(cmd, save)
	}
	if save != nil {
		return save
	}
	return cmd
}

func sessionIDFromSidebarKey(key string) string {
	key = strings.TrimSpace(key)
	if key == "" || !strings.HasPrefix(key, "sess:") {
		return ""
	}
	return strings.TrimSpace(strings.TrimPrefix(key, "sess:"))
}

func (m *Model) invalidateSidebarProjection(reason sidebarProjectionChangeReason) {
	if m == nil {
		return
	}
	policy := m.sidebarProjectionInvalidationPolicy
	if policy == nil {
		policy = NewDefaultSidebarProjectionInvalidationPolicy()
		m.sidebarProjectionInvalidationPolicy = policy
	}
	if !policy.ShouldInvalidate(reason) {
		return
	}
	m.sidebarProjectionRevision++
}

func (m *Model) setActiveWorkspaceGroupIDs(ids []string) bool {
	if m == nil {
		return false
	}
	normalized := append([]string(nil), ids...)
	if slicesEqual(m.appState.ActiveWorkspaceGroupIDs, normalized) {
		return false
	}
	m.appState.ActiveWorkspaceGroupIDs = normalized
	m.hasAppState = true
	m.invalidateSidebarProjection(sidebarProjectionChangeGroup)
	return true
}

func (m *Model) setWorkspacesData(workspaces []*types.Workspace) {
	if m == nil {
		return
	}
	m.workspaces = workspaces
	m.invalidateSidebarProjection(sidebarProjectionChangeWorkspace)
}

func (m *Model) setSessionsAndMeta(sessions []*types.Session, meta map[string]*types.SessionMeta) {
	if m == nil {
		return
	}
	m.sessions = sessions
	m.sessionMeta = meta
	if m.recents != nil {
		m.recents.ObserveSessions(sessions)
	}
	m.invalidateSidebarProjection(sidebarProjectionChangeSessions)
}

func (m *Model) setWorktreesData(workspaceID string, worktrees []*types.Worktree) {
	if m == nil || strings.TrimSpace(workspaceID) == "" {
		return
	}
	if m.worktrees == nil {
		m.worktrees = map[string][]*types.Worktree{}
	}
	m.worktrees[workspaceID] = worktrees
	m.invalidateSidebarProjection(sidebarProjectionChangeWorktree)
}

func (m *Model) setShowDismissed(show bool) {
	if m == nil {
		return
	}
	if m.showDismissed == show {
		return
	}
	m.showDismissed = show
	m.invalidateSidebarProjection(sidebarProjectionChangeDismissed)
}

func (m *Model) handleMenuGroupChange(previous []string) bool {
	if m.menu == nil {
		return false
	}
	next := m.menu.SelectedGroupIDs()
	if slicesEqual(previous, next) {
		return false
	}
	m.setActiveWorkspaceGroupIDs(next)
	m.applySidebarItemsIfDirty()
	return true
}

func (m *Model) applySidebarItems() {
	if m.sidebar == nil {
		m.resetStream()
		m.setContentText("No sessions.")
		return
	}
	projection := m.buildSidebarProjection()
	m.sidebar.recentsState = m.sidebarRecentsState(projection.Sessions)
	item := m.sidebar.Apply(projection.Workspaces, m.worktrees, projection.Sessions, m.sessionMeta, m.appState.ActiveWorkspaceID, m.appState.ActiveWorktreeID, m.showDismissed)
	m.sidebarProjectionApplied = m.sidebarProjectionRevision
	if item == nil {
		m.resetStream()
		m.setContentText("No sessions.")
		return
	}
	if !item.isSession() {
		m.resetStream()
		if item.kind == sidebarRecentsAll || item.kind == sidebarRecentsReady || item.kind == sidebarRecentsRunning {
			if m.mode != uiModeRecents {
				m.enterRecentsView(item)
			} else {
				m.refreshRecentsContent()
			}
			return
		}
		m.setContentText("Select a session.")
	}
	if m.mode == uiModeRecents {
		m.refreshRecentsContent()
	}
}

func (m *Model) buildSidebarProjection() SidebarProjection {
	if m == nil {
		return SidebarProjection{}
	}
	builder := m.sidebarProjectionBuilder
	if builder == nil {
		builder = NewDefaultSidebarProjectionBuilder()
		m.sidebarProjectionBuilder = builder
	}
	return builder.Build(SidebarProjectionInput{
		Workspaces:         m.workspaces,
		Worktrees:          m.worktrees,
		Sessions:           m.sessions,
		SessionMeta:        m.sessionMeta,
		ActiveWorkspaceIDs: append([]string(nil), m.appState.ActiveWorkspaceGroupIDs...),
	})
}

func (m *Model) sidebarRecentsState(visibleSessions []*types.Session) sidebarRecentsState {
	if m == nil || !m.showRecents || m.recents == nil {
		return sidebarRecentsState{}
	}
	if len(visibleSessions) == 0 {
		return sidebarRecentsState{Enabled: true}
	}
	visible := make(map[string]struct{}, len(visibleSessions))
	for _, session := range visibleSessions {
		if session == nil || strings.TrimSpace(session.ID) == "" {
			continue
		}
		visible[strings.TrimSpace(session.ID)] = struct{}{}
	}
	readyCount := 0
	for _, id := range m.recents.ReadyIDs() {
		if _, ok := visible[id]; ok {
			readyCount++
		}
	}
	runningCount := 0
	for _, id := range m.recents.RunningIDs() {
		if _, ok := visible[id]; ok {
			runningCount++
		}
	}
	return sidebarRecentsState{
		Enabled:      true,
		ReadyCount:   readyCount,
		RunningCount: runningCount,
	}
}

func (m *Model) applySidebarItemsIfDirty() {
	if m == nil {
		return
	}
	if m.sidebarProjectionApplied == m.sidebarProjectionRevision {
		return
	}
	m.applySidebarItems()
}

func (m *Model) sidebarWidth() int {
	return computeSidebarWidth(m.width, m.appState.SidebarCollapsed)
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
		case confirmDeleteNote:
			if strings.TrimSpace(action.noteID) == "" {
				m.setValidationStatus("select a note to delete")
				return nil
			}
			m.setStatusMessage("deleting note")
			return deleteNoteCmd(m.notesAPI, action.noteID)
		case confirmDismissSessions:
			if len(action.sessionIDs) == 0 {
				m.setValidationStatus("no session selected")
				return nil
			}
			if len(action.sessionIDs) == 1 {
				m.setStatusMessage("dismissing " + action.sessionIDs[0])
				return dismissSessionCmd(m.sessionAPI, action.sessionIDs[0])
			}
			m.setStatusMessage(fmt.Sprintf("dismissing %d sessions", len(action.sessionIDs)))
			return dismissManySessionsCmd(m.sessionAPI, action.sessionIDs)
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

func (m *Model) confirmDeleteNote(noteID string) {
	if m.confirm == nil {
		return
	}
	noteID = strings.TrimSpace(noteID)
	if noteID == "" {
		m.setValidationStatus("select a note to delete")
		return
	}
	message := "Delete note?"
	if note := m.noteByID(noteID); note != nil {
		message = fmt.Sprintf("Delete note %q?", noteTitle(note))
	}
	m.pendingConfirm = confirmAction{
		kind:   confirmDeleteNote,
		noteID: noteID,
	}
	if m.menu != nil {
		m.menu.CloseAll()
	}
	if m.contextMenu != nil {
		m.contextMenu.Close()
	}
	m.confirm.Open("Delete Note", message, "Delete", "Cancel")
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
	m.clockNow = now
	m.consumeStreamTick(now)
	m.consumeCodexTick(now)
	m.consumeItemTick(now)
	if m.streamRenderScheduler != nil && m.streamRenderScheduler.ShouldRender(now) {
		m.renderViewport()
		m.streamRenderScheduler.MarkRendered(now)
	}
	if m.loading {
		m.loader, _ = m.loader.Update(spinner.TickMsg{Time: now, ID: m.loader.ID()})
		m.setLoadingContent()
	}
	if m.toastText != "" && !m.toastActive(now) {
		m.clearToast()
	}
	m.maybeShowNextStartupToast(now)
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
	if cmd := m.maybeAutoRefreshSessionMeta(now); cmd != nil {
		cmds = append(cmds, cmd)
	}
	m.maybeRefreshRelativeTimestampLabels(now)
	cmds = append(cmds, m.tickCmd())
	return tea.Batch(cmds...)
}

func (m *Model) maybeRefreshRelativeTimestampLabels(now time.Time) {
	if m == nil || normalizeChatTimestampMode(m.timestampMode) != ChatTimestampModeRelative {
		return
	}
	if m.contentBlocks == nil {
		return
	}
	nextBucket := chatTimestampRenderBucket(m.timestampMode, now)
	if nextBucket < 0 || nextBucket == m.renderedForRelativeBucket {
		return
	}
	m.renderViewport()
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

func (m *Model) consumeStreamTick(now time.Time) {
	if m.stream == nil {
		return
	}
	lines, changed, closed := m.stream.ConsumeTick()
	if closed {
		m.setBackgroundStatus("stream closed")
	}
	if changed {
		if m.transcriptViewportVisible() {
			m.applyLinesNoRender(lines, true)
			m.requestStreamRender(now)
		}
	}
}

func (m *Model) consumeCodexTick(now time.Time) {
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
		if m.transcriptViewportVisible() {
			m.applyBlocksNoRender(blocks)
			m.requestStreamRender(now)
		}
		if sessionID != "" {
			if m.transcriptViewportVisible() {
				m.noteRequestVisibleUpdate(sessionID)
			}
			if key := m.selectedKey(); key != "" {
				m.cacheTranscriptBlocks(key, blocks)
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
				base := m.codexStream.Blocks()
				blocks := mergeApprovalBlocks(base, requests, m.sessionApprovalResolutions[sessionID])
				m.codexStream.SetSnapshotBlocks(blocks)
				blocks = m.codexStream.Blocks()
				if m.transcriptViewportVisible() {
					m.applyBlocksNoRender(blocks)
					m.requestStreamRender(now)
				}
				if key := m.selectedKey(); key != "" {
					m.cacheTranscriptBlocks(key, blocks)
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

func (m *Model) consumeItemTick(now time.Time) {
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
		blocks := m.itemStream.Blocks()
		if m.transcriptViewportVisible() {
			m.applyBlocksNoRender(blocks)
			m.requestStreamRender(now)
		}
		if sessionID != "" {
			m.noteRequestEvent(sessionID, 1)
			if m.transcriptViewportVisible() {
				m.noteRequestVisibleUpdate(sessionID)
			}
			if key := m.selectedKey(); key != "" {
				m.cacheTranscriptBlocks(key, blocks)
			}
		}
	}
}

func (m *Model) fetchSessionsCmd(refresh bool) tea.Cmd {
	if m == nil || m.sessionSelectionAPI == nil {
		return nil
	}
	opts := fetchSessionsOptions{
		refresh:          refresh,
		includeDismissed: m.showDismissed,
	}
	if refresh {
		opts.workspaceID = m.refreshWorkspaceID()
	}
	return fetchSessionsWithMetaCmd(m.sessionSelectionAPI, opts)
}

func (m *Model) maybeAutoRefreshSessionMeta(now time.Time) tea.Cmd {
	if m == nil || m.sessionSelectionAPI == nil {
		return nil
	}
	if m.sessionMetaSyncPending || m.sessionMetaRefreshPending {
		return nil
	}
	if now.Sub(m.lastSessionMetaSyncAt) >= sessionMetaSyncDelay {
		m.sessionMetaSyncPending = true
		m.lastSessionMetaSyncAt = now
		return m.fetchSessionsCmd(true)
	}
	if now.Sub(m.lastSessionMetaRefreshAt) >= sessionMetaRefreshDelay {
		m.sessionMetaRefreshPending = true
		m.lastSessionMetaRefreshAt = now
		return m.fetchSessionsCmd(false)
	}
	return nil
}

func (m *Model) noteSessionMetaActivity(sessionID, turnID string, at time.Time) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return
	}
	if m.sessionMeta == nil {
		m.sessionMeta = map[string]*types.SessionMeta{}
	}
	entry := m.sessionMeta[sessionID]
	if entry == nil {
		entry = &types.SessionMeta{SessionID: sessionID}
		m.sessionMeta[sessionID] = entry
	}
	if !at.IsZero() {
		ts := at.UTC()
		entry.LastActiveAt = &ts
	}
	turnID = strings.TrimSpace(turnID)
	if turnID != "" {
		entry.LastTurnID = turnID
	}
}

func (m *Model) refreshWorkspaceID() string {
	if item := m.selectedItem(); item != nil {
		if workspaceID := strings.TrimSpace(item.workspaceID()); workspaceID != "" && workspaceID != unassignedWorkspaceID {
			return workspaceID
		}
	}
	workspaceID := strings.TrimSpace(m.appState.ActiveWorkspaceID)
	if workspaceID == unassignedWorkspaceID {
		return ""
	}
	return workspaceID
}

func (m *Model) toggleShowDismissed() tea.Cmd {
	m.setShowDismissed(!m.showDismissed)
	if m.showDismissed {
		m.setStatusMessage("showing dismissed sessions")
	} else {
		m.setStatusMessage("hiding dismissed sessions")
	}
	return m.fetchSessionsCmd(false)
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
	m.applyLinesNoRender(lines, escape)
	m.renderViewport()
}

func (m *Model) applyLinesNoRender(lines []string, escape bool) {
	m.contentRaw = strings.Join(lines, "\n")
	m.contentEsc = escape
	m.contentBlocks = nil
	m.contentBlockMetaByID = nil
	m.contentBlockSpans = nil
	m.reasoningSnapshotHash = 0
	m.reasoningSnapshotHas = false
	m.reasoningSnapshotCollapsed = false
	m.clearMessageSelection()
	m.contentVersion++
	m.searchVersion = -1
	m.sectionVersion = -1
}

func (m *Model) applyBlocks(blocks []ChatBlock) {
	m.applyBlocksNoRenderWithMeta(blocks, nil)
	m.renderViewport()
}

func (m *Model) applyBlocksNoRender(blocks []ChatBlock) {
	m.applyBlocksNoRenderWithMeta(blocks, nil)
}

func (m *Model) applyBlocksWithMeta(blocks []ChatBlock, metaByBlockID map[string]ChatBlockMetaPresentation) {
	m.applyBlocksNoRenderWithMeta(blocks, metaByBlockID)
	m.renderViewport()
}

func (m *Model) applyBlocksNoRenderWithMeta(blocks []ChatBlock, metaByBlockID map[string]ChatBlockMetaPresentation) {
	if len(blocks) == 0 {
		m.contentBlocks = nil
		m.contentBlockMetaByID = nil
		m.contentBlockSpans = nil
		m.reasoningSnapshotHash = 0
		m.reasoningSnapshotHas = false
		m.reasoningSnapshotCollapsed = false
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
		m.contentBlockMetaByID = cloneChatBlockMetaByID(metaByBlockID)
		hash, hasReasoning, hasCollapsed := reasoningSnapshotState(resolved)
		m.reasoningSnapshotHash = hash
		m.reasoningSnapshotHas = hasReasoning
		m.reasoningSnapshotCollapsed = hasCollapsed
	}
	m.clampMessageSelection()
	m.contentRaw = ""
	m.contentEsc = false
	m.contentVersion++
	m.searchVersion = -1
	m.sectionVersion = -1
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
	case uiModeNormal, uiModeCompose, uiModeRecents, uiModeSearch, uiModeNotes, uiModeAddNote:
		return true
	default:
		return false
	}
}

func (m *Model) viewportScrollbarView() string {
	if m.viewport.Height() <= 0 {
		return ""
	}
	height := m.viewport.Height()
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
		top = int(math.Round(float64(m.viewport.YOffset()) / float64(denom) * float64(maxStart)))
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
	m.contentBlockMetaByID = nil
	m.contentBlockSpans = nil
	m.reasoningSnapshotHash = 0
	m.reasoningSnapshotHas = false
	m.reasoningSnapshotCollapsed = false
	m.clearMessageSelection()
	m.contentVersion++
	m.searchVersion = -1
	m.sectionVersion = -1
	m.renderViewport()
}

func (m *Model) renderViewport() {
	if m.viewport.Width() <= 0 {
		return
	}
	renderWidth := m.viewport.Width()
	if !m.appState.SidebarCollapsed && renderWidth > 1 {
		renderWidth -= 1
	}
	selectedRenderIndex := m.selectedMessageRenderIndex()
	now := m.clockNow
	if now.IsZero() {
		now = time.Now()
	}
	mode := normalizeChatTimestampMode(m.timestampMode)
	relativeBucket := chatTimestampRenderBucket(mode, now)
	needsRender := m.renderedForWidth != renderWidth ||
		m.renderedForContent != m.contentVersion ||
		m.renderedForSelection != selectedRenderIndex ||
		m.renderedForTimestampMode != mode ||
		m.renderedForRelativeBucket != relativeBucket
	if needsRender {
		pipeline := m.renderPipeline
		if pipeline == nil {
			pipeline = NewDefaultRenderPipeline()
			m.renderPipeline = pipeline
		}
		result := pipeline.Render(RenderRequest{
			Width:              renderWidth,
			MaxLines:           maxViewportLines,
			RawContent:         m.contentRaw,
			EscapeMarkdown:     m.contentEsc,
			Blocks:             m.contentBlocks,
			BlockMetaByID:      m.contentBlockMetaByID,
			SelectedBlockIndex: selectedRenderIndex,
			TimestampMode:      mode,
			TimestampNow:       now,
		})
		m.renderedText = result.Text
		m.renderedLines = result.Lines
		m.renderedPlain = result.PlainLines
		m.contentBlockSpans = result.Spans
		m.renderedForWidth = renderWidth
		m.renderedForContent = m.contentVersion
		m.renderedForSelection = selectedRenderIndex
		m.renderedForTimestampMode = mode
		m.renderedForRelativeBucket = relativeBucket
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

func (m *Model) requestStreamRender(now time.Time) {
	if m == nil {
		return
	}
	if m.streamRenderScheduler == nil {
		m.renderViewport()
		return
	}
	if m.streamRenderScheduler.Request(now) {
		m.renderViewport()
		m.streamRenderScheduler.MarkRendered(now)
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
	if m.mode != uiModeNormal && m.mode != uiModeCompose && m.mode != uiModeRecents && m.mode != uiModeNotes && m.mode != uiModeAddNote {
		return false
	}
	wasFollowing := m.follow
	scrolledDown := false
	switch msg.String() {
	case "up":
		m.pauseFollow(true)
		m.viewport.ScrollUp(1)
	case "down":
		m.viewport.ScrollDown(1)
		scrolledDown = true
	case "pgup":
		m.pauseFollow(true)
		m.viewport.PageUp()
	case "pgdown":
		m.viewport.PageDown()
		scrolledDown = true
	case "ctrl+f":
		m.viewport.PageDown()
		scrolledDown = true
	case "ctrl+u":
		m.pauseFollow(true)
		m.viewport.HalfPageUp()
	case "ctrl+d":
		m.viewport.HalfPageDown()
		scrolledDown = true
	case "home":
		m.pauseFollow(true)
		m.viewport.GotoTop()
	case "end":
		m.enableFollow(true)
		return true
	default:
		return false
	}
	m.maybeResumeFollowAfterManualScroll(wasFollowing, scrolledDown)
	return true
}

func (m *Model) toggleVisibleReasoning() bool {
	if len(m.contentBlocks) == 0 || len(m.contentBlockSpans) == 0 {
		return false
	}
	start := m.viewport.YOffset()
	end := start + m.viewport.Height() - 1
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
	absolute := m.viewport.YOffset() + line
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
	maxOffset := total - m.viewport.Height()
	if maxOffset < 0 {
		return 0
	}
	return maxOffset
}

func (m *Model) isViewportAtBottom() bool {
	return m.viewport.YOffset() >= m.maxViewportYOffset()
}

func (m *Model) maybeResumeFollowAfterManualScroll(wasFollowing, scrolledDown bool) {
	if wasFollowing || !scrolledDown {
		return
	}
	if m.isViewportAtBottom() {
		m.setFollowEnabled(true, true)
	}
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
	if m.reduceSidebarDragMouse(msg, layout) {
		return true
	}

	mouse := msg.Mouse()
	if mouse.Button == tea.MouseLeft || mouse.Button == tea.MouseRight {
		if _, ok := msg.(tea.MouseClickMsg); !ok {
			return false
		}
	}
	if m.reduceContextMenuLeftPressMouse(msg) {
		return true
	}
	if m.reduceContextMenuRightPressMouse(msg, layout) {
		return true
	}
	switch mouse.Button {
	case tea.MouseWheelUp:
		return m.reduceMouseWheel(msg, layout, -1)
	case tea.MouseWheelDown:
		return m.reduceMouseWheel(msg, layout, 1)
	case tea.MouseLeft:
	default:
		return false
	}
	if mouse.Button != tea.MouseLeft {
		return false
	}
	if m.reduceMenuLeftPressMouse(msg) {
		return true
	}
	if m.reduceSidebarScrollbarLeftPressMouse(msg, layout) {
		return true
	}
	if m.reduceComposeOptionPickerLeftPressMouse(msg, layout) {
		return true
	}
	if m.reduceComposeControlsLeftPressMouse(msg, layout) {
		return true
	}
	if m.reduceInputFocusLeftPressMouse(msg, layout) {
		return true
	}
	if m.reduceGlobalStatusCopyLeftPressMouse(msg) {
		return true
	}
	if m.reduceNotesPanelLeftPressMouse(msg, layout) {
		return true
	}
	if m.reduceTranscriptApprovalButtonLeftPressMouse(msg, layout) {
		return true
	}
	if m.reduceTranscriptPinLeftPressMouse(msg, layout) {
		return true
	}
	if m.reduceTranscriptNotesFilterLeftPressMouse(msg, layout) {
		return true
	}
	if m.reduceTranscriptMoveLeftPressMouse(msg, layout) {
		return true
	}
	if m.reduceTranscriptDeleteLeftPressMouse(msg, layout) {
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

func (m *Model) transcriptViewportVisible() bool {
	return m.mode != uiModeNotes && m.mode != uiModeAddNote
}

func (m *Model) cacheTranscriptBlocks(key string, blocks []ChatBlock) {
	key = strings.TrimSpace(key)
	if key == "" {
		return
	}
	m.transcriptCache[key] = append([]ChatBlock(nil), blocks...)
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
	provider := m.providerForSessionID(sessionID)
	base := m.currentBlocks()
	if !m.transcriptViewportVisible() {
		if provider == "codex" && m.codexStream != nil {
			base = m.codexStream.Blocks()
		} else if shouldStreamItems(provider) && m.itemStream != nil {
			base = m.itemStream.Blocks()
		}
	}
	requests := m.sessionApprovals[sessionID]
	if len(base) == 0 && len(requests) == 0 {
		return
	}
	blocks := mergeApprovalBlocks(base, requests, m.sessionApprovalResolutions[sessionID])
	if provider == "codex" && m.codexStream != nil {
		m.codexStream.SetSnapshotBlocks(blocks)
		blocks = m.codexStream.Blocks()
	} else if shouldStreamItems(provider) && m.itemStream != nil {
		m.itemStream.SetSnapshotBlocks(blocks)
		blocks = m.itemStream.Blocks()
	}
	if m.transcriptViewportVisible() {
		m.applyBlocks(blocks)
	}
	if key := m.selectedKey(); key != "" {
		m.cacheTranscriptBlocks(key, blocks)
	}
}

func (m *Model) toggleSidebar() {
	m.startUILatencyAction(uiLatencyActionToggleSessionsSidebar, "")
	defer m.finishUILatencyAction(uiLatencyActionToggleSessionsSidebar, "", uiLatencyOutcomeOK)

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
	m.composeDrafts = importDraftMap(state.ComposeDrafts, composeHistoryMaxSessions)
	m.noteDrafts = importDraftMap(state.NoteDrafts, composeHistoryMaxSessions)
	m.hasAppState = true
	if m.menu != nil {
		if state.ActiveWorkspaceGroupIDs == nil {
			m.menu.SetSelectedGroupIDs([]string{"ungrouped"})
			m.appState.ActiveWorkspaceGroupIDs = []string{"ungrouped"}
		} else {
			m.menu.SetSelectedGroupIDs(state.ActiveWorkspaceGroupIDs)
		}
	}
	m.invalidateSidebarProjection(sidebarProjectionChangeAppState)
	m.syncSidebarExpansionFromAppState()
	m.updateDelegate()
}

func (m *Model) updateDelegate() {
	if m.sidebar != nil {
		m.sidebar.SetActive(m.appState.ActiveWorkspaceID, m.appState.ActiveWorktreeID)
		m.sidebar.SetProviderBadges(m.appState.ProviderBadges)
	}
}

func (m *Model) syncSidebarExpansionFromAppState() {
	if m == nil || m.sidebar == nil {
		return
	}
	m.sidebar.SetExpansionOverrides(m.appState.SidebarWorkspaceExpanded, m.appState.SidebarWorktreeExpanded)
}

func (m *Model) syncAppStateSidebarExpansion() bool {
	if m == nil || m.sidebar == nil {
		return false
	}
	workspaceExpanded, worktreeExpanded := m.sidebar.ExpansionOverrides()
	if mapStringBoolEqual(m.appState.SidebarWorkspaceExpanded, workspaceExpanded) &&
		mapStringBoolEqual(m.appState.SidebarWorktreeExpanded, worktreeExpanded) {
		return false
	}
	m.appState.SidebarWorkspaceExpanded = workspaceExpanded
	m.appState.SidebarWorktreeExpanded = worktreeExpanded
	m.hasAppState = true
	return true
}

func (m *Model) saveAppStateCmd() tea.Cmd {
	if m.stateAPI == nil || !m.hasAppState {
		return nil
	}
	m.syncAppStateComposeHistory()
	m.syncAppStateInputDrafts()
	m.appStateSaveSeq++
	requestSeq := m.appStateSaveSeq
	state := m.appState
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
		defer cancel()
		updated, err := m.stateAPI.UpdateAppState(ctx, &state)
		return appStateSavedMsg{requestSeq: requestSeq, state: updated, err: err}
	}
}

func appStateSaveFlushCmd(requestSeq int) tea.Cmd {
	return tea.Tick(appStateSaveDebounce, func(time.Time) tea.Msg {
		return appStateSaveFlushMsg{requestSeq: requestSeq}
	})
}

func (m *Model) requestAppStateSaveCmd() tea.Cmd {
	if m == nil || m.stateAPI == nil || !m.hasAppState {
		return nil
	}
	m.appStateSaveDirty = true
	if m.appStateSaveInFlight || m.appStateSaveScheduled {
		return nil
	}
	m.appStateSaveScheduled = true
	m.appStateSaveScheduledSeq++
	return appStateSaveFlushCmd(m.appStateSaveScheduledSeq)
}

func (m *Model) flushAppStateSaveCmd(requestSeq int) tea.Cmd {
	if m == nil {
		return nil
	}
	if requestSeq > 0 && requestSeq != m.appStateSaveScheduledSeq {
		return nil
	}
	m.appStateSaveScheduled = false
	if !m.appStateSaveDirty || m.appStateSaveInFlight {
		return nil
	}
	save := m.saveAppStateCmd()
	if save == nil {
		m.appStateSaveDirty = false
		return nil
	}
	m.appStateSaveDirty = false
	m.appStateSaveInFlight = true
	return save
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
		m.workspacePicker.ClearQuery()
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
		m.groupSelectPicker.ClearQuery()
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
		m.groupSelectPicker.ClearQuery()
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
		m.groupSelectPicker.ClearQuery()
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
		m.groupPicker.ClearQuery()
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
		m.workspaceMulti.ClearQuery()
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

func (m *Model) enterRenameForSelection() {
	item := m.selectedItem()
	if item == nil {
		m.setValidationStatus("select an item to rename")
		return
	}
	switch item.kind {
	case sidebarWorkspace:
		if item.workspace == nil || item.workspace.ID == "" || item.workspace.ID == unassignedWorkspaceID {
			m.setValidationStatus("select a workspace to rename")
			return
		}
		m.enterRenameWorkspace(item.workspace.ID)
	case sidebarWorktree:
		if item.worktree == nil || item.worktree.ID == "" || item.worktree.WorkspaceID == "" {
			m.setValidationStatus("select a worktree to rename")
			return
		}
		m.enterRenameWorktree(item.worktree.WorkspaceID, item.worktree.ID)
	case sidebarSession:
		if item.session == nil || item.session.ID == "" {
			m.setValidationStatus("select a session to rename")
			return
		}
		m.enterRenameSession(item.session.ID)
	default:
		m.setValidationStatus("select an item to rename")
	}
}

func (m *Model) enterDismissOrDeleteForSelection() {
	item := m.selectedItem()
	if item == nil {
		m.setValidationStatus("select an item to dismiss or delete")
		return
	}
	switch item.kind {
	case sidebarWorkspace:
		if item.workspace == nil || item.workspace.ID == "" || item.workspace.ID == unassignedWorkspaceID {
			m.setValidationStatus("select a workspace to delete")
			return
		}
		m.confirmDeleteWorkspace(item.workspace.ID)
	case sidebarWorktree:
		if item.worktree == nil || item.worktree.ID == "" || item.worktree.WorkspaceID == "" {
			m.setValidationStatus("select a worktree to delete")
			return
		}
		m.confirmDeleteWorktree(item.worktree.WorkspaceID, item.worktree.ID)
	case sidebarSession:
		if item.session == nil || item.session.ID == "" {
			m.setValidationStatus("select a session to dismiss")
			return
		}
		m.confirmDismissSessions([]string{item.session.ID})
	default:
		m.setValidationStatus("select an item to dismiss or delete")
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

func (m *Model) enterRenameWorktree(workspaceID, id string) {
	m.mode = uiModeRenameWorktree
	m.renameWorktreeWorkspaceID = workspaceID
	m.renameWorktreeID = id
	if m.renameInput != nil {
		name := ""
		if wt := m.worktreeByID(id); wt != nil {
			name = wt.Name
		}
		m.renameInput.SetValue(name)
		m.renameInput.Focus()
	}
	if m.input != nil {
		m.input.FocusChatInput()
	}
	m.setStatusMessage("rename worktree")
}

func (m *Model) exitRenameWorktree(status string) {
	m.mode = uiModeNormal
	if m.renameInput != nil {
		m.renameInput.SetValue("")
		m.renameInput.Blur()
	}
	m.renameWorktreeWorkspaceID = ""
	m.renameWorktreeID = ""
	if m.input != nil {
		m.input.FocusSidebar()
	}
	if status != "" {
		m.setStatusMessage(status)
	}
}

func (m *Model) enterRenameSession(id string) {
	m.mode = uiModeRenameSession
	m.renameSessionID = id
	if m.renameInput != nil {
		name := ""
		var session *types.Session
		for _, candidate := range m.sessions {
			if candidate != nil && candidate.ID == id {
				session = candidate
				break
			}
		}
		name = sessionTitle(session, m.sessionMeta[id])
		m.renameInput.SetValue(name)
		m.renameInput.Focus()
	}
	if m.input != nil {
		m.input.FocusChatInput()
	}
	m.setStatusMessage("rename session")
}

func (m *Model) exitRenameSession(status string) {
	m.mode = uiModeNormal
	if m.renameInput != nil {
		m.renameInput.SetValue("")
		m.renameInput.Blur()
	}
	m.renameSessionID = ""
	if m.input != nil {
		m.input.FocusSidebar()
	}
	if status != "" {
		m.setStatusMessage(status)
	}
}

func (m *Model) enterCompose(sessionID string) {
	if m.mode == uiModeCompose {
		m.saveCurrentComposeDraft()
	}
	m.clearPendingComposeOptionRequest()
	m.mode = uiModeCompose
	m.closeComposeOptionPicker()
	label := m.selectedSessionLabel()
	if m.compose != nil {
		m.compose.Enter(sessionID, label)
	}
	m.resetComposeHistoryCursor()
	if m.chatInput != nil {
		m.chatInput.SetPlaceholder("message")
		m.restoreComposeDraft(sessionID)
		m.chatInput.Focus()
	}
	if m.input != nil {
		m.input.FocusChatInput()
	}
	m.setStatusMessage("compose message")
	m.resize(m.width, m.height)
}

func (m *Model) exitCompose(status string) {
	m.startUILatencyAction(uiLatencyActionExitCompose, "")
	defer m.finishUILatencyAction(uiLatencyActionExitCompose, "", uiLatencyOutcomeOK)

	m.saveCurrentComposeDraft()
	m.clearPendingComposeOptionRequest()
	m.mode = uiModeNormal
	m.closeComposeOptionPicker()
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
	m.clearPendingComposeOptionRequest()
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
	m.clearPendingComposeOptionRequest()
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
	m.newSession.runtimeOptions = m.composeDefaultsForProvider(provider)
	m.resetStream()
	m.mode = uiModeCompose
	m.closeComposeOptionPicker()
	if m.compose != nil {
		m.compose.Enter("", "New session")
	}
	m.setContentText("New session. Send your first message to start.")
	if m.chatInput != nil {
		m.chatInput.SetPlaceholder("new session message")
		m.chatInput.Focus()
	}
	if m.input != nil {
		m.input.FocusChatInput()
	}
	m.setStatusMessage("provider set: " + provider)
	m.resize(m.width, m.height)
	return fetchProviderOptionsCmd(m.sessionAPI, provider)
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
	m.searchIndex = selectSearchIndex(m.searchMatches, m.viewport.YOffset(), 1)
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
		m.searchIndex = selectSearchIndex(m.searchMatches, m.viewport.YOffset(), delta)
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
	request := m.approvalRequestForSession(sessionID, requestID)
	if request == nil && m.pendingApproval != nil {
		pending := m.pendingApproval
		pendingSessionID := strings.TrimSpace(pending.SessionID)
		if pending.RequestID == requestID && (pendingSessionID == "" || strings.EqualFold(pendingSessionID, sessionID)) {
			request = cloneApprovalRequest(pending)
		}
	}
	if strings.EqualFold(strings.TrimSpace(decision), "accept") && approvalRequestNeedsResponse(request) {
		m.enterApprovalResponse(sessionID, request)
		return nil
	}
	m.setStatusMessage("sending approval")
	return approveSessionCmd(m.sessionAPI, sessionID, requestID, decision, nil)
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
	current := m.viewport.YOffset()
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
	outcome := uiLatencyOutcomeOK
	m.startUILatencyAction(uiLatencyActionOpenNewSession, "")
	defer func() {
		m.finishUILatencyAction(uiLatencyActionOpenNewSession, "", outcome)
	}()

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
		outcome = uiLatencyOutcomeValidation
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

func providerSupportsApprovals(provider string) bool {
	return providers.CapabilitiesFor(provider).SupportsApprovals
}

func providerSupportsEvents(provider string) bool {
	return providers.CapabilitiesFor(provider).SupportsEvents
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
	layout := computeStatusLineLayout(width, help, status)
	return layout.render()
}

func statusLineStatusBounds(width int, help, status string) (int, int, bool) {
	layout := computeStatusLineLayout(width, help, status)
	return layout.statusBounds()
}

type statusLineLayout struct {
	width   int
	help    string
	status  string
	padding int
}

func computeStatusLineLayout(width int, help, status string) statusLineLayout {
	layout := statusLineLayout{width: width}
	if width <= 0 {
		layout.help = help
		layout.status = status
		if strings.TrimSpace(help) != "" && strings.TrimSpace(status) != "" {
			layout.padding = statusLinePadding
		}
		return layout
	}

	status = truncateToWidth(status, width)
	statusWidth := lipgloss.Width(status)
	if statusWidth <= 0 {
		layout.help = truncateToWidth(help, width)
		return layout
	}
	layout.status = status

	maxHelpWidth := width - statusWidth - statusLinePadding
	if maxHelpWidth > 0 {
		layout.help = truncateToWidth(help, maxHelpWidth)
	}

	helpWidth := lipgloss.Width(layout.help)
	layout.padding = width - helpWidth - statusWidth
	if helpWidth > 0 && layout.padding < statusLinePadding {
		layout.padding = statusLinePadding
	}
	if helpWidth == 0 && layout.padding < 0 {
		layout.padding = 0
	}

	return layout
}

func (l statusLineLayout) render() string {
	if l.help == "" && l.status == "" {
		return ""
	}
	if l.width <= 0 {
		if l.help == "" {
			return l.status
		}
		if l.status == "" {
			return l.help
		}
		padding := l.padding
		if padding < statusLinePadding {
			padding = statusLinePadding
		}
		return l.help + strings.Repeat(" ", padding) + l.status
	}
	if l.status == "" {
		return l.help
	}
	padding := l.padding
	if padding < 0 {
		padding = 0
	}
	return l.help + strings.Repeat(" ", padding) + l.status
}

func (l statusLineLayout) statusBounds() (int, int, bool) {
	statusWidth := lipgloss.Width(l.status)
	if statusWidth <= 0 {
		return 0, 0, false
	}
	start := lipgloss.Width(l.help) + l.padding
	if l.width > 0 {
		maxCol := l.width - 1
		if maxCol < 0 {
			return 0, 0, false
		}
		if start < 0 {
			start = 0
		}
		if start > maxCol {
			return 0, 0, false
		}
		end := start + statusWidth - 1
		if end > maxCol {
			end = maxCol
		}
		if end < start {
			return 0, 0, false
		}
		return start, end, true
	}
	if start < 0 {
		start = 0
	}
	end := start + statusWidth - 1
	return start, end, true
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

func mapStringBoolEqual(a, b map[string]bool) bool {
	if len(a) != len(b) {
		return false
	}
	for key, value := range a {
		if other, ok := b[key]; !ok || other != value {
			return false
		}
	}
	return true
}

func cloneChatBlockMetaByID(input map[string]ChatBlockMetaPresentation) map[string]ChatBlockMetaPresentation {
	if len(input) == 0 {
		return nil
	}
	cloned := make(map[string]ChatBlockMetaPresentation, len(input))
	for key, meta := range input {
		id := strings.TrimSpace(key)
		if id == "" {
			continue
		}
		copyMeta := ChatBlockMetaPresentation{
			Label: strings.TrimSpace(meta.Label),
		}
		if len(meta.Controls) > 0 {
			copyMeta.Controls = make([]ChatMetaControl, 0, len(meta.Controls))
			for _, control := range meta.Controls {
				label := strings.TrimSpace(control.Label)
				if label == "" {
					continue
				}
				copyMeta.Controls = append(copyMeta.Controls, ChatMetaControl{
					Label: label,
					Tone:  control.Tone,
				})
			}
		}
		cloned[id] = copyMeta
	}
	if len(cloned) == 0 {
		return nil
	}
	return cloned
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
