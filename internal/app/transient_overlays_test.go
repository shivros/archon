package app

import (
	"strings"
	"testing"

	xansi "github.com/charmbracelet/x/ansi"
)

type testTransientOverlayProvider struct {
	overlay LayerOverlay
}

func (p testTransientOverlayProvider) Build(_ *Model, _ TransientOverlayContext) (LayerOverlay, bool) {
	return p.overlay, true
}

type testBlockJoiner struct {
	called int
}

func (j *testBlockJoiner) CombineHorizontal(left, right string, gap int) string {
	j.called++
	return "joined"
}

type testOverlayComposer struct {
	called       int
	lastOverlays []LayerOverlay
	output       string
}

func (c *testOverlayComposer) Compose(base string, overlays []LayerOverlay) string {
	c.called++
	c.lastOverlays = append(c.lastOverlays[:0], overlays...)
	if c.output != "" {
		return c.output
	}
	return base
}

type testLayerComposer struct {
	composeCalled bool
	joinCalled    bool
}

func (c *testLayerComposer) Compose(base string, overlays []LayerOverlay) string {
	c.composeCalled = true
	if len(overlays) == 0 {
		return base
	}
	return NewTextOverlayComposer().Compose(base, overlays)
}

func (c *testLayerComposer) CombineHorizontal(left, right string, gap int) string {
	c.joinCalled = true
	return "bridge-joined"
}

func TestOverlayTransientViewsUsesRegisteredOverlayProviders(t *testing.T) {
	m := NewModel(nil, WithTransientOverlayProviders([]TransientOverlayProvider{
		testTransientOverlayProvider{overlay: LayerOverlay{X: 2, Y: 1, Block: "ZZ"}},
	}))
	m.resize(20, 8)
	body := "0123456789\nabcdefghij\nklmnopqrst"

	got := m.overlayTransientViews(body)
	want := "0123456789\nabZZefghij\nklmnopqrst"
	if got != want {
		t.Fatalf("unexpected composed output:\nwant:\n%s\n\ngot:\n%s", want, got)
	}
}

func TestMenuDropdownOverlayProviderUsesInjectedBlockJoiner(t *testing.T) {
	joiner := &testBlockJoiner{}
	m := NewModel(nil,
		WithTransientOverlayProviders([]TransientOverlayProvider{menuDropdownOverlayProvider{}}),
		WithBlockJoiner(joiner),
	)
	m.resize(80, 20)
	if m.menu == nil {
		t.Fatalf("expected menu controller")
	}
	m.menu.OpenBar()
	m.menu.OpenDropdown()
	m.menu.OpenSubmenu(submenuWorkspaces)

	body := "base-line-0\nbase-line-1\nbase-line-2"
	got := m.overlayTransientViews(body)
	if joiner.called == 0 {
		t.Fatalf("expected injected block joiner to be called")
	}
	if got == body {
		t.Fatalf("expected dropdown overlay to change rendered output")
	}
}

func TestOverlayTransientViewsProviderOrderLastOverlayWins(t *testing.T) {
	m := NewModel(nil, WithTransientOverlayProviders([]TransientOverlayProvider{
		testTransientOverlayProvider{overlay: LayerOverlay{X: 1, Y: 0, Block: "A"}},
		testTransientOverlayProvider{overlay: LayerOverlay{X: 1, Y: 0, Block: "B"}},
	}))
	m.resize(20, 8)

	got := m.overlayTransientViews("12345")
	if got != "1B345" {
		t.Fatalf("expected later overlay to win on overlap, got %q", got)
	}
}

func TestWithOverlayComposerOverridesCompositionEngine(t *testing.T) {
	composer := &testOverlayComposer{output: "custom-output"}
	m := NewModel(nil,
		WithOverlayComposer(composer),
		WithTransientOverlayProviders([]TransientOverlayProvider{
			testTransientOverlayProvider{overlay: LayerOverlay{X: 0, Y: 0, Block: "X"}},
		}),
	)
	m.resize(20, 8)

	got := m.overlayTransientViews("base")
	if got != "custom-output" {
		t.Fatalf("expected custom overlay composer output, got %q", got)
	}
	if composer.called == 0 {
		t.Fatalf("expected custom overlay composer to be called")
	}
}

