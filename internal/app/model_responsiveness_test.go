package app

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"control/internal/daemon/transcriptdomain"
	"control/internal/guidedworkflows"
	"control/internal/types"
)

func TestShouldUpdateSidebarForMessageRoutesOnlyInteractiveMessages(t *testing.T) {
	policy := defaultSidebarUpdatePolicy{}
	cases := []struct {
		name string
		msg  tea.Msg
		want bool
	}{
		{name: "key", msg: tea.KeyPressMsg{}, want: true},
		{name: "mouse", msg: tea.MouseWheelMsg{}, want: true},
		{name: "window size", msg: tea.WindowSizeMsg{}, want: true},
		{name: "tick", msg: tickMsg(time.Time{}), want: false},
		{name: "history", msg: historyMsg{}, want: false},
		{name: "paste", msg: tea.PasteMsg{}, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := policy.ShouldUpdateSidebar(tc.msg); got != tc.want {
				t.Fatalf("ShouldUpdateSidebar(%T) = %v, want %v", tc.msg, got, tc.want)
			}
		})
	}
}

func TestResolveMouseModeUsesAllMotionOnlyWhileDragging(t *testing.T) {
	if got := resolveMouseMode(false, false); got != tea.MouseModeCellMotion {
		t.Fatalf("expected cell motion when not dragging, got %v", got)
	}
	if got := resolveMouseMode(true, false); got != tea.MouseModeAllMotion {
		t.Fatalf("expected all motion when dragging, got %v", got)
	}
	if got := resolveMouseMode(false, true); got != tea.MouseModeAllMotion {
		t.Fatalf("expected all motion when split dragging, got %v", got)
	}
}

func TestReduceStateMessagesHistoryModestPayloadDefersProjection(t *testing.T) {
	m := NewModel(nil)
	m.pendingSessionKey = "sess:s1"
	m.loadingKey = "sess:s1"
	m.loading = true
	m.sessions = []*types.Session{{ID: "s1", Provider: "codex"}}

	handled, cmd := m.reduceStateMessages(historyMsg{
		id:    "s1",
		key:   "sess:s1",
		items: fakeHistoryItems(4),
	})
	if !handled {
		t.Fatalf("expected history message to be handled")
	}
	if cmd == nil {
		t.Fatalf("expected history projection to run asynchronously for modest payload")
	}
	if !m.loading {
		t.Fatalf("expected loading state to remain visible until async projection is applied")
	}

	projected, ok := cmd().(sessionBlocksProjectedMsg)
	if !ok {
		t.Fatalf("expected sessionBlocksProjectedMsg, got %T", cmd())
	}
	handled, followUp := m.reduceStateMessages(projected)
	if !handled {
		t.Fatalf("expected projected message to be handled")
	}
	if followUp != nil {
		t.Fatalf("expected no follow-up command for projected message")
	}
	if m.loading {
		t.Fatalf("expected loading state to clear once projected transcript is applied")
	}
	if len(m.currentBlocks()) == 0 {
		t.Fatalf("expected projected transcript blocks to be applied")
	}
}

func TestIsLoadingTargetPrefersKeyThenFallsBackToSessionID(t *testing.T) {
	m := NewModel(nil)
	m.loading = true
	m.loadingKey = "sess:s1"

	cases := []struct {
		name      string
		sessionID string
		key       string
		want      bool
	}{
		{name: "matching key", sessionID: "other", key: "sess:s1", want: true},
		{name: "fallback session id", sessionID: "s1", key: "", want: true},
		{name: "mismatched key still falls back to session", sessionID: "s1", key: "sess:other", want: true},
		{name: "mismatched key and session", sessionID: "other", key: "sess:other", want: false},
		{name: "mismatched session", sessionID: "other", key: "", want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := m.isLoadingTarget(tc.sessionID, tc.key); got != tc.want {
				t.Fatalf("isLoadingTarget(%q, %q) = %v, want %v", tc.sessionID, tc.key, got, tc.want)
			}
		})
	}

	m.loading = false
	if m.isLoadingTarget("s1", "sess:s1") {
		t.Fatalf("expected non-loading model to reject loading target")
	}
}

func TestMarkTranscriptLoadingSignalWithOutcomeNoopForMismatchedSession(t *testing.T) {
	sink := NewInMemoryUILatencySink()
	m := NewModel(nil, WithUILatencySink(sink))
	m.loading = true
	m.loadingKey = "sess:s1"
	m.startUILatencyAction(uiLatencyActionSwitchSession, "sess:s1")

	m.markTranscriptLoadingSignalWithOutcome("s2", uiLatencyOutcomeCacheHit)

	if !m.loading || m.loadingKey != "sess:s1" {
		t.Fatalf("expected mismatched loading signal to keep loading state")
	}
	if got := countLatencyMetricsByName(sink.Snapshot(), uiLatencyActionSwitchSession); got != 0 {
		t.Fatalf("expected no latency metrics for mismatched loading signal, got %d", got)
	}
}

func TestMarkTranscriptLoadingSignalClearsLoadingWithDefaultOutcome(t *testing.T) {
	sink := NewInMemoryUILatencySink()
	m := NewModel(nil, WithUILatencySink(sink))
	m.loading = true
	m.loadingKey = "sess:s1"
	m.startUILatencyAction(uiLatencyActionSwitchSession, "sess:s1")

	m.markTranscriptLoadingSignal("s1")

	if m.loading || m.loadingKey != "" {
		t.Fatalf("expected loading signal to clear loading state")
	}
	if !hasLatencyMetric(sink.Snapshot(), uiLatencyActionSwitchSession, UILatencyCategoryAction, uiLatencyOutcomeOK) {
		t.Fatalf("expected default ok latency outcome after loading signal")
	}
}

