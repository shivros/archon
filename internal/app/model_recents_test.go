package app

import (
	"context"
	"strings"
	"testing"
	"time"
	"unicode"

	tea "charm.land/bubbletea/v2"
	xansi "github.com/charmbracelet/x/ansi"

	"control/internal/client"
	"control/internal/daemon/transcriptdomain"
	"control/internal/types"
)

type stubInterruptSessionAPI struct {
	calls []string
}

func (s *stubInterruptSessionAPI) ListSessionsWithMeta(context.Context) ([]*types.Session, []*types.SessionMeta, error) {
	return nil, nil, nil
}

func (s *stubInterruptSessionAPI) GetProviderOptions(context.Context, string) (*types.ProviderOptionCatalog, error) {
	return nil, nil
}

func (s *stubInterruptSessionAPI) TailItems(context.Context, string, int) (*client.TailItemsResponse, error) {
	return nil, nil
}

func (s *stubInterruptSessionAPI) History(context.Context, string, int) (*client.TailItemsResponse, error) {
	return nil, nil
}

func (s *stubInterruptSessionAPI) TailStream(context.Context, string, string) (<-chan types.LogEvent, func(), error) {
	return nil, func() {}, nil
}

func (s *stubInterruptSessionAPI) DebugStream(context.Context, string) (<-chan types.DebugEvent, func(), error) {
	return nil, func() {}, nil
}

func (s *stubInterruptSessionAPI) KillSession(context.Context, string) error {
	return nil
}

func (s *stubInterruptSessionAPI) MarkSessionExited(context.Context, string) error {
	return nil
}

func (s *stubInterruptSessionAPI) DismissSession(context.Context, string) error {
	return nil
}

func (s *stubInterruptSessionAPI) UndismissSession(context.Context, string) error {
	return nil
}

func (s *stubInterruptSessionAPI) UpdateSession(context.Context, string, client.UpdateSessionRequest) error {
	return nil
}

func (s *stubInterruptSessionAPI) SendMessage(context.Context, string, client.SendSessionRequest) (*client.SendSessionResponse, error) {
	return nil, nil
}

func (s *stubInterruptSessionAPI) ApproveSession(context.Context, string, client.ApproveSessionRequest) error {
	return nil
}

func (s *stubInterruptSessionAPI) ListApprovals(context.Context, string) ([]*types.Approval, error) {
	return nil, nil
}

func (s *stubInterruptSessionAPI) InterruptSession(_ context.Context, id string) error {
	s.calls = append(s.calls, id)
	return nil
}

func (s *stubInterruptSessionAPI) StartWorkspaceSession(context.Context, string, string, client.StartSessionRequest) (*types.Session, error) {
	return nil, nil
}

func (s *stubInterruptSessionAPI) PinSessionMessage(context.Context, string, client.PinSessionNoteRequest) (*types.Note, error) {
	return nil, nil
}

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

func TestRecentsRunningEntryShowsInterruptControlWhenSupported(t *testing.T) {
	m := NewModel(nil)
	now := time.Now().UTC()
	m.showRecents = true
	m.appState.ActiveWorkspaceGroupIDs = []string{"ungrouped"}
	m.sessions = []*types.Session{
		{ID: "s1", Provider: "codex", Status: types.SessionStatusInactive, CreatedAt: now},
	}
	m.sessionMeta = map[string]*types.SessionMeta{
		"s1": {SessionID: "s1", LastTurnID: "turn-1"},
	}
	m.recents.StartRun("s1", "turn-0", now.Add(-time.Minute))

	block, meta := m.buildRecentsEntryBlock(m.buildRecentsEntry(m.sessions[0], recentsEntryRunning))
	if block.ID != "recents:running:s1" {
		t.Fatalf("unexpected block id %q", block.ID)
	}
	if !containsMetaControl(meta.Controls, recentsControlInterrupt) {
		t.Fatalf("expected interrupt control for running recents entry, got %#v", meta.Controls)
	}
}

