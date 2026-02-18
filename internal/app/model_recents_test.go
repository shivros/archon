package app

import (
	"strings"
	"testing"
	"time"
	"unicode"

	tea "charm.land/bubbletea/v2"
	xansi "github.com/charmbracelet/x/ansi"

	"control/internal/types"
)

func TestApplySelectionStateEntersRecentsMode(t *testing.T) {
	m := NewModel(nil)
	handled, _, _ := m.applySelectionState(&sidebarItem{kind: sidebarRecentsAll})
	if !handled {
		t.Fatalf("expected recents selection to be handled")
	}
	if m.mode != uiModeRecents {
		t.Fatalf("expected recents mode, got %v", m.mode)
	}
	if len(m.contentBlocks) == 0 {
		t.Fatalf("expected recents blocks to render")
	}
	meta, ok := m.contentBlockMetaByID["recents:help"]
	if !ok {
		t.Fatalf("expected recents help metadata to be present")
	}
	if !strings.Contains(meta.Label, "Recents overview") {
		t.Fatalf("expected recents overview block meta, got %q", meta.Label)
	}
}

func TestDismissSelectedRecentsReadyRemovesQueueItem(t *testing.T) {
	m := NewModel(nil)
	now := time.Now().UTC()
	m.showRecents = true
	m.appState.ActiveWorkspaceGroupIDs = []string{"ungrouped"}
	m.workspaces = []*types.Workspace{
		{ID: "ws1", Name: "Workspace"},
	}
	m.sessions = []*types.Session{
		{ID: "s1", Provider: "codex", Status: types.SessionStatusRunning, CreatedAt: now},
	}
	m.sessionMeta = map[string]*types.SessionMeta{
		"s1": {SessionID: "s1", WorkspaceID: "ws1", LastTurnID: "turn-a1"},
	}
	m.recents.StartRun("s1", "turn-u1", now.Add(-time.Minute))
	m.recents.ObserveMeta(m.sessionMeta, now)
	m.mode = uiModeRecents
	m.recentsSelectedSessionID = "s1"

	if !m.dismissSelectedRecentsReady() {
		t.Fatalf("expected dismiss to succeed")
	}
	if m.recents.IsReady("s1") {
		t.Fatalf("expected s1 to be removed from ready queue")
	}
}

func TestRecentsCardRendersControlsAboveBubble(t *testing.T) {
	m := NewModel(nil)
	now := time.Now().UTC()
	m.showRecents = true
	m.width = 120
	m.height = 40
	m.viewport.SetWidth(90)
	m.appState.ActiveWorkspaceGroupIDs = []string{"ungrouped"}
	m.workspaces = []*types.Workspace{
		{ID: "ws1", Name: "Workspace"},
	}
	m.sessions = []*types.Session{
		{ID: "s1", Provider: "codex", Status: types.SessionStatusRunning, CreatedAt: now},
	}
	m.sessionMeta = map[string]*types.SessionMeta{
		"s1": {SessionID: "s1", WorkspaceID: "ws1", LastTurnID: "turn-1"},
	}
	m.recents.StartRun("s1", "turn-0", now.Add(-time.Minute))
	m.recentsSelectedSessionID = "s1"
	m.recentsPreviews = map[string]recentsPreview{
		"s1": {Revision: "turn-1", Preview: "assistant preview"},
	}
	m.mode = uiModeRecents
	m.refreshRecentsContent()
	plain := xansi.Strip(m.renderedText)
	replyIndex := strings.Index(plain, "[Reply]")
	bubbleIndex := strings.Index(plain, "assistant preview")
	if replyIndex < 0 {
		t.Fatalf("expected reply control in recents card, got %q", plain)
	}
	if bubbleIndex < 0 {
		t.Fatalf("expected assistant preview text in recents bubble, got %q", plain)
	}
	if replyIndex > bubbleIndex {
		t.Fatalf("expected controls above bubble text, got %q", plain)
	}
	if !strings.Contains(m.renderedText, "\x1b[") {
		t.Fatalf("expected ANSI-styled recents rendering")
	}
	if strings.Contains(plain, "[38;5;") || strings.Contains(plain, "[0m") {
		t.Fatalf("expected no leaked ANSI fragments in plain output, got %q", plain)
	}
}