func TestApplySelectionStateNonSessionAbortsPendingSessionLoad(t *testing.T) {
	sink := NewInMemoryUILatencySink()
	m := NewModel(nil, WithUILatencySink(sink))
	m.pendingSessionKey = "sess:s1"
	m.pendingTranscriptSnapshotRetryCount = map[string]int{"sess:s1": 1}
	m.loading = true
	m.loadingKey = "sess:s1"
	m.startUILatencyAction(uiLatencyActionSwitchSession, "sess:s1")
	m.replaceRequestScope(requestScopeSessionLoad)
	initialGeneration := m.renderGeneration

	handled, stateChanged, _ := m.applySelectionState(&sidebarItem{
		kind:      sidebarWorkspace,
		workspace: &types.Workspace{ID: "ws-1", Name: "Workspace"},
	})
	if !handled {
		t.Fatalf("expected workspace selection to be handled")
	}
	if !stateChanged {
		t.Fatalf("expected workspace selection to update active app state")
	}
	if m.hasRequestScope(requestScopeSessionLoad) {
		t.Fatalf("expected workspace selection to cancel session load scope")
	}
	if m.pendingSessionKey != "" {
		t.Fatalf("expected workspace selection to clear pending session key, got %q", m.pendingSessionKey)
	}
	if m.loading || m.loadingKey != "" {
		t.Fatalf("expected workspace selection to clear loading indicator")
	}
	if got := m.pendingTranscriptSnapshotRetryCount["sess:s1"]; got != 0 {
		t.Fatalf("expected pending snapshot retry state for previous session to be cleared, got %d", got)
	}
	if m.renderGeneration != initialGeneration+1 {
		t.Fatalf("expected workspace selection to invalidate viewport render generation")
	}
	if !hasLatencyMetric(sink.Snapshot(), uiLatencyActionSwitchSession, UILatencyCategoryAction, uiLatencyOutcomeCanceled) {
		t.Fatalf("expected navigation-away selection to emit canceled switch-session latency outcome")
	}
}

func TestApplySelectionStateNonSessionKindsAbortPendingSessionLoad(t *testing.T) {
	cases := []struct {
		name              string
		item              *sidebarItem
		expectStateChange bool
	}{
		{
			name: "worktree",
			item: &sidebarItem{
				kind:     sidebarWorktree,
				worktree: &types.Worktree{ID: "wt-1", WorkspaceID: "ws-1", Name: "Worktree"},
			},
			expectStateChange: true,
		},
		{
			name: "workflow",
			item: &sidebarItem{
				kind: sidebarWorkflow,
				workflow: &guidedworkflows.WorkflowRun{
					ID:          "gwf-1",
					WorkspaceID: "ws-2",
					WorktreeID:  "wt-2",
					Status:      guidedworkflows.WorkflowRunStatusRunning,
				},
			},
			expectStateChange: true,
		},
		{
			name:              "default non-session",
			item:              &sidebarItem{kind: sidebarItemKind(999)},
			expectStateChange: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			sink := NewInMemoryUILatencySink()
			m := NewModel(nil, WithUILatencySink(sink))
			m.pendingSessionKey = "sess:s1"
			m.pendingTranscriptSnapshotRetryCount = map[string]int{"sess:s1": 1}
			m.loading = true
			m.loadingKey = "sess:s1"
			m.startUILatencyAction(uiLatencyActionSwitchSession, "sess:s1")
			m.replaceRequestScope(requestScopeSessionLoad)
			initialGeneration := m.renderGeneration

			handled, stateChanged, _ := m.applySelectionState(tc.item)
			if !handled {
				t.Fatalf("expected selection %q to be handled", tc.name)
			}
			if stateChanged != tc.expectStateChange {
				t.Fatalf("stateChanged=%v, want %v", stateChanged, tc.expectStateChange)
			}
			if m.hasRequestScope(requestScopeSessionLoad) {
				t.Fatalf("expected selection %q to cancel session-load scope", tc.name)
			}
			if m.pendingSessionKey != "" {
				t.Fatalf("expected selection %q to clear pending session key, got %q", tc.name, m.pendingSessionKey)
			}
			if m.loading || m.loadingKey != "" {
				t.Fatalf("expected selection %q to clear loading state", tc.name)
			}
			if got := m.pendingTranscriptSnapshotRetryCount["sess:s1"]; got != 0 {
				t.Fatalf("expected selection %q to clear pending snapshot retry state, got %d", tc.name, got)
			}
			if m.renderGeneration <= initialGeneration {
				t.Fatalf("expected selection %q to advance viewport render generation", tc.name)
			}
			if !hasLatencyMetric(sink.Snapshot(), uiLatencyActionSwitchSession, UILatencyCategoryAction, uiLatencyOutcomeCanceled) {
				t.Fatalf("expected selection %q to emit canceled switch-session latency metric", tc.name)
			}
		})
	}
}

func TestFinishSessionLoadLatencyForKeyDefaultsOutcome(t *testing.T) {
	sink := NewInMemoryUILatencySink()
	m := NewModel(nil, WithUILatencySink(sink))
	m.startUILatencyAction(uiLatencyActionSwitchSession, "sess:s1")

	m.finishSessionLoadLatencyForKey("sess:s1", "")

	if !hasLatencyMetric(sink.Snapshot(), uiLatencyActionSwitchSession, UILatencyCategoryAction, uiLatencyOutcomeOK) {
		t.Fatalf("expected blank latency outcome to default to ok")
	}
}

func TestSettleSessionLoadProjectionClearsLoadingWhenNotVisible(t *testing.T) {
	sink := NewInMemoryUILatencySink()
	m := NewModel(nil, WithUILatencySink(sink))
	m.loading = true
	m.loadingKey = "sess:s1"
	m.startUILatencyAction(uiLatencyActionSwitchSession, "sess:s1")

	m.settleSessionLoadProjection("s1", "sess:s1", viewportRenderOutcome{}, false, "")

	if m.loading || m.loadingKey != "" {
		t.Fatalf("expected non-visible settlement to clear loading state")
	}
	if !hasLatencyMetric(sink.Snapshot(), uiLatencyActionSwitchSession, UILatencyCategoryAction, uiLatencyOutcomeOK) {
		t.Fatalf("expected non-visible settlement to finish switch-session latency")
	}
}

