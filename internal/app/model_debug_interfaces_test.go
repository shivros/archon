package app

import (
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	xansi "github.com/charmbracelet/x/ansi"

	"control/internal/types"
)

type fakeDebugPanelView struct {
	lastContent string
	view        string
	height      int
	yOffset     int
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
func (f *fakeDebugPanelView) YOffset() int {
	return max(0, f.yOffset)
}
func (f *fakeDebugPanelView) View() (string, int) {
	return f.view, f.height
}

type fakeDebugStreamViewModel struct {
	content       string
	entries       []DebugStreamEntry
	changed       bool
	closed        bool
	closedByClose bool
}

func (f *fakeDebugStreamViewModel) SetStream(<-chan types.DebugEvent, func()) {}
func (f *fakeDebugStreamViewModel) Reset()                                    {}
func (f *fakeDebugStreamViewModel) Close()                                    { f.closedByClose = true }
func (f *fakeDebugStreamViewModel) Lines() []string                           { return nil }
func (f *fakeDebugStreamViewModel) Entries() []DebugStreamEntry               { return f.entries }
func (f *fakeDebugStreamViewModel) HasStream() bool                           { return true }
func (f *fakeDebugStreamViewModel) Content() string                           { return f.content }
func (f *fakeDebugStreamViewModel) ConsumeTick() ([]string, bool, bool) {
	return nil, f.changed, f.closed
}

func TestModelDebugInterfacesConsumeTickRefreshesPanelContent(t *testing.T) {
	m := NewModel(nil)
	panel := &fakeDebugPanelView{view: "panel", height: 2}
	stream := &fakeDebugStreamViewModel{
		content: "stream output",
		entries: []DebugStreamEntry{{ID: "debug-1", Display: "stream output"}},
		changed: true,
	}
	m.debugPanel = panel
	m.debugStream = stream
	m.debugStreamSnapshot = stream
	m.debugPanelWidth = 80

	m.consumeDebugTick(time.Now())

	if plain := xansi.Strip(panel.lastContent); plain == "" || !strings.Contains(plain, "stream output") {
		t.Fatalf("expected panel content to be refreshed from stream, got %q", panel.lastContent)
	}
}

func TestModelDebugInterfacesConsumeTickClosedSetsStatus(t *testing.T) {
	m := NewModel(nil)
	m.debugPanel = &fakeDebugPanelView{}
	stream := &fakeDebugStreamViewModel{content: "", closed: true}
	m.debugStream = stream
	m.debugStreamSnapshot = stream

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
}

func TestModelDebugInterfacesHandleDebugPanelAllCommandBranches(t *testing.T) {
	m := NewModel(nil)
	panel := &fakeDebugPanelView{height: 6}
	m.debugPanel = panel
	m.debugPanelVisible = true
	m.appState.DebugStreamsEnabled = true
	_ = m.handleDebugPanelScrollKey(tea.KeyPressMsg{Code: tea.KeyUp, Mod: tea.ModShift})
	_ = m.handleDebugPanelScrollKey(tea.KeyPressMsg{Code: tea.KeyDown, Mod: tea.ModShift})
	_ = m.handleDebugPanelScrollKey(tea.KeyPressMsg{Code: tea.KeyPgDown, Mod: tea.ModShift})
	_ = m.handleDebugPanelScrollKey(tea.KeyPressMsg{Code: tea.KeyPgUp, Mod: tea.ModShift})
	_ = m.handleDebugPanelScrollKey(tea.KeyPressMsg{Code: tea.KeyHome, Mod: tea.ModShift})
	_ = m.handleDebugPanelScrollKey(tea.KeyPressMsg{Code: tea.KeyEnd, Mod: tea.ModShift})

	if panel.scrollUp == 0 || panel.scrollDown == 0 {
		t.Fatalf("expected vertical command branches to execute")
	}
	if panel.pageDown == 0 || panel.pageUp == 0 {
		t.Fatalf("expected page branches to execute")
	}
	if panel.gotoTop == 0 || panel.gotoBottom == 0 {
		t.Fatalf("expected goto branches to execute")
	}
}

func TestModelDebugInterfacesHandleDebugPanelShiftArrowVerticalFallbacks(t *testing.T) {
	m := NewModel(nil)
	panel := &fakeDebugPanelView{height: 6}
	m.debugPanel = panel
	m.debugPanelVisible = true
	m.appState.DebugStreamsEnabled = true
	// Override command bindings so fallback paths are required.
	m.applyKeybindings(NewKeybindings(map[string]string{
		KeyCommandDebugPanelUp:   "f1",
		KeyCommandDebugPanelDown: "f2",
	}))

	_ = m.handleDebugPanelScrollKey(tea.KeyPressMsg{Code: tea.KeyUp, Mod: tea.ModShift})
	_ = m.handleDebugPanelScrollKey(tea.KeyPressMsg{Code: tea.KeyDown, Mod: tea.ModShift})

	if panel.scrollUp == 0 || panel.scrollDown == 0 {
		t.Fatalf("expected shift-arrow vertical fallback branches to execute")
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

func TestModelDebugInterfacesDebugPanelLeftPressCopy(t *testing.T) {
	m := NewModel(nil)
	m.appState.DebugStreamsEnabled = true
	m.debugPanelVisible = true
	m.debugPanelWidth = 72
	m.debugPanel.Resize(72, 10)
	m.debugStream = &fakeDebugStreamViewModel{
		entries: []DebugStreamEntry{{ID: "debug-1", Display: "copy me"}},
	}
	m.debugStreamSnapshot = m.debugStream.(debugStreamSnapshot)
	m.refreshDebugPanelContent()
	layout := mouseLayout{panelVisible: true, panelStart: 20, panelWidth: 72}

	span := m.debugPanelSpans[0]
	copyHit := renderedMetaControlHit{}
	found := false
	for _, control := range span.MetaControls {
		if control.ID == debugMetaControlCopy {
			copyHit = control
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected copy control hitbox in debug panel span")
	}
	x := layout.panelStart + copyHit.Start
	y := copyHit.Line - m.debugPanel.YOffset() + 1
	handled := m.reduceDebugPanelLeftPressMouse(tea.MouseClickMsg{Button: tea.MouseLeft, X: x, Y: y}, layout)
	if !handled {
		t.Fatalf("expected debug panel copy click to be handled")
	}
	if m.pendingMouseCmd == nil {
		t.Fatalf("expected copy click to enqueue clipboard command")
	}
}

func TestModelDebugInterfacesDebugPanelLeftPressToggle(t *testing.T) {
	m := NewModel(nil)
	m.appState.DebugStreamsEnabled = true
	m.debugPanelVisible = true
	m.debugPanelWidth = 72
	m.debugPanel.Resize(72, 10)
	m.debugStream = &fakeDebugStreamViewModel{
		entries: []DebugStreamEntry{{ID: "debug-1", Display: "l1\nl2\nl3\nl4\nl5\nl6"}},
	}
	m.debugStreamSnapshot = m.debugStream.(debugStreamSnapshot)
	m.refreshDebugPanelContent()
	layout := mouseLayout{panelVisible: true, panelStart: 20, panelWidth: 72}

	span := m.debugPanelSpans[0]
	toggleHit := renderedMetaControlHit{}
	found := false
	for _, control := range span.MetaControls {
		if control.ID == debugMetaControlToggle {
			toggleHit = control
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected toggle control hitbox in debug panel span")
	}
	x := layout.panelStart + toggleHit.Start
	y := toggleHit.Line - m.debugPanel.YOffset() + 1
	handled := m.reduceDebugPanelLeftPressMouse(tea.MouseClickMsg{Button: tea.MouseLeft, X: x, Y: y}, layout)
	if !handled {
		t.Fatalf("expected debug panel toggle click to be handled")
	}
	if !m.debugPanelExpandedByID["debug-1"] {
		t.Fatalf("expected debug event to toggle expanded state")
	}
}

func TestModelDebugInterfacesDebugPanelLeftPressNotOnControl(t *testing.T) {
	m := NewModel(nil)
	m.appState.DebugStreamsEnabled = true
	m.debugPanelVisible = true
	m.debugPanelWidth = 72
	m.debugPanel.Resize(72, 10)
	m.debugStream = &fakeDebugStreamViewModel{
		entries: []DebugStreamEntry{{ID: "debug-1", Display: "plain text"}},
	}
	m.debugStreamSnapshot = m.debugStream.(debugStreamSnapshot)
	m.refreshDebugPanelContent()
	layout := mouseLayout{panelVisible: true, panelStart: 20, panelWidth: 72}

	handled := m.reduceDebugPanelLeftPressMouse(tea.MouseClickMsg{Button: tea.MouseLeft, X: layout.panelStart + 2, Y: 3}, layout)
	if handled {
		t.Fatalf("expected body click outside control hitboxes to be ignored")
	}
}

func TestModelDebugInterfacesDebugPanelLeftPressInitializesInteractionFallback(t *testing.T) {
	m := NewModel(nil)
	m.appState.DebugStreamsEnabled = true
	m.debugPanelVisible = true
	m.debugPanelWidth = 72
	m.debugPanel.Resize(72, 10)
	m.debugPanelInteractionService = nil
	m.debugStream = &fakeDebugStreamViewModel{
		entries: []DebugStreamEntry{{ID: "debug-1", Display: "l1\nl2\nl3\nl4\nl5\nl6"}},
	}
	m.debugStreamSnapshot = m.debugStream.(debugStreamSnapshot)
	m.refreshDebugPanelContent()
	layout := mouseLayout{panelVisible: true, panelStart: 20, panelWidth: 72}

	span := m.debugPanelSpans[0]
	toggleHit := renderedMetaControlHit{}
	for _, control := range span.MetaControls {
		if control.ID == debugMetaControlToggle {
			toggleHit = control
			break
		}
	}
	x := layout.panelStart + toggleHit.Start
	y := toggleHit.Line - m.debugPanel.YOffset() + 1
	handled := m.reduceDebugPanelLeftPressMouse(tea.MouseClickMsg{Button: tea.MouseLeft, X: x, Y: y}, layout)
	if !handled {
		t.Fatalf("expected click to be handled via fallback interaction service")
	}
	if m.debugPanelInteractionService == nil {
		t.Fatalf("expected fallback interaction service to be initialized")
	}
}

func TestModelDebugInterfacesQuitClosesDebugStream(t *testing.T) {
	m := NewModel(nil)
	stream := &fakeDebugStreamViewModel{}
	m.debugStream = stream
	m.debugStreamSnapshot = stream

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