func TestStartRecentsReplyUsesSharedMultilineInputStyle(t *testing.T) {
	m := NewModel(nil)
	now := time.Now().UTC()
	m.showRecents = true
	m.appState.ActiveWorkspaceGroupIDs = []string{"ungrouped"}
	m.workspaces = []*types.Workspace{{ID: "ws1", Name: "Workspace"}}
	m.sessions = []*types.Session{
		{ID: "s1", Provider: "codex", Status: types.SessionStatusRunning, CreatedAt: now},
	}
	m.sessionMeta = map[string]*types.SessionMeta{
		"s1": {SessionID: "s1", WorkspaceID: "ws1", LastTurnID: "turn-1"},
	}
	m.recents.StartRun("s1", "turn-0", now.Add(-time.Minute))
	m.mode = uiModeRecents
	m.recentsSelectedSessionID = "s1"

	if !m.startRecentsReply() {
		t.Fatalf("expected to start recents reply")
	}
	if m.chatInput == nil || m.recentsReplyInput == nil {
		t.Fatalf("expected chat and recents reply inputs")
	}
	if got, want := m.recentsReplyInput.Height(), m.chatInput.Height(); got != want {
		t.Fatalf("expected recents reply input height to match chat input style, got %d want %d", got, want)
	}
}

func TestRecentsReplyShiftEnterInsertsNewline(t *testing.T) {
	m := NewModel(nil)
	now := time.Now().UTC()
	m.showRecents = true
	m.appState.ActiveWorkspaceGroupIDs = []string{"ungrouped"}
	m.workspaces = []*types.Workspace{{ID: "ws1", Name: "Workspace"}}
	m.sessions = []*types.Session{
		{ID: "s1", Provider: "codex", Status: types.SessionStatusRunning, CreatedAt: now},
	}
	m.sessionMeta = map[string]*types.SessionMeta{
		"s1": {SessionID: "s1", WorkspaceID: "ws1", LastTurnID: "turn-1"},
	}
	m.recents.StartRun("s1", "turn-0", now.Add(-time.Minute))
	m.mode = uiModeRecents
	m.recentsSelectedSessionID = "s1"
	if !m.startRecentsReply() {
		t.Fatalf("expected to start recents reply")
	}

	handled, _ := m.reduceRecentsMode(tea.KeyPressMsg{Code: tea.KeyEnter, Mod: tea.ModShift})
	if !handled {
		t.Fatalf("expected shift+enter to be handled by recents reply input")
	}
	if got := m.recentsReplyInput.Value(); !strings.Contains(got, "\n") {
		t.Fatalf("expected shift+enter to insert newline, got %q", got)
	}
}

func TestRecentsEntryShowsWorktreeInLocationLabel(t *testing.T) {
	m := NewModel(nil)
	now := time.Now().UTC()
	m.showRecents = true
	m.width = 120
	m.height = 40
	m.viewport.SetWidth(90)
	m.appState.ActiveWorkspaceGroupIDs = []string{"ungrouped"}
	m.workspaces = []*types.Workspace{
		{ID: "ws1", Name: "Workspace"},
	}
	m.worktrees = map[string][]*types.Worktree{
		"ws1": {
			{ID: "wt1", WorkspaceID: "ws1", Name: "feature/refactor"},
		},
	}
	m.sessions = []*types.Session{
		{ID: "s1", Provider: "codex", Status: types.SessionStatusRunning, CreatedAt: now},
	}
	m.sessionMeta = map[string]*types.SessionMeta{
		"s1": {SessionID: "s1", WorkspaceID: "ws1", WorktreeID: "wt1", LastTurnID: "turn-1"},
	}
	m.recents.StartRun("s1", "turn-0", now.Add(-time.Minute))
	m.recentsPreviews = map[string]recentsPreview{
		"s1": {Revision: "turn-1", Preview: "assistant preview"},
	}
	m.mode = uiModeRecents

	m.refreshRecentsContent()
	meta, ok := m.contentBlockMetaByID["recents:running:s1"]
	if !ok {
		t.Fatalf("expected recents running block metadata")
	}
	if !strings.Contains(meta.Label, "Workspace / feature/refactor") {
		t.Fatalf("expected recents location to include worktree, got %q", meta.Label)
	}
}