func TestReduceStateMessagesIgnoresStaleSessionProjection(t *testing.T) {
	m := NewModel(nil)
	m.pendingSessionKey = "sess:s1"
	m.sessions = []*types.Session{{ID: "s1", Provider: "codex"}}

	handled, cmd1 := m.reduceStateMessages(historyMsg{
		id:    "s1",
		key:   "sess:s1",
		items: fakeHistoryItems(2),
	})
	if !handled || cmd1 == nil {
		t.Fatalf("expected first async projection command")
	}

	handled, cmd2 := m.reduceStateMessages(historyMsg{
		id:    "s1",
		key:   "sess:s1",
		items: fakeHistoryItems(3),
	})
	if !handled || cmd2 == nil {
		t.Fatalf("expected second async projection command")
	}

	proj2 := cmd2().(sessionBlocksProjectedMsg)
	_, _ = m.reduceStateMessages(proj2)
	latestText := tailBlockText(m.currentBlocks())
	if latestText == "" {
		t.Fatalf("expected latest projection to apply blocks")
	}

	proj1 := cmd1().(sessionBlocksProjectedMsg)
	_, _ = m.reduceStateMessages(proj1)
	if got := tailBlockText(m.currentBlocks()); got != latestText {
		t.Fatalf("expected stale projection to be ignored, got %q want %q", got, latestText)
	}
}

func TestAsyncSessionProjectionCancelsSupersededWork(t *testing.T) {
	projector := &blockingSessionBlockProjector{}
	m := NewModel(nil,
		WithSessionProjectionPolicy(testSessionProjectionPolicy{asyncAt: 1, maxTokens: 4}),
		WithSessionBlockProjector(projector),
	)
	m.pendingSessionKey = "sess:s1"
	m.loadingKey = "sess:s1"
	m.loading = true
	m.sessions = []*types.Session{{ID: "s1", Provider: "codex"}}
	m.replaceRequestScope(requestScopeSessionLoad)

	handled, cmd1 := m.reduceStateMessages(historyMsg{
		id:    "s1",
		key:   "sess:s1",
		items: fakeHistoryItems(1),
	})
	if !handled || cmd1 == nil {
		t.Fatalf("expected first async projection command")
	}

	msg1Ch := make(chan tea.Msg, 1)
	go func() {
		msg1Ch <- cmd1()
	}()

	call1 := waitForProjectionCall(t, projector, 0)
	select {
	case <-call1.started:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for first projection to start")
	}

	handled, cmd2 := m.reduceStateMessages(historyMsg{
		id:    "s1",
		key:   "sess:s1",
		items: fakeHistoryItems(2),
	})
	if !handled || cmd2 == nil {
		t.Fatalf("expected second async projection command")
	}

	select {
	case <-call1.canceled:
	case <-time.After(2 * time.Second):
		t.Fatalf("expected superseded projection to be canceled")
	}

	msg1 := (<-msg1Ch).(sessionBlocksProjectedMsg)
	if !isCanceledRequestError(msg1.err) {
		t.Fatalf("expected first projection to return canceled error, got %v", msg1.err)
	}

	msg2Ch := make(chan tea.Msg, 1)
	go func() {
		msg2Ch <- cmd2()
	}()

	call2 := waitForProjectionCall(t, projector, 1)
	select {
	case <-call2.started:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for second projection to start")
	}
	close(call2.continueCh)

	msg2 := (<-msg2Ch).(sessionBlocksProjectedMsg)
	if msg2.err != nil {
		t.Fatalf("expected second projection to complete successfully, got %v", msg2.err)
	}

	if handled, followUp := m.reduceStateMessages(msg2); !handled || followUp != nil {
		t.Fatalf("expected second projection result to apply without follow-up")
	}
	latestText := tailBlockText(m.currentBlocks())
	if latestText == "" || !strings.Contains(latestText, "reply-001") {
		t.Fatalf("expected latest projection output to apply, got %q", latestText)
	}

	if handled, followUp := m.reduceStateMessages(msg1); !handled || followUp != nil {
		t.Fatalf("expected canceled stale projection message to be handled without follow-up")
	}
	if got := tailBlockText(m.currentBlocks()); got != latestText {
		t.Fatalf("expected canceled superseded projection to leave latest content intact, got %q want %q", got, latestText)
	}
}

func TestCanceledCurrentSessionProjectionClearsPendingToken(t *testing.T) {
	projector := &blockingSessionBlockProjector{}
	m := NewModel(nil,
		WithSessionProjectionPolicy(testSessionProjectionPolicy{asyncAt: 1, maxTokens: 4}),
		WithSessionBlockProjector(projector),
	)
	m.pendingSessionKey = "sess:s1"
	m.sessions = []*types.Session{{ID: "s1", Provider: "codex"}}
	m.replaceRequestScope(requestScopeSessionLoad)

	handled, cmd := m.reduceStateMessages(historyMsg{
		id:    "s1",
		key:   "sess:s1",
		items: fakeHistoryItems(1),
	})
	if !handled || cmd == nil {
		t.Fatalf("expected async projection command")
	}
	token := sessionProjectionToken("sess:s1", "s1")
	if !m.sessionProjectionCoordinatorOrDefault().HasPending(token) {
		t.Fatalf("expected pending projection before cancellation")
	}

	msgCh := make(chan tea.Msg, 1)
	go func() {
		msgCh <- cmd()
	}()

	call := waitForProjectionCall(t, projector, 0)
	select {
	case <-call.started:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for projection to start")
	}

	m.cancelRequestScope(requestScopeSessionLoad)

	select {
	case <-call.canceled:
	case <-time.After(2 * time.Second):
		t.Fatalf("expected active projection to cancel with request scope")
	}

	msg := (<-msgCh).(sessionBlocksProjectedMsg)
	if !isCanceledRequestError(msg.err) {
		t.Fatalf("expected canceled projection message, got %v", msg.err)
	}

	if handled, followUp := m.reduceStateMessages(msg); !handled || followUp != nil {
		t.Fatalf("expected canceled projection message to be handled without follow-up")
	}
	if m.sessionProjectionCoordinatorOrDefault().HasPending(token) {
		t.Fatalf("expected canceled current projection to release pending token")
	}
}