func TestWithLayerComposerCompatibilityBridgeSetsComposerAndJoiner(t *testing.T) {
	bridge := &testLayerComposer{}
	m := NewModel(nil,
		WithLayerComposer(bridge),
		WithTransientOverlayProviders([]TransientOverlayProvider{menuDropdownOverlayProvider{}}),
	)
	m.resize(80, 20)
	if m.menu == nil {
		t.Fatalf("expected menu controller")
	}
	m.menu.OpenBar()
	m.menu.OpenDropdown()
	m.menu.OpenSubmenu(submenuWorkspaces)

	got := m.overlayTransientViews("line0\nline1\nline2")
	if !bridge.composeCalled {
		t.Fatalf("expected bridge composer to be used")
	}
	if !bridge.joinCalled {
		t.Fatalf("expected bridge joiner to be used")
	}
	if !strings.Contains(xansi.Strip(got), "bridge-joined") {
		t.Fatalf("expected bridged join output in composed view")
	}
}

func TestTransientOverlayProvidersOptionNilOrEmptyFallsBackToDefaults(t *testing.T) {
	mNil := NewModel(nil, WithTransientOverlayProviders(nil))
	mNil.resize(80, 20)
	plainNil := xansi.Strip(mNil.overlayTransientViews("base0\nbase1"))
	if !strings.Contains(plainNil, "Workspaces") {
		t.Fatalf("expected default providers when nil slice supplied")
	}

	mEmpty := NewModel(nil, WithTransientOverlayProviders([]TransientOverlayProvider{}))
	mEmpty.resize(80, 20)
	plainEmpty := xansi.Strip(mEmpty.overlayTransientViews("base0\nbase1"))
	if !strings.Contains(plainEmpty, "Workspaces") {
		t.Fatalf("expected default providers when empty slice supplied")
	}
}

func TestTransientOverlayProvidersOptionFiltersNilEntries(t *testing.T) {
	m := NewModel(nil, WithTransientOverlayProviders([]TransientOverlayProvider{
		nil,
		testTransientOverlayProvider{overlay: LayerOverlay{X: 2, Y: 0, Block: "ZZ"}},
	}))
	m.resize(20, 8)

	got := m.overlayTransientViews("abcdef")
	if got != "abZZef" {
		t.Fatalf("expected non-nil provider to remain active, got %q", got)
	}
}

func TestOverlayComposerAndJoinerFallbacksWhenUnset(t *testing.T) {
	m := NewModel(nil, WithTransientOverlayProviders([]TransientOverlayProvider{
		testTransientOverlayProvider{overlay: LayerOverlay{X: 1, Y: 0, Block: "Q"}},
	}))
	m.resize(80, 20)
	m.overlayComposer = nil
	got := m.overlayTransientViews("abcd")
	if got != "aQcd" {
		t.Fatalf("expected fallback overlay composer to apply overlay, got %q", got)
	}

	m2 := NewModel(nil, WithTransientOverlayProviders([]TransientOverlayProvider{menuDropdownOverlayProvider{}}))
	m2.resize(80, 20)
	m2.overlayBlockJoiner = nil
	if m2.menu == nil {
		t.Fatalf("expected menu controller")
	}
	m2.menu.OpenBar()
	m2.menu.OpenDropdown()
	m2.menu.OpenSubmenu(submenuWorkspaces)
	plain := xansi.Strip(m2.overlayTransientViews("line0\nline1\nline2"))
	if !strings.Contains(plain, "Create Workspace") {
		t.Fatalf("expected fallback block joiner to build submenu")
	}
}

func TestToastOverlayProviderSkipsWhenStatusHistoryOpen(t *testing.T) {
	m := NewModel(nil)
	m.resize(80, 20)
	m.showWarningToast("transient message")
	m.statusHistoryOverlay.Open()

	provider := toastOverlayProvider{}
	if _, ok := provider.Build(&m, TransientOverlayContext{BodyHeight: 10}); ok {
		t.Fatalf("expected no toast overlay while status history is open")
	}
}

func TestTransientOverlayDefaultsHandleNilModel(t *testing.T) {
	var m *Model
	if m.overlayComposerOrDefault() == nil {
		t.Fatalf("expected default overlay composer for nil model")
	}
	if m.overlayBlockJoinerOrDefault() == nil {
		t.Fatalf("expected default block joiner for nil model")
	}
	if len(m.transientOverlayProvidersOrDefault()) == 0 {
		t.Fatalf("expected default overlay providers for nil model")
	}
}

func TestWithTransientOverlayProvidersOptionNoOpForNilModel(t *testing.T) {
	opt := WithTransientOverlayProviders([]TransientOverlayProvider{
		testTransientOverlayProvider{overlay: LayerOverlay{X: 0, Y: 0, Block: "X"}},
	})
	var m *Model
	opt(m)
}
