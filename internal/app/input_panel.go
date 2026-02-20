package app

import (
	"strings"

	"charm.land/lipgloss/v2"
)

type InputFooterProvider interface {
	InputFooter() string
}

type InputFooterFunc func() string

func (f InputFooterFunc) InputFooter() string {
	if f == nil {
		return ""
	}
	return strings.TrimRight(f(), "\n")
}

type InputPanel struct {
	Input  *TextInput
	Footer InputFooterProvider
	Frame  InputPanelFrame
}

type InputPanelFrame interface {
	Render(content string) string
	VerticalInsetLines() int
}

type LipglossInputPanelFrame struct {
	Style lipgloss.Style
}

func (f LipglossInputPanelFrame) Render(content string) string {
	return f.Style.Render(content)
}

func (f LipglossInputPanelFrame) VerticalInsetLines() int {
	return f.Style.GetVerticalFrameSize()
}
