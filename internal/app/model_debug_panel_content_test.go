package app

import (
	"testing"

	tea "charm.land/bubbletea/v2"

	"control/internal/types"
)

func applyDebugProjectionCmdForContentTests(m *Model, cmd tea.Cmd) {
	if m == nil || cmd == nil {
		return
	}
	msg := cmd()
	projected, ok := msg.(debugPanelProjectedMsg)
	if !ok {
		return
	}
	m.applyDebugPanelProjection(projected)
}

type debugConsumerNoSnapshot struct{}

func (debugConsumerNoSnapshot) SetStream(<-chan types.DebugEvent, func()) {}
func (debugConsumerNoSnapshot) Reset()                                    {}
func (debugConsumerNoSnapshot) Close()                                    {}
func (debugConsumerNoSnapshot) HasStream() bool                           { return true }
func (debugConsumerNoSnapshot) ConsumeTick() ([]string, bool, bool)       { return nil, false, false }

type fakeDebugPresenter struct {
	presentation DebugPanelPresentation
}

func (f fakeDebugPresenter) Present([]DebugStreamEntry, int, DebugPanelPresentationState) DebugPanelPresentation {
	return f.presentation
}

type fakeDebugBlocksRenderer struct {
	rendered string
	spans    []renderedBlockSpan
}

func (f fakeDebugBlocksRenderer) Render([]ChatBlock, int, map[string]ChatBlockMetaPresentation) (string, []renderedBlockSpan) {
	return f.rendered, f.spans
}

func TestLoadDebugEntriesAdoptsSnapshotFromConsumerWhenPossible(t *testing.T) {
	m := NewModel(nil)
	stream := &fakeDebugStreamViewModel{entries: []DebugStreamEntry{{ID: "debug-1", Display: "entry"}}}
	m.debugStream = stream
	m.debugStreamSnapshot = nil

	entries := m.loadDebugEntries()
	if len(entries) != 1 {
		t.Fatalf("expected one entry via adopted snapshot, got %#v", entries)
	}
	if m.debugStreamSnapshot == nil {
		t.Fatalf("expected snapshot interface to be adopted from debug consumer")
	}
}

func TestLoadDebugEntriesReturnsNilWhenConsumerLacksSnapshot(t *testing.T) {
	m := NewModel(nil)
	m.debugStream = debugConsumerNoSnapshot{}
	m.debugStreamSnapshot = nil
	if entries := m.loadDebugEntries(); len(entries) != 0 {
		t.Fatalf("expected nil/empty entries when no snapshot is available, got %#v", entries)
	}
}

func TestPresentDebugEntriesInitializesPresenterFallback(t *testing.T) {
	m := NewModel(nil)
	m.debugPanelPresenter = nil
	m.debugPanelWidth = 60
	m.debugPanelExpandedByID = map[string]bool{}

	presentation := m.presentDebugEntries([]DebugStreamEntry{{ID: "debug-1", Display: "hello"}})
	if m.debugPanelPresenter == nil {
		t.Fatalf("expected presenter fallback to be initialized")
	}
	if len(presentation.Blocks) != 1 {
		t.Fatalf("expected one presented block, got %d", len(presentation.Blocks))
	}
}

func TestApplyDebugPanelPresentationInitializesRendererFallback(t *testing.T) {
	m := NewModel(nil)
	m.debugPanel = &fakeDebugPanelView{height: 5}
	m.debugPanelWidth = 40
	m.debugPanelBlocksRenderer = nil
	p := DebugPanelPresentation{
		Blocks:       []ChatBlock{{ID: "debug-1", Role: ChatRoleSystem, Text: "body"}},
		MetaByID:     map[string]ChatBlockMetaPresentation{"debug-1": {Label: "Debug Event"}},
		CopyTextByID: map[string]string{"debug-1": "body"},
	}

	m.applyDebugPanelPresentation(p)
	if m.debugPanelBlocksRenderer == nil {
		t.Fatalf("expected renderer fallback to be initialized")
	}
	if len(m.debugPanelBlocks) != 1 || len(m.debugPanelSpans) == 0 {
		t.Fatalf("expected rendered debug presentation to be applied")
	}
}

func TestApplyDebugPanelPresentationEmptyRenderFallsBackToWaitingMessage(t *testing.T) {
	m := NewModel(nil)
	panel := &fakeDebugPanelView{height: 5}
	m.debugPanel = panel
	m.debugPanelWidth = 40
	m.debugPanelBlocksRenderer = fakeDebugBlocksRenderer{rendered: "", spans: nil}
	m.applyDebugPanelPresentation(DebugPanelPresentation{})
	if panel.lastContent != debugPanelWaitingMessage {
		t.Fatalf("expected waiting message fallback, got %q", panel.lastContent)
	}
}

