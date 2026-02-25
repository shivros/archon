package app

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"control/internal/types"
)

type fakeDebugPanelView struct {
	lastContent string
	view        string
	height      int
	scrollUp    int
	scrollDown  int
	scrollLeft  int
	scrollRight int
	pageUp      int
	pageDown    int
	gotoTop     int
	gotoBottom  int
}

func (f *fakeDebugPanelView) Resize(int, int) {}
func (f *fakeDebugPanelView) SetContent(content string) {
	f.lastContent = content
}
func (f *fakeDebugPanelView) ScrollUp(lines int) bool {
	f.scrollUp += lines
	return true
}
func (f *fakeDebugPanelView) ScrollDown(lines int) bool {
	f.scrollDown += lines
	return true
}
func (f *fakeDebugPanelView) PageUp() bool {
	f.pageUp++
	return true
}
func (f *fakeDebugPanelView) PageDown() bool {
	f.pageDown++
	return true
}
func (f *fakeDebugPanelView) ScrollLeft(cols int) bool {
	f.scrollLeft += cols
	return true
}
func (f *fakeDebugPanelView) ScrollRight(cols int) bool {
	f.scrollRight += cols
	return true
}
func (f *fakeDebugPanelView) GotoTop() bool {
	f.gotoTop++
	return true
}
func (f *fakeDebugPanelView) GotoBottom() bool {
	f.gotoBottom++
	return true
}
func (f *fakeDebugPanelView) Height() int { return max(1, f.height) }
func (f *fakeDebugPanelView) View() (string, int) {
	return f.view, f.height
}

type fakeDebugStreamViewModel struct {
	content       string
	changed       bool
	closed        bool
	closedByClose bool
}

func (f *fakeDebugStreamViewModel) SetStream(<-chan types.DebugEvent, func()) {}
func (f *fakeDebugStreamViewModel) Reset()                                    {}
func (f *fakeDebugStreamViewModel) Close()                                    { f.closedByClose = true }
func (f *fakeDebugStreamViewModel) Lines() []string                           { return nil }
func (f *fakeDebugStreamViewModel) HasStream() bool                           { return true }
func (f *fakeDebugStreamViewModel) Content() string                           { return f.content }
func (f *fakeDebugStreamViewModel) ConsumeTick() ([]string, bool, bool) {
	return nil, f.changed, f.closed
}

func TestModelDebugInterfacesConsumeTickRefreshesPanelContent(t *testing.T) {
	m := NewModel(nil)
	panel := &fakeDebugPanelView{view: "panel", height: 2}
	stream := &fakeDebugStreamViewModel{content: "stream output", changed: true}
	m.debugPanel = panel
	m.debugStream = stream

	m.consumeDebugTick(time.Now())

	if panel.lastContent != "stream output" {
		t.Fatalf("expected panel content to be refreshed from stream, got %q", panel.lastContent)
	}
}

func TestModelDebugInterfacesConsumeTickClosedSetsStatus(t *testing.T) {
	m := NewModel(nil)
	m.debugPanel = &fakeDebugPanelView{}
	m.debugStream = &fakeDebugStreamViewModel{content: "", closed: true}

	m.consumeDebugTick(time.Now())

	if m.status != "debug stream closed" {
		t.Fatalf("expected closed status, got %q", m.status)
	}
}

func TestModelDebugInterfacesHandleViewportScrollRoutesToDebugPanel(t *testing.T) {
	m := NewModel(nil)
	panel := &fakeDebugPanelView{height: 4}
	m.debugPanel = panel
	m.debugPanelVisible = true
	m.appState.DebugStreamsEnabled = true

	if !m.handleViewportScroll(tea.KeyPressMsg{Code: 'J', Text: "J"}) {
		t.Fatalf("expected debug panel key to be handled")
	}
	if panel.scrollDown != 1 {
		t.Fatalf("expected debug panel vertical scroll down, got %d", panel.scrollDown)
	}
	if !m.handleViewportScroll(tea.KeyPressMsg{Code: 'L', Text: "L"}) {
		t.Fatalf("expected debug panel horizontal key to be handled")
	}
	if panel.scrollRight == 0 {
		t.Fatalf("expected debug panel horizontal scroll right")
	}
}

func TestModelDebugInterfacesHandleDebugPanelAllCommandBranches(t *testing.T) {
	m := NewModel(nil)
	panel := &fakeDebugPanelView{height: 6}
	m.debugPanel = panel
	m.debugPanelVisible = true
	m.appState.DebugStreamsEnabled = true
	_ = m.handleDebugPanelScrollKey(tea.KeyPressMsg{Code: tea.KeyUp, Mod: tea.ModShift})
	_ = m.handleDebugPanelScrollKey(tea.KeyPressMsg{Code: tea.KeyDown, Mod: tea.ModShift})
	_ = m.handleDebugPanelScrollKey(tea.KeyPressMsg{Code: tea.KeyLeft, Mod: tea.ModShift})
	_ = m.handleDebugPanelScrollKey(tea.KeyPressMsg{Code: tea.KeyRight, Mod: tea.ModShift})
	_ = m.handleDebugPanelScrollKey(tea.KeyPressMsg{Code: tea.KeyPgDown, Mod: tea.ModShift})
	_ = m.handleDebugPanelScrollKey(tea.KeyPressMsg{Code: tea.KeyPgUp, Mod: tea.ModShift})
	_ = m.handleDebugPanelScrollKey(tea.KeyPressMsg{Code: tea.KeyHome, Mod: tea.ModShift})
	_ = m.handleDebugPanelScrollKey(tea.KeyPressMsg{Code: tea.KeyEnd, Mod: tea.ModShift})

	if panel.scrollUp == 0 || panel.scrollDown == 0 || panel.scrollLeft == 0 || panel.scrollRight == 0 {
		t.Fatalf("expected directional command branches to execute")
	}
	if panel.pageDown == 0 || panel.pageUp == 0 {
		t.Fatalf("expected page branches to execute")
	}
	if panel.gotoTop == 0 || panel.gotoBottom == 0 {
		t.Fatalf("expected goto branches to execute")
	}
}