func TestApplySelectionStateNonSessionCancelsInFlightSessionProjection(t *testing.T) {
	projector := &blockingSessionBlockProjector{}
	m := NewModel(nil,
		WithSessionProjectionPolicy(testSessionProjectionPolicy{asyncAt: 1, maxTokens: 4}),
		WithSessionBlockProjector(projector),
	)
	m.pendingSessionKey = "sess:s1"
	m.loading = true
	m.loadingKey = "sess:s1"
	m.sessions = []*types.Session{{ID: "s1", Provider: "codex"}}
	m.replaceRequestScope(requestScopeSessionLoad)

	handled, cmd := m.reduceStateMessages(historyMsg{
		id:    "s1",
		key:   "sess:s1",
		items: fakeHistoryItems(1),
	})
	if !handled || cmd == nil {
		t.Fatalf("expected async projection command")
	}

	msgCh := make(chan tea.Msg, 1)
	go func() {
		msgCh <- cmd()
	}()

	call := waitForProjectionCall(t, projector, 0)
	select {
	case <-call.started:
	case <-time.After(2 * time.Second):
		t.Fatalf("timed out waiting for projection to start")
	}

	handled, _, _ = m.applySelectionState(&sidebarItem{kind: sidebarRecentsAll})
	if !handled {
		t.Fatalf("expected recents selection to be handled")
	}

	select {
	case <-call.canceled:
	case <-time.After(2 * time.Second):
		t.Fatalf("expected navigation-away selection to cancel active projection")
	}

	msg := (<-msgCh).(sessionBlocksProjectedMsg)
	if !isCanceledRequestError(msg.err) {
		t.Fatalf("expected canceled projection message after navigation-away selection, got %v", msg.err)
	}
	if m.hasRequestScope(requestScopeSessionLoad) {
		t.Fatalf("expected recents selection to remove session load request scope")
	}
	if m.pendingSessionKey != "" {
		t.Fatalf("expected recents selection to clear pending session key, got %q", m.pendingSessionKey)
	}
	if m.loading || m.loadingKey != "" {
		t.Fatalf("expected recents selection to clear loading state")
	}
}

type testSessionProjectionPolicy struct {
	asyncAt   int
	maxTokens int
}

func (p testSessionProjectionPolicy) ShouldProjectAsync(input SessionProjectionDecisionInput) bool {
	return input.ItemCount >= p.asyncAt
}

func (p testSessionProjectionPolicy) MaxTrackedProjectionTokens() int {
	return p.maxTokens
}

type recordingSessionProjectionPolicy struct {
	maxTokens   int
	shouldAsync bool
	inputs      []SessionProjectionDecisionInput
}

func (p *recordingSessionProjectionPolicy) ShouldProjectAsync(input SessionProjectionDecisionInput) bool {
	p.inputs = append(p.inputs, input)
	return p.shouldAsync
}

func (p *recordingSessionProjectionPolicy) MaxTrackedProjectionTokens() int {
	if p.maxTokens <= 0 {
		return defaultSessionProjectionMaxTokens
	}
	return p.maxTokens
}

type testSidebarUpdatePolicy struct {
	allow bool
}

func (p testSidebarUpdatePolicy) ShouldUpdateSidebar(tea.Msg) bool {
	return p.allow
}

type testSessionProjectionPostProcessor struct {
	calls int
	last  SessionProjectionPostProcessInput
}

func (p *testSessionProjectionPostProcessor) PostProcessSessionProjection(_ *Model, input SessionProjectionPostProcessInput) {
	p.calls++
	p.last = input
}

type testDebugPanelProjectionCoordinator struct{}

func (testDebugPanelProjectionCoordinator) Schedule(DebugPanelProjectionRequest) tea.Cmd { return nil }
func (testDebugPanelProjectionCoordinator) IsCurrent(int) bool                           { return true }
func (testDebugPanelProjectionCoordinator) Consume(int)                                  {}
func (testDebugPanelProjectionCoordinator) Invalidate()                                  {}

type blockingSessionProjectionCall struct {
	started    chan struct{}
	continueCh chan struct{}
	canceled   chan struct{}
}

type blockingSessionBlockProjector struct {
	mu    sync.Mutex
	calls []*blockingSessionProjectionCall
}

func (p *blockingSessionBlockProjector) ProjectSessionBlocks(ctx context.Context, input SessionBlockProjectionInput) ([]ChatBlock, error) {
	call := &blockingSessionProjectionCall{
		started:    make(chan struct{}),
		continueCh: make(chan struct{}),
		canceled:   make(chan struct{}),
	}
	p.mu.Lock()
	p.calls = append(p.calls, call)
	p.mu.Unlock()
	close(call.started)
	select {
	case <-call.continueCh:
		blocks, err := projectSessionBlocksFromItemsWithContext(
			ctx,
			input.Provider,
			input.Rules,
			input.Items,
			input.Previous,
			input.Approvals,
			input.Resolutions,
		)
		return blocks, err
	case <-ctx.Done():
		close(call.canceled)
		return nil, ctx.Err()
	}
}

func (p *blockingSessionBlockProjector) call(index int) *blockingSessionProjectionCall {
	p.mu.Lock()
	defer p.mu.Unlock()
	if index < 0 || index >= len(p.calls) {
		return nil
	}
	return p.calls[index]
}

func waitForProjectionCall(t *testing.T, projector *blockingSessionBlockProjector, index int) *blockingSessionProjectionCall {
	t.Helper()
	deadline := time.After(2 * time.Second)
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for {
		if call := projector.call(index); call != nil {
			return call
		}
		select {
		case <-deadline:
			t.Fatalf("timed out waiting for projection call %d", index)
		case <-tick.C:
		}
	}
}

