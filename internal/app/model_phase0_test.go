package app

import (
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"control/internal/types"
)

func TestPhase0ComposeSendKeepsLocalState(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	m.enterCompose("s1")
	m.chatInput.SetValue("hello from compose")

	nextModel, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	next := asModel(t, nextModel)

	if cmd == nil {
		t.Fatalf("expected send command")
	}
	if next.status != "sending message" {
		t.Fatalf("expected sending status, got %q", next.status)
	}
	if got := next.chatInput.Value(); got != "" {
		t.Fatalf("expected chat input to be cleared, got %q", got)
	}

	history := next.composeHistory["s1"]
	if history == nil || len(history.entries) != 1 || history.entries[0] != "hello from compose" {
		t.Fatalf("expected compose history entry, got %#v", history)
	}

	entry, ok := next.pendingSends[1]
	if !ok {
		t.Fatalf("expected pending send token 1 to be registered")
	}
	if entry.sessionID != "s1" || entry.provider != "codex" {
		t.Fatalf("unexpected pending send entry: %#v", entry)
	}

	blocks := next.currentBlocks()
	if len(blocks) == 0 {
		t.Fatalf("expected local transcript to include user message")
	}
	if blocks[0].Role != ChatRoleUser || blocks[0].Status != ChatStatusSending || blocks[0].Text != "hello from compose" {
		t.Fatalf("unexpected first block: %#v", blocks[0])
	}
}

func TestPhase0ScheduleSessionLoadDebouncesSelection(t *testing.T) {
	m := NewModel(nil)
	m.selectSeq = 41
	item := &sidebarItem{
		kind:    sidebarSession,
		session: &types.Session{ID: "s1"},
	}

	cmd := m.scheduleSessionLoad(item, time.Millisecond)
	if cmd == nil {
		t.Fatalf("expected debounce command")
	}
	msg := cmd()
	selectMsg, ok := msg.(selectDebounceMsg)
	if !ok {
		t.Fatalf("expected selectDebounceMsg, got %T", msg)
	}
	if selectMsg.id != "s1" || selectMsg.seq != 42 {
		t.Fatalf("unexpected debounce payload: %#v", selectMsg)
	}
}

func TestPhase0LoadSelectedSessionResetsApprovalAndUsesCache(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	item := m.selectedItem()
	if item == nil || item.session == nil {
		t.Fatalf("expected selected session")
	}

	m.pendingApproval = &ApprovalRequest{RequestID: 99}
	m.pendingSessionKey = "stale"
	m.loading = true
	m.loadingKey = "stale"
	m.transcriptCache[item.key()] = []ChatBlock{{Role: ChatRoleAgent, Text: "cached reply"}}

	cmd := m.loadSelectedSession(item)
	if cmd == nil {
		t.Fatalf("expected load command")
	}
	if m.pendingApproval != nil {
		t.Fatalf("expected pending approval to be cleared")
	}
	if m.pendingSessionKey != item.key() {
		t.Fatalf("expected pending key %q, got %q", item.key(), m.pendingSessionKey)
	}
	if m.status != "loading s1" {
		t.Fatalf("expected loading status, got %q", m.status)
	}
	if m.loading {
		t.Fatalf("expected loading=false when cache exists")
	}
	if m.loadingKey != "" {
		t.Fatalf("expected loading key to clear, got %q", m.loadingKey)
	}
	blocks := m.currentBlocks()
	if len(blocks) != 1 || blocks[0].Text != "cached reply" {
		t.Fatalf("expected cached blocks to be applied, got %#v", blocks)
	}
}

func TestPhase0SelectApprovalRequestChoosesLatest(t *testing.T) {
	older := &types.Approval{
		SessionID: "s1",
		RequestID: 1,
		Method:    "item/commandExecution/requestApproval",
		Params:    []byte(`{"parsedCmd":"echo older"}`),
		CreatedAt: time.Date(2026, 1, 1, 10, 0, 0, 0, time.UTC),
	}
	newer := &types.Approval{
		SessionID: "s1",
		RequestID: 2,
		Method:    "item/commandExecution/requestApproval",
		Params:    []byte(`{"parsedCmd":"go test ./..."}`),
		CreatedAt: time.Date(2026, 1, 1, 11, 0, 0, 0, time.UTC),
	}

	req := selectApprovalRequest([]*types.Approval{older, nil, newer})
	if req == nil {
		t.Fatalf("expected approval request")
	}
	if req.RequestID != 2 || req.Summary != "command" || req.Detail != "go test ./..." {
		t.Fatalf("unexpected approval request: %#v", req)
	}
}

func TestPhase0ConsumeCodexTickSetsPendingApprovalStatus(t *testing.T) {
	m := NewModel(nil)
	requestID := 7

	events := make(chan types.CodexEvent, 1)
	events <- types.CodexEvent{
		ID:     &requestID,
		Method: "item/commandExecution/requestApproval",
		Params: []byte(`{"parsedCmd":"go test ./..."}`),
	}
	close(events)

	m.codexStream.SetStream(events, nil)
	m.consumeCodexTick()

	if m.pendingApproval == nil {
		t.Fatalf("expected pending approval to be visible")
	}
	if m.pendingApproval.RequestID != 7 {
		t.Fatalf("expected request id 7, got %d", m.pendingApproval.RequestID)
	}
	if m.pendingApproval.Summary != "command" || m.pendingApproval.Detail != "go test ./..." {
		t.Fatalf("unexpected pending approval: %#v", m.pendingApproval)
	}
	if m.status != "approval required: command (go test ./...)" {
		t.Fatalf("unexpected status %q", m.status)
	}
}

func newPhase0ModelWithSession(provider string) Model {
	m := NewModel(nil)
	now := time.Now().UTC()
	m.appState.ActiveWorkspaceGroupIDs = []string{"ungrouped"}
	m.workspaces = []*types.Workspace{
		{ID: "ws1", Name: "Workspace"},
	}
	m.sessions = []*types.Session{
		{
			ID:        "s1",
			Provider:  provider,
			Status:    types.SessionStatusRunning,
			CreatedAt: now,
			Title:     "Session",
		},
	}
	m.sessionMeta = map[string]*types.SessionMeta{
		"s1": {SessionID: "s1", WorkspaceID: "ws1"},
	}
	m.applySidebarItems()
	return m
}

func asModel(t *testing.T, model tea.Model) Model {
	t.Helper()
	v, ok := model.(*Model)
	if !ok {
		t.Fatalf("expected *app.Model update result, got %T", model)
		return Model{}
	}
	return *v
}