func TestApplyDebugPanelControlUnknownControlNoop(t *testing.T) {
	m := NewModel(nil)
	m.debugPanelExpandedByID = map[string]bool{"debug-1": false}
	if cmd := m.applyDebugPanelControl(debugPanelControlHit{BlockID: "debug-1", ControlID: "unknown"}); cmd != nil {
		t.Fatalf("expected unknown control to return nil command")
	}
	if m.debugPanelExpandedByID["debug-1"] {
		t.Fatalf("expected unknown control to leave expanded state unchanged")
	}
}

func TestApplyDebugPanelEmptyClearsState(t *testing.T) {
	m := NewModel(nil)
	panel := &fakeDebugPanelView{height: 5}
	m.debugPanel = panel
	m.debugPanelBlocks = []ChatBlock{{ID: "debug-1"}}
	m.debugPanelSpans = []renderedBlockSpan{{ID: "debug-1"}}
	m.debugPanelMetaByID = map[string]ChatBlockMetaPresentation{"debug-1": {Label: "Debug Event"}}
	m.debugPanelCopyByID = map[string]string{"debug-1": "text"}

	m.applyDebugPanelEmpty()
	if len(m.debugPanelBlocks) != 0 || len(m.debugPanelSpans) != 0 || len(m.debugPanelMetaByID) != 0 || len(m.debugPanelCopyByID) != 0 {
		t.Fatalf("expected debug panel state to be cleared")
	}
	if panel.lastContent != debugPanelWaitingMessage {
		t.Fatalf("expected waiting message content, got %q", panel.lastContent)
	}
}

func TestRefreshDebugPanelContentUsesInjectedPresenter(t *testing.T) {
	m := NewModel(nil)
	panel := &fakeDebugPanelView{height: 5}
	m.debugPanel = panel
	m.debugPanelWidth = 40
	stream := &fakeDebugStreamViewModel{entries: []DebugStreamEntry{{ID: "debug-1", Display: "stream"}}}
	m.debugStream = stream
	m.debugStreamSnapshot = stream
	m.debugPanelPresenter = fakeDebugPresenter{presentation: DebugPanelPresentation{
		Blocks:       []ChatBlock{{ID: "debug-1", Role: ChatRoleSystem, Text: "from presenter"}},
		MetaByID:     map[string]ChatBlockMetaPresentation{"debug-1": {Label: "Debug Event"}},
		CopyTextByID: map[string]string{"debug-1": "from presenter"},
	}}
	m.debugPanelBlocksRenderer = fakeDebugBlocksRenderer{
		rendered: "rendered by fake",
		spans:    []renderedBlockSpan{{ID: "debug-1", StartLine: 0, EndLine: 0}},
	}

	applyDebugProjectionCmdForContentTests(&m, m.refreshDebugPanelContent())
	if panel.lastContent != "rendered by fake" {
		t.Fatalf("expected injected presenter/renderer output, got %q", panel.lastContent)
	}
}

func TestRefreshDebugPanelContentDefersWhenProjectionIsInFlight(t *testing.T) {
	m := NewModel(nil)
	panel := &fakeDebugPanelView{height: 5}
	m.debugPanel = panel
	m.debugPanelVisible = true
	m.debugPanelWidth = 40
	stream := &fakeDebugStreamViewModel{entries: []DebugStreamEntry{{ID: "debug-1", Display: "stream"}}}
	m.debugStream = stream
	m.debugStreamSnapshot = stream
	m.debugPanelLoading = true
	m.debugPanelRefreshPending = false

	cmd := m.refreshDebugPanelContent()
	if cmd != nil {
		t.Fatalf("expected no projection command while loading")
	}
	if !m.debugPanelRefreshPending {
		t.Fatalf("expected pending refresh to be queued")
	}
}

func TestRefreshDebugPanelContentDefersWhenPanelNotVisible(t *testing.T) {
	m := NewModel(nil)
	panel := &fakeDebugPanelView{height: 5}
	m.debugPanel = panel
	m.debugPanelVisible = false
	m.debugPanelWidth = 40
	m.appState.DebugStreamsEnabled = true
	stream := &fakeDebugStreamViewModel{entries: []DebugStreamEntry{{ID: "debug-1", Display: "stream"}}}
	m.debugStream = stream
	m.debugStreamSnapshot = stream
	m.debugPanelRefreshPending = false

	cmd := m.refreshDebugPanelContent()
	if cmd != nil {
		t.Fatalf("expected no projection command when panel is hidden")
	}
	if !m.debugPanelRefreshPending {
		t.Fatalf("expected hidden panel refresh to be deferred")
	}
}
