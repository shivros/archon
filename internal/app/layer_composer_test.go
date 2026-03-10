package app

import "testing"

func TestTextLayerComposerComposeOverlays(t *testing.T) {
	composer := NewTextOverlayComposer()
	base := "0123456789\nabcdefghij\nklmnopqrst\nuvwxyzABCD"

	result := composer.Compose(base, []LayerOverlay{
		{X: 2, Y: 1, Block: "XX"},
		{X: 5, Y: 2, Block: "YY\nZZ"},
	})

	want := "0123456789\nabXXefghij\nklmnoYYrst\nuvwxyZZBCD"
	if result != want {
		t.Fatalf("unexpected composed output:\nwant:\n%s\n\ngot:\n%s", want, result)
	}
}

func TestTextLayerComposerComposeFullWidthOverlay(t *testing.T) {
	composer := NewTextOverlayComposer()
	base := "line0\nline1\nline2"

	result := composer.Compose(base, []LayerOverlay{
		{X: 0, Y: 1, Block: "ABCDE"},
	})

	want := "line0\nABCDE\nline2"
	if result != want {
		t.Fatalf("unexpected composed output:\nwant:\n%s\n\ngot:\n%s", want, result)
	}
}

func TestTextLayerComposerCombineHorizontal(t *testing.T) {
	joiner := NewDefaultBlockJoiner()
	left := "L1\nL2"
	right := "R1\nR2"

	result := joiner.CombineHorizontal(left, right, 1)
	if result == "" {
		t.Fatalf("expected non-empty combined output")
	}
	if result == left || result == right {
		t.Fatalf("expected merged output, got %q", result)
	}
}

func TestTextLayerComposerComposeNegativeXClipsOverlay(t *testing.T) {
	composer := NewTextOverlayComposer()
	base := "abcdef"

	result := composer.Compose(base, []LayerOverlay{
		{X: -2, Y: 0, Block: "WXYZ"},
	})

	if result != "YZcdef" {
		t.Fatalf("expected negative x clip result %q, got %q", "YZcdef", result)
	}
}

func TestTextLayerComposerComposeOutOfBoundsYIgnored(t *testing.T) {
	composer := NewTextOverlayComposer()
	base := "line0\nline1"

	result := composer.Compose(base, []LayerOverlay{
		{X: 0, Y: 3, Block: "ZZ"},
		{X: 0, Y: -2, Block: "YY"},
	})

	if result != base {
		t.Fatalf("expected out-of-bounds y overlay to be ignored, got %q", result)
	}
}

func TestTextLayerComposerComposeOverlayCanExtendPastBaseWidth(t *testing.T) {
	composer := NewTextOverlayComposer()
	base := "abc"

	result := composer.Compose(base, []LayerOverlay{
		{X: 1, Y: 0, Block: "WXYZ"},
	})

	if result != "aWXYZ" {
		t.Fatalf("expected overlay to extend base width, got %q", result)
	}
}

func TestTextLayerComposerComposeBlankOverlayLinePreservesBaseLine(t *testing.T) {
	composer := NewTextOverlayComposer()
	base := "line0\nline1"

	result := composer.Compose(base, []LayerOverlay{
		{X: 1, Y: 0, Block: "\nZZ"},
	})

	want := "line0\nlZZe1"
	if result != want {
		t.Fatalf("expected blank overlay line to preserve base line:\nwant=%q\ngot=%q", want, result)
	}
}

func TestNewTextLayerComposerCompatibilityBridge(t *testing.T) {
	composer := NewTextLayerComposer()
	if composer == nil {
		t.Fatalf("expected legacy text layer composer constructor to return a composer")
	}
	composed := composer.Compose("line0\nline1", []LayerOverlay{{X: 0, Y: 1, Block: "AB"}})
	if composed != "line0\nABne1" {
		t.Fatalf("unexpected legacy composer result %q", composed)
	}
	joined := composer.CombineHorizontal("L", "R", 1)
	if joined != "L R" {
		t.Fatalf("unexpected legacy composer join result %q", joined)
	}
}

func TestComposerOptionsIgnoreNilInputsAndWireDependencies(t *testing.T) {
	var nilModel *Model
	WithOverlayComposer(NewTextOverlayComposer())(nilModel)
	WithBlockJoiner(NewDefaultBlockJoiner())(nilModel)
	WithLayerComposer(NewTextLayerComposer())(nilModel)

	m := &Model{}
	WithOverlayComposer(nil)(m)
	WithBlockJoiner(nil)(m)
	WithLayerComposer(nil)(m)
	if m.overlayComposer != nil || m.overlayBlockJoiner != nil {
		t.Fatalf("expected nil composers/joiners to be ignored")
	}

	WithOverlayComposer(NewTextOverlayComposer())(m)
	if m.overlayComposer == nil {
		t.Fatalf("expected overlay composer to be wired")
	}

	WithBlockJoiner(NewDefaultBlockJoiner())(m)
	if m.overlayBlockJoiner == nil {
		t.Fatalf("expected block joiner to be wired")
	}
}

func TestTextCanvasStringNilReceiverReturnsEmpty(t *testing.T) {
	var canvas *textCanvas
	if got := canvas.String(); got != "" {
		t.Fatalf("expected nil canvas string to be empty, got %q", got)
	}
}