func TestWithSidebarUpdatePolicyConfiguresModelAndDefaultReset(t *testing.T) {
	WithSidebarUpdatePolicy(testSidebarUpdatePolicy{allow: true})(nil)

	m := NewModel(nil, WithSidebarUpdatePolicy(testSidebarUpdatePolicy{allow: false}))
	if got := m.sidebarUpdatePolicyOrDefault().ShouldUpdateSidebar(tea.KeyPressMsg{}); got {
		t.Fatalf("expected custom sidebar policy to be used")
	}

	WithSidebarUpdatePolicy(nil)(&m)
	if got := m.sidebarUpdatePolicyOrDefault().ShouldUpdateSidebar(tea.KeyPressMsg{}); !got {
		t.Fatalf("expected default sidebar policy after nil reset")
	}
}

func TestWithSessionProjectionPolicyConfiguresModelAndDefaultReset(t *testing.T) {
	WithSessionProjectionPolicy(testSessionProjectionPolicy{asyncAt: 1, maxTokens: 1})(nil)

	m := NewModel(nil, WithSessionProjectionPolicy(testSessionProjectionPolicy{
		asyncAt:   10,
		maxTokens: 3,
	}))
	policy := m.sessionProjectionPolicyOrDefault()
	if policy.ShouldProjectAsync(SessionProjectionDecisionInput{ItemCount: 5}) {
		t.Fatalf("expected custom projection policy to gate async projection")
	}
	if got := policy.MaxTrackedProjectionTokens(); got != 3 {
		t.Fatalf("expected custom max tokens 3, got %d", got)
	}
	if coordinator := m.sessionProjectionCoordinatorOrDefault(); coordinator == nil {
		t.Fatalf("expected session projection coordinator to be initialized")
	}

	WithSessionProjectionPolicy(nil)(&m)
	defaultPolicy := m.sessionProjectionPolicyOrDefault()
	if defaultPolicy.ShouldProjectAsync(SessionProjectionDecisionInput{}) {
		t.Fatalf("expected default projection policy to keep empty payloads synchronous")
	}
	if defaultPolicy.ShouldProjectAsync(SessionProjectionDecisionInput{ItemCount: 3}) {
		t.Fatalf("expected default projection policy to keep non-fetched payloads synchronous")
	}
	if !defaultPolicy.ShouldProjectAsync(SessionProjectionDecisionInput{ItemCount: 1, IsFetchedPayload: true}) {
		t.Fatalf("expected default projection policy to defer any non-empty payload")
	}
	if got := defaultPolicy.MaxTrackedProjectionTokens(); got != defaultSessionProjectionMaxTokens {
		t.Fatalf("expected default max tokens %d, got %d", defaultSessionProjectionMaxTokens, got)
	}
}

func TestAsyncSessionProjectionCmdPassesDecisionInputForFetchedPayloads(t *testing.T) {
	policy := &recordingSessionProjectionPolicy{maxTokens: 4, shouldAsync: true}
	m := NewModel(nil, WithSessionProjectionPolicy(policy))
	m.pendingSessionKey = "sess:s1"
	m.sessions = []*types.Session{{ID: "s1", Provider: "codex"}}
	m.setApprovalsForSession("s1", []*ApprovalRequest{{
		RequestID: 1,
		SessionID: "s1",
		Method:    "tool/requestUserInput",
	}})

	cases := []struct {
		name       string
		msg        tea.Msg
		wantSource sessionProjectionSource
	}{
		{
			name: "history",
			msg: historyMsg{
				id:    "s1",
				key:   "sess:s1",
				items: fakeHistoryItems(2),
			},
			wantSource: sessionProjectionSourceHistory,
		},
		{
			name: "tail",
			msg: tailMsg{
				id:    "s1",
				key:   "sess:s1",
				items: fakeHistoryItems(2),
			},
			wantSource: sessionProjectionSourceTail,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			policy.inputs = nil
			handled, cmd := m.reduceStateMessages(tc.msg)
			if !handled {
				t.Fatalf("expected %s message to be handled", tc.name)
			}
			if cmd == nil {
				t.Fatalf("expected %s projection to go async", tc.name)
			}
			if got := len(policy.inputs); got != 1 {
				t.Fatalf("expected one decision input, got %d", got)
			}
			input := policy.inputs[0]
			if input.ItemCount != 2 {
				t.Fatalf("expected item count 2, got %d", input.ItemCount)
			}
			if input.Source != tc.wantSource {
				t.Fatalf("expected source %q, got %q", tc.wantSource, input.Source)
			}
			if input.Provider != "codex" {
				t.Fatalf("expected provider codex, got %q", input.Provider)
			}
			if !input.HasApprovals {
				t.Fatalf("expected approvals to be reflected in decision input")
			}
			if !input.IsFetchedPayload {
				t.Fatalf("expected fetched payload flag to be true")
			}
		})
	}
}

func TestProjectAndApplySessionItemsFallsBackToSyncWhenPolicyRejectsAsync(t *testing.T) {
	policy := &recordingSessionProjectionPolicy{maxTokens: 4, shouldAsync: false}
	m := NewModel(nil, WithSessionProjectionPolicy(policy))
	m.pendingSessionKey = "sess:s1"
	m.loadingKey = "sess:s1"
	m.loading = true
	m.sessions = []*types.Session{{ID: "s1", Provider: "codex"}}

	handled, cmd := m.reduceStateMessages(historyMsg{
		id:    "s1",
		key:   "sess:s1",
		items: fakeHistoryItems(2),
	})
	if !handled {
		t.Fatalf("expected history message to be handled")
	}
	if cmd != nil {
		t.Fatalf("expected forced-sync policy to keep projection inline")
	}
	if got := len(policy.inputs); got != 1 {
		t.Fatalf("expected one decision input, got %d", got)
	}
	if !policy.inputs[0].IsFetchedPayload || policy.inputs[0].Source != sessionProjectionSourceHistory {
		t.Fatalf("unexpected decision input %#v", policy.inputs[0])
	}
	if len(m.currentBlocks()) == 0 {
		t.Fatalf("expected sync fallback to apply projected blocks immediately")
	}
	if m.loading {
		t.Fatalf("expected sync fallback to settle loading")
	}
}