func TestReduceRecentsModeInterruptsSelectedRunningSession(t *testing.T) {
	sessionAPI := &stubInterruptSessionAPI{}
	m := NewModel(nil)
	now := time.Now().UTC()
	m.sessionAPI = sessionAPI
	m.showRecents = true
	m.appState.ActiveWorkspaceGroupIDs = []string{"ungrouped"}
	m.sessions = []*types.Session{
		{ID: "s1", Provider: "codex", Status: types.SessionStatusInactive, CreatedAt: now},
	}
	m.sessionMeta = map[string]*types.SessionMeta{
		"s1": {SessionID: "s1", LastTurnID: "turn-1"},
	}
	m.recents.StartRun("s1", "turn-0", now.Add(-time.Minute))
	m.mode = uiModeRecents
	m.recentsSelectedSessionID = "s1"

	handled, cmd := m.reduceRecentsMode(keyRune('i'))
	if !handled {
		t.Fatalf("expected interrupt hotkey to be handled in recents mode")
	}
	if cmd == nil {
		t.Fatalf("expected interrupt command")
	}
	if got := m.status; got != "interrupting s1" {
		t.Fatalf("unexpected status %q", got)
	}
	msg := cmd()
	interrupt, ok := msg.(interruptMsg)
	if !ok {
		t.Fatalf("expected interruptMsg, got %T", msg)
	}
	if interrupt.id != "s1" {
		t.Fatalf("expected interrupt session id s1, got %q", interrupt.id)
	}
	if len(sessionAPI.calls) != 1 || sessionAPI.calls[0] != "s1" {
		t.Fatalf("expected interrupt call for s1, got %#v", sessionAPI.calls)
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

func containsMetaControl(controls []ChatMetaControl, id ChatMetaControlID) bool {
	for _, control := range controls {
		if control.ID == id {
			return true
		}
	}
	return false
}

func TestRecentsReplyRemappedInputNewlineInsertsNewline(t *testing.T) {
	m := NewModel(nil)
	m.applyKeybindings(NewKeybindings(map[string]string{
		KeyCommandInputNewline: "f7",
	}))
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

	handled, _ := m.reduceRecentsMode(tea.KeyPressMsg{Code: tea.KeyF7})
	if !handled {
		t.Fatalf("expected remapped input newline to be handled by recents reply input")
	}
	if got := m.recentsReplyInput.Value(); !strings.Contains(got, "\n") {
		t.Fatalf("expected remapped input newline to insert newline, got %q", got)
	}
}

func TestRecentsReplyClearCommandClearsInput(t *testing.T) {
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
	m.recentsReplyInput.SetValue("reply text")

	handled, cmd := m.reduceRecentsMode(tea.KeyPressMsg{Code: 'c', Mod: tea.ModCtrl})
	if !handled {
		t.Fatalf("expected clear command to be handled by recents reply input")
	}
	if cmd != nil {
		t.Fatalf("expected no command for clear action")
	}
	if got := m.recentsReplyInput.Value(); got != "" {
		t.Fatalf("expected recents reply input to clear, got %q", got)
	}
	if m.recentsReplySessionID != "s1" {
		t.Fatalf("expected recents reply target to remain active, got %q", m.recentsReplySessionID)
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
	if !strings.Contains(meta.PrimaryLabel, "Workspace / feature/refactor") {
		t.Fatalf("expected recents location to include worktree, got %q", meta.PrimaryLabel)
	}
	if meta.Label != "Running" {
		t.Fatalf("expected recents secondary metadata line to contain running status, got %q", meta.Label)
	}
}

func TestRecentsMetadataUsesPrimaryAndSecondaryLines(t *testing.T) {
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
	m.recentsPreviews = map[string]recentsPreview{
		"s1": {Revision: "turn-1", Preview: "assistant preview"},
	}
	m.mode = uiModeRecents
	m.recentsSelectedSessionID = "s1"

	m.refreshRecentsContent()
	plain := xansi.Strip(m.renderedText)
	lines := strings.Split(plain, "\n")
	primaryLine := ""
	secondaryLine := ""
	for _, line := range lines {
		if strings.Contains(line, "Workspace") && strings.Contains(line, "•") {
			primaryLine = line
		}
		if strings.Contains(line, "Running") && strings.Contains(line, "[Reply]") {
			secondaryLine = line
		}
	}
	if primaryLine == "" {
		t.Fatalf("expected primary metadata line with session and location, got %q", plain)
	}
	if strings.Contains(primaryLine, "[Reply]") {
		t.Fatalf("expected controls on secondary metadata line, got primary %q", primaryLine)
	}
	if secondaryLine == "" {
		t.Fatalf("expected secondary metadata line with status and controls, got %q", plain)
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

func TestRecentsTurnCompletedMessageWithoutMatchedSignalDoesNotCompleteRun(t *testing.T) {
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
		turnID:       "",
		matched:      false,
	})
	if !handled {
		t.Fatalf("expected recents completion message to be handled")
	}
	if cmd != nil {
		t.Fatalf("expected no persistence command when completion signal is unmatched")
	}
	if !m.recents.IsRunning("s1") {
		t.Fatalf("expected s1 to remain running without a matched completion signal")
	}
	if m.recents.IsReady("s1") {
		t.Fatalf("expected s1 to remain out of ready without a matched completion signal")
	}
	if _, watching := m.recentsCompletionWatching["s1"]; watching {
		t.Fatalf("expected completion watcher to clear")
	}
}

func TestRecentsTurnCompletedMessageUsesMetaFallbackWhenPolicyAllows(t *testing.T) {
	m := NewModel(nil)
	now := time.Now().UTC()
	m.sessions = []*types.Session{
		{ID: "s1", Provider: "codex", Status: types.SessionStatusRunning, CreatedAt: now},
	}
	m.sessionMeta = map[string]*types.SessionMeta{
		"s1": {SessionID: "s1", LastTurnID: "turn-meta"},
	}
	m.recents.StartRun("s1", "turn-42", now)
	m.recentsCompletionWatching["s1"] = "turn-42"
	policy := &recentsModelCompletionPolicyStub{
		shouldUseMetaFallback: true,
		completionTurnID:      "turn-meta",
	}
	m.recentsCompletionPolicy = policy

	handled, cmd := m.reduceStateMessages(recentsTurnCompletedMsg{
		id:           "s1",
		expectedTurn: "turn-42",
		matched:      true,
	})
	if !handled {
		t.Fatalf("expected recents completion message to be handled")
	}
	if cmd == nil {
		t.Fatalf("expected persistence command when fallback resolves completion turn id")
	}
	if policy.completionCalls != 1 {
		t.Fatalf("expected completion fallback call, got %d", policy.completionCalls)
	}
	if !m.recents.IsReady("s1") {
		t.Fatalf("expected s1 to move into ready via meta fallback completion")
	}
}

func TestRecentsTurnCompletedMessageSkipsMetaFallbackWhenPolicyDisallows(t *testing.T) {
	m := NewModel(nil)
	now := time.Now().UTC()
	m.sessions = []*types.Session{
		{ID: "s1", Provider: "codex", Status: types.SessionStatusRunning, CreatedAt: now},
	}
	m.sessionMeta = map[string]*types.SessionMeta{
		"s1": {SessionID: "s1", LastTurnID: "turn-meta"},
	}
	m.recents.StartRun("s1", "turn-42", now)
	policy := &recentsModelCompletionPolicyStub{
		shouldUseMetaFallback: false,
		completionTurnID:      "turn-meta",
	}
	m.recentsCompletionPolicy = policy

	handled, cmd := m.reduceStateMessages(recentsTurnCompletedMsg{
		id:           "s1",
		expectedTurn: "turn-42",
		matched:      true,
	})
	if !handled {
		t.Fatalf("expected recents completion message to be handled")
	}
	if cmd != nil {
		t.Fatalf("expected no persistence command when fallback is disallowed")
	}
	if policy.completionCalls != 0 {
		t.Fatalf("expected completion fallback to be skipped, got %d calls", policy.completionCalls)
	}
	if !m.recents.IsRunning("s1") {
		t.Fatalf("expected s1 to remain running when fallback is disallowed")
	}
	if m.recents.IsReady("s1") {
		t.Fatalf("expected s1 to remain out of ready when fallback is disallowed")
	}
}

func TestBeginRecentsCompletionWatchUsesSignalPolicyFromModel(t *testing.T) {
	m := NewModel(nil)
	now := time.Now().UTC()
	m.sessions = []*types.Session{
		{ID: "s1", Provider: "codex", Status: types.SessionStatusRunning, CreatedAt: now},
	}
	stream := make(chan transcriptdomain.TranscriptEvent, 1)
	stream <- transcriptdomain.TranscriptEvent{Kind: transcriptdomain.TranscriptEventDelta}
	close(stream)
	m.sessionTranscriptAPI = recentsModelTranscriptAPIStub{stream: stream}
	m.recentsCompletionSignalPolicy = recentsModelSignalPolicyStub{
		matchKind: transcriptdomain.TranscriptEventDelta,
		turnID:    "turn-from-model-policy",
	}

	cmd := m.beginRecentsCompletionWatch("s1", "turn-expected")
	if cmd == nil {
		t.Fatalf("expected completion watch command")
	}
	msg, ok := cmd().(recentsTurnCompletedMsg)
	if !ok {
		t.Fatalf("expected recentsTurnCompletedMsg")
	}
	if msg.err != nil {
		t.Fatalf("expected no error, got %v", msg.err)
	}
	if msg.turnID != "turn-from-model-policy" {
		t.Fatalf("expected signal policy turn id, got %q", msg.turnID)
	}
}

func TestHandleRecentsCompletionSignalGuards(t *testing.T) {
	var nilModel *Model
	if cmd := nilModel.handleRecentsCompletionSignal("s1", "turn-1"); cmd != nil {
		t.Fatalf("expected nil model guard to return nil command")
	}

	m := NewModel(nil)
	m.recents = nil
	if cmd := m.handleRecentsCompletionSignal("s1", "turn-1"); cmd != nil {
		t.Fatalf("expected nil recents guard to return nil command")
	}

	m = NewModel(nil)
	if cmd := m.handleRecentsCompletionSignal(" ", "turn-1"); cmd != nil {
		t.Fatalf("expected blank session id guard to return nil command")
	}
	if cmd := m.handleRecentsCompletionSignal("s1", "turn-1"); cmd != nil {
		t.Fatalf("expected no-watcher guard to return nil command")
	}
}

func TestHandleRecentsCompletionSignalCompletesWatchedRun(t *testing.T) {
	m := NewModel(nil)
	now := time.Now().UTC()
	m.recents.StartRun("s1", "turn-0", now)
	m.recentsCompletionWatching["s1"] = "turn-0"

	cmd := m.handleRecentsCompletionSignal("s1", "turn-1")
	if !m.recents.IsReady("s1") {
		t.Fatalf("expected watched run to move to ready after completion signal")
	}
	if _, ok := m.recentsCompletionWatching["s1"]; ok {
		t.Fatalf("expected watcher to be removed after completion signal")
	}
	if cmd == nil {
		t.Fatalf("expected completion to request persistence command")
	}
}

func TestRecentsAndComposeObserveSameSessionWithoutDuplicateTranscriptAttach(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	api := newCountingTranscriptAPIStub("s1")
	m.sessionTranscriptAPI = api
	m.enterCompose("s1")

	streamMsg := openTranscriptStreamCmd(api, "s1", "")().(transcriptStreamMsg)
	m.applyTranscriptStreamMsg(streamMsg)
	if api.OpenCount("s1") != 1 {
		t.Fatalf("expected one compose transcript attach, got %d", api.OpenCount("s1"))
	}

	stream := api.Stream("s1")
	stream <- transcriptdomain.TranscriptEvent{
		Kind:     transcriptdomain.TranscriptEventDelta,
		Revision: transcriptdomain.MustParseRevisionToken("1"),
		Delta: []transcriptdomain.Block{{
			Kind: "assistant_message",
			Role: "assistant",
			Text: "compose-visible output",
		}},
	}
	if cmd := m.consumeTranscriptTick(time.Now()); cmd != nil {
		_ = cmd()
	}

	m.showRecents = true
	m.enterRecentsView(&sidebarItem{kind: sidebarRecentsAll})
	preview := m.previewForSession("s1", "")
	if strings.TrimSpace(preview.Preview) == "" {
		t.Fatalf("expected recents to observe compose transcript cache for same session")
	}

	if cmd := m.beginRecentsCompletionWatch("s1", "turn-0"); cmd != nil {
		t.Fatalf("expected recents to reuse shared transcript follow when compose stream is active")
	}
	if api.OpenCount("s1") != 1 {
		t.Fatalf("expected no duplicate transcript attach for compose+recents observers, got %d", api.OpenCount("s1"))
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

type recentsModelTranscriptAPIStub struct {
	stream <-chan transcriptdomain.TranscriptEvent
}

func (s recentsModelTranscriptAPIStub) GetTranscriptSnapshot(context.Context, string, int) (*client.TranscriptSnapshotResponse, error) {
	return &client.TranscriptSnapshotResponse{}, nil
}

func (s recentsModelTranscriptAPIStub) TranscriptStream(context.Context, string, string) (<-chan transcriptdomain.TranscriptEvent, func(), error) {
	return s.stream, func() {}, nil
}

type recentsModelSignalPolicyStub struct {
	matchKind transcriptdomain.TranscriptEventKind
	turnID    string
}

func (s recentsModelSignalPolicyStub) CompletionFromTranscriptEvent(event transcriptdomain.TranscriptEvent) (string, bool) {
	if event.Kind == s.matchKind {
		return s.turnID, true
	}
	return "", false
}

type recentsModelCompletionPolicyStub struct {
	shouldUseMetaFallback bool
	completionTurnID      string
	completionCalls       int
}

func (recentsModelCompletionPolicyStub) ShouldWatchCompletion(string) bool { return true }

func (s recentsModelCompletionPolicyStub) RunBaselineTurnID(sendTurnID string, _ *types.SessionMeta) string {
	return strings.TrimSpace(sendTurnID)
}

func (s *recentsModelCompletionPolicyStub) ShouldUseMetaFallback(string) bool {
	if s == nil {
		return false
	}
	return s.shouldUseMetaFallback
}

func (s *recentsModelCompletionPolicyStub) CompletionTurnID(_ string, _ *types.SessionMeta) string {
	if s == nil {
		return ""
	}
	s.completionCalls++
	return strings.TrimSpace(s.completionTurnID)
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
