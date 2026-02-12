package app

import "strings"

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
}

func (p InputPanel) View() (line string, scrollable bool) {
	if p.Input == nil {
		return "", false
	}
	line = p.Input.View()
	if p.Footer != nil {
		if footer := p.Footer.InputFooter(); footer != "" {
			line += "\n" + footer
		}
	}
	return line, p.Input.CanScroll()
}
