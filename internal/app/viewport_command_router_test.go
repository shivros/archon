package app

import (
	"fmt"
	"testing"
)

type stubViewportCommandContext struct {
	debugNavigable bool
	debugTop       int
	debugBottom    int
	transcriptTop  int
	enableFollow   []bool
	pauseFollow    []bool
}

func (s *stubViewportCommandContext) DebugPanelNavigable() bool { return s.debugNavigable }
func (s *stubViewportCommandContext) DebugPanelGotoTop() bool {
	s.debugTop++
	return true
}
func (s *stubViewportCommandContext) DebugPanelGotoBottom() bool {
	s.debugBottom++
	return true
}
func (s *stubViewportCommandContext) TranscriptGotoTop() { s.transcriptTop++ }
func (s *stubViewportCommandContext) EnableFollow(value bool) {
	s.enableFollow = append(s.enableFollow, value)
}
func (s *stubViewportCommandContext) PauseFollow(value bool) {
	s.pauseFollow = append(s.pauseFollow, value)
}

type stubViewportCommandRouter struct {
	topCalls    int
	bottomCalls int
}

func (s *stubViewportCommandRouter) RouteTop(ViewportCommandContext) bool {
	s.topCalls++
	return true
}

func (s *stubViewportCommandRouter) RouteBottom(ViewportCommandContext) bool {
	s.bottomCalls++
	return true
}

func TestDefaultViewportCommandRouterRoutesTopToDebugPanel(t *testing.T) {
	router := defaultViewportCommandRouter{}
	ctx := &stubViewportCommandContext{debugNavigable: true}

	if !router.RouteTop(ctx) {
		t.Fatalf("expected top route to be handled")
	}
	if ctx.debugTop != 1 || ctx.transcriptTop != 0 {
		t.Fatalf("expected debug top route, got debug=%d transcript=%d", ctx.debugTop, ctx.transcriptTop)
	}
}

func TestDefaultViewportCommandRouterRoutesTopToTranscript(t *testing.T) {
	router := defaultViewportCommandRouter{}
	ctx := &stubViewportCommandContext{debugNavigable: false}

	if !router.RouteTop(ctx) {
		t.Fatalf("expected top route to be handled")
	}
	if ctx.transcriptTop != 1 {
		t.Fatalf("expected transcript top route")
	}
	if len(ctx.pauseFollow) != 1 || !ctx.pauseFollow[0] {
		t.Fatalf("expected pause follow(true), got %#v", ctx.pauseFollow)
	}
}

func TestDefaultViewportCommandRouterRoutesBottomByContext(t *testing.T) {
	router := defaultViewportCommandRouter{}
	debugCtx := &stubViewportCommandContext{debugNavigable: true}
	chatCtx := &stubViewportCommandContext{debugNavigable: false}

	if !router.RouteBottom(debugCtx) {
		t.Fatalf("expected debug bottom route to be handled")
	}
	if debugCtx.debugBottom != 1 || len(debugCtx.enableFollow) != 0 {
		t.Fatalf("expected debug bottom to avoid follow toggles, got debug=%d follow=%#v", debugCtx.debugBottom, debugCtx.enableFollow)
	}
	if !router.RouteBottom(chatCtx) {
		t.Fatalf("expected chat bottom route to be handled")
	}
	if len(chatCtx.enableFollow) != 1 || !chatCtx.enableFollow[0] {
		t.Fatalf("expected enable follow(true), got %#v", chatCtx.enableFollow)
	}
}

func TestWithViewportCommandRouterConfiguresAndResetsDefault(t *testing.T) {
	custom := &stubViewportCommandRouter{}
	m := NewModel(nil, WithViewportCommandRouter(custom))
	if m.viewportCommandRouter != custom {
		t.Fatalf("expected custom viewport command router")
	}
	WithViewportCommandRouter(nil)(&m)
	if _, ok := m.viewportCommandRouter.(defaultViewportCommandRouter); !ok {
		t.Fatalf("expected default viewport command router after reset, got %T", m.viewportCommandRouter)
	}
}

func TestWithViewportCommandRouterHandlesNilModel(t *testing.T) {
	WithViewportCommandRouter(&stubViewportCommandRouter{})(nil)
}

func TestDefaultViewportCommandRouterWithModelContextRoutesTranscriptTopAndFollow(t *testing.T) {
	m := NewModel(nil)
	m.resize(120, 30)
	blocks := make([]ChatBlock, 0, 120)
	for i := 0; i < 120; i++ {
		blocks = append(blocks, ChatBlock{Role: ChatRoleAgent, Text: fmt.Sprintf("line %d", i)})
	}
	m.applyBlocks(blocks)
	m.enableFollow(false)
	m.pauseFollow(false)
	m.viewport.GotoBottom()
	if m.viewport.YOffset() == 0 {
		t.Fatalf("expected non-zero starting offset")
	}

	router := defaultViewportCommandRouter{}
	if !router.RouteTop(newModelViewportCommandContext(&m)) {
		t.Fatalf("expected top route to be handled")
	}
	if got := m.viewport.YOffset(); got != 0 {
		t.Fatalf("expected transcript route to jump to top, got offset %d", got)
	}
	if m.follow {
		t.Fatalf("expected follow to pause after transcript top routing")
	}
}

func TestDefaultViewportCommandRouterWithModelContextBottomEnablesFollow(t *testing.T) {
	m := NewModel(nil)
	m.resize(120, 30)
	m.pauseFollow(false)
	if m.follow {
		t.Fatalf("expected follow to be paused at start")
	}

	router := defaultViewportCommandRouter{}
	if !router.RouteBottom(newModelViewportCommandContext(&m)) {
		t.Fatalf("expected bottom route to be handled")
	}
	if !m.follow {
		t.Fatalf("expected follow to be enabled by chat bottom routing")
	}
}

func TestViewportCommandRouterOrDefaultHandlesNilModel(t *testing.T) {
	var m *Model
	if _, ok := m.viewportCommandRouterOrDefault().(defaultViewportCommandRouter); !ok {
		t.Fatalf("expected default router for nil model")
	}
}

func TestDefaultViewportCommandRouterHandlesNilContext(t *testing.T) {
	router := defaultViewportCommandRouter{}
	if router.RouteTop(nil) {
		t.Fatalf("expected nil top context to be unhandled")
	}
	if router.RouteBottom(nil) {
		t.Fatalf("expected nil bottom context to be unhandled")
	}
}

func TestModelViewportCommandContextHandlesNilModel(t *testing.T) {
	ctx := newModelViewportCommandContext(nil).(modelViewportCommandContext)
	if ctx.DebugPanelNavigable() {
		t.Fatalf("expected nil model to report non-navigable")
	}
	if ctx.DebugPanelGotoTop() {
		t.Fatalf("expected nil model top to return false")
	}
	if ctx.DebugPanelGotoBottom() {
		t.Fatalf("expected nil model bottom to return false")
	}
	ctx.TranscriptGotoTop()
	ctx.EnableFollow(true)
	ctx.PauseFollow(true)
}