func TestWithSessionProjectionCoordinatorConfiguresModelAndDefaultReset(t *testing.T) {
	WithSessionProjectionCoordinator(NewDefaultSessionProjectionCoordinator(testSessionProjectionPolicy{asyncAt: 1, maxTokens: 1}, nil))(nil)

	custom := NewDefaultSessionProjectionCoordinator(testSessionProjectionPolicy{asyncAt: 1, maxTokens: 2}, nil)
	m := NewModel(nil, WithSessionProjectionCoordinator(custom))
	if m.sessionProjectionCoordinator != custom {
		t.Fatalf("expected custom session projection coordinator to be installed")
	}

	WithSessionProjectionCoordinator(nil)(&m)
	if m.sessionProjectionCoordinator == nil {
		t.Fatalf("expected default session projection coordinator after nil reset")
	}
}

func TestSessionProjectionCoordinatorOrDefaultHandlesNilModelAndCachesDefault(t *testing.T) {
	var nilModel *Model
	if coordinator := nilModel.sessionProjectionCoordinatorOrDefault(); coordinator == nil {
		t.Fatalf("expected nil model to return default coordinator")
	}

	m := NewModel(nil)
	m.sessionProjectionCoordinator = nil
	coordinator := m.sessionProjectionCoordinatorOrDefault()
	if coordinator == nil {
		t.Fatalf("expected fallback coordinator")
	}
	if m.sessionProjectionCoordinator == nil {
		t.Fatalf("expected fallback coordinator to be cached on model")
	}
}

func TestWithSessionBlockProjectorConfiguresModelAndDefaultReset(t *testing.T) {
	WithSessionBlockProjector(&blockingSessionBlockProjector{})(nil)

	custom := &blockingSessionBlockProjector{}
	m := NewModel(nil, WithSessionBlockProjector(custom))
	if m.sessionBlockProjector != custom {
		t.Fatalf("expected custom session block projector to be installed")
	}

	WithSessionBlockProjector(nil)(&m)
	if _, ok := m.sessionBlockProjector.(defaultSessionBlockProjector); !ok {
		t.Fatalf("expected default session block projector after nil reset, got %T", m.sessionBlockProjector)
	}
}

func TestSessionBlockProjectorOrDefaultHandlesNilModelAndFallback(t *testing.T) {
	var nilModel *Model
	if projector := nilModel.sessionBlockProjectorOrDefault(); projector == nil {
		t.Fatalf("expected nil model to return default session block projector")
	}

	m := NewModel(nil)
	m.sessionBlockProjector = nil
	if _, ok := m.sessionBlockProjectorOrDefault().(defaultSessionBlockProjector); !ok {
		t.Fatalf("expected default projector when field is nil")
	}
}

func TestWithSessionProjectionPostProcessorConfiguresModelAndDefaultReset(t *testing.T) {
	WithSessionProjectionPostProcessor(&testSessionProjectionPostProcessor{})(nil)

	processor := &testSessionProjectionPostProcessor{}
	m := NewModel(nil, WithSessionProjectionPostProcessor(processor))
	blocks := []ChatBlock{{Role: ChatRoleUser, Text: "turn", TurnID: "turn-1"}}
	m.applySessionProjection(sessionProjectionSourceTail, "s1", "", blocks)
	if processor.calls != 1 {
		t.Fatalf("expected custom post processor to be called once, got %d", processor.calls)
	}
	if processor.last.Source != sessionProjectionSourceTail || processor.last.SessionID != "s1" || len(processor.last.Blocks) != 1 {
		t.Fatalf("unexpected post processor input: %#v", processor.last)
	}

	WithSessionProjectionPostProcessor(nil)(&m)
	m.setPendingWorkflowTurnFocus("s1", "turn-1")
	m.applySessionProjection(sessionProjectionSourceHistory, "s1", "", blocks)
	if m.pendingWorkflowTurnFocus != nil {
		t.Fatalf("expected default post processor to clear pending workflow turn focus")
	}
}

func TestPolicyDefaultsHandleNilModel(t *testing.T) {
	var m *Model
	if got := m.sidebarUpdatePolicyOrDefault().ShouldUpdateSidebar(tea.WindowSizeMsg{}); !got {
		t.Fatalf("expected default sidebar policy for nil model")
	}
	if got := m.sessionProjectionPolicyOrDefault().MaxTrackedProjectionTokens(); got != defaultSessionProjectionMaxTokens {
		t.Fatalf("expected default projection token cap for nil model, got %d", got)
	}
	if processor := m.sessionProjectionPostProcessorOrDefault(); processor == nil {
		t.Fatalf("expected default projection post processor for nil model")
	}
	if got := m.debugPanelProjectionPolicyOrDefault().MaxTrackedProjectionTokens(); got != defaultDebugPanelProjectionMaxTokens {
		t.Fatalf("expected default debug projection token cap for nil model, got %d", got)
	}
	if coordinator := m.debugPanelProjectionCoordinatorOrDefault(); coordinator == nil {
		t.Fatalf("expected default debug projection coordinator for nil model")
	}
}

