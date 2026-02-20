package app

import "charm.land/lipgloss/v2"

type InputPanelLayout struct {
	line       string
	scrollable bool
	inputLines int
	footerRows int
}

func BuildInputPanelLayout(panel InputPanel) InputPanelLayout {
	if panel.Input == nil {
		return InputPanelLayout{}
	}

	inputView := panel.Input.View()
	inputLines := maxInt(1, panel.Input.Height())
	if panel.Frame != nil {
		inputView = panel.Frame.Render(inputView)
		inputLines += maxInt(0, panel.Frame.VerticalInsetLines())
	}

	footer := ""
	if panel.Footer != nil {
		footer = panel.Footer.InputFooter()
	}

	layout := InputPanelLayout{
		line:       inputView,
		scrollable: panel.Input.CanScroll(),
		inputLines: inputLines,
	}
	if footer == "" {
		return layout
	}

	layout.footerRows = maxInt(1, lipgloss.Height(footer))
	layout.line += "\n" + footer
	return layout
}

func (l InputPanelLayout) View() (line string, scrollable bool) {
	return l.line, l.scrollable
}

func (l InputPanelLayout) LineCount() int {
	return l.inputLines + l.footerRows
}

func (l InputPanelLayout) InputLineCount() int {
	return l.inputLines
}

func (l InputPanelLayout) FooterStartRow() (int, bool) {
	if l.footerRows == 0 {
		return 0, false
	}
	return l.inputLines, true
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
