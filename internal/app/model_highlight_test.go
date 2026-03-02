package app

import (
	"testing"

	tea "charm.land/bubbletea/v2"
)

type stubHighlightCoordinator struct {
	state        highlightState
	beginResult  bool
	updateResult bool
	endResult    bool
	clearResult  bool
}

func (s *stubHighlightCoordinator) Begin(_ tea.MouseMsg, _ mouseLayout) bool  { return s.beginResult }
func (s *stubHighlightCoordinator) Update(_ tea.MouseMsg, _ mouseLayout) bool { return s.updateResult }
func (s *stubHighlightCoordinator) End(_ tea.MouseMsg, _ mouseLayout) bool    { return s.endResult }
func (s *stubHighlightCoordinator) Clear() bool                               { return s.clearResult }
func (s *stubHighlightCoordinator) State() highlightState                     { return s.state }

func TestModelHighlightedMainBlockRangeGuardsByContextAndBounds(t *testing.T) {
	m := NewModel(nil)
	m.highlight = &stubHighlightCoordinator{
		state: highlightState{
			Range: highlightRange{
				HasSelection: true,
				Context:      highlightContextChatTranscript,
				BlockStart:   2,
				BlockEnd:     5,
			},
		},
	}
	start, end, ok := m.highlightedMainBlockRange()
	if !ok || start != 2 || end != 5 {
		t.Fatalf("expected transcript range 2..5, got %d..%d (ok=%v)", start, end, ok)
	}

	m.highlight = &stubHighlightCoordinator{
		state: highlightState{
			Range: highlightRange{
				HasSelection: true,
				Context:      highlightContextSideNotesPanel,
				BlockStart:   2,
				BlockEnd:     5,
			},
		},
	}
	if _, _, ok := m.highlightedMainBlockRange(); ok {
		t.Fatalf("expected notes panel context to be rejected for main range")
	}

	m.highlight = &stubHighlightCoordinator{
		state: highlightState{
			Range: highlightRange{
				HasSelection: true,
				Context:      highlightContextMainNotes,
				BlockStart:   4,
				BlockEnd:     3,
			},
		},
	}
	if _, _, ok := m.highlightedMainBlockRange(); ok {
		t.Fatalf("expected invalid bounds to be rejected")
	}
}

func TestModelHighlightedNotesPanelBlockRangeRequiresPanelContext(t *testing.T) {
	m := NewModel(nil)
	m.highlight = &stubHighlightCoordinator{
		state: highlightState{
			Range: highlightRange{
				HasSelection: true,
				Context:      highlightContextSideNotesPanel,
				BlockStart:   1,
				BlockEnd:     3,
			},
		},
	}
	start, end, ok := m.highlightedNotesPanelBlockRange()
	if !ok || start != 1 || end != 3 {
		t.Fatalf("expected panel range 1..3, got %d..%d (ok=%v)", start, end, ok)
	}

	m.highlight = &stubHighlightCoordinator{
		state: highlightState{
			Range: highlightRange{
				HasSelection: true,
				Context:      highlightContextChatTranscript,
				BlockStart:   1,
				BlockEnd:     3,
			},
		},
	}
	if _, _, ok := m.highlightedNotesPanelBlockRange(); ok {
		t.Fatalf("expected non-panel context to be rejected")
	}

	m.highlight = &stubHighlightCoordinator{
		state: highlightState{
			Range: highlightRange{
				HasSelection: true,
				Context:      highlightContextSideNotesPanel,
				BlockStart:   5,
				BlockEnd:     4,
			},
		},
	}
	if _, _, ok := m.highlightedNotesPanelBlockRange(); ok {
		t.Fatalf("expected invalid panel bounds to be rejected")
	}
}

func TestModelRebindHighlightAdapterUpdatesPointer(t *testing.T) {
	m := NewModel(nil)
	m.highlightAdapter = newModelHighlightAdapter(nil)

	m.rebindHighlightAdapter()

	if m.highlightAdapter.model != &m {
		t.Fatalf("expected adapter model rebound to current model instance")
	}
}

func TestModelRebindHighlightAdapterNilSafe(t *testing.T) {
	var m *Model
	m.rebindHighlightAdapter()

	withNilAdapter := NewModel(nil)
	withNilAdapter.highlightAdapter = nil
	withNilAdapter.rebindHighlightAdapter()
}