func TestWithDebugPanelProjectionPolicyConfiguresModelAndDefaultReset(t *testing.T) {
	WithDebugPanelProjectionPolicy(testDebugProjectionPolicy{max: 2})(nil)

	m := NewModel(nil, WithDebugPanelProjectionPolicy(testDebugProjectionPolicy{max: 5}))
	if got := m.debugPanelProjectionPolicyOrDefault().MaxTrackedProjectionTokens(); got != 5 {
		t.Fatalf("expected custom debug projection max tokens 5, got %d", got)
	}
	if m.debugPanelProjectionCoordinator == nil {
		t.Fatalf("expected debug projection coordinator to be initialized")
	}

	WithDebugPanelProjectionPolicy(nil)(&m)
	if got := m.debugPanelProjectionPolicyOrDefault().MaxTrackedProjectionTokens(); got != defaultDebugPanelProjectionMaxTokens {
		t.Fatalf("expected default debug projection max tokens %d, got %d", defaultDebugPanelProjectionMaxTokens, got)
	}
	if m.debugPanelProjectionCoordinator == nil {
		t.Fatalf("expected default debug projection coordinator after reset")
	}
}

func TestWithDebugPanelProjectionCoordinatorConfiguresModelAndDefaultReset(t *testing.T) {
	WithDebugPanelProjectionCoordinator(testDebugPanelProjectionCoordinator{})(nil)

	custom := testDebugPanelProjectionCoordinator{}
	m := NewModel(nil, WithDebugPanelProjectionCoordinator(custom))
	if _, ok := m.debugPanelProjectionCoordinator.(testDebugPanelProjectionCoordinator); !ok {
		t.Fatalf("expected custom debug projection coordinator")
	}

	WithDebugPanelProjectionCoordinator(nil)(&m)
	if _, ok := m.debugPanelProjectionCoordinator.(*defaultDebugPanelProjectionCoordinator); !ok {
		t.Fatalf("expected default debug projection coordinator after nil reset, got %T", m.debugPanelProjectionCoordinator)
	}
}

func TestDebugPanelProjectionCoordinatorOrDefaultInitializesWhenMissing(t *testing.T) {
	m := NewModel(nil)
	m.debugPanelProjectionCoordinator = nil
	coordinator := m.debugPanelProjectionCoordinatorOrDefault()
	if coordinator == nil {
		t.Fatalf("expected coordinator fallback")
	}
	if m.debugPanelProjectionCoordinator == nil {
		t.Fatalf("expected coordinator fallback to be cached on model")
	}
}

func TestReduceStateMessagesPrunesSessionProjectionTokens(t *testing.T) {
	m := NewModel(nil, WithSessionProjectionPolicy(testSessionProjectionPolicy{
		asyncAt:   1,
		maxTokens: 2,
	}))
	m.sessions = []*types.Session{{ID: "s1", Provider: "codex"}}

	keys := []string{"sess:s1:a", "sess:s1:b", "sess:s1:c"}
	for _, key := range keys {
		m.pendingSessionKey = key
		handled, cmd := m.reduceStateMessages(historyMsg{
			id:    "s1",
			key:   key,
			items: fakeHistoryItems(1),
		})
		if !handled || cmd == nil {
			t.Fatalf("expected async projection command for key %q", key)
		}
	}

	latest := m.sessionProjectionCoordinatorOrDefault().LatestByToken()
	if got := len(latest); got != 2 {
		t.Fatalf("expected projection token tracker to be capped at 2, got %d", got)
	}
	if _, ok := latest["key:sess:s1:a"]; ok {
		t.Fatalf("expected oldest projection token to be pruned")
	}
}

func TestReduceStateMessagesHistoryErrorClearsLoadingAndFinishesLatency(t *testing.T) {
	sink := NewInMemoryUILatencySink()
	m := NewModel(nil, WithUILatencySink(sink))
	m.pendingSessionKey = "sess:s1"
	m.loadingKey = "sess:s1"
	m.loading = true
	m.startUILatencyAction(uiLatencyActionSwitchSession, "sess:s1")

	handled, cmd := m.reduceStateMessages(historyMsg{
		id:  "s1",
		key: "sess:s1",
		err: errors.New("boom"),
	})
	if !handled {
		t.Fatalf("expected history error to be handled")
	}
	if cmd != nil {
		t.Fatalf("expected no follow-up command on history error")
	}
	if m.loading {
		t.Fatalf("expected loading state to clear on history error")
	}
	if got := m.contentRaw; got != "Error loading history." {
		t.Fatalf("expected error content, got %q", got)
	}
	if got := m.status; !strings.Contains(got, "history error: boom") {
		t.Fatalf("expected background error status, got %q", got)
	}
	if !hasLatencyMetric(sink.Snapshot(), uiLatencyActionSwitchSession, UILatencyCategoryAction, uiLatencyOutcomeError) {
		t.Fatalf("expected switch-session latency action to finish with error outcome")
	}
}

func TestSessionProjectionTokenPrefersKeyThenID(t *testing.T) {
	cases := []struct {
		name string
		key  string
		id   string
		want string
	}{
		{name: "key", key: " sess:1 ", id: "ignored", want: "key:sess:1"},
		{name: "id", key: "   ", id: " s1 ", want: "id:s1"},
		{name: "empty", key: "", id: "", want: ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := sessionProjectionToken(tc.key, tc.id); got != tc.want {
				t.Fatalf("sessionProjectionToken(%q, %q) = %q, want %q", tc.key, tc.id, got, tc.want)
			}
		})
	}
}

func TestBuildSessionBlocksFromItemsProjectsHydratedBlocks(t *testing.T) {
	m := NewModel(nil)
	blocks := m.buildSessionBlocksFromItems("s1", "codex", fakeHistoryItems(1), nil)
	if len(blocks) == 0 {
		t.Fatalf("expected projected blocks")
	}
	if got := tailBlockText(blocks); !strings.Contains(got, "reply-000") {
		t.Fatalf("expected projected text to include history content, got %q", got)
	}
}

