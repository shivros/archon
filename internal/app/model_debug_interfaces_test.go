package app

import (
	"testing"
	"time"

	"control/internal/types"
)

type fakeDebugPanelView struct {
	lastContent string
	view        string
	height      int
}

func (f *fakeDebugPanelView) Resize(int, int) {}
func (f *fakeDebugPanelView) SetContent(content string) {
	f.lastContent = content
}
func (f *fakeDebugPanelView) View() (string, int) {
	return f.view, f.height
}

type fakeDebugStreamViewModel struct {
	content string
	changed bool
	closed  bool
}

func (f *fakeDebugStreamViewModel) SetStream(<-chan types.DebugEvent, func()) {}
func (f *fakeDebugStreamViewModel) Reset()                                    {}
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