func TestModelDebugInterfacesHandleDebugPanelShiftArrowFallbacks(t *testing.T) {
	m := NewModel(nil)
	panel := &fakeDebugPanelView{height: 6}
	m.debugPanel = panel
	m.debugPanelVisible = true
	m.appState.DebugStreamsEnabled = true
	// Override command bindings so fallback paths are required.
	m.applyKeybindings(NewKeybindings(map[string]string{
		KeyCommandDebugPanelUp:    "f1",
		KeyCommandDebugPanelDown:  "f2",
		KeyCommandDebugPanelLeft:  "f3",
		KeyCommandDebugPanelRight: "f4",
	}))

	_ = m.handleDebugPanelScrollKey(tea.KeyPressMsg{Code: tea.KeyUp, Mod: tea.ModShift})
	_ = m.handleDebugPanelScrollKey(tea.KeyPressMsg{Code: tea.KeyDown, Mod: tea.ModShift})
	_ = m.handleDebugPanelScrollKey(tea.KeyPressMsg{Code: tea.KeyLeft, Mod: tea.ModShift})
	_ = m.handleDebugPanelScrollKey(tea.KeyPressMsg{Code: tea.KeyRight, Mod: tea.ModShift})

	if panel.scrollUp == 0 || panel.scrollDown == 0 || panel.scrollLeft == 0 || panel.scrollRight == 0 {
		t.Fatalf("expected shift-arrow fallback branches to execute")
	}
}

func TestModelDebugInterfacesHandleDebugPanelNotNavigable(t *testing.T) {
	m := NewModel(nil)
	panel := &fakeDebugPanelView{height: 5}
	m.debugPanel = panel
	m.debugPanelVisible = false
	m.appState.DebugStreamsEnabled = true

	if m.handleDebugPanelScrollKey(tea.KeyPressMsg{Code: 'J', Text: "J"}) {
		t.Fatalf("expected key handler to return false when panel hidden")
	}
}

func TestModelDebugInterfacesDebugPanelWheelScroll(t *testing.T) {
	m := NewModel(nil)
	panel := &fakeDebugPanelView{height: 5}
	m.debugPanel = panel
	m.debugPanelVisible = true
	m.appState.DebugStreamsEnabled = true
	layout := mouseLayout{panelVisible: true, panelStart: 20, panelWidth: 30}

	handled := m.reduceDebugPanelWheelMouse(tea.MouseClickMsg{Button: tea.MouseWheelDown, X: 22, Y: 2}, layout, 1)
	if !handled {
		t.Fatalf("expected wheel event to be routed to debug panel")
	}
	if panel.scrollDown != 3 {
		t.Fatalf("expected wheel-down to scroll panel down by 3, got %d", panel.scrollDown)
	}
}

func TestModelDebugInterfacesDebugPanelWheelGuards(t *testing.T) {
	m := NewModel(nil)
	panel := &fakeDebugPanelView{height: 5}
	m.debugPanel = panel
	m.debugPanelVisible = true
	m.appState.DebugStreamsEnabled = true

	if m.reduceDebugPanelWheelMouse(tea.MouseClickMsg{Button: tea.MouseWheelDown, X: 22, Y: 2}, mouseLayout{}, 1) {
		t.Fatalf("expected false when panel is not visible in layout")
	}
	layout := mouseLayout{panelVisible: true, panelStart: 20, panelWidth: 10}
	if m.reduceDebugPanelWheelMouse(tea.MouseClickMsg{Button: tea.MouseWheelDown, X: 5, Y: 2}, layout, 1) {
		t.Fatalf("expected false when X outside panel")
	}
	if m.reduceDebugPanelWheelMouse(tea.MouseClickMsg{Button: tea.MouseWheelDown, X: 22, Y: 99}, layout, 1) {
		t.Fatalf("expected false when Y outside panel")
	}
	m.debugPanel = nil
	if m.reduceDebugPanelWheelMouse(tea.MouseClickMsg{Button: tea.MouseWheelDown, X: 22, Y: 1}, layout, 1) {
		t.Fatalf("expected false when debug panel is nil")
	}
}

func TestModelDebugInterfacesQuitClosesDebugStream(t *testing.T) {
	m := NewModel(nil)
	stream := &fakeDebugStreamViewModel{}
	m.debugStream = stream

	handled, cmd := m.reduceMenuAndAppKeys(tea.KeyPressMsg{Text: "q", Code: 'q'})
	if !handled {
		t.Fatalf("expected quit key to be handled")
	}
	if cmd == nil {
		t.Fatalf("expected quit command")
	}
	if !stream.closedByClose {
		t.Fatalf("expected debug stream to be closed on quit")
	}
}
