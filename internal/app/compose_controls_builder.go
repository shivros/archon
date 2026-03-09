package app

import "strings"

type ComposeControlDescriptor struct {
	action composeControlAction
	kind   composeOptionKind
	label  string
	active bool
}

type ComposeInterruptDescriptor struct {
	label string
}

type ComposeControlsBuildInput struct {
	Controls  []ComposeControlDescriptor
	Interrupt *ComposeInterruptDescriptor
	Width     int
}

type ComposeControlsBuildOutput struct {
	Line  string
	Spans []composeControlSpan
}

type ComposeControlsBuilder interface {
	Build(input ComposeControlsBuildInput) ComposeControlsBuildOutput
}

type defaultComposeControlsBuilder struct{}

func WithComposeControlsBuilder(builder ComposeControlsBuilder) ModelOption {
	return func(m *Model) {
		if m == nil || m.chatAddonController == nil {
			return
		}
		m.chatAddonController.setComposeControlsBuilder(builder)
	}
}

func (defaultComposeControlsBuilder) Build(input ComposeControlsBuildInput) ComposeControlsBuildOutput {
	controls := sanitizeComposeControlDescriptors(input.Controls)
	spans := make([]composeControlSpan, 0, len(controls)+1)
	var b strings.Builder
	col := 0
	for i, control := range controls {
		if i > 0 {
			b.WriteString("  |  ")
			col += 5
		}
		label := control.label
		if control.active {
			label = "[" + label + "]"
		}
		start := col
		b.WriteString(label)
		col += len(label)
		spans = append(spans, composeControlSpan{
			action: control.action,
			kind:   control.kind,
			start:  start,
			end:    col - 1,
		})
	}
	line := b.String()
	interrupt := normalizeComposeInterruptDescriptor(input.Interrupt)
	if interrupt != nil {
		button := "[" + interrupt.label + "]"
		start := 0
		if input.Width > 0 {
			if line == "" {
				if input.Width > len(button) {
					line = strings.Repeat(" ", input.Width-len(button)) + button
					start = input.Width - len(button)
				} else {
					line = button
				}
			} else if len(line)+2+len(button) <= input.Width {
				spaces := input.Width - len(line) - len(button)
				if spaces < 2 {
					spaces = 2
				}
				line += strings.Repeat(" ", spaces) + button
				start = len(line) - len(button)
			} else {
				line += "  " + button
				start = len(line) - len(button)
			}
		} else {
			if line != "" {
				line += "  "
			}
			start = len(line)
			line += button
		}
		spans = append(spans, composeControlSpan{
			action: composeControlActionInterruptTurn,
			kind:   composeOptionNone,
			start:  start,
			end:    start + len(button) - 1,
		})
	}
	return ComposeControlsBuildOutput{Line: line, Spans: spans}
}

func sanitizeComposeControlDescriptors(controls []ComposeControlDescriptor) []ComposeControlDescriptor {
	if len(controls) == 0 {
		return nil
	}
	out := make([]ComposeControlDescriptor, 0, len(controls))
	for _, control := range controls {
		label := strings.TrimSpace(control.label)
		if label == "" {
			continue
		}
		control.label = label
		out = append(out, control)
	}
	return out
}

func normalizeComposeInterruptDescriptor(interrupt *ComposeInterruptDescriptor) *ComposeInterruptDescriptor {
	if interrupt == nil {
		return nil
	}
	label := strings.TrimSpace(interrupt.label)
	if label == "" {
		return nil
	}
	return &ComposeInterruptDescriptor{label: label}
}
