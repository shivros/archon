package app

import (
	"strings"
	"testing"
)

type stubComposeControlsBuilder struct {
	output ComposeControlsBuildOutput
}

func (s stubComposeControlsBuilder) Build(ComposeControlsBuildInput) ComposeControlsBuildOutput {
	return s.output
}

func TestWithComposeControlsBuilderConfiguresController(t *testing.T) {
	custom := stubComposeControlsBuilder{
		output: ComposeControlsBuildOutput{Line: "custom"},
	}
	m := NewModel(nil, WithComposeControlsBuilder(custom))
	m.mode = uiModeCompose
	m.newSession = &newSessionTarget{provider: "codex"}
	line := m.composeControlsLine()
	if line != "custom" {
		t.Fatalf("expected custom controls builder output, got %q", line)
	}

	WithComposeControlsBuilder(nil)(&m)
	line = m.composeControlsLine()
	if line == "custom" {
		t.Fatalf("expected reset to default controls builder")
	}
}

func TestWithComposeControlsBuilderHandlesNilModel(t *testing.T) {
	WithComposeControlsBuilder(stubComposeControlsBuilder{})(nil)
}

func findComposeControlSpanForAction(spans []composeControlSpan, action composeControlAction) (composeControlSpan, bool) {
	for _, span := range spans {
		if span.action == action {
			return span, true
		}
	}
	return composeControlSpan{}, false
}

func TestDefaultComposeControlsBuilderBuildsControlsAndInterrupt(t *testing.T) {
	builder := defaultComposeControlsBuilder{}
	out := builder.Build(ComposeControlsBuildInput{
		Controls: []ComposeControlDescriptor{
			{action: composeControlActionOpenOption, kind: composeOptionModel, label: "Model: gpt-5", active: true},
			{action: composeControlActionOpenOption, kind: composeOptionAccess, label: "Access: full"},
		},
		Interrupt: &ComposeInterruptDescriptor{label: "Interrupt"},
		Width:     70,
	})

	if !strings.Contains(out.Line, "[Model: gpt-5]") {
		t.Fatalf("expected active model control in output line, got %q", out.Line)
	}
	if !strings.HasSuffix(out.Line, "[Interrupt]") {
		t.Fatalf("expected interrupt button suffix, got %q", out.Line)
	}
	if len(out.Line) != 70 {
		t.Fatalf("expected width-aligned line length 70, got %d", len(out.Line))
	}
	if len(out.Spans) != 3 {
		t.Fatalf("expected 3 control spans, got %d", len(out.Spans))
	}
	interrupt, ok := findComposeControlSpanForAction(out.Spans, composeControlActionInterruptTurn)
	if !ok {
		t.Fatalf("expected interrupt span")
	}
	if got := out.Line[interrupt.start : interrupt.end+1]; got != "[Interrupt]" {
		t.Fatalf("unexpected interrupt span text %q", got)
	}
}

func TestDefaultComposeControlsBuilderBuildsInterruptOnly(t *testing.T) {
	builder := defaultComposeControlsBuilder{}
	out := builder.Build(ComposeControlsBuildInput{
		Interrupt: &ComposeInterruptDescriptor{label: "Interrupt"},
		Width:     24,
	})

	if got := strings.TrimSpace(out.Line); got != "[Interrupt]" {
		t.Fatalf("expected interrupt-only line, got %q", out.Line)
	}
	if len(out.Spans) != 1 || out.Spans[0].action != composeControlActionInterruptTurn {
		t.Fatalf("expected only interrupt span, got %#v", out.Spans)
	}
}

func TestDefaultComposeControlsBuilderSkipsBlankControls(t *testing.T) {
	builder := defaultComposeControlsBuilder{}
	out := builder.Build(ComposeControlsBuildInput{
		Controls: []ComposeControlDescriptor{
			{action: composeControlActionOpenOption, kind: composeOptionModel, label: "   "},
			{action: composeControlActionOpenOption, kind: composeOptionAccess, label: "Access: on-request"},
		},
	})

	if strings.Contains(out.Line, "  |  ") {
		t.Fatalf("did not expect separator for blank control, got %q", out.Line)
	}
	if out.Line != "Access: on-request" {
		t.Fatalf("unexpected line %q", out.Line)
	}
	if len(out.Spans) != 1 {
		t.Fatalf("expected one span, got %d", len(out.Spans))
	}
}

func TestDefaultComposeControlsBuilderInterruptKeepsLabelWhenWidthTooSmall(t *testing.T) {
	builder := defaultComposeControlsBuilder{}
	out := builder.Build(ComposeControlsBuildInput{
		Interrupt: &ComposeInterruptDescriptor{label: "Interrupt"},
		Width:     4,
	})

	if out.Line != "[Interrupt]" {
		t.Fatalf("expected full interrupt label without truncation, got %q", out.Line)
	}
	if len(out.Spans) != 1 || out.Spans[0].start != 0 {
		t.Fatalf("expected interrupt span at origin, got %#v", out.Spans)
	}
}

func TestDefaultComposeControlsBuilderIgnoresBlankInterruptLabel(t *testing.T) {
	builder := defaultComposeControlsBuilder{}
	out := builder.Build(ComposeControlsBuildInput{
		Controls: []ComposeControlDescriptor{
			{action: composeControlActionOpenOption, kind: composeOptionModel, label: "Model: gpt-5"},
		},
		Interrupt: &ComposeInterruptDescriptor{label: "   "},
	})

	if strings.Contains(out.Line, "Interrupt") {
		t.Fatalf("expected blank interrupt label to be ignored, got %q", out.Line)
	}
	if len(out.Spans) != 1 {
		t.Fatalf("expected only primary control span, got %#v", out.Spans)
	}
}