func TestProjectSessionBlocksFromItemsRespectsCoalesceReasoningFlag(t *testing.T) {
	items := []map[string]any{
		{"type": "reasoning", "id": "r1", "summary": []any{"first"}},
		{"type": "reasoning", "id": "r2", "summary": []any{"second"}},
	}

	notCoalesced := projectSessionBlocksFromItems(
		"codex",
		sessionBlockProjectionRules{CoalesceReasoning: false, SupportsApprovals: false},
		items,
		nil,
		nil,
		nil,
	)
	if len(notCoalesced) != 2 {
		t.Fatalf("expected two separate reasoning blocks without coalescing, got %#v", notCoalesced)
	}

	coalesced := projectSessionBlocksFromItems(
		"codex",
		sessionBlockProjectionRules{CoalesceReasoning: true, SupportsApprovals: false},
		items,
		nil,
		nil,
		nil,
	)
	if len(coalesced) != 1 {
		t.Fatalf("expected one merged reasoning block with coalescing, got %#v", coalesced)
	}
	if coalesced[0].Role != ChatRoleReasoning {
		t.Fatalf("expected merged reasoning role, got %s", coalesced[0].Role)
	}
	if !strings.Contains(coalesced[0].Text, "first") || !strings.Contains(coalesced[0].Text, "second") {
		t.Fatalf("expected merged reasoning text to include both summaries, got %q", coalesced[0].Text)
	}
}

func TestSessionBlockProjectionRulesUseTranscriptCapabilitiesOverride(t *testing.T) {
	m := NewModel(nil)

	rules := m.sessionBlockProjectionRules("s1", "codex")
	if !rules.SupportsApprovals {
		t.Fatalf("expected codex provider defaults to support approvals")
	}
	if !rules.CoalesceReasoning {
		t.Fatalf("expected reasoning coalescing to stay enabled by default")
	}

	m.setSessionTranscriptCapabilities("s1", transcriptdomain.CapabilityEnvelope{SupportsApprovals: false})
	rules = m.sessionBlockProjectionRules("s1", "codex")
	if rules.SupportsApprovals {
		t.Fatalf("expected transcript capabilities override to disable approvals support")
	}

	m.setSessionTranscriptCapabilities("s2", transcriptdomain.CapabilityEnvelope{SupportsApprovals: true})
	rules = m.sessionBlockProjectionRules("s2", "custom")
	if !rules.SupportsApprovals {
		t.Fatalf("expected transcript capabilities override to enable approvals support")
	}
}

func TestApplySessionProjectionCachesSelectedKeyWhenProjectionHasNoKey(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	if m.sidebar == nil || !m.sidebar.SelectBySessionID("s1") {
		t.Fatalf("expected session selection to be available")
	}
	selectedKey := m.selectedKey()
	if selectedKey == "" {
		t.Fatalf("expected selected key")
	}

	blocks := []ChatBlock{{Role: ChatRoleAgent, Text: "projected"}}
	m.applySessionProjection(sessionProjectionSourceTail, "s1", "", blocks)

	cached := m.transcriptCache[selectedKey]
	if len(cached) != 1 || cached[0].Text != "projected" {
		t.Fatalf("expected selected-key cache update, got %#v", cached)
	}
	if got := m.status; got != "tail updated" {
		t.Fatalf("expected background status update, got %q", got)
	}
}

func TestApplySessionProjectionSkipsCacheWhenNoKeyAndNonSelectedSession(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	if m.sidebar == nil || !m.sidebar.SelectBySessionID("s1") {
		t.Fatalf("expected session selection to be available")
	}
	m.transcriptCache = map[string][]ChatBlock{}
	m.applyBlocks([]ChatBlock{{Role: ChatRoleAgent, Text: "visible-session"}})

	m.applySessionProjection(sessionProjectionSourceHistory, "other-session", "", []ChatBlock{
		{Role: ChatRoleAgent, Text: "ignored cache"},
	})

	visible := m.currentBlocks()
	if len(visible) != 1 || visible[0].Text != "visible-session" {
		t.Fatalf("expected visible transcript to remain unchanged for non-active session projection, got %#v", visible)
	}
	if got := len(m.transcriptCache); got != 0 {
		t.Fatalf("expected no cache writes for non-selected session without key, got %d", got)
	}
	if got := m.status; got != "history updated" {
		t.Fatalf("expected background status update, got %q", got)
	}
}

func TestShouldApplySessionProjectionToVisibleUsesActiveContext(t *testing.T) {
	m := newPhase0ModelWithSession("codex")
	if m.sidebar == nil || !m.sidebar.SelectBySessionID("s1") {
		t.Fatalf("expected session selection to be available")
	}
	selectedKey := m.selectedKey()
	if !m.shouldApplySessionProjectionToVisible("s1", selectedKey) {
		t.Fatalf("expected matching selected key to apply to visible transcript")
	}
	if !m.shouldApplySessionProjectionToVisible("s1", "") {
		t.Fatalf("expected matching active session id to apply to visible transcript")
	}
	if m.shouldApplySessionProjectionToVisible("s2", "") {
		t.Fatalf("expected non-active session id to be ignored for visible projection")
	}
	if m.shouldApplySessionProjectionToVisible("s2", "sess:s2") {
		t.Fatalf("expected non-selected key to be ignored for visible projection")
	}
}

func TestHandleSessionItemsMessageErrorReturnsEarlyWithoutError(t *testing.T) {
	m := NewModel(nil)
	m.status = "unchanged"
	m.handleSessionItemsMessageError(sessionItemsMessageContext{
		source: sessionProjectionSourceHistory,
		id:     "s1",
		key:    "sess:s1",
	}, nil)
	if got := m.status; got != "unchanged" {
		t.Fatalf("expected no status update when error is nil, got %q", got)
	}

	var nilModel *Model
	nilModel.handleSessionItemsMessageError(sessionItemsMessageContext{
		source: sessionProjectionSourceHistory,
		id:     "s1",
		key:    "sess:s1",
	}, errors.New("boom"))
}

func fakeHistoryItems(n int) []map[string]any {
	items := make([]map[string]any, 0, n)
	for i := 0; i < n; i++ {
		items = append(items, map[string]any{
			"type": "agentMessage",
			"text": fmt.Sprintf("reply-%03d", i),
		})
	}
	return items
}

func tailBlockText(blocks []ChatBlock) string {
	if len(blocks) == 0 {
		return ""
	}
	return blocks[len(blocks)-1].Text
}
