package app

import "testing"

type testInputPanelFrame struct {
	inset   int
	render  string
	suffix  string
	prefix  string
	newline bool
}

func (f testInputPanelFrame) Render(content string) string {
	if f.render != "" {
		return f.render
	}
	out := content
	if f.prefix != "" {
		out = f.prefix + out
	}
	if f.newline {
		out += "\nframe-extra-line"
	}
	if f.suffix != "" {
		out += f.suffix
	}
	return out
}

func (f testInputPanelFrame) VerticalInsetLines() int {
	return f.inset
}

func TestBuildInputPanelLayoutUsesFrameInsetContractForGeometry(t *testing.T) {
	input := NewTextInput(40, TextInputConfig{Height: 2})
	input.SetValue("hello")

	layout := BuildInputPanelLayout(InputPanel{
		Input: input,
		Frame: testInputPanelFrame{
			inset:   0,
			newline: true, // Would inflate rendered height if geometry were inferred from output.
		},
	})
	if got, want := layout.InputLineCount(), input.Height(); got != want {
		t.Fatalf("expected input lines from frame inset contract: got %d want %d", got, want)
	}
}

func TestBuildInputPanelLayoutAddsFrameInsetAndFooterRows(t *testing.T) {
	input := NewTextInput(40, TextInputConfig{Height: 3})
	input.SetValue("hello")

	layout := BuildInputPanelLayout(InputPanel{
		Input: input,
		Frame: testInputPanelFrame{inset: 4},
		Footer: InputFooterFunc(func() string {
			return "line one\nline two"
		}),
	})

	if got, want := layout.InputLineCount(), input.Height()+4; got != want {
		t.Fatalf("expected input lines %d, got %d", want, got)
	}
	if got, want := layout.LineCount(), input.Height()+4+2; got != want {
		t.Fatalf("expected total line count %d, got %d", want, got)
	}
	footerRow, ok := layout.FooterStartRow()
	if !ok {
		t.Fatalf("expected footer start row")
	}
	if got, want := footerRow, input.Height()+4; got != want {
		t.Fatalf("expected footer row %d, got %d", want, got)
	}
}

func TestBuildInputPanelLayoutReturnsZeroLayoutForNilInput(t *testing.T) {
	layout := BuildInputPanelLayout(InputPanel{})
	if got := layout.LineCount(); got != 0 {
		t.Fatalf("expected zero line count, got %d", got)
	}
	if got := layout.InputLineCount(); got != 0 {
		t.Fatalf("expected zero input line count, got %d", got)
	}
	if _, ok := layout.FooterStartRow(); ok {
		t.Fatalf("did not expect footer row for nil input")
	}
}

func TestInputFooterFuncNilReturnsEmptyString(t *testing.T) {
	var footer InputFooterFunc
	if got := footer.InputFooter(); got != "" {
		t.Fatalf("expected nil footer func to return empty string, got %q", got)
	}
}

func TestMaxIntReturnsLargerValue(t *testing.T) {
	if got := maxInt(4, 2); got != 4 {
		t.Fatalf("expected maxInt to return first operand when larger, got %d", got)
	}
	if got := maxInt(2, 4); got != 4 {
		t.Fatalf("expected maxInt to return second operand when larger, got %d", got)
	}
}
