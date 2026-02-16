package app

import "testing"

func TestTextLayerComposerComposeOverlays(t *testing.T) {
	composer := NewTextLayerComposer()
	base := "line0\nline1\nline2\nline3"

	result := composer.Compose(base, []LayerOverlay{
		{Row: 1, Block: "A"},
		{Row: 2, Block: "B\nC"},
	})

	want := "line0\nA\nB\nC"
	if result != want {
		t.Fatalf("unexpected composed output:\nwant:\n%s\n\ngot:\n%s", want, result)
	}
}

func TestTextLayerComposerCombineHorizontal(t *testing.T) {
	composer := NewTextLayerComposer()
	left := "L1\nL2"
	right := "R1\nR2"

	result := composer.CombineHorizontal(left, right, 1)
	if result == "" {
		t.Fatalf("expected non-empty combined output")
	}
	if result == left || result == right {
		t.Fatalf("expected merged output, got %q", result)
	}
}
