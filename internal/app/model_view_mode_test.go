package app

import "testing"

func TestVisibleInputPanelLayoutReturnsFalseWithoutLayout(t *testing.T) {
	if _, ok := visibleInputPanelLayout(InputPanelLayout{}, false); ok {
		t.Fatalf("expected missing layout to be hidden")
	}
}

func TestVisibleInputPanelLayoutReturnsFalseForEmptyRenderedView(t *testing.T) {
	layout := InputPanelLayout{
		line:       "",
		inputLines: 3,
		footerRows: 1,
	}
	if _, ok := visibleInputPanelLayout(layout, true); ok {
		t.Fatalf("expected empty rendered input panel to be hidden")
	}
}

func TestVisibleInputPanelLayoutKeepsNonEmptyRenderedView(t *testing.T) {
	layout := InputPanelLayout{
		line:       "input",
		inputLines: 3,
		footerRows: 1,
	}
	visible, ok := visibleInputPanelLayout(layout, true)
	if !ok {
		t.Fatalf("expected non-empty rendered input panel to remain visible")
	}
	if got, want := visible.LineCount(), 4; got != want {
		t.Fatalf("expected visible layout line count %d, got %d", want, got)
	}
}
