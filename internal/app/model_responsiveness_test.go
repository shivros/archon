package app

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

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
	if got := resolveMouseMode(false); got != tea.MouseModeCellMotion {
		t.Fatalf("expected cell motion when not dragging, got %v", got)
	}
	if got := resolveMouseMode(true); got != tea.MouseModeAllMotion {
		t.Fatalf("expected all motion when dragging, got %v", got)
	}
}

func TestReduceStateMessagesHistoryLargePayloadDefersProjection(t *testing.T) {
	m := NewModel(nil)
	m.pendingSessionKey = "sess:s1"
	m.loadingKey = "sess:s1"
	m.loading = true
	m.sessions = []*types.Session{{ID: "s1", Provider: "codex"}}

	handled, cmd := m.reduceStateMessages(historyMsg{
		id:    "s1",
		key:   "sess:s1",
		items: fakeHistoryItems(defaultSessionProjectionAsyncThreshold + 1),
	})
	if !handled {
		t.Fatalf("expected history message to be handled")
	}
	if cmd == nil {
		t.Fatalf("expected history projection to run asynchronously for large payload")
	}
	if m.loading {
		t.Fatalf("expected loading state to clear before async projection result")
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
	if len(m.currentBlocks()) == 0 {
		t.Fatalf("expected projected transcript blocks to be applied")
	}
}

func TestReduceStateMessagesIgnoresStaleSessionProjection(t *testing.T) {
	m := NewModel(nil)
	m.pendingSessionKey = "sess:s1"
	m.sessions = []*types.Session{{ID: "s1", Provider: "codex"}}

	handled, cmd1 := m.reduceStateMessages(historyMsg{
		id:    "s1",
		key:   "sess:s1",
		items: fakeHistoryItems(defaultSessionProjectionAsyncThreshold + 2),
	})
	if !handled || cmd1 == nil {
		t.Fatalf("expected first async projection command")
	}

	handled, cmd2 := m.reduceStateMessages(historyMsg{
		id:    "s1",
		key:   "sess:s1",
		items: fakeHistoryItems(defaultSessionProjectionAsyncThreshold + 3),
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

type testSessionProjectionPolicy struct {
	asyncAt   int
	maxTokens int
}

func (p testSessionProjectionPolicy) ShouldProjectAsync(itemCount int) bool {
	return itemCount >= p.asyncAt
}

func (p testSessionProjectionPolicy) MaxTrackedProjectionTokens() int {
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
	if policy.ShouldProjectAsync(5) {
		t.Fatalf("expected custom projection policy to gate async projection")
	}
	if got := policy.MaxTrackedProjectionTokens(); got != 3 {
		t.Fatalf("expected custom max tokens 3, got %d", got)
	}

	WithSessionProjectionPolicy(nil)(&m)
	defaultPolicy := m.sessionProjectionPolicyOrDefault()
	if !defaultPolicy.ShouldProjectAsync(defaultSessionProjectionAsyncThreshold) {
		t.Fatalf("expected default projection policy after nil reset")
	}
	if got := defaultPolicy.MaxTrackedProjectionTokens(); got != defaultSessionProjectionMaxTokens {
		t.Fatalf("expected default max tokens %d, got %d", defaultSessionProjectionMaxTokens, got)
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

	if got := len(m.sessionProjectionLatest); got != 2 {
		t.Fatalf("expected projection token tracker to be capped at 2, got %d", got)
	}
	if _, ok := m.sessionProjectionLatest["key:sess:s1:a"]; ok {
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

func TestConsumeSessionProjectionTokenRemovesOnlyMatchingLatest(t *testing.T) {
	m := NewModel(nil)
	m.sessionProjectionLatest = map[string]int{
		"key:sess:s1": 7,
		"id:s1":       3,
	}

	m.consumeSessionProjectionToken("sess:s1", "", 6)
	if _, ok := m.sessionProjectionLatest["key:sess:s1"]; !ok {
		t.Fatalf("expected token to remain when projection seq does not match latest")
	}

	m.consumeSessionProjectionToken("sess:s1", "", 7)
	if _, ok := m.sessionProjectionLatest["key:sess:s1"]; ok {
		t.Fatalf("expected matching latest key token to be consumed")
	}

	m.consumeSessionProjectionToken("", "s1", 0)
	if _, ok := m.sessionProjectionLatest["id:s1"]; !ok {
		t.Fatalf("expected seq<=0 to skip consumption")
	}

	m.consumeSessionProjectionToken(" ", " ", 3)
	if _, ok := m.sessionProjectionLatest["id:s1"]; !ok {
		t.Fatalf("expected blank token inputs to be ignored")
	}

	m.consumeSessionProjectionToken("", "s1", 3)
	if _, ok := m.sessionProjectionLatest["id:s1"]; ok {
		t.Fatalf("expected matching latest id token to be consumed")
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

	m.applySessionProjection(sessionProjectionSourceHistory, "other-session", "", []ChatBlock{
		{Role: ChatRoleAgent, Text: "ignored cache"},
	})

	if got := len(m.transcriptCache); got != 0 {
		t.Fatalf("expected no cache writes for non-selected session without key, got %d", got)
	}
	if got := m.status; got != "history updated" {
		t.Fatalf("expected background status update, got %q", got)
	}
}

func TestNextSessionProjectionSeqHandlesNilAndBlankTokens(t *testing.T) {
	var nilModel *Model
	if got := nilModel.nextSessionProjectionSeq("key:s1", 1); got != 0 {
		t.Fatalf("expected nil model seq to be 0, got %d", got)
	}

	m := NewModel(nil)
	m.sessionProjectionLatest = nil
	if got := m.nextSessionProjectionSeq("   ", 1); got != 0 {
		t.Fatalf("expected blank token seq to be 0, got %d", got)
	}
	if m.sessionProjectionLatest != nil {
		t.Fatalf("expected blank token to avoid tracker map allocation")
	}

	seq := m.nextSessionProjectionSeq("key:a", 0)
	if seq != 1 {
		t.Fatalf("expected first seq to be 1, got %d", seq)
	}
	if got := m.sessionProjectionLatest["key:a"]; got != 1 {
		t.Fatalf("expected token to be tracked at seq 1, got %d", got)
	}
}

func TestIsCurrentSessionProjectionCoversGuardBranches(t *testing.T) {
	var nilModel *Model
	if !nilModel.isCurrentSessionProjection("sess:s1", "s1", 5) {
		t.Fatalf("expected nil model to treat projection as current")
	}

	m := NewModel(nil)
	if !m.isCurrentSessionProjection("sess:s1", "s1", 0) {
		t.Fatalf("expected non-positive seq to be treated as current")
	}
	if m.isCurrentSessionProjection(" ", " ", 1) {
		t.Fatalf("expected empty token to be treated as stale")
	}

	m.sessionProjectionLatest = nil
	if m.isCurrentSessionProjection("sess:s1", "s1", 1) {
		t.Fatalf("expected nil tracker map to treat projection as stale")
	}

	m.sessionProjectionLatest = map[string]int{"key:sess:s1": 7}
	if m.isCurrentSessionProjection("sess:other", "s1", 7) {
		t.Fatalf("expected unknown token to be treated as stale")
	}
	if m.isCurrentSessionProjection("sess:s1", "s1", 6) {
		t.Fatalf("expected mismatched seq to be treated as stale")
	}
	if !m.isCurrentSessionProjection("sess:s1", "s1", 7) {
		t.Fatalf("expected matching seq to be treated as current")
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
