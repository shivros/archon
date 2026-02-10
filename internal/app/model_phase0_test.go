package app

import (
	"strings"
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

func TestPhase0ConsumeCodexTickKeepsApprovalOrderWithLaterAgentMessages(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	if m.sidebar == nil || !m.sidebar.SelectBySessionID("s1") {
		t.Fatalf("expected selected session")
	}
	m.codexStream.SetSnapshotBlocks([]ChatBlock{{Role: ChatRoleUser, Text: "user"}})

	req1 := 1
	req2 := 2
	events := make(chan types.CodexEvent, 16)
	events <- types.CodexEvent{
		Method: "item/started",
		Params: []byte(`{"item":{"type":"agentMessage","id":"agent-1"}}`),
	}
	events <- types.CodexEvent{Method: "item/agentMessage/delta", Params: []byte(`{"delta":"agent one"}`)}
	events <- types.CodexEvent{Method: "item/completed", Params: []byte(`{"item":{"type":"agentMessage"}}`)}
	events <- types.CodexEvent{
		ID:     &req1,
		Method: "item/commandExecution/requestApproval",
		Params: []byte(`{"parsedCmd":"cmd one"}`),
	}
	events <- types.CodexEvent{
		ID:     &req2,
		Method: "item/commandExecution/requestApproval",
		Params: []byte(`{"parsedCmd":"cmd two"}`),
	}
	events <- types.CodexEvent{
		Method: "item/started",
		Params: []byte(`{"item":{"type":"agentMessage","id":"agent-2"}}`),
	}
	events <- types.CodexEvent{Method: "item/agentMessage/delta", Params: []byte(`{"delta":"agent two"}`)}
	events <- types.CodexEvent{Method: "item/completed", Params: []byte(`{"item":{"type":"agentMessage"}}`)}
	close(events)

	m.codexStream.SetStream(events, nil)
	m.consumeCodexTick()

	blocks := m.currentBlocks()
	if len(blocks) != 5 {
		t.Fatalf("expected 5 blocks, got %#v", blocks)
	}
	expected := []ChatRole{ChatRoleUser, ChatRoleAgent, ChatRoleApproval, ChatRoleApproval, ChatRoleAgent}
	for i, want := range expected {
		if blocks[i].Role != want {
			t.Fatalf("unexpected role order at %d: got %s want %s (blocks=%#v)", i, blocks[i].Role, want, blocks)
		}
	}
	if blocks[2].RequestID != 1 || blocks[3].RequestID != 2 {
		t.Fatalf("unexpected approval order: %#v", blocks)
	}
}

func TestPhase0ApprovePendingAllowsRequestIDZero(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	if m.sidebar == nil || !m.sidebar.SelectBySessionID("s1") {
		t.Fatalf("expected selected session")
	}
	m.pendingApproval = &ApprovalRequest{RequestID: 0, Summary: "command"}

	cmd := m.approvePending("accept")
	if cmd == nil {
		t.Fatalf("expected approval command")
	}
	if m.status != "sending approval" {
		t.Fatalf("unexpected status %q", m.status)
	}
}

func TestPhase0ApprovePendingUsesApprovalSessionWhenSidebarNotOnSession(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	if m.sidebar == nil || !m.sidebar.SelectBySessionID("s1") {
		t.Fatalf("expected selected session")
	}
	items := m.sidebar.Items()
	for i, item := range items {
		entry, ok := item.(*sidebarItem)
		if !ok || entry == nil || entry.kind != sidebarWorkspace {
			continue
		}
		m.sidebar.Select(i)
		break
	}
	if m.selectedSessionID() != "" {
		t.Fatalf("expected no selected session")
	}
	m.pendingApproval = &ApprovalRequest{RequestID: 4, SessionID: "s1", Summary: "command"}

	cmd := m.approvePending("accept")
	if cmd == nil {
		t.Fatalf("expected approval command")
	}
	if m.status != "sending approval" {
		t.Fatalf("unexpected status %q", m.status)
	}
}

func TestPhase0ConsumeCodexTickAcceptsApprovalRequestIDZero(t *testing.T) {
	m := NewModel(nil)
	requestID := 0

	events := make(chan types.CodexEvent, 1)
	events <- types.CodexEvent{
		ID:     &requestID,
		Method: "item/commandExecution/requestApproval",
		Params: []byte(`{"parsedCmd":"echo ok"}`),
	}
	close(events)

	m.codexStream.SetStream(events, nil)
	m.consumeCodexTick()

	if m.pendingApproval == nil {
		t.Fatalf("expected pending approval to be visible")
	}
	if m.pendingApproval.RequestID != 0 {
		t.Fatalf("expected request id 0, got %d", m.pendingApproval.RequestID)
	}
}

func TestPhase0ApprovalsMsgAddsApprovalBlock(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	if m.sidebar == nil || !m.sidebar.SelectBySessionID("s1") {
		t.Fatalf("expected selected session")
	}
	m.applyBlocks([]ChatBlock{{Role: ChatRoleAgent, Text: "reply"}})

	handled, _ := m.reduceStateMessages(approvalsMsg{
		id: "s1",
		approvals: []*types.Approval{
			{
				SessionID: "s1",
				RequestID: 0,
				Method:    "item/commandExecution/requestApproval",
				Params:    []byte(`{"parsedCmd":"go test ./..."}`),
				CreatedAt: time.Date(2026, 1, 1, 11, 0, 0, 0, time.UTC),
			},
		},
	})
	if !handled {
		t.Fatalf("expected approvals msg to be handled")
	}
	blocks := m.currentBlocks()
	if len(blocks) < 2 {
		t.Fatalf("expected approval block to be appended, got %#v", blocks)
	}
	last := blocks[len(blocks)-1]
	if last.Role != ChatRoleApproval || last.RequestID != 0 {
		t.Fatalf("unexpected approval block: %#v", last)
	}
}

func TestPhase0ApprovalMsgReplacesPendingWithResolvedMarker(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	if m.sidebar == nil || !m.sidebar.SelectBySessionID("s1") {
		t.Fatalf("expected selected session")
	}
	m.setApprovalsForSession("s1", []*ApprovalRequest{
		{
			RequestID: 0,
			Method:    "item/commandExecution/requestApproval",
			Summary:   "command",
			Detail:    "go test ./...",
			CreatedAt: time.Date(2026, 1, 1, 11, 0, 0, 0, time.UTC),
		},
	})
	m.pendingApproval = latestApprovalRequest(m.sessionApprovals["s1"])
	m.applyBlocks(mergeApprovalBlocks([]ChatBlock{{Role: ChatRoleAgent, Text: "reply"}}, m.sessionApprovals["s1"], nil))

	handled, _ := m.reduceStateMessages(approvalMsg{
		id:        "s1",
		requestID: 0,
		decision:  "accept",
	})
	if !handled {
		t.Fatalf("expected approval msg to be handled")
	}
	if m.pendingApproval != nil {
		t.Fatalf("expected pending approval to clear")
	}
	if len(m.sessionApprovals["s1"]) != 0 {
		t.Fatalf("expected pending approvals to clear, got %#v", m.sessionApprovals["s1"])
	}
	if len(m.sessionApprovalResolutions["s1"]) != 1 {
		t.Fatalf("expected one approval resolution, got %#v", m.sessionApprovalResolutions["s1"])
	}
	blocks := m.currentBlocks()
	if len(blocks) < 2 {
		t.Fatalf("expected resolved approval block, got %#v", blocks)
	}
	last := blocks[len(blocks)-1]
	if last.Role != ChatRoleApprovalResolved || last.RequestID != 0 {
		t.Fatalf("unexpected resolved approval block: %#v", last)
	}
	if !strings.Contains(strings.ToLower(last.Text), "approved") {
		t.Fatalf("expected approved marker text, got %q", last.Text)
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