func TestRecentsTurnCompletedMessageMovesRunToReady(t *testing.T) {
	m := NewModel(nil)
	now := time.Now().UTC()
	m.sessions = []*types.Session{
		{ID: "s1", Provider: "codex", Status: types.SessionStatusRunning, CreatedAt: now},
	}
	m.sessionMeta = map[string]*types.SessionMeta{
		"s1": {SessionID: "s1", LastTurnID: "turn-42"},
	}
	m.recents.StartRun("s1", "turn-42", now)
	m.recentsCompletionWatching["s1"] = "turn-42"

	handled, cmd := m.reduceStateMessages(recentsTurnCompletedMsg{
		id:           "s1",
		expectedTurn: "turn-42",
		turnID:       "turn-42",
	})
	if !handled {
		t.Fatalf("expected recents completion message to be handled")
	}
	if cmd == nil {
		t.Fatalf("expected recents completion to request app-state persistence")
	}
	if _, ok := cmd().(appStateSaveFlushMsg); !ok {
		t.Fatalf("expected app-state save debounce command, got %T", cmd())
	}
	if m.recents.IsRunning("s1") {
		t.Fatalf("expected s1 to leave running after completion")
	}
	if !m.recents.IsReady("s1") {
		t.Fatalf("expected s1 to move into ready")
	}
	if _, watching := m.recentsCompletionWatching["s1"]; watching {
		t.Fatalf("expected completion watcher to clear")
	}
}

func TestFormatRecentsPreviewTextRemovesANSIEscapeFragments(t *testing.T) {
	preview, full := formatRecentsPreviewText("\x1b[38;5;117mhello\x1b[0m\nworld")
	if strings.TrimSpace(full) == "" {
		t.Fatalf("expected full text to be retained")
	}
	if strings.Contains(preview, "[38;5;117m") || strings.Contains(preview, "[0m") {
		t.Fatalf("expected preview to strip ANSI escape fragments, got %q", preview)
	}
	if !strings.Contains(preview, "hello world") {
		t.Fatalf("expected flattened preview text, got %q", preview)
	}
}

func TestRecentsSwitchFilterUsesSidebarRows(t *testing.T) {
	m := NewModel(nil)
	m.showRecents = true
	m.appState.ActiveWorkspaceGroupIDs = []string{"ungrouped"}
	m.applySidebarItems()
	m.enterRecentsView(&sidebarItem{kind: sidebarRecentsAll})

	_ = m.switchRecentsFilter(sidebarRecentsFilterRunning)
	if got := m.recentsFilter(); got != sidebarRecentsFilterRunning {
		t.Fatalf("expected running filter after switch, got %q", got)
	}
}

func TestRecentsKeyTabCyclesFilters(t *testing.T) {
	m := NewModel(nil)
	m.showRecents = true
	m.appState.ActiveWorkspaceGroupIDs = []string{"ungrouped"}
	m.applySidebarItems()
	m.enterRecentsView(&sidebarItem{kind: sidebarRecentsAll})

	handled, _ := m.reduceRecentsMode(tea.KeyPressMsg{Code: tea.KeyTab})
	if !handled {
		t.Fatalf("expected tab to be handled in recents mode")
	}
	if got := m.recentsFilter(); got != sidebarRecentsFilterReady {
		t.Fatalf("expected ready filter after tab cycle, got %q", got)
	}
}

func TestRecentsEmptySectionTextIsContextual(t *testing.T) {
	m := NewModel(nil)
	m.showRecents = true
	m.appState.ActiveWorkspaceGroupIDs = []string{"ungrouped"}
	m.applySidebarItems()

	_ = m.switchRecentsFilter(sidebarRecentsFilterReady)
	m.enterRecentsView(&sidebarItem{kind: sidebarRecentsReady})
	m.refreshRecentsContent()
	plain := normalizeWhitespace(xansi.Strip(m.renderedText))
	if !strings.Contains(plain, "No ready") || !strings.Contains(plain, "waiting for") || !strings.Contains(plain, "reply.") {
		t.Fatalf("expected contextual empty-state text for ready section, got %q", plain)
	}
}

func normalizeWhitespace(value string) string {
	var b strings.Builder
	lastSpace := false
	for _, r := range value {
		if unicode.IsSpace(r) {
			if lastSpace {
				continue
			}
			b.WriteRune(' ')
			lastSpace = true
			continue
		}
		b.WriteRune(r)
		lastSpace = false
	}
	return strings.TrimSpace(b.String())
}
